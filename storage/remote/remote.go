// Copyright 2015 The Prometheus Authors
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

package remote

import (
	"net/url"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	influx "github.com/influxdb/influxdb/client"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage/remote/graphite"
	"github.com/prometheus/prometheus/storage/remote/influxdb"
	"github.com/prometheus/prometheus/storage/remote/opentsdb"
)

// Storage collects multiple remote storage queues.
type Storage struct {
	queues         []*StorageQueueManager
	externalLabels model.LabelSet
	mtx            sync.RWMutex
}

// ApplyConfig updates the status state as the new config requires.
func (s *Storage) ApplyConfig(conf *config.Config) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.externalLabels = conf.GlobalConfig.ExternalLabels
	return nil
}

// New returns a new remote Storage.
func New(o *Options) (*Storage, error) {
	s := &Storage{}
	if o.GraphiteAddress != "" {
		c := graphite.NewClient(
			o.GraphiteAddress, o.GraphiteTransport,
			o.StorageTimeout, o.GraphitePrefix)
		s.queues = append(s.queues, NewStorageQueueManager(c, nil))
	}
	if o.OpentsdbURL != "" {
		c := opentsdb.NewClient(o.OpentsdbURL, o.StorageTimeout)
		s.queues = append(s.queues, NewStorageQueueManager(c, nil))
	}
	if o.InfluxdbURL != nil {
		conf := influx.Config{
			URL:      *o.InfluxdbURL,
			Username: o.InfluxdbUsername,
			Password: o.InfluxdbPassword,
			Timeout:  o.StorageTimeout,
		}
		c := influxdb.NewClient(conf, o.InfluxdbDatabase, o.InfluxdbRetentionPolicy)
		prometheus.MustRegister(c)
		s.queues = append(s.queues, NewStorageQueueManager(c, nil))
	}
	if o.Address != "" {
		c, err := NewClient(o.Address, o.StorageTimeout)
		if err != nil {
			return nil, err
		}
		s.queues = append(s.queues, NewStorageQueueManager(c, nil))
	}
	if len(s.queues) == 0 {
		return nil, nil
	}
	return s, nil
}

// Options contains configuration parameters for a remote storage.
type Options struct {
	StorageTimeout          time.Duration
	InfluxdbURL             *url.URL
	InfluxdbRetentionPolicy string
	InfluxdbUsername        string
	InfluxdbPassword        string
	InfluxdbDatabase        string
	OpentsdbURL             string
	GraphiteAddress         string
	GraphiteTransport       string
	GraphitePrefix          string
	// TODO: This just being called "Address" will make more sense once the
	// other remote storage mechanisms are removed.
	Address string
}

// Run starts the background processing of the storage queues.
func (s *Storage) Run() {
	for _, q := range s.queues {
		go q.Run()
	}
}

// Stop the background processing of the storage queues.
func (s *Storage) Stop() {
	for _, q := range s.queues {
		q.Stop()
	}
}

// Append implements storage.SampleAppender. Always returns nil.
func (s *Storage) Append(smpl *model.Sample) error {
	s.mtx.RLock()

	var snew model.Sample
	snew = *smpl
	snew.Metric = smpl.Metric.Clone()

	for ln, lv := range s.externalLabels {
		if _, ok := smpl.Metric[ln]; !ok {
			snew.Metric[ln] = lv
		}
	}
	s.mtx.RUnlock()

	for _, q := range s.queues {
		q.Append(&snew)
	}
	return nil
}

// NeedsThrottling implements storage.SampleAppender. It will always return
// false as a remote storage drops samples on the floor if backlogging instead
// of asking for throttling.
func (s *Storage) NeedsThrottling() bool {
	return false
}

// Describe implements prometheus.Collector.
func (s *Storage) Describe(ch chan<- *prometheus.Desc) {
	for _, q := range s.queues {
		q.Describe(ch)
	}
}

// Collect implements prometheus.Collector.
func (s *Storage) Collect(ch chan<- prometheus.Metric) {
	for _, q := range s.queues {
		q.Collect(ch)
	}
}
