package agent

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/hashicorp/mdns"
)

const (
	mdnsPollInterval  = 60 * time.Second
	mdnsQuietInterval = 100 * time.Millisecond
)

// AgentMDNS is used to advertise ourself using mDNS and to
// attempt to join peers periodically using mDNS queries.
type AgentMDNS struct {
	agent    *Agent
	discover string
	logger   *log.Logger
	seen     map[string]struct{}
	server   *mdns.Server
	replay   bool
	iface    *net.Interface
}

// NewAgentMDNS is used to create a new AgentMDNS
func NewAgentMDNS(agent *Agent, logOutput io.Writer, replay bool,
	node, discover string, iface *net.Interface, bind net.IP, port int) (*AgentMDNS, error) {
	// Create the service
	service, err := mdns.NewMDNSService(
		node,
		mdnsName(discover),
		"",
		"",
		port,
		[]net.IP{bind},
		[]string{fmt.Sprintf("Serf '%s' cluster", discover)})
	if err != nil {
		return nil, err
	}

	// Configure mdns server
	conf := &mdns.Config{
		Zone:  service,
		Iface: iface,
	}

	// Create the server
	server, err := mdns.NewServer(conf)
	if err != nil {
		return nil, err
	}

	// Initialize the AgentMDNS
	m := &AgentMDNS{
		agent:    agent,
		discover: discover,
		logger:   log.New(logOutput, "", log.LstdFlags),
		seen:     make(map[string]struct{}),
		server:   server,
		replay:   replay,
		iface:    iface,
	}

	// Start the background workers
	go m.run()
	return m, nil
}

// run is a long running goroutine that scans for new hosts periodically
func (m *AgentMDNS) run() {
	hosts := make(chan *mdns.ServiceEntry, 32)
	poll := time.After(0)
	var quiet <-chan time.Time
	var join []string

	for {
		select {
		case h := <-hosts:
			// Format the host address
			addr := net.TCPAddr{IP: h.Addr, Port: h.Port}
			addrS := addr.String()

			// Skip if we've handled this host already
			if _, ok := m.seen[addrS]; ok {
				continue
			}

			// Queue for handling
			join = append(join, addrS)
			quiet = time.After(mdnsQuietInterval)

		case <-quiet:
			// Attempt the join
			n, err := m.agent.Join(join, m.replay)
			if err != nil {
				m.logger.Printf("[ERR] agent.mdns: Failed to join: %v", err)
			}
			if n > 0 {
				m.logger.Printf("[INFO] agent.mdns: Joined %d hosts", n)
			}

			// Mark all as seen
			for _, n := range join {
				m.seen[n] = struct{}{}
			}
			join = nil

		case <-poll:
			poll = time.After(mdnsPollInterval)
			go m.poll(hosts)
		}
	}
}

// poll is invoked periodically to check for new hosts
func (m *AgentMDNS) poll(hosts chan *mdns.ServiceEntry) {
	params := mdns.QueryParam{
		Service:   mdnsName(m.discover),
		Interface: m.iface,
		Entries:   hosts,
	}
	if err := mdns.Query(&params); err != nil {
		m.logger.Printf("[ERR] agent.mdns: Failed to poll for new hosts: %v", err)
	}
}

// mdnsName returns the service name to register and to lookup
func mdnsName(discover string) string {
	return fmt.Sprintf("_serf_%s._tcp", discover)
}
