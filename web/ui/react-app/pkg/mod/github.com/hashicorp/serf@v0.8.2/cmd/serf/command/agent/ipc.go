package agent

/*
 The agent exposes an IPC mechanism that is used for both controlling
 Serf as well as providing a fast streaming mechanism for events. This
 allows other applications to easily leverage Serf as the event layer.

 We additionally make use of the IPC layer to also handle RPC calls from
 the CLI to unify the code paths. This results in a split Request/Response
 as well as streaming mode of operation.

 The system is fairly simple, each client opens a TCP connection to the
 agent. The connection is initialized with a handshake which establishes
 the protocol version being used. This is to allow for future changes to
 the protocol.

 Once initialized, clients send commands and wait for responses. Certain
 commands will cause the client to subscribe to events, and those will be
 pushed down the socket as they are received. This provides a low-latency
 mechanism for applications to send and receive events, while also providing
 a flexible control mechanism for Serf.
*/

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/armon/go-metrics"
	"github.com/hashicorp/go-msgpack/codec"
	"github.com/hashicorp/logutils"
	"github.com/hashicorp/serf/coordinate"
	"github.com/hashicorp/serf/serf"
)

const (
	MinIPCVersion = 1
	MaxIPCVersion = 1
)

const (
	handshakeCommand       = "handshake"
	eventCommand           = "event"
	forceLeaveCommand      = "force-leave"
	joinCommand            = "join"
	membersCommand         = "members"
	membersFilteredCommand = "members-filtered"
	streamCommand          = "stream"
	stopCommand            = "stop"
	monitorCommand         = "monitor"
	leaveCommand           = "leave"
	installKeyCommand      = "install-key"
	useKeyCommand          = "use-key"
	removeKeyCommand       = "remove-key"
	listKeysCommand        = "list-keys"
	tagsCommand            = "tags"
	queryCommand           = "query"
	respondCommand         = "respond"
	authCommand            = "auth"
	statsCommand           = "stats"
	getCoordinateCommand   = "get-coordinate"
)

const (
	unsupportedCommand    = "Unsupported command"
	unsupportedIPCVersion = "Unsupported IPC version"
	duplicateHandshake    = "Handshake already performed"
	handshakeRequired     = "Handshake required"
	monitorExists         = "Monitor already exists"
	invalidFilter         = "Invalid event filter"
	streamExists          = "Stream with given sequence exists"
	invalidQueryID        = "No pending queries matching ID"
	authRequired          = "Authentication required"
	invalidAuthToken      = "Invalid authentication token"
)

const (
	queryRecordAck      = "ack"
	queryRecordResponse = "response"
	queryRecordDone     = "done"
)

// Request header is sent before each request
type requestHeader struct {
	Command string
	Seq     uint64
}

// Response header is sent before each response
type responseHeader struct {
	Seq   uint64
	Error string
}

type handshakeRequest struct {
	Version int32
}

type authRequest struct {
	AuthKey string
}

type coordinateRequest struct {
	Node string
}

type coordinateResponse struct {
	Coord coordinate.Coordinate
	Ok    bool
}

type eventRequest struct {
	Name     string
	Payload  []byte
	Coalesce bool
}

type forceLeaveRequest struct {
	Node string
}

type joinRequest struct {
	Existing []string
	Replay   bool
}

type joinResponse struct {
	Num int32
}

type membersFilteredRequest struct {
	Tags   map[string]string
	Status string
	Name   string
}

type membersResponse struct {
	Members []Member
}

type keyRequest struct {
	Key string
}

type keyResponse struct {
	Messages map[string]string
	Keys     map[string]int
	NumNodes int
	NumErr   int
	NumResp  int
}

type monitorRequest struct {
	LogLevel string
}

type streamRequest struct {
	Type string
}

type stopRequest struct {
	Stop uint64
}

type tagsRequest struct {
	Tags       map[string]string
	DeleteTags []string
}

type queryRequest struct {
	FilterNodes []string
	FilterTags  map[string]string
	RequestAck  bool
	RelayFactor uint8
	Timeout     time.Duration
	Name        string
	Payload     []byte
}

type respondRequest struct {
	ID      uint64
	Payload []byte
}

type queryRecord struct {
	Type    string
	From    string
	Payload []byte
}

type logRecord struct {
	Log string
}

type userEventRecord struct {
	Event    string
	LTime    serf.LamportTime
	Name     string
	Payload  []byte
	Coalesce bool
}

type queryEventRecord struct {
	Event   string
	ID      uint64 // ID is opaque to client, used to respond
	LTime   serf.LamportTime
	Name    string
	Payload []byte
}

type Member struct {
	Name        string
	Addr        net.IP
	Port        uint16
	Tags        map[string]string
	Status      string
	ProtocolMin uint8
	ProtocolMax uint8
	ProtocolCur uint8
	DelegateMin uint8
	DelegateMax uint8
	DelegateCur uint8
}

type memberEventRecord struct {
	Event   string
	Members []Member
}

type AgentIPC struct {
	sync.Mutex
	agent     *Agent
	authKey   string
	clients   map[string]*IPCClient
	listener  net.Listener
	logger    *log.Logger
	logWriter *logWriter
	stop      bool
	stopCh    chan struct{}
}

type IPCClient struct {
	queryID      uint64 // Used to increment query IDs
	name         string
	conn         net.Conn
	reader       *bufio.Reader
	writer       *bufio.Writer
	dec          *codec.Decoder
	enc          *codec.Encoder
	writeLock    sync.Mutex
	version      int32 // From the handshake, 0 before
	logStreamer  *logStream
	eventStreams map[uint64]*eventStream

	pendingQueries map[uint64]*serf.Query
	queryLock      sync.Mutex

	didAuth bool // Did we get an auth token yet?
}

// send is used to send an object using the MsgPack encoding. send
// is serialized to prevent write overlaps, while properly buffering.
func (c *IPCClient) Send(header *responseHeader, obj interface{}) error {
	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	if err := c.enc.Encode(header); err != nil {
		return err
	}

	if obj != nil {
		if err := c.enc.Encode(obj); err != nil {
			return err
		}
	}

	if err := c.writer.Flush(); err != nil {
		return err
	}

	return nil
}

func (c *IPCClient) String() string {
	return fmt.Sprintf("ipc.client: %v", c.conn.RemoteAddr())
}

// nextQueryID safely generates a new query ID
func (c *IPCClient) nextQueryID() uint64 {
	return atomic.AddUint64(&c.queryID, 1)
}

// RegisterQuery is used to register a pending query that may
// get a response. The ID of the query is returned
func (c *IPCClient) RegisterQuery(q *serf.Query) uint64 {
	// Generate a unique-per-client ID
	id := c.nextQueryID()

	// Ensure the query deadline is in the future
	timeout := q.Deadline().Sub(time.Now())
	if timeout < 0 {
		return id
	}

	// Register the query
	c.queryLock.Lock()
	c.pendingQueries[id] = q
	c.queryLock.Unlock()

	// Setup a timer to deregister after the timeout
	time.AfterFunc(timeout, func() {
		c.queryLock.Lock()
		delete(c.pendingQueries, id)
		c.queryLock.Unlock()
	})
	return id
}

// NewAgentIPC is used to create a new Agent IPC handler
func NewAgentIPC(agent *Agent, authKey string, listener net.Listener,
	logOutput io.Writer, logWriter *logWriter) *AgentIPC {
	if logOutput == nil {
		logOutput = os.Stderr
	}
	ipc := &AgentIPC{
		agent:     agent,
		authKey:   authKey,
		clients:   make(map[string]*IPCClient),
		listener:  listener,
		logger:    log.New(logOutput, "", log.LstdFlags),
		logWriter: logWriter,
		stopCh:    make(chan struct{}),
	}
	go ipc.listen()
	return ipc
}

// Shutdown is used to shutdown the IPC layer
func (i *AgentIPC) Shutdown() {
	i.Lock()
	defer i.Unlock()

	if i.stop {
		return
	}

	i.stop = true
	close(i.stopCh)
	i.listener.Close()

	// Close the existing connections
	for _, client := range i.clients {
		client.conn.Close()
	}
}

// listen is a long running routine that listens for new clients
func (i *AgentIPC) listen() {
	for {
		conn, err := i.listener.Accept()
		if err != nil {
			if i.stop {
				return
			}
			i.logger.Printf("[ERR] agent.ipc: Failed to accept client: %v", err)
			continue
		}
		i.logger.Printf("[INFO] agent.ipc: Accepted client: %v", conn.RemoteAddr())
		metrics.IncrCounter([]string{"agent", "ipc", "accept"}, 1)

		// Wrap the connection in a client
		client := &IPCClient{
			name:           conn.RemoteAddr().String(),
			conn:           conn,
			reader:         bufio.NewReader(conn),
			writer:         bufio.NewWriter(conn),
			eventStreams:   make(map[uint64]*eventStream),
			pendingQueries: make(map[uint64]*serf.Query),
		}
		client.dec = codec.NewDecoder(client.reader,
			&codec.MsgpackHandle{RawToString: true, WriteExt: true})
		client.enc = codec.NewEncoder(client.writer,
			&codec.MsgpackHandle{RawToString: true, WriteExt: true})

		// Register the client
		i.Lock()
		if !i.stop {
			i.clients[client.name] = client
			go i.handleClient(client)
		} else {
			conn.Close()
		}
		i.Unlock()
	}
}

// deregisterClient is called to cleanup after a client disconnects
func (i *AgentIPC) deregisterClient(client *IPCClient) {
	// Close the socket
	client.conn.Close()

	// Remove from the clients list
	i.Lock()
	delete(i.clients, client.name)
	i.Unlock()

	// Remove from the log writer
	if client.logStreamer != nil {
		i.logWriter.DeregisterHandler(client.logStreamer)
		client.logStreamer.Stop()
	}

	// Remove from event handlers
	for _, es := range client.eventStreams {
		i.agent.DeregisterEventHandler(es)
		es.Stop()
	}
}

// handleClient is a long running routine that handles a single client
func (i *AgentIPC) handleClient(client *IPCClient) {
	defer i.deregisterClient(client)
	var reqHeader requestHeader
	for {
		// Decode the header
		if err := client.dec.Decode(&reqHeader); err != nil {
			if !i.stop {
				// The second part of this if is to block socket
				// errors from Windows which appear to happen every
				// time there is an EOF.
				if err != io.EOF && !strings.Contains(strings.ToLower(err.Error()), "wsarecv") {
					i.logger.Printf("[ERR] agent.ipc: failed to decode request header: %v", err)
				}
			}
			return
		}

		// Evaluate the command
		if err := i.handleRequest(client, &reqHeader); err != nil {
			i.logger.Printf("[ERR] agent.ipc: Failed to evaluate request: %v", err)
			return
		}
	}
}

// handleRequest is used to evaluate a single client command
func (i *AgentIPC) handleRequest(client *IPCClient, reqHeader *requestHeader) error {
	// Look for a command field
	command := reqHeader.Command
	seq := reqHeader.Seq

	// Ensure the handshake is performed before other commands
	if command != handshakeCommand && client.version == 0 {
		respHeader := responseHeader{Seq: seq, Error: handshakeRequired}
		client.Send(&respHeader, nil)
		return fmt.Errorf(handshakeRequired)
	}
	metrics.IncrCounter([]string{"agent", "ipc", "command"}, 1)

	// Ensure the client has authenticated after the handshake if necessary
	if i.authKey != "" && !client.didAuth && command != authCommand && command != handshakeCommand {
		i.logger.Printf("[WARN] agent.ipc: Client sending commands before auth")
		respHeader := responseHeader{Seq: seq, Error: authRequired}
		client.Send(&respHeader, nil)
		return nil
	}

	// Dispatch command specific handlers
	switch command {
	case handshakeCommand:
		return i.handleHandshake(client, seq)

	case authCommand:
		return i.handleAuth(client, seq)

	case eventCommand:
		return i.handleEvent(client, seq)

	case membersCommand, membersFilteredCommand:
		return i.handleMembers(client, command, seq)

	case streamCommand:
		return i.handleStream(client, seq)

	case monitorCommand:
		return i.handleMonitor(client, seq)

	case stopCommand:
		return i.handleStop(client, seq)

	case forceLeaveCommand:
		return i.handleForceLeave(client, seq)

	case joinCommand:
		return i.handleJoin(client, seq)

	case leaveCommand:
		return i.handleLeave(client, seq)

	case installKeyCommand:
		return i.handleInstallKey(client, seq)

	case useKeyCommand:
		return i.handleUseKey(client, seq)

	case removeKeyCommand:
		return i.handleRemoveKey(client, seq)

	case listKeysCommand:
		return i.handleListKeys(client, seq)

	case tagsCommand:
		return i.handleTags(client, seq)

	case queryCommand:
		return i.handleQuery(client, seq)

	case respondCommand:
		return i.handleRespond(client, seq)

	case statsCommand:
		return i.handleStats(client, seq)

	case getCoordinateCommand:
		return i.handleGetCoordinate(client, seq)

	default:
		respHeader := responseHeader{Seq: seq, Error: unsupportedCommand}
		client.Send(&respHeader, nil)
		return fmt.Errorf("command '%s' not recognized", command)
	}
}

func (i *AgentIPC) handleHandshake(client *IPCClient, seq uint64) error {
	var req handshakeRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	resp := responseHeader{
		Seq:   seq,
		Error: "",
	}

	// Check the version
	if req.Version < MinIPCVersion || req.Version > MaxIPCVersion {
		resp.Error = unsupportedIPCVersion
	} else if client.version != 0 {
		resp.Error = duplicateHandshake
	} else {
		client.version = req.Version
	}
	return client.Send(&resp, nil)
}

func (i *AgentIPC) handleAuth(client *IPCClient, seq uint64) error {
	var req authRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	resp := responseHeader{
		Seq:   seq,
		Error: "",
	}

	// Check the token matches
	if req.AuthKey == i.authKey {
		client.didAuth = true
	} else {
		resp.Error = invalidAuthToken
	}
	return client.Send(&resp, nil)
}

func (i *AgentIPC) handleEvent(client *IPCClient, seq uint64) error {
	var req eventRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	// Attempt the send
	err := i.agent.UserEvent(req.Name, req.Payload, req.Coalesce)

	// Respond
	resp := responseHeader{
		Seq:   seq,
		Error: errToString(err),
	}
	return client.Send(&resp, nil)
}

func (i *AgentIPC) handleForceLeave(client *IPCClient, seq uint64) error {
	var req forceLeaveRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	// Attempt leave
	err := i.agent.ForceLeave(req.Node)

	// Respond
	resp := responseHeader{
		Seq:   seq,
		Error: errToString(err),
	}
	return client.Send(&resp, nil)
}

func (i *AgentIPC) handleJoin(client *IPCClient, seq uint64) error {
	var req joinRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	// Attempt the join
	num, err := i.agent.Join(req.Existing, req.Replay)

	// Respond
	header := responseHeader{
		Seq:   seq,
		Error: errToString(err),
	}
	resp := joinResponse{
		Num: int32(num),
	}
	return client.Send(&header, &resp)
}

func (i *AgentIPC) handleMembers(client *IPCClient, command string, seq uint64) error {
	serf := i.agent.Serf()
	raw := serf.Members()
	members := make([]Member, 0, len(raw))

	if command == membersFilteredCommand {
		var req membersFilteredRequest
		err := client.dec.Decode(&req)
		if err != nil {
			return fmt.Errorf("decode failed: %v", err)
		}
		raw, err = i.filterMembers(raw, req.Tags, req.Status, req.Name)
		if err != nil {
			return err
		}
	}

	for _, m := range raw {
		sm := Member{
			Name:        m.Name,
			Addr:        m.Addr,
			Port:        m.Port,
			Tags:        m.Tags,
			Status:      m.Status.String(),
			ProtocolMin: m.ProtocolMin,
			ProtocolMax: m.ProtocolMax,
			ProtocolCur: m.ProtocolCur,
			DelegateMin: m.DelegateMin,
			DelegateMax: m.DelegateMax,
			DelegateCur: m.DelegateCur,
		}
		members = append(members, sm)
	}

	header := responseHeader{
		Seq:   seq,
		Error: "",
	}
	resp := membersResponse{
		Members: members,
	}
	return client.Send(&header, &resp)
}

func (i *AgentIPC) filterMembers(members []serf.Member, tags map[string]string,
	status string, name string) ([]serf.Member, error) {

	result := make([]serf.Member, 0, len(members))

	// Pre-compile all the regular expressions
	tagsRe := make(map[string]*regexp.Regexp)
	for tag, expr := range tags {
		re, err := regexp.Compile(fmt.Sprintf("^%s$", expr))
		if err != nil {
			return nil, fmt.Errorf("Failed to compile regex: %v", err)
		}
		tagsRe[tag] = re
	}

	statusRe, err := regexp.Compile(fmt.Sprintf("^%s$", status))
	if err != nil {
		return nil, fmt.Errorf("Failed to compile regex: %v", err)
	}

	nameRe, err := regexp.Compile(fmt.Sprintf("^%s$", name))
	if err != nil {
		return nil, fmt.Errorf("Failed to compile regex: %v", err)
	}

OUTER:
	for _, m := range members {
		// Check if tags were passed, and if they match
		for tag := range tags {
			if !tagsRe[tag].MatchString(m.Tags[tag]) {
				continue OUTER
			}
		}

		// Check if status matches
		if status != "" && !statusRe.MatchString(m.Status.String()) {
			continue
		}

		// Check if node name matches
		if name != "" && !nameRe.MatchString(m.Name) {
			continue
		}

		// Made it past the filters!
		result = append(result, m)
	}

	return result, nil
}

func (i *AgentIPC) handleInstallKey(client *IPCClient, seq uint64) error {
	var req keyRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	queryResp, err := i.agent.InstallKey(req.Key)

	header := responseHeader{
		Seq:   seq,
		Error: errToString(err),
	}
	resp := keyResponse{
		Messages: queryResp.Messages,
		NumNodes: queryResp.NumNodes,
		NumErr:   queryResp.NumErr,
		NumResp:  queryResp.NumResp,
	}

	return client.Send(&header, &resp)
}

func (i *AgentIPC) handleUseKey(client *IPCClient, seq uint64) error {
	var req keyRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	queryResp, err := i.agent.UseKey(req.Key)

	header := responseHeader{
		Seq:   seq,
		Error: errToString(err),
	}
	resp := keyResponse{
		Messages: queryResp.Messages,
		NumNodes: queryResp.NumNodes,
		NumErr:   queryResp.NumErr,
		NumResp:  queryResp.NumResp,
	}

	return client.Send(&header, &resp)
}

func (i *AgentIPC) handleRemoveKey(client *IPCClient, seq uint64) error {
	var req keyRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	queryResp, err := i.agent.RemoveKey(req.Key)

	header := responseHeader{
		Seq:   seq,
		Error: errToString(err),
	}
	resp := keyResponse{
		Messages: queryResp.Messages,
		NumNodes: queryResp.NumNodes,
		NumErr:   queryResp.NumErr,
		NumResp:  queryResp.NumResp,
	}

	return client.Send(&header, &resp)
}

func (i *AgentIPC) handleListKeys(client *IPCClient, seq uint64) error {
	queryResp, err := i.agent.ListKeys()

	header := responseHeader{
		Seq:   seq,
		Error: errToString(err),
	}
	resp := keyResponse{
		Messages: queryResp.Messages,
		Keys:     queryResp.Keys,
		NumNodes: queryResp.NumNodes,
		NumErr:   queryResp.NumErr,
		NumResp:  queryResp.NumResp,
	}

	return client.Send(&header, &resp)
}

func (i *AgentIPC) handleStream(client *IPCClient, seq uint64) error {
	var es *eventStream
	var req streamRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	resp := responseHeader{
		Seq:   seq,
		Error: "",
	}

	// Create the event filters
	filters := ParseEventFilter(req.Type)
	for _, f := range filters {
		if !f.Valid() {
			resp.Error = invalidFilter
			goto SEND
		}
	}

	// Check if there is an existing stream
	if _, ok := client.eventStreams[seq]; ok {
		resp.Error = streamExists
		goto SEND
	}

	// Create an event streamer
	es = newEventStream(client, filters, seq, i.logger)
	client.eventStreams[seq] = es

	// Register with the agent. Defer so that we can respond before
	// registration, avoids any possible race condition
	defer i.agent.RegisterEventHandler(es)

SEND:
	return client.Send(&resp, nil)
}

func (i *AgentIPC) handleMonitor(client *IPCClient, seq uint64) error {
	var req monitorRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	resp := responseHeader{
		Seq:   seq,
		Error: "",
	}

	// Upper case the log level
	req.LogLevel = strings.ToUpper(req.LogLevel)

	// Create a level filter
	filter := LevelFilter()
	filter.MinLevel = logutils.LogLevel(req.LogLevel)
	if !ValidateLevelFilter(filter.MinLevel, filter) {
		resp.Error = fmt.Sprintf("Unknown log level: %s", filter.MinLevel)
		goto SEND
	}

	// Check if there is an existing monitor
	if client.logStreamer != nil {
		resp.Error = monitorExists
		goto SEND
	}

	// Create a log streamer
	client.logStreamer = newLogStream(client, filter, seq, i.logger)

	// Register with the log writer. Defer so that we can respond before
	// registration, avoids any possible race condition
	defer i.logWriter.RegisterHandler(client.logStreamer)

SEND:
	return client.Send(&resp, nil)
}

func (i *AgentIPC) handleStop(client *IPCClient, seq uint64) error {
	var req stopRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	// Remove a log monitor if any
	if client.logStreamer != nil && client.logStreamer.seq == req.Stop {
		i.logWriter.DeregisterHandler(client.logStreamer)
		client.logStreamer.Stop()
		client.logStreamer = nil
	}

	// Remove an event stream if any
	if es, ok := client.eventStreams[req.Stop]; ok {
		i.agent.DeregisterEventHandler(es)
		es.Stop()
		delete(client.eventStreams, req.Stop)
	}

	// Always succeed
	resp := responseHeader{Seq: seq, Error: ""}
	return client.Send(&resp, nil)
}

func (i *AgentIPC) handleLeave(client *IPCClient, seq uint64) error {
	i.logger.Printf("[INFO] agent.ipc: Graceful leave triggered")

	// Do the leave
	err := i.agent.Leave()
	if err != nil {
		i.logger.Printf("[ERR] agent.ipc: leave failed: %v", err)
	}
	resp := responseHeader{Seq: seq, Error: errToString(err)}

	// Send and wait
	err = client.Send(&resp, nil)

	// Trigger a shutdown!
	if err := i.agent.Shutdown(); err != nil {
		i.logger.Printf("[ERR] agent.ipc: shutdown failed: %v", err)
	}
	return err
}

func (i *AgentIPC) handleTags(client *IPCClient, seq uint64) error {
	var req tagsRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	tags := make(map[string]string)

	for key, val := range i.agent.SerfConfig().Tags {
		var delTag bool
		for _, delkey := range req.DeleteTags {
			delTag = (delTag || delkey == key)
		}
		if !delTag {
			tags[key] = val
		}
	}

	for key, val := range req.Tags {
		tags[key] = val
	}

	err := i.agent.SetTags(tags)

	resp := responseHeader{Seq: seq, Error: errToString(err)}
	return client.Send(&resp, nil)
}

func (i *AgentIPC) handleQuery(client *IPCClient, seq uint64) error {
	var req queryRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	// Setup the query
	params := serf.QueryParam{
		FilterNodes: req.FilterNodes,
		FilterTags:  req.FilterTags,
		RequestAck:  req.RequestAck,
		RelayFactor: req.RelayFactor,
		Timeout:     req.Timeout,
	}

	// Start the query
	queryResp, err := i.agent.Query(req.Name, req.Payload, &params)

	// Stream the query responses
	if err == nil {
		qs := newQueryResponseStream(client, seq, i.logger)
		defer func() {
			go qs.Stream(queryResp)
		}()
	}

	// Respond
	resp := responseHeader{
		Seq:   seq,
		Error: errToString(err),
	}
	return client.Send(&resp, nil)
}

func (i *AgentIPC) handleRespond(client *IPCClient, seq uint64) error {
	var req respondRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	// Lookup the query
	client.queryLock.Lock()
	query, ok := client.pendingQueries[req.ID]
	client.queryLock.Unlock()

	// Respond if we have a pending query
	var err error
	if ok {
		err = query.Respond(req.Payload)
	} else {
		err = fmt.Errorf(invalidQueryID)
	}

	// Respond
	resp := responseHeader{
		Seq:   seq,
		Error: errToString(err),
	}
	return client.Send(&resp, nil)
}

// handleStats is used to get various statistics
func (i *AgentIPC) handleStats(client *IPCClient, seq uint64) error {
	header := responseHeader{
		Seq:   seq,
		Error: "",
	}
	resp := i.agent.Stats()
	return client.Send(&header, resp)
}

// handleGetCoordinate is used to get the cached coordinate for a node.
func (i *AgentIPC) handleGetCoordinate(client *IPCClient, seq uint64) error {
	var req coordinateRequest
	if err := client.dec.Decode(&req); err != nil {
		return fmt.Errorf("decode failed: %v", err)
	}

	// Fetch the coordinate.
	var result coordinate.Coordinate
	coord, ok := i.agent.Serf().GetCachedCoordinate(req.Node)
	if ok {
		result = *coord
	}

	// Respond
	header := responseHeader{
		Seq:   seq,
		Error: errToString(nil),
	}
	resp := coordinateResponse{
		Coord: result,
		Ok:    ok,
	}
	return client.Send(&header, &resp)
}

// Used to convert an error to a string representation
func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
