// Copyright 2024 The Prometheus Authors
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

package cache

import (
	"unsafe"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdbv2/hash"
)

// Series encodes information relating to a series reference used during Append
// calls which includes series id and series labels.
//
//	first item is series ID followed by [ field0, field1, field2 ...., ,id1, id2, id3]
//
// We use this layout to avoid decoding step because we use this information in
// ingestion hot spot.
//
// Since we are only interpreting raw []byte, we don't need to allocate or decode
// this information when appending samples with series reference. We use data directly
// obtained from pebble and release it back when we are done.
type Series []uint64

// New reuses buf to create a new Series.
func New(id uint64, fields, ids []uint64, buf Series) Series {
	buf = buf[:0]
	buf = append(buf, id)
	buf = append(buf, fields...)
	buf = append(buf, ids...)
	return buf
}

// ID returns series ID. This is a uint64 local to the tenant.
func (l Series) ID() uint64 {
	return l[0]
}

// Ref returns a series  ref, which is a hash of l contents.
func (l Series) Ref() storage.SeriesRef {
	return storage.SeriesRef(hash.Bytes(l.Encode()))
}

// Iter terates over label information encoded in l and calls f for each field and its corresponding
// value found.
func (l Series) Iter(f func(field, value uint64)) {
	l = l[1:]
	off := len(l) / 2
	for i := 0; i < off; i++ {
		f(l[i], l[i+off])
	}
}

func (l Series) Encode() []byte {
	return reinterpretSlice[byte](l)
}

func Decode(val []byte) Series {
	return reinterpretSlice[uint64](val)
}

func reinterpretSlice[Out, T any](b []T) []Out {
	if cap(b) == 0 {
		return nil
	}
	out := (*Out)(unsafe.Pointer(&b[:1][0]))

	lenBytes := len(b) * int(unsafe.Sizeof(b[0]))
	capBytes := cap(b) * int(unsafe.Sizeof(b[0]))

	lenOut := lenBytes / int(unsafe.Sizeof(*out))
	capOut := capBytes / int(unsafe.Sizeof(*out))

	return unsafe.Slice(out, capOut)[:lenOut]
}
