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
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

// ExponentialBlockRanges returns the time ranges based on the stepSize.
func ExponentialBlockRanges(minSize int64, steps, stepSize int) []int64 {
	ranges := make([]int64, 0, steps)
	curRange := minSize
	for i := 0; i < steps; i++ {
		ranges = append(ranges, curRange)
		curRange *= int64(stepSize)
	}

	return ranges
}

// Compactor provides compaction against an underlying storage
// of time series data.
type Compactor interface {
	// Plan returns a set of directories that can be compacted concurrently.
	// The directories can be overlapping.
	// Results returned when compactions are in progress are undefined.
	Plan(dir string) ([]string, error)

	// Write persists a Block into a directory.
	// No Block is written when resulting Block has 0 samples, and returns empty ulid.ULID{}.
	Write(dest string, b BlockReader, mint, maxt int64, base *BlockMeta) (ulid.ULID, error)

	// Compact runs compaction against the provided directories. Must
	// only be called concurrently with results of Plan().
	// Can optionally pass a list of already open blocks,
	// to avoid having to reopen them.
	// When resulting Block has 0 samples
	//  * No block is written.
	//  * The source dirs are marked Deletable.
	//  * Returns empty ulid.ULID{}.
	Compact(dest string, dirs []string, open []*Block) (ulid.ULID, error)
}

// LeveledCompactor implements the Compactor interface.
type LeveledCompactor struct {
	metrics                     *CompactorMetrics
	logger                      log.Logger
	ranges                      []int64
	chunkPool                   chunkenc.Pool
	ctx                         context.Context
	maxBlockChunkSegmentSize    int64
	mergeFunc                   storage.VerticalChunkSeriesMergeFunc
	postingsEncoder             index.PostingsEncoder
	enableOverlappingCompaction bool
}

type CompactorMetrics struct {
	Ran               prometheus.Counter
	PopulatingBlocks  prometheus.Gauge
	OverlappingBlocks prometheus.Counter
	Duration          prometheus.Histogram
	ChunkSize         prometheus.Histogram
	ChunkSamples      prometheus.Histogram
	ChunkRange        prometheus.Histogram
}

// NewCompactorMetrics initializes metrics for Compactor.
func NewCompactorMetrics(r prometheus.Registerer) *CompactorMetrics {
	m := &CompactorMetrics{}

	m.Ran = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_compactions_total",
		Help: "Total number of compactions that were executed for the partition.",
	})
	m.PopulatingBlocks = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_compaction_populating_block",
		Help: "Set to 1 when a block is currently being written to the disk.",
	})
	m.OverlappingBlocks = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_vertical_compactions_total",
		Help: "Total number of compactions done on overlapping blocks.",
	})
	m.Duration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:                            "prometheus_tsdb_compaction_duration_seconds",
		Help:                            "Duration of compaction runs",
		Buckets:                         prometheus.ExponentialBuckets(1, 2, 14),
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  100,
		NativeHistogramMinResetDuration: 1 * time.Hour,
	})
	m.ChunkSize = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "prometheus_tsdb_compaction_chunk_size_bytes",
		Help:    "Final size of chunks on their first compaction",
		Buckets: prometheus.ExponentialBuckets(32, 1.5, 12),
	})
	m.ChunkSamples = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "prometheus_tsdb_compaction_chunk_samples",
		Help:    "Final number of samples on their first compaction",
		Buckets: prometheus.ExponentialBuckets(4, 1.5, 12),
	})
	m.ChunkRange = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "prometheus_tsdb_compaction_chunk_range_seconds",
		Help:    "Final time range of chunks on their first compaction",
		Buckets: prometheus.ExponentialBuckets(100, 4, 10),
	})

	if r != nil {
		r.MustRegister(
			m.Ran,
			m.PopulatingBlocks,
			m.OverlappingBlocks,
			m.Duration,
			m.ChunkRange,
			m.ChunkSamples,
			m.ChunkSize,
		)
	}
	return m
}

type LeveledCompactorOptions struct {
	// PE specifies the postings encoder. It is called when compactor is writing out the postings for a label name/value pair during compaction.
	// If it is nil then the default encoder is used. At the moment that is the "raw" encoder. See index.EncodePostingsRaw for more.
	PE index.PostingsEncoder
	// MaxBlockChunkSegmentSize is the max block chunk segment size. If it is 0 then the default chunks.DefaultChunkSegmentSize is used.
	MaxBlockChunkSegmentSize int64
	// MergeFunc is used for merging series together in vertical compaction. By default storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge) is used.
	MergeFunc storage.VerticalChunkSeriesMergeFunc
	// EnableOverlappingCompaction enables compaction of overlapping blocks. In Prometheus it is always enabled.
	// It is useful for downstream projects like Mimir, Cortex, Thanos where they have a separate component that does compaction.
	EnableOverlappingCompaction bool
}

func NewLeveledCompactorWithChunkSize(ctx context.Context, r prometheus.Registerer, l log.Logger, ranges []int64, pool chunkenc.Pool, maxBlockChunkSegmentSize int64, mergeFunc storage.VerticalChunkSeriesMergeFunc) (*LeveledCompactor, error) {
	return NewLeveledCompactorWithOptions(ctx, r, l, ranges, pool, LeveledCompactorOptions{
		MaxBlockChunkSegmentSize:    maxBlockChunkSegmentSize,
		MergeFunc:                   mergeFunc,
		EnableOverlappingCompaction: true,
	})
}

func NewLeveledCompactor(ctx context.Context, r prometheus.Registerer, l log.Logger, ranges []int64, pool chunkenc.Pool, mergeFunc storage.VerticalChunkSeriesMergeFunc) (*LeveledCompactor, error) {
	return NewLeveledCompactorWithOptions(ctx, r, l, ranges, pool, LeveledCompactorOptions{
		MergeFunc:                   mergeFunc,
		EnableOverlappingCompaction: true,
	})
}

func NewLeveledCompactorWithOptions(ctx context.Context, r prometheus.Registerer, l log.Logger, ranges []int64, pool chunkenc.Pool, opts LeveledCompactorOptions) (*LeveledCompactor, error) {
	if len(ranges) == 0 {
		return nil, fmt.Errorf("at least one range must be provided")
	}
	if pool == nil {
		pool = chunkenc.NewPool()
	}
	if l == nil {
		l = log.NewNopLogger()
	}
	mergeFunc := opts.MergeFunc
	if mergeFunc == nil {
		mergeFunc = storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge)
	}
	maxBlockChunkSegmentSize := opts.MaxBlockChunkSegmentSize
	if maxBlockChunkSegmentSize == 0 {
		maxBlockChunkSegmentSize = chunks.DefaultChunkSegmentSize
	}
	pe := opts.PE
	if pe == nil {
		pe = index.EncodePostingsRaw
	}
	return &LeveledCompactor{
		ranges:                      ranges,
		chunkPool:                   pool,
		logger:                      l,
		metrics:                     NewCompactorMetrics(r),
		ctx:                         ctx,
		maxBlockChunkSegmentSize:    maxBlockChunkSegmentSize,
		mergeFunc:                   mergeFunc,
		postingsEncoder:             pe,
		enableOverlappingCompaction: opts.EnableOverlappingCompaction,
	}, nil
}

type dirMeta struct {
	dir  string
	meta *BlockMeta
}

// Plan returns a list of compactable blocks in the provided directory.
func (c *LeveledCompactor) Plan(dir string) ([]string, error) {
	dirs, err := blockDirs(dir)
	if err != nil {
		return nil, err
	}
	if len(dirs) < 1 {
		return nil, nil
	}

	var dms []dirMeta
	for _, dir := range dirs {
		meta, _, err := readMetaFile(dir)
		if err != nil {
			return nil, err
		}
		dms = append(dms, dirMeta{dir, meta})
	}
	return c.plan(dms)
}

func (c *LeveledCompactor) plan(dms []dirMeta) ([]string, error) {
	slices.SortFunc(dms, func(a, b dirMeta) int {
		switch {
		case a.meta.MinTime < b.meta.MinTime:
			return -1
		case a.meta.MinTime > b.meta.MinTime:
			return 1
		default:
			return 0
		}
	})

	res := c.selectOverlappingDirs(dms)
	if len(res) > 0 {
		return res, nil
	}
	// No overlapping blocks, do compaction the usual way.
	// We do not include a recently created block with max(minTime), so the block which was just created from WAL.
	// This gives users a window of a full block size to piece-wise backup new data without having to care about data overlap.
	dms = dms[:len(dms)-1]

	for _, dm := range c.selectDirs(dms) {
		res = append(res, dm.dir)
	}
	if len(res) > 0 {
		return res, nil
	}

	// Compact any blocks with big enough time range that have >5% tombstones.
	for i := len(dms) - 1; i >= 0; i-- {
		meta := dms[i].meta
		if meta.MaxTime-meta.MinTime < c.ranges[len(c.ranges)/2] {
			// If the block is entirely deleted, then we don't care about the block being big enough.
			// TODO: This is assuming single tombstone is for distinct series, which might be no true.
			if meta.Stats.NumTombstones > 0 && meta.Stats.NumTombstones >= meta.Stats.NumSeries {
				return []string{dms[i].dir}, nil
			}
			break
		}
		if float64(meta.Stats.NumTombstones)/float64(meta.Stats.NumSeries+1) > 0.05 {
			return []string{dms[i].dir}, nil
		}
	}

	return nil, nil
}

// selectDirs returns the dir metas that should be compacted into a single new block.
// If only a single block range is configured, the result is always nil.
func (c *LeveledCompactor) selectDirs(ds []dirMeta) []dirMeta {
	if len(c.ranges) < 2 || len(ds) < 1 {
		return nil
	}

	highTime := ds[len(ds)-1].meta.MinTime

	for _, iv := range c.ranges[1:] {
		parts := splitByRange(ds, iv)
		if len(parts) == 0 {
			continue
		}

	Outer:
		for _, p := range parts {
			// Do not select the range if it has a block whose compaction failed.
			for _, dm := range p {
				if dm.meta.Compaction.Failed {
					continue Outer
				}
			}

			mint := p[0].meta.MinTime
			maxt := p[len(p)-1].meta.MaxTime
			// Pick the range of blocks if it spans the full range (potentially with gaps)
			// or is before the most recent block.
			// This ensures we don't compact blocks prematurely when another one of the same
			// size still fits in the range.
			if (maxt-mint == iv || maxt <= highTime) && len(p) > 1 {
				return p
			}
		}
	}

	return nil
}

// selectOverlappingDirs returns all dirs with overlapping time ranges.
// It expects sorted input by mint and returns the overlapping dirs in the same order as received.
func (c *LeveledCompactor) selectOverlappingDirs(ds []dirMeta) []string {
	if !c.enableOverlappingCompaction {
		return nil
	}
	if len(ds) < 2 {
		return nil
	}
	var overlappingDirs []string
	globalMaxt := ds[0].meta.MaxTime
	for i, d := range ds[1:] {
		if d.meta.MinTime < globalMaxt {
			if len(overlappingDirs) == 0 { // When it is the first overlap, need to add the last one as well.
				overlappingDirs = append(overlappingDirs, ds[i].dir)
			}
			overlappingDirs = append(overlappingDirs, d.dir)
		} else if len(overlappingDirs) > 0 {
			break
		}
		if d.meta.MaxTime > globalMaxt {
			globalMaxt = d.meta.MaxTime
		}
	}
	return overlappingDirs
}

// splitByRange splits the directories by the time range. The range sequence starts at 0.
//
// For example, if we have blocks [0-10, 10-20, 50-60, 90-100] and the split range tr is 30
// it returns [0-10, 10-20], [50-60], [90-100].
func splitByRange(ds []dirMeta, tr int64) [][]dirMeta {
	var splitDirs [][]dirMeta

	for i := 0; i < len(ds); {
		var (
			group []dirMeta
			t0    int64
			m     = ds[i].meta
		)
		// Compute start of aligned time range of size tr closest to the current block's start.
		if m.MinTime >= 0 {
			t0 = tr * (m.MinTime / tr)
		} else {
			t0 = tr * ((m.MinTime - tr + 1) / tr)
		}
		// Skip blocks that don't fall into the range. This can happen via mis-alignment or
		// by being the multiple of the intended range.
		if m.MaxTime > t0+tr {
			i++
			continue
		}

		// Add all dirs to the current group that are within [t0, t0+tr].
		for ; i < len(ds); i++ {
			// Either the block falls into the next range or doesn't fit at all (checked above).
			if ds[i].meta.MaxTime > t0+tr {
				break
			}
			group = append(group, ds[i])
		}

		if len(group) > 0 {
			splitDirs = append(splitDirs, group)
		}
	}

	return splitDirs
}

// CompactBlockMetas merges many block metas into one, combining it's source blocks together
// and adjusting compaction level. Min/Max time of result block meta covers all input blocks.
func CompactBlockMetas(uid ulid.ULID, blocks ...*BlockMeta) *BlockMeta {
	res := &BlockMeta{
		ULID: uid,
	}

	sources := map[ulid.ULID]struct{}{}
	mint := blocks[0].MinTime
	maxt := blocks[0].MaxTime

	for _, b := range blocks {
		if b.MinTime < mint {
			mint = b.MinTime
		}
		if b.MaxTime > maxt {
			maxt = b.MaxTime
		}
		if b.Compaction.Level > res.Compaction.Level {
			res.Compaction.Level = b.Compaction.Level
		}
		for _, s := range b.Compaction.Sources {
			sources[s] = struct{}{}
		}
		res.Compaction.Parents = append(res.Compaction.Parents, BlockDesc{
			ULID:    b.ULID,
			MinTime: b.MinTime,
			MaxTime: b.MaxTime,
		})
	}
	res.Compaction.Level++

	for s := range sources {
		res.Compaction.Sources = append(res.Compaction.Sources, s)
	}
	slices.SortFunc(res.Compaction.Sources, func(a, b ulid.ULID) int {
		return a.Compare(b)
	})

	res.MinTime = mint
	res.MaxTime = maxt
	return res
}

// Compact creates a new block in the compactor's directory from the blocks in the
// provided directories.
func (c *LeveledCompactor) Compact(dest string, dirs []string, open []*Block) (uid ulid.ULID, err error) {
	return c.CompactWithBlockPopulator(dest, dirs, open, DefaultBlockPopulator{})
}

func (c *LeveledCompactor) CompactWithBlockPopulator(dest string, dirs []string, open []*Block, blockPopulator BlockPopulator) (uid ulid.ULID, err error) {
	var (
		blocks []BlockReader
		bs     []*Block
		metas  []*BlockMeta
		uids   []string
	)
	start := time.Now()

	for _, d := range dirs {
		meta, _, err := readMetaFile(d)
		if err != nil {
			return uid, err
		}

		var b *Block

		// Use already open blocks if we can, to avoid
		// having the index data in memory twice.
		for _, o := range open {
			if meta.ULID == o.Meta().ULID {
				b = o
				break
			}
		}

		if b == nil {
			var err error
			b, err = OpenBlock(c.logger, d, c.chunkPool)
			if err != nil {
				return uid, err
			}
			defer b.Close()
		}

		metas = append(metas, meta)
		blocks = append(blocks, b)
		bs = append(bs, b)
		uids = append(uids, meta.ULID.String())
	}

	uid = ulid.MustNew(ulid.Now(), rand.Reader)

	meta := CompactBlockMetas(uid, metas...)
	err = c.write(dest, meta, blockPopulator, blocks...)
	if err == nil {
		if meta.Stats.NumSamples == 0 {
			for _, b := range bs {
				b.meta.Compaction.Deletable = true
				n, err := writeMetaFile(c.logger, b.dir, &b.meta)
				if err != nil {
					level.Error(c.logger).Log(
						"msg", "Failed to write 'Deletable' to meta file after compaction",
						"ulid", b.meta.ULID,
					)
				}
				b.numBytesMeta = n
			}
			uid = ulid.ULID{}
			level.Info(c.logger).Log(
				"msg", "compact blocks resulted in empty block",
				"count", len(blocks),
				"sources", fmt.Sprintf("%v", uids),
				"duration", time.Since(start),
			)
		} else {
			level.Info(c.logger).Log(
				"msg", "compact blocks",
				"count", len(blocks),
				"mint", meta.MinTime,
				"maxt", meta.MaxTime,
				"ulid", meta.ULID,
				"sources", fmt.Sprintf("%v", uids),
				"duration", time.Since(start),
			)
		}
		return uid, nil
	}

	errs := tsdb_errors.NewMulti(err)
	if !errors.Is(err, context.Canceled) {
		for _, b := range bs {
			if err := b.setCompactionFailed(); err != nil {
				errs.Add(fmt.Errorf("setting compaction failed for block: %s: %w", b.Dir(), err))
			}
		}
	}

	return uid, errs.Err()
}

func (c *LeveledCompactor) Write(dest string, b BlockReader, mint, maxt int64, base *BlockMeta) (ulid.ULID, error) {
	start := time.Now()

	uid := ulid.MustNew(ulid.Now(), rand.Reader)

	meta := &BlockMeta{
		ULID:    uid,
		MinTime: mint,
		MaxTime: maxt,
	}
	meta.Compaction.Level = 1
	meta.Compaction.Sources = []ulid.ULID{uid}

	if base != nil {
		meta.Compaction.Parents = []BlockDesc{
			{ULID: base.ULID, MinTime: base.MinTime, MaxTime: base.MaxTime},
		}
		if base.Compaction.FromOutOfOrder() {
			meta.Compaction.SetOutOfOrder()
		}
	}

	err := c.write(dest, meta, DefaultBlockPopulator{}, b)
	if err != nil {
		return uid, err
	}

	if meta.Stats.NumSamples == 0 {
		level.Info(c.logger).Log(
			"msg", "write block resulted in empty block",
			"mint", meta.MinTime,
			"maxt", meta.MaxTime,
			"duration", time.Since(start),
		)
		return ulid.ULID{}, nil
	}

	level.Info(c.logger).Log(
		"msg", "write block",
		"mint", meta.MinTime,
		"maxt", meta.MaxTime,
		"ulid", meta.ULID,
		"duration", time.Since(start),
		"ooo", meta.Compaction.FromOutOfOrder(),
	)
	return uid, nil
}

// instrumentedChunkWriter is used for level 1 compactions to record statistics
// about compacted chunks.
type instrumentedChunkWriter struct {
	ChunkWriter

	size    prometheus.Histogram
	samples prometheus.Histogram
	trange  prometheus.Histogram
}

func (w *instrumentedChunkWriter) WriteChunks(chunks ...chunks.Meta) error {
	for _, c := range chunks {
		w.size.Observe(float64(len(c.Chunk.Bytes())))
		w.samples.Observe(float64(c.Chunk.NumSamples()))
		w.trange.Observe(float64(c.MaxTime - c.MinTime))
	}
	return w.ChunkWriter.WriteChunks(chunks...)
}

// write creates a new block that is the union of the provided blocks into dir.
func (c *LeveledCompactor) write(dest string, meta *BlockMeta, blockPopulator BlockPopulator, blocks ...BlockReader) (err error) {
	dir := filepath.Join(dest, meta.ULID.String())
	tmp := dir + tmpForCreationBlockDirSuffix
	var closers []io.Closer
	defer func(t time.Time) {
		err = tsdb_errors.NewMulti(err, tsdb_errors.CloseAll(closers)).Err()

		// RemoveAll returns no error when tmp doesn't exist so it is safe to always run it.
		if err := os.RemoveAll(tmp); err != nil {
			level.Error(c.logger).Log("msg", "removed tmp folder after failed compaction", "err", err.Error())
		}
		c.metrics.Ran.Inc()
		c.metrics.Duration.Observe(time.Since(t).Seconds())
	}(time.Now())

	if err = os.RemoveAll(tmp); err != nil {
		return err
	}

	if err = os.MkdirAll(tmp, 0o777); err != nil {
		return err
	}

	// Populate chunk and index files into temporary directory with
	// data of all blocks.
	var chunkw ChunkWriter

	chunkw, err = chunks.NewWriterWithSegSize(chunkDir(tmp), c.maxBlockChunkSegmentSize)
	if err != nil {
		return fmt.Errorf("open chunk writer: %w", err)
	}
	closers = append(closers, chunkw)
	// Record written chunk sizes on level 1 compactions.
	if meta.Compaction.Level == 1 {
		chunkw = &instrumentedChunkWriter{
			ChunkWriter: chunkw,
			size:        c.metrics.ChunkSize,
			samples:     c.metrics.ChunkSamples,
			trange:      c.metrics.ChunkRange,
		}
	}

	indexw, err := index.NewWriterWithEncoder(c.ctx, filepath.Join(tmp, indexFilename), c.postingsEncoder)
	if err != nil {
		return fmt.Errorf("open index writer: %w", err)
	}
	closers = append(closers, indexw)

	if err := blockPopulator.PopulateBlock(c.ctx, c.metrics, c.logger, c.chunkPool, c.mergeFunc, blocks, meta, indexw, chunkw); err != nil {
		return fmt.Errorf("populate block: %w", err)
	}

	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	default:
	}

	// We are explicitly closing them here to check for error even
	// though these are covered under defer. This is because in Windows,
	// you cannot delete these unless they are closed and the defer is to
	// make sure they are closed if the function exits due to an error above.
	errs := tsdb_errors.NewMulti()
	for _, w := range closers {
		errs.Add(w.Close())
	}
	closers = closers[:0] // Avoid closing the writers twice in the defer.
	if errs.Err() != nil {
		return errs.Err()
	}

	// Populated block is empty, so exit early.
	if meta.Stats.NumSamples == 0 {
		return nil
	}

	if _, err = writeMetaFile(c.logger, tmp, meta); err != nil {
		return fmt.Errorf("write merged meta: %w", err)
	}

	// Create an empty tombstones file.
	if _, err := tombstones.WriteFile(c.logger, tmp, tombstones.NewMemTombstones()); err != nil {
		return fmt.Errorf("write new tombstones file: %w", err)
	}

	df, err := fileutil.OpenDir(tmp)
	if err != nil {
		return fmt.Errorf("open temporary block dir: %w", err)
	}
	defer func() {
		if df != nil {
			df.Close()
		}
	}()

	if err := df.Sync(); err != nil {
		return fmt.Errorf("sync temporary dir file: %w", err)
	}

	// Close temp dir before rename block dir (for windows platform).
	if err = df.Close(); err != nil {
		return fmt.Errorf("close temporary dir: %w", err)
	}
	df = nil

	// Block successfully written, make it visible in destination dir by moving it from tmp one.
	if err := fileutil.Replace(tmp, dir); err != nil {
		return fmt.Errorf("rename block dir: %w", err)
	}

	return nil
}

type BlockPopulator interface {
	PopulateBlock(ctx context.Context, metrics *CompactorMetrics, logger log.Logger, chunkPool chunkenc.Pool, mergeFunc storage.VerticalChunkSeriesMergeFunc, blocks []BlockReader, meta *BlockMeta, indexw IndexWriter, chunkw ChunkWriter) error
}

type DefaultBlockPopulator struct{}

// PopulateBlock fills the index and chunk writers with new data gathered as the union
// of the provided blocks. It returns meta information for the new block.
// It expects sorted blocks input by mint.
func (c DefaultBlockPopulator) PopulateBlock(ctx context.Context, metrics *CompactorMetrics, logger log.Logger, chunkPool chunkenc.Pool, mergeFunc storage.VerticalChunkSeriesMergeFunc, blocks []BlockReader, meta *BlockMeta, indexw IndexWriter, chunkw ChunkWriter) (err error) {
	if len(blocks) == 0 {
		return errors.New("cannot populate block from no readers")
	}

	var (
		sets        []storage.ChunkSeriesSet
		symbols     index.StringIter
		closers     []io.Closer
		overlapping bool
	)
	defer func() {
		errs := tsdb_errors.NewMulti(err)
		if cerr := tsdb_errors.CloseAll(closers); cerr != nil {
			errs.Add(fmt.Errorf("close: %w", cerr))
		}
		err = errs.Err()
		metrics.PopulatingBlocks.Set(0)
	}()
	metrics.PopulatingBlocks.Set(1)

	globalMaxt := blocks[0].Meta().MaxTime
	for i, b := range blocks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !overlapping {
			if i > 0 && b.Meta().MinTime < globalMaxt {
				metrics.OverlappingBlocks.Inc()
				overlapping = true
				level.Info(logger).Log("msg", "Found overlapping blocks during compaction", "ulid", meta.ULID)
			}
			if b.Meta().MaxTime > globalMaxt {
				globalMaxt = b.Meta().MaxTime
			}
		}

		indexr, err := b.Index()
		if err != nil {
			return fmt.Errorf("open index reader for block %+v: %w", b.Meta(), err)
		}
		closers = append(closers, indexr)

		chunkr, err := b.Chunks()
		if err != nil {
			return fmt.Errorf("open chunk reader for block %+v: %w", b.Meta(), err)
		}
		closers = append(closers, chunkr)

		tombsr, err := b.Tombstones()
		if err != nil {
			return fmt.Errorf("open tombstone reader for block %+v: %w", b.Meta(), err)
		}
		closers = append(closers, tombsr)

		k, v := index.AllPostingsKey()
		all, err := indexr.Postings(ctx, k, v)
		if err != nil {
			return err
		}
		all = indexr.SortedPostings(all)
		// Blocks meta is half open: [min, max), so subtract 1 to ensure we don't hold samples with exact meta.MaxTime timestamp.
		sets = append(sets, NewBlockChunkSeriesSet(b.Meta().ULID, indexr, chunkr, tombsr, all, meta.MinTime, meta.MaxTime-1, false))
		syms := indexr.Symbols()
		if i == 0 {
			symbols = syms
			continue
		}
		symbols = NewMergedStringIter(symbols, syms)
	}

	for symbols.Next() {
		if err := indexw.AddSymbol(symbols.At()); err != nil {
			return fmt.Errorf("add symbol: %w", err)
		}
	}
	if err := symbols.Err(); err != nil {
		return fmt.Errorf("next symbol: %w", err)
	}

	var (
		ref      = storage.SeriesRef(0)
		chks     []chunks.Meta
		chksIter chunks.Iterator
	)

	set := sets[0]
	if len(sets) > 1 {
		// Merge series using specified chunk series merger.
		// The default one is the compacting series merger.
		set = storage.NewMergeChunkSeriesSet(sets, mergeFunc)
	}

	// Iterate over all sorted chunk series.
	for set.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		s := set.At()
		chksIter = s.Iterator(chksIter)
		chks = chks[:0]
		for chksIter.Next() {
			// We are not iterating in streaming way over chunk as
			// it's more efficient to do bulk write for index and
			// chunk file purposes.
			chks = append(chks, chksIter.At())
		}
		if err := chksIter.Err(); err != nil {
			return fmt.Errorf("chunk iter: %w", err)
		}

		// Skip the series with all deleted chunks.
		if len(chks) == 0 {
			continue
		}

		if err := chunkw.WriteChunks(chks...); err != nil {
			return fmt.Errorf("write chunks: %w", err)
		}
		if err := indexw.AddSeries(ref, s.Labels(), chks...); err != nil {
			return fmt.Errorf("add series: %w", err)
		}

		meta.Stats.NumChunks += uint64(len(chks))
		meta.Stats.NumSeries++
		for _, chk := range chks {
			meta.Stats.NumSamples += uint64(chk.Chunk.NumSamples())
		}

		for _, chk := range chks {
			if err := chunkPool.Put(chk.Chunk); err != nil {
				return fmt.Errorf("put chunk: %w", err)
			}
		}
		ref++
	}
	if err := set.Err(); err != nil {
		return fmt.Errorf("iterate compaction set: %w", err)
	}

	return nil
}
