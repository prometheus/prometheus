// Copyright The Prometheus Authors
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

package compression

import (
	"errors"
	"fmt"

	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
)

// Type represents the compression type used for encoding and decoding data.
type Type string

const (
	// None represents no compression case.
	// None it's a default when Type is empty.
	None Type = "none"
	// Snappy represents snappy block format.
	Snappy Type = "snappy"
	// Zstd represents zstd compression.
	Zstd Type = "zstd"
)

// Encoder provides compression encoding functionality for supported compression
// types. It is agnostic to the content being compressed, operating on byte
// slices of serialized data streams. The encoder maintains internal state for
// Zstd compression and can handle multiple compression types including None,
// Snappy, and Zstd.
type Encoder struct {
	w *zstd.Encoder
}

// NewEncoder creates a new Encoder. Returns an error if the zstd encoder cannot
// be initialized.
func NewEncoder() (*Encoder, error) {
	e := &Encoder{}
	w, err := zstd.NewWriter(nil)
	if err != nil {
		return nil, err
	}
	e.w = w
	return e, nil
}

// Encode returns the encoded form of src for the given compression type. It also
// returns the indicator if the compression was performed. Encode may skip
// compressing for None type, but also when src is too large e.g. for Snappy block format.
//
// The buf is used as a buffer for returned encoding, and it must not overlap with
// src. It is valid to pass a nil buf.
func (e *Encoder) Encode(t Type, src, buf []byte) (_ []byte, compressed bool, err error) {
	switch {
	case len(src) == 0, t == "", t == None:
		return src, false, nil
	case t == Snappy:
		// If MaxEncodedLen is less than 0 the record is too large to be compressed.
		if snappy.MaxEncodedLen(len(src)) < 0 {
			return src, false, nil
		}

		// The snappy library uses `len` to calculate if we need a new buffer.
		// In order to allocate as few buffers as possible make the length
		// equal to the capacity.
		buf = buf[:cap(buf)]
		return snappy.Encode(buf, src), true, nil
	case t == Zstd:
		if e == nil {
			return nil, false, errors.New("zstd requested but encoder was not initialized with NewEncoder()")
		}
		return e.w.EncodeAll(src, buf[:0]), true, nil
	default:
		return nil, false, fmt.Errorf("unsupported compression type: %s", t)
	}
}

// Decoder provides decompression functionality for supported compression types.
// It is agnostic to the content being decompressed, operating on byte slices of
// serialized data streams. The decoder maintains internal state for Zstd
// decompression and can handle multiple compression types including None,
// Snappy, and Zstd.
type Decoder struct {
	r *zstd.Decoder
}

// NewDecoder creates a new Decoder.
func NewDecoder() *Decoder {
	d := &Decoder{}

	// Calling zstd.NewReader with a nil io.Reader and no options cannot return an error.
	r, _ := zstd.NewReader(nil)
	d.r = r
	return d
}

// Decode returns the decoded form of src or error, given expected compression type.
//
// The buf is used as a buffer for the returned decoded entry, and it must not
// overlap with src. It is valid to pass a nil buf.
func (d *Decoder) Decode(t Type, src, buf []byte) (_ []byte, err error) {
	switch {
	case len(src) == 0, t == "", t == None:
		return src, nil
	case t == Snappy:
		// The snappy library uses `len` to calculate if we need a new buffer.
		// In order to allocate as few buffers as possible make the length
		// equal to the capacity.
		buf = buf[:cap(buf)]
		return snappy.Decode(buf, src)
	case t == Zstd:
		if d == nil {
			return nil, errors.New("zstd requested but Decoder was not initialized with NewDecoder()")
		}
		return d.r.DecodeAll(src, buf[:0])
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", t)
	}
}
