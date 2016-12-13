package tsdb

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/fabxc/tsdb/chunks"
)

// SeriesReader provides reading access of serialized time series data.
type SeriesReader interface {
	// Chunk returns the series data chunk with the given reference.
	Chunk(ref uint32) (chunks.Chunk, error)
}

// seriesReader implements a SeriesReader for a serialized byte stream
// of series data.
type seriesReader struct {
	// The underlying byte slice holding the encoded series data.
	b []byte
}

func newSeriesReader(b []byte) (*seriesReader, error) {
	// Verify magic number.
	if m := binary.BigEndian.Uint32(b[:4]); m != MagicSeries {
		return nil, fmt.Errorf("invalid magic number %x", m)
	}
	return &seriesReader{b: b}, nil
}

func (s *seriesReader) Chunk(offset uint32) (chunks.Chunk, error) {
	b := s.b[offset:]

	l, n := binary.Uvarint(b)
	if n < 0 {
		return nil, fmt.Errorf("reading chunk length failed")
	}
	b = b[n:]
	enc := chunks.Encoding(b[0])

	c, err := chunks.FromData(enc, b[1:1+l])
	if err != nil {
		return nil, err
	}
	return c, nil
}

// IndexReader provides reading access of serialized index data.
type IndexReader interface {
	// Stats returns statisitics about the indexed data.
	Stats() (*BlockStats, error)

	// LabelValues returns the possible label values
	LabelValues(names ...string) (StringTuples, error)

	// Postings returns the postings list iteartor for the label pair.
	Postings(name, value string) (Iterator, error)

	// Series returns the series for the given reference.
	Series(ref uint32) (Series, error)
}

// StringTuples provides access to a sorted list of string tuples.
type StringTuples interface {
	// Total number of tuples in the list.
	Len() int
	// At returns the tuple at position i.
	At(i int) ([]string, error)
}

type indexReader struct {
	series SeriesReader

	// The underlying byte slice holding the encoded series data.
	b []byte

	// Cached hashmaps of section offsets.
	labels   map[string]uint32
	postings map[string]uint32
}

var (
	errInvalidSize = fmt.Errorf("invalid size")
	errInvalidFlag = fmt.Errorf("invalid flag")
)

func newIndexReader(s SeriesReader, b []byte) (*indexReader, error) {
	if len(b) < 16 {
		return nil, errInvalidSize
	}
	r := &indexReader{
		series: s,
		b:      b,
	}

	// Verify magic number.
	if m := binary.BigEndian.Uint32(b[:4]); m != MagicIndex {
		return nil, fmt.Errorf("invalid magic number %x", m)
	}

	var err error
	// The last two 4 bytes hold the pointers to the hashmaps.
	loff := binary.BigEndian.Uint32(b[len(b)-8 : len(b)-4])
	poff := binary.BigEndian.Uint32(b[len(b)-4:])

	if r.labels, err = readHashmap(r.section(loff)); err != nil {
		return nil, err
	}
	if r.postings, err = readHashmap(r.section(poff)); err != nil {
		return nil, err
	}

	return r, nil
}

func readHashmap(flag byte, b []byte, err error) (map[string]uint32, error) {
	if err != nil {
		return nil, err
	}
	if flag != flagStd {
		return nil, errInvalidFlag
	}
	h := make(map[string]uint32, 512)

	for len(b) > 0 {
		l, n := binary.Uvarint(b)
		if n < 1 {
			return nil, errInvalidSize
		}
		s := string(b[n : n+int(l)])
		b = b[n+int(l):]

		o, n := binary.Uvarint(b)
		if n < 1 {
			return nil, errInvalidSize
		}
		b = b[n:]

		h[s] = uint32(o)
	}

	return h, nil
}

func (r *indexReader) section(o uint32) (byte, []byte, error) {
	b := r.b[o:]

	if len(b) < 5 {
		return 0, nil, errInvalidSize
	}

	flag := r.b[0]
	l := binary.BigEndian.Uint32(b[1:5])

	b = b[5:]

	if len(b) < int(l) {
		return 0, nil, errInvalidSize
	}
	return flag, b, nil
}

func (r *indexReader) lookupSymbol(o uint32) ([]byte, error) {
	l, n := binary.Uvarint(r.b[o:])
	if n < 0 {
		return nil, fmt.Errorf("reading symbol length failed")
	}

	end := int(o) + n + int(l)
	if end > len(r.b) {
		return nil, fmt.Errorf("invalid length")
	}

	return r.b[int(o)+n : end], nil
}

func (r *indexReader) Stats() (*BlockStats, error) {
	return nil, nil
}

func (r *indexReader) LabelValues(names ...string) (StringTuples, error) {
	key := strings.Join(names, string(sep))
	off, ok := r.labels[key]
	if !ok {
		return nil, fmt.Errorf("label index doesn't exist")
	}

	flag, b, err := r.section(off)
	if err != nil {
		return nil, fmt.Errorf("section: %s", err)
	}
	if flag != flagStd {
		return nil, errInvalidFlag
	}
	l, n := binary.Uvarint(b)
	if n < 1 {
		return nil, errInvalidSize
	}

	st := &serializedStringTuples{
		l:      int(l),
		b:      b[n:],
		lookup: r.lookupSymbol,
	}
	return st, nil
}

func (r *indexReader) Series(ref uint32) (Series, error) {
	k, n := binary.Uvarint(r.b[ref:])
	if n < 1 {
		return nil, errInvalidSize
	}

	b := r.b[int(ref)+n:]
	offsets := make([]uint32, 0, k)

	for i := 0; i < int(k); i++ {
		o, n := binary.Uvarint(b)
		if n < 1 {
			return nil, errInvalidSize
		}
		offsets = append(offsets, uint32(o))

		b = b[n:]
	}
	// Offests must occur in pairs representing name and value.
	if len(offsets)&1 != 0 {
		return nil, errInvalidSize
	}

	// TODO(fabxc): Fully materialize series for now. Figure out later if it
	// makes sense to decode those lazily.
	// The references are expected to be sorted and match the order of
	// the underlying strings.
	labels := make(Labels, 0, k)

	for i := 0; i < int(k); i += 2 {
		n, err := r.lookupSymbol(offsets[i])
		if err != nil {
			return nil, err
		}
		v, err := r.lookupSymbol(offsets[i+1])
		if err != nil {
			return nil, err
		}
		labels = append(labels, Label{
			Name:  string(n),
			Value: string(v),
		})
	}

	// Read the chunk offsets.
	k, n = binary.Uvarint(r.b[ref:])
	if n < 1 {
		return nil, errInvalidSize
	}

	b = b[n:]
	coffsets := make([]ChunkOffset, 0, k)

	for i := 0; i < int(k); i++ {
		v, n := binary.Varint(b)
		if n < 1 {
			return nil, errInvalidSize
		}
		b = b[n:]

		o, n := binary.Uvarint(b)
		if n < 1 {
			return nil, errInvalidSize
		}
		b = b[n:]

		coffsets = append(coffsets, ChunkOffset{
			Offset: uint32(o),
			Value:  v,
		})
	}

	s := &series{
		labels:  labels,
		offsets: coffsets,
		chunk:   r.series.Chunk,
	}
	return s, nil
}

type series struct {
	labels  Labels
	offsets []ChunkOffset // in-order chunk refs
	chunk   func(ref uint32) (chunks.Chunk, error)
}

func (s *series) Labels() Labels {
	return s.labels
}

func (s *series) Iterator() SeriesIterator {
	var cs []chunks.Chunk

	return newChunkSeriesIterator(cs)
}

type stringTuples struct {
	l int      // tuple length
	s []string // flattened tuple entries
}

func newStringTuples(s []string, l int) (*stringTuples, error) {
	if len(s)%l != 0 {
		return nil, errInvalidSize
	}
	return &stringTuples{s: s, l: l}, nil
}

func (t *stringTuples) Len() int                   { return len(t.s) / t.l }
func (t *stringTuples) At(i int) ([]string, error) { return t.s[i : i+t.l], nil }

func (t *stringTuples) Swap(i, j int) {
	c := make([]string, t.l)
	copy(c, t.s[i:i+t.l])

	for k := 0; k < t.l; k++ {
		t.s[i+k] = t.s[j+k]
		t.s[j+k] = c[k]
	}
}

func (t *stringTuples) Less(i, j int) bool {
	for k := 0; k < t.l; k++ {
		d := strings.Compare(t.s[i+k], t.s[j+k])

		if d < 0 {
			return true
		}
		if d > 0 {
			return false
		}
	}
	return false
}

type serializedStringTuples struct {
	l      int
	b      []byte
	lookup func(uint32) ([]byte, error)
}

func (t *serializedStringTuples) Len() int {
	// TODO(fabxc): Cache this?
	return len(t.b) / (4 * t.l)
}

func (t *serializedStringTuples) At(i int) ([]string, error) {
	if len(t.b) < (i+t.l)*4 {
		return nil, errInvalidSize
	}
	res := make([]string, t.l)

	for k := 0; k < t.l; k++ {
		offset := binary.BigEndian.Uint32(t.b[i*4:])

		b, err := t.lookup(offset)
		if err != nil {
			return nil, fmt.Errorf("lookup: %s", err)
		}
		res = append(res, string(b))
	}

	return res, nil
}
