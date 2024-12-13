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

package batch

import (
	"encoding/binary"

	"github.com/cockroachdb/pebble"
	"github.com/gernest/roaring"
	"github.com/gernest/roaring/shardwidth"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdbv2/cache"
	"github.com/prometheus/prometheus/tsdbv2/encoding"
	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
	"github.com/prometheus/prometheus/tsdbv2/fields"
	"github.com/prometheus/prometheus/tsdbv2/translate"
)

type DB interface {
	translate.Translator
	SetRef(tenant uint64, series cache.Series) error
	GetRef(tenant uint64, ref storage.SeriesRef, f func(val []byte) error) error
}

type batchKey struct {
	tenant    uint64
	shard     uint64
	field     uint64
	existence bool
}

type Batch struct {
	tr               DB
	ba               *pebble.Batch
	fields           map[batchKey]*roaring.Bitmap
	seq              seqBatch
	shards           *roaring.Bitmap
	baseFieldMapping fields.BaseMapping
	shard            map[uint64]uint64
	cache            struct {
		fields []uint64
		ids    []uint64
		series cache.Series
	}
	writer            bitmaps.Batch
	defaultRootTenant bool
}

func New(ba *pebble.Batch, tr DB, root bool) *Batch {
	return &Batch{
		tr:                tr,
		ba:                ba,
		defaultRootTenant: root,
		fields:            make(map[batchKey]*roaring.Bitmap),
		seq:               make(seqBatch),
		shards:            roaring.NewBitmap(),
		shard:             make(map[uint64]uint64),
		baseFieldMapping:  make(fields.BaseMapping),
	}
}

var _ storage.Appender = (*Batch)(nil)

func (ba *Batch) Release() {
	*ba = Batch{}
}

func (ba *Batch) nextSeq(tenant, field uint64) (uint64, error) {
	nxt, err := ba.tr.NextSeq(tenant, field)
	if err != nil {
		return 0, err
	}
	shard := nxt / shardwidth.ShardWidth
	ba.shard[tenant] = shard
	ba.seq.Set(tenant, field, nxt)
	bitmaps.Equality(ba.shards, shard, tenant)
	return nxt, nil
}

func (ba *Batch) Commit() error {
	defer ba.Release()

	for k, v := range ba.fields {
		err := ba.commitKey(k, v)
		if err != nil {
			_ = ba.Rollback()
			return err
		}
	}

	err := ba.commitKey(
		batchKey{
			tenant: encoding.RootTenant,
			field:  fields.Shards.Hash(),
		}, ba.shards)
	if err != nil {
		_ = ba.Rollback()
		return err
	}

	key := make([]byte, 1+8+8)
	var b [8]byte
	err = ba.seq.Iter(func(tenant, field, value uint64) error {
		key = encoding.AppendSeq(tenant, field, key[:0])
		binary.BigEndian.PutUint64(b[:], value)
		return ba.ba.Set(key, b[:], nil)
	})
	if err != nil {
		_ = ba.Rollback()
		return err
	}

	err = ba.baseFieldMapping.Iter(func(tenant, shard uint64, ra *roaring.Bitmap) error {
		return ra.ForEach(func(u uint64) error {
			key = encoding.AppendFieldMapping(tenant, shard, u, key[:0])
			return ba.ba.Set(key, nil, nil)
		})
	})
	if err != nil {
		_ = ba.ba.Close()
		return err
	}
	return ba.ba.Commit(nil)
}

func (ba *Batch) commitKey(key batchKey, ra *roaring.Bitmap) error {
	if !ra.Any() {
		return nil
	}
	if key.existence {
		return ba.writer.WriteExistence(ba.ba, key.tenant, key.shard, key.field, ra)
	}
	return ba.writer.WriteData(ba.ba, key.tenant, key.shard, key.field, ra)
}

func (ba *Batch) Rollback() error {
	defer ba.Release()
	return ba.ba.Close()
}

func (ba *Batch) SetOptions(_ *storage.AppendOptions) {}

type seqBatch map[uint64]map[uint64]uint64

func (ba seqBatch) Set(tenant, field, id uint64) {
	t, ok := ba[tenant]
	if !ok {
		t = make(map[uint64]uint64)
		ba[tenant] = t
	}
	t[field] = id
}

func (ba seqBatch) Iter(f func(tenant, field, value uint64) error) error {
	for t, fv := range ba {
		for k, v := range fv {
			err := f(t, k, v)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
