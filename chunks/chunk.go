// Copyright 2017 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chunks

import (
	"encoding/binary"
	"fmt"
)

// Encoding is the identifier for a chunk encoding.
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
	NumSamples() int
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
