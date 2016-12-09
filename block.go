package tsdb

import (
	"fmt"
	"io"
	"sort"
	"unsafe"
)

const (
	magicIndex  = 0xCAFECAFE
	magicSeries = 0xAFFEAFFE
)

// Block handles reads against a block of time series data within a time window.
type Block interface{}

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
	metaSize        = int(unsafe.Sizeof(meta{}))
	seriesStatsSize = int(unsafe.Sizeof(blockStats{}))
)

type meta struct {
	magic uint32
	flag  byte
	_     [7]byte // padding/reserved
}

type blockStats struct {
	chunks  uint32
	samples uint64
	_       [4]byte // padding/reserved
}

func (s *persistedSeries) meta() *meta {
	return (*meta)(unsafe.Pointer(&s.data[0]))
}

func (s *persistedSeries) stats() *blockStats {
	// The stats start right behind the block meta data.
	return (*blockStats)(unsafe.Pointer(&s.data[metaSize]))
}

// seriesAt returns the series stored at offset as a skiplist and the chunks
// it points to as a byte slice.
func (s *persistedSeries) seriesAt(offset int) (skiplist, []byte, error) {
	offset += metaSize
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
