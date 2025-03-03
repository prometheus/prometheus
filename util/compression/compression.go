// Copyright 2025 The Prometheus Authors
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
)

// Type represents a valid compression type supported by this package.
type Type = string

const (
	// None represents no compression case.
	// None it's a default when Type is empty.
	None Type = "none"
	// Snappy represents snappy block format.
	Snappy Type = "snappy"
	// Zstd represents "speed" mode of Zstd (Zstandard https://facebook.github.io/zstd/).
	// This is roughly equivalent to the default Zstandard mode (level 3).
	Zstd Type = "zstd"
)

func Types() []Type { return []Type{None, Snappy, Zstd} }

// Encode returns the encoded form of src for the given compression type. It also
// returns the indicator if the compression was performed. Encode may skip
// compressing for None type, but also when src is too large e.g. for Snappy block format.
//
// The buf allows passing various buffer implementations that make encoding more
// efficient. See NewSyncEncodeBuffer and NewConcurrentEncodeBuffer for further
// details. For non-zstd compression types, it is valid to pass nil buf.
//
// Encode is concurrency-safe, however note the concurrency limits for the
// buffer of your choice.
func Encode(t Type, src []byte, buf EncodeBuffer) (ret []byte, compressed bool, err error) {
	var b []byte
	if buf != nil {
		b = buf.get()
		defer func() {
			buf.put(ret)
		}()
	}

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
		b = b[:cap(b)]
		return snappy.Encode(b, src), true, nil
	case t == Zstd:
		if buf == nil {
			return nil, false, errors.New("zstd requested but EncodeBuffer was not provided")
		}
		return buf.zstdEncBuf().EncodeAll(src, b[:0]), true, nil
	default:
		return nil, false, fmt.Errorf("unsupported compression type: %s", t)
	}
}

// Decode returns the decoded form of src for the given compression type.
//
// The buf allows passing various buffer implementations that make decoding more
// efficient. See NewSyncDecodeBuffer and NewConcurrentDecodeBuffer for further
// details. For non-zstd compression types, it is valid to pass nil buf.
//
// Decode is concurrency-safe, however note the concurrency limits for the
// buffer of your choice.
func Decode(t Type, src []byte, buf DecodeBuffer) (ret []byte, err error) {
	var b []byte
	if buf != nil {
		b = buf.get()
		defer func() {
			buf.put(ret)
		}()
	}

	switch {
	case len(src) == 0, t == "", t == None:
		return src, nil
	case t == Snappy:
		// The snappy library uses `len` to calculate if we need a new buffer.
		// In order to allocate as few buffers as possible make the length
		// equal to the capacity.
		b = b[:cap(b)]
		return snappy.Decode(b, src)
	case t == Zstd:
		if buf == nil {
			return nil, errors.New("zstd requested but DecodeBuffer was not provided")
		}
		return buf.zstdDecBuf().DecodeAll(src, b[:0])
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", t)
	}
}
