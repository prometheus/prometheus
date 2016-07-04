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
	"fmt"
	"sync"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/metric"
)

// An IngesterClient groups functionality for writing to and reading from a
// single ingester.
type IngesterClient struct {
	Appender storage.SampleAppender
	Querier  Querier
}

// Distributor is a storage.SampleAppender and a frankenstein.Querier which
// forwards appends and queries to individual ingesters.
type Distributor struct {
	consul     ConsulClient
	ring       *Ring
	cfg        DistributorConfig
	clientsMtx sync.RWMutex
	clients    map[string]*IngesterClient

	quit chan struct{}
	done chan struct{}
}

// DistributorConfig contains the configuration require to
// create a Distributor
type DistributorConfig struct {
	ConsulHost    string
	ConsulPrefix  string
	ClientFactory func(string) (*IngesterClient, error)
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
		clients: map[string]*IngesterClient{},
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

func (d *Distributor) getClientFor(hostname string) (*IngesterClient, error) {
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

// Append implements SampleAppender.
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
	return client.Appender.Append(sample)
}

func metricNameFromLabelMatchers(matchers ...*metric.LabelMatcher) (model.LabelValue, error) {
	for _, m := range matchers {
		if m.Name == model.MetricNameLabel {
			if m.Type != metric.Equal {
				return "", fmt.Errorf("non-equality matchers are not supported on the metric name")
			}
			return m.Value, nil
		}
	}
	return "", fmt.Errorf("no metric name matcher found")
}

// Query implements Querier.
func (d *Distributor) Query(from, to model.Time, matchers ...*metric.LabelMatcher) (model.Matrix, error) {
	metricName, err := metricNameFromLabelMatchers(matchers...)
	if err != nil {
		return nil, err
	}

	metric := model.Metric{
		model.MetricNameLabel: metricName,
	}
	key := model.SignatureForLabels(metric, model.MetricNameLabel)
	collector, err := d.ring.Get(uint64(key))
	if err != nil {
		return nil, err
	}

	client, err := d.getClientFor(collector.Hostname)
	if err != nil {
		return nil, err
	}

	return client.Querier.Query(from, to, matchers...)
}

// NeedsThrottling implements SampleAppender.
func (*Distributor) NeedsThrottling() bool {
	return false
}
