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

package fields

import (
	"github.com/cespare/xxhash/v2"
	"github.com/gernest/roaring"
)

type Field byte

const (
	Unknown Field = 1 + iota
	Series
	Labels
	Timestamp
	Value
	Histogram
	Exemplar
	Kind
	Metadata

	Tenant
	Shards
	eof
)

var SampleTable = []Field{
	Series, Kind, Labels, Timestamp, Value,
}

func Source(table []Field) (o [][]string) {
	o = make([][]string, 0, len(table)+1)
	o = append(o, []string{"Name", "DataType"})
	for _, f := range table {
		o = append(o, []string{f.String(), f.Type().String()})
	}
	return
}

func (f Field) Type() DataType {
	switch f {
	case Series:
		return BitSliced
	case Kind:
		return Equality
	case Labels:
		return Equality
	case Timestamp:
		return BitSliced
	case Value:
		return BitSliced
	default:
		return DataType(0)
	}
}

func (f Field) String() string {
	switch f {
	case Series:
		return "series"
	case Kind:
		return "kind"
	case Labels:
		return "labels"
	case Timestamp:
		return "timestamp"
	case Value:
		return "value"
	default:
		return ""
	}
}

type DataType byte

const (
	Equality DataType = 1 + iota
	BitSliced
)

func (d DataType) String() string {
	switch d {
	case Equality:
		return "Equality"
	case BitSliced:
		return "BSI"
	default:
		return ""
	}
}

var hash = make([]uint64, eof)

func init() {
	var o [1]byte
	var h xxhash.Digest
	for i := Unknown; i < eof; i++ {
		o[0] = byte(i)
		h.Reset()
		_, _ = h.Write(o[:])
		hash[i] = h.Sum64()
	}
}

func (f Field) Hash() uint64 {
	return hash[f]
}

type BaseMapping map[uint64]map[uint64]*roaring.Bitmap

func (p BaseMapping) Iter(f func(tenant, shard uint64, ra *roaring.Bitmap) error) error {
	for tenant, s := range p {
		for shard, ra := range s {
			err := f(tenant, shard, ra)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (p BaseMapping) Set(tenant, shard, field uint64) {
	s, ok := p[tenant]
	if !ok {
		s = make(map[uint64]*roaring.Bitmap)
		p[tenant] = s
	}
	f, ok := s[shard]
	if !ok {
		f = roaring.NewBitmap(field)
		s[shard] = f
		return
	}
	_, _ = f.Add(field)
}
