package client

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-msgpack/codec"
	"github.com/hashicorp/logutils"
	"github.com/hashicorp/serf/coordinate"
)

const (
	// This is the default IO timeout for the client
	DefaultTimeout = 10 * time.Second
)

var (
	clientClosed = fmt.Errorf("client closed")
)

type seqCallback struct {
	handler func(*responseHeader)
}

func (sc *seqCallback) Handle(resp *responseHeader) {
	sc.handler(resp)
}
func (sc *seqCallback) Cleanup() {}

// seqHandler interface is used to handle responses
type seqHandler interface {
	Handle(*responseHeader)
	Cleanup()
}

// Config is provided to ClientFromConfig to make
// a new RPCClient from the given configuration
type Config struct {
	// Addr must be the RPC address to contact
	Addr string

	// If provided, the client will perform key based auth
	AuthKey string

	// If provided, overrides the DefaultTimeout used for
	// IO deadlines
	Timeout time.Duration
}

// RPCClient is used to make requests to the Agent using an RPC mechanism.
// Additionally, the client manages event streams and monitors, enabling a client
// to easily receive event notifications instead of using the fork/exec mechanism.
type RPCClient struct {
	seq uint64

	timeout   time.Duration
	conn      *net.TCPConn
	reader    *bufio.Reader
	writer    *bufio.Writer
	dec       *codec.Decoder
	enc       *codec.Encoder
	writeLock sync.Mutex

	dispatch     map[uint64]seqHandler
	dispatchLock sync.Mutex

	shutdown     bool
	shutdownCh   chan struct{}
	shutdownLock sync.Mutex
}

// send is used to send an object using the MsgPack encoding. send
// is serialized to prevent write overlaps, while properly buffering.
func (c *RPCClient) send(header *requestHeader, obj interface{}) error {
	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	if c.shutdown {
		return clientClosed
	}

	// Setup an IO deadline, this way we won't wait indefinitely
	// if the client has hung.
	if err := c.conn.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
		return err
	}

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

// NewRPCClient is used to create a new RPC client given the
// RPC address of the Serf agent. This will return a client,
// or an error if the connection could not be established.
// This will use the DefaultTimeout for the client.
func NewRPCClient(addr string) (*RPCClient, error) {
	conf := Config{Addr: addr}
	return ClientFromConfig(&conf)
}

// ClientFromConfig is used to create a new RPC client given the
// configuration object. This will return a client, or an error if
// the connection could not be established.
func ClientFromConfig(c *Config) (*RPCClient, error) {
	// Setup the defaults
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}

	// Try to dial to serf
	conn, err := net.DialTimeout("tcp", c.Addr, c.Timeout)
	if err != nil {
		return nil, err
	}

	// Create the client
	client := &RPCClient{
		seq:        0,
		timeout:    c.Timeout,
		conn:       conn.(*net.TCPConn),
		reader:     bufio.NewReader(conn),
		writer:     bufio.NewWriter(conn),
		dispatch:   make(map[uint64]seqHandler),
		shutdownCh: make(chan struct{}),
	}
	client.dec = codec.NewDecoder(client.reader,
		&codec.MsgpackHandle{RawToString: true, WriteExt: true})
	client.enc = codec.NewEncoder(client.writer,
		&codec.MsgpackHandle{RawToString: true, WriteExt: true})
	go client.listen()

	// Do the initial handshake
	if err := client.handshake(); err != nil {
		client.Close()
		return nil, err
	}

	// Do the initial authentication if needed
	if c.AuthKey != "" {
		if err := client.auth(c.AuthKey); err != nil {
			client.Close()
			return nil, err
		}
	}

	return client, err
}

// StreamHandle is an opaque handle passed to stop to stop streaming
type StreamHandle uint64

func (c *RPCClient) IsClosed() bool {
	return c.shutdown
}

// Close is used to free any resources associated with the client
func (c *RPCClient) Close() error {
	c.shutdownLock.Lock()
	defer c.shutdownLock.Unlock()

	if !c.shutdown {
		c.shutdown = true
		close(c.shutdownCh)
		c.deregisterAll()
		return c.conn.Close()
	}
	return nil
}

// ForceLeave is used to ask the agent to issue a leave command for
// a given node
func (c *RPCClient) ForceLeave(node string) error {
	header := requestHeader{
		Command: forceLeaveCommand,
		Seq:     c.getSeq(),
	}
	req := forceLeaveRequest{
		Node: node,
	}
	return c.genericRPC(&header, &req, nil)
}

// Join is used to instruct the agent to attempt a join
func (c *RPCClient) Join(addrs []string, replay bool) (int, error) {
	header := requestHeader{
		Command: joinCommand,
		Seq:     c.getSeq(),
	}
	req := joinRequest{
		Existing: addrs,
		Replay:   replay,
	}
	var resp joinResponse

	err := c.genericRPC(&header, &req, &resp)
	return int(resp.Num), err
}

// Members is used to fetch a list of known members
func (c *RPCClient) Members() ([]Member, error) {
	header := requestHeader{
		Command: membersCommand,
		Seq:     c.getSeq(),
	}
	var resp membersResponse

	err := c.genericRPC(&header, nil, &resp)
	return resp.Members, err
}

// MembersFiltered returns a subset of members
func (c *RPCClient) MembersFiltered(tags map[string]string, status string,
	name string) ([]Member, error) {
	header := requestHeader{
		Command: membersFilteredCommand,
		Seq:     c.getSeq(),
	}
	req := membersFilteredRequest{
		Tags:   tags,
		Status: status,
		Name:   name,
	}
	var resp membersResponse

	err := c.genericRPC(&header, &req, &resp)
	return resp.Members, err
}

// UserEvent is used to trigger sending an event
func (c *RPCClient) UserEvent(name string, payload []byte, coalesce bool) error {
	header := requestHeader{
		Command: eventCommand,
		Seq:     c.getSeq(),
	}
	req := eventRequest{
		Name:     name,
		Payload:  payload,
		Coalesce: coalesce,
	}
	return c.genericRPC(&header, &req, nil)
}

// Leave is used to trigger a graceful leave and shutdown of the agent
func (c *RPCClient) Leave() error {
	header := requestHeader{
		Command: leaveCommand,
		Seq:     c.getSeq(),
	}
	return c.genericRPC(&header, nil, nil)
}

// UpdateTags will modify the tags on a running serf agent
func (c *RPCClient) UpdateTags(tags map[string]string, delTags []string) error {
	header := requestHeader{
		Command: tagsCommand,
		Seq:     c.getSeq(),
	}
	req := tagsRequest{
		Tags:       tags,
		DeleteTags: delTags,
	}
	return c.genericRPC(&header, &req, nil)
}

// Respond allows a client to respond to a query event. The ID is the
// ID of the Query to respond to, and the given payload is the response.
func (c *RPCClient) Respond(id uint64, buf []byte) error {
	header := requestHeader{
		Command: respondCommand,
		Seq:     c.getSeq(),
	}
	req := respondRequest{
		ID:      id,
		Payload: buf,
	}
	return c.genericRPC(&header, &req, nil)
}

// IntallKey installs a new encryption key onto the keyring
func (c *RPCClient) InstallKey(key string) (map[string]string, error) {
	header := requestHeader{
		Command: installKeyCommand,
		Seq:     c.getSeq(),
	}
	req := keyRequest{
		Key: key,
	}

	resp := keyResponse{}
	err := c.genericRPC(&header, &req, &resp)

	return resp.Messages, err
}

// UseKey changes the primary encryption key on the keyring
func (c *RPCClient) UseKey(key string) (map[string]string, error) {
	header := requestHeader{
		Command: useKeyCommand,
		Seq:     c.getSeq(),
	}
	req := keyRequest{
		Key: key,
	}

	resp := keyResponse{}
	err := c.genericRPC(&header, &req, &resp)

	return resp.Messages, err
}

// RemoveKey changes the primary encryption key on the keyring
func (c *RPCClient) RemoveKey(key string) (map[string]string, error) {
	header := requestHeader{
		Command: removeKeyCommand,
		Seq:     c.getSeq(),
	}
	req := keyRequest{
		Key: key,
	}

	resp := keyResponse{}
	err := c.genericRPC(&header, &req, &resp)

	return resp.Messages, err
}

// ListKeys returns all of the active keys on each member of the cluster
func (c *RPCClient) ListKeys() (map[string]int, int, map[string]string, error) {
	header := requestHeader{
		Command: listKeysCommand,
		Seq:     c.getSeq(),
	}

	resp := keyResponse{}
	err := c.genericRPC(&header, nil, &resp)

	return resp.Keys, resp.NumNodes, resp.Messages, err
}

// Stats is used to get debugging state information
func (c *RPCClient) Stats() (map[string]map[string]string, error) {
	header := requestHeader{
		Command: statsCommand,
		Seq:     c.getSeq(),
	}
	var resp map[string]map[string]string

	err := c.genericRPC(&header, nil, &resp)
	return resp, err
}

// GetCoordinate is used to retrieve the cached coordinate of a node.
func (c *RPCClient) GetCoordinate(node string) (*coordinate.Coordinate, error) {
	header := requestHeader{
		Command: getCoordinateCommand,
		Seq:     c.getSeq(),
	}
	req := coordinateRequest{
		Node: node,
	}
	var resp coordinateResponse

	if err := c.genericRPC(&header, &req, &resp); err != nil {
		return nil, err
	}
	if resp.Ok {
		return &resp.Coord, nil
	}
	return nil, nil
}

type monitorHandler struct {
	client *RPCClient
	closed bool
	init   bool
	initCh chan<- error
	logCh  chan<- string
	seq    uint64
}

func (mh *monitorHandler) Handle(resp *responseHeader) {
	// Initialize on the first response
	if !mh.init {
		mh.init = true
		mh.initCh <- strToError(resp.Error)
		return
	}

	// Decode logs for all other responses
	var rec logRecord
	if err := mh.client.dec.Decode(&rec); err != nil {
		log.Printf("[ERR] Failed to decode log: %v", err)
		mh.client.deregisterHandler(mh.seq)
		return
	}
	select {
	case mh.logCh <- rec.Log:
	default:
		log.Printf("[ERR] Dropping log! Monitor channel full")
	}
}

func (mh *monitorHandler) Cleanup() {
	if !mh.closed {
		if !mh.init {
			mh.init = true
			mh.initCh <- fmt.Errorf("Stream closed")
		}
		if mh.logCh != nil {
			close(mh.logCh)
		}
		mh.closed = true
	}
}

// Monitor is used to subscribe to the logs of the agent
func (c *RPCClient) Monitor(level logutils.LogLevel, ch chan<- string) (StreamHandle, error) {
	// Setup the request
	seq := c.getSeq()
	header := requestHeader{
		Command: monitorCommand,
		Seq:     seq,
	}
	req := monitorRequest{
		LogLevel: string(level),
	}

	// Create a monitor handler
	initCh := make(chan error, 1)
	handler := &monitorHandler{
		client: c,
		initCh: initCh,
		logCh:  ch,
		seq:    seq,
	}
	c.handleSeq(seq, handler)

	// Send the request
	if err := c.send(&header, &req); err != nil {
		c.deregisterHandler(seq)
		return 0, err
	}

	// Wait for a response
	select {
	case err := <-initCh:
		return StreamHandle(seq), err
	case <-c.shutdownCh:
		c.deregisterHandler(seq)
		return 0, clientClosed
	}
}

type streamHandler struct {
	client  *RPCClient
	closed  bool
	init    bool
	initCh  chan<- error
	eventCh chan<- map[string]interface{}
	seq     uint64
}

func (sh *streamHandler) Handle(resp *responseHeader) {
	// Initialize on the first response
	if !sh.init {
		sh.init = true
		sh.initCh <- strToError(resp.Error)
		return
	}

	// Decode logs for all other responses
	var rec map[string]interface{}
	if err := sh.client.dec.Decode(&rec); err != nil {
		log.Printf("[ERR] Failed to decode stream record: %v", err)
		sh.client.deregisterHandler(sh.seq)
		return
	}
	select {
	case sh.eventCh <- rec:
	default:
		log.Printf("[ERR] Dropping event! Stream channel full")
	}
}

func (sh *streamHandler) Cleanup() {
	if !sh.closed {
		if !sh.init {
			sh.init = true
			sh.initCh <- fmt.Errorf("Stream closed")
		}
		if sh.eventCh != nil {
			close(sh.eventCh)
		}
		sh.closed = true
	}
}

// Stream is used to subscribe to events
func (c *RPCClient) Stream(filter string, ch chan<- map[string]interface{}) (StreamHandle, error) {
	// Setup the request
	seq := c.getSeq()
	header := requestHeader{
		Command: streamCommand,
		Seq:     seq,
	}
	req := streamRequest{
		Type: filter,
	}

	// Create a monitor handler
	initCh := make(chan error, 1)
	handler := &streamHandler{
		client:  c,
		initCh:  initCh,
		eventCh: ch,
		seq:     seq,
	}
	c.handleSeq(seq, handler)

	// Send the request
	if err := c.send(&header, &req); err != nil {
		c.deregisterHandler(seq)
		return 0, err
	}

	// Wait for a response
	select {
	case err := <-initCh:
		return StreamHandle(seq), err
	case <-c.shutdownCh:
		c.deregisterHandler(seq)
		return 0, clientClosed
	}
}

type queryHandler struct {
	client *RPCClient
	closed bool
	init   bool
	initCh chan<- error
	ackCh  chan<- string
	respCh chan<- NodeResponse
	seq    uint64
}

func (qh *queryHandler) Handle(resp *responseHeader) {
	// Initialize on the first response
	if !qh.init {
		qh.init = true
		qh.initCh <- strToError(resp.Error)
		return
	}

	// Decode the query response
	var rec queryRecord
	if err := qh.client.dec.Decode(&rec); err != nil {
		log.Printf("[ERR] Failed to decode query response: %v", err)
		qh.client.deregisterHandler(qh.seq)
		return
	}

	switch rec.Type {
	case queryRecordAck:
		select {
		case qh.ackCh <- rec.From:
		default:
			log.Printf("[ERR] Dropping query ack, channel full")
		}

	case queryRecordResponse:
		select {
		case qh.respCh <- NodeResponse{rec.From, rec.Payload}:
		default:
			log.Printf("[ERR] Dropping query response, channel full")
		}

	case queryRecordDone:
		// No further records coming
		qh.client.deregisterHandler(qh.seq)

	default:
		log.Printf("[ERR] Unrecognized query record type: %s", rec.Type)
	}
}

func (qh *queryHandler) Cleanup() {
	if !qh.closed {
		if !qh.init {
			qh.init = true
			qh.initCh <- fmt.Errorf("Stream closed")
		}
		if qh.ackCh != nil {
			close(qh.ackCh)
		}
		if qh.respCh != nil {
			close(qh.respCh)
		}
		qh.closed = true
	}
}

// QueryParam is provided to query set various settings.
type QueryParam struct {
	FilterNodes []string            // A list of node names to restrict query to
	FilterTags  map[string]string   // A map of tag name to regex to filter on
	RequestAck  bool                // Should nodes ack the query receipt
	RelayFactor uint8               // Duplicate response count to be relayed back to sender for redundancy.
	Timeout     time.Duration       // Maximum query duration. Optional, will be set automatically.
	Name        string              // Opaque query name
	Payload     []byte              // Opaque query payload
	AckCh       chan<- string       // Channel to send Ack replies on
	RespCh      chan<- NodeResponse // Channel to send responses on
}

// Query initiates a new query message using the given parameters, and streams
// acks and responses over the given channels. The channels will not block on
// sends and should be buffered. At the end of the query, the channels will be
// closed.
func (c *RPCClient) Query(params *QueryParam) error {
	// Setup the request
	seq := c.getSeq()
	header := requestHeader{
		Command: queryCommand,
		Seq:     seq,
	}
	req := queryRequest{
		FilterNodes: params.FilterNodes,
		FilterTags:  params.FilterTags,
		RequestAck:  params.RequestAck,
		RelayFactor: params.RelayFactor,
		Timeout:     params.Timeout,
		Name:        params.Name,
		Payload:     params.Payload,
	}

	// Create a query handler
	initCh := make(chan error, 1)
	handler := &queryHandler{
		client: c,
		initCh: initCh,
		ackCh:  params.AckCh,
		respCh: params.RespCh,
		seq:    seq,
	}
	c.handleSeq(seq, handler)

	// Send the request
	if err := c.send(&header, &req); err != nil {
		c.deregisterHandler(seq)
		return err
	}

	// Wait for a response
	select {
	case err := <-initCh:
		return err
	case <-c.shutdownCh:
		c.deregisterHandler(seq)
		return clientClosed
	}
}

// Stop is used to unsubscribe from logs or event streams
func (c *RPCClient) Stop(handle StreamHandle) error {
	// Deregister locally first to stop delivery
	c.deregisterHandler(uint64(handle))

	header := requestHeader{
		Command: stopCommand,
		Seq:     c.getSeq(),
	}
	req := stopRequest{
		Stop: uint64(handle),
	}
	return c.genericRPC(&header, &req, nil)
}

// handshake is used to perform the initial handshake on connect
func (c *RPCClient) handshake() error {
	header := requestHeader{
		Command: handshakeCommand,
		Seq:     c.getSeq(),
	}
	req := handshakeRequest{
		Version: maxIPCVersion,
	}
	return c.genericRPC(&header, &req, nil)
}

// auth is used to perform the initial authentication on connect
func (c *RPCClient) auth(authKey string) error {
	header := requestHeader{
		Command: authCommand,
		Seq:     c.getSeq(),
	}
	req := authRequest{
		AuthKey: authKey,
	}
	return c.genericRPC(&header, &req, nil)
}

// genericRPC is used to send a request and wait for an
// errorSequenceResponse, potentially returning an error
func (c *RPCClient) genericRPC(header *requestHeader, req interface{}, resp interface{}) error {
	// Setup a response handler
	errCh := make(chan error, 1)
	handler := func(respHeader *responseHeader) {
		// If we get an auth error, we should not wait for a request body
		if respHeader.Error == authRequired {
			goto SEND_ERR
		}
		if resp != nil {
			err := c.dec.Decode(resp)
			if err != nil {
				errCh <- err
				return
			}
		}
	SEND_ERR:
		errCh <- strToError(respHeader.Error)
	}
	c.handleSeq(header.Seq, &seqCallback{handler: handler})
	defer c.deregisterHandler(header.Seq)

	// Send the request
	if err := c.send(header, req); err != nil {
		return err
	}

	// Wait for a response
	select {
	case err := <-errCh:
		return err
	case <-c.shutdownCh:
		return clientClosed
	}
}

// strToError converts a string to an error if not blank
func strToError(s string) error {
	if s != "" {
		return fmt.Errorf(s)
	}
	return nil
}

// getSeq returns the next sequence number in a safe manner
func (c *RPCClient) getSeq() uint64 {
	return atomic.AddUint64(&c.seq, 1)
}

// deregisterAll is used to deregister all handlers
func (c *RPCClient) deregisterAll() {
	c.dispatchLock.Lock()
	defer c.dispatchLock.Unlock()

	for _, seqH := range c.dispatch {
		seqH.Cleanup()
	}
	c.dispatch = make(map[uint64]seqHandler)
}

// deregisterHandler is used to deregister a handler
func (c *RPCClient) deregisterHandler(seq uint64) {
	c.dispatchLock.Lock()
	seqH, ok := c.dispatch[seq]
	delete(c.dispatch, seq)
	c.dispatchLock.Unlock()

	if ok {
		seqH.Cleanup()
	}
}

// handleSeq is used to setup a handlerto wait on a response for
// a given sequence number.
func (c *RPCClient) handleSeq(seq uint64, handler seqHandler) {
	c.dispatchLock.Lock()
	defer c.dispatchLock.Unlock()
	c.dispatch[seq] = handler
}

// respondSeq is used to respond to a given sequence number
func (c *RPCClient) respondSeq(seq uint64, respHeader *responseHeader) {
	c.dispatchLock.Lock()
	seqL, ok := c.dispatch[seq]
	c.dispatchLock.Unlock()

	// Get a registered listener, ignore if none
	if ok {
		seqL.Handle(respHeader)
	}
}

// listen is used to processes data coming over the IPC channel,
// and wrote it to the correct destination based on seq no
func (c *RPCClient) listen() {
	defer c.Close()
	var respHeader responseHeader
	for {
		if err := c.dec.Decode(&respHeader); err != nil {
			if !c.shutdown {
				log.Printf("[ERR] agent.client: Failed to decode response header: %v", err)
			}
			break
		}
		c.respondSeq(respHeader.Seq, &respHeader)
	}
}
