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
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

var (
	samplesIn = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "samples_in_total",
		Help:      "Samples in to remote storage, compare to samples out for queue managers.",
	})
	exemplarsIn = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "exemplars_in_total",
		Help:      "Exemplars in to remote storage, compare to exemplars out for queue managers.",
	})
	histogramsIn = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "histograms_in_total",
		Help:      "HistogramSamples in to remote storage, compare to histograms out for queue managers.",
	})
)

// WriteStorage represents all the remote write storage.
type WriteStorage struct {
	logger log.Logger
	reg    prometheus.Registerer
	mtx    sync.Mutex

	watcherMetrics    *wlog.WatcherMetrics
	liveReaderMetrics *wlog.LiveReaderMetrics
	externalLabels    labels.Labels
	dir               string
	queues            map[string]*QueueManager
	samplesIn         *ewmaRate
	flushDeadline     time.Duration
	interner          *pool
	scraper           ReadyScrapeManager
	quit              chan struct{}

	// For timestampTracker.
	highestTimestamp *maxTimestamp
}

// NewWriteStorage creates and runs a WriteStorage.
func NewWriteStorage(logger log.Logger, reg prometheus.Registerer, dir string, flushDeadline time.Duration, sm ReadyScrapeManager) *WriteStorage {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	rws := &WriteStorage{
		queues:            make(map[string]*QueueManager),
		watcherMetrics:    wlog.NewWatcherMetrics(reg),
		liveReaderMetrics: wlog.NewLiveReaderMetrics(reg),
		logger:            logger,
		reg:               reg,
		flushDeadline:     flushDeadline,
		samplesIn:         newEWMARate(ewmaWeight, shardUpdateDuration),
		dir:               dir,
		interner:          newPool(),
		scraper:           sm,
		quit:              make(chan struct{}),
		highestTimestamp: &maxTimestamp{
			Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "highest_timestamp_in_seconds",
				Help:      "Highest timestamp that has come into the remote storage via the Appender interface, in seconds since epoch.",
			}),
		},
	}
	if reg != nil {
		reg.MustRegister(rws.highestTimestamp)
	}
	go rws.run()
	return rws
}

func (rws *WriteStorage) run() {
	ticker := time.NewTicker(shardUpdateDuration)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rws.samplesIn.tick()
		case <-rws.quit:
			return
		}
	}
}

// ApplyConfig updates the state as the new config requires.
// Only stop & create queues which have changes.
func (rws *WriteStorage) ApplyConfig(conf *config.Config) error {
	rws.mtx.Lock()
	defer rws.mtx.Unlock()

	// Remote write queues only need to change if the remote write config or
	// external labels change.
	externalLabelUnchanged := labels.Equal(conf.GlobalConfig.ExternalLabels, rws.externalLabels)
	rws.externalLabels = conf.GlobalConfig.ExternalLabels

	newQueues := make(map[string]*QueueManager)
	newHashes := []string{}
	for _, rwConf := range conf.RemoteWriteConfigs {
		hash, err := toHash(rwConf)
		if err != nil {
			return err
		}

		// Don't allow duplicate remote write configs.
		if _, ok := newQueues[hash]; ok {
			return fmt.Errorf("duplicate remote write configs are not allowed, found duplicate for URL: %s", rwConf.URL)
		}

		// Set the queue name to the config hash if the user has not set
		// a name in their remote write config so we can still differentiate
		// between queues that have the same remote write endpoint.
		name := hash[:6]
		if rwConf.Name != "" {
			name = rwConf.Name
		}

		c, err := NewWriteClient(name, &ClientConfig{
			URL:              rwConf.URL,
			Timeout:          rwConf.RemoteTimeout,
			HTTPClientConfig: rwConf.HTTPClientConfig,
			SigV4Config:      rwConf.SigV4Config,
			AzureADConfig:    rwConf.AzureADConfig,
			Headers:          rwConf.Headers,
			RetryOnRateLimit: rwConf.QueueConfig.RetryOnRateLimit,
		})
		if err != nil {
			return err
		}

		queue, ok := rws.queues[hash]
		if externalLabelUnchanged && ok {
			// Update the client in case any secret configuration has changed.
			queue.SetClient(c)
			newQueues[hash] = queue
			delete(rws.queues, hash)
			continue
		}

		// Redacted to remove any passwords in the URL (that are
		// technically accepted but not recommended) since this is
		// only used for metric labels.
		endpoint := rwConf.URL.Redacted()
		newQueues[hash] = NewQueueManager(
			newQueueManagerMetrics(rws.reg, name, endpoint),
			rws.watcherMetrics,
			rws.liveReaderMetrics,
			rws.logger,
			rws.dir,
			rws.samplesIn,
			rwConf.QueueConfig,
			rwConf.MetadataConfig,
			conf.GlobalConfig.ExternalLabels,
			rwConf.WriteRelabelConfigs,
			c,
			rws.flushDeadline,
			rws.interner,
			rws.highestTimestamp,
			rws.scraper,
			rwConf.SendExemplars,
			rwConf.SendNativeHistograms,
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
func (rws *WriteStorage) Appender(_ context.Context) storage.Appender {
	return &timestampTracker{
		writeStorage:         rws,
		highestRecvTimestamp: rws.highestTimestamp,
	}
}

// LowestSentTimestamp returns the lowest sent timestamp across all queues.
func (rws *WriteStorage) LowestSentTimestamp() int64 {
	rws.mtx.Lock()
	defer rws.mtx.Unlock()

	var lowestTs int64 = math.MaxInt64

	for _, q := range rws.queues {
		ts := int64(q.metrics.highestSentTimestamp.Get() * 1000)
		if ts < lowestTs {
			lowestTs = ts
		}
	}
	if len(rws.queues) == 0 {
		lowestTs = 0
	}

	return lowestTs
}

// Close closes the WriteStorage.
func (rws *WriteStorage) Close() error {
	rws.mtx.Lock()
	defer rws.mtx.Unlock()
	for _, q := range rws.queues {
		q.Stop()
	}
	close(rws.quit)
	return nil
}

type timestampTracker struct {
	writeStorage         *WriteStorage
	samples              int64
	exemplars            int64
	histograms           int64
	highestTimestamp     int64
	highestRecvTimestamp *maxTimestamp
}

// Append implements storage.Appender.
func (t *timestampTracker) Append(_ storage.SeriesRef, _ labels.Labels, ts int64, _ float64) (storage.SeriesRef, error) {
	t.samples++
	if ts > t.highestTimestamp {
		t.highestTimestamp = ts
	}
	return 0, nil
}

func (t *timestampTracker) AppendExemplar(_ storage.SeriesRef, _ labels.Labels, _ exemplar.Exemplar) (storage.SeriesRef, error) {
	t.exemplars++
	return 0, nil
}

func (t *timestampTracker) AppendHistogram(_ storage.SeriesRef, _ labels.Labels, ts int64, _ *histogram.Histogram, _ *histogram.FloatHistogram) (storage.SeriesRef, error) {
	t.histograms++
	if ts > t.highestTimestamp {
		t.highestTimestamp = ts
	}
	return 0, nil
}

func (t *timestampTracker) UpdateMetadata(_ storage.SeriesRef, _ labels.Labels, _ metadata.Metadata) (storage.SeriesRef, error) {
	// TODO: Add and increment a `metadata` field when we get around to wiring metadata in remote_write.
	// UpadteMetadata is no-op for remote write (where timestampTracker is being used) for now.
	return 0, nil
}

// Commit implements storage.Appender.
func (t *timestampTracker) Commit() error {
	t.writeStorage.samplesIn.incr(t.samples + t.exemplars + t.histograms)

	samplesIn.Add(float64(t.samples))
	exemplarsIn.Add(float64(t.exemplars))
	histogramsIn.Add(float64(t.histograms))
	t.highestRecvTimestamp.Set(float64(t.highestTimestamp / 1000))
	return nil
}

// Rollback implements storage.Appender.
func (*timestampTracker) Rollback() error {
	return nil
}
