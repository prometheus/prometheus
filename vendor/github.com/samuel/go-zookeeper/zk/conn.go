// Package zk is a native Go client library for the ZooKeeper orchestration service.
package zk

/*
TODO:
* make sure a ping response comes back in a reasonable time

Possible watcher events:
* Event{Type: EventNotWatching, State: StateDisconnected, Path: path, Err: err}
*/

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ErrNoServer indicates that an operation cannot be completed
// because attempts to connect to all servers in the list failed.
var ErrNoServer = errors.New("zk: could not connect to a server")

// ErrInvalidPath indicates that an operation was being attempted on
// an invalid path. (e.g. empty path)
var ErrInvalidPath = errors.New("zk: invalid path")

// DefaultLogger uses the stdlib log package for logging.
var DefaultLogger Logger = defaultLogger{}

const (
	bufferSize      = 1536 * 1024
	eventChanSize   = 6
	sendChanSize    = 16
	protectedPrefix = "_c_"
)

type watchType int

const (
	watchTypeData = iota
	watchTypeExist
	watchTypeChild
)

type watchPathType struct {
	path  string
	wType watchType
}

type Dialer func(network, address string, timeout time.Duration) (net.Conn, error)

// Logger is an interface that can be implemented to provide custom log output.
type Logger interface {
	Printf(string, ...interface{})
}

type authCreds struct {
	scheme string
	auth   []byte
}

type Conn struct {
	lastZxid         int64
	sessionID        int64
	state            State // must be 32-bit aligned
	xid              uint32
	sessionTimeoutMs int32 // session timeout in milliseconds
	passwd           []byte

	dialer         Dialer
	hostProvider   HostProvider
	serverMu       sync.Mutex // protects server
	server         string     // remember the address/port of the current server
	conn           net.Conn
	eventChan      chan Event
	eventCallback  EventCallback // may be nil
	shouldQuit     chan struct{}
	pingInterval   time.Duration
	recvTimeout    time.Duration
	connectTimeout time.Duration

	creds   []authCreds
	credsMu sync.Mutex // protects server

	sendChan     chan *request
	requests     map[int32]*request // Xid -> pending request
	requestsLock sync.Mutex
	watchers     map[watchPathType][]chan Event
	watchersLock sync.Mutex
	closeChan    chan struct{} // channel to tell send loop stop

	// Debug (used by unit tests)
	reconnectDelay time.Duration

	logger Logger

	buf []byte
}

// connOption represents a connection option.
type connOption func(c *Conn)

type request struct {
	xid        int32
	opcode     int32
	pkt        interface{}
	recvStruct interface{}
	recvChan   chan response

	// Because sending and receiving happen in separate go routines, there's
	// a possible race condition when creating watches from outside the read
	// loop. We must ensure that a watcher gets added to the list synchronously
	// with the response from the server on any request that creates a watch.
	// In order to not hard code the watch logic for each opcode in the recv
	// loop the caller can use recvFunc to insert some synchronously code
	// after a response.
	recvFunc func(*request, *responseHeader, error)
}

type response struct {
	zxid int64
	err  error
}

type Event struct {
	Type   EventType
	State  State
	Path   string // For non-session events, the path of the watched node.
	Err    error
	Server string // For connection events
}

// HostProvider is used to represent a set of hosts a ZooKeeper client should connect to.
// It is an analog of the Java equivalent:
// http://svn.apache.org/viewvc/zookeeper/trunk/src/java/main/org/apache/zookeeper/client/HostProvider.java?view=markup
type HostProvider interface {
	// Init is called first, with the servers specified in the connection string.
	Init(servers []string) error
	// Len returns the number of servers.
	Len() int
	// Next returns the next server to connect to. retryStart will be true if we've looped through
	// all known servers without Connected() being called.
	Next() (server string, retryStart bool)
	// Notify the HostProvider of a successful connection.
	Connected()
}

// ConnectWithDialer establishes a new connection to a pool of zookeeper servers
// using a custom Dialer. See Connect for further information about session timeout.
// This method is deprecated and provided for compatibility: use the WithDialer option instead.
func ConnectWithDialer(servers []string, sessionTimeout time.Duration, dialer Dialer) (*Conn, <-chan Event, error) {
	return Connect(servers, sessionTimeout, WithDialer(dialer))
}

// Connect establishes a new connection to a pool of zookeeper
// servers. The provided session timeout sets the amount of time for which
// a session is considered valid after losing connection to a server. Within
// the session timeout it's possible to reestablish a connection to a different
// server and keep the same session. This is means any ephemeral nodes and
// watches are maintained.
func Connect(servers []string, sessionTimeout time.Duration, options ...connOption) (*Conn, <-chan Event, error) {
	if len(servers) == 0 {
		return nil, nil, errors.New("zk: server list must not be empty")
	}

	srvs := make([]string, len(servers))

	for i, addr := range servers {
		if strings.Contains(addr, ":") {
			srvs[i] = addr
		} else {
			srvs[i] = addr + ":" + strconv.Itoa(DefaultPort)
		}
	}

	// Randomize the order of the servers to avoid creating hotspots
	stringShuffle(srvs)

	ec := make(chan Event, eventChanSize)
	conn := &Conn{
		dialer:         net.DialTimeout,
		hostProvider:   &DNSHostProvider{},
		conn:           nil,
		state:          StateDisconnected,
		eventChan:      ec,
		shouldQuit:     make(chan struct{}),
		connectTimeout: 1 * time.Second,
		sendChan:       make(chan *request, sendChanSize),
		requests:       make(map[int32]*request),
		watchers:       make(map[watchPathType][]chan Event),
		passwd:         emptyPassword,
		logger:         DefaultLogger,
		buf:            make([]byte, bufferSize),

		// Debug
		reconnectDelay: 0,
	}

	// Set provided options.
	for _, option := range options {
		option(conn)
	}

	if err := conn.hostProvider.Init(srvs); err != nil {
		return nil, nil, err
	}

	conn.setTimeouts(int32(sessionTimeout / time.Millisecond))

	go func() {
		conn.loop()
		conn.flushRequests(ErrClosing)
		conn.invalidateWatches(ErrClosing)
		close(conn.eventChan)
	}()
	return conn, ec, nil
}

// WithDialer returns a connection option specifying a non-default Dialer.
func WithDialer(dialer Dialer) connOption {
	return func(c *Conn) {
		c.dialer = dialer
	}
}

// WithHostProvider returns a connection option specifying a non-default HostProvider.
func WithHostProvider(hostProvider HostProvider) connOption {
	return func(c *Conn) {
		c.hostProvider = hostProvider
	}
}

// EventCallback is a function that is called when an Event occurs.
type EventCallback func(Event)

// WithEventCallback returns a connection option that specifies an event
// callback.
// The callback must not block - doing so would delay the ZK go routines.
func WithEventCallback(cb EventCallback) connOption {
	return func(c *Conn) {
		c.eventCallback = cb
	}
}

func (c *Conn) Close() {
	close(c.shouldQuit)

	select {
	case <-c.queueRequest(opClose, &closeRequest{}, &closeResponse{}, nil):
	case <-time.After(time.Second):
	}
}

// State returns the current state of the connection.
func (c *Conn) State() State {
	return State(atomic.LoadInt32((*int32)(&c.state)))
}

// SessionID returns the current session id of the connection.
func (c *Conn) SessionID() int64 {
	return atomic.LoadInt64(&c.sessionID)
}

// SetLogger sets the logger to be used for printing errors.
// Logger is an interface provided by this package.
func (c *Conn) SetLogger(l Logger) {
	c.logger = l
}

func (c *Conn) setTimeouts(sessionTimeoutMs int32) {
	c.sessionTimeoutMs = sessionTimeoutMs
	sessionTimeout := time.Duration(sessionTimeoutMs) * time.Millisecond
	c.recvTimeout = sessionTimeout * 2 / 3
	c.pingInterval = c.recvTimeout / 2
}

func (c *Conn) setState(state State) {
	atomic.StoreInt32((*int32)(&c.state), int32(state))
	c.sendEvent(Event{Type: EventSession, State: state, Server: c.Server()})
}

func (c *Conn) sendEvent(evt Event) {
	if c.eventCallback != nil {
		c.eventCallback(evt)
	}

	select {
	case c.eventChan <- evt:
	default:
		// panic("zk: event channel full - it must be monitored and never allowed to be full")
	}
}

func (c *Conn) connect() error {
	var retryStart bool
	for {
		c.serverMu.Lock()
		c.server, retryStart = c.hostProvider.Next()
		c.serverMu.Unlock()
		c.setState(StateConnecting)
		if retryStart {
			c.flushUnsentRequests(ErrNoServer)
			select {
			case <-time.After(time.Second):
				// pass
			case <-c.shouldQuit:
				c.setState(StateDisconnected)
				c.flushUnsentRequests(ErrClosing)
				return ErrClosing
			}
		}

		zkConn, err := c.dialer("tcp", c.Server(), c.connectTimeout)
		if err == nil {
			c.conn = zkConn
			c.setState(StateConnected)
			c.logger.Printf("Connected to %s", c.Server())
			return nil
		}

		c.logger.Printf("Failed to connect to %s: %+v", c.Server(), err)
	}
}

func (c *Conn) resendZkAuth(reauthReadyChan chan struct{}) {
	c.credsMu.Lock()
	defer c.credsMu.Unlock()

	defer close(reauthReadyChan)

	c.logger.Printf("Re-submitting `%d` credentials after reconnect",
		len(c.creds))

	for _, cred := range c.creds {
		resChan, err := c.sendRequest(
			opSetAuth,
			&setAuthRequest{Type: 0,
				Scheme: cred.scheme,
				Auth:   cred.auth,
			},
			&setAuthResponse{},
			nil)

		if err != nil {
			c.logger.Printf("Call to sendRequest failed during credential resubmit: %s", err)
			// FIXME(prozlach): lets ignore errors for now
			continue
		}

		res := <-resChan
		if res.err != nil {
			c.logger.Printf("Credential re-submit failed: %s", res.err)
			// FIXME(prozlach): lets ignore errors for now
			continue
		}
	}
}

func (c *Conn) sendRequest(
	opcode int32,
	req interface{},
	res interface{},
	recvFunc func(*request, *responseHeader, error),
) (
	<-chan response,
	error,
) {
	rq := &request{
		xid:        c.nextXid(),
		opcode:     opcode,
		pkt:        req,
		recvStruct: res,
		recvChan:   make(chan response, 1),
		recvFunc:   recvFunc,
	}

	if err := c.sendData(rq); err != nil {
		return nil, err
	}

	return rq.recvChan, nil
}

func (c *Conn) loop() {
	for {
		if err := c.connect(); err != nil {
			// c.Close() was called
			return
		}

		err := c.authenticate()
		switch {
		case err == ErrSessionExpired:
			c.logger.Printf("Authentication failed: %s", err)
			c.invalidateWatches(err)
		case err != nil && c.conn != nil:
			c.logger.Printf("Authentication failed: %s", err)
			c.conn.Close()
		case err == nil:
			c.logger.Printf("Authenticated: id=%d, timeout=%d", c.SessionID(), c.sessionTimeoutMs)
			c.hostProvider.Connected()        // mark success
			c.closeChan = make(chan struct{}) // channel to tell send loop stop
			reauthChan := make(chan struct{}) // channel to tell send loop that authdata has been resubmitted

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				<-reauthChan
				err := c.sendLoop()
				c.logger.Printf("Send loop terminated: err=%v", err)
				c.conn.Close() // causes recv loop to EOF/exit
				wg.Done()
			}()

			wg.Add(1)
			go func() {
				err := c.recvLoop(c.conn)
				c.logger.Printf("Recv loop terminated: err=%v", err)
				if err == nil {
					panic("zk: recvLoop should never return nil error")
				}
				close(c.closeChan) // tell send loop to exit
				wg.Done()
			}()

			c.resendZkAuth(reauthChan)

			c.sendSetWatches()
			wg.Wait()
		}

		c.setState(StateDisconnected)

		select {
		case <-c.shouldQuit:
			c.flushRequests(ErrClosing)
			return
		default:
		}

		if err != ErrSessionExpired {
			err = ErrConnectionClosed
		}
		c.flushRequests(err)

		if c.reconnectDelay > 0 {
			select {
			case <-c.shouldQuit:
				return
			case <-time.After(c.reconnectDelay):
			}
		}
	}
}

func (c *Conn) flushUnsentRequests(err error) {
	for {
		select {
		default:
			return
		case req := <-c.sendChan:
			req.recvChan <- response{-1, err}
		}
	}
}

// Send error to all pending requests and clear request map
func (c *Conn) flushRequests(err error) {
	c.requestsLock.Lock()
	for _, req := range c.requests {
		req.recvChan <- response{-1, err}
	}
	c.requests = make(map[int32]*request)
	c.requestsLock.Unlock()
}

// Send error to all watchers and clear watchers map
func (c *Conn) invalidateWatches(err error) {
	c.watchersLock.Lock()
	defer c.watchersLock.Unlock()

	if len(c.watchers) >= 0 {
		for pathType, watchers := range c.watchers {
			ev := Event{Type: EventNotWatching, State: StateDisconnected, Path: pathType.path, Err: err}
			for _, ch := range watchers {
				ch <- ev
				close(ch)
			}
		}
		c.watchers = make(map[watchPathType][]chan Event)
	}
}

func (c *Conn) sendSetWatches() {
	c.watchersLock.Lock()
	defer c.watchersLock.Unlock()

	if len(c.watchers) == 0 {
		return
	}

	req := &setWatchesRequest{
		RelativeZxid: c.lastZxid,
		DataWatches:  make([]string, 0),
		ExistWatches: make([]string, 0),
		ChildWatches: make([]string, 0),
	}
	n := 0
	for pathType, watchers := range c.watchers {
		if len(watchers) == 0 {
			continue
		}
		switch pathType.wType {
		case watchTypeData:
			req.DataWatches = append(req.DataWatches, pathType.path)
		case watchTypeExist:
			req.ExistWatches = append(req.ExistWatches, pathType.path)
		case watchTypeChild:
			req.ChildWatches = append(req.ChildWatches, pathType.path)
		}
		n++
	}
	if n == 0 {
		return
	}

	go func() {
		res := &setWatchesResponse{}
		_, err := c.request(opSetWatches, req, res, nil)
		if err != nil {
			c.logger.Printf("Failed to set previous watches: %s", err.Error())
		}
	}()
}

func (c *Conn) authenticate() error {
	buf := make([]byte, 256)

	// Encode and send a connect request.
	n, err := encodePacket(buf[4:], &connectRequest{
		ProtocolVersion: protocolVersion,
		LastZxidSeen:    c.lastZxid,
		TimeOut:         c.sessionTimeoutMs,
		SessionID:       c.SessionID(),
		Passwd:          c.passwd,
	})
	if err != nil {
		return err
	}

	binary.BigEndian.PutUint32(buf[:4], uint32(n))

	c.conn.SetWriteDeadline(time.Now().Add(c.recvTimeout * 10))
	_, err = c.conn.Write(buf[:n+4])
	c.conn.SetWriteDeadline(time.Time{})
	if err != nil {
		return err
	}

	// Receive and decode a connect response.
	c.conn.SetReadDeadline(time.Now().Add(c.recvTimeout * 10))
	_, err = io.ReadFull(c.conn, buf[:4])
	c.conn.SetReadDeadline(time.Time{})
	if err != nil {
		return err
	}

	blen := int(binary.BigEndian.Uint32(buf[:4]))
	if cap(buf) < blen {
		buf = make([]byte, blen)
	}

	_, err = io.ReadFull(c.conn, buf[:blen])
	if err != nil {
		return err
	}

	r := connectResponse{}
	_, err = decodePacket(buf[:blen], &r)
	if err != nil {
		return err
	}
	if r.SessionID == 0 {
		atomic.StoreInt64(&c.sessionID, int64(0))
		c.passwd = emptyPassword
		c.lastZxid = 0
		c.setState(StateExpired)
		return ErrSessionExpired
	}

	atomic.StoreInt64(&c.sessionID, r.SessionID)
	c.setTimeouts(r.TimeOut)
	c.passwd = r.Passwd
	c.setState(StateHasSession)

	return nil
}

func (c *Conn) sendData(req *request) error {
	header := &requestHeader{req.xid, req.opcode}
	n, err := encodePacket(c.buf[4:], header)
	if err != nil {
		req.recvChan <- response{-1, err}
		return nil
	}

	n2, err := encodePacket(c.buf[4+n:], req.pkt)
	if err != nil {
		req.recvChan <- response{-1, err}
		return nil
	}

	n += n2

	binary.BigEndian.PutUint32(c.buf[:4], uint32(n))

	c.requestsLock.Lock()
	select {
	case <-c.closeChan:
		req.recvChan <- response{-1, ErrConnectionClosed}
		c.requestsLock.Unlock()
		return ErrConnectionClosed
	default:
	}
	c.requests[req.xid] = req
	c.requestsLock.Unlock()

	c.conn.SetWriteDeadline(time.Now().Add(c.recvTimeout))
	_, err = c.conn.Write(c.buf[:n+4])
	c.conn.SetWriteDeadline(time.Time{})
	if err != nil {
		req.recvChan <- response{-1, err}
		c.conn.Close()
		return err
	}

	return nil
}

func (c *Conn) sendLoop() error {
	pingTicker := time.NewTicker(c.pingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case req := <-c.sendChan:
			if err := c.sendData(req); err != nil {
				return err
			}
		case <-pingTicker.C:
			n, err := encodePacket(c.buf[4:], &requestHeader{Xid: -2, Opcode: opPing})
			if err != nil {
				panic("zk: opPing should never fail to serialize")
			}

			binary.BigEndian.PutUint32(c.buf[:4], uint32(n))

			c.conn.SetWriteDeadline(time.Now().Add(c.recvTimeout))
			_, err = c.conn.Write(c.buf[:n+4])
			c.conn.SetWriteDeadline(time.Time{})
			if err != nil {
				c.conn.Close()
				return err
			}
		case <-c.closeChan:
			return nil
		}
	}
}

func (c *Conn) recvLoop(conn net.Conn) error {
	buf := make([]byte, bufferSize)
	for {
		// package length
		conn.SetReadDeadline(time.Now().Add(c.recvTimeout))
		_, err := io.ReadFull(conn, buf[:4])
		if err != nil {
			return err
		}

		blen := int(binary.BigEndian.Uint32(buf[:4]))
		if cap(buf) < blen {
			buf = make([]byte, blen)
		}

		_, err = io.ReadFull(conn, buf[:blen])
		conn.SetReadDeadline(time.Time{})
		if err != nil {
			return err
		}

		res := responseHeader{}
		_, err = decodePacket(buf[:16], &res)
		if err != nil {
			return err
		}

		if res.Xid == -1 {
			res := &watcherEvent{}
			_, err := decodePacket(buf[16:blen], res)
			if err != nil {
				return err
			}
			ev := Event{
				Type:  res.Type,
				State: res.State,
				Path:  res.Path,
				Err:   nil,
			}
			c.sendEvent(ev)
			wTypes := make([]watchType, 0, 2)
			switch res.Type {
			case EventNodeCreated:
				wTypes = append(wTypes, watchTypeExist)
			case EventNodeDeleted, EventNodeDataChanged:
				wTypes = append(wTypes, watchTypeExist, watchTypeData, watchTypeChild)
			case EventNodeChildrenChanged:
				wTypes = append(wTypes, watchTypeChild)
			}
			c.watchersLock.Lock()
			for _, t := range wTypes {
				wpt := watchPathType{res.Path, t}
				if watchers := c.watchers[wpt]; watchers != nil && len(watchers) > 0 {
					for _, ch := range watchers {
						ch <- ev
						close(ch)
					}
					delete(c.watchers, wpt)
				}
			}
			c.watchersLock.Unlock()
		} else if res.Xid == -2 {
			// Ping response. Ignore.
		} else if res.Xid < 0 {
			c.logger.Printf("Xid < 0 (%d) but not ping or watcher event", res.Xid)
		} else {
			if res.Zxid > 0 {
				c.lastZxid = res.Zxid
			}

			c.requestsLock.Lock()
			req, ok := c.requests[res.Xid]
			if ok {
				delete(c.requests, res.Xid)
			}
			c.requestsLock.Unlock()

			if !ok {
				c.logger.Printf("Response for unknown request with xid %d", res.Xid)
			} else {
				if res.Err != 0 {
					err = res.Err.toError()
				} else {
					_, err = decodePacket(buf[16:blen], req.recvStruct)
				}
				if req.recvFunc != nil {
					req.recvFunc(req, &res, err)
				}
				req.recvChan <- response{res.Zxid, err}
				if req.opcode == opClose {
					return io.EOF
				}
			}
		}
	}
}

func (c *Conn) nextXid() int32 {
	return int32(atomic.AddUint32(&c.xid, 1) & 0x7fffffff)
}

func (c *Conn) addWatcher(path string, watchType watchType) <-chan Event {
	c.watchersLock.Lock()
	defer c.watchersLock.Unlock()

	ch := make(chan Event, 1)
	wpt := watchPathType{path, watchType}
	c.watchers[wpt] = append(c.watchers[wpt], ch)
	return ch
}

func (c *Conn) queueRequest(opcode int32, req interface{}, res interface{}, recvFunc func(*request, *responseHeader, error)) <-chan response {
	rq := &request{
		xid:        c.nextXid(),
		opcode:     opcode,
		pkt:        req,
		recvStruct: res,
		recvChan:   make(chan response, 1),
		recvFunc:   recvFunc,
	}
	c.sendChan <- rq
	return rq.recvChan
}

func (c *Conn) request(opcode int32, req interface{}, res interface{}, recvFunc func(*request, *responseHeader, error)) (int64, error) {
	r := <-c.queueRequest(opcode, req, res, recvFunc)
	return r.zxid, r.err
}

func (c *Conn) AddAuth(scheme string, auth []byte) error {
	_, err := c.request(opSetAuth, &setAuthRequest{Type: 0, Scheme: scheme, Auth: auth}, &setAuthResponse{}, nil)

	if err != nil {
		return err
	}

	// Remember authdata so that it can be re-submitted on reconnect
	//
	// FIXME(prozlach): For now we treat "userfoo:passbar" and "userfoo:passbar2"
	// as two different entries, which will be re-submitted on reconnet. Some
	// research is needed on how ZK treats these cases and
	// then maybe switch to something like "map[username] = password" to allow
	// only single password for given user with users being unique.
	obj := authCreds{
		scheme: scheme,
		auth:   auth,
	}

	c.credsMu.Lock()
	c.creds = append(c.creds, obj)
	c.credsMu.Unlock()

	return nil
}

func (c *Conn) Children(path string) ([]string, *Stat, error) {
	res := &getChildren2Response{}
	_, err := c.request(opGetChildren2, &getChildren2Request{Path: path, Watch: false}, res, nil)
	return res.Children, &res.Stat, err
}

func (c *Conn) ChildrenW(path string) ([]string, *Stat, <-chan Event, error) {
	var ech <-chan Event
	res := &getChildren2Response{}
	_, err := c.request(opGetChildren2, &getChildren2Request{Path: path, Watch: true}, res, func(req *request, res *responseHeader, err error) {
		if err == nil {
			ech = c.addWatcher(path, watchTypeChild)
		}
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return res.Children, &res.Stat, ech, err
}

func (c *Conn) Get(path string) ([]byte, *Stat, error) {
	res := &getDataResponse{}
	_, err := c.request(opGetData, &getDataRequest{Path: path, Watch: false}, res, nil)
	return res.Data, &res.Stat, err
}

// GetW returns the contents of a znode and sets a watch
func (c *Conn) GetW(path string) ([]byte, *Stat, <-chan Event, error) {
	var ech <-chan Event
	res := &getDataResponse{}
	_, err := c.request(opGetData, &getDataRequest{Path: path, Watch: true}, res, func(req *request, res *responseHeader, err error) {
		if err == nil {
			ech = c.addWatcher(path, watchTypeData)
		}
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return res.Data, &res.Stat, ech, err
}

func (c *Conn) Set(path string, data []byte, version int32) (*Stat, error) {
	if path == "" {
		return nil, ErrInvalidPath
	}
	res := &setDataResponse{}
	_, err := c.request(opSetData, &SetDataRequest{path, data, version}, res, nil)
	return &res.Stat, err
}

func (c *Conn) Create(path string, data []byte, flags int32, acl []ACL) (string, error) {
	res := &createResponse{}
	_, err := c.request(opCreate, &CreateRequest{path, data, acl, flags}, res, nil)
	return res.Path, err
}

// CreateProtectedEphemeralSequential fixes a race condition if the server crashes
// after it creates the node. On reconnect the session may still be valid so the
// ephemeral node still exists. Therefore, on reconnect we need to check if a node
// with a GUID generated on create exists.
func (c *Conn) CreateProtectedEphemeralSequential(path string, data []byte, acl []ACL) (string, error) {
	var guid [16]byte
	_, err := io.ReadFull(rand.Reader, guid[:16])
	if err != nil {
		return "", err
	}
	guidStr := fmt.Sprintf("%x", guid)

	parts := strings.Split(path, "/")
	parts[len(parts)-1] = fmt.Sprintf("%s%s-%s", protectedPrefix, guidStr, parts[len(parts)-1])
	rootPath := strings.Join(parts[:len(parts)-1], "/")
	protectedPath := strings.Join(parts, "/")

	var newPath string
	for i := 0; i < 3; i++ {
		newPath, err = c.Create(protectedPath, data, FlagEphemeral|FlagSequence, acl)
		switch err {
		case ErrSessionExpired:
			// No need to search for the node since it can't exist. Just try again.
		case ErrConnectionClosed:
			children, _, err := c.Children(rootPath)
			if err != nil {
				return "", err
			}
			for _, p := range children {
				parts := strings.Split(p, "/")
				if pth := parts[len(parts)-1]; strings.HasPrefix(pth, protectedPrefix) {
					if g := pth[len(protectedPrefix) : len(protectedPrefix)+32]; g == guidStr {
						return rootPath + "/" + p, nil
					}
				}
			}
		case nil:
			return newPath, nil
		default:
			return "", err
		}
	}
	return "", err
}

func (c *Conn) Delete(path string, version int32) error {
	_, err := c.request(opDelete, &DeleteRequest{path, version}, &deleteResponse{}, nil)
	return err
}

func (c *Conn) Exists(path string) (bool, *Stat, error) {
	res := &existsResponse{}
	_, err := c.request(opExists, &existsRequest{Path: path, Watch: false}, res, nil)
	exists := true
	if err == ErrNoNode {
		exists = false
		err = nil
	}
	return exists, &res.Stat, err
}

func (c *Conn) ExistsW(path string) (bool, *Stat, <-chan Event, error) {
	var ech <-chan Event
	res := &existsResponse{}
	_, err := c.request(opExists, &existsRequest{Path: path, Watch: true}, res, func(req *request, res *responseHeader, err error) {
		if err == nil {
			ech = c.addWatcher(path, watchTypeData)
		} else if err == ErrNoNode {
			ech = c.addWatcher(path, watchTypeExist)
		}
	})
	exists := true
	if err == ErrNoNode {
		exists = false
		err = nil
	}
	if err != nil {
		return false, nil, nil, err
	}
	return exists, &res.Stat, ech, err
}

func (c *Conn) GetACL(path string) ([]ACL, *Stat, error) {
	res := &getAclResponse{}
	_, err := c.request(opGetAcl, &getAclRequest{Path: path}, res, nil)
	return res.Acl, &res.Stat, err
}
func (c *Conn) SetACL(path string, acl []ACL, version int32) (*Stat, error) {
	res := &setAclResponse{}
	_, err := c.request(opSetAcl, &setAclRequest{Path: path, Acl: acl, Version: version}, res, nil)
	return &res.Stat, err
}

func (c *Conn) Sync(path string) (string, error) {
	res := &syncResponse{}
	_, err := c.request(opSync, &syncRequest{Path: path}, res, nil)
	return res.Path, err
}

type MultiResponse struct {
	Stat   *Stat
	String string
	Error  error
}

// Multi executes multiple ZooKeeper operations or none of them. The provided
// ops must be one of *CreateRequest, *DeleteRequest, *SetDataRequest, or
// *CheckVersionRequest.
func (c *Conn) Multi(ops ...interface{}) ([]MultiResponse, error) {
	req := &multiRequest{
		Ops:        make([]multiRequestOp, 0, len(ops)),
		DoneHeader: multiHeader{Type: -1, Done: true, Err: -1},
	}
	for _, op := range ops {
		var opCode int32
		switch op.(type) {
		case *CreateRequest:
			opCode = opCreate
		case *SetDataRequest:
			opCode = opSetData
		case *DeleteRequest:
			opCode = opDelete
		case *CheckVersionRequest:
			opCode = opCheck
		default:
			return nil, fmt.Errorf("unknown operation type %T", op)
		}
		req.Ops = append(req.Ops, multiRequestOp{multiHeader{opCode, false, -1}, op})
	}
	res := &multiResponse{}
	_, err := c.request(opMulti, req, res, nil)
	mr := make([]MultiResponse, len(res.Ops))
	for i, op := range res.Ops {
		mr[i] = MultiResponse{Stat: op.Stat, String: op.String, Error: op.Err.toError()}
	}
	return mr, err
}

// Server returns the current or last-connected server name.
func (c *Conn) Server() string {
	c.serverMu.Lock()
	defer c.serverMu.Unlock()
	return c.server
}
