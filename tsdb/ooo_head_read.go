package tsdb

import (
	"errors"
	"math"
	"sort"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

var _ IndexReader = &OOOHeadIndexReader{}

// OOOHeadIndexReader implements IndexReader so ooo samples in the head can be
// accessed.
// It also has a reference to headIndexReader so we can leverage on its
// IndexReader implementation for all the methods that remain the same. We
// decided to do this to avoid code duplication.
// The only methods that change are the ones about getting Series and Postings.
type OOOHeadIndexReader struct {
	*headIndexReader // A reference to the headIndexReader so we can reuse as many interface implementation as possible.
}

func NewOOOHeadIndexReader(head *Head, mint, maxt int64) *OOOHeadIndexReader {
	hr := &headIndexReader{
		head: head,
		mint: mint,
		maxt: maxt,
	}
	return &OOOHeadIndexReader{hr}
}

func (oh *OOOHeadIndexReader) Series(ref storage.SeriesRef, lbls *labels.Labels, chks *[]chunks.Meta) error {
	return oh.series(ref, lbls, chks, 0)
}

// The passed lastMmapRef tells upto what max m-map chunk that we can consider.
// If it is 0, it means all chunks need to be considered.
// If it is non-0, then the oooHeadChunk must not be considered.
func (oh *OOOHeadIndexReader) series(ref storage.SeriesRef, lbls *labels.Labels, chks *[]chunks.Meta, lastMmapRef chunks.ChunkDiskMapperRef) error {
	s := oh.head.series.getByID(chunks.HeadSeriesRef(ref))

	if s == nil {
		oh.head.metrics.seriesNotFound.Inc()
		return storage.ErrNotFound
	}
	*lbls = append((*lbls)[:0], s.lset...)

	if chks == nil {
		return nil
	}

	s.Lock()
	defer s.Unlock()
	*chks = (*chks)[:0]

	tmpChks := make([]chunks.Meta, 0, len(s.oooMmappedChunks))

	// We define these markers to track the last chunk reference while we
	// fill the chunk meta.
	// These markers are useful to give consistent responses to repeated queries
	// even if new chunks that might be overlapping or not are added afterwards.
	// Also, lastMinT and lastMaxT are initialized to the max int as a sentinel
	// value to know they are unset.
	var lastChunkRef chunks.ChunkRef
	lastMinT, lastMaxT := int64(math.MaxInt64), int64(math.MaxInt64)

	addChunk := func(minT, maxT int64, ref chunks.ChunkRef) {
		// the first time we get called is for the last included chunk.
		// set the markers accordingly
		if lastMinT == int64(math.MaxInt64) {
			lastChunkRef = ref
			lastMinT = minT
			lastMaxT = maxT
		}

		tmpChks = append(tmpChks, chunks.Meta{
			MinTime:        minT,
			MaxTime:        maxT,
			Ref:            ref,
			OOOLastRef:     lastChunkRef,
			OOOLastMinTime: lastMinT,
			OOOLastMaxTime: lastMaxT,
		})
	}

	// Collect all chunks that overlap the query range, in order from most recent to most old,
	// so we can set the correct markers.
	if s.oooHeadChunk != nil {
		c := s.oooHeadChunk
		if c.OverlapsClosedInterval(oh.mint, oh.maxt) && lastMmapRef == 0 {
			ref := chunks.ChunkRef(chunks.NewHeadChunkRef(s.ref, s.oooHeadChunkID(len(s.oooMmappedChunks))))
			addChunk(c.minTime, c.maxTime, ref)
		}
	}
	for i := len(s.oooMmappedChunks) - 1; i >= 0; i-- {
		c := s.oooMmappedChunks[i]
		if c.OverlapsClosedInterval(oh.mint, oh.maxt) && (lastMmapRef == 0 || lastMmapRef.GreaterThanOrEqualTo(c.ref)) {
			ref := chunks.ChunkRef(chunks.NewHeadChunkRef(s.ref, s.oooHeadChunkID(i)))
			addChunk(c.minTime, c.maxTime, ref)
		}
	}

	// There is nothing to do if we did not collect any chunk
	if len(tmpChks) == 0 {
		return nil
	}

	// Next we want to sort all the collected chunks by min time so we can find
	// those that overlap.
	sort.Sort(metaByMinTimeAndMinRef(tmpChks))

	// Next we want to iterate the sorted collected chunks and only return the
	// chunks Meta the first chunk that overlaps with others.
	// Example chunks of a series: 5:(100, 200) 6:(500, 600) 7:(150, 250) 8:(550, 650)
	// In the example 5 overlaps with 7 and 6 overlaps with 8 so we only want to
	// to return chunk Metas for chunk 5 and chunk 6
	*chks = append(*chks, tmpChks[0])
	maxTime := tmpChks[0].MaxTime // tracks the maxTime of the previous "to be merged chunk"
	for _, c := range tmpChks[1:] {
		if c.MinTime > maxTime {
			*chks = append(*chks, c)
			maxTime = c.MaxTime
		} else if c.MaxTime > maxTime {
			maxTime = c.MaxTime
			(*chks)[len(*chks)-1].MaxTime = c.MaxTime
		}
	}

	return nil
}

type chunkMetaAndChunkDiskMapperRef struct {
	meta     chunks.Meta
	ref      chunks.ChunkDiskMapperRef
	origMinT int64
	origMaxT int64
}

type byMinTimeAndMinRef []chunkMetaAndChunkDiskMapperRef

func (b byMinTimeAndMinRef) Len() int { return len(b) }
func (b byMinTimeAndMinRef) Less(i, j int) bool {
	if b[i].meta.MinTime == b[j].meta.MinTime {
		return b[i].meta.Ref < b[j].meta.Ref
	}
	return b[i].meta.MinTime < b[j].meta.MinTime
}

func (b byMinTimeAndMinRef) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

type metaByMinTimeAndMinRef []chunks.Meta

func (b metaByMinTimeAndMinRef) Len() int { return len(b) }
func (b metaByMinTimeAndMinRef) Less(i, j int) bool {
	if b[i].MinTime == b[j].MinTime {
		return b[i].Ref < b[j].Ref
	}
	return b[i].MinTime < b[j].MinTime
}

func (b metaByMinTimeAndMinRef) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

func (oh *OOOHeadIndexReader) Postings(name string, values ...string) (index.Postings, error) {
	switch len(values) {
	case 0:
		return index.EmptyPostings(), nil
	case 1:
		return oh.head.postings.Get(name, values[0]), nil // TODO(ganesh) Also call GetOOOPostings
	default:
		// TODO(ganesh) We want to only return postings for out of order series.
		res := make([]index.Postings, 0, len(values))
		for _, value := range values {
			res = append(res, oh.head.postings.Get(name, value)) // TODO(ganesh) Also call GetOOOPostings
		}
		return index.Merge(res...), nil
	}
}

type OOOHeadChunkReader struct {
	head       *Head
	mint, maxt int64
}

func NewOOOHeadChunkReader(head *Head, mint, maxt int64) *OOOHeadChunkReader {
	return &OOOHeadChunkReader{
		head: head,
		mint: mint,
		maxt: maxt,
	}
}

func (cr OOOHeadChunkReader) Chunk(meta chunks.Meta) (chunkenc.Chunk, error) {
	sid, _ := chunks.HeadChunkRef(meta.Ref).Unpack()

	s := cr.head.series.getByID(sid)
	// This means that the series has been garbage collected.
	if s == nil {
		return nil, storage.ErrNotFound
	}

	s.Lock()
	c, err := s.oooMergedChunk(meta, cr.head.chunkDiskMapper, cr.mint, cr.maxt)
	s.Unlock()
	if err != nil {
		return nil, err
	}

	// This means that the query range did not overlap with the requested chunk.
	if len(c.chunks) == 0 {
		return nil, storage.ErrNotFound
	}

	return c, nil
}

func (cr OOOHeadChunkReader) Close() error {
	return nil
}

type OOOCompactionHead struct {
	oooIR       *OOOHeadIndexReader
	lastMmapRef chunks.ChunkDiskMapperRef
	lastWBLFile int
	postings    []storage.SeriesRef
	chunkRange  int64
	mint, maxt  int64 // Among all the compactable chunks.
}

// NewOOOCompactionHead does the following:
// 1. M-maps all the in-memory ooo chunks.
// 2. Compute the expected block ranges while iterating through all ooo series and store it.
// 3. Store the list of postings having ooo series.
// 4. Cuts a new WBL file for the OOO WBL.
// All the above together have a bit of CPU and memory overhead, and can have a bit of impact
// on the sample append latency. So call NewOOOCompactionHead only right before compaction.
func NewOOOCompactionHead(head *Head) (*OOOCompactionHead, error) {
	newWBLFile, err := head.wbl.NextSegment()
	if err != nil {
		return nil, err
	}

	ch := &OOOCompactionHead{
		chunkRange:  head.chunkRange.Load(),
		mint:        math.MaxInt64,
		maxt:        math.MinInt64,
		lastWBLFile: newWBLFile,
	}

	ch.oooIR = NewOOOHeadIndexReader(head, math.MinInt64, math.MaxInt64)
	n, v := index.AllPostingsKey()

	// TODO: verify this gets only ooo samples.
	p, err := ch.oooIR.Postings(n, v)
	if err != nil {
		return nil, err
	}
	p = ch.oooIR.SortedPostings(p)

	var lastSeq, lastOff int
	for p.Next() {
		seriesRef := p.At()
		ms := head.series.getByID(chunks.HeadSeriesRef(seriesRef))
		if ms == nil {
			continue
		}

		// M-map the in-memory chunk and keep track of the last one.
		// Also build the block ranges -> series map.
		// TODO: consider having a lock specifically for ooo data.
		ms.Lock()

		mmapRef := ms.mmapCurrentOOOHeadChunk(head.chunkDiskMapper)
		if mmapRef == 0 && len(ms.oooMmappedChunks) > 0 {
			// Nothing was m-mapped. So take the mmapRef from the existing slice if it exists.
			mmapRef = ms.oooMmappedChunks[len(ms.oooMmappedChunks)-1].ref
		}
		seq, off := mmapRef.Unpack()
		if seq > lastSeq || (seq == lastSeq && off > lastOff) {
			ch.lastMmapRef, lastSeq, lastOff = mmapRef, seq, off
		}
		if len(ms.oooMmappedChunks) > 0 {
			ch.postings = append(ch.postings, seriesRef)
			for _, c := range ms.oooMmappedChunks {
				if c.minTime < ch.mint {
					ch.mint = c.minTime
				}
				if c.maxTime > ch.maxt {
					ch.maxt = c.maxTime
				}
			}
		}
		ms.Unlock()
	}

	return ch, nil
}

func (ch *OOOCompactionHead) Index() (IndexReader, error) {
	return NewOOOCompactionHeadIndexReader(ch), nil
}

func (ch *OOOCompactionHead) Chunks() (ChunkReader, error) {
	return NewOOOHeadChunkReader(ch.oooIR.head, ch.oooIR.mint, ch.oooIR.maxt), nil
}

func (ch *OOOCompactionHead) Tombstones() (tombstones.Reader, error) {
	return tombstones.NewMemTombstones(), nil
}

func (ch *OOOCompactionHead) Meta() BlockMeta {
	var id [16]byte
	copy(id[:], "copy(id[:], \"ooo_compact_head\")")
	return BlockMeta{
		MinTime: ch.mint,
		MaxTime: ch.maxt,
		ULID:    id,
		Stats: BlockStats{
			NumSeries: uint64(len(ch.postings)),
		},
	}
}

// CloneForTimeRange clones the OOOCompactionHead such that the IndexReader and ChunkReader
// obtained from this only looks at the m-map chunks within the given time ranges while not looking
// beyond the ch.lastMmapRef.
// Only the method of BlockReader interface are valid for the cloned OOOCompactionHead.
func (ch *OOOCompactionHead) CloneForTimeRange(mint, maxt int64) *OOOCompactionHead {
	return &OOOCompactionHead{
		oooIR:       NewOOOHeadIndexReader(ch.oooIR.head, mint, maxt),
		lastMmapRef: ch.lastMmapRef,
		postings:    ch.postings,
		chunkRange:  ch.chunkRange,
		mint:        ch.mint,
		maxt:        ch.maxt,
	}
}

func (ch *OOOCompactionHead) Size() int64                            { return 0 }
func (ch *OOOCompactionHead) MinTime() int64                         { return ch.mint }
func (ch *OOOCompactionHead) MaxTime() int64                         { return ch.maxt }
func (ch *OOOCompactionHead) ChunkRange() int64                      { return ch.chunkRange }
func (ch *OOOCompactionHead) LastMmapRef() chunks.ChunkDiskMapperRef { return ch.lastMmapRef }
func (ch *OOOCompactionHead) LastWBLFile() int                       { return ch.lastWBLFile }

type OOOCompactionHeadIndexReader struct {
	ch *OOOCompactionHead
}

func NewOOOCompactionHeadIndexReader(ch *OOOCompactionHead) IndexReader {
	return &OOOCompactionHeadIndexReader{ch: ch}
}

func (ir *OOOCompactionHeadIndexReader) Symbols() index.StringIter {
	return ir.ch.oooIR.Symbols()
}

func (ir *OOOCompactionHeadIndexReader) Postings(name string, values ...string) (index.Postings, error) {
	n, v := index.AllPostingsKey()
	if name != n || len(values) != 1 || values[0] != v {
		return nil, errors.New("only AllPostingsKey is supported")
	}
	return index.NewListPostings(ir.ch.postings), nil
}

func (ir *OOOCompactionHeadIndexReader) SortedPostings(p index.Postings) index.Postings {
	// This will already be sorted from the Postings() call above.
	return p
}

func (ir *OOOCompactionHeadIndexReader) ShardedPostings(p index.Postings, shardIndex, shardCount uint64) index.Postings {
	return ir.ch.oooIR.ShardedPostings(p, shardIndex, shardCount)
}

func (ir *OOOCompactionHeadIndexReader) Series(ref storage.SeriesRef, lset *labels.Labels, chks *[]chunks.Meta) error {
	return ir.ch.oooIR.series(ref, lset, chks, ir.ch.lastMmapRef)
}

func (ir *OOOCompactionHeadIndexReader) SortedLabelValues(name string, matchers ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (ir *OOOCompactionHeadIndexReader) LabelValues(name string, matchers ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (ir *OOOCompactionHeadIndexReader) PostingsForMatchers(concurrent bool, ms ...*labels.Matcher) (index.Postings, error) {
	return nil, errors.New("not implemented")
}

func (ir *OOOCompactionHeadIndexReader) LabelNames(matchers ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (ir *OOOCompactionHeadIndexReader) LabelValueFor(id storage.SeriesRef, label string) (string, error) {
	return "", errors.New("not implemented")
}

func (ir *OOOCompactionHeadIndexReader) LabelNamesFor(ids ...storage.SeriesRef) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (ir *OOOCompactionHeadIndexReader) Close() error {
	return ir.ch.oooIR.Close()
}
