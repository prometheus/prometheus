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
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"go.uber.org/atomic"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/zeropool"
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

	// defaultIsolationDisabled is true if isolation is disabled by default.
	defaultIsolationDisabled = false

	defaultWALReplayConcurrency = runtime.GOMAXPROCS(0)
)

// Head handles reads and writes of time series data within a time window.
type Head struct {
	chunkRange               atomic.Int64
	numSeries                atomic.Uint64
	minOOOTime, maxOOOTime   atomic.Int64 // TODO(jesusvazquez) These should be updated after garbage collection.
	minTime, maxTime         atomic.Int64 // Current min and max of the samples included in the head. TODO(jesusvazquez) Ensure these are properly tracked.
	minValidTime             atomic.Int64 // Mint allowed to be added to the head. It shouldn't be lower than the maxt of the last persisted block.
	lastWALTruncationTime    atomic.Int64
	lastMemoryTruncationTime atomic.Int64
	lastSeriesID             atomic.Uint64
	// All the ooo m-map chunks should be after this. This is used to truncate old ooo m-map chunks.
	// This should be typecasted to chunks.ChunkDiskMapperRef after loading.
	minOOOMmapRef atomic.Uint64

	metrics             *headMetrics
	opts                *HeadOptions
	wal, wbl            *wlog.WL
	exemplarMetrics     *ExemplarMetrics
	exemplars           ExemplarStorage
	logger              log.Logger
	appendPool          zeropool.Pool[[]record.RefSample]
	exemplarsPool       zeropool.Pool[[]exemplarWithSeriesRef]
	histogramsPool      zeropool.Pool[[]record.RefHistogramSample]
	floatHistogramsPool zeropool.Pool[[]record.RefFloatHistogramSample]
	metadataPool        zeropool.Pool[[]record.RefMetadata]
	seriesPool          zeropool.Pool[[]*memSeries]
	bytesPool           zeropool.Pool[[]byte]
	memChunkPool        sync.Pool

	// All series addressable by their ID or hash.
	series *stripeSeries

	deletedMtx sync.Mutex
	deleted    map[chunks.HeadSeriesRef]int // Deleted series, and what WAL segment they must be kept until.

	// TODO(codesome): Extend MemPostings to return only OOOPostings, Set OOOStatus, ... Like an additional map of ooo postings.
	postings *index.MemPostings // Postings lists for terms.

	tombstones *tombstones.MemTombstones

	iso *isolation

	oooIso *oooIsolation

	cardinalityMutex      sync.Mutex
	cardinalityCache      *index.PostingsStats // Posting stats cache which will expire after 30sec.
	cardinalityCacheKey   string
	lastPostingsStatsCall time.Duration // Last posting stats call (PostingsCardinalityStats()) time for caching.

	// chunkDiskMapper is used to write and read Head chunks to/from disk.
	chunkDiskMapper *chunks.ChunkDiskMapper

	chunkSnapshotMtx sync.Mutex

	closedMtx sync.Mutex
	closed    bool

	stats *HeadStats
	reg   prometheus.Registerer

	writeNotified wlog.WriteNotified

	memTruncationInProcess atomic.Bool
}

type ExemplarStorage interface {
	storage.ExemplarQueryable
	AddExemplar(labels.Labels, exemplar.Exemplar) error
	ValidateExemplar(labels.Labels, exemplar.Exemplar) error
	IterateExemplars(f func(seriesLabels labels.Labels, e exemplar.Exemplar) error) error
}

// HeadOptions are parameters for the Head block.
type HeadOptions struct {
	// Runtime reloadable option. At the top of the struct for 32 bit OS:
	// https://pkg.go.dev/sync/atomic#pkg-note-BUG
	MaxExemplars atomic.Int64

	OutOfOrderTimeWindow atomic.Int64
	OutOfOrderCapMax     atomic.Int64

	// EnableNativeHistograms enables the ingestion of native histograms.
	EnableNativeHistograms atomic.Bool

	// EnableCreatedTimestampZeroIngestion enables the ingestion of the created timestamp as a synthetic zero sample.
	// See: https://github.com/prometheus/proposals/blob/main/proposals/2023-06-13_created-timestamp.md
	EnableCreatedTimestampZeroIngestion bool

	ChunkRange int64
	// ChunkDirRoot is the parent directory of the chunks directory.
	ChunkDirRoot         string
	ChunkPool            chunkenc.Pool
	ChunkWriteBufferSize int
	ChunkWriteQueueSize  int

	SamplesPerChunk int

	// StripeSize sets the number of entries in the hash map, it must be a power of 2.
	// A larger StripeSize will allocate more memory up-front, but will increase performance when handling a large number of series.
	// A smaller StripeSize reduces the memory allocated, but can decrease performance with large number of series.
	StripeSize                     int
	SeriesCallback                 SeriesLifecycleCallback
	EnableExemplarStorage          bool
	EnableMemorySnapshotOnShutdown bool

	IsolationDisabled bool

	// Maximum number of CPUs that can simultaneously processes WAL replay.
	// The default value is GOMAXPROCS.
	// If it is set to a negative value or zero, the default value is used.
	WALReplayConcurrency int
}

const (
	// DefaultOutOfOrderCapMax is the default maximum size of an in-memory out-of-order chunk.
	DefaultOutOfOrderCapMax int64 = 32
	// DefaultSamplesPerChunk provides a default target number of samples per chunk.
	DefaultSamplesPerChunk = 120
)

func DefaultHeadOptions() *HeadOptions {
	ho := &HeadOptions{
		ChunkRange:           DefaultBlockDuration,
		ChunkDirRoot:         "",
		ChunkPool:            chunkenc.NewPool(),
		ChunkWriteBufferSize: chunks.DefaultWriteBufferSize,
		ChunkWriteQueueSize:  chunks.DefaultWriteQueueSize,
		SamplesPerChunk:      DefaultSamplesPerChunk,
		StripeSize:           DefaultStripeSize,
		SeriesCallback:       &noopSeriesLifecycleCallback{},
		IsolationDisabled:    defaultIsolationDisabled,
		WALReplayConcurrency: defaultWALReplayConcurrency,
	}
	ho.OutOfOrderCapMax.Store(DefaultOutOfOrderCapMax)
	return ho
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
	PostDeletion(map[chunks.HeadSeriesRef]labels.Labels)
}

// NewHead opens the head block in dir.
func NewHead(r prometheus.Registerer, l log.Logger, wal, wbl *wlog.WL, opts *HeadOptions, stats *HeadStats) (*Head, error) {
	var err error
	if l == nil {
		l = log.NewNopLogger()
	}

	if opts.OutOfOrderTimeWindow.Load() < 0 {
		opts.OutOfOrderTimeWindow.Store(0)
	}

	// Time window can be set on runtime. So the capMin and capMax should be valid
	// even if ooo is not enabled yet.
	capMax := opts.OutOfOrderCapMax.Load()
	if capMax <= 0 || capMax > 255 {
		return nil, fmt.Errorf("OOOCapMax of %d is invalid. must be > 0 and <= 255", capMax)
	}

	if opts.ChunkRange < 1 {
		return nil, fmt.Errorf("invalid chunk range %d", opts.ChunkRange)
	}
	if opts.SeriesCallback == nil {
		opts.SeriesCallback = &noopSeriesLifecycleCallback{}
	}

	if stats == nil {
		stats = NewHeadStats()
	}

	if !opts.EnableExemplarStorage {
		opts.MaxExemplars.Store(0)
	}

	h := &Head{
		wal:    wal,
		wbl:    wbl,
		logger: l,
		opts:   opts,
		memChunkPool: sync.Pool{
			New: func() interface{} {
				return &memChunk{}
			},
		},
		stats: stats,
		reg:   r,
	}
	if err := h.resetInMemoryState(); err != nil {
		return nil, err
	}

	if opts.ChunkPool == nil {
		opts.ChunkPool = chunkenc.NewPool()
	}

	if opts.WALReplayConcurrency <= 0 {
		opts.WALReplayConcurrency = defaultWALReplayConcurrency
	}

	h.chunkDiskMapper, err = chunks.NewChunkDiskMapper(
		r,
		mmappedChunksDir(opts.ChunkDirRoot),
		opts.ChunkPool,
		opts.ChunkWriteBufferSize,
		opts.ChunkWriteQueueSize,
	)
	if err != nil {
		return nil, err
	}
	h.metrics = newHeadMetrics(h, r)

	return h, nil
}

func (h *Head) resetInMemoryState() error {
	var err error
	var em *ExemplarMetrics
	if h.exemplars != nil {
		ce, ok := h.exemplars.(*CircularExemplarStorage)
		if ok {
			em = ce.metrics
		}
	}
	if em == nil {
		em = NewExemplarMetrics(h.reg)
	}
	es, err := NewCircularExemplarStorage(h.opts.MaxExemplars.Load(), em)
	if err != nil {
		return err
	}

	h.iso = newIsolation(h.opts.IsolationDisabled)
	h.oooIso = newOOOIsolation()

	h.exemplarMetrics = em
	h.exemplars = es
	h.series = newStripeSeries(h.opts.StripeSize, h.opts.SeriesCallback)
	h.postings = index.NewUnorderedMemPostings()
	h.tombstones = tombstones.NewMemTombstones()
	h.deleted = map[chunks.HeadSeriesRef]int{}
	h.chunkRange.Store(h.opts.ChunkRange)
	h.minTime.Store(math.MaxInt64)
	h.maxTime.Store(math.MinInt64)
	h.minOOOTime.Store(math.MaxInt64)
	h.maxOOOTime.Store(math.MinInt64)
	h.lastWALTruncationTime.Store(math.MinInt64)
	h.lastMemoryTruncationTime.Store(math.MinInt64)
	return nil
}

type headMetrics struct {
	activeAppenders           prometheus.Gauge
	series                    prometheus.GaugeFunc
	seriesCreated             prometheus.Counter
	seriesRemoved             prometheus.Counter
	seriesNotFound            prometheus.Counter
	chunks                    prometheus.Gauge
	chunksCreated             prometheus.Counter
	chunksRemoved             prometheus.Counter
	gcDuration                prometheus.Summary
	samplesAppended           *prometheus.CounterVec
	outOfOrderSamplesAppended prometheus.Counter
	outOfBoundSamples         *prometheus.CounterVec
	outOfOrderSamples         *prometheus.CounterVec
	tooOldSamples             *prometheus.CounterVec
	walTruncateDuration       prometheus.Summary
	walCorruptionsTotal       prometheus.Counter
	dataTotalReplayDuration   prometheus.Gauge
	headTruncateFail          prometheus.Counter
	headTruncateTotal         prometheus.Counter
	checkpointDeleteFail      prometheus.Counter
	checkpointDeleteTotal     prometheus.Counter
	checkpointCreationFail    prometheus.Counter
	checkpointCreationTotal   prometheus.Counter
	mmapChunkCorruptionTotal  prometheus.Counter
	snapshotReplayErrorTotal  prometheus.Counter // Will be either 0 or 1.
	oooHistogram              prometheus.Histogram
	mmapChunksTotal           prometheus.Counter
}

const (
	sampleMetricTypeFloat     = "float"
	sampleMetricTypeHistogram = "histogram"
)

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
		dataTotalReplayDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_data_replay_duration_seconds",
			Help: "Time taken to replay the data on disk.",
		}),
		samplesAppended: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_samples_appended_total",
			Help: "Total number of appended samples.",
		}, []string{"type"}),
		outOfOrderSamplesAppended: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_out_of_order_samples_appended_total",
			Help: "Total number of appended out of order samples.",
		}),
		outOfBoundSamples: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_bound_samples_total",
			Help: "Total number of out of bound samples ingestion failed attempts with out of order support disabled.",
		}, []string{"type"}),
		outOfOrderSamples: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_samples_total",
			Help: "Total number of out of order samples ingestion failed attempts due to out of order being disabled.",
		}, []string{"type"}),
		tooOldSamples: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_too_old_samples_total",
			Help: "Total number of out of order samples ingestion failed attempts with out of support enabled, but sample outside of time window.",
		}, []string{"type"}),
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
		snapshotReplayErrorTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_snapshot_replay_error_total",
			Help: "Total number snapshot replays that failed.",
		}),
		oooHistogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "prometheus_tsdb_sample_ooo_delta",
			Help: "Delta in seconds by which a sample is considered out of order (reported regardless of OOO time window and whether sample is accepted or not).",
			Buckets: []float64{
				60 * 10,      // 10 min
				60 * 30,      // 30 min
				60 * 60,      // 60 min
				60 * 60 * 2,  // 2h
				60 * 60 * 3,  // 3h
				60 * 60 * 6,  // 6h
				60 * 60 * 12, // 12h
			},
		}),
		mmapChunksTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_mmap_chunks_total",
			Help: "Total number of chunks that were memory-mapped.",
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
			m.dataTotalReplayDuration,
			m.samplesAppended,
			m.outOfOrderSamplesAppended,
			m.outOfBoundSamples,
			m.outOfOrderSamples,
			m.tooOldSamples,
			m.headTruncateFail,
			m.headTruncateTotal,
			m.checkpointDeleteFail,
			m.checkpointDeleteTotal,
			m.checkpointCreationFail,
			m.checkpointCreationTotal,
			m.mmapChunksTotal,
			m.mmapChunkCorruptionTotal,
			m.snapshotReplayErrorTotal,
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
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{
				Name: "prometheus_tsdb_head_chunks_storage_size_bytes",
				Help: "Size of the chunks_head directory.",
			}, func() float64 {
				val, err := h.chunkDiskMapper.Size()
				if err != nil {
					level.Error(h.logger).Log("msg", "Failed to calculate size of \"chunks_head\" dir",
						"err", err.Error())
				}
				return float64(val)
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
	defer func() {
		h.postings.EnsureOrder(h.opts.WALReplayConcurrency)
	}()
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

	snapIdx, snapOffset := -1, 0
	refSeries := make(map[chunks.HeadSeriesRef]*memSeries)

	snapshotLoaded := false
	if h.opts.EnableMemorySnapshotOnShutdown {
		level.Info(h.logger).Log("msg", "Chunk snapshot is enabled, replaying from the snapshot")
		// If there are any WAL files, there should be at least one WAL file with an index that is current or newer
		// than the snapshot index. If the WAL index is behind the snapshot index somehow, the snapshot is assumed
		// to be outdated.
		loadSnapshot := true
		if h.wal != nil {
			_, endAt, err := wlog.Segments(h.wal.Dir())
			if err != nil {
				return fmt.Errorf("finding WAL segments: %w", err)
			}

			_, idx, _, err := LastChunkSnapshot(h.opts.ChunkDirRoot)
			if err != nil && !errors.Is(err, record.ErrNotFound) {
				level.Error(h.logger).Log("msg", "Could not find last snapshot", "err", err)
			}

			if err == nil && endAt < idx {
				loadSnapshot = false
				level.Warn(h.logger).Log("msg", "Last WAL file is behind snapshot, removing snapshots")
				if err := DeleteChunkSnapshots(h.opts.ChunkDirRoot, math.MaxInt, math.MaxInt); err != nil {
					level.Error(h.logger).Log("msg", "Error while deleting snapshot directories", "err", err)
				}
			}
		}
		if loadSnapshot {
			var err error
			snapIdx, snapOffset, refSeries, err = h.loadChunkSnapshot()
			if err == nil {
				snapshotLoaded = true
				level.Info(h.logger).Log("msg", "Chunk snapshot loading time", "duration", time.Since(start).String())
			}
			if err != nil {
				snapIdx, snapOffset = -1, 0
				refSeries = make(map[chunks.HeadSeriesRef]*memSeries)

				h.metrics.snapshotReplayErrorTotal.Inc()
				level.Error(h.logger).Log("msg", "Failed to load chunk snapshot", "err", err)
				// We clear the partially loaded data to replay fresh from the WAL.
				if err := h.resetInMemoryState(); err != nil {
					return err
				}
			}
		}
	}

	mmapChunkReplayStart := time.Now()
	var (
		mmappedChunks    map[chunks.HeadSeriesRef][]*mmappedChunk
		oooMmappedChunks map[chunks.HeadSeriesRef][]*mmappedChunk
		lastMmapRef      chunks.ChunkDiskMapperRef
		err              error
	)
	if snapshotLoaded || h.wal != nil {
		// If snapshot was not loaded and if there is no WAL, then m-map chunks will be discarded
		// anyway. So we only load m-map chunks when it won't be discarded.
		mmappedChunks, oooMmappedChunks, lastMmapRef, err = h.loadMmappedChunks(refSeries)
		if err != nil {
			// TODO(codesome): clear out all m-map chunks here for refSeries.
			level.Error(h.logger).Log("msg", "Loading on-disk chunks failed", "err", err)
			var cerr *chunks.CorruptionErr
			if errors.As(err, &cerr) {
				h.metrics.mmapChunkCorruptionTotal.Inc()
			}

			// Discard snapshot data since we need to replay the WAL for the missed m-map chunks data.
			snapIdx, snapOffset = -1, 0

			// If this fails, data will be recovered from WAL.
			// Hence we wont lose any data (given WAL is not corrupt).
			mmappedChunks, oooMmappedChunks, lastMmapRef, err = h.removeCorruptedMmappedChunks(err)
			if err != nil {
				return err
			}
		}
		level.Info(h.logger).Log("msg", "On-disk memory mappable chunks replay completed", "duration", time.Since(mmapChunkReplayStart).String())
	}

	if h.wal == nil {
		level.Info(h.logger).Log("msg", "WAL not found")
		return nil
	}

	level.Info(h.logger).Log("msg", "Replaying WAL, this may take a while")

	checkpointReplayStart := time.Now()
	// Backfill the checkpoint first if it exists.
	dir, startFrom, err := wlog.LastCheckpoint(h.wal.Dir())
	if err != nil && !errors.Is(err, record.ErrNotFound) {
		return fmt.Errorf("find last checkpoint: %w", err)
	}

	// Find the last segment.
	_, endAt, e := wlog.Segments(h.wal.Dir())
	if e != nil {
		return fmt.Errorf("finding WAL segments: %w", e)
	}

	h.startWALReplayStatus(startFrom, endAt)

	multiRef := map[chunks.HeadSeriesRef]chunks.HeadSeriesRef{}
	if err == nil && startFrom >= snapIdx {
		sr, err := wlog.NewSegmentsReader(dir)
		if err != nil {
			return fmt.Errorf("open checkpoint: %w", err)
		}
		defer func() {
			if err := sr.Close(); err != nil {
				level.Warn(h.logger).Log("msg", "Error while closing the wal segments reader", "err", err)
			}
		}()

		// A corrupted checkpoint is a hard error for now and requires user
		// intervention. There's likely little data that can be recovered anyway.
		if err := h.loadWAL(wlog.NewReader(sr), multiRef, mmappedChunks, oooMmappedChunks); err != nil {
			return fmt.Errorf("backfill checkpoint: %w", err)
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
		s, err := wlog.OpenReadSegment(wlog.SegmentName(h.wal.Dir(), i))
		if err != nil {
			return fmt.Errorf("open WAL segment: %d: %w", i, err)
		}

		offset := 0
		if i == snapIdx {
			offset = snapOffset
		}
		sr, err := wlog.NewSegmentBufReaderWithOffset(offset, s)
		if errors.Is(err, io.EOF) {
			// File does not exist.
			continue
		}
		if err != nil {
			return fmt.Errorf("segment reader (offset=%d): %w", offset, err)
		}
		err = h.loadWAL(wlog.NewReader(sr), multiRef, mmappedChunks, oooMmappedChunks)
		if err := sr.Close(); err != nil {
			level.Warn(h.logger).Log("msg", "Error while closing the wal segments reader", "err", err)
		}
		if err != nil {
			return err
		}
		level.Info(h.logger).Log("msg", "WAL segment loaded", "segment", i, "maxSegment", endAt)
		h.updateWALReplayStatusRead(i)
	}
	walReplayDuration := time.Since(walReplayStart)

	wblReplayStart := time.Now()
	if h.wbl != nil {
		// Replay WBL.
		startFrom, endAt, e = wlog.Segments(h.wbl.Dir())
		if e != nil {
			return &errLoadWbl{fmt.Errorf("finding WBL segments: %w", e)}
		}
		h.startWALReplayStatus(startFrom, endAt)

		for i := startFrom; i <= endAt; i++ {
			s, err := wlog.OpenReadSegment(wlog.SegmentName(h.wbl.Dir(), i))
			if err != nil {
				return &errLoadWbl{fmt.Errorf("open WBL segment: %d: %w", i, err)}
			}

			sr := wlog.NewSegmentBufReader(s)
			err = h.loadWBL(wlog.NewReader(sr), multiRef, lastMmapRef)
			if err := sr.Close(); err != nil {
				level.Warn(h.logger).Log("msg", "Error while closing the wbl segments reader", "err", err)
			}
			if err != nil {
				return &errLoadWbl{err}
			}
			level.Info(h.logger).Log("msg", "WBL segment loaded", "segment", i, "maxSegment", endAt)
			h.updateWALReplayStatusRead(i)
		}
	}

	wblReplayDuration := time.Since(wblReplayStart)

	totalReplayDuration := time.Since(start)
	h.metrics.dataTotalReplayDuration.Set(totalReplayDuration.Seconds())
	level.Info(h.logger).Log(
		"msg", "WAL replay completed",
		"checkpoint_replay_duration", checkpointReplayDuration.String(),
		"wal_replay_duration", walReplayDuration.String(),
		"wbl_replay_duration", wblReplayDuration.String(),
		"total_replay_duration", totalReplayDuration.String(),
	)

	return nil
}

func (h *Head) loadMmappedChunks(refSeries map[chunks.HeadSeriesRef]*memSeries) (map[chunks.HeadSeriesRef][]*mmappedChunk, map[chunks.HeadSeriesRef][]*mmappedChunk, chunks.ChunkDiskMapperRef, error) {
	mmappedChunks := map[chunks.HeadSeriesRef][]*mmappedChunk{}
	oooMmappedChunks := map[chunks.HeadSeriesRef][]*mmappedChunk{}
	var lastRef, secondLastRef chunks.ChunkDiskMapperRef
	if err := h.chunkDiskMapper.IterateAllChunks(func(seriesRef chunks.HeadSeriesRef, chunkRef chunks.ChunkDiskMapperRef, mint, maxt int64, numSamples uint16, encoding chunkenc.Encoding, isOOO bool) error {
		secondLastRef = lastRef
		lastRef = chunkRef
		if !isOOO && maxt < h.minValidTime.Load() {
			return nil
		}

		// We ignore any chunk that doesn't have a valid encoding
		if !chunkenc.IsValidEncoding(encoding) {
			return nil
		}

		ms, ok := refSeries[seriesRef]

		if isOOO {
			if !ok {
				oooMmappedChunks[seriesRef] = append(oooMmappedChunks[seriesRef], &mmappedChunk{
					ref:        chunkRef,
					minTime:    mint,
					maxTime:    maxt,
					numSamples: numSamples,
				})
				return nil
			}

			h.metrics.chunks.Inc()
			h.metrics.chunksCreated.Inc()

			if ms.ooo == nil {
				ms.ooo = &memSeriesOOOFields{}
			}

			ms.ooo.oooMmappedChunks = append(ms.ooo.oooMmappedChunks, &mmappedChunk{
				ref:        chunkRef,
				minTime:    mint,
				maxTime:    maxt,
				numSamples: numSamples,
			})

			h.updateMinOOOMaxOOOTime(mint, maxt)
			return nil
		}

		if !ok {
			slice := mmappedChunks[seriesRef]
			if len(slice) > 0 && slice[len(slice)-1].maxTime >= mint {
				h.metrics.mmapChunkCorruptionTotal.Inc()
				return fmt.Errorf("out of sequence m-mapped chunk for series ref %d, last chunk: [%d, %d], new: [%d, %d]",
					seriesRef, slice[len(slice)-1].minTime, slice[len(slice)-1].maxTime, mint, maxt)
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
			h.metrics.mmapChunkCorruptionTotal.Inc()
			return fmt.Errorf("out of sequence m-mapped chunk for series ref %d, last chunk: [%d, %d], new: [%d, %d]",
				seriesRef, ms.mmappedChunks[len(ms.mmappedChunks)-1].minTime, ms.mmappedChunks[len(ms.mmappedChunks)-1].maxTime,
				mint, maxt)
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
		if ms.headChunks != nil && maxt >= ms.headChunks.minTime {
			// The head chunk was completed and was m-mapped after taking the snapshot.
			// Hence remove this chunk.
			ms.nextAt = 0
			ms.headChunks = nil
			ms.app = nil
		}
		return nil
	}); err != nil {
		// secondLastRef because the lastRef caused an error.
		return nil, nil, secondLastRef, fmt.Errorf("iterate on on-disk chunks: %w", err)
	}
	return mmappedChunks, oooMmappedChunks, lastRef, nil
}

// removeCorruptedMmappedChunks attempts to delete the corrupted mmapped chunks and if it fails, it clears all the previously
// loaded mmapped chunks.
func (h *Head) removeCorruptedMmappedChunks(err error) (map[chunks.HeadSeriesRef][]*mmappedChunk, map[chunks.HeadSeriesRef][]*mmappedChunk, chunks.ChunkDiskMapperRef, error) {
	level.Info(h.logger).Log("msg", "Deleting mmapped chunk files")
	// We never want to preserve the in-memory series from snapshots if we are repairing m-map chunks.
	if err := h.resetInMemoryState(); err != nil {
		return map[chunks.HeadSeriesRef][]*mmappedChunk{}, map[chunks.HeadSeriesRef][]*mmappedChunk{}, 0, err
	}

	level.Info(h.logger).Log("msg", "Deleting mmapped chunk files")

	if err := h.chunkDiskMapper.DeleteCorrupted(err); err != nil {
		level.Info(h.logger).Log("msg", "Deletion of corrupted mmap chunk files failed, discarding chunk files completely", "err", err)
		if err := h.chunkDiskMapper.Truncate(math.MaxUint32); err != nil {
			level.Error(h.logger).Log("msg", "Deletion of all mmap chunk files failed", "err", err)
		}
		return map[chunks.HeadSeriesRef][]*mmappedChunk{}, map[chunks.HeadSeriesRef][]*mmappedChunk{}, 0, nil
	}

	level.Info(h.logger).Log("msg", "Deletion of mmap chunk files successful, reattempting m-mapping the on-disk chunks")
	mmappedChunks, oooMmappedChunks, lastRef, err := h.loadMmappedChunks(make(map[chunks.HeadSeriesRef]*memSeries))
	if err != nil {
		level.Error(h.logger).Log("msg", "Loading on-disk chunks failed, discarding chunk files completely", "err", err)
		if err := h.chunkDiskMapper.Truncate(math.MaxUint32); err != nil {
			level.Error(h.logger).Log("msg", "Deletion of all mmap chunk files failed after failed loading", "err", err)
		}
		mmappedChunks = map[chunks.HeadSeriesRef][]*mmappedChunk{}
	}

	return mmappedChunks, oooMmappedChunks, lastRef, nil
}

func (h *Head) ApplyConfig(cfg *config.Config, wbl *wlog.WL) {
	oooTimeWindow := int64(0)
	if cfg.StorageConfig.TSDBConfig != nil {
		oooTimeWindow = cfg.StorageConfig.TSDBConfig.OutOfOrderTimeWindow
	}
	if oooTimeWindow < 0 {
		oooTimeWindow = 0
	}

	h.SetOutOfOrderTimeWindow(oooTimeWindow, wbl)

	if !h.opts.EnableExemplarStorage {
		return
	}

	// Head uses opts.MaxExemplars in combination with opts.EnableExemplarStorage
	// to decide if it should pass exemplars along to its exemplar storage, so we
	// need to update opts.MaxExemplars here.
	prevSize := h.opts.MaxExemplars.Load()
	h.opts.MaxExemplars.Store(cfg.StorageConfig.ExemplarsConfig.MaxExemplars)
	newSize := h.opts.MaxExemplars.Load()

	if prevSize == newSize {
		return
	}

	migrated := h.exemplars.(*CircularExemplarStorage).Resize(newSize)
	level.Info(h.logger).Log("msg", "Exemplar storage resized", "from", prevSize, "to", newSize, "migrated", migrated)
}

// SetOutOfOrderTimeWindow updates the out of order related parameters.
// If the Head already has a WBL set, then the wbl will be ignored.
func (h *Head) SetOutOfOrderTimeWindow(oooTimeWindow int64, wbl *wlog.WL) {
	if oooTimeWindow > 0 && h.wbl == nil {
		h.wbl = wbl
	}

	h.opts.OutOfOrderTimeWindow.Store(oooTimeWindow)
}

// EnableNativeHistograms enables the native histogram feature.
func (h *Head) EnableNativeHistograms() {
	h.opts.EnableNativeHistograms.Store(true)
}

// DisableNativeHistograms disables the native histogram feature.
func (h *Head) DisableNativeHistograms() {
	h.opts.EnableNativeHistograms.Store(false)
}

// PostingsCardinalityStats returns highest cardinality stats by label and value names.
func (h *Head) PostingsCardinalityStats(statsByLabelName string, limit int) *index.PostingsStats {
	cacheKey := statsByLabelName + ";" + strconv.Itoa(limit)

	h.cardinalityMutex.Lock()
	defer h.cardinalityMutex.Unlock()
	if h.cardinalityCacheKey != cacheKey {
		h.cardinalityCache = nil
	} else {
		currentTime := time.Duration(time.Now().Unix()) * time.Second
		seconds := currentTime - h.lastPostingsStatsCall
		if seconds > cardinalityCacheExpirationTime {
			h.cardinalityCache = nil
		}
	}
	if h.cardinalityCache != nil {
		return h.cardinalityCache
	}
	h.cardinalityCacheKey = cacheKey
	h.cardinalityCache = h.postings.Stats(statsByLabelName, limit)
	h.lastPostingsStatsCall = time.Duration(time.Now().Unix()) * time.Second

	return h.cardinalityCache
}

func (h *Head) updateMinMaxTime(mint, maxt int64) {
	for {
		lt := h.MinTime()
		if mint >= lt {
			break
		}
		if h.minTime.CompareAndSwap(lt, mint) {
			break
		}
	}
	for {
		ht := h.MaxTime()
		if maxt <= ht {
			break
		}
		if h.maxTime.CompareAndSwap(ht, maxt) {
			break
		}
	}
}

func (h *Head) updateMinOOOMaxOOOTime(mint, maxt int64) {
	for {
		lt := h.MinOOOTime()
		if mint >= lt {
			break
		}
		if h.minOOOTime.CompareAndSwap(lt, mint) {
			break
		}
	}
	for {
		ht := h.MaxOOOTime()
		if maxt <= ht {
			break
		}
		if h.maxOOOTime.CompareAndSwap(ht, maxt) {
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

// OverlapsClosedInterval returns true if the head overlaps [mint, maxt].
func (h *Head) OverlapsClosedInterval(mint, maxt int64) bool {
	return h.MinTime() <= maxt && mint <= h.MaxTime()
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
		h.maxTime.CompareAndSwap(h.MaxTime(), mint)
	}

	// This was an initial call to Truncate after loading blocks on startup.
	// We haven't read back the WAL yet, so do not attempt to truncate it.
	if initialize {
		return nil
	}

	h.metrics.headTruncateTotal.Inc()
	return h.truncateSeriesAndChunkDiskMapper("truncateMemory")
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

// WaitForPendingReadersForOOOChunksAtOrBefore is like WaitForPendingReadersInTimeRange, except it waits for
// queries touching OOO chunks less than or equal to chunk to finish querying.
func (h *Head) WaitForPendingReadersForOOOChunksAtOrBefore(chunk chunks.ChunkDiskMapperRef) {
	for h.oooIso.HasOpenReadsAtOrBefore(chunk) {
		time.Sleep(500 * time.Millisecond)
	}
}

// WaitForAppendersOverlapping waits for appends overlapping maxt to finish.
func (h *Head) WaitForAppendersOverlapping(maxt int64) {
	for maxt >= h.iso.lowestAppendTime() {
		time.Sleep(500 * time.Millisecond)
	}
}

// IsQuerierCollidingWithTruncation returns if the current querier needs to be closed and if a new querier
// has to be created. In the latter case, the method also returns the new mint to be used for creating the
// new range head and the new querier. This methods helps preventing races with the truncation of in-memory data.
//
// NOTE: The querier should already be taken before calling this.
func (h *Head) IsQuerierCollidingWithTruncation(querierMint, querierMaxt int64) (shouldClose, getNew bool, newMint int64) {
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

	first, last, err := wlog.Segments(h.wal.Dir())
	if err != nil {
		return fmt.Errorf("get segment range: %w", err)
	}
	// Start a new segment, so low ingestion volume TSDB don't have more WAL than
	// needed.
	if _, err := h.wal.NextSegment(); err != nil {
		return fmt.Errorf("next segment: %w", err)
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

	keep := func(id chunks.HeadSeriesRef) bool {
		if h.series.getByID(id) != nil {
			return true
		}
		h.deletedMtx.Lock()
		keepUntil, ok := h.deleted[id]
		h.deletedMtx.Unlock()
		return ok && keepUntil > last
	}
	h.metrics.checkpointCreationTotal.Inc()
	if _, err = wlog.Checkpoint(h.logger, h.wal, first, last, keep, mint); err != nil {
		h.metrics.checkpointCreationFail.Inc()
		var cerr *chunks.CorruptionErr
		if errors.As(err, &cerr) {
			h.metrics.walCorruptionsTotal.Inc()
		}
		return fmt.Errorf("create checkpoint: %w", err)
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
		if segment <= last {
			delete(h.deleted, ref)
		}
	}
	h.deletedMtx.Unlock()

	h.metrics.checkpointDeleteTotal.Inc()
	if err := wlog.DeleteCheckpoints(h.wal.Dir(), last); err != nil {
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

// truncateOOO
//   - waits for any pending reads that potentially touch chunks less than or equal to newMinOOOMmapRef
//   - truncates the OOO WBL files whose index is strictly less than lastWBLFile.
//   - garbage collects all the m-map chunks from the memory that are less than or equal to newMinOOOMmapRef
//     and then deletes the series that do not have any data anymore.
//
// The caller is responsible for ensuring that no further queriers will be created that reference chunks less
// than or equal to newMinOOOMmapRef before calling truncateOOO.
func (h *Head) truncateOOO(lastWBLFile int, newMinOOOMmapRef chunks.ChunkDiskMapperRef) error {
	curMinOOOMmapRef := chunks.ChunkDiskMapperRef(h.minOOOMmapRef.Load())
	if newMinOOOMmapRef.GreaterThan(curMinOOOMmapRef) {
		h.WaitForPendingReadersForOOOChunksAtOrBefore(newMinOOOMmapRef)
		h.minOOOMmapRef.Store(uint64(newMinOOOMmapRef))

		if err := h.truncateSeriesAndChunkDiskMapper("truncateOOO"); err != nil {
			return err
		}
	}

	if h.wbl == nil {
		return nil
	}

	return h.wbl.Truncate(lastWBLFile)
}

// truncateSeriesAndChunkDiskMapper is a helper function for truncateMemory and truncateOOO.
// It runs GC on the Head and truncates the ChunkDiskMapper accordingly.
func (h *Head) truncateSeriesAndChunkDiskMapper(caller string) error {
	start := time.Now()
	headMaxt := h.MaxTime()
	actualMint, minOOOTime, minMmapFile := h.gc()
	level.Info(h.logger).Log("msg", "Head GC completed", "caller", caller, "duration", time.Since(start))
	h.metrics.gcDuration.Observe(time.Since(start).Seconds())

	if actualMint > h.minTime.Load() {
		// The actual mint of the head is higher than the one asked to truncate.
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
	if headMaxt-h.opts.OutOfOrderTimeWindow.Load() < minOOOTime {
		// The allowed OOO window is lower than the min OOO time seen during GC.
		// So it is possible that some OOO sample was inserted that was less that minOOOTime.
		// So we play safe and set it to the min that was possible.
		minOOOTime = headMaxt - h.opts.OutOfOrderTimeWindow.Load()
	}
	h.minOOOTime.Store(minOOOTime)

	// Truncate the chunk m-mapper.
	if err := h.chunkDiskMapper.Truncate(uint32(minMmapFile)); err != nil {
		return fmt.Errorf("truncate chunks.HeadReadWriter by file number: %w", err)
	}
	return nil
}

type Stats struct {
	NumSeries         uint64
	MinTime, MaxTime  int64
	IndexPostingStats *index.PostingsStats
}

// Stats returns important current HEAD statistics. Note that it is expensive to
// calculate these.
func (h *Head) Stats(statsByLabelName string, limit int) *Stats {
	return &Stats{
		NumSeries:         h.NumSeries(),
		MaxTime:           h.MaxTime(),
		MinTime:           h.MinTime(),
		IndexPostingStats: h.PostingsCardinalityStats(statsByLabelName, limit),
	}
}

// RangeHead allows querying Head via an IndexReader, ChunkReader and tombstones.Reader
// but only within a restricted range.  Used for queries and compactions.
type RangeHead struct {
	head       *Head
	mint, maxt int64

	isolationOff bool
}

// NewRangeHead returns a *RangeHead.
// There are no restrictions on mint/maxt.
func NewRangeHead(head *Head, mint, maxt int64) *RangeHead {
	return &RangeHead{
		head: head,
		mint: mint,
		maxt: maxt,
	}
}

// NewRangeHeadWithIsolationDisabled returns a *RangeHead that does not create an isolationState.
func NewRangeHeadWithIsolationDisabled(head *Head, mint, maxt int64) *RangeHead {
	rh := NewRangeHead(head, mint, maxt)
	rh.isolationOff = true
	return rh
}

func (h *RangeHead) Index() (IndexReader, error) {
	return h.head.indexRange(h.mint, h.maxt), nil
}

func (h *RangeHead) Chunks() (ChunkReader, error) {
	var isoState *isolationState
	if !h.isolationOff {
		isoState = h.head.iso.State(h.mint, h.maxt)
	}
	return h.head.chunksRange(h.mint, h.maxt, isoState)
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

var rangeHeadULID = ulid.MustParse("0000000000XXXXXXXRANGEHEAD")

func (h *RangeHead) Meta() BlockMeta {
	return BlockMeta{
		MinTime: h.MinTime(),
		MaxTime: h.MaxTime(),
		ULID:    rangeHeadULID,
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
func (h *Head) Delete(ctx context.Context, mint, maxt int64, ms ...*labels.Matcher) error {
	// Do not delete anything beyond the currently valid range.
	mint, maxt = clampInterval(mint, maxt, h.MinTime(), h.MaxTime())

	ir := h.indexRange(mint, maxt)

	p, err := PostingsForMatchers(ctx, ir, ms...)
	if err != nil {
		return fmt.Errorf("select series: %w", err)
	}

	var stones []tombstones.Stone
	for p.Next() {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("select series: %w", err)
		}

		series := h.series.getByID(chunks.HeadSeriesRef(p.At()))
		if series == nil {
			level.Debug(h.logger).Log("msg", "Series not found in Head.Delete")
			continue
		}

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
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("select series: %w", err)
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
// It returns
// * The actual min times of the chunks present in the Head.
// * The min OOO time seen during the GC.
// * Min mmap file number seen in the series (in-order and out-of-order) after gc'ing the series.
func (h *Head) gc() (actualInOrderMint, minOOOTime int64, minMmapFile int) {
	// Only data strictly lower than this timestamp must be deleted.
	mint := h.MinTime()
	// Only ooo m-map chunks strictly lower than or equal to this ref
	// must be deleted.
	minOOOMmapRef := chunks.ChunkDiskMapperRef(h.minOOOMmapRef.Load())

	// Drop old chunks and remember series IDs and hashes if they can be
	// deleted entirely.
	deleted, chunksRemoved, actualInOrderMint, minOOOTime, minMmapFile := h.series.gc(mint, minOOOMmapRef)
	seriesRemoved := len(deleted)

	h.metrics.seriesRemoved.Add(float64(seriesRemoved))
	h.metrics.chunksRemoved.Add(float64(chunksRemoved))
	h.metrics.chunks.Sub(float64(chunksRemoved))
	h.numSeries.Sub(uint64(seriesRemoved))

	// Remove deleted series IDs from the postings lists.
	h.postings.Delete(deleted)

	// Remove tombstones referring to the deleted series.
	h.tombstones.DeleteTombstones(deleted)
	h.tombstones.TruncateBefore(mint)

	if h.wal != nil {
		_, last, _ := wlog.Segments(h.wal.Dir())
		h.deletedMtx.Lock()
		// Keep series records until we're past segment 'last'
		// because the WAL will still have samples records with
		// this ref ID. If we didn't keep these series records then
		// on start up when we replay the WAL, or any other code
		// that reads the WAL, wouldn't be able to use those
		// samples since we would have no labels for that ref ID.
		for ref := range deleted {
			h.deleted[chunks.HeadSeriesRef(ref)] = last
		}
		h.deletedMtx.Unlock()
	}

	return actualInOrderMint, minOOOTime, minMmapFile
}

// Tombstones returns a new reader over the head's tombstones.
func (h *Head) Tombstones() (tombstones.Reader, error) {
	return h.tombstones, nil
}

// NumSeries returns the number of active series in the head.
func (h *Head) NumSeries() uint64 {
	return h.numSeries.Load()
}

var headULID = ulid.MustParse("0000000000XXXXXXXXXXXXHEAD")

// Meta returns meta information about the head.
// The head is dynamic so will return dynamic results.
func (h *Head) Meta() BlockMeta {
	return BlockMeta{
		MinTime: h.MinTime(),
		MaxTime: h.MaxTime(),
		ULID:    headULID,
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

// MinOOOTime returns the lowest time bound on visible data in the out of order
// head.
func (h *Head) MinOOOTime() int64 {
	return h.minOOOTime.Load()
}

// MaxOOOTime returns the highest timestamp on visible data in the out of order
// head.
func (h *Head) MaxOOOTime() int64 {
	return h.maxOOOTime.Load()
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

	// mmap all but last chunk in case we're performing snapshot since that only
	// takes samples from most recent head chunk.
	h.mmapHeadChunks()

	errs := tsdb_errors.NewMulti(h.chunkDiskMapper.Close())
	if h.wal != nil {
		errs.Add(h.wal.Close())
	}
	if h.wbl != nil {
		errs.Add(h.wbl.Close())
	}
	if errs.Err() == nil && h.opts.EnableMemorySnapshotOnShutdown {
		errs.Add(h.performChunkSnapshot())
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
	id := chunks.HeadSeriesRef(h.lastSeriesID.Inc())

	return h.getOrCreateWithID(id, hash, lset)
}

func (h *Head) getOrCreateWithID(id chunks.HeadSeriesRef, hash uint64, lset labels.Labels) (*memSeries, bool, error) {
	s, created, err := h.series.getOrSet(hash, lset, func() *memSeries {
		return newMemSeries(lset, id, h.opts.IsolationDisabled)
	})
	if err != nil {
		return nil, false, err
	}
	if !created {
		return s, false, nil
	}

	h.metrics.seriesCreated.Inc()
	h.numSeries.Inc()

	h.postings.Add(storage.SeriesRef(id), lset)
	return s, true, nil
}

// mmapHeadChunks will iterate all memSeries stored on Head and call mmapHeadChunks() on each of them.
//
// There are two types of chunks that store samples for each memSeries:
// A) Head chunk - stored on Go heap, when new samples are appended they go there.
// B) M-mapped chunks - memory mapped chunks, kernel manages the memory for us on-demand, these chunks
//
//	are read-only.
//
// Calling mmapHeadChunks() will iterate all memSeries and m-mmap all chunks that should be m-mapped.
// The m-mapping operation is needs to be serialised and so it goes via central lock.
// If there are multiple concurrent memSeries that need to m-map some chunk then they can block each-other.
//
// To minimise the effect of locking on TSDB operations m-mapping is serialised and done away from
// sample append path, since waiting on a lock inside an append would lock the entire memSeries for
// (potentially) a long time, since that could eventually delay next scrape and/or cause query timeouts.
func (h *Head) mmapHeadChunks() {
	var count int
	for i := 0; i < h.series.size; i++ {
		h.series.locks[i].RLock()
		for _, series := range h.series.series[i] {
			series.Lock()
			count += series.mmapChunks(h.chunkDiskMapper)
			series.Unlock()
		}
		h.series.locks[i].RUnlock()
	}
	h.metrics.mmapChunksTotal.Add(float64(count))
}

// seriesHashmap lets TSDB find a memSeries by its label set, via a 64-bit hash.
// There is one map for the common case where the hash value is unique, and a
// second map for the case that two series have the same hash value.
// Each series is in only one of the maps.
// Its methods require the hash to be submitted with it to avoid re-computations throughout
// the code.
type seriesHashmap struct {
	unique    map[uint64]*memSeries
	conflicts map[uint64][]*memSeries
}

func (m *seriesHashmap) get(hash uint64, lset labels.Labels) *memSeries {
	if s, found := m.unique[hash]; found {
		if labels.Equal(s.lset, lset) {
			return s
		}
	}
	for _, s := range m.conflicts[hash] {
		if labels.Equal(s.lset, lset) {
			return s
		}
	}
	return nil
}

func (m *seriesHashmap) set(hash uint64, s *memSeries) {
	if existing, found := m.unique[hash]; !found || labels.Equal(existing.lset, s.lset) {
		m.unique[hash] = s
		return
	}
	if m.conflicts == nil {
		m.conflicts = make(map[uint64][]*memSeries)
	}
	l := m.conflicts[hash]
	for i, prev := range l {
		if labels.Equal(prev.lset, s.lset) {
			l[i] = s
			return
		}
	}
	m.conflicts[hash] = append(l, s)
}

func (m *seriesHashmap) del(hash uint64, lset labels.Labels) {
	var rem []*memSeries
	unique, found := m.unique[hash]
	switch {
	case !found:
		return
	case labels.Equal(unique.lset, lset):
		conflicts := m.conflicts[hash]
		if len(conflicts) == 0 {
			delete(m.unique, hash)
			return
		}
		rem = conflicts
	default:
		rem = append(rem, unique)
		for _, s := range m.conflicts[hash] {
			if !labels.Equal(s.lset, lset) {
				rem = append(rem, s)
			}
		}
	}
	m.unique[hash] = rem[0]
	if len(rem) == 1 {
		delete(m.conflicts, hash)
	} else {
		m.conflicts[hash] = rem[1:]
	}
}

const (
	// DefaultStripeSize is the default number of entries to allocate in the stripeSeries hash map.
	DefaultStripeSize = 1 << 14
)

// stripeSeries holds series by HeadSeriesRef ("ID") and also by hash of their labels.
// ID-based lookups via getByID() are preferred over getByHash() for performance reasons.
// It locks modulo ranges of IDs and hashes to reduce lock contention.
// The locks are padded to not be on the same cache line. Filling the padded space
// with the maps was profiled to be slower – likely due to the additional pointer
// dereferences.
type stripeSeries struct {
	size                    int
	series                  []map[chunks.HeadSeriesRef]*memSeries // Sharded by ref. A series ref is the value of `size` when the series was being newly added.
	hashes                  []seriesHashmap                       // Sharded by label hash.
	locks                   []stripeLock                          // Sharded by ref for series access, by label hash for hashes access.
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
		series:                  make([]map[chunks.HeadSeriesRef]*memSeries, stripeSize),
		hashes:                  make([]seriesHashmap, stripeSize),
		locks:                   make([]stripeLock, stripeSize),
		seriesLifecycleCallback: seriesCallback,
	}

	for i := range s.series {
		s.series[i] = map[chunks.HeadSeriesRef]*memSeries{}
	}
	for i := range s.hashes {
		s.hashes[i] = seriesHashmap{
			unique:    map[uint64]*memSeries{},
			conflicts: nil, // Initialized on demand in set().
		}
	}
	return s
}

// gc garbage collects old chunks that are strictly before mint and removes
// series entirely that have no chunks left.
// note: returning map[chunks.HeadSeriesRef]struct{} would be more accurate,
// but the returned map goes into postings.Delete() which expects a map[storage.SeriesRef]struct
// and there's no easy way to cast maps.
// minMmapFile is the min mmap file number seen in the series (in-order and out-of-order) after gc'ing the series.
func (s *stripeSeries) gc(mint int64, minOOOMmapRef chunks.ChunkDiskMapperRef) (_ map[storage.SeriesRef]struct{}, _ int, _, _ int64, minMmapFile int) {
	var (
		deleted                     = map[storage.SeriesRef]struct{}{}
		rmChunks                    = 0
		actualMint            int64 = math.MaxInt64
		minOOOTime            int64 = math.MaxInt64
		deletedFromPrevStripe       = 0
	)
	minMmapFile = math.MaxInt32

	// For one series, truncate old chunks and check if any chunks left. If not, mark as deleted and collect the ID.
	check := func(hashShard int, hash uint64, series *memSeries, deletedForCallback map[chunks.HeadSeriesRef]labels.Labels) {
		series.Lock()
		defer series.Unlock()

		rmChunks += series.truncateChunksBefore(mint, minOOOMmapRef)

		if len(series.mmappedChunks) > 0 {
			seq, _ := series.mmappedChunks[0].ref.Unpack()
			if seq < minMmapFile {
				minMmapFile = seq
			}
		}
		if series.ooo != nil && len(series.ooo.oooMmappedChunks) > 0 {
			seq, _ := series.ooo.oooMmappedChunks[0].ref.Unpack()
			if seq < minMmapFile {
				minMmapFile = seq
			}
			for _, ch := range series.ooo.oooMmappedChunks {
				if ch.minTime < minOOOTime {
					minOOOTime = ch.minTime
				}
			}
		}
		if series.ooo != nil && series.ooo.oooHeadChunk != nil {
			if series.ooo.oooHeadChunk.minTime < minOOOTime {
				minOOOTime = series.ooo.oooHeadChunk.minTime
			}
		}
		if len(series.mmappedChunks) > 0 || series.headChunks != nil || series.pendingCommit ||
			(series.ooo != nil && (len(series.ooo.oooMmappedChunks) > 0 || series.ooo.oooHeadChunk != nil)) {
			seriesMint := series.minTime()
			if seriesMint < actualMint {
				actualMint = seriesMint
			}
			return
		}
		// The series is gone entirely. We need to keep the series lock
		// and make sure we have acquired the stripe locks for hash and ID of the
		// series alike.
		// If we don't hold them all, there's a very small chance that a series receives
		// samples again while we are half-way into deleting it.
		refShard := int(series.ref) & (s.size - 1)
		if hashShard != refShard {
			s.locks[refShard].Lock()
			defer s.locks[refShard].Unlock()
		}

		deleted[storage.SeriesRef(series.ref)] = struct{}{}
		s.hashes[hashShard].del(hash, series.lset)
		delete(s.series[refShard], series.ref)
		deletedForCallback[series.ref] = series.lset
	}

	// Run through all series shard by shard, checking which should be deleted.
	for i := 0; i < s.size; i++ {
		deletedForCallback := make(map[chunks.HeadSeriesRef]labels.Labels, deletedFromPrevStripe)
		s.locks[i].Lock()

		// Delete conflicts first so seriesHashmap.del doesn't move them to the `unique` field,
		// after deleting `unique`.
		for hash, all := range s.hashes[i].conflicts {
			for _, series := range all {
				check(i, hash, series, deletedForCallback)
			}
		}
		for hash, series := range s.hashes[i].unique {
			check(i, hash, series, deletedForCallback)
		}

		s.locks[i].Unlock()

		s.seriesLifecycleCallback.PostDeletion(deletedForCallback)
		deletedFromPrevStripe = len(deletedForCallback)
	}

	if actualMint == math.MaxInt64 {
		actualMint = mint
	}

	return deleted, rmChunks, actualMint, minOOOTime, minMmapFile
}

func (s *stripeSeries) getByID(id chunks.HeadSeriesRef) *memSeries {
	i := uint64(id) & uint64(s.size-1)

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

	i = uint64(series.ref) & uint64(s.size-1)

	s.locks[i].Lock()
	s.series[i][series.ref] = series
	s.locks[i].Unlock()

	return series, true, nil
}

type sample struct {
	t  int64
	f  float64
	h  *histogram.Histogram
	fh *histogram.FloatHistogram
}

func newSample(t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram) chunks.Sample {
	return sample{t, v, h, fh}
}

func (s sample) T() int64                      { return s.t }
func (s sample) F() float64                    { return s.f }
func (s sample) H() *histogram.Histogram       { return s.h }
func (s sample) FH() *histogram.FloatHistogram { return s.fh }

func (s sample) Type() chunkenc.ValueType {
	switch {
	case s.h != nil:
		return chunkenc.ValHistogram
	case s.fh != nil:
		return chunkenc.ValFloatHistogram
	default:
		return chunkenc.ValFloat
	}
}

// memSeries is the in-memory representation of a series. None of its methods
// are goroutine safe and it is the caller's responsibility to lock it.
type memSeries struct {
	sync.RWMutex

	ref  chunks.HeadSeriesRef
	lset labels.Labels
	meta *metadata.Metadata

	// Immutable chunks on disk that have not yet gone into a block, in order of ascending time stamps.
	// When compaction runs, chunks get moved into a block and all pointers are shifted like so:
	//
	//                                    /------- let's say these 2 chunks get stored into a block
	//                                    |  |
	// before compaction: mmappedChunks=[p5,p6,p7,p8,p9] firstChunkID=5
	//  after compaction: mmappedChunks=[p7,p8,p9]       firstChunkID=7
	//
	// pN is the pointer to the mmappedChunk referered to by HeadChunkID=N
	mmappedChunks []*mmappedChunk
	// Most recent chunks in memory that are still being built or waiting to be mmapped.
	// This is a linked list, headChunks points to the most recent chunk, headChunks.next points
	// to older chunk and so on.
	headChunks   *memChunk
	firstChunkID chunks.HeadChunkID // HeadChunkID for mmappedChunks[0]

	ooo *memSeriesOOOFields

	mmMaxTime int64 // Max time of any mmapped chunk, only used during WAL replay.

	nextAt                           int64 // Timestamp at which to cut the next chunk.
	histogramChunkHasComputedEndTime bool  // True if nextAt has been predicted for the current histograms chunk; false otherwise.

	// We keep the last value here (in addition to appending it to the chunk) so we can check for duplicates.
	lastValue float64

	// We keep the last histogram value here (in addition to appending it to the chunk) so we can check for duplicates.
	lastHistogramValue      *histogram.Histogram
	lastFloatHistogramValue *histogram.FloatHistogram

	// Current appender for the head chunk. Set when a new head chunk is cut.
	// It is nil only if headChunks is nil. E.g. if there was an appender that created a new series, but rolled back the commit
	// (the first sample would create a headChunk, hence appender, but rollback skipped it while the Append() call would create a series).
	app chunkenc.Appender

	// txs is nil if isolation is disabled.
	txs *txRing

	pendingCommit bool // Whether there are samples waiting to be committed to this series.
}

// memSeriesOOOFields contains the fields required by memSeries
// to handle out-of-order data.
type memSeriesOOOFields struct {
	oooMmappedChunks []*mmappedChunk    // Immutable chunks on disk containing OOO samples.
	oooHeadChunk     *oooHeadChunk      // Most recent chunk for ooo samples in memory that's still being built.
	firstOOOChunkID  chunks.HeadChunkID // HeadOOOChunkID for oooMmappedChunks[0].
}

func newMemSeries(lset labels.Labels, id chunks.HeadSeriesRef, isolationDisabled bool) *memSeries {
	s := &memSeries{
		lset:   lset,
		ref:    id,
		nextAt: math.MinInt64,
	}
	if !isolationDisabled {
		s.txs = newTxRing(4)
	}
	return s
}

func (s *memSeries) minTime() int64 {
	if len(s.mmappedChunks) > 0 {
		return s.mmappedChunks[0].minTime
	}
	if s.headChunks != nil {
		return s.headChunks.oldest().minTime
	}
	return math.MinInt64
}

func (s *memSeries) maxTime() int64 {
	// The highest timestamps will always be in the regular (non-OOO) chunks, even if OOO is enabled.
	if s.headChunks != nil {
		return s.headChunks.maxTime
	}
	if len(s.mmappedChunks) > 0 {
		return s.mmappedChunks[len(s.mmappedChunks)-1].maxTime
	}
	return math.MinInt64
}

// truncateChunksBefore removes all chunks from the series that
// have no timestamp at or after mint.
// Chunk IDs remain unchanged.
func (s *memSeries) truncateChunksBefore(mint int64, minOOOMmapRef chunks.ChunkDiskMapperRef) int {
	var removedInOrder int
	if s.headChunks != nil {
		var i int
		var nextChk *memChunk
		chk := s.headChunks
		for chk != nil {
			if chk.maxTime < mint {
				// If any head chunk is truncated, we can truncate all mmapped chunks.
				removedInOrder = chk.len() + len(s.mmappedChunks)
				s.firstChunkID += chunks.HeadChunkID(removedInOrder)
				if i == 0 {
					// This is the first chunk on the list so we need to remove the entire list.
					s.headChunks = nil
				} else {
					// This is NOT the first chunk, unlink it from parent.
					nextChk.prev = nil
				}
				s.mmappedChunks = nil
				break
			}
			nextChk = chk
			chk = chk.prev
			i++
		}
	}
	if len(s.mmappedChunks) > 0 {
		for i, c := range s.mmappedChunks {
			if c.maxTime >= mint {
				break
			}
			removedInOrder = i + 1
		}
		s.mmappedChunks = append(s.mmappedChunks[:0], s.mmappedChunks[removedInOrder:]...)
		s.firstChunkID += chunks.HeadChunkID(removedInOrder)
	}

	var removedOOO int
	if s.ooo != nil && len(s.ooo.oooMmappedChunks) > 0 {
		for i, c := range s.ooo.oooMmappedChunks {
			if c.ref.GreaterThan(minOOOMmapRef) {
				break
			}
			removedOOO = i + 1
		}
		s.ooo.oooMmappedChunks = append(s.ooo.oooMmappedChunks[:0], s.ooo.oooMmappedChunks[removedOOO:]...)
		s.ooo.firstOOOChunkID += chunks.HeadChunkID(removedOOO)

		if len(s.ooo.oooMmappedChunks) == 0 && s.ooo.oooHeadChunk == nil {
			s.ooo = nil
		}
	}

	return removedInOrder + removedOOO
}

// cleanupAppendIDsBelow cleans up older appendIDs. Has to be called after
// acquiring lock.
func (s *memSeries) cleanupAppendIDsBelow(bound uint64) {
	if s.txs != nil {
		s.txs.cleanupAppendIDsBelow(bound)
	}
}

type memChunk struct {
	chunk            chunkenc.Chunk
	minTime, maxTime int64
	prev             *memChunk // Link to the previous element on the list.
}

// len returns the length of memChunk list, including the element it was called on.
func (mc *memChunk) len() (count int) {
	elem := mc
	for elem != nil {
		count++
		elem = elem.prev
	}
	return count
}

// oldest returns the oldest element on the list.
// For single element list this will be the same memChunk oldest() was called on.
func (mc *memChunk) oldest() (elem *memChunk) {
	elem = mc
	for elem.prev != nil {
		elem = elem.prev
	}
	return elem
}

// atOffset returns a memChunk that's Nth element on the linked list.
func (mc *memChunk) atOffset(offset int) (elem *memChunk) {
	if offset == 0 {
		return mc
	}
	if offset < 0 {
		return nil
	}

	var i int
	elem = mc
	for i < offset {
		i++
		elem = elem.prev
		if elem == nil {
			break
		}
	}

	return elem
}

type oooHeadChunk struct {
	chunk            *OOOChunk
	minTime, maxTime int64 // can probably be removed and pulled out of the chunk instead
}

// OverlapsClosedInterval returns true if the chunk overlaps [mint, maxt].
func (mc *oooHeadChunk) OverlapsClosedInterval(mint, maxt int64) bool {
	return overlapsClosedInterval(mc.minTime, mc.maxTime, mint, maxt)
}

// OverlapsClosedInterval returns true if the chunk overlaps [mint, maxt].
func (mc *memChunk) OverlapsClosedInterval(mint, maxt int64) bool {
	return overlapsClosedInterval(mc.minTime, mc.maxTime, mint, maxt)
}

func overlapsClosedInterval(mint1, maxt1, mint2, maxt2 int64) bool {
	return mint1 <= maxt2 && mint2 <= maxt1
}

// mmappedChunk describes a head chunk on disk that has been mmapped.
type mmappedChunk struct {
	ref              chunks.ChunkDiskMapperRef
	numSamples       uint16
	minTime, maxTime int64
}

// Returns true if the chunk overlaps [mint, maxt].
func (mc *mmappedChunk) OverlapsClosedInterval(mint, maxt int64) bool {
	return overlapsClosedInterval(mc.minTime, mc.maxTime, mint, maxt)
}

type noopSeriesLifecycleCallback struct{}

func (noopSeriesLifecycleCallback) PreCreation(labels.Labels) error                     { return nil }
func (noopSeriesLifecycleCallback) PostCreation(labels.Labels)                          {}
func (noopSeriesLifecycleCallback) PostDeletion(map[chunks.HeadSeriesRef]labels.Labels) {}

func (h *Head) Size() int64 {
	var walSize, wblSize int64
	if h.wal != nil {
		walSize, _ = h.wal.Size()
	}
	if h.wbl != nil {
		wblSize, _ = h.wbl.Size()
	}
	cdmSize, _ := h.chunkDiskMapper.Size()
	return walSize + wblSize + cdmSize
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
