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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/wal"
	"go.uber.org/atomic"
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
)

// WriteStorage represents all the remote write storage.
type WriteStorage struct {
	logger log.Logger
	reg    prometheus.Registerer
	mtx    sync.Mutex

	watcherMetrics    *wal.WatcherMetrics
	liveReaderMetrics *wal.LiveReaderMetrics
	externalLabels    labels.Labels
	walDir            string
	queues            map[string]*QueueManager
	samplesIn         *ewmaRate
	flushDeadline     time.Duration
	interner          *pool
	scraper           ReadyScrapeManager

	// For timestampTracker.
	highestTimestamp *maxTimestamp

	checkpoint CheckpointRecord

	done chan struct{}
	// Soft shutdown context will prevent new enqueues and deadlocks.
	softShutdown chan struct{}

	// Hard shutdown context is used to terminate outgoing HTTP connections
	// after giving them a chance to terminate.
	hardShutdown          context.CancelFunc
	droppedOnHardShutdown atomic.Uint32
}

// NewWriteStorage creates and runs a WriteStorage.
func NewWriteStorage(logger log.Logger, reg prometheus.Registerer, walDir string, flushDeadline time.Duration, sm ReadyScrapeManager) *WriteStorage {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	rws := &WriteStorage{
		queues:            make(map[string]*QueueManager),
		watcherMetrics:    wal.NewWatcherMetrics(reg),
		liveReaderMetrics: wal.NewLiveReaderMetrics(reg),
		logger:            logger,
		reg:               reg,
		flushDeadline:     flushDeadline,
		samplesIn:         newEWMARate(ewmaWeight, shardUpdateDuration),
		walDir:            walDir,
		interner:          newPool(),
		scraper:           sm,
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

	var hardShutdownCtx context.Context
	hardShutdownCtx, rws.hardShutdown = context.WithCancel(context.Background())
	rws.softShutdown = make(chan struct{})
	rws.done = make(chan struct{})
	rws.droppedOnHardShutdown.Store(0)

	go rws.startTimedRecording(hardShutdownCtx)

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

		endpoint := rwConf.URL.String()
		newQueues[hash] = NewQueueManager(
			newQueueManagerMetrics(rws.reg, name, endpoint),
			rws.watcherMetrics,
			rws.liveReaderMetrics,
			rws.logger,
			rws.walDir,
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

// timedCheckpointRecord records the Checkpoint Record every set time
func (rws *WriteStorage) startTimedRecording(ctx context.Context) error {
	for {
		<-time.After(time.Duration(saveTime) * time.Millisecond)
		rws.writeCheckpointRecordJSON()

		select {
		case <-ctx.Done():
			return ctx.Err()
		}

	}
}

// writeCheckpointRecordJSON writes the segment record in a JSON file
func (rws *WriteStorage) writeCheckpointRecordJSON() {
	tempFilepath := "data/TempCheckpoint.json"
	filepath := "data/CheckpointRecord.json"

	rws.checkpoint.TimeRecorded = time.Now()
	rws.checkpoint.Checkpoints = rws.getEndpointRecordData()

	// Marshal json
	data, err := json.MarshalIndent(rws.checkpoint, "", "\t")
	if err != nil {
		level.Error(rws.logger).Log("error with Marshal: ", err)
	}

	err = ioutil.WriteFile(tempFilepath, data, 0600)
	if err != nil {
		level.Error(rws.logger).Log("Error with Writing to JSON: ", err)
	}

	// Sync temporary directory before rename.
	df, err := fileutil.OpenDir(tempFilepath)
	if err != nil {
		level.Error(rws.logger).Log("open temporary checkpoint directory", err)
	}
	if err := df.Sync(); err != nil {
		df.Close()
		level.Error(rws.logger).Log("Sync temporary checkpoint directory", err)
	}
	if err = df.Close(); err != nil {
		level.Error(rws.logger).Log("Close temporary checkpoint directory", err)
	}

	if err := fileutil.Replace(tempFilepath, filepath); err != nil {
		level.Error(rws.logger).Log("Rename checkpoint directory", err)
	}
}

// getEndpointRecordData gets the EndpointRecord from each queue_manager an, rwConf.URLns a array of EndpointRecords.
func (rws *WriteStorage) getEndpointRecordData() []EndpointRecord {
	record := []EndpointRecord{}

	for _, qMan := range rws.queues {
		qMan.UpdateEndpointRecord()

		tempRecord := EndpointRecord{
			Segment:  qMan.endpointRecord.Segment,
			Endpoint: qMan.endpointRecord.Endpoint,
		}
		record = append(record, tempRecord)
	}
	return record
}

// Close closes the WriteStorage.
func (rws *WriteStorage) Close() error {
	rws.mtx.Lock()
	defer rws.mtx.Unlock()
	for _, q := range rws.queues {
		q.Stop()
	}

	rws.writeCheckpointRecordJSON()

	return nil
}

type timestampTracker struct {
	writeStorage         *WriteStorage
	samples              int64
	exemplars            int64
	highestTimestamp     int64
	highestRecvTimestamp *maxTimestamp
}

// Append implements storage.Appender.
func (t *timestampTracker) Append(_ uint64, _ labels.Labels, ts int64, _ float64) (uint64, error) {
	t.samples++
	if ts > t.highestTimestamp {
		t.highestTimestamp = ts
	}
	return 0, nil
}

func (t *timestampTracker) AppendExemplar(_ uint64, _ labels.Labels, _ exemplar.Exemplar) (uint64, error) {
	t.exemplars++
	return 0, nil
}

// Commit implements storage.Appender.
func (t *timestampTracker) Commit() error {
	t.writeStorage.samplesIn.incr(t.samples + t.exemplars)

	samplesIn.Add(float64(t.samples))
	exemplarsIn.Add(float64(t.exemplars))
	t.highestRecvTimestamp.Set(float64(t.highestTimestamp / 1000))
	return nil
}

// Rollback implements storage.Appender.
func (*timestampTracker) Rollback() error {
	return nil
}
