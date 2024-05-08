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

// Package tsdb implements a time series storage for float64 sample data.
package tsdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	_ "github.com/prometheus/prometheus/tsdb/goversion" // Load the package into main to make sure minimum Go version is met.
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

const (
	// Default duration of a block in milliseconds.
	DefaultBlockDuration = int64(2 * time.Hour / time.Millisecond)

	// Block dir suffixes to make deletion and creation operations atomic.
	// We decided to do suffixes instead of creating meta.json as last (or delete as first) one,
	// because in error case you still can recover meta.json from the block content within local TSDB dir.
	// TODO(bwplotka): TSDB can end up with various .tmp files (e.g meta.json.tmp, WAL or segment tmp file. Think
	// about removing those too on start to save space. Currently only blocks tmp dirs are removed.
	tmpForDeletionBlockDirSuffix = ".tmp-for-deletion"
	tmpForCreationBlockDirSuffix = ".tmp-for-creation"
	// Pre-2.21 tmp dir suffix, used in clean-up functions.
	tmpLegacy = ".tmp"
)

// ErrNotReady is returned if the underlying storage is not ready yet.
var ErrNotReady = errors.New("TSDB not ready")

// DefaultOptions used for the DB. They are reasonable for setups using
// millisecond precision timestamps.
func DefaultOptions() *Options {
	return &Options{
		WALSegmentSize:              wlog.DefaultSegmentSize,
		MaxBlockChunkSegmentSize:    chunks.DefaultChunkSegmentSize,
		RetentionDuration:           int64(15 * 24 * time.Hour / time.Millisecond),
		MinBlockDuration:            DefaultBlockDuration,
		MaxBlockDuration:            DefaultBlockDuration,
		NoLockfile:                  false,
		SamplesPerChunk:             DefaultSamplesPerChunk,
		WALCompression:              wlog.CompressionNone,
		StripeSize:                  DefaultStripeSize,
		HeadChunksWriteBufferSize:   chunks.DefaultWriteBufferSize,
		IsolationDisabled:           defaultIsolationDisabled,
		HeadChunksWriteQueueSize:    chunks.DefaultWriteQueueSize,
		OutOfOrderCapMax:            DefaultOutOfOrderCapMax,
		EnableOverlappingCompaction: true,
		EnableSharding:              false,
	}
}

// Options of the DB storage.
type Options struct {
	// Segments (wal files) max size.
	// WALSegmentSize = 0, segment size is default size.
	// WALSegmentSize > 0, segment size is WALSegmentSize.
	// WALSegmentSize < 0, wal is disabled.
	WALSegmentSize int

	// MaxBlockChunkSegmentSize is the max size of block chunk segment files.
	// MaxBlockChunkSegmentSize = 0, chunk segment size is default size.
	// MaxBlockChunkSegmentSize > 0, chunk segment size is MaxBlockChunkSegmentSize.
	MaxBlockChunkSegmentSize int64

	// Duration of persisted data to keep.
	// Unit agnostic as long as unit is consistent with MinBlockDuration and MaxBlockDuration.
	// Typically it is in milliseconds.
	RetentionDuration int64

	// Maximum number of bytes in blocks to be retained.
	// 0 or less means disabled.
	// NOTE: For proper storage calculations need to consider
	// the size of the WAL folder which is not added when calculating
	// the current size of the database.
	MaxBytes int64

	// NoLockfile disables creation and consideration of a lock file.
	NoLockfile bool

	// WALCompression configures the compression type to use on records in the WAL.
	WALCompression wlog.CompressionType

	// Maximum number of CPUs that can simultaneously processes WAL replay.
	// If it is <=0, then GOMAXPROCS is used.
	WALReplayConcurrency int

	// StripeSize is the size in entries of the series hash map. Reducing the size will save memory but impact performance.
	StripeSize int

	// The timestamp range of head blocks after which they get persisted.
	// It's the minimum duration of any persisted block.
	// Unit agnostic as long as unit is consistent with RetentionDuration and MaxBlockDuration.
	// Typically it is in milliseconds.
	MinBlockDuration int64

	// The maximum timestamp range of compacted blocks.
	// Unit agnostic as long as unit is consistent with MinBlockDuration and RetentionDuration.
	// Typically it is in milliseconds.
	MaxBlockDuration int64

	// HeadChunksWriteBufferSize configures the write buffer size used by the head chunks mapper.
	HeadChunksWriteBufferSize int

	// HeadChunksWriteQueueSize configures the size of the chunk write queue used in the head chunks mapper.
	HeadChunksWriteQueueSize int

	// SamplesPerChunk configures the target number of samples per chunk.
	SamplesPerChunk int

	// SeriesLifecycleCallback specifies a list of callbacks that will be called during a lifecycle of a series.
	// It is always a no-op in Prometheus and mainly meant for external users who import TSDB.
	SeriesLifecycleCallback SeriesLifecycleCallback

	// BlocksToDelete is a function which returns the blocks which can be deleted.
	// It is always the default time and size based retention in Prometheus and
	// mainly meant for external users who import TSDB.
	BlocksToDelete BlocksToDeleteFunc

	// Enables the in memory exemplar storage.
	EnableExemplarStorage bool

	// Enables the snapshot of in-memory chunks on shutdown. This makes restarts faster.
	EnableMemorySnapshotOnShutdown bool

	// MaxExemplars sets the size, in # of exemplars stored, of the single circular buffer used to store exemplars in memory.
	// See tsdb/exemplar.go, specifically the CircularExemplarStorage struct and it's constructor NewCircularExemplarStorage.
	MaxExemplars int64

	// Disables isolation between reads and in-flight appends.
	IsolationDisabled bool

	// EnableNativeHistograms enables the ingestion of native histograms.
	EnableNativeHistograms bool

	// OutOfOrderTimeWindow specifies how much out of order is allowed, if any.
	// This can change during run-time, so this value from here should only be used
	// while initialising.
	OutOfOrderTimeWindow int64

	// OutOfOrderCapMax is maximum capacity for OOO chunks (in samples).
	// If it is <=0, the default value is assumed.
	OutOfOrderCapMax int64

	// Compaction of overlapping blocks are allowed if EnableOverlappingCompaction is true.
	// This is an optional flag for overlapping blocks.
	// The reason why this flag exists is because there are various users of the TSDB
	// that do not want vertical compaction happening on ingest time. Instead,
	// they'd rather keep overlapping blocks and let another component do the overlapping compaction later.
	// For Prometheus, this will always be true.
	EnableOverlappingCompaction bool

	// EnableSharding enables query sharding support in TSDB.
	EnableSharding bool
}

type BlocksToDeleteFunc func(blocks []*Block) map[ulid.ULID]struct{}

// DB handles reads and writes of time series falling into
// a hashed partition of a seriedb.
type DB struct {
	dir    string
	locker *tsdbutil.DirLocker

	logger         log.Logger
	metrics        *dbMetrics
	opts           *Options
	chunkPool      chunkenc.Pool
	compactor      Compactor
	blocksToDelete BlocksToDeleteFunc

	// mtx must be held when modifying the general block layout or lastGarbageCollectedMmapRef.
	mtx    sync.RWMutex
	blocks []*Block

	// The last OOO chunk that was compacted and written to disk. New queriers must not read chunks less
	// than or equal to this reference, as these chunks could be garbage collected at any time.
	lastGarbageCollectedMmapRef chunks.ChunkDiskMapperRef

	head *Head

	compactc chan struct{}
	donec    chan struct{}
	stopc    chan struct{}

	// cmtx ensures that compactions and deletions don't run simultaneously.
	cmtx sync.Mutex

	// autoCompactMtx ensures that no compaction gets triggered while
	// changing the autoCompact var.
	autoCompactMtx sync.Mutex
	autoCompact    bool

	// Cancel a running compaction when a shutdown is initiated.
	compactCancel context.CancelFunc

	// oooWasEnabled is true if out of order support was enabled at least one time
	// during the time TSDB was up. In which case we need to keep supporting
	// out-of-order compaction and vertical queries.
	oooWasEnabled atomic.Bool

	writeNotified wlog.WriteNotified

	registerer prometheus.Registerer
}

type dbMetrics struct {
	loadedBlocks         prometheus.GaugeFunc
	symbolTableSize      prometheus.GaugeFunc
	reloads              prometheus.Counter
	reloadsFailed        prometheus.Counter
	compactionsFailed    prometheus.Counter
	compactionsTriggered prometheus.Counter
	compactionsSkipped   prometheus.Counter
	sizeRetentionCount   prometheus.Counter
	timeRetentionCount   prometheus.Counter
	startTime            prometheus.GaugeFunc
	tombCleanTimer       prometheus.Histogram
	blocksBytes          prometheus.Gauge
	maxBytes             prometheus.Gauge
	retentionDuration    prometheus.Gauge
}

func newDBMetrics(db *DB, r prometheus.Registerer) *dbMetrics {
	m := &dbMetrics{}

	m.loadedBlocks = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_blocks_loaded",
		Help: "Number of currently loaded data blocks",
	}, func() float64 {
		db.mtx.RLock()
		defer db.mtx.RUnlock()
		return float64(len(db.blocks))
	})
	m.symbolTableSize = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_symbol_table_size_bytes",
		Help: "Size of symbol table in memory for loaded blocks",
	}, func() float64 {
		db.mtx.RLock()
		blocks := db.blocks
		db.mtx.RUnlock()
		symTblSize := uint64(0)
		for _, b := range blocks {
			symTblSize += b.GetSymbolTableSize()
		}
		return float64(symTblSize)
	})
	m.reloads = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_reloads_total",
		Help: "Number of times the database reloaded block data from disk.",
	})
	m.reloadsFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_reloads_failures_total",
		Help: "Number of times the database failed to reloadBlocks block data from disk.",
	})
	m.compactionsTriggered = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_compactions_triggered_total",
		Help: "Total number of triggered compactions for the partition.",
	})
	m.compactionsFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_compactions_failed_total",
		Help: "Total number of compactions that failed for the partition.",
	})
	m.timeRetentionCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_time_retentions_total",
		Help: "The number of times that blocks were deleted because the maximum time limit was exceeded.",
	})
	m.compactionsSkipped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_compactions_skipped_total",
		Help: "Total number of skipped compactions due to disabled auto compaction.",
	})
	m.startTime = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_lowest_timestamp",
		Help: "Lowest timestamp value stored in the database. The unit is decided by the library consumer.",
	}, func() float64 {
		db.mtx.RLock()
		defer db.mtx.RUnlock()
		if len(db.blocks) == 0 {
			return float64(db.head.MinTime())
		}
		return float64(db.blocks[0].meta.MinTime)
	})
	m.tombCleanTimer = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:                            "prometheus_tsdb_tombstone_cleanup_seconds",
		Help:                            "The time taken to recompact blocks to remove tombstones.",
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  100,
		NativeHistogramMinResetDuration: 1 * time.Hour,
	})
	m.blocksBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_storage_blocks_bytes",
		Help: "The number of bytes that are currently used for local storage by all blocks.",
	})
	m.maxBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_retention_limit_bytes",
		Help: "Max number of bytes to be retained in the tsdb blocks, configured 0 means disabled",
	})
	m.retentionDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_retention_limit_seconds",
		Help: "How long to retain samples in storage.",
	})
	m.sizeRetentionCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_size_retentions_total",
		Help: "The number of times that blocks were deleted because the maximum number of bytes was exceeded.",
	})

	if r != nil {
		r.MustRegister(
			m.loadedBlocks,
			m.symbolTableSize,
			m.reloads,
			m.reloadsFailed,
			m.compactionsFailed,
			m.compactionsTriggered,
			m.compactionsSkipped,
			m.sizeRetentionCount,
			m.timeRetentionCount,
			m.startTime,
			m.tombCleanTimer,
			m.blocksBytes,
			m.maxBytes,
			m.retentionDuration,
		)
	}
	return m
}

// DBStats contains statistics about the DB separated by component (eg. head).
// They are available before the DB has finished initializing.
type DBStats struct {
	Head *HeadStats
}

// NewDBStats returns a new DBStats object initialized using the
// new function from each component.
func NewDBStats() *DBStats {
	return &DBStats{
		Head: NewHeadStats(),
	}
}

// ErrClosed is returned when the db is closed.
var ErrClosed = errors.New("db already closed")

// DBReadOnly provides APIs for read only operations on a database.
// Current implementation doesn't support concurrency so
// all API calls should happen in the same go routine.
type DBReadOnly struct {
	logger  log.Logger
	dir     string
	closers []io.Closer
	closed  chan struct{}
}

// OpenDBReadOnly opens DB in the given directory for read only operations.
func OpenDBReadOnly(dir string, l log.Logger) (*DBReadOnly, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("opening the db dir: %w", err)
	}

	if l == nil {
		l = log.NewNopLogger()
	}

	return &DBReadOnly{
		logger: l,
		dir:    dir,
		closed: make(chan struct{}),
	}, nil
}

// FlushWAL creates a new block containing all data that's currently in the memory buffer/WAL.
// Samples that are in existing blocks will not be written to the new block.
// Note that if the read only database is running concurrently with a
// writable database then writing the WAL to the database directory can race.
func (db *DBReadOnly) FlushWAL(dir string) (returnErr error) {
	blockReaders, err := db.Blocks()
	if err != nil {
		return fmt.Errorf("read blocks: %w", err)
	}
	maxBlockTime := int64(math.MinInt64)
	if len(blockReaders) > 0 {
		maxBlockTime = blockReaders[len(blockReaders)-1].Meta().MaxTime
	}
	w, err := wlog.Open(db.logger, filepath.Join(db.dir, "wal"))
	if err != nil {
		return err
	}
	var wbl *wlog.WL
	wblDir := filepath.Join(db.dir, wlog.WblDirName)
	if _, err := os.Stat(wblDir); !os.IsNotExist(err) {
		wbl, err = wlog.Open(db.logger, wblDir)
		if err != nil {
			return err
		}
	}
	opts := DefaultHeadOptions()
	opts.ChunkDirRoot = db.dir
	head, err := NewHead(nil, db.logger, w, wbl, opts, NewHeadStats())
	if err != nil {
		return err
	}
	defer func() {
		errs := tsdb_errors.NewMulti(returnErr)
		if err := head.Close(); err != nil {
			errs.Add(fmt.Errorf("closing Head: %w", err))
		}
		returnErr = errs.Err()
	}()
	// Set the min valid time for the ingested wal samples
	// to be no lower than the maxt of the last block.
	if err := head.Init(maxBlockTime); err != nil {
		return fmt.Errorf("read WAL: %w", err)
	}
	mint := head.MinTime()
	maxt := head.MaxTime()
	rh := NewRangeHead(head, mint, maxt)
	compactor, err := NewLeveledCompactor(
		context.Background(),
		nil,
		db.logger,
		ExponentialBlockRanges(DefaultOptions().MinBlockDuration, 3, 5),
		chunkenc.NewPool(), nil,
	)
	if err != nil {
		return fmt.Errorf("create leveled compactor: %w", err)
	}
	// Add +1 millisecond to block maxt because block intervals are half-open: [b.MinTime, b.MaxTime).
	// Because of this block intervals are always +1 than the total samples it includes.
	_, err = compactor.Write(dir, rh, mint, maxt+1, nil)
	if err != nil {
		return fmt.Errorf("writing WAL: %w", err)
	}
	return nil
}

func (db *DBReadOnly) loadDataAsQueryable(maxt int64) (storage.SampleAndChunkQueryable, error) {
	select {
	case <-db.closed:
		return nil, ErrClosed
	default:
	}
	blockReaders, err := db.Blocks()
	if err != nil {
		return nil, err
	}
	blocks := make([]*Block, len(blockReaders))
	for i, b := range blockReaders {
		b, ok := b.(*Block)
		if !ok {
			return nil, errors.New("unable to convert a read only block to a normal block")
		}
		blocks[i] = b
	}

	opts := DefaultHeadOptions()
	opts.ChunkDirRoot = db.dir
	head, err := NewHead(nil, db.logger, nil, nil, opts, NewHeadStats())
	if err != nil {
		return nil, err
	}
	maxBlockTime := int64(math.MinInt64)
	if len(blocks) > 0 {
		maxBlockTime = blocks[len(blocks)-1].Meta().MaxTime
	}

	// Also add the WAL if the current blocks don't cover the requests time range.
	if maxBlockTime <= maxt {
		if err := head.Close(); err != nil {
			return nil, err
		}
		w, err := wlog.Open(db.logger, filepath.Join(db.dir, "wal"))
		if err != nil {
			return nil, err
		}
		var wbl *wlog.WL
		wblDir := filepath.Join(db.dir, wlog.WblDirName)
		if _, err := os.Stat(wblDir); !os.IsNotExist(err) {
			wbl, err = wlog.Open(db.logger, wblDir)
			if err != nil {
				return nil, err
			}
		}
		opts := DefaultHeadOptions()
		opts.ChunkDirRoot = db.dir
		head, err = NewHead(nil, db.logger, w, wbl, opts, NewHeadStats())
		if err != nil {
			return nil, err
		}
		// Set the min valid time for the ingested wal samples
		// to be no lower than the maxt of the last block.
		if err := head.Init(maxBlockTime); err != nil {
			return nil, fmt.Errorf("read WAL: %w", err)
		}
		// Set the wal and the wbl to nil to disable related operations.
		// This is mainly to avoid blocking when closing the head.
		head.wal = nil
		head.wbl = nil
	}

	db.closers = append(db.closers, head)
	return &DB{
		dir:    db.dir,
		logger: db.logger,
		blocks: blocks,
		head:   head,
	}, nil
}

// Querier loads the blocks and wal and returns a new querier over the data partition for the given time range.
// Current implementation doesn't support multiple Queriers.
func (db *DBReadOnly) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := db.loadDataAsQueryable(maxt)
	if err != nil {
		return nil, err
	}
	return q.Querier(mint, maxt)
}

// ChunkQuerier loads blocks and the wal and returns a new chunk querier over the data partition for the given time range.
// Current implementation doesn't support multiple ChunkQueriers.
func (db *DBReadOnly) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	q, err := db.loadDataAsQueryable(maxt)
	if err != nil {
		return nil, err
	}
	return q.ChunkQuerier(mint, maxt)
}

// Blocks returns a slice of block readers for persisted blocks.
func (db *DBReadOnly) Blocks() ([]BlockReader, error) {
	select {
	case <-db.closed:
		return nil, ErrClosed
	default:
	}
	loadable, corrupted, err := openBlocks(db.logger, db.dir, nil, nil)
	if err != nil {
		return nil, err
	}

	// Corrupted blocks that have been superseded by a loadable block can be safely ignored.
	for _, block := range loadable {
		for _, b := range block.Meta().Compaction.Parents {
			delete(corrupted, b.ULID)
		}
	}
	if len(corrupted) > 0 {
		for _, b := range loadable {
			if err := b.Close(); err != nil {
				level.Warn(db.logger).Log("msg", "Closing block failed", "err", err, "block", b)
			}
		}
		errs := tsdb_errors.NewMulti()
		for ulid, err := range corrupted {
			if err != nil {
				errs.Add(fmt.Errorf("corrupted block %s: %w", ulid.String(), err))
			}
		}
		return nil, errs.Err()
	}

	if len(loadable) == 0 {
		return nil, nil
	}

	slices.SortFunc(loadable, func(a, b *Block) int {
		switch {
		case a.Meta().MinTime < b.Meta().MinTime:
			return -1
		case a.Meta().MinTime > b.Meta().MinTime:
			return 1
		default:
			return 0
		}
	})

	blockMetas := make([]BlockMeta, 0, len(loadable))
	for _, b := range loadable {
		blockMetas = append(blockMetas, b.Meta())
	}
	if overlaps := OverlappingBlocks(blockMetas); len(overlaps) > 0 {
		level.Warn(db.logger).Log("msg", "Overlapping blocks found during opening", "detail", overlaps.String())
	}

	// Close all previously open readers and add the new ones to the cache.
	for _, closer := range db.closers {
		closer.Close()
	}

	blockClosers := make([]io.Closer, len(loadable))
	blockReaders := make([]BlockReader, len(loadable))
	for i, b := range loadable {
		blockClosers[i] = b
		blockReaders[i] = b
	}
	db.closers = blockClosers

	return blockReaders, nil
}

// LastBlockID returns the BlockID of latest block.
func (db *DBReadOnly) LastBlockID() (string, error) {
	entries, err := os.ReadDir(db.dir)
	if err != nil {
		return "", err
	}

	max := uint64(0)

	lastBlockID := ""

	for _, e := range entries {
		// Check if dir is a block dir or not.
		dirName := e.Name()
		ulidObj, err := ulid.ParseStrict(dirName)
		if err != nil {
			continue // Not a block dir.
		}
		timestamp := ulidObj.Time()
		if timestamp > max {
			max = timestamp
			lastBlockID = dirName
		}
	}

	if lastBlockID == "" {
		return "", errors.New("no blocks found")
	}

	return lastBlockID, nil
}

// Block returns a block reader by given block id.
func (db *DBReadOnly) Block(blockID string) (BlockReader, error) {
	select {
	case <-db.closed:
		return nil, ErrClosed
	default:
	}

	_, err := os.Stat(filepath.Join(db.dir, blockID))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("invalid block ID %s", blockID)
	}

	block, err := OpenBlock(db.logger, filepath.Join(db.dir, blockID), nil)
	if err != nil {
		return nil, err
	}
	db.closers = append(db.closers, block)

	return block, nil
}

// Close all block readers.
func (db *DBReadOnly) Close() error {
	select {
	case <-db.closed:
		return ErrClosed
	default:
	}
	close(db.closed)

	return tsdb_errors.CloseAll(db.closers)
}

// Open returns a new DB in the given directory. If options are empty, DefaultOptions will be used.
func Open(dir string, l log.Logger, r prometheus.Registerer, opts *Options, stats *DBStats) (db *DB, err error) {
	var rngs []int64
	opts, rngs = validateOpts(opts, nil)

	return open(dir, l, r, opts, rngs, stats)
}

func validateOpts(opts *Options, rngs []int64) (*Options, []int64) {
	if opts == nil {
		opts = DefaultOptions()
	}
	if opts.StripeSize <= 0 {
		opts.StripeSize = DefaultStripeSize
	}
	if opts.HeadChunksWriteBufferSize <= 0 {
		opts.HeadChunksWriteBufferSize = chunks.DefaultWriteBufferSize
	}
	if opts.HeadChunksWriteQueueSize < 0 {
		opts.HeadChunksWriteQueueSize = chunks.DefaultWriteQueueSize
	}
	if opts.SamplesPerChunk <= 0 {
		opts.SamplesPerChunk = DefaultSamplesPerChunk
	}
	if opts.MaxBlockChunkSegmentSize <= 0 {
		opts.MaxBlockChunkSegmentSize = chunks.DefaultChunkSegmentSize
	}
	if opts.MinBlockDuration <= 0 {
		opts.MinBlockDuration = DefaultBlockDuration
	}
	if opts.MinBlockDuration > opts.MaxBlockDuration {
		opts.MaxBlockDuration = opts.MinBlockDuration
	}
	if opts.OutOfOrderCapMax <= 0 {
		opts.OutOfOrderCapMax = DefaultOutOfOrderCapMax
	}
	if opts.OutOfOrderTimeWindow < 0 {
		opts.OutOfOrderTimeWindow = 0
	}

	if len(rngs) == 0 {
		// Start with smallest block duration and create exponential buckets until the exceed the
		// configured maximum block duration.
		rngs = ExponentialBlockRanges(opts.MinBlockDuration, 10, 3)
	}
	return opts, rngs
}

// open returns a new DB in the given directory.
// It initializes the lockfile, WAL, compactor, and Head (by replaying the WAL), and runs the database.
// It is not safe to open more than one DB in the same directory.
func open(dir string, l log.Logger, r prometheus.Registerer, opts *Options, rngs []int64, stats *DBStats) (_ *DB, returnedErr error) {
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return nil, err
	}
	if l == nil {
		l = log.NewNopLogger()
	}
	if stats == nil {
		stats = NewDBStats()
	}

	for i, v := range rngs {
		if v > opts.MaxBlockDuration {
			rngs = rngs[:i]
			break
		}
	}

	// Fixup bad format written by Prometheus 2.1.
	if err := repairBadIndexVersion(l, dir); err != nil {
		return nil, fmt.Errorf("repair bad index version: %w", err)
	}

	walDir := filepath.Join(dir, "wal")
	wblDir := filepath.Join(dir, wlog.WblDirName)

	for _, tmpDir := range []string{walDir, dir} {
		// Remove tmp dirs.
		if err := removeBestEffortTmpDirs(l, tmpDir); err != nil {
			return nil, fmt.Errorf("remove tmp dirs: %w", err)
		}
	}

	db := &DB{
		dir:            dir,
		logger:         l,
		opts:           opts,
		compactc:       make(chan struct{}, 1),
		donec:          make(chan struct{}),
		stopc:          make(chan struct{}),
		autoCompact:    true,
		chunkPool:      chunkenc.NewPool(),
		blocksToDelete: opts.BlocksToDelete,
		registerer:     r,
	}
	defer func() {
		// Close files if startup fails somewhere.
		if returnedErr == nil {
			return
		}

		close(db.donec) // DB is never run if it was an error, so close this channel here.
		errs := tsdb_errors.NewMulti(returnedErr)
		if err := db.Close(); err != nil {
			errs.Add(fmt.Errorf("close DB after failed startup: %w", err))
		}
		returnedErr = errs.Err()
	}()

	if db.blocksToDelete == nil {
		db.blocksToDelete = DefaultBlocksToDelete(db)
	}

	var err error
	db.locker, err = tsdbutil.NewDirLocker(dir, "tsdb", db.logger, r)
	if err != nil {
		return nil, err
	}
	if !opts.NoLockfile {
		if err := db.locker.Lock(); err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	db.compactor, err = NewLeveledCompactorWithOptions(ctx, r, l, rngs, db.chunkPool, LeveledCompactorOptions{
		MaxBlockChunkSegmentSize:    opts.MaxBlockChunkSegmentSize,
		EnableOverlappingCompaction: opts.EnableOverlappingCompaction,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create leveled compactor: %w", err)
	}
	db.compactCancel = cancel

	var wal, wbl *wlog.WL
	segmentSize := wlog.DefaultSegmentSize
	// Wal is enabled.
	if opts.WALSegmentSize >= 0 {
		// Wal is set to a custom size.
		if opts.WALSegmentSize > 0 {
			segmentSize = opts.WALSegmentSize
		}
		wal, err = wlog.NewSize(l, r, walDir, segmentSize, opts.WALCompression)
		if err != nil {
			return nil, err
		}
		// Check if there is a WBL on disk, in which case we should replay that data.
		wblSize, err := fileutil.DirSize(wblDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if opts.OutOfOrderTimeWindow > 0 || wblSize > 0 {
			wbl, err = wlog.NewSize(l, r, wblDir, segmentSize, opts.WALCompression)
			if err != nil {
				return nil, err
			}
		}
	}
	db.oooWasEnabled.Store(opts.OutOfOrderTimeWindow > 0)
	headOpts := DefaultHeadOptions()
	headOpts.ChunkRange = rngs[0]
	headOpts.ChunkDirRoot = dir
	headOpts.ChunkPool = db.chunkPool
	headOpts.ChunkWriteBufferSize = opts.HeadChunksWriteBufferSize
	headOpts.ChunkWriteQueueSize = opts.HeadChunksWriteQueueSize
	headOpts.SamplesPerChunk = opts.SamplesPerChunk
	headOpts.StripeSize = opts.StripeSize
	headOpts.SeriesCallback = opts.SeriesLifecycleCallback
	headOpts.EnableExemplarStorage = opts.EnableExemplarStorage
	headOpts.MaxExemplars.Store(opts.MaxExemplars)
	headOpts.EnableMemorySnapshotOnShutdown = opts.EnableMemorySnapshotOnShutdown
	headOpts.EnableNativeHistograms.Store(opts.EnableNativeHistograms)
	headOpts.OutOfOrderTimeWindow.Store(opts.OutOfOrderTimeWindow)
	headOpts.OutOfOrderCapMax.Store(opts.OutOfOrderCapMax)
	headOpts.EnableSharding = opts.EnableSharding
	if opts.WALReplayConcurrency > 0 {
		headOpts.WALReplayConcurrency = opts.WALReplayConcurrency
	}
	if opts.IsolationDisabled {
		// We only override this flag if isolation is disabled at DB level. We use the default otherwise.
		headOpts.IsolationDisabled = opts.IsolationDisabled
	}
	db.head, err = NewHead(r, l, wal, wbl, headOpts, stats.Head)
	if err != nil {
		return nil, err
	}
	db.head.writeNotified = db.writeNotified

	// Register metrics after assigning the head block.
	db.metrics = newDBMetrics(db, r)
	maxBytes := opts.MaxBytes
	if maxBytes < 0 {
		maxBytes = 0
	}
	db.metrics.maxBytes.Set(float64(maxBytes))
	db.metrics.retentionDuration.Set((time.Duration(opts.RetentionDuration) * time.Millisecond).Seconds())

	if err := db.reload(); err != nil {
		return nil, err
	}
	// Set the min valid time for the ingested samples
	// to be no lower than the maxt of the last block.
	minValidTime := int64(math.MinInt64)
	// We do not consider blocks created from out-of-order samples for Head's minValidTime
	// since minValidTime is only for the in-order data and we do not want to discard unnecessary
	// samples from the Head.
	inOrderMaxTime, ok := db.inOrderBlocksMaxTime()
	if ok {
		minValidTime = inOrderMaxTime
	}

	if initErr := db.head.Init(minValidTime); initErr != nil {
		db.head.metrics.walCorruptionsTotal.Inc()
		var e *errLoadWbl
		if errors.As(initErr, &e) {
			level.Warn(db.logger).Log("msg", "Encountered WBL read error, attempting repair", "err", initErr)
			if err := wbl.Repair(e.err); err != nil {
				return nil, fmt.Errorf("repair corrupted WBL: %w", err)
			}
			level.Info(db.logger).Log("msg", "Successfully repaired WBL")
		} else {
			level.Warn(db.logger).Log("msg", "Encountered WAL read error, attempting repair", "err", initErr)
			if err := wal.Repair(initErr); err != nil {
				return nil, fmt.Errorf("repair corrupted WAL: %w", err)
			}
			level.Info(db.logger).Log("msg", "Successfully repaired WAL")
		}
	}

	if db.head.MinOOOTime() != int64(math.MaxInt64) {
		// Some OOO data was replayed from the disk that needs compaction and cleanup.
		db.oooWasEnabled.Store(true)
	}

	go db.run(ctx)

	return db, nil
}

func removeBestEffortTmpDirs(l log.Logger, dir string) error {
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, f := range files {
		if isTmpDir(f) {
			if err := os.RemoveAll(filepath.Join(dir, f.Name())); err != nil {
				level.Error(l).Log("msg", "failed to delete tmp block dir", "dir", filepath.Join(dir, f.Name()), "err", err)
				continue
			}
			level.Info(l).Log("msg", "Found and deleted tmp block dir", "dir", filepath.Join(dir, f.Name()))
		}
	}
	return nil
}

// StartTime implements the Storage interface.
func (db *DB) StartTime() (int64, error) {
	db.mtx.RLock()
	defer db.mtx.RUnlock()

	if len(db.blocks) > 0 {
		return db.blocks[0].Meta().MinTime, nil
	}
	return db.head.MinTime(), nil
}

// Dir returns the directory of the database.
func (db *DB) Dir() string {
	return db.dir
}

func (db *DB) run(ctx context.Context) {
	defer close(db.donec)

	backoff := time.Duration(0)

	for {
		select {
		case <-db.stopc:
			return
		case <-time.After(backoff):
		}

		select {
		case <-time.After(1 * time.Minute):
			db.cmtx.Lock()
			if err := db.reloadBlocks(); err != nil {
				level.Error(db.logger).Log("msg", "reloadBlocks", "err", err)
			}
			db.cmtx.Unlock()

			select {
			case db.compactc <- struct{}{}:
			default:
			}
			// We attempt mmapping of head chunks regularly.
			db.head.mmapHeadChunks()
		case <-db.compactc:
			db.metrics.compactionsTriggered.Inc()

			db.autoCompactMtx.Lock()
			if db.autoCompact {
				if err := db.Compact(ctx); err != nil {
					level.Error(db.logger).Log("msg", "compaction failed", "err", err)
					backoff = exponential(backoff, 1*time.Second, 1*time.Minute)
				} else {
					backoff = 0
				}
			} else {
				db.metrics.compactionsSkipped.Inc()
			}
			db.autoCompactMtx.Unlock()
		case <-db.stopc:
			return
		}
	}
}

// Appender opens a new appender against the database.
func (db *DB) Appender(ctx context.Context) storage.Appender {
	return dbAppender{db: db, Appender: db.head.Appender(ctx)}
}

// ApplyConfig applies a new config to the DB.
// Behaviour of 'OutOfOrderTimeWindow' is as follows:
// OOO enabled = oooTimeWindow > 0. OOO disabled = oooTimeWindow is 0.
// 1) Before: OOO disabled, Now: OOO enabled =>
//   - A new WBL is created for the head block.
//   - OOO compaction is enabled.
//   - Overlapping queries are enabled.
//
// 2) Before: OOO enabled, Now: OOO enabled =>
//   - Only the time window is updated.
//
// 3) Before: OOO enabled, Now: OOO disabled =>
//   - Time Window set to 0. So no new OOO samples will be allowed.
//   - OOO WBL will stay and will be eventually cleaned up.
//   - OOO Compaction and overlapping queries will remain enabled until a restart or until all OOO samples are compacted.
//
// 4) Before: OOO disabled, Now: OOO disabled => no-op.
func (db *DB) ApplyConfig(conf *config.Config) error {
	oooTimeWindow := int64(0)
	if conf.StorageConfig.TSDBConfig != nil {
		oooTimeWindow = conf.StorageConfig.TSDBConfig.OutOfOrderTimeWindow
	}
	if oooTimeWindow < 0 {
		oooTimeWindow = 0
	}

	// Create WBL if it was not present and if OOO is enabled with WAL enabled.
	var wblog *wlog.WL
	var err error
	switch {
	case db.head.wbl != nil:
		// The existing WBL from the disk might have been replayed while OOO was disabled.
		wblog = db.head.wbl
	case !db.oooWasEnabled.Load() && oooTimeWindow > 0 && db.opts.WALSegmentSize >= 0:
		segmentSize := wlog.DefaultSegmentSize
		// Wal is set to a custom size.
		if db.opts.WALSegmentSize > 0 {
			segmentSize = db.opts.WALSegmentSize
		}
		oooWalDir := filepath.Join(db.dir, wlog.WblDirName)
		wblog, err = wlog.NewSize(db.logger, db.registerer, oooWalDir, segmentSize, db.opts.WALCompression)
		if err != nil {
			return err
		}
	}

	db.opts.OutOfOrderTimeWindow = oooTimeWindow
	db.head.ApplyConfig(conf, wblog)

	if !db.oooWasEnabled.Load() {
		db.oooWasEnabled.Store(oooTimeWindow > 0)
	}
	return nil
}

// EnableNativeHistograms enables the native histogram feature.
func (db *DB) EnableNativeHistograms() {
	db.head.EnableNativeHistograms()
}

// DisableNativeHistograms disables the native histogram feature.
func (db *DB) DisableNativeHistograms() {
	db.head.DisableNativeHistograms()
}

// dbAppender wraps the DB's head appender and triggers compactions on commit
// if necessary.
type dbAppender struct {
	storage.Appender
	db *DB
}

var _ storage.GetRef = dbAppender{}

func (a dbAppender) GetRef(lset labels.Labels, hash uint64) (storage.SeriesRef, labels.Labels) {
	if g, ok := a.Appender.(storage.GetRef); ok {
		return g.GetRef(lset, hash)
	}
	return 0, labels.EmptyLabels()
}

func (a dbAppender) Commit() error {
	err := a.Appender.Commit()

	// We could just run this check every few minutes practically. But for benchmarks
	// and high frequency use cases this is the safer way.
	if a.db.head.compactable() {
		select {
		case a.db.compactc <- struct{}{}:
		default:
		}
	}
	return err
}

// Compact data if possible. After successful compaction blocks are reloaded
// which will also delete the blocks that fall out of the retention window.
// Old blocks are only deleted on reloadBlocks based on the new block's parent information.
// See DB.reloadBlocks documentation for further information.
func (db *DB) Compact(ctx context.Context) (returnErr error) {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()
	defer func() {
		if returnErr != nil && !errors.Is(returnErr, context.Canceled) {
			// If we got an error because context was canceled then we're most likely
			// shutting down TSDB and we don't need to report this on metrics
			db.metrics.compactionsFailed.Inc()
		}
	}()

	lastBlockMaxt := int64(math.MinInt64)
	defer func() {
		errs := tsdb_errors.NewMulti(returnErr)
		if err := db.head.truncateWAL(lastBlockMaxt); err != nil {
			errs.Add(fmt.Errorf("WAL truncation in Compact defer: %w", err))
		}
		returnErr = errs.Err()
	}()

	start := time.Now()
	// Check whether we have pending head blocks that are ready to be persisted.
	// They have the highest priority.
	for {
		select {
		case <-db.stopc:
			return nil
		default:
		}
		if !db.head.compactable() {
			break
		}
		mint := db.head.MinTime()
		maxt := rangeForTimestamp(mint, db.head.chunkRange.Load())

		// Wrap head into a range that bounds all reads to it.
		// We remove 1 millisecond from maxt because block
		// intervals are half-open: [b.MinTime, b.MaxTime). But
		// chunk intervals are closed: [c.MinTime, c.MaxTime];
		// so in order to make sure that overlaps are evaluated
		// consistently, we explicitly remove the last value
		// from the block interval here.
		rh := NewRangeHeadWithIsolationDisabled(db.head, mint, maxt-1)

		// Compaction runs with isolation disabled, because head.compactable()
		// ensures that maxt is more than chunkRange/2 back from now, and
		// head.appendableMinValidTime() ensures that no new appends can start within the compaction range.
		// We do need to wait for any overlapping appenders that started previously to finish.
		db.head.WaitForAppendersOverlapping(rh.MaxTime())

		if err := db.compactHead(rh); err != nil {
			return fmt.Errorf("compact head: %w", err)
		}
		// Consider only successful compactions for WAL truncation.
		lastBlockMaxt = maxt
	}

	// Clear some disk space before compacting blocks, especially important
	// when Head compaction happened over a long time range.
	if err := db.head.truncateWAL(lastBlockMaxt); err != nil {
		return fmt.Errorf("WAL truncation in Compact: %w", err)
	}

	compactionDuration := time.Since(start)
	if compactionDuration.Milliseconds() > db.head.chunkRange.Load() {
		level.Warn(db.logger).Log(
			"msg", "Head compaction took longer than the block time range, compactions are falling behind and won't be able to catch up",
			"duration", compactionDuration.String(),
			"block_range", db.head.chunkRange.Load(),
		)
	}

	if lastBlockMaxt != math.MinInt64 {
		// The head was compacted, so we compact OOO head as well.
		if err := db.compactOOOHead(ctx); err != nil {
			return fmt.Errorf("compact ooo head: %w", err)
		}
	}

	return db.compactBlocks()
}

// CompactHead compacts the given RangeHead.
func (db *DB) CompactHead(head *RangeHead) error {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	if err := db.compactHead(head); err != nil {
		return fmt.Errorf("compact head: %w", err)
	}

	if err := db.head.truncateWAL(head.BlockMaxTime()); err != nil {
		return fmt.Errorf("WAL truncation: %w", err)
	}
	return nil
}

// CompactOOOHead compacts the OOO Head.
func (db *DB) CompactOOOHead(ctx context.Context) error {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	return db.compactOOOHead(ctx)
}

func (db *DB) compactOOOHead(ctx context.Context) error {
	if !db.oooWasEnabled.Load() {
		return nil
	}
	oooHead, err := NewOOOCompactionHead(ctx, db.head)
	if err != nil {
		return fmt.Errorf("get ooo compaction head: %w", err)
	}

	ulids, err := db.compactOOO(db.dir, oooHead)
	if err != nil {
		return fmt.Errorf("compact ooo head: %w", err)
	}
	if err := db.reloadBlocks(); err != nil {
		errs := tsdb_errors.NewMulti(err)
		for _, uid := range ulids {
			if errRemoveAll := os.RemoveAll(filepath.Join(db.dir, uid.String())); errRemoveAll != nil {
				errs.Add(errRemoveAll)
			}
		}
		return fmt.Errorf("reloadBlocks blocks after failed compact ooo head: %w", errs.Err())
	}

	lastWBLFile, minOOOMmapRef := oooHead.LastWBLFile(), oooHead.LastMmapRef()
	if lastWBLFile != 0 || minOOOMmapRef != 0 {
		if minOOOMmapRef != 0 {
			// Ensure that no more queriers are created that will reference chunks we're about to garbage collect.
			// truncateOOO waits for any existing queriers that reference chunks we're about to garbage collect to
			// complete before running garbage collection, so we don't need to do that here.
			//
			// We take mtx to ensure that Querier() and ChunkQuerier() don't miss blocks: without this, they could
			// capture the list of blocks before the call to reloadBlocks() above runs, but then capture
			// lastGarbageCollectedMmapRef after we update it here, and therefore not query either the blocks we've just
			// written or the head chunks those blocks were created from.
			db.mtx.Lock()
			db.lastGarbageCollectedMmapRef = minOOOMmapRef
			db.mtx.Unlock()
		}

		if err := db.head.truncateOOO(lastWBLFile, minOOOMmapRef); err != nil {
			return fmt.Errorf("truncate ooo wbl: %w", err)
		}
	}

	return nil
}

// compactOOO creates a new block per possible block range in the compactor's directory from the OOO Head given.
// Each ULID in the result corresponds to a block in a unique time range.
func (db *DB) compactOOO(dest string, oooHead *OOOCompactionHead) (_ []ulid.ULID, err error) {
	start := time.Now()

	blockSize := oooHead.ChunkRange()
	oooHeadMint, oooHeadMaxt := oooHead.MinTime(), oooHead.MaxTime()
	ulids := make([]ulid.ULID, 0)
	defer func() {
		if err != nil {
			// Best effort removal of created block on any error.
			for _, uid := range ulids {
				_ = os.RemoveAll(filepath.Join(db.dir, uid.String()))
			}
		}
	}()

	meta := &BlockMeta{}
	meta.Compaction.SetOutOfOrder()
	for t := blockSize * (oooHeadMint / blockSize); t <= oooHeadMaxt; t += blockSize {
		mint, maxt := t, t+blockSize
		// Block intervals are half-open: [b.MinTime, b.MaxTime). Block intervals are always +1 than the total samples it includes.
		uid, err := db.compactor.Write(dest, oooHead.CloneForTimeRange(mint, maxt-1), mint, maxt, meta)
		if err != nil {
			return nil, err
		}
		if uid.Compare(ulid.ULID{}) != 0 {
			ulids = append(ulids, uid)
		}
	}

	if len(ulids) == 0 {
		level.Info(db.logger).Log(
			"msg", "compact ooo head resulted in no blocks",
			"duration", time.Since(start),
		)
		return nil, nil
	}

	level.Info(db.logger).Log(
		"msg", "out-of-order compaction completed",
		"duration", time.Since(start),
		"ulids", fmt.Sprintf("%v", ulids),
	)
	return ulids, nil
}

// compactHead compacts the given RangeHead.
// The compaction mutex should be held before calling this method.
func (db *DB) compactHead(head *RangeHead) error {
	uid, err := db.compactor.Write(db.dir, head, head.MinTime(), head.BlockMaxTime(), nil)
	if err != nil {
		return fmt.Errorf("persist head block: %w", err)
	}

	if err := db.reloadBlocks(); err != nil {
		if errRemoveAll := os.RemoveAll(filepath.Join(db.dir, uid.String())); errRemoveAll != nil {
			return tsdb_errors.NewMulti(
				fmt.Errorf("reloadBlocks blocks: %w", err),
				fmt.Errorf("delete persisted head block after failed db reloadBlocks:%s: %w", uid, errRemoveAll),
			).Err()
		}
		return fmt.Errorf("reloadBlocks blocks: %w", err)
	}
	if err = db.head.truncateMemory(head.BlockMaxTime()); err != nil {
		return fmt.Errorf("head memory truncate: %w", err)
	}
	return nil
}

// compactBlocks compacts all the eligible on-disk blocks.
// The compaction mutex should be held before calling this method.
func (db *DB) compactBlocks() (err error) {
	// Check for compactions of multiple blocks.
	for {
		// If we have a lot of blocks to compact the whole process might take
		// long enough that we end up with a HEAD block that needs to be written.
		// Check if that's the case and stop compactions early.
		if db.head.compactable() {
			level.Warn(db.logger).Log("msg", "aborting block compactions to persit the head block")
			return nil
		}

		plan, err := db.compactor.Plan(db.dir)
		if err != nil {
			return fmt.Errorf("plan compaction: %w", err)
		}
		if len(plan) == 0 {
			break
		}

		select {
		case <-db.stopc:
			return nil
		default:
		}

		uid, err := db.compactor.Compact(db.dir, plan, db.blocks)
		if err != nil {
			return fmt.Errorf("compact %s: %w", plan, err)
		}

		if err := db.reloadBlocks(); err != nil {
			if err := os.RemoveAll(filepath.Join(db.dir, uid.String())); err != nil {
				return fmt.Errorf("delete compacted block after failed db reloadBlocks:%s: %w", uid, err)
			}
			return fmt.Errorf("reloadBlocks blocks: %w", err)
		}
	}

	return nil
}

// getBlock iterates a given block range to find a block by a given id.
// If found it returns the block itself and a boolean to indicate that it was found.
func getBlock(allBlocks []*Block, id ulid.ULID) (*Block, bool) {
	for _, b := range allBlocks {
		if b.Meta().ULID == id {
			return b, true
		}
	}
	return nil, false
}

// reload reloads blocks and truncates the head and its WAL.
func (db *DB) reload() error {
	if err := db.reloadBlocks(); err != nil {
		return fmt.Errorf("reloadBlocks: %w", err)
	}
	maxt, ok := db.inOrderBlocksMaxTime()
	if !ok {
		return nil
	}
	if err := db.head.Truncate(maxt); err != nil {
		return fmt.Errorf("head truncate: %w", err)
	}
	return nil
}

// reloadBlocks reloads blocks without touching head.
// Blocks that are obsolete due to replacement or retention will be deleted.
func (db *DB) reloadBlocks() (err error) {
	defer func() {
		if err != nil {
			db.metrics.reloadsFailed.Inc()
		}
		db.metrics.reloads.Inc()
	}()

	// Now that we reload TSDB every minute, there is a high chance for a race condition with a reload
	// triggered by CleanTombstones(). We need to lock the reload to avoid the situation where
	// a normal reload and CleanTombstones try to delete the same block.
	db.mtx.Lock()
	defer db.mtx.Unlock()

	loadable, corrupted, err := openBlocks(db.logger, db.dir, db.blocks, db.chunkPool)
	if err != nil {
		return err
	}

	deletableULIDs := db.blocksToDelete(loadable)
	deletable := make(map[ulid.ULID]*Block, len(deletableULIDs))

	// Mark all parents of loaded blocks as deletable (no matter if they exists). This makes it resilient against the process
	// crashing towards the end of a compaction but before deletions. By doing that, we can pick up the deletion where it left off during a crash.
	for _, block := range loadable {
		if _, ok := deletableULIDs[block.meta.ULID]; ok {
			deletable[block.meta.ULID] = block
		}
		for _, b := range block.Meta().Compaction.Parents {
			if _, ok := corrupted[b.ULID]; ok {
				delete(corrupted, b.ULID)
				level.Warn(db.logger).Log("msg", "Found corrupted block, but replaced by compacted one so it's safe to delete. This should not happen with atomic deletes.", "block", b.ULID)
			}
			deletable[b.ULID] = nil
		}
	}

	if len(corrupted) > 0 {
		// Corrupted but no child loaded for it.
		// Close all new blocks to release the lock for windows.
		for _, block := range loadable {
			if _, open := getBlock(db.blocks, block.Meta().ULID); !open {
				block.Close()
			}
		}
		errs := tsdb_errors.NewMulti()
		for ulid, err := range corrupted {
			if err != nil {
				errs.Add(fmt.Errorf("corrupted block %s: %w", ulid.String(), err))
			}
		}
		return errs.Err()
	}

	var (
		toLoad     []*Block
		blocksSize int64
	)
	// All deletable blocks should be unloaded.
	// NOTE: We need to loop through loadable one more time as there might be loadable ready to be removed (replaced by compacted block).
	for _, block := range loadable {
		if _, ok := deletable[block.Meta().ULID]; ok {
			deletable[block.Meta().ULID] = block
			continue
		}

		toLoad = append(toLoad, block)
		blocksSize += block.Size()
	}
	db.metrics.blocksBytes.Set(float64(blocksSize))

	slices.SortFunc(toLoad, func(a, b *Block) int {
		switch {
		case a.Meta().MinTime < b.Meta().MinTime:
			return -1
		case a.Meta().MinTime > b.Meta().MinTime:
			return 1
		default:
			return 0
		}
	})

	// Swap new blocks first for subsequently created readers to be seen.
	oldBlocks := db.blocks
	db.blocks = toLoad

	blockMetas := make([]BlockMeta, 0, len(toLoad))
	for _, b := range toLoad {
		blockMetas = append(blockMetas, b.Meta())
	}
	if overlaps := OverlappingBlocks(blockMetas); len(overlaps) > 0 {
		level.Warn(db.logger).Log("msg", "Overlapping blocks found during reloadBlocks", "detail", overlaps.String())
	}

	// Append blocks to old, deletable blocks, so we can close them.
	for _, b := range oldBlocks {
		if _, ok := deletable[b.Meta().ULID]; ok {
			deletable[b.Meta().ULID] = b
		}
	}
	if err := db.deleteBlocks(deletable); err != nil {
		return fmt.Errorf("delete %v blocks: %w", len(deletable), err)
	}
	return nil
}

func openBlocks(l log.Logger, dir string, loaded []*Block, chunkPool chunkenc.Pool) (blocks []*Block, corrupted map[ulid.ULID]error, err error) {
	bDirs, err := blockDirs(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("find blocks: %w", err)
	}

	corrupted = make(map[ulid.ULID]error)
	for _, bDir := range bDirs {
		meta, _, err := readMetaFile(bDir)
		if err != nil {
			level.Error(l).Log("msg", "Failed to read meta.json for a block during reloadBlocks. Skipping", "dir", bDir, "err", err)
			continue
		}

		// See if we already have the block in memory or open it otherwise.
		block, open := getBlock(loaded, meta.ULID)
		if !open {
			block, err = OpenBlock(l, bDir, chunkPool)
			if err != nil {
				corrupted[meta.ULID] = err
				continue
			}
		}
		blocks = append(blocks, block)
	}
	return blocks, corrupted, nil
}

// DefaultBlocksToDelete returns a filter which decides time based and size based
// retention from the options of the db.
func DefaultBlocksToDelete(db *DB) BlocksToDeleteFunc {
	return func(blocks []*Block) map[ulid.ULID]struct{} {
		return deletableBlocks(db, blocks)
	}
}

// deletableBlocks returns all currently loaded blocks past retention policy or already compacted into a new block.
func deletableBlocks(db *DB, blocks []*Block) map[ulid.ULID]struct{} {
	deletable := make(map[ulid.ULID]struct{})

	// Sort the blocks by time - newest to oldest (largest to smallest timestamp).
	// This ensures that the retentions will remove the oldest  blocks.
	slices.SortFunc(blocks, func(a, b *Block) int {
		switch {
		case b.Meta().MaxTime < a.Meta().MaxTime:
			return -1
		case b.Meta().MaxTime > a.Meta().MaxTime:
			return 1
		default:
			return 0
		}
	})

	for _, block := range blocks {
		if block.Meta().Compaction.Deletable {
			deletable[block.Meta().ULID] = struct{}{}
		}
	}

	for ulid := range BeyondTimeRetention(db, blocks) {
		deletable[ulid] = struct{}{}
	}

	for ulid := range BeyondSizeRetention(db, blocks) {
		deletable[ulid] = struct{}{}
	}

	return deletable
}

// BeyondTimeRetention returns those blocks which are beyond the time retention
// set in the db options.
func BeyondTimeRetention(db *DB, blocks []*Block) (deletable map[ulid.ULID]struct{}) {
	// Time retention is disabled or no blocks to work with.
	if len(blocks) == 0 || db.opts.RetentionDuration == 0 {
		return
	}

	deletable = make(map[ulid.ULID]struct{})
	for i, block := range blocks {
		// The difference between the first block and this block is greater than or equal to
		// the retention period so any blocks after that are added as deletable.
		if i > 0 && blocks[0].Meta().MaxTime-block.Meta().MaxTime >= db.opts.RetentionDuration {
			for _, b := range blocks[i:] {
				deletable[b.meta.ULID] = struct{}{}
			}
			db.metrics.timeRetentionCount.Inc()
			break
		}
	}
	return deletable
}

// BeyondSizeRetention returns those blocks which are beyond the size retention
// set in the db options.
func BeyondSizeRetention(db *DB, blocks []*Block) (deletable map[ulid.ULID]struct{}) {
	// Size retention is disabled or no blocks to work with.
	if len(blocks) == 0 || db.opts.MaxBytes <= 0 {
		return
	}

	deletable = make(map[ulid.ULID]struct{})

	// Initializing size counter with WAL size and Head chunks
	// written to disk, as that is part of the retention strategy.
	blocksSize := db.Head().Size()
	for i, block := range blocks {
		blocksSize += block.Size()
		if blocksSize > db.opts.MaxBytes {
			// Add this and all following blocks for deletion.
			for _, b := range blocks[i:] {
				deletable[b.meta.ULID] = struct{}{}
			}
			db.metrics.sizeRetentionCount.Inc()
			break
		}
	}
	return deletable
}

// deleteBlocks closes the block if loaded and deletes blocks from the disk if exists.
// When the map contains a non nil block object it means it is loaded in memory
// so needs to be closed first as it might need to wait for pending readers to complete.
func (db *DB) deleteBlocks(blocks map[ulid.ULID]*Block) error {
	for ulid, block := range blocks {
		if block != nil {
			if err := block.Close(); err != nil {
				level.Warn(db.logger).Log("msg", "Closing block failed", "err", err, "block", ulid)
			}
		}

		toDelete := filepath.Join(db.dir, ulid.String())
		switch _, err := os.Stat(toDelete); {
		case os.IsNotExist(err):
			// Noop.
			continue
		case err != nil:
			return fmt.Errorf("stat dir %v: %w", toDelete, err)
		}

		// Replace atomically to avoid partial block when process would crash during deletion.
		tmpToDelete := filepath.Join(db.dir, fmt.Sprintf("%s%s", ulid, tmpForDeletionBlockDirSuffix))
		if err := fileutil.Replace(toDelete, tmpToDelete); err != nil {
			return fmt.Errorf("replace of obsolete block for deletion %s: %w", ulid, err)
		}
		if err := os.RemoveAll(tmpToDelete); err != nil {
			return fmt.Errorf("delete obsolete block %s: %w", ulid, err)
		}
		level.Info(db.logger).Log("msg", "Deleting obsolete block", "block", ulid)
	}

	return nil
}

// TimeRange specifies minTime and maxTime range.
type TimeRange struct {
	Min, Max int64
}

// Overlaps contains overlapping blocks aggregated by overlapping range.
type Overlaps map[TimeRange][]BlockMeta

// String returns human readable string form of overlapped blocks.
func (o Overlaps) String() string {
	var res []string
	for r, overlaps := range o {
		var groups []string
		for _, m := range overlaps {
			groups = append(groups, fmt.Sprintf(
				"<ulid: %s, mint: %d, maxt: %d, range: %s>",
				m.ULID.String(),
				m.MinTime,
				m.MaxTime,
				(time.Duration((m.MaxTime-m.MinTime)/1000)*time.Second).String(),
			))
		}
		res = append(res, fmt.Sprintf(
			"[mint: %d, maxt: %d, range: %s, blocks: %d]: %s",
			r.Min, r.Max,
			(time.Duration((r.Max-r.Min)/1000)*time.Second).String(),
			len(overlaps),
			strings.Join(groups, ", ")),
		)
	}
	return strings.Join(res, "\n")
}

// OverlappingBlocks returns all overlapping blocks from given meta files.
func OverlappingBlocks(bm []BlockMeta) Overlaps {
	if len(bm) <= 1 {
		return nil
	}
	var (
		overlaps [][]BlockMeta

		// pending contains not ended blocks in regards to "current" timestamp.
		pending = []BlockMeta{bm[0]}
		// continuousPending helps to aggregate same overlaps to single group.
		continuousPending = true
	)

	// We have here blocks sorted by minTime. We iterate over each block and treat its minTime as our "current" timestamp.
	// We check if any of the pending block finished (blocks that we have seen before, but their maxTime was still ahead current
	// timestamp). If not, it means they overlap with our current block. In the same time current block is assumed pending.
	for _, b := range bm[1:] {
		var newPending []BlockMeta

		for _, p := range pending {
			// "b.MinTime" is our current time.
			if b.MinTime >= p.MaxTime {
				continuousPending = false
				continue
			}

			// "p" overlaps with "b" and "p" is still pending.
			newPending = append(newPending, p)
		}

		// Our block "b" is now pending.
		pending = append(newPending, b)
		if len(newPending) == 0 {
			// No overlaps.
			continue
		}

		if continuousPending && len(overlaps) > 0 {
			overlaps[len(overlaps)-1] = append(overlaps[len(overlaps)-1], b)
			continue
		}
		overlaps = append(overlaps, append(newPending, b))
		// Start new pendings.
		continuousPending = true
	}

	// Fetch the critical overlapped time range foreach overlap groups.
	overlapGroups := Overlaps{}
	for _, overlap := range overlaps {
		minRange := TimeRange{Min: 0, Max: math.MaxInt64}
		for _, b := range overlap {
			if minRange.Max > b.MaxTime {
				minRange.Max = b.MaxTime
			}

			if minRange.Min < b.MinTime {
				minRange.Min = b.MinTime
			}
		}
		overlapGroups[minRange] = overlap
	}

	return overlapGroups
}

func (db *DB) String() string {
	return "HEAD"
}

// Blocks returns the databases persisted blocks.
func (db *DB) Blocks() []*Block {
	db.mtx.RLock()
	defer db.mtx.RUnlock()

	return db.blocks
}

// inOrderBlocksMaxTime returns the max time among the blocks that were not totally created
// out of out-of-order data. If the returned boolean is true, it means there is at least
// one such block.
func (db *DB) inOrderBlocksMaxTime() (maxt int64, ok bool) {
	maxt, ok = int64(math.MinInt64), false
	// If blocks are overlapping, last block might not have the max time. So check all blocks.
	for _, b := range db.Blocks() {
		if !b.meta.Compaction.FromOutOfOrder() && b.meta.MaxTime > maxt {
			ok = true
			maxt = b.meta.MaxTime
		}
	}
	return maxt, ok
}

// Head returns the databases's head.
func (db *DB) Head() *Head {
	return db.head
}

// Close the partition.
func (db *DB) Close() error {
	close(db.stopc)
	if db.compactCancel != nil {
		db.compactCancel()
	}
	<-db.donec

	db.mtx.Lock()
	defer db.mtx.Unlock()

	var g errgroup.Group

	// blocks also contains all head blocks.
	for _, pb := range db.blocks {
		g.Go(pb.Close)
	}

	errs := tsdb_errors.NewMulti(g.Wait(), db.locker.Release())
	if db.head != nil {
		errs.Add(db.head.Close())
	}
	return errs.Err()
}

// DisableCompactions disables auto compactions.
func (db *DB) DisableCompactions() {
	db.autoCompactMtx.Lock()
	defer db.autoCompactMtx.Unlock()

	db.autoCompact = false
	level.Info(db.logger).Log("msg", "Compactions disabled")
}

// EnableCompactions enables auto compactions.
func (db *DB) EnableCompactions() {
	db.autoCompactMtx.Lock()
	defer db.autoCompactMtx.Unlock()

	db.autoCompact = true
	level.Info(db.logger).Log("msg", "Compactions enabled")
}

// ForceHeadMMap is intended for use only in tests and benchmarks.
func (db *DB) ForceHeadMMap() {
	db.head.mmapHeadChunks()
}

// Snapshot writes the current data to the directory. If withHead is set to true it
// will create a new block containing all data that's currently in the memory buffer/WAL.
func (db *DB) Snapshot(dir string, withHead bool) error {
	if dir == db.dir {
		return fmt.Errorf("cannot snapshot into base directory")
	}
	if _, err := ulid.ParseStrict(dir); err == nil {
		return fmt.Errorf("dir must not be a valid ULID")
	}

	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	db.mtx.RLock()
	defer db.mtx.RUnlock()

	for _, b := range db.blocks {
		level.Info(db.logger).Log("msg", "Snapshotting block", "block", b)

		if err := b.Snapshot(dir); err != nil {
			return fmt.Errorf("error snapshotting block: %s: %w", b.Dir(), err)
		}
	}
	if !withHead {
		return nil
	}

	mint := db.head.MinTime()
	maxt := db.head.MaxTime()
	head := NewRangeHead(db.head, mint, maxt)
	// Add +1 millisecond to block maxt because block intervals are half-open: [b.MinTime, b.MaxTime).
	// Because of this block intervals are always +1 than the total samples it includes.
	if _, err := db.compactor.Write(dir, head, mint, maxt+1, nil); err != nil {
		return fmt.Errorf("snapshot head block: %w", err)
	}
	return nil
}

// Querier returns a new querier over the data partition for the given time range.
func (db *DB) Querier(mint, maxt int64) (_ storage.Querier, err error) {
	var blocks []BlockReader

	db.mtx.RLock()
	defer db.mtx.RUnlock()

	for _, b := range db.blocks {
		if b.OverlapsClosedInterval(mint, maxt) {
			blocks = append(blocks, b)
		}
	}

	blockQueriers := make([]storage.Querier, 0, len(blocks)+2) // +2 to allow for possible in-order and OOO head queriers

	defer func() {
		if err != nil {
			// If we fail, all previously opened queriers must be closed.
			for _, q := range blockQueriers {
				// TODO(bwplotka): Handle error.
				_ = q.Close()
			}
		}
	}()

	if maxt >= db.head.MinTime() {
		rh := NewRangeHead(db.head, mint, maxt)
		var err error
		inOrderHeadQuerier, err := NewBlockQuerier(rh, mint, maxt)
		if err != nil {
			return nil, fmt.Errorf("open block querier for head %s: %w", rh, err)
		}

		// Getting the querier above registers itself in the queue that the truncation waits on.
		// So if the querier is currently not colliding with any truncation, we can continue to use it and still
		// won't run into a race later since any truncation that comes after will wait on this querier if it overlaps.
		shouldClose, getNew, newMint := db.head.IsQuerierCollidingWithTruncation(mint, maxt)
		if shouldClose {
			if err := inOrderHeadQuerier.Close(); err != nil {
				return nil, fmt.Errorf("closing head block querier %s: %w", rh, err)
			}
			inOrderHeadQuerier = nil
		}
		if getNew {
			rh := NewRangeHead(db.head, newMint, maxt)
			inOrderHeadQuerier, err = NewBlockQuerier(rh, newMint, maxt)
			if err != nil {
				return nil, fmt.Errorf("open block querier for head while getting new querier %s: %w", rh, err)
			}
		}

		if inOrderHeadQuerier != nil {
			blockQueriers = append(blockQueriers, inOrderHeadQuerier)
		}
	}

	if overlapsClosedInterval(mint, maxt, db.head.MinOOOTime(), db.head.MaxOOOTime()) {
		rh := NewOOORangeHead(db.head, mint, maxt, db.lastGarbageCollectedMmapRef)
		var err error
		outOfOrderHeadQuerier, err := NewBlockQuerier(rh, mint, maxt)
		if err != nil {
			// If NewBlockQuerier() failed, make sure to clean up the pending read created by NewOOORangeHead.
			rh.isoState.Close()

			return nil, fmt.Errorf("open block querier for ooo head %s: %w", rh, err)
		}

		blockQueriers = append(blockQueriers, outOfOrderHeadQuerier)
	}

	for _, b := range blocks {
		q, err := NewBlockQuerier(b, mint, maxt)
		if err != nil {
			return nil, fmt.Errorf("open querier for block %s: %w", b, err)
		}
		blockQueriers = append(blockQueriers, q)
	}

	return storage.NewMergeQuerier(blockQueriers, nil, storage.ChainedSeriesMerge), nil
}

// blockChunkQuerierForRange returns individual block chunk queriers from the persistent blocks, in-order head block, and the
// out-of-order head block, overlapping with the given time range.
func (db *DB) blockChunkQuerierForRange(mint, maxt int64) (_ []storage.ChunkQuerier, err error) {
	var blocks []BlockReader

	db.mtx.RLock()
	defer db.mtx.RUnlock()

	for _, b := range db.blocks {
		if b.OverlapsClosedInterval(mint, maxt) {
			blocks = append(blocks, b)
		}
	}

	blockQueriers := make([]storage.ChunkQuerier, 0, len(blocks)+2) // +2 to allow for possible in-order and OOO head queriers

	defer func() {
		if err != nil {
			// If we fail, all previously opened queriers must be closed.
			for _, q := range blockQueriers {
				// TODO(bwplotka): Handle error.
				_ = q.Close()
			}
		}
	}()

	if maxt >= db.head.MinTime() {
		rh := NewRangeHead(db.head, mint, maxt)
		inOrderHeadQuerier, err := NewBlockChunkQuerier(rh, mint, maxt)
		if err != nil {
			return nil, fmt.Errorf("open querier for head %s: %w", rh, err)
		}

		// Getting the querier above registers itself in the queue that the truncation waits on.
		// So if the querier is currently not colliding with any truncation, we can continue to use it and still
		// won't run into a race later since any truncation that comes after will wait on this querier if it overlaps.
		shouldClose, getNew, newMint := db.head.IsQuerierCollidingWithTruncation(mint, maxt)
		if shouldClose {
			if err := inOrderHeadQuerier.Close(); err != nil {
				return nil, fmt.Errorf("closing head querier %s: %w", rh, err)
			}
			inOrderHeadQuerier = nil
		}
		if getNew {
			rh := NewRangeHead(db.head, newMint, maxt)
			inOrderHeadQuerier, err = NewBlockChunkQuerier(rh, newMint, maxt)
			if err != nil {
				return nil, fmt.Errorf("open querier for head while getting new querier %s: %w", rh, err)
			}
		}

		if inOrderHeadQuerier != nil {
			blockQueriers = append(blockQueriers, inOrderHeadQuerier)
		}
	}

	if overlapsClosedInterval(mint, maxt, db.head.MinOOOTime(), db.head.MaxOOOTime()) {
		rh := NewOOORangeHead(db.head, mint, maxt, db.lastGarbageCollectedMmapRef)
		outOfOrderHeadQuerier, err := NewBlockChunkQuerier(rh, mint, maxt)
		if err != nil {
			return nil, fmt.Errorf("open block chunk querier for ooo head %s: %w", rh, err)
		}

		blockQueriers = append(blockQueriers, outOfOrderHeadQuerier)
	}

	for _, b := range blocks {
		q, err := NewBlockChunkQuerier(b, mint, maxt)
		if err != nil {
			return nil, fmt.Errorf("open querier for block %s: %w", b, err)
		}
		blockQueriers = append(blockQueriers, q)
	}

	return blockQueriers, nil
}

// ChunkQuerier returns a new chunk querier over the data partition for the given time range.
func (db *DB) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	blockQueriers, err := db.blockChunkQuerierForRange(mint, maxt)
	if err != nil {
		return nil, err
	}
	return storage.NewMergeChunkQuerier(blockQueriers, nil, storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge)), nil
}

func (db *DB) ExemplarQuerier(ctx context.Context) (storage.ExemplarQuerier, error) {
	return db.head.exemplars.ExemplarQuerier(ctx)
}

func rangeForTimestamp(t, width int64) (maxt int64) {
	return (t/width)*width + width
}

// Delete implements deletion of metrics. It only has atomicity guarantees on a per-block basis.
func (db *DB) Delete(ctx context.Context, mint, maxt int64, ms ...*labels.Matcher) error {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	var g errgroup.Group

	db.mtx.RLock()
	defer db.mtx.RUnlock()

	for _, b := range db.blocks {
		if b.OverlapsClosedInterval(mint, maxt) {
			g.Go(func(b *Block) func() error {
				return func() error { return b.Delete(ctx, mint, maxt, ms...) }
			}(b))
		}
	}
	if db.head.OverlapsClosedInterval(mint, maxt) {
		g.Go(func() error {
			return db.head.Delete(ctx, mint, maxt, ms...)
		})
	}

	return g.Wait()
}

// CleanTombstones re-writes any blocks with tombstones.
func (db *DB) CleanTombstones() (err error) {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	start := time.Now()
	defer func() {
		db.metrics.tombCleanTimer.Observe(time.Since(start).Seconds())
	}()

	cleanUpCompleted := false
	// Repeat cleanup until there is no tombstones left.
	for !cleanUpCompleted {
		cleanUpCompleted = true

		for _, pb := range db.Blocks() {
			uid, safeToDelete, cleanErr := pb.CleanTombstones(db.Dir(), db.compactor)
			if cleanErr != nil {
				return fmt.Errorf("clean tombstones: %s: %w", pb.Dir(), cleanErr)
			}
			if !safeToDelete {
				// There was nothing to clean.
				continue
			}

			// In case tombstones of the old block covers the whole block,
			// then there would be no resultant block to tell the parent.
			// The lock protects against race conditions when deleting blocks
			// during an already running reload.
			db.mtx.Lock()
			pb.meta.Compaction.Deletable = safeToDelete
			db.mtx.Unlock()
			cleanUpCompleted = false
			if err = db.reloadBlocks(); err == nil { // Will try to delete old block.
				// Successful reload will change the existing blocks.
				// We need to loop over the new set of blocks.
				break
			}

			// Delete new block if it was created.
			if uid != nil && *uid != (ulid.ULID{}) {
				dir := filepath.Join(db.Dir(), uid.String())
				if err := os.RemoveAll(dir); err != nil {
					level.Error(db.logger).Log("msg", "failed to delete block after failed `CleanTombstones`", "dir", dir, "err", err)
				}
			}
			if err != nil {
				return fmt.Errorf("reload blocks: %w", err)
			}
			return nil
		}
	}
	return nil
}

func (db *DB) SetWriteNotified(wn wlog.WriteNotified) {
	db.writeNotified = wn
	// It's possible we already created the head struct, so we should also set the WN for that.
	db.head.writeNotified = wn
}

func isBlockDir(fi fs.DirEntry) bool {
	if !fi.IsDir() {
		return false
	}
	_, err := ulid.ParseStrict(fi.Name())
	return err == nil
}

// isTmpDir returns true if the given file-info contains a block ULID, a checkpoint prefix,
// or a chunk snapshot prefix and a tmp extension.
func isTmpDir(fi fs.DirEntry) bool {
	if !fi.IsDir() {
		return false
	}

	fn := fi.Name()
	ext := filepath.Ext(fn)
	if ext == tmpForDeletionBlockDirSuffix || ext == tmpForCreationBlockDirSuffix || ext == tmpLegacy {
		if strings.HasPrefix(fn, "checkpoint.") {
			return true
		}
		if strings.HasPrefix(fn, chunkSnapshotPrefix) {
			return true
		}
		if _, err := ulid.ParseStrict(fn[:len(fn)-len(ext)]); err == nil {
			return true
		}
	}
	return false
}

func blockDirs(dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var dirs []string

	for _, f := range files {
		if isBlockDir(f) {
			dirs = append(dirs, filepath.Join(dir, f.Name()))
		}
	}
	return dirs, nil
}

func exponential(d, min, max time.Duration) time.Duration {
	d *= 2
	if d < min {
		d = min
	}
	if d > max {
		d = max
	}
	return d
}
