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

package cursor

import (
	"bytes"
	"encoding/binary"
	"math"
	"slices"

	"github.com/cockroachdb/pebble"
	"github.com/gernest/roaring"
	"github.com/gernest/roaring/shardwidth"

	"github.com/prometheus/prometheus/tsdbv2/encoding"
	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
	"github.com/prometheus/prometheus/tsdbv2/fields"
)

// Cursor is a wrapper around pebble.Iterator that provides method to travers over
// bitmap containers.
//
// Only works with a single tenant.
type Cursor struct {
	it     *pebble.Iterator
	lo, hi encoding.Key
	tenant uint64
}

var _ bitmaps.Reader = (*Cursor)(nil)

func New(it *pebble.Iterator, tenant uint64) *Cursor {
	return &Cursor{it: it, tenant: tenant}
}

func (cu *Cursor) Reset() {
	cu.lo.Reset()
	cu.hi.Reset()
}

// ResetData seeks to the first data bitmap container for shard / field . Returns true
// if a matching container was found.
func (cu *Cursor) ResetData(shard, field uint64) bool {
	cu.Reset()
	cu.lo.SetData(cu.tenant, shard, field, 0)
	cu.hi.SetData(cu.tenant, shard, field, math.MaxUint64)
	return cu.it.SeekGE(cu.lo[:]) && cu.Valid()
}

// First is a wrapper for Seek(0).
func (cu *Cursor) First() bool {
	return cu.Seek(0)
}

// ResetExistence seeks to the first existence bitmap container for shard / field . Returns true
// if a matching container was found.
func (cu *Cursor) ResetExistence(shard, field uint64) bool {
	cu.Reset()
	cu.lo.SetExistence(cu.tenant, shard, field, 0)
	cu.hi.SetExistence(cu.tenant, shard, field, math.MaxUint64)
	return cu.it.SeekGE(cu.lo[:]) && cu.Valid()
}

// Valid returns true if the cursor is at a valid container belonging to the same field
// we positioned with ResetData or ResetExistence.
func (cu *Cursor) Valid() bool {
	return cu.it.Valid() &&
		bytes.Compare(cu.it.Key(), cu.hi[:]) == -1
}

// Next advances the cursor to the next container. Return true if a valid container was found.
func (cu *Cursor) Next() bool {
	ok := cu.it.Next() && cu.Valid()
	if !ok {
		return false
	}
	copy(cu.lo[:], cu.it.Key())
	return true
}

// Key returns container Key of the current position of the cursor.
func (cu *Cursor) Key() uint64 {
	return encoding.ContainerKey(cu.it.Key())
}

// Value returns current container and its key pointed by the cursor.
func (cu *Cursor) Value() (uint64, *roaring.Container) {
	key := cu.it.Key()
	return encoding.ContainerKey(key), roaring.DecodeContainer(cu.it.Value())
}

// Container returns container  of the current position of the cursor.
func (cu *Cursor) Container() *roaring.Container {
	return roaring.DecodeContainer(cu.it.Value())
}

// Max returns maximum value recorded by the field bitmap. It is mainly used to detect
// bit depth with BSI encoded fields.
func (cu *Cursor) Max() uint64 {
	if !cu.it.SeekLT(cu.hi[:]) {
		return 0
	}
	key := cu.it.Key()
	if bytes.Compare(key, cu.lo[:]) == -1 {
		return 0
	}
	ck := encoding.ContainerKey(key)
	value := roaring.LastValueFromEncodedContainer(cu.it.Value())
	return ((ck << 16) | uint64(value))
}

// ClearRecords finds all containers matching filter match and adds their difference to u.
func (cu *Cursor) ClearRecords(match *roaring.Bitmap, u *Update) {
	cu.MatchBitmap(match, func(key *encoding.Key, data, filter *roaring.Container) {
		u.Add(key, data.DifferenceInPlace(filter))
	})
}

// MatchBitmap traverse current field and finds all containers that match filter. Calls f
// with  qualified container key, matched container with correspoding container found in passed filter.
//
// Used by drop package to clearremove bits from records.
func (cu *Cursor) MatchBitmap(filter *roaring.Bitmap, f func(key *encoding.Key, data, filter *roaring.Container)) {
	containers := make([]*roaring.Container, bitmaps.RowWidth)
	nextOffsets := make([]uint64, bitmaps.RowWidth)
	iter, _ := filter.Containers.Iterator(0)
	last := uint64(0)
	count := 0
	for iter.Next() {
		k, v := iter.Value()
		k &= bitmaps.KeyMask
		containers[k] = v
		last = k
		count++
	}
	if count == 1 {
		for i := range containers {
			nextOffsets[i] = last
		}
	} else {
		for i := range containers {
			if containers[i] != nil {
				for int(last) != i {
					nextOffsets[last] = uint64(i)
					last = (last + 1) % bitmaps.RowWidth
				}
			}
		}
	}
	cu.Seek(0)
	for cu.Valid() {
		key := roaring.FilterKey(cu.Key())
		pos := key & bitmaps.KeyMask
		if containers[pos] == nil {
			res := key.RejectUntilOffset(nextOffsets[pos])
			if res.NoKey > (key + 64) {
				cu.Seek(uint64(res.NoKey))
				continue
			}
			cu.Next()
			continue
		}
		co := cu.Container()
		if roaring.IntersectionAny(co, containers[pos]) {
			f(&cu.lo, co, containers[pos])
			cu.Next()
			continue
		}
		res := key.RejectUntilOffset(nextOffsets[pos])
		if res.NoKey > (key + 64) {
			cu.Seek(uint64(res.NoKey))
			continue
		}
		cu.Next()
	}
}

// Seek moves the iterator cursor to container matching key or the nearest container to key.
func (cu *Cursor) Seek(key uint64) bool {
	cu.lo.SetContainer(key)
	return cu.it.SeekGE(cu.lo[:]) && cu.Valid()
}

func (cu *Cursor) OffsetRange(offset, start, endx uint64) *roaring.Bitmap {
	other := roaring.NewSliceBitmap()
	off := highbits(offset)
	hi0, hi1 := highbits(start), highbits(endx)
	if !cu.Seek(hi0) {
		return other
	}
	for ; cu.Valid(); cu.it.Next() {
		key := cu.it.Key()
		ckey := binary.BigEndian.Uint64(key[len(key)-8:])
		if ckey >= hi1 {
			break
		}
		other.Containers.Put(off+(ckey-hi0), roaring.DecodeContainer(cu.it.Value()).Clone())
	}
	return other
}

func (cu *Cursor) TimeRange(shard uint64, match *roaring.Bitmap, mint, maxt int64, kinds ...encoding.SeriesKind) *roaring.Bitmap {
	if len(kinds) > 0 {
		// select kind
		all := make([]*roaring.Bitmap, 0, len(kinds))
		if !cu.ResetData(shard, fields.Kind.Hash()) {
			return roaring.NewBitmap()
		}
		for _, kind := range kinds {
			all = append(all, bitmaps.Row(cu, shard, uint64(kind)))
		}
		kindMatch := all[0].Union(all[1:]...)
		if match == nil {
			match = kindMatch
		} else {
			match = match.Intersect(kindMatch)
		}
		if !match.Any() {
			return match
		}
	}
	if !cu.ResetData(shard, fields.Timestamp.Hash()) {
		return roaring.NewBitmap()
	}
	depth := cu.Max() / shardwidth.ShardWidth
	ra := bitmaps.Range(cu, bitmaps.BETWEEN, shard, depth, mint, maxt)
	if match != nil {
		return ra.Intersect(match)
	}
	return ra
}

func (cu *Cursor) ReadBSI(field, shard uint64, match *roaring.Bitmap) []uint64 {
	if !cu.ResetData(shard, field) {
		return make([]uint64, match.Count())
	}
	return bitmaps.ExtractBSI(cu, shard, match)
}

func (cu *Cursor) ReadEquality(field fields.Field, shard uint64, match *roaring.Bitmap) []uint64 {
	if !cu.ResetData(shard, field.Hash()) {
		return make([]uint64, match.Count())
	}
	return bitmaps.ExtractEquality(cu, shard, match)
}

// Update tracks container details scheduled for update/deletion. We intentionally dont use
// []byte container keys to simplify reuse and ensures we will alwats work with correct
// containers.
//
// Post processing after populating can be done by individual data key component, all
// fields are guaranteed to be of the same size and each index on a field refers to the same
// record for each field.
type Update struct {
	Tenant    []uint64
	Shard     []uint64
	Field     []uint64
	Key       []uint64
	Container []*roaring.Container
}

// IterKey iterate over all record containers scheduled for update. Reuses key to generate qualified
// database key for each container.
//
// co is quaranteed to nver be nil, use co.N()==0 to check for empty containers.
// Halts iteration when f returns a non nil error.
func (u *Update) IterKey(key *encoding.Key, f func(key []byte, co *roaring.Container) error) error {
	for i := range u.Key {
		key.SetData(
			u.Tenant[i],
			u.Shard[i],
			u.Field[i],
			u.Key[i],
		)
		err := f(key[:], u.Container[i])
		if err != nil {
			return err
		}
	}
	return nil
}

// Reserve extends u capacity to n containers.
func (u *Update) Reserve(n int) {
	u.Tenant = slices.Grow(u.Tenant, n)
	u.Shard = slices.Grow(u.Shard, n)
	u.Field = slices.Grow(u.Field, n)
	u.Key = slices.Grow(u.Key, n)
	u.Container = slices.Grow(u.Container, n)
}

// Add records container co withe the qiven qualified key.
func (u *Update) Add(key *encoding.Key, co *roaring.Container) {
	u.Tenant = append(u.Tenant, key.Tenant())
	u.Shard = append(u.Shard, key.Shard())
	u.Field = append(u.Field, key.Field())
	u.Key = append(u.Key, key.Container())
	u.Container = append(u.Container, co)
}

func highbits(v uint64) uint64 { return v >> 16 }
