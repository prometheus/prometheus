package tsdb

import (
	"os"
	"path/filepath"
	"time"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/fabxc/tsdb/labels"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

type compactor struct {
	metrics *compactorMetrics
	opts    *compactorOptions
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
	m.duration = prometheus.NewHistogram(prometheus.HistogramOpts{
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

type compactorOptions struct {
	maxBlockRange uint64
}

func newCompactor(r prometheus.Registerer, opts *compactorOptions) *compactor {
	return &compactor{
		opts:    opts,
		metrics: newCompactorMetrics(r),
	}
}

type compactionInfo struct {
	generation int
	mint, maxt int64
}

const compactionBlocksLen = 4

// pick returns a range [i, j) in the blocks that are suitable to be compacted
// into a single block at position i.
func (c *compactor) pick(bs []compactionInfo) (i, j int, ok bool) {
	if len(bs) == 0 {
		return 0, 0, false
	}

	// First, we always compact pending in-memory blocks – oldest first.
	for i, b := range bs {
		if b.generation > 0 {
			continue
		}
		// Directly compact into 2nd generation with previous generation 1 blocks.
		if i+1 >= compactionBlocksLen {
			match := true
			for _, pb := range bs[i-compactionBlocksLen+1 : i] {
				match = match && pb.generation == 1
			}
			if match {
				return i - compactionBlocksLen + 1, i + 1, true
			}
		}
		// If we have enough generation 0 blocks to directly move to the
		// 2nd generation, skip generation 1.
		if len(bs)-i >= compactionBlocksLen {
			// Guard against the newly compacted block becoming larger than
			// the previous one.
			if i == 0 || bs[i-1].generation >= 2 {
				return i, i + compactionBlocksLen, true
			}
		}

		// No optimizations possible, naiively compact the new block.
		return i, i + 1, true
	}

	// Then we care about compacting multiple blocks, starting with the oldest.
	for i := 0; i < len(bs)-compactionBlocksLen; i += compactionBlocksLen {
		if c.match(bs[i : i+2]) {
			return i, i + compactionBlocksLen, true
		}
	}

	return 0, 0, false
}

func (c *compactor) match(bs []compactionInfo) bool {
	g := bs[0].generation
	if g >= 5 {
		return false
	}

	for _, b := range bs {
		if b.generation == 0 {
			continue
		}
		if b.generation != g {
			return false
		}
	}

	return uint64(bs[len(bs)-1].maxt-bs[0].mint) <= c.opts.maxBlockRange
}

func mergeBlockMetas(blocks ...Block) (res BlockMeta) {
	m0 := blocks[0].Meta()

	res.Sequence = m0.Sequence
	res.MinTime = m0.MinTime
	res.MaxTime = blocks[len(blocks)-1].Meta().MaxTime

	g := m0.Compaction.Generation
	if g == 0 && len(blocks) > 1 {
		g++
	}
	res.Compaction.Generation = g + 1

	for _, b := range blocks {
		res.Stats.NumSamples += b.Meta().Stats.NumSamples
	}
	return res
}

func (c *compactor) compact(dir string, blocks ...Block) (err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			c.metrics.failed.Inc()
		}
		c.metrics.duration.Observe(time.Since(start).Seconds())
	}()

	if fileutil.Exist(dir) {
		if err = os.RemoveAll(dir); err != nil {
			return err
		}
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	chunkf, err := fileutil.LockFile(chunksFileName(dir), os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return errors.Wrap(err, "create chunk file")
	}
	indexf, err := fileutil.LockFile(indexFileName(dir), os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return errors.Wrap(err, "create index file")
	}

	indexw := newIndexWriter(indexf)
	chunkw := newChunkWriter(chunkf)

	if err = c.write(dir, blocks, indexw, chunkw); err != nil {
		return errors.Wrap(err, "write compaction")
	}

	if err = chunkw.Close(); err != nil {
		return errors.Wrap(err, "close chunk writer")
	}
	if err = indexw.Close(); err != nil {
		return errors.Wrap(err, "close index writer")
	}
	if err = fileutil.Fsync(chunkf.File); err != nil {
		return errors.Wrap(err, "fsync chunk file")
	}
	if err = fileutil.Fsync(indexf.File); err != nil {
		return errors.Wrap(err, "fsync index file")
	}
	if err = chunkf.Close(); err != nil {
		return errors.Wrap(err, "close chunk file")
	}
	if err = indexf.Close(); err != nil {
		return errors.Wrap(err, "close index file")
	}
	return nil
}

func (c *compactor) write(dir string, blocks []Block, indexw IndexWriter, chunkw ChunkWriter) error {
	var set compactionSet

	for i, b := range blocks {
		all, err := b.Index().Postings("", "")
		if err != nil {
			return err
		}
		// TODO(fabxc): find more transparent way of handling this.
		if hb, ok := b.(*headBlock); ok {
			all = hb.remapPostings(all)
		}
		s := newCompactionSeriesSet(b.Index(), b.Series(), all)

		if i == 0 {
			set = s
			continue
		}
		set, err = newCompactionMerger(set, s)
		if err != nil {
			return err
		}
	}

	// We fully rebuild the postings list index from merged series.
	var (
		postings = &memPostings{m: make(map[term][]uint32, 512)}
		values   = map[string]stringset{}
		i        = uint32(0)
		meta     = mergeBlockMetas(blocks...)
	)

	for set.Next() {
		lset, chunks := set.At()
		if err := chunkw.WriteChunks(chunks...); err != nil {
			return err
		}

		indexw.AddSeries(i, lset, chunks...)

		meta.Stats.NumChunks += uint64(len(chunks))
		meta.Stats.NumSeries++

		for _, l := range lset {
			valset, ok := values[l.Name]
			if !ok {
				valset = stringset{}
				values[l.Name] = valset
			}
			valset.set(l.Value)

			postings.add(i, term{name: l.Name, value: l.Value})
		}
		i++
	}
	if set.Err() != nil {
		return set.Err()
	}

	s := make([]string, 0, 256)
	for n, v := range values {
		s = s[:0]

		for x := range v {
			s = append(s, x)
		}
		if err := indexw.WriteLabelIndex([]string{n}, s); err != nil {
			return err
		}
	}

	for t := range postings.m {
		if err := indexw.WritePostings(t.name, t.value, postings.get(t)); err != nil {
			return err
		}
	}
	// Write a postings list containing all series.
	all := make([]uint32, i)
	for i := range all {
		all[i] = uint32(i)
	}
	if err := indexw.WritePostings("", "", newListPostings(all)); err != nil {
		return err
	}

	return writeMetaFile(dir, &meta)
}

type compactionSet interface {
	Next() bool
	At() (labels.Labels, []ChunkMeta)
	Err() error
}

type compactionSeriesSet struct {
	p      Postings
	index  IndexReader
	series SeriesReader

	l   labels.Labels
	c   []ChunkMeta
	err error
}

func newCompactionSeriesSet(i IndexReader, s SeriesReader, p Postings) *compactionSeriesSet {
	return &compactionSeriesSet{
		index:  i,
		series: s,
		p:      p,
	}
}

func (c *compactionSeriesSet) Next() bool {
	if !c.p.Next() {
		return false
	}

	c.l, c.c, c.err = c.index.Series(c.p.At())
	if c.err != nil {
		return false
	}
	for i := range c.c {
		chk := &c.c[i]

		chk.Chunk, c.err = c.series.Chunk(chk.Ref)
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

func (c *compactionSeriesSet) At() (labels.Labels, []ChunkMeta) {
	return c.l, c.c
}

type compactionMerger struct {
	a, b compactionSet

	aok, bok bool
	l        labels.Labels
	c        []ChunkMeta
}

type compactionSeries struct {
	labels labels.Labels
	chunks []ChunkMeta
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
	a, _ := c.a.At()
	b, _ := c.b.At()
	return labels.Compare(a, b)
}

func (c *compactionMerger) Next() bool {
	if !c.aok && !c.bok || c.Err() != nil {
		return false
	}

	d := c.compare()
	// Both sets contain the current series. Chain them into a single one.
	if d > 0 {
		c.l, c.c = c.b.At()
		c.bok = c.b.Next()
	} else if d < 0 {
		c.l, c.c = c.a.At()
		c.aok = c.a.Next()
	} else {
		l, ca := c.a.At()
		_, cb := c.b.At()

		c.l = l
		c.c = append(ca, cb...)

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

func (c *compactionMerger) At() (labels.Labels, []ChunkMeta) {
	return c.l, c.c
}

func renameDir(from, to string) error {
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
	if err = fileutil.Fsync(pdir); err != nil {
		return err
	}
	if err = pdir.Close(); err != nil {
		return err
	}
	return nil
}
