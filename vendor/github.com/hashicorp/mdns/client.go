package mdns

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go.net/ipv4"
	"github.com/hashicorp/go.net/ipv6"
	"github.com/miekg/dns"
)

// ServiceEntry is returned after we query for a service
type ServiceEntry struct {
	Name       string
	Host       string
	AddrV4     net.IP
	AddrV6     net.IP
	Port       int
	Info       string
	InfoFields []string

	Addr net.IP // @Deprecated

	hasTXT bool
	sent   bool
}

// complete is used to check if we have all the info we need
func (s *ServiceEntry) complete() bool {
	return (s.AddrV4 != nil || s.AddrV6 != nil || s.Addr != nil) && s.Port != 0 && s.hasTXT
}

// QueryParam is used to customize how a Lookup is performed
type QueryParam struct {
	Service             string               // Service to lookup
	Domain              string               // Lookup domain, default "local"
	Timeout             time.Duration        // Lookup timeout, default 1 second
	Interface           *net.Interface       // Multicast interface to use
	Entries             chan<- *ServiceEntry // Entries Channel
	WantUnicastResponse bool                 // Unicast response desired, as per 5.4 in RFC
}

// DefaultParams is used to return a default set of QueryParam's
func DefaultParams(service string) *QueryParam {
	return &QueryParam{
		Service:             service,
		Domain:              "local",
		Timeout:             time.Second,
		Entries:             make(chan *ServiceEntry),
		WantUnicastResponse: false, // TODO(reddaly): Change this default.
	}
}

// Query looks up a given service, in a domain, waiting at most
// for a timeout before finishing the query. The results are streamed
// to a channel. Sends will not block, so clients should make sure to
// either read or buffer.
func Query(params *QueryParam) error {
	// Create a new client
	client, err := newClient()
	if err != nil {
		return err
	}
	defer client.Close()

	// Set the multicast interface
	if params.Interface != nil {
		if err := client.setInterface(params.Interface); err != nil {
			return err
		}
	}

	// Ensure defaults are set
	if params.Domain == "" {
		params.Domain = "local"
	}
	if params.Timeout == 0 {
		params.Timeout = time.Second
	}

	// Run the query
	return client.query(params)
}

// Lookup is the same as Query, however it uses all the default parameters
func Lookup(service string, entries chan<- *ServiceEntry) error {
	params := DefaultParams(service)
	params.Entries = entries
	return Query(params)
}

// Client provides a query interface that can be used to
// search for service providers using mDNS
type client struct {
	ipv4UnicastConn *net.UDPConn
	ipv6UnicastConn *net.UDPConn

	ipv4MulticastConn *net.UDPConn
	ipv6MulticastConn *net.UDPConn

	closed    bool
	closedCh  chan struct{} // TODO(reddaly): This doesn't appear to be used.
	closeLock sync.Mutex
}

// NewClient creates a new mdns Client that can be used to query
// for records
func newClient() (*client, error) {
	// TODO(reddaly): At least attempt to bind to the port required in the spec.
	// Create a IPv4 listener
	uconn4, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		log.Printf("[ERR] mdns: Failed to bind to udp4 port: %v", err)
	}
	uconn6, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6zero, Port: 0})
	if err != nil {
		log.Printf("[ERR] mdns: Failed to bind to udp6 port: %v", err)
	}

	if uconn4 == nil && uconn6 == nil {
		return nil, fmt.Errorf("failed to bind to any unicast udp port")
	}

	mconn4, err := net.ListenMulticastUDP("udp4", nil, ipv4Addr)
	if err != nil {
		log.Printf("[ERR] mdns: Failed to bind to udp4 port: %v", err)
	}
	mconn6, err := net.ListenMulticastUDP("udp6", nil, ipv6Addr)
	if err != nil {
		log.Printf("[ERR] mdns: Failed to bind to udp6 port: %v", err)
	}

	if mconn4 == nil && mconn6 == nil {
		return nil, fmt.Errorf("failed to bind to any multicast udp port")
	}

	c := &client{
		ipv4MulticastConn: mconn4,
		ipv6MulticastConn: mconn6,
		ipv4UnicastConn:   uconn4,
		ipv6UnicastConn:   uconn6,
		closedCh:          make(chan struct{}),
	}
	return c, nil
}

// Close is used to cleanup the client
func (c *client) Close() error {
	c.closeLock.Lock()
	defer c.closeLock.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	log.Printf("[INFO] mdns: Closing client %v", *c)
	close(c.closedCh)

	if c.ipv4UnicastConn != nil {
		c.ipv4UnicastConn.Close()
	}
	if c.ipv6UnicastConn != nil {
		c.ipv6UnicastConn.Close()
	}
	if c.ipv4MulticastConn != nil {
		c.ipv4MulticastConn.Close()
	}
	if c.ipv6MulticastConn != nil {
		c.ipv6MulticastConn.Close()
	}

	return nil
}

// setInterface is used to set the query interface, uses sytem
// default if not provided
func (c *client) setInterface(iface *net.Interface) error {
	p := ipv4.NewPacketConn(c.ipv4UnicastConn)
	if err := p.SetMulticastInterface(iface); err != nil {
		return err
	}
	p2 := ipv6.NewPacketConn(c.ipv6UnicastConn)
	if err := p2.SetMulticastInterface(iface); err != nil {
		return err
	}
	p = ipv4.NewPacketConn(c.ipv4MulticastConn)
	if err := p.SetMulticastInterface(iface); err != nil {
		return err
	}
	p2 = ipv6.NewPacketConn(c.ipv6MulticastConn)
	if err := p2.SetMulticastInterface(iface); err != nil {
		return err
	}
	return nil
}

// query is used to perform a lookup and stream results
func (c *client) query(params *QueryParam) error {
	// Create the service name
	serviceAddr := fmt.Sprintf("%s.%s.", trimDot(params.Service), trimDot(params.Domain))

	// Start listening for response packets
	msgCh := make(chan *dns.Msg, 32)
	go c.recv(c.ipv4UnicastConn, msgCh)
	go c.recv(c.ipv6UnicastConn, msgCh)
	go c.recv(c.ipv4MulticastConn, msgCh)
	go c.recv(c.ipv6MulticastConn, msgCh)

	// Send the query
	m := new(dns.Msg)
	m.SetQuestion(serviceAddr, dns.TypePTR)
	// RFC 6762, section 18.12.  Repurposing of Top Bit of qclass in Question
	// Section
	//
	// In the Question Section of a Multicast DNS query, the top bit of the qclass
	// field is used to indicate that unicast responses are preferred for this
	// particular question.  (See Section 5.4.)
	if params.WantUnicastResponse {
		m.Question[0].Qclass |= 1 << 15
	}
	m.RecursionDesired = false
	if err := c.sendQuery(m); err != nil {
		return err
	}

	// Map the in-progress responses
	inprogress := make(map[string]*ServiceEntry)

	// Listen until we reach the timeout
	finish := time.After(params.Timeout)
	for {
		select {
		case resp := <-msgCh:
			var inp *ServiceEntry
			for _, answer := range append(resp.Answer, resp.Extra...) {
				// TODO(reddaly): Check that response corresponds to serviceAddr?
				switch rr := answer.(type) {
				case *dns.PTR:
					// Create new entry for this
					inp = ensureName(inprogress, rr.Ptr)

				case *dns.SRV:
					// Check for a target mismatch
					if rr.Target != rr.Hdr.Name {
						alias(inprogress, rr.Hdr.Name, rr.Target)
					}

					// Get the port
					inp = ensureName(inprogress, rr.Hdr.Name)
					inp.Host = rr.Target
					inp.Port = int(rr.Port)

				case *dns.TXT:
					// Pull out the txt
					inp = ensureName(inprogress, rr.Hdr.Name)
					inp.Info = strings.Join(rr.Txt, "|")
					inp.InfoFields = rr.Txt
					inp.hasTXT = true

				case *dns.A:
					// Pull out the IP
					inp = ensureName(inprogress, rr.Hdr.Name)
					inp.Addr = rr.A // @Deprecated
					inp.AddrV4 = rr.A

				case *dns.AAAA:
					// Pull out the IP
					inp = ensureName(inprogress, rr.Hdr.Name)
					inp.Addr = rr.AAAA // @Deprecated
					inp.AddrV6 = rr.AAAA
				}
			}

			if inp == nil {
				continue
			}

			// Check if this entry is complete
			if inp.complete() {
				if inp.sent {
					continue
				}
				inp.sent = true
				select {
				case params.Entries <- inp:
				default:
				}
			} else {
				// Fire off a node specific query
				m := new(dns.Msg)
				m.SetQuestion(inp.Name, dns.TypePTR)
				m.RecursionDesired = false
				if err := c.sendQuery(m); err != nil {
					log.Printf("[ERR] mdns: Failed to query instance %s: %v", inp.Name, err)
				}
			}
		case <-finish:
			return nil
		}
	}
}

// sendQuery is used to multicast a query out
func (c *client) sendQuery(q *dns.Msg) error {
	buf, err := q.Pack()
	if err != nil {
		return err
	}
	if c.ipv4UnicastConn != nil {
		c.ipv4UnicastConn.WriteToUDP(buf, ipv4Addr)
	}
	if c.ipv6UnicastConn != nil {
		c.ipv6UnicastConn.WriteToUDP(buf, ipv6Addr)
	}
	return nil
}

// recv is used to receive until we get a shutdown
func (c *client) recv(l *net.UDPConn, msgCh chan *dns.Msg) {
	if l == nil {
		return
	}
	buf := make([]byte, 65536)
	for !c.closed {
		n, err := l.Read(buf)
		if err != nil {
			log.Printf("[ERR] mdns: Failed to read packet: %v", err)
			continue
		}
		msg := new(dns.Msg)
		if err := msg.Unpack(buf[:n]); err != nil {
			log.Printf("[ERR] mdns: Failed to unpack packet: %v", err)
			continue
		}
		select {
		case msgCh <- msg:
		case <-c.closedCh:
			return
		}
	}
}

// ensureName is used to ensure the named node is in progress
func ensureName(inprogress map[string]*ServiceEntry, name string) *ServiceEntry {
	if inp, ok := inprogress[name]; ok {
		return inp
	}
	inp := &ServiceEntry{
		Name: name,
	}
	inprogress[name] = inp
	return inp
}

// alias is used to setup an alias between two entries
func alias(inprogress map[string]*ServiceEntry, src, dst string) {
	srcEntry := ensureName(inprogress, src)
	inprogress[dst] = srcEntry
}
