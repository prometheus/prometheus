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
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/storage/metric"
)

// Distributor is a storage.SampleAppender and a frankenstein.Querier which
// forwards appends and queries to individual ingesters.
type Distributor struct {
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
	ConsulClient  ConsulClient
	ConsulPrefix  string
	ClientFactory func(string) (*IngesterClient, error)
}

// Collector is the serialised state in Consul representing
// an individual collector.
type Collector struct {
	ID       string   `json:"ID"`
	Hostname string   `json:"hostname"`
	Tokens   []uint64 `json:"tokens"`
}

// NewDistributor constructs a new Distributor
func NewDistributor(cfg DistributorConfig) (*Distributor, error) {
	d := &Distributor{
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
	d.cfg.ConsulClient.WatchPrefix(d.cfg.ConsulPrefix, &Collector{}, d.quit, func(key string, value interface{}) bool {
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
func (d *Distributor) Append(ctx context.Context, samples []*model.Sample) error {
	samplesByIngester := map[string][]*model.Sample{}
	for _, sample := range samples {
		key := model.SignatureForLabels(sample.Metric, model.MetricNameLabel)
		collector, err := d.ring.Get(uint64(key))
		if err != nil {
			return err
		}
		otherSamples := samplesByIngester[collector.Hostname]
		samplesByIngester[collector.Hostname] = append(otherSamples, sample)
	}

	// TODO paralellise
	for hostname, samples := range samplesByIngester {
		client, err := d.getClientFor(hostname)
		if err != nil {
			return err
		}
		if err := client.Append(ctx, samples); err != nil {
			return err
		}
	}

	return nil
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
func (d *Distributor) Query(ctx context.Context, from, to model.Time, matchers ...*metric.LabelMatcher) (model.Matrix, error) {
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

	return client.Query(ctx, from, to, matchers...)
}

// NeedsThrottling implements SampleAppender.
func (*Distributor) NeedsThrottling(_ context.Context) bool {
	return false
}
