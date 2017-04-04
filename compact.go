package tsdb

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/go-kit/kit/log"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/tsdb/labels"
)

// Compactor provides compaction against an underlying storage
// of time series data.
type Compactor interface {
	// Plan returns a set of non-overlapping directories that can
	// be compacted concurrently.
	// Results returned when compactions are in progress are undefined.
	Plan(dir string) ([][]string, error)

	// Write persists a Block into a directory.
	Write(dir string, b Block) error

	// Compact runs compaction against the provided directories. Must
	// only be called concurrently with results of Plan().
	Compact(dirs ...string) error
}

// compactor implements the Compactor interface.
type compactor struct {
	metrics *compactorMetrics
	logger  log.Logger
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

func newCompactor(r prometheus.Registerer, l log.Logger, opts *compactorOptions) *compactor {
	return &compactor{
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

func (c *compactor) Plan(dir string) ([][]string, error) {
	dirs, err := blockDirs(dir)
	if err != nil {
		return nil, err
	}

	var bs []*BlockMeta

	for _, dir := range dirs {
		meta, err := readMetaFile(dir)
		if err != nil {
			return nil, err
		}
		if meta.Compaction.Generation > 0 {
			bs = append(bs, meta)
		}
	}

	if len(bs) == 0 {
		return nil, nil
	}

	sliceDirs := func(i, j int) [][]string {
		var res []string
		for k := i; k < j; k++ {
			res = append(res, dirs[k])
		}
		return [][]string{res}
	}

	// Then we care about compacting multiple blocks, starting with the oldest.
	for i := 0; i < len(bs)-compactionBlocksLen+1; i++ {
		if c.match(bs[i : i+3]) {
			return sliceDirs(i, i+compactionBlocksLen), nil
		}
	}

	return nil, nil
}

func (c *compactor) match(bs []*BlockMeta) bool {
	g := bs[0].Compaction.Generation

	for _, b := range bs {
		if b.Compaction.Generation != g {
			return false
		}
	}
	return uint64(bs[len(bs)-1].MaxTime-bs[0].MinTime) <= c.opts.maxBlockRange
}

func mergeBlockMetas(blocks ...Block) (res BlockMeta) {
	m0 := blocks[0].Meta()

	entropy := rand.New(rand.NewSource(time.Now().UnixNano()))

	res.Sequence = m0.Sequence
	res.MinTime = m0.MinTime
	res.MaxTime = blocks[len(blocks)-1].Meta().MaxTime
	res.ULID = ulid.MustNew(ulid.Now(), entropy)

	res.Compaction.Generation = m0.Compaction.Generation + 1

	for _, b := range blocks {
		res.Stats.NumSamples += b.Meta().Stats.NumSamples
	}
	return res
}

func (c *compactor) Compact(dirs ...string) (err error) {
	var blocks []Block

	for _, d := range dirs {
		b, err := newPersistedBlock(d)
		if err != nil {
			return err
		}
		defer b.Close()

		blocks = append(blocks, b)
	}

	return c.write(dirs[0], blocks...)
}

func (c *compactor) Write(dir string, b Block) error {
	return c.write(dir, b)
}

// write creates a new block that is the union of the provided blocks into dir.
// It cleans up all files of the old blocks after completing successfully.
func (c *compactor) write(dir string, blocks ...Block) (err error) {
	c.logger.Log("msg", "compact blocks", "blocks", fmt.Sprintf("%v", blocks))

	defer func(t time.Time) {
		if err != nil {
			c.metrics.failed.Inc()
		}
		c.metrics.duration.Observe(time.Since(t).Seconds())
	}(time.Now())

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

	meta, err := c.populate(blocks, indexw, chunkw)
	if err != nil {
		return errors.Wrap(err, "write compaction")
	}
	if err = writeMetaFile(tmp, meta); err != nil {
		return errors.Wrap(err, "write merged meta")
	}

	if err = chunkw.Close(); err != nil {
		return errors.Wrap(err, "close chunk writer")
	}
	if err = indexw.Close(); err != nil {
		return errors.Wrap(err, "close index writer")
	}

	// Block successfully written, make visible and remove old ones.
	if err := renameFile(tmp, dir); err != nil {
		return errors.Wrap(err, "rename block dir")
	}
	for _, b := range blocks[1:] {
		if err := os.RemoveAll(b.Dir()); err != nil {
			return err
		}
	}
	// Properly sync parent dir to ensure changes are visible.
	df, err := fileutil.OpenDir(dir)
	if err != nil {
		return errors.Wrap(err, "sync block dir")
	}
	if err := fileutil.Fsync(df); err != nil {
		return errors.Wrap(err, "sync block dir")
	}

	return nil
}

// populate fills the index and chunk writers with new data gathered as the union
// of the provided blocks. It returns meta information for the new block.
func (c *compactor) populate(blocks []Block, indexw IndexWriter, chunkw ChunkWriter) (*BlockMeta, error) {
	var set compactionSet

	for i, b := range blocks {
		all, err := b.Index().Postings("", "")
		if err != nil {
			return nil, err
		}
		s := newCompactionSeriesSet(b.Index(), b.Chunks(), all)

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
		meta     = mergeBlockMetas(blocks...)
	)

	for set.Next() {
		lset, chunks := set.At()
		if err := chunkw.WriteChunks(chunks...); err != nil {
			return nil, err
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
	At() (labels.Labels, []*ChunkMeta)
	Err() error
}

type compactionSeriesSet struct {
	p      Postings
	index  IndexReader
	chunks ChunkReader

	l   labels.Labels
	c   []*ChunkMeta
	err error
}

func newCompactionSeriesSet(i IndexReader, c ChunkReader, p Postings) *compactionSeriesSet {
	return &compactionSeriesSet{
		index:  i,
		chunks: c,
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
	for _, chk := range c.c {
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

func (c *compactionSeriesSet) At() (labels.Labels, []*ChunkMeta) {
	return c.l, c.c
}

type compactionMerger struct {
	a, b compactionSet

	aok, bok bool
	l        labels.Labels
	c        []*ChunkMeta
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

func (c *compactionMerger) At() (labels.Labels, []*ChunkMeta) {
	return c.l, c.c
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
	if err = fileutil.Fsync(pdir); err != nil {
		return err
	}
	if err = pdir.Close(); err != nil {
		return err
	}
	return nil
}
