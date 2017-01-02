package tsdb

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/fabxc/tsdb/labels"
	"github.com/go-kit/kit/log"
	"github.com/prometheus/prometheus/pkg/timestamp"
)

type compactor struct {
	shard  *Shard
	logger log.Logger

	triggerc chan struct{}
	donec    chan struct{}
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
		if len(c.shard.persisted) < 2 {
			continue
		}
		dir := fmt.Sprintf("compacted-%d", timestamp.FromTime(time.Now()))

		p, err := newPersister(dir)
		if err != nil {
			c.logger.Log("msg", "creating persister failed", "err", err)
			continue
		}

		if err := c.compact(p, c.shard.persisted[0], c.shard.persisted[1]); err != nil {
			c.logger.Log("msg", "compaction failed", "err", err)
			continue
		}
		if err := p.Close(); err != nil {
			c.logger.Log("msg", "compaction failed", "err", err)
		}
	}
	close(c.donec)
}

func (c *compactor) Close() error {
	close(c.triggerc)
	<-c.donec
	return nil
}

func (c *compactor) compact(p *persister, a, b block) error {
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
		if err := p.chunkw.WriteSeries(i, lset, chunks); err != nil {
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

	if err := p.indexw.WriteStats(stats); err != nil {
		return err
	}

	s := make([]string, 0, 256)
	for n, v := range values {
		s = s[:0]

		for x := range v {
			s = append(s, x)
		}
		if err := p.indexw.WriteLabelIndex([]string{n}, s); err != nil {
			return err
		}
	}

	for t := range postings.m {
		if err := p.indexw.WritePostings(t.name, t.value, postings.get(t)); err != nil {
			return err
		}
	}
	// Write a postings list containing all series.
	all := make([]uint32, i)
	for i := range all {
		all[i] = uint32(i)
	}
	if err := p.indexw.WritePostings("", "", newListPostings(all)); err != nil {
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

type persister struct {
	dir, tmpdir string

	chunkf, indexf *fileutil.LockedFile

	chunkw SeriesWriter
	indexw IndexWriter
}

func newPersister(dir string) (*persister, error) {
	p := &persister{
		dir:    dir,
		tmpdir: dir + ".tmp",
	}
	var err error

	// Write to temporary directory to make persistence appear atomic.
	if fileutil.Exist(p.tmpdir) {
		if err := os.RemoveAll(p.tmpdir); err != nil {
			return nil, err
		}
	}
	if err := fileutil.CreateDirAll(p.tmpdir); err != nil {
		return nil, err
	}

	p.chunkf, err = fileutil.LockFile(chunksFileName(p.tmpdir), os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}
	p.indexf, err = fileutil.LockFile(indexFileName(p.tmpdir), os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}

	p.indexw = newIndexWriter(p.indexf)
	p.chunkw = newSeriesWriter(p.chunkf, p.indexw)

	return p, nil
}

func (p *persister) Close() error {
	if err := p.chunkw.Close(); err != nil {
		return err
	}
	if err := p.indexw.Close(); err != nil {
		return err
	}
	if err := fileutil.Fsync(p.chunkf.File); err != nil {
		return err
	}
	if err := fileutil.Fsync(p.indexf.File); err != nil {
		return err
	}
	if err := p.chunkf.Close(); err != nil {
		return err
	}
	if err := p.indexf.Close(); err != nil {
		return err
	}

	return renameDir(p.tmpdir, p.dir)
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
