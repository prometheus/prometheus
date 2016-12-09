package chunks

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Encoding is the identifier for a chunk encoding
type Encoding uint8

func (e Encoding) String() string {
	switch e {
	case EncNone:
		return "none"
	case EncXOR:
		return "XOR"
	}
	return "<unknown>"
}

// The different available chunk encodings.
const (
	EncNone Encoding = iota
	EncXOR
)

var (
	// ErrChunkFull is returned if the remaining size of a chunk cannot
	// fit the appended data.
	ErrChunkFull = errors.New("chunk full")
)

// Chunk holds a sequence of sample pairs that can be iterated over and appended to.
type Chunk interface {
	Bytes() []byte
	Encoding() Encoding
	Appender() (Appender, error)
	Iterator() Iterator
}

// FromData returns a chunk from a byte slice of chunk data.
func FromData(e Encoding, d []byte) (Chunk, error) {
	switch e {
	case EncXOR:
		return &XORChunk{
			b:   &bstream{count: 8},
			num: binary.LittleEndian.Uint16(d),
		}, nil
	}
	return nil, fmt.Errorf("unknown chunk encoding: %d", e)
}

// Iterator provides iterating access over sample pairs in chunks.
type Iterator interface {
	StreamingIterator

	// Seek(t int64) bool
	// SeekBefore(t int64) bool
	// Next() bool
	// Values() (int64, float64)
	// Err() error
}

// Appender adds sample pairs to a chunk.
type Appender interface {
	Append(int64, float64) error
}

// StreamingIterator is a simple iterator that can only get the next value.
type StreamingIterator interface {
	Values() (int64, float64)
	Err() error
	Next() bool
}

// fancyIterator wraps a StreamingIterator and implements a regular
// Iterator with it.
type fancyIterator struct {
	StreamingIterator
}
