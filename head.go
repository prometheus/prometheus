package tsdb

import (
	"errors"
	"os"
	"sort"
	"sync"

	"github.com/fabxc/tsdb/chunks"
	"github.com/fabxc/tsdb/labels"
)

// HeadBlock handles reads and writes of time series data within a time window.
type HeadBlock struct {
	mtx sync.RWMutex

	// descs holds all chunk descs for the head block. Each chunk implicitly
	// is assigned the index as its ID.
	descs []*chunkDesc
	// hashes contains a collision map of label set hashes of chunks
	// to their chunk descs.
	hashes map[uint64][]*chunkDesc

	values   map[string]stringset // label names to possible values
	postings *memPostings         // postings lists for terms

	wal *WAL

	stats BlockStats
}

// OpenHeadBlock creates a new empty head block.
func OpenHeadBlock(dir string, baseTime int64) (*HeadBlock, error) {
	wal, err := OpenWAL(dir)
	if err != nil {
		return nil, err
	}

	b := &HeadBlock{
		descs:    []*chunkDesc{},
		hashes:   map[uint64][]*chunkDesc{},
		values:   map[string]stringset{},
		postings: &memPostings{m: make(map[term][]uint32)},
		wal:      wal,
	}
	b.stats.MinTime = baseTime

	err = wal.ReadAll(&walHandler{
		series: func(lset labels.Labels) {
			b.create(lset.Hash(), lset)
		},
		sample: func(s hashedSample) {
			b.descs[s.ref].append(s.t, s.v)
			b.stats.SampleCount++
		},
	})
	if err != nil {
		return nil, err
	}

	return b, nil
}

// Close syncs all data and closes underlying resources of the head block.
func (h *HeadBlock) Close() error {
	return h.wal.Close()
}

func (h *HeadBlock) index() IndexReader {
	return h
}

func (h *HeadBlock) series() SeriesReader {
	return h
}

// Chunk returns the chunk for the reference number.
func (h *HeadBlock) Chunk(ref uint32) (chunks.Chunk, error) {
	if int(ref) >= len(h.descs) {
		return nil, errNotFound
	}
	return h.descs[int(ref)].chunk, nil
}

func (h *HeadBlock) interval() (int64, int64) {
	return h.stats.MinTime, h.stats.MaxTime
}

// Stats returns statisitics about the indexed data.
func (h *HeadBlock) Stats() (BlockStats, error) {
	return h.stats, nil
}

// LabelValues returns the possible label values
func (h *HeadBlock) LabelValues(names ...string) (StringTuples, error) {
	if len(names) != 1 {
		return nil, errInvalidSize
	}
	var sl []string

	for s := range h.values[names[0]] {
		sl = append(sl, s)
	}
	sort.Strings(sl)

	t := &stringTuples{
		l: len(names),
		s: sl,
	}
	return t, nil
}

// Postings returns the postings list iterator for the label pair.
func (h *HeadBlock) Postings(name, value string) (Postings, error) {
	return h.postings.get(term{name: name, value: value}), nil
}

// Series returns the series for the given reference.
func (h *HeadBlock) Series(ref uint32) (labels.Labels, []ChunkMeta, error) {
	if int(ref) >= len(h.descs) {
		return nil, nil, errNotFound
	}
	cd := h.descs[ref]

	return cd.lset, []ChunkMeta{{MinTime: h.stats.MinTime, Ref: ref}}, nil
}

func (h *HeadBlock) LabelIndices() ([][]string, error) {
	res := [][]string{}

	for s := range h.values {
		res = append(res, []string{s})
	}
	return res, nil
}

// get retrieves the chunk with the hash and label set and creates
// a new one if it doesn't exist yet.
func (h *HeadBlock) get(hash uint64, lset labels.Labels) *chunkDesc {
	cds := h.hashes[hash]

	for _, cd := range cds {
		if cd.lset.Equals(lset) {
			return cd
		}
	}
	return nil
}

func (h *HeadBlock) create(hash uint64, lset labels.Labels) *chunkDesc {
	var err error

	cd := &chunkDesc{
		lset:  lset,
		chunk: chunks.NewXORChunk(),
	}
	cd.app, err = cd.chunk.Appender()
	if err != nil {
		// Getting an Appender for a new chunk must not panic.
		panic(err)
	}
	// Index the new chunk.
	cd.ref = uint32(len(h.descs))

	h.descs = append(h.descs, cd)
	h.hashes[hash] = append(h.hashes[hash], cd)

	// Add each label pair as a term to the inverted index.
	terms := make([]term, 0, len(lset))

	for _, l := range lset {
		terms = append(terms, term{name: l.Name, value: l.Value})

		valset, ok := h.values[l.Name]
		if !ok {
			valset = stringset{}
			h.values[l.Name] = valset
		}
		valset.set(l.Value)
	}
	h.postings.add(cd.ref, terms...)

	// For the head block there's exactly one chunk per series.
	h.stats.ChunkCount++
	h.stats.SeriesCount++

	return cd
}

var (
	ErrOutOfOrderSample = errors.New("out of order sample")
	ErrAmendSample      = errors.New("amending sample")
)

func (h *HeadBlock) appendBatch(samples []hashedSample) error {
	// Find head chunks for all samples and allocate new IDs/refs for
	// ones we haven't seen before.
	var (
		newSeries    []labels.Labels
		newHashes    []uint64
		uniqueHashes = map[uint64]uint32{}
	)

	for i := range samples {
		s := &samples[i]

		cd := h.get(s.hash, s.labels)
		if cd != nil {
			// Samples must only occur in order.
			if s.t < cd.lastTimestamp {
				return ErrOutOfOrderSample
			}
			if cd.lastTimestamp == s.t && cd.lastValue != s.v {
				return ErrAmendSample
			}
			// TODO(fabxc): sample refs are only scoped within a block for
			// now and we ignore any previously set value
			s.ref = cd.ref
			continue
		}

		// There may be several samples for a new series in a batch.
		// We don't want to reserve a new space for each.
		if ref, ok := uniqueHashes[s.hash]; ok {
			s.ref = ref
			continue
		}
		s.ref = uint32(len(h.descs) + len(newSeries))
		uniqueHashes[s.hash] = s.ref

		newSeries = append(newSeries, s.labels)
		newHashes = append(newHashes, s.hash)
	}

	// Write all new series and samples to the WAL and add it to the
	// in-mem database on success.
	if err := h.wal.Log(newSeries, samples); err != nil {
		return err
	}

	// After the samples were successfully written to the WAL, there may
	// be no further failures.
	for i, s := range newSeries {
		h.create(newHashes[i], s)
	}

	for _, s := range samples {
		cd := h.descs[s.ref]
		// Skip duplicate samples.
		if cd.lastTimestamp == s.t && cd.lastValue != s.v {
			continue
		}
		cd.append(s.t, s.v)

		if s.t > h.stats.MaxTime {
			h.stats.MaxTime = s.t
		}
		h.stats.SampleCount++
	}

	return nil
}

func (h *HeadBlock) persist(p string) (int64, error) {
	sf, err := os.Create(chunksFileName(p))
	if err != nil {
		return 0, err
	}
	xf, err := os.Create(indexFileName(p))
	if err != nil {
		return 0, err
	}

	iw := newIndexWriter(xf)
	sw := newSeriesWriter(sf, iw)

	defer sw.Close()
	defer iw.Close()

	for ref, cd := range h.descs {
		if err := sw.WriteSeries(uint32(ref), cd.lset, []ChunkMeta{
			{
				MinTime: cd.firsTimestamp,
				MaxTime: cd.lastTimestamp,
				Chunk:   cd.chunk,
			},
		}); err != nil {
			return 0, err
		}
	}

	if err := iw.WriteStats(h.stats); err != nil {
		return 0, err
	}
	for n, v := range h.values {
		s := make([]string, 0, len(v))
		for x := range v {
			s = append(s, x)
		}

		if err := iw.WriteLabelIndex([]string{n}, s); err != nil {
			return 0, err
		}
	}

	for t := range h.postings.m {
		if err := iw.WritePostings(t.name, t.value, h.postings.get(t)); err != nil {
			return 0, err
		}
	}
	// Write a postings list containing all series.
	all := make([]uint32, len(h.descs))
	for i := range all {
		all[i] = uint32(i)
	}
	if err := iw.WritePostings("", "", newListPostings(all)); err != nil {
		return 0, err
	}

	// Everything written successfully, we can remove the WAL.
	if err := h.wal.Close(); err != nil {
		return 0, err
	}
	if err := os.Remove(h.wal.f.Name()); err != nil {
		return 0, err
	}

	return iw.Size() + sw.Size(), err
}
