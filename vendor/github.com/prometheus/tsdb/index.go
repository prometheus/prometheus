package tsdb

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/labels"
)

const (
	// MagicIndex 4 bytes at the head of an index file.
	MagicIndex = 0xBAAAD700

	indexFormatV1 = 1
)

const compactionPageBytes = minSectorSize * 64

// IndexWriter serialized the index for a block of series data.
// The methods must generally be called in order they are specified.
type IndexWriter interface {
	// AddSeries populates the index writer witha series and its offsets
	// of chunks that the index can reference.
	// The reference number is used to resolve a series against the postings
	// list iterator. It only has to be available during the write processing.
	AddSeries(ref uint32, l labels.Labels, chunks ...*ChunkMeta) error

	// WriteLabelIndex serializes an index from label names to values.
	// The passed in values chained tuples of strings of the length of names.
	WriteLabelIndex(names []string, values []string) error

	// WritePostings writes a postings list for a single label pair.
	// The Postings here contain refs to the series that were added.
	WritePostings(name, value string, it Postings) error

	// Close writes any finalization and closes the resources associated with
	// the underlying writer.
	Close() error
}

type indexWriterSeries struct {
	labels labels.Labels
	chunks []*ChunkMeta // series file offset of chunks
	offset uint32       // index file offset of series reference
}

// indexWriter implements the IndexWriter interface for the standard
// serialization format.
type indexWriter struct {
	f       *os.File
	bufw    *bufio.Writer
	n       int64
	started bool

	// Reusable memory.
	b       []byte
	uint32s []uint32

	series       map[uint32]*indexWriterSeries
	symbols      map[string]uint32 // symbol offsets
	labelIndexes []hashEntry       // label index offsets
	postings     []hashEntry       // postings lists offsets

	crc32 hash.Hash
}

func newIndexWriter(dir string) (*indexWriter, error) {
	df, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "index"), os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}
	if err := fileutil.Fsync(df); err != nil {
		return nil, errors.Wrap(err, "sync dir")
	}

	iw := &indexWriter{
		f:    f,
		bufw: bufio.NewWriterSize(f, 1<<22),
		n:    0,

		// Reusable memory.
		b:       make([]byte, 0, 1<<23),
		uint32s: make([]uint32, 0, 1<<15),

		// Caches.
		symbols: make(map[string]uint32, 1<<13),
		series:  make(map[uint32]*indexWriterSeries, 1<<16),
		crc32:   crc32.New(crc32.MakeTable(crc32.Castagnoli)),
	}
	if err := iw.writeMeta(); err != nil {
		return nil, err
	}
	return iw, nil
}

func (w *indexWriter) write(wr io.Writer, b []byte) error {
	n, err := wr.Write(b)
	w.n += int64(n)
	return err
}

// section writes a CRC32 checksummed section of length l and guarded by flag.
func (w *indexWriter) section(l int, flag byte, f func(w io.Writer) error) error {
	w.crc32.Reset()
	wr := io.MultiWriter(w.crc32, w.bufw)

	b := [5]byte{flag, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[1:], uint32(l))

	if err := w.write(wr, b[:]); err != nil {
		return errors.Wrap(err, "writing header")
	}

	if err := f(wr); err != nil {
		return errors.Wrap(err, "write contents")
	}
	if err := w.write(w.bufw, w.crc32.Sum(nil)); err != nil {
		return errors.Wrap(err, "writing checksum")
	}
	return nil
}

func (w *indexWriter) writeMeta() error {
	b := [8]byte{}

	binary.BigEndian.PutUint32(b[:4], MagicIndex)
	b[4] = flagStd

	return w.write(w.bufw, b[:])
}

func (w *indexWriter) AddSeries(ref uint32, lset labels.Labels, chunks ...*ChunkMeta) error {
	if _, ok := w.series[ref]; ok {
		return errors.Errorf("series with reference %d already added", ref)
	}
	// Populate the symbol table from all label sets we have to reference.
	for _, l := range lset {
		w.symbols[l.Name] = 0
		w.symbols[l.Value] = 0
	}

	w.series[ref] = &indexWriterSeries{
		labels: lset,
		chunks: chunks,
	}
	return nil
}

func (w *indexWriter) writeSymbols() error {
	// Generate sorted list of strings we will store as reference table.
	symbols := make([]string, 0, len(w.symbols))
	for s := range w.symbols {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	// The start of the section plus a 5 byte section header are our base.
	// TODO(fabxc): switch to relative offsets and hold sections in a TOC.
	base := uint32(w.n) + 5

	buf := [binary.MaxVarintLen32]byte{}
	w.b = append(w.b[:0], flagStd)

	for _, s := range symbols {
		w.symbols[s] = base + uint32(len(w.b))

		n := binary.PutUvarint(buf[:], uint64(len(s)))
		w.b = append(w.b, buf[:n]...)
		w.b = append(w.b, s...)
	}

	return w.section(len(w.b), flagStd, func(wr io.Writer) error {
		return w.write(wr, w.b)
	})
}

type indexWriterSeriesSlice []*indexWriterSeries

func (s indexWriterSeriesSlice) Len() int      { return len(s) }
func (s indexWriterSeriesSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s indexWriterSeriesSlice) Less(i, j int) bool {
	return labels.Compare(s[i].labels, s[j].labels) < 0
}

func (w *indexWriter) writeSeries() error {
	// Series must be stored sorted along their labels.
	series := make(indexWriterSeriesSlice, 0, len(w.series))

	for _, s := range w.series {
		series = append(series, s)
	}
	sort.Sort(series)

	// Current end of file plus 5 bytes for section header.
	// TODO(fabxc): switch to relative offsets.
	base := uint32(w.n) + 5

	w.b = w.b[:0]
	buf := make([]byte, binary.MaxVarintLen64)

	for _, s := range series {
		// Write label set symbol references.
		s.offset = base + uint32(len(w.b))

		n := binary.PutUvarint(buf, uint64(len(s.labels)))
		w.b = append(w.b, buf[:n]...)

		for _, l := range s.labels {
			n = binary.PutUvarint(buf, uint64(w.symbols[l.Name]))
			w.b = append(w.b, buf[:n]...)
			n = binary.PutUvarint(buf, uint64(w.symbols[l.Value]))
			w.b = append(w.b, buf[:n]...)
		}

		// Write chunks meta data including reference into chunk file.
		n = binary.PutUvarint(buf, uint64(len(s.chunks)))
		w.b = append(w.b, buf[:n]...)

		for _, c := range s.chunks {
			n = binary.PutVarint(buf, c.MinTime)
			w.b = append(w.b, buf[:n]...)
			n = binary.PutVarint(buf, c.MaxTime)
			w.b = append(w.b, buf[:n]...)

			n = binary.PutUvarint(buf, uint64(c.Ref))
			w.b = append(w.b, buf[:n]...)
		}
	}

	return w.section(len(w.b), flagStd, func(wr io.Writer) error {
		return w.write(wr, w.b)
	})
}

func (w *indexWriter) init() error {
	if err := w.writeSymbols(); err != nil {
		return err
	}
	if err := w.writeSeries(); err != nil {
		return err
	}
	w.started = true

	return nil
}

func (w *indexWriter) WriteLabelIndex(names []string, values []string) error {
	if !w.started {
		if err := w.init(); err != nil {
			return err
		}
	}

	valt, err := newStringTuples(values, len(names))
	if err != nil {
		return err
	}
	sort.Sort(valt)

	w.labelIndexes = append(w.labelIndexes, hashEntry{
		name:   strings.Join(names, string(sep)),
		offset: uint32(w.n),
	})

	buf := make([]byte, binary.MaxVarintLen32)
	n := binary.PutUvarint(buf, uint64(len(names)))

	l := n + len(values)*4

	return w.section(l, flagStd, func(wr io.Writer) error {
		// First byte indicates tuple size for index.
		if err := w.write(wr, buf[:n]); err != nil {
			return err
		}

		for _, v := range valt.s {
			binary.BigEndian.PutUint32(buf, w.symbols[v])

			if err := w.write(wr, buf[:4]); err != nil {
				return err
			}
		}
		return nil
	})
}

func (w *indexWriter) WritePostings(name, value string, it Postings) error {
	if !w.started {
		if err := w.init(); err != nil {
			return err
		}
	}

	key := name + string(sep) + value

	w.postings = append(w.postings, hashEntry{
		name:   key,
		offset: uint32(w.n),
	})

	// Order of the references in the postings list does not imply order
	// of the series references within the persisted block they are mapped to.
	// We have to sort the new references again.
	refs := w.uint32s[:0]

	for it.Next() {
		s, ok := w.series[it.At()]
		if !ok {
			return errors.Errorf("series for reference %d not found", it.At())
		}
		refs = append(refs, s.offset)
	}
	if err := it.Err(); err != nil {
		return err
	}

	sort.Sort(uint32slice(refs))

	w.b = w.b[:0]
	buf := make([]byte, 4)

	for _, r := range refs {
		binary.BigEndian.PutUint32(buf, r)
		w.b = append(w.b, buf...)
	}

	w.uint32s = refs[:0]

	return w.section(len(w.b), flagStd, func(wr io.Writer) error {
		return w.write(wr, w.b)
	})
}

type uint32slice []uint32

func (s uint32slice) Len() int           { return len(s) }
func (s uint32slice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s uint32slice) Less(i, j int) bool { return s[i] < s[j] }

type hashEntry struct {
	name   string
	offset uint32
}

func (w *indexWriter) writeHashmap(h []hashEntry) error {
	w.b = w.b[:0]
	buf := [binary.MaxVarintLen32]byte{}

	for _, e := range h {
		n := binary.PutUvarint(buf[:], uint64(len(e.name)))
		w.b = append(w.b, buf[:n]...)
		w.b = append(w.b, e.name...)

		n = binary.PutUvarint(buf[:], uint64(e.offset))
		w.b = append(w.b, buf[:n]...)
	}

	return w.section(len(w.b), flagStd, func(wr io.Writer) error {
		return w.write(wr, w.b)
	})
}

func (w *indexWriter) finalize() error {
	// Write out hash maps to jump to correct label index and postings sections.
	lo := uint32(w.n)
	if err := w.writeHashmap(w.labelIndexes); err != nil {
		return err
	}

	po := uint32(w.n)
	if err := w.writeHashmap(w.postings); err != nil {
		return err
	}

	// Terminate index file with offsets to hashmaps. This is the entry Pointer
	// for any index query.
	// TODO(fabxc): also store offset to series section to allow plain
	// iteration over all existing series?
	b := [8]byte{}
	binary.BigEndian.PutUint32(b[:4], lo)
	binary.BigEndian.PutUint32(b[4:], po)

	return w.write(w.bufw, b[:])
}

func (w *indexWriter) Close() error {
	if err := w.finalize(); err != nil {
		return err
	}
	if err := w.bufw.Flush(); err != nil {
		return err
	}
	if err := fileutil.Fsync(w.f); err != nil {
		return err
	}
	return w.f.Close()
}

// IndexReader provides reading access of serialized index data.
type IndexReader interface {
	// LabelValues returns the possible label values
	LabelValues(names ...string) (StringTuples, error)

	// Postings returns the postings list iterator for the label pair.
	// The Postings here contain the offsets to the series inside the index.
	Postings(name, value string) (Postings, error)

	// Series returns the series for the given reference.
	Series(ref uint32) (labels.Labels, []*ChunkMeta, error)

	// LabelIndices returns the label pairs for which indices exist.
	LabelIndices() ([][]string, error)

	// Close released the underlying resources of the reader.
	Close() error
}

// StringTuples provides access to a sorted list of string tuples.
type StringTuples interface {
	// Total number of tuples in the list.
	Len() int
	// At returns the tuple at position i.
	At(i int) ([]string, error)
}

type indexReader struct {
	// The underlying byte slice holding the encoded series data.
	b []byte

	// Close that releases the underlying resources of the byte slice.
	c io.Closer

	// Cached hashmaps of section offsets.
	labels   map[string]uint32
	postings map[string]uint32
}

var (
	errInvalidSize = fmt.Errorf("invalid size")
	errInvalidFlag = fmt.Errorf("invalid flag")
)

// newIndexReader returns a new indexReader on the given directory.
func newIndexReader(dir string) (*indexReader, error) {
	f, err := openMmapFile(filepath.Join(dir, "index"))
	if err != nil {
		return nil, err
	}
	r := &indexReader{b: f.b, c: f}

	// Verify magic number.
	if len(f.b) < 4 {
		return nil, errors.Wrap(errInvalidSize, "index header")
	}
	if m := binary.BigEndian.Uint32(r.b[:4]); m != MagicIndex {
		return nil, errors.Errorf("invalid magic number %x", m)
	}

	// The last two 4 bytes hold the pointers to the hashmaps.
	loff := binary.BigEndian.Uint32(r.b[len(r.b)-8 : len(r.b)-4])
	poff := binary.BigEndian.Uint32(r.b[len(r.b)-4:])

	flag, b, err := r.section(loff)
	if err != nil {
		return nil, errors.Wrapf(err, "label index hashmap section at %d", loff)
	}
	if r.labels, err = readHashmap(flag, b); err != nil {
		return nil, errors.Wrap(err, "read label index hashmap")
	}
	flag, b, err = r.section(poff)
	if err != nil {
		return nil, errors.Wrapf(err, "postings hashmap section at %d", loff)
	}
	if r.postings, err = readHashmap(flag, b); err != nil {
		return nil, errors.Wrap(err, "read postings hashmap")
	}

	return r, nil
}

func readHashmap(flag byte, b []byte) (map[string]uint32, error) {
	if flag != flagStd {
		return nil, errInvalidFlag
	}
	h := make(map[string]uint32, 512)

	for len(b) > 0 {
		l, n := binary.Uvarint(b)
		if n < 1 {
			return nil, errors.Wrap(errInvalidSize, "read key length")
		}
		b = b[n:]

		if len(b) < int(l) {
			return nil, errors.Wrap(errInvalidSize, "read key")
		}
		s := string(b[:l])
		b = b[l:]

		o, n := binary.Uvarint(b)
		if n < 1 {
			return nil, errors.Wrap(errInvalidSize, "read offset value")
		}
		b = b[n:]

		h[s] = uint32(o)
	}

	return h, nil
}

func (r *indexReader) Close() error {
	return r.c.Close()
}

func (r *indexReader) section(o uint32) (byte, []byte, error) {
	b := r.b[o:]

	if len(b) < 5 {
		return 0, nil, errors.Wrap(errInvalidSize, "read header")
	}

	flag := b[0]
	l := binary.BigEndian.Uint32(b[1:5])

	b = b[5:]

	// b must have the given length plus 4 bytes for the CRC32 checksum.
	if len(b) < int(l)+4 {
		return 0, nil, errors.Wrap(errInvalidSize, "section content")
	}
	return flag, b[:l], nil
}

func (r *indexReader) lookupSymbol(o uint32) (string, error) {
	if int(o) > len(r.b) {
		return "", errors.Errorf("invalid symbol offset %d", o)
	}
	l, n := binary.Uvarint(r.b[o:])
	if n < 0 {
		return "", errors.New("reading symbol length failed")
	}

	end := int(o) + n + int(l)
	if end > len(r.b) {
		return "", errors.New("invalid length")
	}
	b := r.b[int(o)+n : end]

	return yoloString(b), nil
}

func (r *indexReader) LabelValues(names ...string) (StringTuples, error) {
	key := strings.Join(names, string(sep))
	off, ok := r.labels[key]
	if !ok {
		// XXX(fabxc): hot fix. Should return a partial data error and handle cases
		// where the entire block has no data gracefully.
		return emptyStringTuples{}, nil
		//return nil, fmt.Errorf("label index doesn't exist")
	}

	flag, b, err := r.section(off)
	if err != nil {
		return nil, errors.Wrapf(err, "section at %d", off)
	}
	if flag != flagStd {
		return nil, errInvalidFlag
	}
	l, n := binary.Uvarint(b)
	if n < 1 {
		return nil, errors.Wrap(errInvalidSize, "read label index size")
	}

	st := &serializedStringTuples{
		l:      int(l),
		b:      b[n:],
		lookup: r.lookupSymbol,
	}
	return st, nil
}

type emptyStringTuples struct{}

func (emptyStringTuples) At(i int) ([]string, error) { return nil, nil }
func (emptyStringTuples) Len() int                   { return 0 }

func (r *indexReader) LabelIndices() ([][]string, error) {
	res := [][]string{}

	for s := range r.labels {
		res = append(res, strings.Split(s, string(sep)))
	}
	return res, nil
}

func (r *indexReader) Series(ref uint32) (labels.Labels, []*ChunkMeta, error) {
	k, n := binary.Uvarint(r.b[ref:])
	if n < 1 {
		return nil, nil, errors.Wrap(errInvalidSize, "number of labels")
	}

	b := r.b[int(ref)+n:]
	lbls := make(labels.Labels, 0, k)

	for i := 0; i < 2*int(k); i += 2 {
		o, m := binary.Uvarint(b)
		if m < 1 {
			return nil, nil, errors.Wrap(errInvalidSize, "symbol offset")
		}
		n, err := r.lookupSymbol(uint32(o))
		if err != nil {
			return nil, nil, errors.Wrap(err, "symbol lookup")
		}
		b = b[m:]

		o, m = binary.Uvarint(b)
		if m < 1 {
			return nil, nil, errors.Wrap(errInvalidSize, "symbol offset")
		}
		v, err := r.lookupSymbol(uint32(o))
		if err != nil {
			return nil, nil, errors.Wrap(err, "symbol lookup")
		}
		b = b[m:]

		lbls = append(lbls, labels.Label{
			Name:  n,
			Value: v,
		})
	}

	// Read the chunks meta data.
	k, n = binary.Uvarint(b)
	if n < 1 {
		return nil, nil, errors.Wrap(errInvalidSize, "number of chunks")
	}

	b = b[n:]
	chunks := make([]*ChunkMeta, 0, k)

	for i := 0; i < int(k); i++ {
		firstTime, n := binary.Varint(b)
		if n < 1 {
			return nil, nil, errors.Wrap(errInvalidSize, "first time")
		}
		b = b[n:]

		lastTime, n := binary.Varint(b)
		if n < 1 {
			return nil, nil, errors.Wrap(errInvalidSize, "last time")
		}
		b = b[n:]

		o, n := binary.Uvarint(b)
		if n < 1 {
			return nil, nil, errors.Wrap(errInvalidSize, "chunk offset")
		}
		b = b[n:]

		chunks = append(chunks, &ChunkMeta{
			Ref:     o,
			MinTime: firstTime,
			MaxTime: lastTime,
		})
	}

	return lbls, chunks, nil
}

func (r *indexReader) Postings(name, value string) (Postings, error) {
	key := name + string(sep) + value

	off, ok := r.postings[key]
	if !ok {
		return emptyPostings, nil
	}

	flag, b, err := r.section(off)
	if err != nil {
		return nil, errors.Wrapf(err, "section at %d", off)
	}

	if flag != flagStd {
		return nil, errors.Wrapf(errInvalidFlag, "section at %d", off)
	}

	// Add iterator over the bytes.
	if len(b)%4 != 0 {
		return nil, errors.Wrap(errInvalidSize, "plain postings entry")
	}
	return newBigEndianPostings(b), nil
}

type stringTuples struct {
	l int      // tuple length
	s []string // flattened tuple entries
}

func newStringTuples(s []string, l int) (*stringTuples, error) {
	if len(s)%l != 0 {
		return nil, errors.Wrap(errInvalidSize, "string tuple list")
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
	lookup func(uint32) (string, error)
}

func (t *serializedStringTuples) Len() int {
	// TODO(fabxc): Cache this?
	return len(t.b) / (4 * t.l)
}

func (t *serializedStringTuples) At(i int) ([]string, error) {
	if len(t.b) < (i+t.l)*4 {
		return nil, errInvalidSize
	}
	res := make([]string, 0, t.l)

	for k := 0; k < t.l; k++ {
		offset := binary.BigEndian.Uint32(t.b[(i+k)*4:])

		s, err := t.lookup(offset)
		if err != nil {
			return nil, errors.Wrap(err, "symbol lookup")
		}
		res = append(res, s)
	}

	return res, nil
}
