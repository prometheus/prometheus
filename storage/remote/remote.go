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
	"time"

	"github.com/prometheus/prometheus/storage/remote/influxdb"
	"github.com/prometheus/prometheus/storage/remote/opentsdb"

	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"
)

// Storage collects multiple remote storage queues.
type Storage struct {
	queues []*StorageQueueManager
}

// New returns a new remote Storage.
func New(o *Options) *Storage {
	s := &Storage{}
	if o.OpentsdbURL != "" {
		c := opentsdb.NewClient(o.OpentsdbURL, o.StorageTimeout)
		s.queues = append(s.queues, NewStorageQueueManager(c, 100*1024))
	}
	if o.InfluxdbURL != "" {
		c := influxdb.NewClient(o.InfluxdbURL, o.StorageTimeout, o.InfluxdbDatabase, o.InfluxdbRetentionPolicy)
		s.queues = append(s.queues, NewStorageQueueManager(c, 100*1024))
	}
	if len(s.queues) == 0 {
		return nil
	}
	return s
}

// Options contains configuration parameters for a remote storage.
type Options struct {
	StorageTimeout          time.Duration
	InfluxdbURL             string
	InfluxdbRetentionPolicy string
	InfluxdbDatabase        string
	OpentsdbURL             string
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

// Append implements storage.SampleAppender.
func (s *Storage) Append(smpl *clientmodel.Sample) {
	for _, q := range s.queues {
		q.Append(smpl)
	}
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
