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
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
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
	reg    prometheus.Registerer
	logger log.Logger
	mtx    sync.Mutex

	queueMetrics      *queueManagerMetrics
	configHash        string
	externalLabelHash string
	walDir            string
	queues            map[string]*QueueManager
	samplesIn         *ewmaRate
	flushDeadline     time.Duration
}

// NewWriteStorage creates and runs a WriteStorage.
func NewWriteStorage(logger log.Logger, reg prometheus.Registerer, walDir string, flushDeadline time.Duration) *WriteStorage {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	rws := &WriteStorage{
		queues:        make(map[string]*QueueManager),
		reg:           reg,
		queueMetrics:  newQueueManagerMetrics(reg),
		logger:        logger,
		flushDeadline: flushDeadline,
		samplesIn:     newEWMARate(ewmaWeight, shardUpdateDuration),
		walDir:        walDir,
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
// Only stop & create queues which have changes.
func (rws *WriteStorage) ApplyConfig(conf *config.Config) error {
	rws.mtx.Lock()
	defer rws.mtx.Unlock()

	configHash, err := toHash(conf.RemoteWriteConfigs)
	if err != nil {
		return err
	}
	externalLabelHash, err := toHash(conf.GlobalConfig.ExternalLabels)
	if err != nil {
		return err
	}

	// Remote write queues only need to change if the remote write config or
	// external labels change.
	externalLabelUnchanged := externalLabelHash == rws.externalLabelHash
	if configHash == rws.configHash && externalLabelUnchanged {
		level.Debug(rws.logger).Log("msg", "remote write config has not changed, no need to restart QueueManagers")
		return nil
	}

	rws.configHash = configHash
	rws.externalLabelHash = externalLabelHash

	newQueues := make(map[string]*QueueManager)
	newHashes := []string{}
	for _, rwConf := range conf.RemoteWriteConfigs {
		hash, err := toHash(rwConf)
		if err != nil {
			return err
		}

		// Set the queue name to the config hash if the user has not set
		// a name in their remote write config so we can still differentiate
		// between queues that have the same remote write endpoint.
		name := string(hash[:6])
		if rwConf.Name != "" {
			name = rwConf.Name
		}

		// Don't allow duplicate remote write configs.
		if _, ok := newQueues[hash]; ok {
			return fmt.Errorf("duplicate remote write configs are not allowed, found duplicate for URL: %s", rwConf.URL)
		}

		var nameUnchanged bool
		queue, ok := rws.queues[hash]
		if ok {
			nameUnchanged = queue.client.Name() == name
		}
		if externalLabelUnchanged && nameUnchanged {
			newQueues[hash] = queue
			delete(rws.queues, hash)
			continue
		}

		c, err := NewClient(name, &ClientConfig{
			URL:              rwConf.URL,
			Timeout:          rwConf.RemoteTimeout,
			HTTPClientConfig: rwConf.HTTPClientConfig,
		})
		if err != nil {
			return err
		}
		newQueues[hash] = NewQueueManager(
			rws.reg,
			rws.queueMetrics,
			rws.logger,
			rws.walDir,
			rws.samplesIn,
			rwConf.QueueConfig,
			conf.GlobalConfig.ExternalLabels,
			rwConf.WriteRelabelConfigs,
			c,
			rws.flushDeadline,
		)
		// Keep track of which queues are new so we know which to start.
		newHashes = append(newHashes, hash)
	}

	// Anything remaining in rws.queues is a queue who's config has
	// changed or was removed from the overall remote write config.
	for _, q := range rws.queues {
		q.Stop()
	}

	for _, hash := range newHashes {
		newQueues[hash].Start()
	}

	rws.queues = newQueues

	return nil
}

// Appender implements storage.Storage.
func (rws *WriteStorage) Appender() storage.Appender {
	return &timestampTracker{
		writeStorage: rws,
	}
}

// Close closes the WriteStorage.
func (rws *WriteStorage) Close() error {
	rws.mtx.Lock()
	defer rws.mtx.Unlock()
	for _, q := range rws.queues {
		q.Stop()
	}
	return nil
}

type timestampTracker struct {
	writeStorage     *WriteStorage
	samples          int64
	highestTimestamp int64
}

// Add implements storage.Appender.
func (t *timestampTracker) Add(_ labels.Labels, ts int64, _ float64) (uint64, error) {
	t.samples++
	if ts > t.highestTimestamp {
		t.highestTimestamp = ts
	}
	return 0, nil
}

// AddFast implements storage.Appender.
func (t *timestampTracker) AddFast(_ uint64, ts int64, v float64) error {
	_, err := t.Add(nil, ts, v)
	return err
}

// Commit implements storage.Appender.
func (t *timestampTracker) Commit() error {
	t.writeStorage.samplesIn.incr(t.samples)

	samplesIn.Add(float64(t.samples))
	highestTimestamp.Set(float64(t.highestTimestamp / 1000))
	return nil
}

// Rollback implements storage.Appender.
func (*timestampTracker) Rollback() error {
	return nil
}
