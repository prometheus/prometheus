// Copyright 2017 The Prometheus Authors
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
	"crypto/md5"
	"encoding/json"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/tsdb"
)

var (
	samplesIn = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "samples_in_total",
		Help:      "Samples in to remote storage, compare to samples out for queue managers.",
	})
	highestTimestamp = maxGauge{
		Gauge: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "highest_timestamp_in_seconds",
			Help:      "Highest timestamp that has come into the remote storage via the Appender interface, in seconds since epoch.",
		}),
	}
)

// WriteStorage represents all the remote write storage.
type WriteStorage struct {
	logger log.Logger
	mtx    sync.Mutex

	configHash    [16]byte
	walDir        string
	queues        []*QueueManager
	samplesIn     *ewmaRate
	flushDeadline time.Duration
	walWatcher    *WALWatcher
}

// NewWriteStorage creates and runs a WriteStorage.
func NewWriteStorage(logger log.Logger, walDir string, flushDeadline time.Duration, useWALForRate bool) *WriteStorage {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	rws := &WriteStorage{
		logger:        logger,
		flushDeadline: flushDeadline,
		samplesIn:     newEWMARate(ewmaWeight, shardUpdateDuration),
		walDir:        walDir,
	}
	if useWALForRate {
		rws.walWatcher = NewWALWatcher(logger, "samplesInRate", rws.writeTo(), walDir)
		rws.walWatcher.Start()
	}
	go rws.run()
	return rws
}

func (rws *WriteStorage) run() {
	ticker := time.NewTicker(shardUpdateDuration)
	defer ticker.Stop()
	for range ticker.C {
		rws.samplesIn.tick()
	}
}

// ApplyConfig updates the state as the new config requires.
func (rws *WriteStorage) ApplyConfig(conf *config.Config) error {
	rws.mtx.Lock()
	defer rws.mtx.Unlock()

	// Remote write queues only need to change if the remote write config or
	// external labels change. Hash these together and only reload if the hash
	// changes.
	cfgBytes, err := json.Marshal(conf.RemoteWriteConfigs)
	if err != nil {
		return err
	}
	externalLabelBytes, err := json.Marshal(conf.GlobalConfig.ExternalLabels)
	if err != nil {
		return err
	}

	hash := md5.Sum(append(cfgBytes, externalLabelBytes...))
	if hash == rws.configHash {
		level.Debug(rws.logger).Log("msg", "remote write config has not changed, no need to restart QueueManagers")
		return nil
	}

	rws.configHash = hash

	// Update write queues
	newQueues := []*QueueManager{}
	// TODO: we should only stop & recreate queues which have changes,
	// as this can be quite disruptive.
	for i, rwConf := range conf.RemoteWriteConfigs {
		c, err := NewClient(i, &ClientConfig{
			URL:              rwConf.URL,
			Timeout:          rwConf.RemoteTimeout,
			HTTPClientConfig: rwConf.HTTPClientConfig,
		})
		if err != nil {
			return err
		}
		newQueues = append(newQueues, NewQueueManager(
			rws.logger,
			rws.walDir,
			rws.samplesIn,
			rwConf.QueueConfig,
			conf.GlobalConfig.ExternalLabels,
			rwConf.WriteRelabelConfigs,
			c,
			rws.flushDeadline,
		))
	}

	for _, q := range rws.queues {
		q.Stop()
	}

	rws.queues = newQueues
	for _, q := range rws.queues {
		q.Start()
	}
	return nil
}

// Appender implements storage.Storage.
func (rws *WriteStorage) Appender() (storage.Appender, error) {
	return &timestampTracker{
		remoteWriteStorage: rws,
	}, nil
}

func (rws *WriteStorage) writeTo() writeTo {
	return &timestampTracker{
		remoteWriteStorage: rws,
	}
}

// Close closes the RemoteWriteStorage.
func (rws *WriteStorage) Close() error {
	rws.mtx.Lock()
	defer rws.mtx.Unlock()
	for _, q := range rws.queues {
		q.Stop()
	}
	if rws.walWatcher != nil {
		rws.walWatcher.Stop()
	}
	return nil
}

type timestampTracker struct {
	remoteWriteStorage *WriteStorage
	samples            int64
	highestTimestamp   int64
}

// Add implements storage.Appender.
func (t *timestampTracker) Add(_ labels.Labels, ts int64, v float64) (uint64, error) {
	t.samples++
	if ts > t.highestTimestamp {
		t.highestTimestamp = ts
	}
	return 0, nil
}

// AddFast implements storage.Appender.
func (t *timestampTracker) AddFast(l labels.Labels, _ uint64, ts int64, v float64) error {
	_, err := t.Add(l, ts, v)
	return err
}

// Commit implements storage.Appender.
func (t *timestampTracker) Commit() error {
	t.remoteWriteStorage.samplesIn.incr(t.samples)

	samplesIn.Add(float64(t.samples))
	highestTimestamp.Set(float64(t.highestTimestamp / 1000))
	return nil
}

// Rollback implements storage.Appender.
func (*timestampTracker) Rollback() error {
	return nil
}

// Append implements writeTo.
func (t *timestampTracker) Append(samples []tsdb.RefSample) bool {
	for _, sample := range samples {
		if sample.T > t.highestTimestamp {
			t.highestTimestamp = sample.T
		}
	}
	t.remoteWriteStorage.samplesIn.incr(int64(len(samples)))
	samplesIn.Add(float64(len(samples)))
	highestTimestamp.Set(float64(t.highestTimestamp / 1000))
	return true
}

// StoreSeries implements writeTo.
func (t *timestampTracker) StoreSeries(_ []tsdb.RefSeries, _ int) {}

// StoreSeries implements writeTo.
func (t *timestampTracker) SeriesReset(_ int) {}
