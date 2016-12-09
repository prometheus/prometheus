package tsdb

import (
	"fmt"
	"hash/crc64"
	"io"
	"sort"
	"unsafe"

	"github.com/fabxc/tsdb/chunks"
)

const (
	magicIndex  = 0xCAFECAFE
	magicSeries = 0xAFFEAFFE
)

// Block handles reads against a block of time series data within a time window.
type Block interface{}

type block interface {
	stats() *blockStats
	seriesData() seriesDataIterator
}

type persistedBlock struct {
}

type seriesDataIterator interface {
	next() bool
	values() (skiplist, []chunks.Chunk)
	err() error
}

func compactBlocks(a, b block) error {
	return nil
}

type persistedSeries struct {
	size    int
	dataref []byte
	data    *[maxMapSize]byte
}

const (
	flagNone = 0
	flagStd  = 1
)

const (
	seriesMetaSize  = int(unsafe.Sizeof(seriesMeta{}))
	seriesStatsSize = int(unsafe.Sizeof(blockStats{}))
)

type seriesMeta struct {
	magic uint32
	flag  byte
	_     [7]byte // padding/reserved
}

type blockStats struct {
	chunks  uint32
	samples uint64
	_       [4]byte // padding/reserved
}

func (s *persistedSeries) meta() *seriesMeta {
	return (*seriesMeta)(unsafe.Pointer(&s.data[0]))
}

func (s *persistedSeries) stats() *blockStats {
	// The stats start right behind the block meta data.
	return (*blockStats)(unsafe.Pointer(&s.data[seriesMetaSize]))
}

// seriesAt returns the series stored at offset as a skiplist and the chunks
// it points to as a byte slice.
func (s *persistedSeries) seriesAt(offset int) (skiplist, []byte, error) {
	offset += seriesMetaSize
	offset += seriesStatsSize

	switch b := s.data[offset]; b {
	case flagStd:
	default:
		return nil, nil, fmt.Errorf("invalid flag: %x", b)
	}
	offset++

	var (
		slLen  = *(*uint16)(unsafe.Pointer(&s.data[offset]))
		slSize = int(slLen) / int(unsafe.Sizeof(skiplistPair{}))
		sl     = ((*[maxAllocSize]skiplistPair)(unsafe.Pointer(&s.data[offset+2])))[:slSize]
	)
	offset += 3

	chunksLen := *(*uint32)(unsafe.Pointer(&s.data[offset]))
	chunks := ((*[maxAllocSize]byte)(unsafe.Pointer(&s.data[offset])))[:chunksLen]

	return simpleSkiplist(sl), chunks, nil
}

// A skiplist maps offsets to values. The values found in the data at an
// offset are strictly greater than the indexed value.
type skiplist interface {
	// A skiplist can serialize itself into a writer.
	io.WriterTo

	// offset returns the offset to data containing values of x and lower.
	offset(x int64) (uint32, bool)
}

// simpleSkiplist is a slice of plain value/offset pairs.
type simpleSkiplist []skiplistPair

type skiplistPair struct {
	value  int64
	offset uint32
}

func (sl simpleSkiplist) offset(x int64) (uint32, bool) {
	// Search for the first offset that contains data greater than x.
	i := sort.Search(len(sl), func(i int) bool { return sl[i].value >= x })

	// If no element was found return false. If the first element is found,
	// there's no previous offset actually containing values that are x or lower.
	if i == len(sl) || i == 0 {
		return 0, false
	}
	return sl[i-1].offset, true
}

func (sl simpleSkiplist) WriteTo(w io.Writer) (n int64, err error) {
	for _, s := range sl {
		b := ((*[unsafe.Sizeof(skiplistPair{})]byte)(unsafe.Pointer(&s)))[:]

		m, err := w.Write(b)
		if err != nil {
			return n + int64(m), err
		}
		n += int64(m)
	}
	return n, err
}

type blockWriter struct {
	block block
}

func (bw *blockWriter) writeSeries(ow io.Writer) (n int64, err error) {
	// Duplicate all writes through a CRC64 hash writer.
	h := crc64.New(crc64.MakeTable(crc64.ECMA))
	w := io.MultiWriter(h, ow)

	// Write file header including padding.
	//
	// XXX(fabxc): binary.Write is theoretically more appropriate for serialization.
	// However, we'll have to pick correct endianness for the unsafe casts to work
	// when reading again. That and the added slowness due to reflection seem to make
	// it somewhat pointless.
	meta := &seriesMeta{magic: magicSeries, flag: flagStd}
	metab := ((*[seriesMetaSize]byte)(unsafe.Pointer(meta)))[:]

	m, err := w.Write(metab)
	if err != nil {
		return n + int64(m), err
	}
	n += int64(m)

	// Write stats section including padding.
	statsb := ((*[seriesStatsSize]byte)(unsafe.Pointer(bw.block.stats())))[:]

	m, err = w.Write(statsb)
	if err != nil {
		return n + int64(m), err
	}
	n += int64(m)

	// Write series data sections.
	//
	// TODO(fabxc): cache the offsets so we can use them on writing down the index.
	it := bw.block.seriesData()

	for it.next() {
		sl, chunks := it.values()

		m, err := sl.WriteTo(w)
		if err != nil {
			return n + int64(m), err
		}
		n += int64(m)

		for _, c := range chunks {
			m, err := w.Write(c.Bytes())
			if err != nil {
				return n + int64(m), err
			}
			n += int64(m)
		}
	}
	if it.err() != nil {
		return n, it.err()
	}

	// Write checksum to the original writer.
	m, err = ow.Write(h.Sum(nil))
	return n + int64(m), err
}
