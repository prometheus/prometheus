// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package frankenstein

import (
	"sync"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage"
)

// Distributor is a storage.SampleAppender which forwards Append calls
// to another consistenty-hashed SampleAppender.
type Distributor struct {
	consul     ConsulClient
	ring       *Ring
	cfg        DistributorConfig
	clientsMtx sync.RWMutex
	clients    map[string]storage.SampleAppender

	quit chan struct{}
	done chan struct{}
}

// DistributorConfig contains the configuration require to
// create a Distributor
type DistributorConfig struct {
	ConsulHost    string
	ConsulPrefix  string
	ClientFactory func(string) (storage.SampleAppender, error)
}

// Collector is the serialised state in Consul representing
// an individual collector.
type Collector struct {
	Hostname string   `json:"hostname"`
	Tokens   []uint64 `json:"tokens"`
}

// NewDistributor constructs a new Distributor
func NewDistributor(cfg DistributorConfig) (*Distributor, error) {
	consul, err := NewConsulClient(cfg.ConsulHost)
	if err != nil {
		return nil, err
	}
	d := &Distributor{
		consul:  consul,
		ring:    NewRing(),
		cfg:     cfg,
		clients: map[string]storage.SampleAppender{},
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go d.loop()
	return d, nil
}

// Stop the distributor.
func (d *Distributor) Stop() {
	close(d.quit)
	<-d.done
}

func (d *Distributor) loop() {
	defer close(d.done)
	d.consul.WatchPrefix(d.cfg.ConsulPrefix, &Collector{}, d.quit, func(key string, value interface{}) bool {
		c := *value.(*Collector)
		log.Infof("Got update to collector: %#v", c)
		d.ring.Update(c)
		return true
	})
}

func (d *Distributor) getClientFor(hostname string) (storage.SampleAppender, error) {
	d.clientsMtx.RLock()
	client, ok := d.clients[hostname]
	d.clientsMtx.RUnlock()
	if ok {
		return client, nil
	}

	d.clientsMtx.Lock()
	defer d.clientsMtx.Unlock()
	client, ok = d.clients[hostname]
	if ok {
		return client, nil
	}

	client, err := d.cfg.ClientFactory(hostname)
	if err != nil {
		return nil, err
	}
	d.clients[hostname] = client
	return client, nil
}

// Append implements storage.SampleAppender
func (d *Distributor) Append(sample *model.Sample) error {
	key := model.SignatureForLabels(sample.Metric, model.MetricNameLabel)
	collector, err := d.ring.Get(uint64(key))
	if err != nil {
		return err
	}

	client, err := d.getClientFor(collector.Hostname)
	if err != nil {
		return err
	}
	return client.Append(sample)
}

// NeedsThrottling implements storage.SampleAppender
func (*Distributor) NeedsThrottling() bool {
	return false
}
