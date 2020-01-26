package dnssrv

import (
	"fmt"
	"net"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/sd"
	"github.com/go-kit/kit/sd/internal/instance"
)

// Instancer yields instances from the named DNS SRV record. The name is
// resolved on a fixed schedule. Priorities and weights are ignored.
type Instancer struct {
	cache  *instance.Cache
	name   string
	logger log.Logger
	quit   chan struct{}
}

// NewInstancer returns a DNS SRV instancer.
func NewInstancer(
	name string,
	ttl time.Duration,
	logger log.Logger,
) *Instancer {
	return NewInstancerDetailed(name, time.NewTicker(ttl), net.LookupSRV, logger)
}

// NewInstancerDetailed is the same as NewInstancer, but allows users to
// provide an explicit lookup refresh ticker instead of a TTL, and specify the
// lookup function instead of using net.LookupSRV.
func NewInstancerDetailed(
	name string,
	refresh *time.Ticker,
	lookup Lookup,
	logger log.Logger,
) *Instancer {
	p := &Instancer{
		cache:  instance.NewCache(),
		name:   name,
		logger: logger,
		quit:   make(chan struct{}),
	}

	instances, err := p.resolve(lookup)
	if err == nil {
		logger.Log("name", name, "instances", len(instances))
	} else {
		logger.Log("name", name, "err", err)
	}
	p.cache.Update(sd.Event{Instances: instances, Err: err})

	go p.loop(refresh, lookup)
	return p
}

// Stop terminates the Instancer.
func (p *Instancer) Stop() {
	close(p.quit)
}

func (p *Instancer) loop(t *time.Ticker, lookup Lookup) {
	defer t.Stop()
	for {
		select {
		case <-t.C:
			instances, err := p.resolve(lookup)
			if err != nil {
				p.logger.Log("name", p.name, "err", err)
				p.cache.Update(sd.Event{Err: err})
				continue // don't replace potentially-good with bad
			}
			p.cache.Update(sd.Event{Instances: instances})

		case <-p.quit:
			return
		}
	}
}

func (p *Instancer) resolve(lookup Lookup) ([]string, error) {
	_, addrs, err := lookup("", "", p.name)
	if err != nil {
		return nil, err
	}
	instances := make([]string, len(addrs))
	for i, addr := range addrs {
		instances[i] = net.JoinHostPort(addr.Target, fmt.Sprint(addr.Port))
	}
	return instances, nil
}

// Register implements Instancer.
func (s *Instancer) Register(ch chan<- sd.Event) {
	s.cache.Register(ch)
}

// Deregister implements Instancer.
func (s *Instancer) Deregister(ch chan<- sd.Event) {
	s.cache.Deregister(ch)
}
