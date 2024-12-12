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
	"github.com/gernest/roaring"
)

type Filter interface {
	Apply(r Reader, shard uint64) *roaring.Bitmap
}

type And struct {
	Left, Right Filter
}

var _ Filter = (*And)(nil)

func (a And) Apply(r Reader, shard uint64) *roaring.Bitmap {
	if a.Left == nil {
		return a.Right.Apply(r, shard)
	}
	left := a.Left.Apply(r, shard)
	if !left.Any() {
		return left
	}
	return left.Intersect(a.Right.Apply(r, shard))
}

type Or struct {
	Left, Right Filter
}

var _ Filter = (*Or)(nil)

func (o Or) Apply(r Reader, shard uint64) *roaring.Bitmap {
	if o.Left == nil {
		return o.Right.Apply(r, shard)
	}
	left := o.Left.Apply(r, shard)
	return left.Union(o.Right.Apply(r, shard))
}

type Yes struct {
	Values []uint64
	Field  uint64
}

var _ Filter = (*Yes)(nil)

func (f *Yes) Apply(r Reader, shard uint64) *roaring.Bitmap {
	if len(f.Values) == 0 {
		return roaring.NewBitmap()
	}
	if !r.ResetData(shard, f.Field) {
		return roaring.NewBitmap()
	}
	all := make([]*roaring.Bitmap, 0, len(f.Values))
	for _, v := range f.Values {
		all = append(all, Row(r, shard, v))
	}
	b := all[0]
	match := b.Union(all[1:]...)
	return match
}

type No struct {
	Values []uint64
	Field  uint64
}

var _ Filter = (*No)(nil)

func (f *No) Apply(r Reader, shard uint64) *roaring.Bitmap {
	if len(f.Values) == 0 {
		return roaring.NewBitmap()
	}
	if !r.ResetExistence(shard, f.Field) {
		return roaring.NewBitmap()
	}
	ex := Existence(r, shard)
	if !r.ResetData(shard, f.Field) {
		return roaring.NewBitmap()
	}
	all := make([]*roaring.Bitmap, 0, len(f.Values))
	for _, v := range f.Values {
		all = append(all, ex.Difference(Row(r, shard, v)))
	}
	b := all[0]
	return b.Union(all[1:]...)
}
