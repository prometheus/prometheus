package chunks

import (
	"encoding/binary"
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
			b:   &bstream{count: 0, stream: d},
			num: binary.BigEndian.Uint16(d),
		}, nil
	}
	return nil, fmt.Errorf("unknown chunk encoding: %d", e)
}

// Appender adds sample pairs to a chunk.
type Appender interface {
	Append(int64, float64)
}

// Iterator is a simple iterator that can only get the next value.
type Iterator interface {
	At() (int64, float64)
	Err() error
	Next() bool
}
