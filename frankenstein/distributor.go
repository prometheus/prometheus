package frankenstein

import (
	"sync"

	"github.com/prometheus/common/model"
)

type distributor struct {
	consul ConsulClient
	ring   *Ring
	cfg    distributorConfig

	quit chan struct{}
	done sync.WaitGroup
}

type distributorConfig struct {
	consulHost   string
	consulPrefix string
}

type collector struct {
	hostname string
	tokens   []uint64
}

func NewDistributor(cfg distributorConfig) (*distributor, error) {
	consul, err := NewConsulClient(cfg.consulHost)
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
	d.consul.WatchPrefix(d.cfg.consulPrefix, &collector{}, d.quit, func(key string, value interface{}) bool {
		c := *value.(*collector)
		d.ring.Update(c)
		return true
	})
}

// Append implements storage.SampleAppender
func (d *distributor) Append(*model.Sample) error {
	return nil
}

// NeedsThrottling implements storage.SampleAppender
func (*distributor) NeedsThrottling() bool {
	return false
}
