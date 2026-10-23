// Copyright The Prometheus Authors
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
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

// savepointWriteInterval is the interval at which run() writes the savepoint.
// It is a var (not a const) so tests can shorten it.
var savepointWriteInterval = 30 * time.Second

var (
	samplesIn = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "samples_in_total",
		Help:      "Samples in to remote storage, compare to samples out for queue managers. Deprecated, check prometheus_wal_watcher_records_read_total and prometheus_remote_storage_samples_dropped_total",
	})
	exemplarsIn = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "exemplars_in_total",
		Help:      "Exemplars in to remote storage, compare to exemplars out for queue managers. Deprecated, check prometheus_wal_watcher_records_read_total and prometheus_remote_storage_exemplars_dropped_total",
	})
	histogramsIn = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "histograms_in_total",
		Help:      "HistogramSamples in to remote storage, compare to histograms out for queue managers. Deprecated, check prometheus_wal_watcher_records_read_total and prometheus_remote_storage_histograms_dropped_total",
	})
)

// WriteStorage represents all the remote write storage.
type WriteStorage struct {
	logger *slog.Logger
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
	done              chan struct{}

	recordBuf *record.BuffersPool

	savepoint Savepoint
	// configApplied is set once the first ApplyConfig succeeds.
	configApplied bool

	// For timestampTracker.
	highestTimestamp *maxTimestamp

	// Savepoint observability metrics. Non-nil only when enableSavepoint is true.
	savepointWritesTotal   prometheus.Counter
	savepointWritesFailed  prometheus.Counter
	savepointLastWriteTime prometheus.Gauge
	savepointEntries       prometheus.Gauge

	enableTypeAndUnitLabels bool
	enableSavepoint         bool
}

// NewWriteStorage creates and runs a WriteStorage.
func NewWriteStorage(logger *slog.Logger, reg prometheus.Registerer, dir string, flushDeadline time.Duration, sm ReadyScrapeManager, enableTypeAndUnitLabels, enableSavepoint bool) *WriteStorage {
	if logger == nil {
		logger = promslog.NewNopLogger()
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
		done:              make(chan struct{}),
		highestTimestamp: &maxTimestamp{
			Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "highest_timestamp_in_seconds",
				Help:      "Highest timestamp that has come into the remote storage via the Appender interface, in seconds since epoch. Initialized to 0 when no data has been received yet. Deprecated, check prometheus_remote_storage_queue_highest_timestamp_seconds which is more accurate.",
			}),
		},
		recordBuf:               record.NewBuffersPool(),
		enableTypeAndUnitLabels: enableTypeAndUnitLabels,
		enableSavepoint:         enableSavepoint,
	}

	if enableSavepoint {
		rws.savepointWritesTotal = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "savepoint_writes_total",
			Help:      "Total number of remote write savepoint write attempts.",
		})
		rws.savepointWritesFailed = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "savepoint_writes_failed_total",
			Help:      "Total number of remote write savepoint write attempts that failed.",
		})
		rws.savepointLastWriteTime = prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "savepoint_last_write_timestamp_seconds",
			Help:      "Unix timestamp of the last successful remote write savepoint write.",
		})
		rws.savepointEntries = prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "savepoint_entries",
			Help:      "Number of queue entries in the most recently written remote write savepoint.",
		})
		if reg != nil {
			reg.MustRegister(
				rws.savepointWritesTotal,
				rws.savepointWritesFailed,
				rws.savepointLastWriteTime,
				rws.savepointEntries,
			)
		}

		sp, err := LoadSavepoint(dir)
		if err != nil {
			logger.Warn("Failed to load remote write savepoint, starting from current position", "err", err)
			sp = make(Savepoint)
		}
		rws.savepoint = sp
	} else if err := os.Remove(savepointFilePath(dir)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		// Drop any stale savepoint left over from a previous run with the feature
		// enabled, so it cannot be acted upon if the feature is re-enabled later.
		logger.Warn("Failed to remove stale remote write savepoint", "err", err)
	}

	if reg != nil {
		reg.MustRegister(rws.highestTimestamp)
	}
	go rws.run()
	return rws
}

func (rws *WriteStorage) run() {
	defer close(rws.done)

	shardTicker := time.NewTicker(shardUpdateDuration)
	defer shardTicker.Stop()

	var savepointC <-chan time.Time
	if rws.enableSavepoint {
		savepointTicker := time.NewTicker(savepointWriteInterval)
		defer savepointTicker.Stop()
		savepointC = savepointTicker.C
	}

	for {
		select {
		case <-shardTicker.C:
			rws.samplesIn.tick()
		case <-savepointC:
			rws.writeSavepoint()
		case <-rws.quit:
			return
		}
	}
}

// newSavepoint builds a Savepoint from current queue positions.
// Must be called with rws.mtx held.
func (rws *WriteStorage) newSavepoint() Savepoint {
	sp := make(Savepoint, len(rws.queues))
	for hash, q := range rws.queues {
		sp[hash] = SavepointEntry{
			Segment: q.watcher.LastProcessedSegment(),
		}
	}
	return sp
}

func (rws *WriteStorage) writeSavepoint() {
	rws.mtx.Lock()
	// Until the first ApplyConfig creates queues, an empty snapshot would wipe the loaded savepoint.
	if !rws.configApplied {
		rws.mtx.Unlock()
		return
	}
	// sp is marshaled outside the lock, so it must not be shared with rws.savepoint.
	// ApplyConfig refreshes rws.savepoint from the live queues itself.
	sp := rws.newSavepoint()
	rws.mtx.Unlock()

	err := sp.Write(rws.dir)
	if err != nil {
		rws.logger.Error("Failed to write remote write savepoint", "err", err)
	}
	rws.recordWriteResult(sp, err == nil)
}

// recordWriteResult updates the savepoint metrics after an attempt to Write.
// Precondition: rws.enableSavepoint is true, so the metric fields are non-nil.
func (rws *WriteStorage) recordWriteResult(sp Savepoint, success bool) {
	rws.savepointWritesTotal.Inc()
	rws.savepointEntries.Set(float64(len(sp)))
	if !success {
		rws.savepointWritesFailed.Inc()
		return
	}
	rws.savepointLastWriteTime.SetToCurrentTime()
}

func (rws *WriteStorage) Notify() {
	rws.mtx.Lock()
	defer rws.mtx.Unlock()

	for _, q := range rws.queues {
		// These should all be non blocking
		q.watcher.Notify()
	}
}

type applyConfigOpts struct {
	writeClientFactory func(name string, cfg *ClientConfig) (WriteClient, error)
}

// ApplyConfigOption modifies WriteStorage.ApplyConfig behavior.
type ApplyConfigOption func(*applyConfigOpts)

// WithWriteClientFactory sets a factory to provide a custom write client.
func WithWriteClientFactory(factory func(name string, cfg *ClientConfig) (WriteClient, error)) ApplyConfigOption {
	return func(opts *applyConfigOpts) {
		opts.writeClientFactory = factory
	}
}

// ApplyConfig updates the state as the new config requires.
// Only stop & create queues which have changes.
func (rws *WriteStorage) ApplyConfig(conf *config.Config, options ...ApplyConfigOption) error {
	rws.mtx.Lock()
	defer rws.mtx.Unlock()

	opts := applyConfigOpts{
		writeClientFactory: NewWriteClient,
	}
	for _, option := range options {
		option(&opts)
	}

	// Remote write queues only need to change if the remote write config or
	// external labels change.
	externalLabelUnchanged := labels.Equal(conf.GlobalConfig.ExternalLabels, rws.externalLabels)

	// Capture live positions before building replacement queues. A reload can recreate
	// queues, for example when external labels change, and replacements resume from
	// rws.savepoint. Refreshing it here avoids replaying from a periodically persisted
	// position that may be up to savepointWriteInterval stale.
	if rws.enableSavepoint {
		for hash, q := range rws.queues {
			rws.savepoint[hash] = SavepointEntry{Segment: q.watcher.LastProcessedSegment()}
		}
	}

	// Nothing in rws is modified until every config has been built, so an
	// error leaves the previous state intact.
	newQueues := make(map[string]*QueueManager)
	reusedClients := make(map[string]WriteClient)
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

		c, err := opts.writeClientFactory(name, &ClientConfig{
			URL:              rwConf.URL,
			WriteProtoMsg:    rwConf.ProtobufMessage,
			Timeout:          rwConf.RemoteTimeout,
			HTTPClientConfig: rwConf.HTTPClientConfig,
			SigV4Config:      rwConf.SigV4Config,
			AzureADConfig:    rwConf.AzureADConfig,
			GoogleIAMConfig:  rwConf.GoogleIAMConfig,
			Headers:          rwConf.Headers,
			RetryOnRateLimit: rwConf.QueueConfig.RetryOnRateLimit,
			RoundRobinDNS:    rwConf.RoundRobinDNS,
		})
		if err != nil {
			return err
		}

		queue, ok := rws.queues[hash]
		if externalLabelUnchanged && ok {
			newQueues[hash] = queue
			reusedClients[hash] = c
			continue
		}

		// Redacted to remove any passwords in the URL (that are
		// technically accepted but not recommended) since this is
		// only used for metric labels.
		endpoint := rwConf.URL.Redacted()

		startSegment := -1
		if rws.enableSavepoint {
			if entry, ok := rws.savepoint[hash]; ok {
				startSegment = entry.Segment
			}
		}

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
			rws.enableTypeAndUnitLabels,
			rwConf.ProtobufMessage,
			rws.recordBuf,
			rwConf.FailedRequestLogging,
			rwConf.ConvertNHCBToClassic,
			startSegment,
		)
		// Keep track of which queues are new so we know which to start.
		newHashes = append(newHashes, hash)
	}

	rws.externalLabels = conf.GlobalConfig.ExternalLabels

	for hash, c := range reusedClients {
		// Update the client in case any secret configuration has changed.
		newQueues[hash].SetClient(c)
		delete(rws.queues, hash)
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

	// Remove savepoint entries for queues that no longer exist.
	if rws.enableSavepoint {
		for hash := range rws.savepoint {
			if _, ok := newQueues[hash]; !ok {
				delete(rws.savepoint, hash)
			}
		}
	}
	rws.configApplied = true

	return nil
}

// Appender implements storage.Storage.
func (rws *WriteStorage) Appender(context.Context) storage.Appender {
	return &timestampTracker{
		baseTimestampTracker: baseTimestampTracker{
			writeStorage:         rws,
			highestRecvTimestamp: rws.highestTimestamp,
		},
	}
}

// AppenderV2 implements storage.Storage.
func (rws *WriteStorage) AppenderV2(context.Context) storage.AppenderV2 {
	return &timestampTrackerV2{
		baseTimestampTracker: baseTimestampTracker{
			writeStorage:         rws,
			highestRecvTimestamp: rws.highestTimestamp,
		},
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
	// Stop the run() goroutine and wait for it to exit before taking the lock.
	// run() may otherwise fire a concurrent writeSavepoint on its ticker, and
	// writeSavepoint itself acquires rws.mtx, so waiting for done while holding
	// the lock would deadlock.
	close(rws.quit)
	<-rws.done

	rws.mtx.Lock()
	defer rws.mtx.Unlock()

	// Write before stopping queues: a stopping queue rejects appends, and its
	// watcher keeps reading past the segments it drops. An empty snapshot is still
	// written so removed queues are cleared, but only once config was applied.
	if rws.enableSavepoint && rws.configApplied {
		sp := rws.newSavepoint()
		err := sp.Write(rws.dir)
		if err != nil {
			rws.logger.Error("Failed to write remote write savepoint", "err", err)
		}
		rws.recordWriteResult(sp, err == nil)
	}

	for _, q := range rws.queues {
		q.Stop()
	}

	rws.watcherMetrics.Unregister()
	rws.liveReaderMetrics.Unregister()

	if rws.reg != nil {
		rws.reg.Unregister(rws.highestTimestamp.Gauge)
		if rws.enableSavepoint {
			rws.reg.Unregister(rws.savepointWritesTotal)
			rws.reg.Unregister(rws.savepointWritesFailed)
			rws.reg.Unregister(rws.savepointLastWriteTime)
			rws.reg.Unregister(rws.savepointEntries)
		}
	}

	return nil
}

type baseTimestampTracker struct {
	writeStorage *WriteStorage

	samples              int64
	exemplars            int64
	histograms           int64
	highestTimestamp     int64
	highestRecvTimestamp *maxTimestamp
}

type timestampTracker struct {
	baseTimestampTracker

	appendOptions *storage.AppendOptions
}

func (t *timestampTracker) SetOptions(opts *storage.AppendOptions) {
	t.appendOptions = opts
}

// Append implements storage.Appender.
func (t *timestampTracker) Append(_ storage.SeriesRef, _ labels.Labels, ts int64, _ float64) (storage.SeriesRef, error) {
	t.samples++
	if ts > t.highestTimestamp {
		t.highestTimestamp = ts
	}
	return 0, nil
}

func (t *timestampTracker) AppendExemplar(storage.SeriesRef, labels.Labels, exemplar.Exemplar) (storage.SeriesRef, error) {
	// highestTimestamp excludes exemplars, as not all queues send them.
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

func (t *timestampTracker) AppendSTZeroSample(_ storage.SeriesRef, _ labels.Labels, _, st int64) (storage.SeriesRef, error) {
	t.samples++
	if st > t.highestTimestamp {
		// Theoretically, we should never see a ST zero sample with a timestamp higher than the highest timestamp we've seen so far.
		// However, we're not going to enforce that here, as it is not the responsibility of the tracker to enforce this.
		t.highestTimestamp = st
	}
	return 0, nil
}

func (t *timestampTracker) AppendHistogramSTZeroSample(_ storage.SeriesRef, _ labels.Labels, _, st int64, _ *histogram.Histogram, _ *histogram.FloatHistogram) (storage.SeriesRef, error) {
	t.histograms++
	if st > t.highestTimestamp {
		// Theoretically, we should never see a ST zero sample with a timestamp higher than the highest timestamp we've seen so far.
		// However, we're not going to enforce that here, as it is not the responsibility of the tracker to enforce this.
		t.highestTimestamp = st
	}
	return 0, nil
}

func (*timestampTracker) UpdateMetadata(storage.SeriesRef, labels.Labels, metadata.Metadata) (storage.SeriesRef, error) {
	// Intentionally a no-op. dataIn measures records that create work for the
	// sharded queue and is used to estimate queue load and backlog. A metadata
	// update only changes cached per-series state; it does not enqueue an item.
	return 0, nil
}

// Commit implements storage.Appender.
func (t *baseTimestampTracker) Commit() error {
	t.writeStorage.samplesIn.incr(t.samples + t.exemplars + t.histograms)

	samplesIn.Add(float64(t.samples))
	exemplarsIn.Add(float64(t.exemplars))
	histogramsIn.Add(float64(t.histograms))
	t.highestRecvTimestamp.Set(float64(t.highestTimestamp / 1000))
	return nil
}

// Rollback implements storage.Appender.
func (*baseTimestampTracker) Rollback() error {
	return nil
}

type timestampTrackerV2 struct {
	baseTimestampTracker
}

// Append implements storage.AppenderV2.
func (t *timestampTrackerV2) Append(ref storage.SeriesRef, _ labels.Labels, _, ts int64, _ float64, h *histogram.Histogram, fh *histogram.FloatHistogram, opts storage.AOptions) (storage.SeriesRef, error) {
	switch {
	case fh != nil, h != nil:
		t.histograms++
	default:
		t.samples++
	}
	if ts > t.highestTimestamp {
		t.highestTimestamp = ts
	}
	t.exemplars += int64(len(opts.Exemplars))
	return ref, nil
}

// AppendExemplars implements storage.ExemplarAppenderV2.
func (t *timestampTrackerV2) AppendExemplars(ref storage.SeriesRef, _ labels.Labels, exemplars []exemplar.Exemplar) (storage.SeriesRef, error) {
	// highestTimestamp excludes exemplars, as not all queues send them.
	t.exemplars += int64(len(exemplars))
	return ref, nil
}

var _ storage.ExemplarAppenderV2 = &timestampTrackerV2{}
