package tsdb

import (
	"encoding/binary"
	"fmt"

	"github.com/fabxc/tsdb/chunks"
)

// SeriesReader provides reading access of serialized time series data.
type SeriesReader interface {
	// Chunk returns the series data chunk at the given offset.
	Chunk(offset uint32) (chunks.Chunk, error)
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
	//

	// Close releases resources associated with the reader.
	Close()
}

type indexReader struct {
	// The underlying byte slice holding the encoded series data.
	b []byte

	// Cached hashmaps of sections for label values
	labelOffsets map[string]uint32
}
