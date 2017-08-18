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
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/go-kit/kit/log"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/labels"
)

// ExponentialBlockRanges returns the time ranges based on the stepSize
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
	// Plan returns a set of non-overlapping directories that can
	// be compacted concurrently.
	// Results returned when compactions are in progress are undefined.
	Plan(dir string) ([]string, error)

	// Write persists a Block into a directory.
	Write(dest string, b Block) error

	// Compact runs compaction against the provided directories. Must
	// only be called concurrently with results of Plan().
	Compact(dest string, dirs ...string) error
}

// LeveledCompactor implements the Compactor interface.
type LeveledCompactor struct {
	dir     string
	metrics *compactorMetrics
	logger  log.Logger
	opts    *LeveledCompactorOptions
}

type compactorMetrics struct {
	ran      prometheus.Counter
	failed   prometheus.Counter
	duration prometheus.Histogram
}

func newCompactorMetrics(r prometheus.Registerer) *compactorMetrics {
	m := &compactorMetrics{}

	m.ran = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsdb_compactions_total",
		Help: "Total number of compactions that were executed for the partition.",
	})
	m.failed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsdb_compactions_failed_total",
		Help: "Total number of compactions that failed for the partition.",
	})
	m.duration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "tsdb_compaction_duration",
		Help: "Duration of compaction runs.",
	})

	if r != nil {
		r.MustRegister(
			m.ran,
			m.failed,
			m.duration,
		)
	}
	return m
}

// LeveledCompactorOptions are the options for a LeveledCompactor.
type LeveledCompactorOptions struct {
	blockRanges []int64
	chunkPool   chunks.Pool
}

// NewLeveledCompactor returns a LeveledCompactor.
func NewLeveledCompactor(r prometheus.Registerer, l log.Logger, opts *LeveledCompactorOptions) *LeveledCompactor {
	if opts == nil {
		opts = &LeveledCompactorOptions{
			chunkPool: chunks.NewPool(),
		}
	}
	return &LeveledCompactor{
		opts:    opts,
		logger:  l,
		metrics: newCompactorMetrics(r),
	}
}

type compactionInfo struct {
	seq        int
	generation int
	mint, maxt int64
}

const compactionBlocksLen = 3

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

	var dms []dirMeta

	for _, dir := range dirs {
		meta, err := readMetaFile(dir)
		if err != nil {
			return nil, err
		}
		if meta.Compaction.Level > 0 {
			dms = append(dms, dirMeta{dir, meta})
		}
	}
	sort.Slice(dms, func(i, j int) bool {
		return dms[i].meta.MinTime < dms[j].meta.MinTime
	})

	return c.plan(dms)
}

func (c *LeveledCompactor) plan(dms []dirMeta) ([]string, error) {
	if len(dms) <= 1 {
		return nil, nil
	}

	var res []string
	for _, dm := range c.selectDirs(dms) {
		res = append(res, dm.dir)
	}
	if len(res) > 0 {
		return res, nil
	}

	// Compact any blocks that have >5% tombstones.
	for i := len(dms) - 1; i >= 0; i-- {
		meta := dms[i].meta
		if meta.MaxTime-meta.MinTime < c.opts.blockRanges[len(c.opts.blockRanges)/2] {
			break
		}

		if meta.Stats.NumSeries/(meta.Stats.NumTombstones+1) <= 20 { // 5%
			return []string{dms[i].dir}, nil
		}
	}

	return nil, nil
}

// selectDirs returns the dir metas that should be compacted into a single new block.
// If only a single block range is configured, the result is always nil.
func (c *LeveledCompactor) selectDirs(ds []dirMeta) []dirMeta {
	if len(c.opts.blockRanges) < 2 || len(ds) < 1 {
		return nil
	}

	highTime := ds[len(ds)-1].meta.MinTime

	for _, iv := range c.opts.blockRanges[1:] {
		parts := splitByRange(ds, iv)
		if len(parts) == 0 {
			continue
		}

		for _, p := range parts {
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
		if ds[i].meta.MinTime < t0 || ds[i].meta.MaxTime > t0+tr {
			i++
			continue
		}

		// Add all dirs to the current group that are within [t0, t0+tr].
		for ; i < len(ds); i++ {
			// Either the block falls into the next range or doesn't fit at all (checked above).
			if ds[i].meta.MinTime < t0 || ds[i].meta.MaxTime > t0+tr {
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

func compactBlockMetas(blocks ...BlockMeta) (res BlockMeta) {
	res.MinTime = blocks[0].MinTime
	res.MaxTime = blocks[len(blocks)-1].MaxTime

	sources := map[ulid.ULID]struct{}{}

	for _, b := range blocks {
		if b.Compaction.Level > res.Compaction.Level {
			res.Compaction.Level = b.Compaction.Level
		}
		for _, s := range b.Compaction.Sources {
			sources[s] = struct{}{}
		}
		// If it's an in memory block, its ULID goes into the sources.
		if b.Compaction.Level == 0 {
			sources[b.ULID] = struct{}{}
		}
	}
	res.Compaction.Level++

	for s := range sources {
		res.Compaction.Sources = append(res.Compaction.Sources, s)
	}
	sort.Slice(res.Compaction.Sources, func(i, j int) bool {
		return res.Compaction.Sources[i].Compare(res.Compaction.Sources[j]) < 0
	})

	return res
}

// Compact creates a new block in the compactor's directory from the blocks in the
// provided directories.
func (c *LeveledCompactor) Compact(dest string, dirs ...string) (err error) {
	var blocks []Block

	for _, d := range dirs {
		b, err := newPersistedBlock(d, c.opts.chunkPool)
		if err != nil {
			return err
		}
		defer b.Close()

		blocks = append(blocks, b)
	}

	entropy := rand.New(rand.NewSource(time.Now().UnixNano()))
	uid := ulid.MustNew(ulid.Now(), entropy)

	return c.write(dest, uid, blocks...)
}

func (c *LeveledCompactor) Write(dest string, b Block) error {
	// Buffering blocks might have been created that often have no data.
	if b.Meta().Stats.NumSeries == 0 {
		return nil
	}

	entropy := rand.New(rand.NewSource(time.Now().UnixNano()))
	uid := ulid.MustNew(ulid.Now(), entropy)

	return c.write(dest, uid, b)
}

// write creates a new block that is the union of the provided blocks into dir.
// It cleans up all files of the old blocks after completing successfully.
func (c *LeveledCompactor) write(dest string, uid ulid.ULID, blocks ...Block) (err error) {
	c.logger.Log("msg", "compact blocks", "blocks", fmt.Sprintf("%v", blocks))

	defer func(t time.Time) {
		if err != nil {
			c.metrics.failed.Inc()
		}
		c.metrics.ran.Inc()
		c.metrics.duration.Observe(time.Since(t).Seconds())
	}(time.Now())

	dir := filepath.Join(dest, uid.String())
	tmp := dir + ".tmp"

	if err = os.RemoveAll(tmp); err != nil {
		return err
	}

	if err = os.MkdirAll(tmp, 0777); err != nil {
		return err
	}

	// Populate chunk and index files into temporary directory with
	// data of all blocks.
	chunkw, err := newChunkWriter(chunkDir(tmp))
	if err != nil {
		return errors.Wrap(err, "open chunk writer")
	}
	indexw, err := newIndexWriter(tmp)
	if err != nil {
		return errors.Wrap(err, "open index writer")
	}

	meta, err := c.populateBlock(blocks, indexw, chunkw)
	if err != nil {
		return errors.Wrap(err, "write compaction")
	}
	meta.ULID = uid

	if err = writeMetaFile(tmp, meta); err != nil {
		return errors.Wrap(err, "write merged meta")
	}

	if err = chunkw.Close(); err != nil {
		return errors.Wrap(err, "close chunk writer")
	}
	if err = indexw.Close(); err != nil {
		return errors.Wrap(err, "close index writer")
	}

	// Create an empty tombstones file.
	if err := writeTombstoneFile(tmp, newEmptyTombstoneReader()); err != nil {
		return errors.Wrap(err, "write new tombstones file")
	}

	// Block successfully written, make visible and remove old ones.
	if err := renameFile(tmp, dir); err != nil {
		return errors.Wrap(err, "rename block dir")
	}
	// Properly sync parent dir to ensure changes are visible.
	df, err := fileutil.OpenDir(dir)
	if err != nil {
		return errors.Wrap(err, "sync block dir")
	}
	defer df.Close()

	if err := fileutil.Fsync(df); err != nil {
		return errors.Wrap(err, "sync block dir")
	}

	return nil
}

// populateBlock fills the index and chunk writers with new data gathered as the union
// of the provided blocks. It returns meta information for the new block.
func (c *LeveledCompactor) populateBlock(blocks []Block, indexw IndexWriter, chunkw ChunkWriter) (*BlockMeta, error) {
	var (
		set        compactionSet
		metas      []BlockMeta
		allSymbols = make(map[string]struct{}, 1<<16)
	)
	for i, b := range blocks {
		metas = append(metas, b.Meta())

		symbols, err := b.Index().Symbols()
		if err != nil {
			return nil, errors.Wrap(err, "read symbols")
		}
		for s := range symbols {
			allSymbols[s] = struct{}{}
		}

		indexr := b.Index()

		all, err := indexr.Postings("", "")
		if err != nil {
			return nil, err
		}
		all = indexr.SortedPostings(all)

		s := newCompactionSeriesSet(indexr, b.Chunks(), b.Tombstones(), all)

		if i == 0 {
			set = s
			continue
		}
		set, err = newCompactionMerger(set, s)
		if err != nil {
			return nil, err
		}
	}

	// We fully rebuild the postings list index from merged series.
	var (
		postings = &memPostings{m: make(map[term][]uint32, 512)}
		values   = map[string]stringset{}
		i        = uint32(0)
		meta     = compactBlockMetas(metas...)
	)

	if err := indexw.AddSymbols(allSymbols); err != nil {
		return nil, errors.Wrap(err, "add symbols")
	}

	for set.Next() {
		lset, chks, dranges := set.At() // The chunks here are not fully deleted.

		// Skip the series with all deleted chunks.
		if len(chks) == 0 {
			continue
		}

		if len(dranges) > 0 {
			// Re-encode the chunk to not have deleted values.
			for _, chk := range chks {
				if intervalOverlap(dranges[0].mint, dranges[len(dranges)-1].maxt, chk.MinTime, chk.MaxTime) {
					newChunk := chunks.NewXORChunk()
					app, err := newChunk.Appender()
					if err != nil {
						return nil, err
					}

					it := &deletedIterator{it: chk.Chunk.Iterator(), intervals: dranges}
					for it.Next() {
						ts, v := it.At()
						app.Append(ts, v)
					}

					chk.Chunk = newChunk
				}
			}
		}
		if err := chunkw.WriteChunks(chks...); err != nil {
			return nil, err
		}

		if err := indexw.AddSeries(i, lset, chks...); err != nil {
			return nil, errors.Wrapf(err, "add series")
		}

		meta.Stats.NumChunks += uint64(len(chks))
		meta.Stats.NumSeries++
		for _, chk := range chks {
			meta.Stats.NumSamples += uint64(chk.Chunk.NumSamples())
		}

		for _, chk := range chks {
			c.opts.chunkPool.Put(chk.Chunk)
		}

		for _, l := range lset {
			valset, ok := values[l.Name]
			if !ok {
				valset = stringset{}
				values[l.Name] = valset
			}
			valset.set(l.Value)

			t := term{name: l.Name, value: l.Value}

			postings.add(i, t)
		}
		i++
	}
	if set.Err() != nil {
		return nil, set.Err()
	}

	s := make([]string, 0, 256)
	for n, v := range values {
		s = s[:0]

		for x := range v {
			s = append(s, x)
		}
		if err := indexw.WriteLabelIndex([]string{n}, s); err != nil {
			return nil, err
		}
	}

	for t := range postings.m {
		if err := indexw.WritePostings(t.name, t.value, postings.get(t)); err != nil {
			return nil, err
		}
	}
	// Write a postings list containing all series.
	all := make([]uint32, i)
	for i := range all {
		all[i] = uint32(i)
	}
	if err := indexw.WritePostings("", "", newListPostings(all)); err != nil {
		return nil, err
	}

	return &meta, nil
}

type compactionSet interface {
	Next() bool
	At() (labels.Labels, []ChunkMeta, intervals)
	Err() error
}

type compactionSeriesSet struct {
	p          Postings
	index      IndexReader
	chunks     ChunkReader
	tombstones TombstoneReader
	series     SeriesSet

	l         labels.Labels
	c         []ChunkMeta
	intervals intervals
	err       error
}

func newCompactionSeriesSet(i IndexReader, c ChunkReader, t TombstoneReader, p Postings) *compactionSeriesSet {
	return &compactionSeriesSet{
		index:      i,
		chunks:     c,
		tombstones: t,
		p:          p,
	}
}

func (c *compactionSeriesSet) Next() bool {
	if !c.p.Next() {
		return false
	}
	c.intervals = c.tombstones.Get(c.p.At())

	if c.err = c.index.Series(c.p.At(), &c.l, &c.c); c.err != nil {
		return false
	}

	// Remove completely deleted chunks.
	if len(c.intervals) > 0 {
		chks := make([]ChunkMeta, 0, len(c.c))
		for _, chk := range c.c {
			if !(interval{chk.MinTime, chk.MaxTime}.isSubrange(c.intervals)) {
				chks = append(chks, chk)
			}
		}

		c.c = chks
	}

	for i := range c.c {
		chk := &c.c[i]

		chk.Chunk, c.err = c.chunks.Chunk(chk.Ref)
		if c.err != nil {
			return false
		}
	}

	return true
}

func (c *compactionSeriesSet) Err() error {
	if c.err != nil {
		return c.err
	}
	return c.p.Err()
}

func (c *compactionSeriesSet) At() (labels.Labels, []ChunkMeta, intervals) {
	return c.l, c.c, c.intervals
}

type compactionMerger struct {
	a, b compactionSet

	aok, bok  bool
	l         labels.Labels
	c         []ChunkMeta
	intervals intervals
}

type compactionSeries struct {
	labels labels.Labels
	chunks []*ChunkMeta
}

func newCompactionMerger(a, b compactionSet) (*compactionMerger, error) {
	c := &compactionMerger{
		a: a,
		b: b,
	}
	// Initialize first elements of both sets as Next() needs
	// one element look-ahead.
	c.aok = c.a.Next()
	c.bok = c.b.Next()

	return c, c.Err()
}

func (c *compactionMerger) compare() int {
	if !c.aok {
		return 1
	}
	if !c.bok {
		return -1
	}
	a, _, _ := c.a.At()
	b, _, _ := c.b.At()
	return labels.Compare(a, b)
}

func (c *compactionMerger) Next() bool {
	if !c.aok && !c.bok || c.Err() != nil {
		return false
	}
	// While advancing child iterators the memory used for labels and chunks
	// may be reused. When picking a series we have to store the result.
	var lset labels.Labels
	var chks []ChunkMeta

	d := c.compare()
	// Both sets contain the current series. Chain them into a single one.
	if d > 0 {
		lset, chks, c.intervals = c.b.At()
		c.l = append(c.l[:0], lset...)
		c.c = append(c.c[:0], chks...)

		c.bok = c.b.Next()
	} else if d < 0 {
		lset, chks, c.intervals = c.a.At()
		c.l = append(c.l[:0], lset...)
		c.c = append(c.c[:0], chks...)

		c.aok = c.a.Next()
	} else {
		l, ca, ra := c.a.At()
		_, cb, rb := c.b.At()
		for _, r := range rb {
			ra = ra.add(r)
		}

		c.l = append(c.l[:0], l...)
		c.c = append(append(c.c[:0], ca...), cb...)
		c.intervals = ra

		c.aok = c.a.Next()
		c.bok = c.b.Next()
	}

	return true
}

func (c *compactionMerger) Err() error {
	if c.a.Err() != nil {
		return c.a.Err()
	}
	return c.b.Err()
}

func (c *compactionMerger) At() (labels.Labels, []ChunkMeta, intervals) {
	return c.l, c.c, c.intervals
}

func renameFile(from, to string) error {
	if err := os.RemoveAll(to); err != nil {
		return err
	}
	if err := os.Rename(from, to); err != nil {
		return err
	}

	// Directory was renamed; sync parent dir to persist rename.
	pdir, err := fileutil.OpenDir(filepath.Dir(to))
	if err != nil {
		return err
	}
	defer pdir.Close()

	if err = fileutil.Fsync(pdir); err != nil {
		return err
	}
	if err = pdir.Close(); err != nil {
		return err
	}
	return nil
}
