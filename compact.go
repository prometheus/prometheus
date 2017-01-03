package tsdb

import (
	"os"
	"path/filepath"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/fabxc/tsdb/labels"
	"github.com/go-kit/kit/log"
)

type compactor struct {
	shard  *Shard
	blocks compactableBlocks
	logger log.Logger

	triggerc chan struct{}
	donec    chan struct{}
}

type compactableBlocks interface {
	compactable() []block
	set([]block)
}

func newCompactor(s *Shard, l log.Logger) (*compactor, error) {
	c := &compactor{
		triggerc: make(chan struct{}, 1),
		donec:    make(chan struct{}),
		shard:    s,
		logger:   l,
	}
	go c.run()

	return c, nil
}

func (c *compactor) trigger() {
	select {
	case c.triggerc <- struct{}{}:
	default:
	}
}

func (c *compactor) run() {
	for range c.triggerc {
		// continue
		// bs := c.blocks.get()

		// if len(bs) < 2 {
		// 	continue
		// }

		// var (
		// 	dir = fmt.Sprintf("compacted-%d", timestamp.FromTime(time.Now()))
		// 	a   = bs[0]
		// 	b   = bs[1]
		// )

		// c.blocks.Lock()

		// if err := persist(dir, func(indexw IndexWriter, chunkw SeriesWriter) error {
		// 	return c.compact(indexw, chunkw, a, b)
		// }); err != nil {
		// 	c.logger.Log("msg", "compaction failed", "err", err)
		// 	continue
		// }

		// c.blocks.Unlock()
	}
	close(c.donec)
}

func (c *compactor) pick() []block {
	return nil
}

func (c *compactor) Close() error {
	close(c.triggerc)
	<-c.donec
	return nil
}

func (c *compactor) compact(indexw IndexWriter, chunkw SeriesWriter, a, b block) error {
	aall, err := a.index().Postings("", "")
	if err != nil {
		return err
	}
	ball, err := b.index().Postings("", "")
	if err != nil {
		return err
	}

	set, err := newCompactionMerger(
		newCompactionSeriesSet(a.index(), a.series(), aall),
		newCompactionSeriesSet(b.index(), b.series(), ball),
	)
	if err != nil {
		return err
	}

	astats, err := a.index().Stats()
	if err != nil {
		return err
	}
	bstats, err := a.index().Stats()
	if err != nil {
		return err
	}

	// We fully rebuild the postings list index from merged series.
	var (
		postings = &memPostings{m: make(map[term][]uint32, 512)}
		values   = map[string]stringset{}
		i        = uint32(0)
	)
	stats := BlockStats{
		MinTime:     astats.MinTime,
		MaxTime:     bstats.MaxTime,
		SampleCount: astats.SampleCount + bstats.SampleCount,
	}

	for set.Next() {
		lset, chunks := set.At()
		if err := chunkw.WriteSeries(i, lset, chunks); err != nil {
			return err
		}

		stats.ChunkCount += uint32(len(chunks))
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
	a, b *compactionSeriesSet

	adone, bdone bool
	l            labels.Labels
	c            []ChunkMeta
}

type compactionSeries struct {
	labels labels.Labels
	chunks []ChunkMeta
}

func newCompactionMerger(a, b *compactionSeriesSet) (*compactionMerger, error) {
	c := &compactionMerger{
		a: a,
		b: b,
	}
	// Initialize first elements of both sets as Next() needs
	// one element look-ahead.
	c.adone = !c.a.Next()
	c.bdone = !c.b.Next()

	return c, c.Err()
}

func (c *compactionMerger) compare() int {
	if c.adone {
		return 1
	}
	if c.bdone {
		return -1
	}
	a, _ := c.a.At()
	b, _ := c.b.At()
	return labels.Compare(a, b)
}

func (c *compactionMerger) Next() bool {
	if c.adone && c.bdone || c.Err() != nil {
		return false
	}

	d := c.compare()
	// Both sets contain the current series. Chain them into a single one.
	if d > 0 {
		c.l, c.c = c.b.At()
		c.bdone = !c.b.Next()
	} else if d < 0 {
		c.l, c.c = c.a.At()
		c.adone = !c.a.Next()
	} else {
		l, ca := c.a.At()
		_, cb := c.b.At()

		c.l = l
		c.c = append(ca, cb...)

		c.adone = !c.a.Next()
		c.bdone = !c.b.Next()
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

func persist(dir string, write func(IndexWriter, SeriesWriter) error) error {
	tmpdir := dir + ".tmp"

	// Write to temporary directory to make persistence appear atomic.
	if fileutil.Exist(tmpdir) {
		if err := os.RemoveAll(tmpdir); err != nil {
			return err
		}
	}
	if err := fileutil.CreateDirAll(tmpdir); err != nil {
		return err
	}

	chunkf, err := fileutil.LockFile(chunksFileName(tmpdir), os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	indexf, err := fileutil.LockFile(indexFileName(tmpdir), os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	indexw := newIndexWriter(indexf)
	chunkw := newSeriesWriter(chunkf, indexw)

	if err := write(indexw, chunkw); err != nil {
		return err
	}

	if err := chunkw.Close(); err != nil {
		return err
	}
	if err := indexw.Close(); err != nil {
		return err
	}
	if err := fileutil.Fsync(chunkf.File); err != nil {
		return err
	}
	if err := fileutil.Fsync(indexf.File); err != nil {
		return err
	}
	if err := chunkf.Close(); err != nil {
		return err
	}
	if err := indexf.Close(); err != nil {
		return err
	}

	return renameDir(tmpdir, dir)
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
