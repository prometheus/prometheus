package frankenstein

import (
	"sync"

	"github.com/prometheus/prometheus/storage"

	"github.com/prometheus/common/model"
)

type distributor struct {
	consul     ConsulClient
	ring       *Ring
	cfg        DistributorConfig
	clientsMtx sync.RWMutex
	clients    map[string]storage.SampleAppender

	quit chan struct{}
	done sync.WaitGroup
}

type DistributorConfig struct {
	ConsulHost   string
	ConsulPrefix string
}

type collector struct {
	hostname string
	tokens   []uint64
}

func NewDistributor(cfg DistributorConfig) (*distributor, error) {
	consul, err := NewConsulClient(cfg.ConsulHost)
	if err != nil {
		return nil, err
	}
	d := &distributor{
		consul: consul,
		ring:   NewRing(),
		cfg:    cfg,
		quit:   make(chan struct{}),
	}
	d.done.Add(1)
	go d.loop()
	return d, nil
}

func (d *distributor) Stop() {
	close(d.quit)
	d.done.Wait()
}

func (d *distributor) loop() {
	defer d.done.Done()
	d.consul.WatchPrefix(d.cfg.ConsulPrefix, &collector{}, d.quit, func(key string, value interface{}) bool {
		c := *value.(*collector)
		d.ring.Update(c)
		return true
	})
}

func (d *distributor) getClientFor(hostname string) storage.SampleAppender {
	d.clientsMtx.RLock()
	client, ok := d.clients[hostname]
	d.clientsMtx.RUnlock()
	if ok {
		return client
	}

	d.clientsMtx.Lock()
	defer d.clientsMtx.Unlock()
	client, ok = d.clients[hostname]
	if ok {
		return client
	}

	client = nil // TODO, when we have a client/server for SampleAppender
	d.clients[hostname] = client
	return client
}

// Append implements storage.SampleAppender
func (d *distributor) Append(sample *model.Sample) error {
	for ln, lv := range sample.Metric {
		if len(lv) == 0 {
			delete(sample.Metric, ln)
		}
	}
	key := sample.Metric.Fingerprint()
	collector, err := d.ring.Get(uint64(key))
	if err != nil {
		return err
	}

	client := d.getClientFor(collector.hostname)
	return client.Append(sample)
}

// NeedsThrottling implements storage.SampleAppender
func (*distributor) NeedsThrottling() bool {
	return false
}
