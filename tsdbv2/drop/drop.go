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

package drop

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"log/slog"

	"github.com/cockroachdb/pebble"
	"github.com/gernest/roaring"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdbv2/cursor"
	"github.com/prometheus/prometheus/tsdbv2/encoding"
	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
	"github.com/prometheus/prometheus/tsdbv2/fields"
	"github.com/prometheus/prometheus/tsdbv2/query/filters"
	"github.com/prometheus/prometheus/tsdbv2/shards"
	"github.com/prometheus/prometheus/tsdbv2/work"
)

// Matching will remove all records for the tenant satisfying matchers, mint and maxt constraints.
//
// This is an expensive operation, care should be taken to ensure it is not executed in tight loops.
// Used to clear reacords after retention period expires or when trying to reduce size of the database.
//
// Distributes shards across available CPU cores. Processing within shards is done sequentialy. One iterator
// is used to search for containers that are to be deleted or updated(by removing matched records).
// After collecting the update information , containers with 0 bits are deleted and the rest are updated
// in a batch.
//
// Fields cleared are
//   - Series
//   - Kind
//   - Labels
//   - Timestamp
//   - Value
func Matching(db *pebble.DB, tenantID uint64, mint, maxt int64, matchers []*labels.Matcher) (err error) {
	filter := filters.Compile(db, tenantID, matchers)
	if filter == nil {
		return errors.New("tsdbv1: no matches found for deletion request")
	}
	shards, err := shards.ReadShardsForTenant(db, tenantID)
	if err != nil {
		return err
	}
	if len(shards) == 0 {
		return errors.New("tsdbv1: no shards found for deletion request")
	}
	work.Work(context.Background(), shards, dropShard(db, mint, maxt, tenantID, filter))
	return
}

func dropShard(db *pebble.DB, mint, maxt int64, tenantID uint64, filter bitmaps.Filter) func(ctx context.Context, shard []uint64) {
	return func(ctx context.Context, shards []uint64) {
		iter, err := db.NewIter(nil)
		if err != nil {
			return
		}
		defer iter.Close()

		cu := cursor.New(iter, tenantID)

		up := new(cursor.Update)
		up.Reserve(1 << 10)

		for i := range shards {
			shard := shards[i]

			match := filter.Apply(cu, shard)
			if !match.Any() {
				continue
			}
			match = cu.TimeRange(shard, match, mint, maxt)
			if !match.Any() {
				continue
			}

			removeBSI(cu, shard, fields.Series, match, up)
			removeLabels(iter, cu, tenantID, shard, match, up)
			removeBSI(cu, shard, fields.Timestamp, match, up)
			removeBSI(cu, shard, fields.Value, match, up)
		}

		ba := db.NewBatch()
		var key encoding.Key
		err = up.IterKey(&key, func(key []byte, co *roaring.Container) error {
			if co == nil || co.N() == 0 {
				return ba.Delete(key, nil)
			}
			return ba.Set(key, co.Encode(), nil)
		})
		if err != nil {
			slog.Error("applying batch delete", "err", err)
			return
		}
		err = ba.Commit(nil)
		if err != nil {
			slog.Error("commit batch delete", "err", err)
			return
		}
	}
}

func removeBSI(cu *cursor.Cursor, shard uint64, field fields.Field, match *roaring.Bitmap, u *cursor.Update) {
	if !cu.ResetData(shard, field.Hash()) {
		return
	}
	cu.ClearRecords(match, u)
}

func removeLabels(iter *pebble.Iterator, cu *cursor.Cursor, tenant, shard uint64, match *roaring.Bitmap, u *cursor.Update) {
	key := encoding.AppendFieldMapping(tenant, shard, 0, nil)
	prefix := key[:len(key)-8]
	for iter.SeekGE(key); iter.Valid(); iter.SeekGE(key) {
		ik := iter.Key()
		if !bytes.HasPrefix(ik, prefix) {
			return
		}

		// update latest key field
		copy(key[len(key)-8:], ik[len(ik)-8:])
		eaualityField := binary.BigEndian.Uint64(ik[len(ik)-8:])
		removeEquality(cu, shard, eaualityField, match, u)
	}
}

func removeEquality(cu *cursor.Cursor, shard, field uint64, match *roaring.Bitmap, u *cursor.Update) {
	if !cu.ResetData(shard, field) {
		return
	}
	cu.ClearRecords(match, u)
	if !cu.ResetExistence(shard, field) {
		return
	}
	cu.ClearRecords(match, u)
}
