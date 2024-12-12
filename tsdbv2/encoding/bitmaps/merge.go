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

package bitmaps

import (
	"io"

	"github.com/cockroachdb/pebble"
	"github.com/gernest/roaring"
)

type Bitmap = roaring.Bitmap

type Merger roaring.Container

var _ pebble.ValueMerger = (*Merger)(nil)

func (m *Merger) MergeNewer(value []byte) error {
	return m.merge(value)
}

func (m *Merger) MergeOlder(value []byte) error {
	return m.merge(value)
}

func (m *Merger) Finish(includesBase bool) ([]byte, io.Closer, error) {
	co := (*roaring.Container)(m)
	return co.Encode(), nil, nil
}

func (m *Merger) merge(value []byte) error {
	n := roaring.Union((*roaring.Container)(m), roaring.DecodeContainer(value))
	co := (*Merger)(n)
	*m = *co
	return nil
}

var Merge = &pebble.Merger{
	Name: "pilosa.RoaringBitmap",
	Merge: func(key, value []byte) (pebble.ValueMerger, error) {
		co := roaring.DecodeContainer(value).Clone()
		return (*Merger)(co), nil
	},
}
