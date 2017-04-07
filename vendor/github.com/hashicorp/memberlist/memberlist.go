/*
memberlist is a library that manages cluster
membership and member failure detection using a gossip based protocol.

The use cases for such a library are far-reaching: all distributed systems
require membership, and memberlist is a re-usable solution to managing
cluster membership and node failure detection.

memberlist is eventually consistent but converges quickly on average.
The speed at which it converges can be heavily tuned via various knobs
on the protocol. Node failures are detected and network partitions are partially
tolerated by attempting to communicate to potentially dead nodes through
multiple routes.
*/
package memberlist

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	sockaddr "github.com/hashicorp/go-sockaddr"
	"github.com/miekg/dns"
)

type Memberlist struct {
	sequenceNum uint32 // Local sequence number
	incarnation uint32 // Local incarnation number
	numNodes    uint32 // Number of known nodes (estimate)

	config         *Config
	shutdown       bool
	shutdownCh     chan struct{}
	leave          bool
	leaveBroadcast chan struct{}

	transport Transport
	handoff   chan msgHandoff

	nodeLock   sync.RWMutex
	nodes      []*nodeState          // Known nodes
	nodeMap    map[string]*nodeState // Maps Addr.String() -> NodeState
	nodeTimers map[string]*suspicion // Maps Addr.String() -> suspicion timer
	awareness  *awareness

	tickerLock sync.Mutex
	tickers    []*time.Ticker
	stopTick   chan struct{}
	probeIndex int

	ackLock     sync.Mutex
	ackHandlers map[uint32]*ackHandler

	broadcasts *TransmitLimitedQueue

	logger *log.Logger
}

// newMemberlist creates the network listeners.
// Does not schedule execution of background maintenance.
func newMemberlist(conf *Config) (*Memberlist, error) {
	if conf.ProtocolVersion < ProtocolVersionMin {
		return nil, fmt.Errorf("Protocol version '%d' too low. Must be in range: [%d, %d]",
			conf.ProtocolVersion, ProtocolVersionMin, ProtocolVersionMax)
	} else if conf.ProtocolVersion > ProtocolVersionMax {
		return nil, fmt.Errorf("Protocol version '%d' too high. Must be in range: [%d, %d]",
			conf.ProtocolVersion, ProtocolVersionMin, ProtocolVersionMax)
	}

	if len(conf.SecretKey) > 0 {
		if conf.Keyring == nil {
			keyring, err := NewKeyring(nil, conf.SecretKey)
			if err != nil {
				return nil, err
			}
			conf.Keyring = keyring
		} else {
			if err := conf.Keyring.AddKey(conf.SecretKey); err != nil {
				return nil, err
			}
			if err := conf.Keyring.UseKey(conf.SecretKey); err != nil {
				return nil, err
			}
		}
	}

	if conf.LogOutput != nil && conf.Logger != nil {
		return nil, fmt.Errorf("Cannot specify both LogOutput and Logger. Please choose a single log configuration setting.")
	}

	logDest := conf.LogOutput
	if logDest == nil {
		logDest = os.Stderr
	}

	logger := conf.Logger
	if logger == nil {
		logger = log.New(logDest, "", log.LstdFlags)
	}

	// Set up a network transport by default if a custom one wasn't given
	// by the config.
	transport := conf.Transport
	if transport == nil {
		nc := &NetTransportConfig{
			BindAddrs: []string{conf.BindAddr},
			BindPort:  conf.BindPort,
			Logger:    logger,
		}
		nt, err := NewNetTransport(nc)
		if err != nil {
			return nil, fmt.Errorf("Could not set up network transport: %v", err)
		}

		if conf.BindPort == 0 {
			port := nt.GetAutoBindPort()
			conf.BindPort = port
			logger.Printf("[DEBUG] Using dynamic bind port %d", port)
		}
		transport = nt
	}

	m := &Memberlist{
		config:         conf,
		shutdownCh:     make(chan struct{}),
		leaveBroadcast: make(chan struct{}, 1),
		transport:      transport,
		handoff:        make(chan msgHandoff, conf.HandoffQueueDepth),
		nodeMap:        make(map[string]*nodeState),
		nodeTimers:     make(map[string]*suspicion),
		awareness:      newAwareness(conf.AwarenessMaxMultiplier),
		ackHandlers:    make(map[uint32]*ackHandler),
		broadcasts:     &TransmitLimitedQueue{RetransmitMult: conf.RetransmitMult},
		logger:         logger,
	}
	m.broadcasts.NumNodes = func() int {
		return m.estNumNodes()
	}
	go m.streamListen()
	go m.packetListen()
	go m.packetHandler()
	return m, nil
}

// Create will create a new Memberlist using the given configuration.
// This will not connect to any other node (see Join) yet, but will start
// all the listeners to allow other nodes to join this memberlist.
// After creating a Memberlist, the configuration given should not be
// modified by the user anymore.
func Create(conf *Config) (*Memberlist, error) {
	m, err := newMemberlist(conf)
	if err != nil {
		return nil, err
	}
	if err := m.setAlive(); err != nil {
		m.Shutdown()
		return nil, err
	}
	m.schedule()
	return m, nil
}

// Join is used to take an existing Memberlist and attempt to join a cluster
// by contacting all the given hosts and performing a state sync. Initially,
// the Memberlist only contains our own state, so doing this will cause
// remote nodes to become aware of the existence of this node, effectively
// joining the cluster.
//
// This returns the number of hosts successfully contacted and an error if
// none could be reached. If an error is returned, the node did not successfully
// join the cluster.
func (m *Memberlist) Join(existing []string) (int, error) {
	numSuccess := 0
	var errs error
	for _, exist := range existing {
		addrs, err := m.resolveAddr(exist)
		if err != nil {
			err = fmt.Errorf("Failed to resolve %s: %v", exist, err)
			errs = multierror.Append(errs, err)
			m.logger.Printf("[WARN] memberlist: %v", err)
			continue
		}

		for _, addr := range addrs {
			hp := joinHostPort(addr.ip.String(), addr.port)
			if err := m.pushPullNode(hp, true); err != nil {
				err = fmt.Errorf("Failed to join %s: %v", addr.ip, err)
				errs = multierror.Append(errs, err)
				m.logger.Printf("[DEBUG] memberlist: %v", err)
				continue
			}
			numSuccess++
		}

	}
	if numSuccess > 0 {
		errs = nil
	}
	return numSuccess, errs
}

// ipPort holds information about a node we want to try to join.
type ipPort struct {
	ip   net.IP
	port uint16
}

// tcpLookupIP is a helper to initiate a TCP-based DNS lookup for the given host.
// The built-in Go resolver will do a UDP lookup first, and will only use TCP if
// the response has the truncate bit set, which isn't common on DNS servers like
// Consul's. By doing the TCP lookup directly, we get the best chance for the
// largest list of hosts to join. Since joins are relatively rare events, it's ok
// to do this rather expensive operation.
func (m *Memberlist) tcpLookupIP(host string, defaultPort uint16) ([]ipPort, error) {
	// Don't attempt any TCP lookups against non-fully qualified domain
	// names, since those will likely come from the resolv.conf file.
	if !strings.Contains(host, ".") {
		return nil, nil
	}

	// Make sure the domain name is terminated with a dot (we know there's
	// at least one character at this point).
	dn := host
	if dn[len(dn)-1] != '.' {
		dn = dn + "."
	}

	// See if we can find a server to try.
	cc, err := dns.ClientConfigFromFile(m.config.DNSConfigPath)
	if err != nil {
		return nil, err
	}
	if len(cc.Servers) > 0 {
		// We support host:port in the DNS config, but need to add the
		// default port if one is not supplied.
		server := cc.Servers[0]
		if !hasPort(server) {
			server = net.JoinHostPort(server, cc.Port)
		}

		// Do the lookup.
		c := new(dns.Client)
		c.Net = "tcp"
		msg := new(dns.Msg)
		msg.SetQuestion(dn, dns.TypeANY)
		in, _, err := c.Exchange(msg, server)
		if err != nil {
			return nil, err
		}

		// Handle any IPs we get back that we can attempt to join.
		var ips []ipPort
		for _, r := range in.Answer {
			switch rr := r.(type) {
			case (*dns.A):
				ips = append(ips, ipPort{rr.A, defaultPort})
			case (*dns.AAAA):
				ips = append(ips, ipPort{rr.AAAA, defaultPort})
			case (*dns.CNAME):
				m.logger.Printf("[DEBUG] memberlist: Ignoring CNAME RR in TCP-first answer for '%s'", host)
			}
		}
		return ips, nil
	}

	return nil, nil
}

// resolveAddr is used to resolve the address into an address,
// port, and error. If no port is given, use the default
func (m *Memberlist) resolveAddr(hostStr string) ([]ipPort, error) {
	// Normalize the incoming string to host:port so we can apply Go's
	// parser to it.
	port := uint16(0)
	if !hasPort(hostStr) {
		hostStr += ":" + strconv.Itoa(m.config.BindPort)
	}
	host, sport, err := net.SplitHostPort(hostStr)
	if err != nil {
		return nil, err
	}

	// This will capture the supplied port, or the default one added above.
	lport, err := strconv.ParseUint(sport, 10, 16)
	if err != nil {
		return nil, err
	}
	port = uint16(lport)

	// If it looks like an IP address we are done. The SplitHostPort() above
	// will make sure the host part is in good shape for parsing, even for
	// IPv6 addresses.
	if ip := net.ParseIP(host); ip != nil {
		return []ipPort{ipPort{ip, port}}, nil
	}

	// First try TCP so we have the best chance for the largest list of
	// hosts to join. If this fails it's not fatal since this isn't a standard
	// way to query DNS, and we have a fallback below.
	ips, err := m.tcpLookupIP(host, port)
	if err != nil {
		m.logger.Printf("[DEBUG] memberlist: TCP-first lookup failed for '%s', falling back to UDP: %s", hostStr, err)
	}
	if len(ips) > 0 {
		return ips, nil
	}

	// If TCP didn't yield anything then use the normal Go resolver which
	// will try UDP, then might possibly try TCP again if the UDP response
	// indicates it was truncated.
	ans, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	ips = make([]ipPort, 0, len(ans))
	for _, ip := range ans {
		ips = append(ips, ipPort{ip, port})
	}
	return ips, nil
}

// setAlive is used to mark this node as being alive. This is the same
// as if we received an alive notification our own network channel for
// ourself.
func (m *Memberlist) setAlive() error {
	// Get the final advertise address from the transport, which may need
	// to see which address we bound to.
	addr, port, err := m.transport.FinalAdvertiseAddr(
		m.config.AdvertiseAddr, m.config.AdvertisePort)
	if err != nil {
		return fmt.Errorf("Failed to get final advertise address: %v")
	}

	// Check if this is a public address without encryption
	ipAddr, err := sockaddr.NewIPAddr(addr.String())
	if err != nil {
		return fmt.Errorf("Failed to parse interface addresses: %v", err)
	}
	ifAddrs := []sockaddr.IfAddr{
		sockaddr.IfAddr{
			SockAddr: ipAddr,
		},
	}
	_, publicIfs, err := sockaddr.IfByRFC("6890", ifAddrs)
	if len(publicIfs) > 0 && !m.config.EncryptionEnabled() {
		m.logger.Printf("[WARN] memberlist: Binding to public address without encryption!")
	}

	// Set any metadata from the delegate.
	var meta []byte
	if m.config.Delegate != nil {
		meta = m.config.Delegate.NodeMeta(MetaMaxSize)
		if len(meta) > MetaMaxSize {
			panic("Node meta data provided is longer than the limit")
		}
	}

	a := alive{
		Incarnation: m.nextIncarnation(),
		Node:        m.config.Name,
		Addr:        addr,
		Port:        uint16(port),
		Meta:        meta,
		Vsn: []uint8{
			ProtocolVersionMin, ProtocolVersionMax, m.config.ProtocolVersion,
			m.config.DelegateProtocolMin, m.config.DelegateProtocolMax,
			m.config.DelegateProtocolVersion,
		},
	}
	m.aliveNode(&a, nil, true)
	return nil
}

// LocalNode is used to return the local Node
func (m *Memberlist) LocalNode() *Node {
	m.nodeLock.RLock()
	defer m.nodeLock.RUnlock()
	state := m.nodeMap[m.config.Name]
	return &state.Node
}

// UpdateNode is used to trigger re-advertising the local node. This is
// primarily used with a Delegate to support dynamic updates to the local
// meta data.  This will block until the update message is successfully
// broadcasted to a member of the cluster, if any exist or until a specified
// timeout is reached.
func (m *Memberlist) UpdateNode(timeout time.Duration) error {
	// Get the node meta data
	var meta []byte
	if m.config.Delegate != nil {
		meta = m.config.Delegate.NodeMeta(MetaMaxSize)
		if len(meta) > MetaMaxSize {
			panic("Node meta data provided is longer than the limit")
		}
	}

	// Get the existing node
	m.nodeLock.RLock()
	state := m.nodeMap[m.config.Name]
	m.nodeLock.RUnlock()

	// Format a new alive message
	a := alive{
		Incarnation: m.nextIncarnation(),
		Node:        m.config.Name,
		Addr:        state.Addr,
		Port:        state.Port,
		Meta:        meta,
		Vsn: []uint8{
			ProtocolVersionMin, ProtocolVersionMax, m.config.ProtocolVersion,
			m.config.DelegateProtocolMin, m.config.DelegateProtocolMax,
			m.config.DelegateProtocolVersion,
		},
	}
	notifyCh := make(chan struct{})
	m.aliveNode(&a, notifyCh, true)

	// Wait for the broadcast or a timeout
	if m.anyAlive() {
		var timeoutCh <-chan time.Time
		if timeout > 0 {
			timeoutCh = time.After(timeout)
		}
		select {
		case <-notifyCh:
		case <-timeoutCh:
			return fmt.Errorf("timeout waiting for update broadcast")
		}
	}
	return nil
}

// SendTo is deprecated in favor of SendBestEffort, which requires a node to
// target.
func (m *Memberlist) SendTo(to net.Addr, msg []byte) error {
	// Encode as a user message
	buf := make([]byte, 1, len(msg)+1)
	buf[0] = byte(userMsg)
	buf = append(buf, msg...)

	// Send the message
	return m.rawSendMsgPacket(to.String(), nil, buf)
}

// SendToUDP is deprecated in favor of SendBestEffort.
func (m *Memberlist) SendToUDP(to *Node, msg []byte) error {
	return m.SendBestEffort(to, msg)
}

// SendToTCP is deprecated in favor of SendReliable.
func (m *Memberlist) SendToTCP(to *Node, msg []byte) error {
	return m.SendReliable(to, msg)
}

// SendBestEffort uses the unreliable packet-oriented interface of the transport
// to target a user message at the given node (this does not use the gossip
// mechanism). The maximum size of the message depends on the configured
// UDPBufferSize for this memberlist instance.
func (m *Memberlist) SendBestEffort(to *Node, msg []byte) error {
	// Encode as a user message
	buf := make([]byte, 1, len(msg)+1)
	buf[0] = byte(userMsg)
	buf = append(buf, msg...)

	// Send the message
	return m.rawSendMsgPacket(to.Address(), to, buf)
}

// SendReliable uses the reliable stream-oriented interface of the transport to
// target a user message at the given node (this does not use the gossip
// mechanism). Delivery is guaranteed if no error is returned, and there is no
// limit on the size of the message.
func (m *Memberlist) SendReliable(to *Node, msg []byte) error {
	return m.sendUserMsg(to.Address(), msg)
}

// Members returns a list of all known live nodes. The node structures
// returned must not be modified. If you wish to modify a Node, make a
// copy first.
func (m *Memberlist) Members() []*Node {
	m.nodeLock.RLock()
	defer m.nodeLock.RUnlock()

	nodes := make([]*Node, 0, len(m.nodes))
	for _, n := range m.nodes {
		if n.State != stateDead {
			nodes = append(nodes, &n.Node)
		}
	}

	return nodes
}

// NumMembers returns the number of alive nodes currently known. Between
// the time of calling this and calling Members, the number of alive nodes
// may have changed, so this shouldn't be used to determine how many
// members will be returned by Members.
func (m *Memberlist) NumMembers() (alive int) {
	m.nodeLock.RLock()
	defer m.nodeLock.RUnlock()

	for _, n := range m.nodes {
		if n.State != stateDead {
			alive++
		}
	}

	return
}

// Leave will broadcast a leave message but will not shutdown the background
// listeners, meaning the node will continue participating in gossip and state
// updates.
//
// This will block until the leave message is successfully broadcasted to
// a member of the cluster, if any exist or until a specified timeout
// is reached.
//
// This method is safe to call multiple times, but must not be called
// after the cluster is already shut down.
func (m *Memberlist) Leave(timeout time.Duration) error {
	m.nodeLock.Lock()
	// We can't defer m.nodeLock.Unlock() because m.deadNode will also try to
	// acquire a lock so we need to Unlock before that.

	if m.shutdown {
		m.nodeLock.Unlock()
		panic("leave after shutdown")
	}

	if !m.leave {
		m.leave = true

		state, ok := m.nodeMap[m.config.Name]
		m.nodeLock.Unlock()
		if !ok {
			m.logger.Printf("[WARN] memberlist: Leave but we're not in the node map.")
			return nil
		}

		d := dead{
			Incarnation: state.Incarnation,
			Node:        state.Name,
		}
		m.deadNode(&d)

		// Block until the broadcast goes out
		if m.anyAlive() {
			var timeoutCh <-chan time.Time
			if timeout > 0 {
				timeoutCh = time.After(timeout)
			}
			select {
			case <-m.leaveBroadcast:
			case <-timeoutCh:
				return fmt.Errorf("timeout waiting for leave broadcast")
			}
		}
	} else {
		m.nodeLock.Unlock()
	}

	return nil
}

// Check for any other alive node.
func (m *Memberlist) anyAlive() bool {
	m.nodeLock.RLock()
	defer m.nodeLock.RUnlock()
	for _, n := range m.nodes {
		if n.State != stateDead && n.Name != m.config.Name {
			return true
		}
	}
	return false
}

// GetHealthScore gives this instance's idea of how well it is meeting the soft
// real-time requirements of the protocol. Lower numbers are better, and zero
// means "totally healthy".
func (m *Memberlist) GetHealthScore() int {
	return m.awareness.GetHealthScore()
}

// ProtocolVersion returns the protocol version currently in use by
// this memberlist.
func (m *Memberlist) ProtocolVersion() uint8 {
	// NOTE: This method exists so that in the future we can control
	// any locking if necessary, if we change the protocol version at
	// runtime, etc.
	return m.config.ProtocolVersion
}

// Shutdown will stop any background maintanence of network activity
// for this memberlist, causing it to appear "dead". A leave message
// will not be broadcasted prior, so the cluster being left will have
// to detect this node's shutdown using probing. If you wish to more
// gracefully exit the cluster, call Leave prior to shutting down.
//
// This method is safe to call multiple times.
func (m *Memberlist) Shutdown() error {
	m.nodeLock.Lock()
	defer m.nodeLock.Unlock()

	if m.shutdown {
		return nil
	}

	// Shut down the transport first, which should block until it's
	// completely torn down. If we kill the memberlist-side handlers
	// those I/O handlers might get stuck.
	m.transport.Shutdown()

	// Now tear down everything else.
	m.shutdown = true
	close(m.shutdownCh)
	m.deschedule()
	return nil
}
