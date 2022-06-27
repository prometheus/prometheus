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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
	"golang.org/x/sync/semaphore"

	"github.com/prometheus/prometheus/model/labels"
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
		curRange = curRange * int64(stepSize)
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
	Write(dest string, b BlockReader, mint, maxt int64, parent *BlockMeta) (ulid.ULID, error)

	// Compact runs compaction against the provided directories. Must
	// only be called concurrently with results of Plan().
	// Can optionally pass a list of already open blocks,
	// to avoid having to reopen them.
	// When resulting Block has 0 samples
	//  * No block is written.
	//  * The source dirs are marked Deletable.
	//  * Returns empty ulid.ULID{}.
	Compact(dest string, dirs []string, open []*Block) (ulid.ULID, error)

	// CompactOOO creates a new block per possible block range in the compactor's directory from the OOO Head given.
	// Each ULID in the result corresponds to a block in a unique time range.
	CompactOOO(dest string, oooHead *OOOCompactionHead) (result []ulid.ULID, err error)
}

// LeveledCompactor implements the Compactor interface.
type LeveledCompactor struct {
	metrics                     *compactorMetrics
	logger                      log.Logger
	ranges                      []int64
	chunkPool                   chunkenc.Pool
	ctx                         context.Context
	maxBlockChunkSegmentSize    int64
	mergeFunc                   storage.VerticalChunkSeriesMergeFunc
	enableOverlappingCompaction bool

	concurrencyOpts LeveledCompactorConcurrencyOptions
}

type compactorMetrics struct {
	ran               prometheus.Counter
	populatingBlocks  prometheus.Gauge
	overlappingBlocks prometheus.Counter
	duration          prometheus.Histogram
	chunkSize         prometheus.Histogram
	chunkSamples      prometheus.Histogram
	chunkRange        prometheus.Histogram
}

func newCompactorMetrics(r prometheus.Registerer) *compactorMetrics {
	m := &compactorMetrics{}

	m.ran = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_compactions_total",
		Help: "Total number of compactions that were executed for the partition.",
	})
	m.populatingBlocks = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_compaction_populating_block",
		Help: "Set to 1 when a block is currently being written to the disk.",
	})
	m.overlappingBlocks = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_vertical_compactions_total",
		Help: "Total number of compactions done on overlapping blocks.",
	})
	m.duration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "prometheus_tsdb_compaction_duration_seconds",
		Help:    "Duration of compaction runs",
		Buckets: prometheus.ExponentialBuckets(1, 2, 14),
	})
	m.chunkSize = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "prometheus_tsdb_compaction_chunk_size_bytes",
		Help:    "Final size of chunks on their first compaction",
		Buckets: prometheus.ExponentialBuckets(32, 1.5, 12),
	})
	m.chunkSamples = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "prometheus_tsdb_compaction_chunk_samples",
		Help:    "Final number of samples on their first compaction",
		Buckets: prometheus.ExponentialBuckets(4, 1.5, 12),
	})
	m.chunkRange = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "prometheus_tsdb_compaction_chunk_range_seconds",
		Help:    "Final time range of chunks on their first compaction",
		Buckets: prometheus.ExponentialBuckets(100, 4, 10),
	})

	if r != nil {
		r.MustRegister(
			m.ran,
			m.populatingBlocks,
			m.overlappingBlocks,
			m.duration,
			m.chunkRange,
			m.chunkSamples,
			m.chunkSize,
		)
	}
	return m
}

// NewLeveledCompactor returns a LeveledCompactor.
func NewLeveledCompactor(ctx context.Context, r prometheus.Registerer, l log.Logger, ranges []int64, pool chunkenc.Pool, mergeFunc storage.VerticalChunkSeriesMergeFunc, enableOverlappingCompaction bool) (*LeveledCompactor, error) {
	return NewLeveledCompactorWithChunkSize(ctx, r, l, ranges, pool, chunks.DefaultChunkSegmentSize, mergeFunc, enableOverlappingCompaction)
}

func NewLeveledCompactorWithChunkSize(ctx context.Context, r prometheus.Registerer, l log.Logger, ranges []int64, pool chunkenc.Pool, maxBlockChunkSegmentSize int64, mergeFunc storage.VerticalChunkSeriesMergeFunc, enableOverlappingCompaction bool) (*LeveledCompactor, error) {
	if len(ranges) == 0 {
		return nil, errors.Errorf("at least one range must be provided")
	}
	if pool == nil {
		pool = chunkenc.NewPool()
	}
	if l == nil {
		l = log.NewNopLogger()
	}
	if mergeFunc == nil {
		mergeFunc = storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge)
	}
	return &LeveledCompactor{
		ranges:                      ranges,
		chunkPool:                   pool,
		logger:                      l,
		metrics:                     newCompactorMetrics(r),
		ctx:                         ctx,
		maxBlockChunkSegmentSize:    maxBlockChunkSegmentSize,
		mergeFunc:                   mergeFunc,
		concurrencyOpts:             DefaultLeveledCompactorConcurrencyOptions(),
		enableOverlappingCompaction: enableOverlappingCompaction,
	}, nil
}

// LeveledCompactorConcurrencyOptions is a collection of concurrency options used by LeveledCompactor.
type LeveledCompactorConcurrencyOptions struct {
	MaxOpeningBlocks     int // Number of goroutines opening blocks before compaction.
	MaxClosingBlocks     int // Max number of blocks that can be closed concurrently during split compaction. Note that closing of newly compacted block uses a lot of memory for writing index.
	SymbolsFlushersCount int // Number of symbols flushers used when doing split compaction.
}

func DefaultLeveledCompactorConcurrencyOptions() LeveledCompactorConcurrencyOptions {
	return LeveledCompactorConcurrencyOptions{
		MaxClosingBlocks:     1,
		SymbolsFlushersCount: 1,
		MaxOpeningBlocks:     1,
	}
}

func (c *LeveledCompactor) SetConcurrencyOptions(opts LeveledCompactorConcurrencyOptions) {
	c.concurrencyOpts = opts
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
	sort.Slice(dms, func(i, j int) bool {
		return dms[i].meta.MinTime < dms[j].meta.MinTime
	})

	res := c.selectOverlappingDirs(dms)
	if len(res) > 0 {
		return res, nil
	}
	// No overlapping blocks or overlapping block compaction not allowed, do compaction the usual way.
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
	sort.Slice(res.Compaction.Sources, func(i, j int) bool {
		return res.Compaction.Sources[i].Compare(res.Compaction.Sources[j]) < 0
	})

	res.MinTime = mint
	res.MaxTime = maxt
	return res
}

// CompactWithSplitting merges and splits the input blocks into shardCount number of output blocks,
// and returns slice of block IDs. Position of returned block ID in the result slice corresponds to the shard index.
// If given output block has no series, corresponding block ID will be zero ULID value.
func (c *LeveledCompactor) CompactWithSplitting(dest string, dirs []string, open []*Block, shardCount uint64) (result []ulid.ULID, _ error) {
	return c.compact(dest, dirs, open, shardCount)
}

// Compact creates a new block in the compactor's directory from the blocks in the
// provided directories.
func (c *LeveledCompactor) Compact(dest string, dirs []string, open []*Block) (uid ulid.ULID, err error) {
	ulids, err := c.compact(dest, dirs, open, 1)
	if err != nil {
		return ulid.ULID{}, err
	}
	return ulids[0], nil
}

// shardedBlock describes single *output* block during compaction. This struct is passed between
// compaction methods to wrap output block details, index and chunk writer together.
// Shard index is determined by the position of this structure in the slice of output blocks.
type shardedBlock struct {
	meta *BlockMeta

	blockDir string
	tmpDir   string // Temp directory used when block is being built (= blockDir + temp suffix)
	chunkw   ChunkWriter
	indexw   IndexWriter
}

func (c *LeveledCompactor) compact(dest string, dirs []string, open []*Block, shardCount uint64) (_ []ulid.ULID, err error) {
	if shardCount == 0 {
		shardCount = 1
	}

	start := time.Now()

	bs, blocksToClose, err := openBlocksForCompaction(dirs, open, c.logger, c.chunkPool, c.concurrencyOpts.MaxOpeningBlocks)
	for _, b := range blocksToClose {
		defer b.Close()
	}

	if err != nil {
		return nil, err
	}

	var (
		blocks []BlockReader
		metas  []*BlockMeta
		uids   []string
	)
	for _, b := range bs {
		blocks = append(blocks, b)
		m := b.Meta()
		metas = append(metas, &m)
		uids = append(uids, b.meta.ULID.String())
	}

	outBlocks := make([]shardedBlock, shardCount)
	outBlocksTime := ulid.Now() // Make all out blocks share the same timestamp in the ULID.
	for ix := range outBlocks {
		outBlocks[ix] = shardedBlock{meta: CompactBlockMetas(ulid.MustNew(outBlocksTime, rand.Reader), metas...)}
	}

	err = c.write(dest, outBlocks, blocks...)
	if err == nil {
		ulids := make([]ulid.ULID, len(outBlocks))
		allOutputBlocksAreEmpty := true

		for ix := range outBlocks {
			meta := outBlocks[ix].meta

			if meta.Stats.NumSamples == 0 {
				level.Info(c.logger).Log(
					"msg", "compact blocks resulted in empty block",
					"count", len(blocks),
					"sources", fmt.Sprintf("%v", uids),
					"duration", time.Since(start),
					"shard", fmt.Sprintf("%d_of_%d", ix+1, shardCount),
				)
			} else {
				allOutputBlocksAreEmpty = false
				ulids[ix] = outBlocks[ix].meta.ULID

				level.Info(c.logger).Log(
					"msg", "compact blocks",
					"count", len(blocks),
					"mint", meta.MinTime,
					"maxt", meta.MaxTime,
					"ulid", meta.ULID,
					"sources", fmt.Sprintf("%v", uids),
					"duration", time.Since(start),
					"shard", fmt.Sprintf("%d_of_%d", ix+1, shardCount),
				)
			}
		}

		if allOutputBlocksAreEmpty {
			// Mark source blocks as deletable.
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
		}

		return ulids, nil
	}

	errs := tsdb_errors.NewMulti(err)
	if err != context.Canceled {
		for _, b := range bs {
			if err := b.setCompactionFailed(); err != nil {
				errs.Add(errors.Wrapf(err, "setting compaction failed for block: %s", b.Dir()))
			}
		}
	}

	return nil, errs.Err()
}

// CompactOOOWithSplitting splits the input OOO Head into shardCount number of output blocks
// per possible block range, and returns slice of block IDs. In result[i][j],
// 'i' corresponds to a single time range of blocks while 'j' corresponds to the shard index.
// If given output block has no series, corresponding block ID will be zero ULID value.
// TODO: write tests for this.
func (c *LeveledCompactor) CompactOOOWithSplitting(dest string, oooHead *OOOCompactionHead, shardCount uint64) (result [][]ulid.ULID, _ error) {
	return c.compactOOO(dest, oooHead, shardCount)
}

// CompactOOO creates a new block per possible block range in the compactor's directory from the OOO Head given.
// Each ULID in the result corresponds to a block in a unique time range.
func (c *LeveledCompactor) CompactOOO(dest string, oooHead *OOOCompactionHead) (result []ulid.ULID, err error) {
	ulids, err := c.compactOOO(dest, oooHead, 1)
	if err != nil {
		return nil, err
	}
	for _, s := range ulids {
		if s[0].Compare(ulid.ULID{}) != 0 {
			result = append(result, s[0])
		}
	}
	return result, err
}

func (c *LeveledCompactor) compactOOO(dest string, oooHead *OOOCompactionHead, shardCount uint64) (_ [][]ulid.ULID, err error) {
	if shardCount == 0 {
		shardCount = 1
	}

	start := time.Now()

	if err != nil {
		return nil, err
	}

	// The first dimension of outBlocks determines the time based splitting (i.e. outBlocks[i] has blocks all for the same time range).
	// The second dimension of outBlocks determines the label based shard (i.e. outBlocks[i][j] is the (j+1)th shard.
	// During ingestion of samples we can identify which ooo blocks will exists so that
	// we dont have to prefill symbols and etc for the blocks that will be empty.
	// With this, len(outBlocks[x]) will still be the same for all x so that we can pick blocks easily.
	// Just that, only some of the outBlocks[x][y] will be valid and populated based on preexisting knowledge of
	// which blocks to expect.
	// In case we see a sample that is not present in the estimated block ranges, we will create them on flight.
	outBlocks := make([][]shardedBlock, 0)
	outBlocksTime := ulid.Now() // Make all out blocks share the same timestamp in the ULID.
	blockSize := oooHead.ChunkRange()
	oooHeadMint, oooHeadMaxt := oooHead.MinTime(), oooHead.MaxTime()
	ulids := make([][]ulid.ULID, 0)
	for t := blockSize * (oooHeadMint / blockSize); t <= oooHeadMaxt; t = t + blockSize {
		mint, maxt := t, t+blockSize

		outBlocks = append(outBlocks, make([]shardedBlock, shardCount))
		ulids = append(ulids, make([]ulid.ULID, shardCount))
		ix := len(outBlocks) - 1

		for jx := range outBlocks[ix] {
			uid := ulid.MustNew(outBlocksTime, rand.Reader)
			meta := &BlockMeta{
				ULID:       uid,
				MinTime:    mint,
				MaxTime:    maxt,
				OutOfOrder: true,
			}
			meta.Compaction.Level = 1
			meta.Compaction.Sources = []ulid.ULID{uid}

			outBlocks[ix][jx] = shardedBlock{
				meta: meta,
			}
			ulids[ix][jx] = meta.ULID
		}

		// Block intervals are half-open: [b.MinTime, b.MaxTime). Block intervals are always +1 than the total samples it includes.
		err := c.write(dest, outBlocks[ix], oooHead.CloneForTimeRange(mint, maxt-1))
		if err != nil {
			// We need to delete all blocks in case there was an error.
			for _, obs := range outBlocks {
				for _, ob := range obs {
					if ob.tmpDir != "" {
						if removeErr := os.RemoveAll(ob.tmpDir); removeErr != nil {
							level.Error(c.logger).Log("msg", "Failed to remove temp folder after failed compaction", "dir", ob.tmpDir, "err", removeErr.Error())
						}
					}
					if ob.blockDir != "" {
						if removeErr := os.RemoveAll(ob.blockDir); removeErr != nil {
							level.Error(c.logger).Log("msg", "Failed to remove block folder after failed compaction", "dir", ob.blockDir, "err", removeErr.Error())
						}
					}
				}
			}
			return nil, err
		}
	}

	noOOOBlock := true
	for ix, obs := range outBlocks {
		for jx := range obs {
			meta := outBlocks[ix][jx].meta
			if meta.Stats.NumSamples != 0 {
				noOOOBlock = false
				level.Info(c.logger).Log(
					"msg", "compact ooo head",
					"mint", meta.MinTime,
					"maxt", meta.MaxTime,
					"ulid", meta.ULID,
					"duration", time.Since(start),
					"shard", fmt.Sprintf("%d_of_%d", jx+1, shardCount),
				)
			} else {
				// This block did not get any data. So clear out the ulid to signal this.
				ulids[ix][jx] = ulid.ULID{}
			}
		}
	}

	if noOOOBlock {
		level.Info(c.logger).Log(
			"msg", "compact ooo head resulted in no blocks",
			"duration", time.Since(start),
		)
		return nil, nil
	}

	return ulids, nil
}

func (c *LeveledCompactor) Write(dest string, b BlockReader, mint, maxt int64, parent *BlockMeta) (ulid.ULID, error) {
	start := time.Now()

	uid := ulid.MustNew(ulid.Now(), rand.Reader)

	meta := &BlockMeta{
		ULID:    uid,
		MinTime: mint,
		MaxTime: maxt,
	}
	meta.Compaction.Level = 1
	meta.Compaction.Sources = []ulid.ULID{uid}

	if parent != nil {
		meta.Compaction.Parents = []BlockDesc{
			{ULID: parent.ULID, MinTime: parent.MinTime, MaxTime: parent.MaxTime},
		}
	}

	err := c.write(dest, []shardedBlock{{meta: meta}}, b)
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

// write creates new output blocks that are the union of the provided blocks into dir.
func (c *LeveledCompactor) write(dest string, outBlocks []shardedBlock, blocks ...BlockReader) (err error) {
	var closers []io.Closer

	defer func(t time.Time) {
		err = tsdb_errors.NewMulti(err, tsdb_errors.CloseAll(closers)).Err()

		for _, ob := range outBlocks {
			if ob.tmpDir != "" {
				// RemoveAll returns no error when tmp doesn't exist so it is safe to always run it.
				if removeErr := os.RemoveAll(ob.tmpDir); removeErr != nil {
					level.Error(c.logger).Log("msg", "Failed to remove temp folder after failed compaction", "dir", ob.tmpDir, "err", removeErr.Error())
				}
			}

			// If there was any error, and we have multiple output blocks, some blocks may have been generated, or at
			// least have existing blockDir. In such case, we want to remove them.
			// BlockDir may also not be set yet, if preparation for some previous blocks have failed.
			if err != nil && ob.blockDir != "" {
				// RemoveAll returns no error when tmp doesn't exist so it is safe to always run it.
				if removeErr := os.RemoveAll(ob.blockDir); removeErr != nil {
					level.Error(c.logger).Log("msg", "Failed to remove block folder after failed compaction", "dir", ob.blockDir, "err", removeErr.Error())
				}
			}
		}
		c.metrics.ran.Inc()
		c.metrics.duration.Observe(time.Since(t).Seconds())
	}(time.Now())

	for ix := range outBlocks {
		dir := filepath.Join(dest, outBlocks[ix].meta.ULID.String())
		tmp := dir + tmpForCreationBlockDirSuffix

		outBlocks[ix].blockDir = dir
		outBlocks[ix].tmpDir = tmp

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
			return errors.Wrap(err, "open chunk writer")
		}
		chunkw = newPreventDoubleCloseChunkWriter(chunkw) // We now close chunkWriter in populateBlock, but keep it in the closers here as well.

		closers = append(closers, chunkw)

		// Record written chunk sizes on level 1 compactions.
		if outBlocks[ix].meta.Compaction.Level == 1 {
			chunkw = &instrumentedChunkWriter{
				ChunkWriter: chunkw,
				size:        c.metrics.chunkSize,
				samples:     c.metrics.chunkSamples,
				trange:      c.metrics.chunkRange,
			}
		}

		outBlocks[ix].chunkw = chunkw

		var indexw IndexWriter
		indexw, err = index.NewWriter(c.ctx, filepath.Join(tmp, indexFilename))
		if err != nil {
			return errors.Wrap(err, "open index writer")
		}
		indexw = newPreventDoubleCloseIndexWriter(indexw) // We now close indexWriter in populateBlock, but keep it in the closers here as well.
		closers = append(closers, indexw)

		outBlocks[ix].indexw = indexw
	}

	// We use MinTime and MaxTime from first output block, because ALL output blocks have the same min/max times set.
	if err := c.populateBlock(blocks, outBlocks[0].meta.MinTime, outBlocks[0].meta.MaxTime, outBlocks); err != nil {
		return errors.Wrap(err, "populate block")
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

	for _, ob := range outBlocks {
		// Populated block is empty, don't write meta file for it.
		if ob.meta.Stats.NumSamples == 0 {
			continue
		}

		if _, err = writeMetaFile(c.logger, ob.tmpDir, ob.meta); err != nil {
			return errors.Wrap(err, "write merged meta")
		}

		// Create an empty tombstones file.
		if _, err := tombstones.WriteFile(c.logger, ob.tmpDir, tombstones.NewMemTombstones()); err != nil {
			return errors.Wrap(err, "write new tombstones file")
		}

		df, err := fileutil.OpenDir(ob.tmpDir)
		if err != nil {
			return errors.Wrap(err, "open temporary block dir")
		}
		defer func() {
			if df != nil {
				df.Close()
			}
		}()

		if err := df.Sync(); err != nil {
			return errors.Wrap(err, "sync temporary dir file")
		}

		// Close temp dir before rename block dir (for windows platform).
		if err = df.Close(); err != nil {
			return errors.Wrap(err, "close temporary dir")
		}
		df = nil

		// Block successfully written, make it visible in destination dir by moving it from tmp one.
		if err := fileutil.Replace(ob.tmpDir, ob.blockDir); err != nil {
			return errors.Wrap(err, "rename block dir")
		}
	}

	return nil
}

func debugOutOfOrderChunks(chks []chunks.Meta, logger log.Logger) {
	if len(chks) <= 1 {
		return
	}

	prevChk := chks[0]
	for i := 1; i < len(chks); i++ {
		currChk := chks[i]

		if currChk.MinTime > prevChk.MaxTime {
			// Not out of order.
			continue
		}

		// Looks like the chunk is out of order.
		prevSafeChk, prevIsSafeChk := prevChk.Chunk.(*safeChunk)
		currSafeChk, currIsSafeChk := currChk.Chunk.(*safeChunk)

		// Get info out of safeChunk (if possible).
		prevHeadChunkID := chunks.HeadChunkID(0)
		currHeadChunkID := chunks.HeadChunkID(0)
		prevLabels := labels.Labels{}
		currLabels := labels.Labels{}
		if prevSafeChk != nil {
			prevHeadChunkID = prevSafeChk.cid
			prevLabels = prevSafeChk.s.lset
		}
		if currSafeChk != nil {
			currHeadChunkID = currSafeChk.cid
			currLabels = currSafeChk.s.lset
		}

		level.Warn(logger).Log(
			"msg", "found out-of-order chunk when compacting",
			"prev_ref", prevChk.Ref,
			"curr_ref", currChk.Ref,
			"prev_min_time", timeFromMillis(prevChk.MinTime).UTC().String(),
			"prev_max_time", timeFromMillis(prevChk.MaxTime).UTC().String(),
			"curr_min_time", timeFromMillis(currChk.MinTime).UTC().String(),
			"curr_max_time", timeFromMillis(currChk.MaxTime).UTC().String(),
			"prev_samples", prevChk.Chunk.NumSamples(),
			"curr_samples", currChk.Chunk.NumSamples(),
			"prev_is_safe_chunk", prevIsSafeChk,
			"curr_is_safe_chunk", currIsSafeChk,
			"prev_head_chunk_id", prevHeadChunkID,
			"curr_head_chunk_id", currHeadChunkID,
			"prev_labelset", prevLabels.String(),
			"curr_labelset", currLabels.String(),
			"num_chunks_for_series", len(chks),
		)
	}
}

func timeFromMillis(ms int64) time.Time {
	return time.Unix(0, ms*int64(time.Millisecond))
}

// populateBlock fills the index and chunk writers of output blocks with new data gathered as the union
// of the provided blocks.
// It expects sorted blocks input by mint.
// If there is more than 1 output block, each output block will only contain series that hash into its shard
// (based on total number of output blocks).
func (c *LeveledCompactor) populateBlock(blocks []BlockReader, minT, maxT int64, outBlocks []shardedBlock) (err error) {
	if len(blocks) == 0 {
		return errors.New("cannot populate block(s) from no readers")
	}

	var (
		sets        []storage.ChunkSeriesSet
		symbolsSets []storage.ChunkSeriesSet // series sets used for finding symbols. Only used when doing sharding.
		symbols     index.StringIter
		closers     []io.Closer
		overlapping bool
	)
	defer func() {
		errs := tsdb_errors.NewMulti(err)
		if cerr := tsdb_errors.CloseAll(closers); cerr != nil {
			errs.Add(errors.Wrap(cerr, "close"))
		}
		err = errs.Err()
		c.metrics.populatingBlocks.Set(0)
	}()
	c.metrics.populatingBlocks.Set(1)

	globalMaxt := blocks[0].Meta().MaxTime
	for i, b := range blocks {
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		default:
		}

		if !overlapping {
			if i > 0 && b.Meta().MinTime < globalMaxt {
				c.metrics.overlappingBlocks.Inc()
				overlapping = true
				level.Info(c.logger).Log("msg", "Found overlapping blocks during compaction")
			}
			if b.Meta().MaxTime > globalMaxt {
				globalMaxt = b.Meta().MaxTime
			}
		}

		indexr, err := b.Index()
		if err != nil {
			return errors.Wrapf(err, "open index reader for block %+v", b.Meta())
		}
		closers = append(closers, indexr)

		chunkr, err := b.Chunks()
		if err != nil {
			return errors.Wrapf(err, "open chunk reader for block %+v", b.Meta())
		}
		closers = append(closers, chunkr)

		tombsr, err := b.Tombstones()
		if err != nil {
			return errors.Wrapf(err, "open tombstone reader for block %+v", b.Meta())
		}
		closers = append(closers, tombsr)

		k, v := index.AllPostingsKey()
		all, err := indexr.Postings(k, v)
		if err != nil {
			return err
		}
		all = indexr.SortedPostings(all)
		// Blocks meta is half open: [min, max), so subtract 1 to ensure we don't hold samples with exact meta.MaxTime timestamp.
		sets = append(sets, newBlockChunkSeriesSet(indexr, chunkr, tombsr, all, minT, maxT-1, false))

		if len(outBlocks) > 1 {
			// To iterate series when populating symbols, we cannot reuse postings we just got, but need to get a new copy.
			// Postings can only be iterated once.
			k, v = index.AllPostingsKey()
			all, err = indexr.Postings(k, v)
			if err != nil {
				return err
			}
			all = indexr.SortedPostings(all)
			// Blocks meta is half open: [min, max), so subtract 1 to ensure we don't hold samples with exact meta.MaxTime timestamp.
			symbolsSets = append(symbolsSets, newBlockChunkSeriesSet(indexr, chunkr, tombsr, all, minT, maxT-1, false))
		} else {
			syms := indexr.Symbols()
			if i == 0 {
				symbols = syms
				continue
			}
			symbols = NewMergedStringIter(symbols, syms)
		}
	}

	if len(outBlocks) == 1 {
		for symbols.Next() {
			if err := outBlocks[0].indexw.AddSymbol(symbols.At()); err != nil {
				return errors.Wrap(err, "add symbol")
			}
		}
		if symbols.Err() != nil {
			return errors.Wrap(symbols.Err(), "next symbol")
		}
	} else {
		if err := c.populateSymbols(symbolsSets, outBlocks); err != nil {
			return err
		}
	}

	// Semaphore for number of blocks that can be closed at once.
	sema := semaphore.NewWeighted(int64(c.concurrencyOpts.MaxClosingBlocks))

	blockWriters := make([]*asyncBlockWriter, len(outBlocks))
	for ix := range outBlocks {
		blockWriters[ix] = newAsyncBlockWriter(c.chunkPool, outBlocks[ix].chunkw, outBlocks[ix].indexw, sema)
		defer blockWriters[ix].closeAsync() // Make sure to close writer to stop goroutine.
	}

	set := sets[0]
	if len(sets) > 1 {
		// Merge series using specified chunk series merger.
		// The default one is the compacting series merger.
		set = storage.NewMergeChunkSeriesSet(sets, c.mergeFunc)
	}

	// Iterate over all sorted chunk series.
	for set.Next() {
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		default:
		}
		s := set.At()

		chksIter := s.Iterator()
		var chks []chunks.Meta
		for chksIter.Next() {
			// We are not iterating in streaming way over chunk as it's more efficient to do bulk write for index and
			// chunk file purposes.
			chks = append(chks, chksIter.At())
		}
		if chksIter.Err() != nil {
			return errors.Wrap(chksIter.Err(), "chunk iter")
		}

		// Skip the series with all deleted chunks.
		if len(chks) == 0 {
			continue
		}

		debugOutOfOrderChunks(chks, c.logger)

		obIx := uint64(0)
		if len(outBlocks) > 1 {
			obIx = s.Labels().Hash() % uint64(len(outBlocks))
		}

		err := blockWriters[obIx].addSeries(s.Labels(), chks)
		if err != nil {
			return errors.Wrap(err, "adding series")
		}
	}
	if set.Err() != nil {
		return errors.Wrap(set.Err(), "iterate compaction set")
	}

	for ix := range blockWriters {
		blockWriters[ix].closeAsync()
	}

	for ix := range blockWriters {
		stats, err := blockWriters[ix].waitFinished()
		if err != nil {
			return errors.Wrap(err, "writing block")
		}

		outBlocks[ix].meta.Stats = stats
	}

	return nil
}

// How many symbols we buffer in memory per output block.
const inMemorySymbolsLimit = 1_000_000

// populateSymbols writes symbols to output blocks. We need to iterate through all series to find
// which series belongs to what block. We collect symbols per sharded block, and then add sorted symbols to
// block's index.
func (c *LeveledCompactor) populateSymbols(sets []storage.ChunkSeriesSet, outBlocks []shardedBlock) error {
	if len(outBlocks) == 0 {
		return errors.New("no output block")
	}

	flushers := newSymbolFlushers(c.concurrencyOpts.SymbolsFlushersCount)
	defer flushers.close() // Make sure to stop flushers before exiting to avoid leaking goroutines.

	batchers := make([]*symbolsBatcher, len(outBlocks))
	for ix := range outBlocks {
		batchers[ix] = newSymbolsBatcher(inMemorySymbolsLimit, outBlocks[ix].tmpDir, flushers)

		// Always include empty symbol. Blocks created from Head always have it in the symbols table,
		// and if we only include symbols from series, we would skip it.
		// It may not be required, but it's small and better be safe than sorry.
		if err := batchers[ix].addSymbol(""); err != nil {
			return errors.Wrap(err, "addSymbol to batcher")
		}
	}

	seriesSet := sets[0]
	if len(sets) > 1 {
		seriesSet = storage.NewMergeChunkSeriesSet(sets, c.mergeFunc)
	}

	for seriesSet.Next() {
		if err := c.ctx.Err(); err != nil {
			return err
		}

		s := seriesSet.At()

		obIx := s.Labels().Hash() % uint64(len(outBlocks))

		for _, l := range s.Labels() {
			if err := batchers[obIx].addSymbol(l.Name); err != nil {
				return errors.Wrap(err, "addSymbol to batcher")
			}
			if err := batchers[obIx].addSymbol(l.Value); err != nil {
				return errors.Wrap(err, "addSymbol to batcher")
			}
		}
	}

	for ix := range outBlocks {
		// Flush the batcher to write remaining symbols.
		if err := batchers[ix].flushSymbols(true); err != nil {
			return errors.Wrap(err, "flushing batcher")
		}
	}

	err := flushers.close()
	if err != nil {
		return errors.Wrap(err, "closing flushers")
	}

	for ix := range outBlocks {
		if err := c.ctx.Err(); err != nil {
			return err
		}

		symbolFiles := batchers[ix].getSymbolFiles()

		it, err := newSymbolsIterator(symbolFiles)
		if err != nil {
			return errors.Wrap(err, "opening symbols iterator")
		}

		// Each symbols iterator must be closed to close underlying files.
		closeIt := it
		defer func() {
			if closeIt != nil {
				_ = closeIt.Close()
			}
		}()

		var sym string
		for sym, err = it.NextSymbol(); err == nil; sym, err = it.NextSymbol() {
			err = outBlocks[ix].indexw.AddSymbol(sym)
			if err != nil {
				return errors.Wrap(err, "AddSymbol")
			}
		}

		if err != io.EOF {
			return errors.Wrap(err, "iterating symbols")
		}

		// if err == io.EOF, we have iterated through all symbols. We can close underlying
		// files now.
		closeIt = nil
		_ = it.Close()

		// Delete symbol files from symbolsBatcher. We don't need to perform the cleanup if populateSymbols
		// or compaction fails, because in that case compactor already removes entire (temp) output block directory.
		for _, fn := range symbolFiles {
			if err := os.Remove(fn); err != nil {
				return errors.Wrap(err, "deleting symbols file")
			}
		}
	}

	return nil
}

// Returns opened blocks, and blocks that should be closed (also returned in case of error).
func openBlocksForCompaction(dirs []string, open []*Block, logger log.Logger, pool chunkenc.Pool, concurrency int) (blocks, blocksToClose []*Block, _ error) {
	blocks = make([]*Block, 0, len(dirs))
	blocksToClose = make([]*Block, 0, len(dirs))

	toOpenCh := make(chan string, len(dirs))
	for _, d := range dirs {
		meta, _, err := readMetaFile(d)
		if err != nil {
			return nil, blocksToClose, err
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

		if b != nil {
			blocks = append(blocks, b)
		} else {
			toOpenCh <- d
		}
	}
	close(toOpenCh)

	type openResult struct {
		b   *Block
		err error
	}

	openResultCh := make(chan openResult, len(toOpenCh))
	// Signals to all opening goroutines that there was an error opening some block, and they can stop early.
	// If openingError is true, at least one error is sent to openResultCh.
	openingError := atomic.NewBool(false)

	wg := sync.WaitGroup{}
	if len(dirs) < concurrency {
		concurrency = len(dirs)
	}
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for d := range toOpenCh {
				if openingError.Load() {
					return
				}

				b, err := OpenBlock(logger, d, pool)
				openResultCh <- openResult{b: b, err: err}

				if err != nil {
					openingError.Store(true)
					return
				}
			}
		}()
	}
	wg.Wait()

	// All writers to openResultCh have stopped, we can close the output channel, so we can range over it.
	close(openResultCh)

	var firstErr error
	for or := range openResultCh {
		if or.err != nil {
			// Don't stop on error, but iterate over all opened blocks to collect blocksToClose.
			if firstErr == nil {
				firstErr = or.err
			}
		} else {
			blocks = append(blocks, or.b)
			blocksToClose = append(blocksToClose, or.b)
		}
	}

	return blocks, blocksToClose, firstErr
}
