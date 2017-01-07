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
	blocks  compactableBlocks
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

type blockStore interface {
	blocks() []block
}

type compactableBlocks interface {
	compactable() []block
}

func newCompactor(blocks compactableBlocks) (*compactor, error) {
	c := &compactor{
		blocks:  blocks,
		metrics: newCompactorMetrics(nil),
	}

	return c, nil
}

const (
	compactionMaxSize = 1 << 30 // 1GB
	compactionBlocks  = 2
)

func (c *compactor) pick() []block {
	bs := c.blocks.compactable()
	if len(bs) == 0 {
		return nil
	}
	if len(bs) == 1 && !bs[0].persisted() {
		return bs
	}
	if !bs[0].persisted() {
		if len(bs) == 2 || !compactionMatch(bs[:3]) {
			return bs[:1]
		}
	}

	for i := 0; i+2 < len(bs); i += 3 {
		tpl := bs[i : i+3]
		if compactionMatch(tpl) {
			return tpl
		}
	}
	return nil
}

func compactionMatch(blocks []block) bool {
	// TODO(fabxc): check whether combined size is below maxCompactionSize.
	// Apply maximum time range? or number of series? – might already be covered by size implicitly.

	// Naively check whether both blocks have roughly the same number of samples
	// and whether the total sample count doesn't exceed 2GB chunk file size
	// by rough approximation.
	n := float64(blocks[0].stats().SampleCount)
	t := n

	for _, b := range blocks[1:] {
		m := float64(b.stats().SampleCount)

		if m < 0.8*n || m > 1.2*n {
			return false
		}
		t += m
	}

	// Pessimistic 10 bytes/sample should do.
	return t < 10*200e6
}

func mergeStats(blocks ...block) (res BlockStats) {
	res.MinTime = blocks[0].stats().MinTime
	res.MaxTime = blocks[len(blocks)-1].stats().MaxTime

	for _, b := range blocks {
		res.SampleCount += b.stats().SampleCount
	}
	return res
}

func (c *compactor) compact(dir string, blocks ...block) (err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			c.metrics.failed.Inc()
		}
		c.metrics.duration.Observe(time.Since(start).Seconds())
	}()

	// Write to temporary directory to make persistence appear atomic.
	if fileutil.Exist(dir) {
		if err = os.RemoveAll(dir); err != nil {
			return err
		}
	}
	if err = fileutil.CreateDirAll(dir); err != nil {
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
	chunkw := newSeriesWriter(chunkf, indexw)

	if err = c.write(blocks, indexw, chunkw); err != nil {
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

func (c *compactor) write(blocks []block, indexw IndexWriter, chunkw SeriesWriter) error {
	var set compactionSet
	for i, b := range blocks {
		all, err := b.index().Postings("", "")
		if err != nil {
			return err
		}
		// TODO(fabxc): find more transparent way of handling this.
		if hb, ok := b.(*HeadBlock); ok {
			all = hb.remapPostings(all)
		}
		s := newCompactionSeriesSet(b.index(), b.series(), all)

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
		stats    = mergeStats(blocks...)
	)

	for set.Next() {
		lset, chunks := set.At()
		if err := chunkw.WriteSeries(i, lset, chunks); err != nil {
			return err
		}

		stats.ChunkCount += uint64(len(chunks))
		stats.SeriesCount++

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

	if err := indexw.WriteStats(stats); err != nil {
		return err
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
	return nil
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
