package tsdb

import (
	"fmt"
	"os"
	"time"

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

		if err := c.compact(dir, c.shard.persisted[0], c.shard.persisted[1]); err != nil {
			c.logger.Log("msg", "compaction failed", "err", err)
		}
	}
	close(c.donec)
}

func (c *compactor) close() error {
	close(c.triggerc)
	<-c.donec
	return nil
}

func (c *compactor) compact(dir string, a, b block) error {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return err
	}

	cf, err := os.Create(chunksFileName(dir))
	if err != nil {
		return err
	}
	xf, err := os.Create(indexFileName(dir))
	if err != nil {
		return err
	}

	index := newIndexWriter(xf)
	series := newSeriesWriter(cf, index)

	defer index.Close()
	defer series.Close()

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
		if err := series.WriteSeries(i, lset, chunks); err != nil {
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

	if err := index.WriteStats(stats); err != nil {
		return err
	}

	s := make([]string, 0, 256)
	for n, v := range values {
		s = s[:0]

		for x := range v {
			s = append(s, x)
		}
		if err := index.WriteLabelIndex([]string{n}, s); err != nil {
			return err
		}
	}

	for t := range postings.m {
		if err := index.WritePostings(t.name, t.value, postings.get(t)); err != nil {
			return err
		}
	}
	// Write a postings list containing all series.
	all := make([]uint32, i)
	for i := range all {
		all[i] = uint32(i)
	}
	if err := index.WritePostings("", "", newListPostings(all)); err != nil {
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
