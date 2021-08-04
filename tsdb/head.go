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

package tsdb

import (
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wal"
)

var (
	// ErrInvalidSample is returned if an appended sample is not valid and can't
	// be ingested.
	ErrInvalidSample = errors.New("invalid sample")
	// ErrInvalidExemplar is returned if an appended exemplar is not valid and can't
	// be ingested.
	ErrInvalidExemplar = errors.New("invalid exemplar")
	// ErrAppenderClosed is returned if an appender has already be successfully
	// rolled back or committed.
	ErrAppenderClosed = errors.New("appender closed")
)

// Head handles reads and writes of time series data within a time window.
type Head struct {
	chunkRange               atomic.Int64
	numSeries                atomic.Uint64
	minTime, maxTime         atomic.Int64 // Current min and max of the samples included in the head.
	minValidTime             atomic.Int64 // Mint allowed to be added to the head. It shouldn't be lower than the maxt of the last persisted block.
	lastWALTruncationTime    atomic.Int64
	lastMemoryTruncationTime atomic.Int64
	lastSeriesID             atomic.Uint64

	metrics         *headMetrics
	opts            *HeadOptions
	wal             *wal.WAL
	exemplarMetrics *ExemplarMetrics
	exemplars       ExemplarStorage
	logger          log.Logger
	appendPool      sync.Pool
	exemplarsPool   sync.Pool
	seriesPool      sync.Pool
	bytesPool       sync.Pool
	memChunkPool    sync.Pool

	// All series addressable by their ID or hash.
	series *stripeSeries

	symMtx  sync.RWMutex
	symbols map[string]struct{}

	deletedMtx sync.Mutex
	deleted    map[uint64]int // Deleted series, and what WAL segment they must be kept until.

	postings *index.MemPostings // Postings lists for terms.

	tombstones *tombstones.MemTombstones

	iso *isolation

	cardinalityMutex      sync.Mutex
	cardinalityCache      *index.PostingsStats // Posting stats cache which will expire after 30sec.
	lastPostingsStatsCall time.Duration        // Last posting stats call (PostingsCardinalityStats()) time for caching.

	// chunkDiskMapper is used to write and read Head chunks to/from disk.
	chunkDiskMapper *chunks.ChunkDiskMapper

	chunkSnapshotMtx sync.Mutex

	closedMtx sync.Mutex
	closed    bool

	stats *HeadStats
	reg   prometheus.Registerer

	memTruncationInProcess atomic.Bool
}

type ExemplarStorage interface {
	storage.ExemplarQueryable
	AddExemplar(labels.Labels, exemplar.Exemplar) error
	ValidateExemplar(labels.Labels, exemplar.Exemplar) error
}

// HeadOptions are parameters for the Head block.
type HeadOptions struct {
	ChunkRange int64
	// ChunkDirRoot is the parent directory of the chunks directory.
	ChunkDirRoot         string
	ChunkPool            chunkenc.Pool
	ChunkWriteBufferSize int
	// StripeSize sets the number of entries in the hash map, it must be a power of 2.
	// A larger StripeSize will allocate more memory up-front, but will increase performance when handling a large number of series.
	// A smaller StripeSize reduces the memory allocated, but can decrease performance with large number of series.
	StripeSize                     int
	SeriesCallback                 SeriesLifecycleCallback
	EnableExemplarStorage          bool
	EnableMemorySnapshotOnShutdown bool

	// Runtime reloadable options.
	MaxExemplars atomic.Int64
}

func DefaultHeadOptions() *HeadOptions {
	return &HeadOptions{
		ChunkRange:           DefaultBlockDuration,
		ChunkDirRoot:         "",
		ChunkPool:            chunkenc.NewPool(),
		ChunkWriteBufferSize: chunks.DefaultWriteBufferSize,
		StripeSize:           DefaultStripeSize,
		SeriesCallback:       &noopSeriesLifecycleCallback{},
	}
}

// SeriesLifecycleCallback specifies a list of callbacks that will be called during a lifecycle of a series.
// It is always a no-op in Prometheus and mainly meant for external users who import TSDB.
// All the callbacks should be safe to be called concurrently.
// It is up to the user to implement soft or hard consistency by making the callbacks
// atomic or non-atomic. Atomic callbacks can cause degradation performance.
type SeriesLifecycleCallback interface {
	// PreCreation is called before creating a series to indicate if the series can be created.
	// A non nil error means the series should not be created.
	PreCreation(labels.Labels) error
	// PostCreation is called after creating a series to indicate a creation of series.
	PostCreation(labels.Labels)
	// PostDeletion is called after deletion of series.
	PostDeletion(...labels.Labels)
}

// NewHead opens the head block in dir.
func NewHead(r prometheus.Registerer, l log.Logger, wal *wal.WAL, opts *HeadOptions, stats *HeadStats) (*Head, error) {
	var err error
	if l == nil {
		l = log.NewNopLogger()
	}
	if opts.ChunkRange < 1 {
		return nil, errors.Errorf("invalid chunk range %d", opts.ChunkRange)
	}
	if opts.SeriesCallback == nil {
		opts.SeriesCallback = &noopSeriesLifecycleCallback{}
	}

	em := NewExemplarMetrics(r)
	es, err := NewCircularExemplarStorage(opts.MaxExemplars.Load(), em)
	if err != nil {
		return nil, err
	}

	if stats == nil {
		stats = NewHeadStats()
	}

	h := &Head{
		wal:             wal,
		logger:          l,
		opts:            opts,
		exemplarMetrics: em,
		exemplars:       es,
		series:          newStripeSeries(opts.StripeSize, opts.SeriesCallback),
		symbols:         map[string]struct{}{},
		postings:        index.NewUnorderedMemPostings(),
		tombstones:      tombstones.NewMemTombstones(),
		iso:             newIsolation(),
		deleted:         map[uint64]int{},
		memChunkPool: sync.Pool{
			New: func() interface{} {
				return &memChunk{}
			},
		},
		stats: stats,
		reg:   r,
	}
	h.chunkRange.Store(opts.ChunkRange)
	h.minTime.Store(math.MaxInt64)
	h.maxTime.Store(math.MinInt64)
	h.lastWALTruncationTime.Store(math.MinInt64)
	h.lastMemoryTruncationTime.Store(math.MinInt64)
	h.metrics = newHeadMetrics(h, r)

	if opts.ChunkPool == nil {
		opts.ChunkPool = chunkenc.NewPool()
	}

	h.chunkDiskMapper, err = chunks.NewChunkDiskMapper(
		mmappedChunksDir(opts.ChunkDirRoot),
		opts.ChunkPool,
		opts.ChunkWriteBufferSize,
	)
	if err != nil {
		return nil, err
	}

	return h, nil
}

type headMetrics struct {
	activeAppenders          prometheus.Gauge
	series                   prometheus.GaugeFunc
	seriesCreated            prometheus.Counter
	seriesRemoved            prometheus.Counter
	seriesNotFound           prometheus.Counter
	chunks                   prometheus.Gauge
	chunksCreated            prometheus.Counter
	chunksRemoved            prometheus.Counter
	gcDuration               prometheus.Summary
	samplesAppended          prometheus.Counter
	outOfBoundSamples        prometheus.Counter
	outOfOrderSamples        prometheus.Counter
	walTruncateDuration      prometheus.Summary
	walCorruptionsTotal      prometheus.Counter
	walTotalReplayDuration   prometheus.Gauge
	headTruncateFail         prometheus.Counter
	headTruncateTotal        prometheus.Counter
	checkpointDeleteFail     prometheus.Counter
	checkpointDeleteTotal    prometheus.Counter
	checkpointCreationFail   prometheus.Counter
	checkpointCreationTotal  prometheus.Counter
	mmapChunkCorruptionTotal prometheus.Counter
}

func newHeadMetrics(h *Head, r prometheus.Registerer) *headMetrics {
	m := &headMetrics{
		activeAppenders: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_active_appenders",
			Help: "Number of currently active appender transactions",
		}),
		series: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_series",
			Help: "Total number of series in the head block.",
		}, func() float64 {
			return float64(h.NumSeries())
		}),
		seriesCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_series_created_total",
			Help: "Total number of series created in the head",
		}),
		seriesRemoved: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_series_removed_total",
			Help: "Total number of series removed in the head",
		}),
		seriesNotFound: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_series_not_found_total",
			Help: "Total number of requests for series that were not found.",
		}),
		chunks: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_chunks",
			Help: "Total number of chunks in the head block.",
		}),
		chunksCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_chunks_created_total",
			Help: "Total number of chunks created in the head",
		}),
		chunksRemoved: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_chunks_removed_total",
			Help: "Total number of chunks removed in the head",
		}),
		gcDuration: prometheus.NewSummary(prometheus.SummaryOpts{
			Name: "prometheus_tsdb_head_gc_duration_seconds",
			Help: "Runtime of garbage collection in the head block.",
		}),
		walTruncateDuration: prometheus.NewSummary(prometheus.SummaryOpts{
			Name: "prometheus_tsdb_wal_truncate_duration_seconds",
			Help: "Duration of WAL truncation.",
		}),
		walCorruptionsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_corruptions_total",
			Help: "Total number of WAL corruptions.",
		}),
		walTotalReplayDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_data_replay_duration_seconds",
			Help: "Time taken to replay the data on disk.",
		}),
		samplesAppended: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_samples_appended_total",
			Help: "Total number of appended samples.",
		}),
		outOfBoundSamples: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_bound_samples_total",
			Help: "Total number of out of bound samples ingestion failed attempts.",
		}),
		outOfOrderSamples: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_samples_total",
			Help: "Total number of out of order samples ingestion failed attempts.",
		}),
		headTruncateFail: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_truncations_failed_total",
			Help: "Total number of head truncations that failed.",
		}),
		headTruncateTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_truncations_total",
			Help: "Total number of head truncations attempted.",
		}),
		checkpointDeleteFail: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_checkpoint_deletions_failed_total",
			Help: "Total number of checkpoint deletions that failed.",
		}),
		checkpointDeleteTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_checkpoint_deletions_total",
			Help: "Total number of checkpoint deletions attempted.",
		}),
		checkpointCreationFail: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_checkpoint_creations_failed_total",
			Help: "Total number of checkpoint creations that failed.",
		}),
		checkpointCreationTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_checkpoint_creations_total",
			Help: "Total number of checkpoint creations attempted.",
		}),
		mmapChunkCorruptionTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_mmap_chunk_corruptions_total",
			Help: "Total number of memory-mapped chunk corruptions.",
		}),
	}

	if r != nil {
		r.MustRegister(
			m.activeAppenders,
			m.series,
			m.chunks,
			m.chunksCreated,
			m.chunksRemoved,
			m.seriesCreated,
			m.seriesRemoved,
			m.seriesNotFound,
			m.gcDuration,
			m.walTruncateDuration,
			m.walCorruptionsTotal,
			m.walTotalReplayDuration,
			m.samplesAppended,
			m.outOfBoundSamples,
			m.outOfOrderSamples,
			m.headTruncateFail,
			m.headTruncateTotal,
			m.checkpointDeleteFail,
			m.checkpointDeleteTotal,
			m.checkpointCreationFail,
			m.checkpointCreationTotal,
			m.mmapChunkCorruptionTotal,
			// Metrics bound to functions and not needed in tests
			// can be created and registered on the spot.
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{
				Name: "prometheus_tsdb_head_max_time",
				Help: "Maximum timestamp of the head block. The unit is decided by the library consumer.",
			}, func() float64 {
				return float64(h.MaxTime())
			}),
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{
				Name: "prometheus_tsdb_head_min_time",
				Help: "Minimum time bound of the head block. The unit is decided by the library consumer.",
			}, func() float64 {
				return float64(h.MinTime())
			}),
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{
				Name: "prometheus_tsdb_isolation_low_watermark",
				Help: "The lowest TSDB append ID that is still referenced.",
			}, func() float64 {
				return float64(h.iso.lowWatermark())
			}),
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{
				Name: "prometheus_tsdb_isolation_high_watermark",
				Help: "The highest TSDB append ID that has been given out.",
			}, func() float64 {
				return float64(h.iso.lastAppendID())
			}),
		)
	}
	return m
}

func mmappedChunksDir(dir string) string { return filepath.Join(dir, "chunks_head") }

// HeadStats are the statistics for the head component of the DB.
type HeadStats struct {
	WALReplayStatus *WALReplayStatus
}

// NewHeadStats returns a new HeadStats object.
func NewHeadStats() *HeadStats {
	return &HeadStats{
		WALReplayStatus: &WALReplayStatus{},
	}
}

// WALReplayStatus contains status information about the WAL replay.
type WALReplayStatus struct {
	sync.RWMutex
	Min     int
	Max     int
	Current int
}

// GetWALReplayStatus returns the WAL replay status information.
func (s *WALReplayStatus) GetWALReplayStatus() WALReplayStatus {
	s.RLock()
	defer s.RUnlock()

	return WALReplayStatus{
		Min:     s.Min,
		Max:     s.Max,
		Current: s.Current,
	}
}

const cardinalityCacheExpirationTime = time.Duration(30) * time.Second

// Init loads data from the write ahead log and prepares the head for writes.
// It should be called before using an appender so that it
// limits the ingested samples to the head min valid time.
func (h *Head) Init(minValidTime int64) error {
	h.minValidTime.Store(minValidTime)
	defer h.postings.EnsureOrder()
	defer h.gc() // After loading the wal remove the obsolete data from the head.
	defer func() {
		// Loading of m-mapped chunks and snapshot can make the mint of the Head
		// to go below minValidTime.
		if h.MinTime() < h.minValidTime.Load() {
			h.minTime.Store(h.minValidTime.Load())
		}
	}()

	level.Info(h.logger).Log("msg", "Replaying on-disk memory mappable chunks if any")
	start := time.Now()

	snapIdx, snapOffset, refSeries, err := h.loadChunkSnapshot()
	if err != nil {
		return err
	}
	level.Info(h.logger).Log("msg", "Chunk snapshot loading time", "duration", time.Since(start).String())

	mmapChunkReplayStart := time.Now()
	mmappedChunks, err := h.loadMmappedChunks(refSeries)
	if err != nil {
		level.Error(h.logger).Log("msg", "Loading on-disk chunks failed", "err", err)
		if _, ok := errors.Cause(err).(*chunks.CorruptionErr); ok {
			h.metrics.mmapChunkCorruptionTotal.Inc()
		}
		// If this fails, data will be recovered from WAL.
		// Hence we wont lose any data (given WAL is not corrupt).
		mmappedChunks = h.removeCorruptedMmappedChunks(err, refSeries)
	}

	level.Info(h.logger).Log("msg", "On-disk memory mappable chunks replay completed", "duration", time.Since(mmapChunkReplayStart).String())
	if h.wal == nil {
		level.Info(h.logger).Log("msg", "WAL not found")
		return nil
	}

	level.Info(h.logger).Log("msg", "Replaying WAL, this may take a while")

	checkpointReplayStart := time.Now()
	// Backfill the checkpoint first if it exists.
	dir, startFrom, err := wal.LastCheckpoint(h.wal.Dir())
	if err != nil && err != record.ErrNotFound {
		return errors.Wrap(err, "find last checkpoint")
	}

	// Find the last segment.
	_, endAt, e := wal.Segments(h.wal.Dir())
	if e != nil {
		return errors.Wrap(e, "finding WAL segments")
	}

	h.startWALReplayStatus(startFrom, endAt)

	multiRef := map[uint64]uint64{}
	if err == nil {
		sr, err := wal.NewSegmentsReader(dir)
		if err != nil {
			return errors.Wrap(err, "open checkpoint")
		}
		defer func() {
			if err := sr.Close(); err != nil {
				level.Warn(h.logger).Log("msg", "Error while closing the wal segments reader", "err", err)
			}
		}()

		// A corrupted checkpoint is a hard error for now and requires user
		// intervention. There's likely little data that can be recovered anyway.
		if err := h.loadWAL(wal.NewReader(sr), multiRef, mmappedChunks); err != nil {
			return errors.Wrap(err, "backfill checkpoint")
		}
		h.updateWALReplayStatusRead(startFrom)
		startFrom++
		level.Info(h.logger).Log("msg", "WAL checkpoint loaded")
	}
	checkpointReplayDuration := time.Since(checkpointReplayStart)

	walReplayStart := time.Now()

	if snapIdx > startFrom {
		startFrom = snapIdx
	}
	// Backfill segments from the most recent checkpoint onwards.
	for i := startFrom; i <= endAt; i++ {
		s, err := wal.OpenReadSegment(wal.SegmentName(h.wal.Dir(), i))
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("open WAL segment: %d", i))
		}

		offset := 0
		if i == snapIdx {
			offset = snapOffset
		}
		sr, err := wal.NewSegmentBufReaderWithOffset(offset, s)
		if err != nil {
			return errors.Wrapf(err, "segment reader (offset=%d)", offset)
		}
		err = h.loadWAL(wal.NewReader(sr), multiRef, mmappedChunks)
		if err := sr.Close(); err != nil {
			level.Warn(h.logger).Log("msg", "Error while closing the wal segments reader", "err", err)
		}
		if err != nil {
			return err
		}
		level.Info(h.logger).Log("msg", "WAL segment loaded", "segment", i, "maxSegment", endAt)
		h.updateWALReplayStatusRead(i)
	}

	walReplayDuration := time.Since(start)
	h.metrics.walTotalReplayDuration.Set(walReplayDuration.Seconds())
	level.Info(h.logger).Log(
		"msg", "WAL replay completed",
		"checkpoint_replay_duration", checkpointReplayDuration.String(),
		"wal_replay_duration", time.Since(walReplayStart).String(),
		"total_replay_duration", walReplayDuration.String(),
	)

	return nil
}

func (h *Head) loadMmappedChunks(refSeries map[uint64]*memSeries) (map[uint64][]*mmappedChunk, error) {
	mmappedChunks := map[uint64][]*mmappedChunk{}
	if err := h.chunkDiskMapper.IterateAllChunks(func(seriesRef, chunkRef uint64, mint, maxt int64, numSamples uint16) error {
		if maxt < h.minValidTime.Load() {
			return nil
		}
		ms, ok := refSeries[seriesRef]
		if !ok {
			slice := mmappedChunks[seriesRef]
			if len(slice) > 0 && slice[len(slice)-1].maxTime >= mint {
				return errors.Errorf("out of sequence m-mapped chunk for series ref %d", seriesRef)
			}

			slice = append(slice, &mmappedChunk{
				ref:        chunkRef,
				minTime:    mint,
				maxTime:    maxt,
				numSamples: numSamples,
			})
			mmappedChunks[seriesRef] = slice
			return nil
		}

		if len(ms.mmappedChunks) > 0 && ms.mmappedChunks[len(ms.mmappedChunks)-1].maxTime >= mint {
			return errors.Errorf("out of sequence m-mapped chunk for series ref %d", seriesRef)
		}

		h.metrics.chunks.Inc()
		h.metrics.chunksCreated.Inc()
		ms.mmappedChunks = append(ms.mmappedChunks, &mmappedChunk{
			ref:        chunkRef,
			minTime:    mint,
			maxTime:    maxt,
			numSamples: numSamples,
		})
		h.updateMinMaxTime(mint, maxt)
		if ms.headChunk != nil && maxt >= ms.headChunk.minTime {
			// The head chunk was completed and was m-mapped after taking the snapshot.
			// Hence remove this chunk.
			ms.nextAt = 0
			ms.headChunk = nil
			ms.app = nil
		}
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "iterate on on-disk chunks")
	}
	return mmappedChunks, nil
}

// removeCorruptedMmappedChunks attempts to delete the corrupted mmapped chunks and if it fails, it clears all the previously
// loaded mmapped chunks.
func (h *Head) removeCorruptedMmappedChunks(err error, refSeries map[uint64]*memSeries) map[uint64][]*mmappedChunk {
	level.Info(h.logger).Log("msg", "Deleting mmapped chunk files")

	if err := h.chunkDiskMapper.DeleteCorrupted(err); err != nil {
		level.Info(h.logger).Log("msg", "Deletion of mmap chunk files failed, discarding chunk files completely", "err", err)
		return map[uint64][]*mmappedChunk{}
	}

	level.Info(h.logger).Log("msg", "Deletion of mmap chunk files successful, reattempting m-mapping the on-disk chunks")
	mmappedChunks, err := h.loadMmappedChunks(refSeries)
	if err != nil {
		level.Error(h.logger).Log("msg", "Loading on-disk chunks failed, discarding chunk files completely", "err", err)
		mmappedChunks = map[uint64][]*mmappedChunk{}
	}

	return mmappedChunks
}

func (h *Head) ApplyConfig(cfg *config.Config) error {
	if !h.opts.EnableExemplarStorage {
		return nil
	}

	// Head uses opts.MaxExemplars in combination with opts.EnableExemplarStorage
	// to decide if it should pass exemplars along to it's exemplar storage, so we
	// need to update opts.MaxExemplars here.
	prevSize := h.opts.MaxExemplars.Load()
	h.opts.MaxExemplars.Store(cfg.StorageConfig.ExemplarsConfig.MaxExemplars)

	if prevSize == h.opts.MaxExemplars.Load() {
		return nil
	}

	migrated := h.exemplars.(*CircularExemplarStorage).Resize(h.opts.MaxExemplars.Load())
	level.Info(h.logger).Log("msg", "Exemplar storage resized", "from", prevSize, "to", h.opts.MaxExemplars, "migrated", migrated)
	return nil
}

// PostingsCardinalityStats returns top 10 highest cardinality stats By label and value names.
func (h *Head) PostingsCardinalityStats(statsByLabelName string) *index.PostingsStats {
	h.cardinalityMutex.Lock()
	defer h.cardinalityMutex.Unlock()
	currentTime := time.Duration(time.Now().Unix()) * time.Second
	seconds := currentTime - h.lastPostingsStatsCall
	if seconds > cardinalityCacheExpirationTime {
		h.cardinalityCache = nil
	}
	if h.cardinalityCache != nil {
		return h.cardinalityCache
	}
	h.cardinalityCache = h.postings.Stats(statsByLabelName)
	h.lastPostingsStatsCall = time.Duration(time.Now().Unix()) * time.Second

	return h.cardinalityCache
}

func (h *Head) updateMinMaxTime(mint, maxt int64) {
	for {
		lt := h.MinTime()
		if mint >= lt {
			break
		}
		if h.minTime.CAS(lt, mint) {
			break
		}
	}
	for {
		ht := h.MaxTime()
		if maxt <= ht {
			break
		}
		if h.maxTime.CAS(ht, maxt) {
			break
		}
	}
}

// SetMinValidTime sets the minimum timestamp the head can ingest.
func (h *Head) SetMinValidTime(minValidTime int64) {
	h.minValidTime.Store(minValidTime)
}

// Truncate removes old data before mint from the head and WAL.
func (h *Head) Truncate(mint int64) (err error) {
	initialize := h.MinTime() == math.MaxInt64
	if err := h.truncateMemory(mint); err != nil {
		return err
	}
	if initialize {
		return nil
	}
	return h.truncateWAL(mint)
}

// truncateMemory removes old data before mint from the head.
func (h *Head) truncateMemory(mint int64) (err error) {
	h.chunkSnapshotMtx.Lock()
	defer h.chunkSnapshotMtx.Unlock()

	defer func() {
		if err != nil {
			h.metrics.headTruncateFail.Inc()
		}
	}()

	initialize := h.MinTime() == math.MaxInt64

	if h.MinTime() >= mint && !initialize {
		return nil
	}

	// The order of these two Store() should not be changed,
	// i.e. truncation time is set before in-process boolean.
	h.lastMemoryTruncationTime.Store(mint)
	h.memTruncationInProcess.Store(true)
	defer h.memTruncationInProcess.Store(false)

	// We wait for pending queries to end that overlap with this truncation.
	if !initialize {
		h.WaitForPendingReadersInTimeRange(h.MinTime(), mint)
	}

	h.minTime.Store(mint)
	h.minValidTime.Store(mint)

	// Ensure that max time is at least as high as min time.
	for h.MaxTime() < mint {
		h.maxTime.CAS(h.MaxTime(), mint)
	}

	// This was an initial call to Truncate after loading blocks on startup.
	// We haven't read back the WAL yet, so do not attempt to truncate it.
	if initialize {
		return nil
	}

	h.metrics.headTruncateTotal.Inc()
	start := time.Now()

	actualMint := h.gc()
	level.Info(h.logger).Log("msg", "Head GC completed", "duration", time.Since(start))
	h.metrics.gcDuration.Observe(time.Since(start).Seconds())
	if actualMint > h.minTime.Load() {
		// The actual mint of the Head is higher than the one asked to truncate.
		appendableMinValidTime := h.appendableMinValidTime()
		if actualMint < appendableMinValidTime {
			h.minTime.Store(actualMint)
			h.minValidTime.Store(actualMint)
		} else {
			// The actual min time is in the appendable window.
			// So we set the mint to the appendableMinValidTime.
			h.minTime.Store(appendableMinValidTime)
			h.minValidTime.Store(appendableMinValidTime)
		}
	}

	// Truncate the chunk m-mapper.
	if err := h.chunkDiskMapper.Truncate(mint); err != nil {
		return errors.Wrap(err, "truncate chunks.HeadReadWriter")
	}
	return nil
}

// WaitForPendingReadersInTimeRange waits for queries overlapping with given range to finish querying.
// The query timeout limits the max wait time of this function implicitly.
// The mint is inclusive and maxt is the truncation time hence exclusive.
func (h *Head) WaitForPendingReadersInTimeRange(mint, maxt int64) {
	maxt-- // Making it inclusive before checking overlaps.
	overlaps := func() bool {
		o := false
		h.iso.TraverseOpenReads(func(s *isolationState) bool {
			if s.mint <= maxt && mint <= s.maxt {
				// Overlaps with the truncation range.
				o = true
				return false
			}
			return true
		})
		return o
	}
	for overlaps() {
		time.Sleep(500 * time.Millisecond)
	}
}

// IsQuerierCollidingWithTruncation returns if the current querier needs to be closed and if a new querier
// has to be created. In the latter case, the method also returns the new mint to be used for creating the
// new range head and the new querier. This methods helps preventing races with the truncation of in-memory data.
//
// NOTE: The querier should already be taken before calling this.
func (h *Head) IsQuerierCollidingWithTruncation(querierMint, querierMaxt int64) (shouldClose bool, getNew bool, newMint int64) {
	if !h.memTruncationInProcess.Load() {
		return false, false, 0
	}
	// Head truncation is in process. It also means that the block that was
	// created for this truncation range is also available.
	// Check if we took a querier that overlaps with this truncation.
	memTruncTime := h.lastMemoryTruncationTime.Load()
	if querierMaxt < memTruncTime {
		// Head compaction has happened and this time range is being truncated.
		// This query doesn't overlap with the Head any longer.
		// We should close this querier to avoid races and the data would be
		// available with the blocks below.
		// Cases:
		// 1.     |------truncation------|
		//   |---query---|
		// 2.     |------truncation------|
		//              |---query---|
		return true, false, 0
	}
	if querierMint < memTruncTime {
		// The truncation time is not same as head mint that we saw above but the
		// query still overlaps with the Head.
		// The truncation started after we got the querier. So it is not safe
		// to use this querier and/or might block truncation. We should get
		// a new querier for the new Head range while remaining will be available
		// in the blocks below.
		// Case:
		//      |------truncation------|
		//                        |----query----|
		// Turns into
		//      |------truncation------|
		//                             |---qu---|
		return true, true, memTruncTime
	}

	// Other case is this, which is a no-op
	//      |------truncation------|
	//                              |---query---|
	return false, false, 0
}

// truncateWAL removes old data before mint from the WAL.
func (h *Head) truncateWAL(mint int64) error {
	h.chunkSnapshotMtx.Lock()
	defer h.chunkSnapshotMtx.Unlock()

	if h.wal == nil || mint <= h.lastWALTruncationTime.Load() {
		return nil
	}
	start := time.Now()
	h.lastWALTruncationTime.Store(mint)

	first, last, err := wal.Segments(h.wal.Dir())
	if err != nil {
		return errors.Wrap(err, "get segment range")
	}
	// Start a new segment, so low ingestion volume TSDB don't have more WAL than
	// needed.
	if err := h.wal.NextSegment(); err != nil {
		return errors.Wrap(err, "next segment")
	}
	last-- // Never consider last segment for checkpoint.
	if last < 0 {
		return nil // no segments yet.
	}
	// The lower two thirds of segments should contain mostly obsolete samples.
	// If we have less than two segments, it's not worth checkpointing yet.
	// With the default 2h blocks, this will keeping up to around 3h worth
	// of WAL segments.
	last = first + (last-first)*2/3
	if last <= first {
		return nil
	}

	keep := func(id uint64) bool {
		if h.series.getByID(id) != nil {
			return true
		}
		h.deletedMtx.Lock()
		_, ok := h.deleted[id]
		h.deletedMtx.Unlock()
		return ok
	}
	h.metrics.checkpointCreationTotal.Inc()
	if _, err = wal.Checkpoint(h.logger, h.wal, first, last, keep, mint); err != nil {
		h.metrics.checkpointCreationFail.Inc()
		if _, ok := errors.Cause(err).(*wal.CorruptionErr); ok {
			h.metrics.walCorruptionsTotal.Inc()
		}
		return errors.Wrap(err, "create checkpoint")
	}
	if err := h.wal.Truncate(last + 1); err != nil {
		// If truncating fails, we'll just try again at the next checkpoint.
		// Leftover segments will just be ignored in the future if there's a checkpoint
		// that supersedes them.
		level.Error(h.logger).Log("msg", "truncating segments failed", "err", err)
	}

	// The checkpoint is written and segments before it is truncated, so we no
	// longer need to track deleted series that are before it.
	h.deletedMtx.Lock()
	for ref, segment := range h.deleted {
		if segment < first {
			delete(h.deleted, ref)
		}
	}
	h.deletedMtx.Unlock()

	h.metrics.checkpointDeleteTotal.Inc()
	if err := wal.DeleteCheckpoints(h.wal.Dir(), last); err != nil {
		// Leftover old checkpoints do not cause problems down the line beyond
		// occupying disk space.
		// They will just be ignored since a higher checkpoint exists.
		level.Error(h.logger).Log("msg", "delete old checkpoints", "err", err)
		h.metrics.checkpointDeleteFail.Inc()
	}
	h.metrics.walTruncateDuration.Observe(time.Since(start).Seconds())

	level.Info(h.logger).Log("msg", "WAL checkpoint complete",
		"first", first, "last", last, "duration", time.Since(start))

	return nil
}

type Stats struct {
	NumSeries         uint64
	MinTime, MaxTime  int64
	IndexPostingStats *index.PostingsStats
}

// Stats returns important current HEAD statistics. Note that it is expensive to
// calculate these.
func (h *Head) Stats(statsByLabelName string) *Stats {
	return &Stats{
		NumSeries:         h.NumSeries(),
		MaxTime:           h.MaxTime(),
		MinTime:           h.MinTime(),
		IndexPostingStats: h.PostingsCardinalityStats(statsByLabelName),
	}
}

type RangeHead struct {
	head       *Head
	mint, maxt int64
}

// NewRangeHead returns a *RangeHead.
func NewRangeHead(head *Head, mint, maxt int64) *RangeHead {
	return &RangeHead{
		head: head,
		mint: mint,
		maxt: maxt,
	}
}

func (h *RangeHead) Index() (IndexReader, error) {
	return h.head.indexRange(h.mint, h.maxt), nil
}

func (h *RangeHead) Chunks() (ChunkReader, error) {
	return h.head.chunksRange(h.mint, h.maxt, h.head.iso.State(h.mint, h.maxt))
}

func (h *RangeHead) Tombstones() (tombstones.Reader, error) {
	return h.head.tombstones, nil
}

func (h *RangeHead) MinTime() int64 {
	return h.mint
}

// MaxTime returns the max time of actual data fetch-able from the head.
// This controls the chunks time range which is closed [b.MinTime, b.MaxTime].
func (h *RangeHead) MaxTime() int64 {
	return h.maxt
}

// BlockMaxTime returns the max time of the potential block created from this head.
// It's different to MaxTime as we need to add +1 millisecond to block maxt because block
// intervals are half-open: [b.MinTime, b.MaxTime). Block intervals are always +1 than the total samples it includes.
func (h *RangeHead) BlockMaxTime() int64 {
	return h.MaxTime() + 1
}

func (h *RangeHead) NumSeries() uint64 {
	return h.head.NumSeries()
}

func (h *RangeHead) Meta() BlockMeta {
	return BlockMeta{
		MinTime: h.MinTime(),
		MaxTime: h.MaxTime(),
		ULID:    h.head.Meta().ULID,
		Stats: BlockStats{
			NumSeries: h.NumSeries(),
		},
	}
}

// String returns an human readable representation of the range head. It's important to
// keep this function in order to avoid the struct dump when the head is stringified in
// errors or logs.
func (h *RangeHead) String() string {
	return fmt.Sprintf("range head (mint: %d, maxt: %d)", h.MinTime(), h.MaxTime())
}

// Delete all samples in the range of [mint, maxt] for series that satisfy the given
// label matchers.
func (h *Head) Delete(mint, maxt int64, ms ...*labels.Matcher) error {
	// Do not delete anything beyond the currently valid range.
	mint, maxt = clampInterval(mint, maxt, h.MinTime(), h.MaxTime())

	ir := h.indexRange(mint, maxt)

	p, err := PostingsForMatchers(ir, ms...)
	if err != nil {
		return errors.Wrap(err, "select series")
	}

	var stones []tombstones.Stone
	for p.Next() {
		series := h.series.getByID(p.At())

		series.RLock()
		t0, t1 := series.minTime(), series.maxTime()
		series.RUnlock()
		if t0 == math.MinInt64 || t1 == math.MinInt64 {
			continue
		}
		// Delete only until the current values and not beyond.
		t0, t1 = clampInterval(mint, maxt, t0, t1)
		stones = append(stones, tombstones.Stone{Ref: p.At(), Intervals: tombstones.Intervals{{Mint: t0, Maxt: t1}}})
	}
	if p.Err() != nil {
		return p.Err()
	}
	if h.wal != nil {
		var enc record.Encoder
		if err := h.wal.Log(enc.Tombstones(stones, nil)); err != nil {
			return err
		}
	}
	for _, s := range stones {
		h.tombstones.AddInterval(s.Ref, s.Intervals[0])
	}

	return nil
}

// gc removes data before the minimum timestamp from the head.
// It returns the actual min times of the chunks present in the Head.
func (h *Head) gc() int64 {
	// Only data strictly lower than this timestamp must be deleted.
	mint := h.MinTime()

	// Drop old chunks and remember series IDs and hashes if they can be
	// deleted entirely.
	deleted, chunksRemoved, actualMint := h.series.gc(mint)
	seriesRemoved := len(deleted)

	h.metrics.seriesRemoved.Add(float64(seriesRemoved))
	h.metrics.chunksRemoved.Add(float64(chunksRemoved))
	h.metrics.chunks.Sub(float64(chunksRemoved))
	h.numSeries.Sub(uint64(seriesRemoved))

	// Remove deleted series IDs from the postings lists.
	h.postings.Delete(deleted)

	if h.wal != nil {
		_, last, _ := wal.Segments(h.wal.Dir())
		h.deletedMtx.Lock()
		// Keep series records until we're past segment 'last'
		// because the WAL will still have samples records with
		// this ref ID. If we didn't keep these series records then
		// on start up when we replay the WAL, or any other code
		// that reads the WAL, wouldn't be able to use those
		// samples since we would have no labels for that ref ID.
		for ref := range deleted {
			h.deleted[ref] = last
		}
		h.deletedMtx.Unlock()
	}

	// Rebuild symbols and label value indices from what is left in the postings terms.
	// symMtx ensures that append of symbols and postings is disabled for rebuild time.
	h.symMtx.Lock()
	defer h.symMtx.Unlock()

	symbols := make(map[string]struct{}, len(h.symbols))
	if err := h.postings.Iter(func(l labels.Label, _ index.Postings) error {
		symbols[l.Name] = struct{}{}
		symbols[l.Value] = struct{}{}
		return nil
	}); err != nil {
		// This should never happen, as the iteration function only returns nil.
		panic(err)
	}
	h.symbols = symbols

	return actualMint
}

// Tombstones returns a new reader over the head's tombstones
func (h *Head) Tombstones() (tombstones.Reader, error) {
	return h.tombstones, nil
}

// NumSeries returns the number of active series in the head.
func (h *Head) NumSeries() uint64 {
	return h.numSeries.Load()
}

// Meta returns meta information about the head.
// The head is dynamic so will return dynamic results.
func (h *Head) Meta() BlockMeta {
	var id [16]byte
	copy(id[:], "______head______")
	return BlockMeta{
		MinTime: h.MinTime(),
		MaxTime: h.MaxTime(),
		ULID:    ulid.ULID(id),
		Stats: BlockStats{
			NumSeries: h.NumSeries(),
		},
	}
}

// MinTime returns the lowest time bound on visible data in the head.
func (h *Head) MinTime() int64 {
	return h.minTime.Load()
}

// MaxTime returns the highest timestamp seen in data of the head.
func (h *Head) MaxTime() int64 {
	return h.maxTime.Load()
}

// compactable returns whether the head has a compactable range.
// The head has a compactable range when the head time range is 1.5 times the chunk range.
// The 0.5 acts as a buffer of the appendable window.
func (h *Head) compactable() bool {
	return h.MaxTime()-h.MinTime() > h.chunkRange.Load()/2*3
}

// Close flushes the WAL and closes the head.
// It also takes a snapshot of in-memory chunks if enabled.
func (h *Head) Close() error {
	h.closedMtx.Lock()
	defer h.closedMtx.Unlock()
	h.closed = true
	errs := tsdb_errors.NewMulti(h.chunkDiskMapper.Close())
	if errs.Err() == nil && h.opts.EnableMemorySnapshotOnShutdown {
		errs.Add(h.performChunkSnapshot())
	}
	if h.wal != nil {
		errs.Add(h.wal.Close())
	}
	return errs.Err()

}

// String returns an human readable representation of the TSDB head. It's important to
// keep this function in order to avoid the struct dump when the head is stringified in
// errors or logs.
func (h *Head) String() string {
	return "head"
}

func (h *Head) getOrCreate(hash uint64, lset labels.Labels) (*memSeries, bool, error) {
	// Just using `getOrCreateWithID` below would be semantically sufficient, but we'd create
	// a new series on every sample inserted via Add(), which causes allocations
	// and makes our series IDs rather random and harder to compress in postings.
	s := h.series.getByHash(hash, lset)
	if s != nil {
		return s, false, nil
	}

	// Optimistically assume that we are the first one to create the series.
	id := h.lastSeriesID.Inc()

	return h.getOrCreateWithID(id, hash, lset)
}

func (h *Head) getOrCreateWithID(id, hash uint64, lset labels.Labels) (*memSeries, bool, error) {
	s, created, err := h.series.getOrSet(hash, lset, func() *memSeries {
		return newMemSeries(lset, id, h.chunkRange.Load(), &h.memChunkPool)
	})
	if err != nil {
		return nil, false, err
	}
	if !created {
		return s, false, nil
	}

	h.metrics.seriesCreated.Inc()
	h.numSeries.Inc()

	h.symMtx.Lock()
	defer h.symMtx.Unlock()

	for _, l := range lset {
		h.symbols[l.Name] = struct{}{}
		h.symbols[l.Value] = struct{}{}
	}

	h.postings.Add(id, lset)
	return s, true, nil
}

// seriesHashmap is a simple hashmap for memSeries by their label set. It is built
// on top of a regular hashmap and holds a slice of series to resolve hash collisions.
// Its methods require the hash to be submitted with it to avoid re-computations throughout
// the code.
type seriesHashmap map[uint64][]*memSeries

func (m seriesHashmap) get(hash uint64, lset labels.Labels) *memSeries {
	for _, s := range m[hash] {
		if labels.Equal(s.lset, lset) {
			return s
		}
	}
	return nil
}

func (m seriesHashmap) set(hash uint64, s *memSeries) {
	l := m[hash]
	for i, prev := range l {
		if labels.Equal(prev.lset, s.lset) {
			l[i] = s
			return
		}
	}
	m[hash] = append(l, s)
}

func (m seriesHashmap) del(hash uint64, lset labels.Labels) {
	var rem []*memSeries
	for _, s := range m[hash] {
		if !labels.Equal(s.lset, lset) {
			rem = append(rem, s)
		}
	}
	if len(rem) == 0 {
		delete(m, hash)
	} else {
		m[hash] = rem
	}
}

const (
	// DefaultStripeSize is the default number of entries to allocate in the stripeSeries hash map.
	DefaultStripeSize = 1 << 14
)

// stripeSeries locks modulo ranges of IDs and hashes to reduce lock contention.
// The locks are padded to not be on the same cache line. Filling the padded space
// with the maps was profiled to be slower – likely due to the additional pointer
// dereferences.
type stripeSeries struct {
	size                    int
	series                  []map[uint64]*memSeries
	hashes                  []seriesHashmap
	locks                   []stripeLock
	seriesLifecycleCallback SeriesLifecycleCallback
}

type stripeLock struct {
	sync.RWMutex
	// Padding to avoid multiple locks being on the same cache line.
	_ [40]byte
}

func newStripeSeries(stripeSize int, seriesCallback SeriesLifecycleCallback) *stripeSeries {
	s := &stripeSeries{
		size:                    stripeSize,
		series:                  make([]map[uint64]*memSeries, stripeSize),
		hashes:                  make([]seriesHashmap, stripeSize),
		locks:                   make([]stripeLock, stripeSize),
		seriesLifecycleCallback: seriesCallback,
	}

	for i := range s.series {
		s.series[i] = map[uint64]*memSeries{}
	}
	for i := range s.hashes {
		s.hashes[i] = seriesHashmap{}
	}
	return s
}

// gc garbage collects old chunks that are strictly before mint and removes
// series entirely that have no chunks left.
func (s *stripeSeries) gc(mint int64) (map[uint64]struct{}, int, int64) {
	var (
		deleted                  = map[uint64]struct{}{}
		deletedForCallback       = []labels.Labels{}
		rmChunks                 = 0
		actualMint         int64 = math.MaxInt64
	)
	// Run through all series and truncate old chunks. Mark those with no
	// chunks left as deleted and store their ID.
	for i := 0; i < s.size; i++ {
		s.locks[i].Lock()

		for hash, all := range s.hashes[i] {
			for _, series := range all {
				series.Lock()
				rmChunks += series.truncateChunksBefore(mint)

				if len(series.mmappedChunks) > 0 || series.headChunk != nil || series.pendingCommit {
					seriesMint := series.minTime()
					if seriesMint < actualMint {
						actualMint = seriesMint
					}
					series.Unlock()
					continue
				}

				// The series is gone entirely. We need to keep the series lock
				// and make sure we have acquired the stripe locks for hash and ID of the
				// series alike.
				// If we don't hold them all, there's a very small chance that a series receives
				// samples again while we are half-way into deleting it.
				j := int(series.ref) & (s.size - 1)

				if i != j {
					s.locks[j].Lock()
				}

				deleted[series.ref] = struct{}{}
				s.hashes[i].del(hash, series.lset)
				delete(s.series[j], series.ref)
				deletedForCallback = append(deletedForCallback, series.lset)

				if i != j {
					s.locks[j].Unlock()
				}

				series.Unlock()
			}
		}

		s.locks[i].Unlock()

		s.seriesLifecycleCallback.PostDeletion(deletedForCallback...)
		deletedForCallback = deletedForCallback[:0]
	}

	if actualMint == math.MaxInt64 {
		actualMint = mint
	}

	return deleted, rmChunks, actualMint
}

func (s *stripeSeries) getByID(id uint64) *memSeries {
	i := id & uint64(s.size-1)

	s.locks[i].RLock()
	series := s.series[i][id]
	s.locks[i].RUnlock()

	return series
}

func (s *stripeSeries) getByHash(hash uint64, lset labels.Labels) *memSeries {
	i := hash & uint64(s.size-1)

	s.locks[i].RLock()
	series := s.hashes[i].get(hash, lset)
	s.locks[i].RUnlock()

	return series
}

func (s *stripeSeries) getOrSet(hash uint64, lset labels.Labels, createSeries func() *memSeries) (*memSeries, bool, error) {
	// PreCreation is called here to avoid calling it inside the lock.
	// It is not necessary to call it just before creating a series,
	// rather it gives a 'hint' whether to create a series or not.
	preCreationErr := s.seriesLifecycleCallback.PreCreation(lset)

	// Create the series, unless the PreCreation() callback as failed.
	// If failed, we'll not allow to create a new series anyway.
	var series *memSeries
	if preCreationErr == nil {
		series = createSeries()
	}

	i := hash & uint64(s.size-1)
	s.locks[i].Lock()

	if prev := s.hashes[i].get(hash, lset); prev != nil {
		s.locks[i].Unlock()
		return prev, false, nil
	}
	if preCreationErr == nil {
		s.hashes[i].set(hash, series)
	}
	s.locks[i].Unlock()

	if preCreationErr != nil {
		// The callback prevented creation of series.
		return nil, false, preCreationErr
	}
	// Setting the series in the s.hashes marks the creation of series
	// as any further calls to this methods would return that series.
	s.seriesLifecycleCallback.PostCreation(series.lset)

	i = series.ref & uint64(s.size-1)

	s.locks[i].Lock()
	s.series[i][series.ref] = series
	s.locks[i].Unlock()

	return series, true, nil
}

type sample struct {
	t int64
	v float64
}

func newSample(t int64, v float64) tsdbutil.Sample { return sample{t, v} }
func (s sample) T() int64                          { return s.t }
func (s sample) V() float64                        { return s.v }

// memSeries is the in-memory representation of a series. None of its methods
// are goroutine safe and it is the caller's responsibility to lock it.
type memSeries struct {
	sync.RWMutex

	ref           uint64
	lset          labels.Labels
	mmappedChunks []*mmappedChunk
	headChunk     *memChunk
	chunkRange    int64
	firstChunkID  int

	nextAt        int64 // Timestamp at which to cut the next chunk.
	sampleBuf     [4]sample
	pendingCommit bool // Whether there are samples waiting to be committed to this series.

	app chunkenc.Appender // Current appender for the chunk.

	memChunkPool *sync.Pool

	txs *txRing
}

func newMemSeries(lset labels.Labels, id uint64, chunkRange int64, memChunkPool *sync.Pool) *memSeries {
	s := &memSeries{
		lset:         lset,
		ref:          id,
		chunkRange:   chunkRange,
		nextAt:       math.MinInt64,
		txs:          newTxRing(4),
		memChunkPool: memChunkPool,
	}
	return s
}

func (s *memSeries) minTime() int64 {
	if len(s.mmappedChunks) > 0 {
		return s.mmappedChunks[0].minTime
	}
	if s.headChunk != nil {
		return s.headChunk.minTime
	}
	return math.MinInt64
}

func (s *memSeries) maxTime() int64 {
	c := s.head()
	if c == nil {
		return math.MinInt64
	}
	return c.maxTime
}

// truncateChunksBefore removes all chunks from the series that
// have no timestamp at or after mint.
// Chunk IDs remain unchanged.
func (s *memSeries) truncateChunksBefore(mint int64) (removed int) {
	if s.headChunk != nil && s.headChunk.maxTime < mint {
		// If head chunk is truncated, we can truncate all mmapped chunks.
		removed = 1 + len(s.mmappedChunks)
		s.firstChunkID += removed
		s.headChunk = nil
		s.mmappedChunks = nil
		return removed
	}
	if len(s.mmappedChunks) > 0 {
		for i, c := range s.mmappedChunks {
			if c.maxTime >= mint {
				break
			}
			removed = i + 1
		}
		s.mmappedChunks = append(s.mmappedChunks[:0], s.mmappedChunks[removed:]...)
		s.firstChunkID += removed
	}
	return removed
}

// cleanupAppendIDsBelow cleans up older appendIDs. Has to be called after
// acquiring lock.
func (s *memSeries) cleanupAppendIDsBelow(bound uint64) {
	s.txs.cleanupAppendIDsBelow(bound)
}

func (s *memSeries) head() *memChunk {
	return s.headChunk
}

type memChunk struct {
	chunk            chunkenc.Chunk
	minTime, maxTime int64
}

// OverlapsClosedInterval returns true if the chunk overlaps [mint, maxt].
func (mc *memChunk) OverlapsClosedInterval(mint, maxt int64) bool {
	return overlapsClosedInterval(mc.minTime, mc.maxTime, mint, maxt)
}

func overlapsClosedInterval(mint1, maxt1, mint2, maxt2 int64) bool {
	return mint1 <= maxt2 && mint2 <= maxt1
}

type mmappedChunk struct {
	ref              uint64
	numSamples       uint16
	minTime, maxTime int64
}

// Returns true if the chunk overlaps [mint, maxt].
func (mc *mmappedChunk) OverlapsClosedInterval(mint, maxt int64) bool {
	return overlapsClosedInterval(mc.minTime, mc.maxTime, mint, maxt)
}

type noopSeriesLifecycleCallback struct{}

func (noopSeriesLifecycleCallback) PreCreation(labels.Labels) error { return nil }
func (noopSeriesLifecycleCallback) PostCreation(labels.Labels)      {}
func (noopSeriesLifecycleCallback) PostDeletion(...labels.Labels)   {}

func (h *Head) Size() int64 {
	var walSize int64
	if h.wal != nil {
		walSize, _ = h.wal.Size()
	}
	cdmSize, _ := h.chunkDiskMapper.Size()
	return walSize + cdmSize
}

func (h *RangeHead) Size() int64 {
	return h.head.Size()
}

func (h *Head) startWALReplayStatus(startFrom, last int) {
	h.stats.WALReplayStatus.Lock()
	defer h.stats.WALReplayStatus.Unlock()

	h.stats.WALReplayStatus.Min = startFrom
	h.stats.WALReplayStatus.Max = last
	h.stats.WALReplayStatus.Current = startFrom
}

func (h *Head) updateWALReplayStatusRead(current int) {
	h.stats.WALReplayStatus.Lock()
	defer h.stats.WALReplayStatus.Unlock()

	h.stats.WALReplayStatus.Current = current
}
