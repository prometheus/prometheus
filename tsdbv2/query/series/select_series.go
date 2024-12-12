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

package series

import (
	"context"

	"github.com/cockroachdb/pebble"
	"github.com/gernest/roaring"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdbv2/cursor"
	"github.com/prometheus/prometheus/tsdbv2/encoding"
	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
	"github.com/prometheus/prometheus/tsdbv2/fields"
	"github.com/prometheus/prometheus/tsdbv2/query/filters"
	ql "github.com/prometheus/prometheus/tsdbv2/query/labels"
	"github.com/prometheus/prometheus/tsdbv2/shards"
	"github.com/prometheus/prometheus/tsdbv2/tenants"
	"github.com/prometheus/prometheus/tsdbv2/work"
)

func Select(ctx context.Context,
	db *pebble.DB,
	source tenants.Sounce,
	_ bool, hints *storage.SelectHints, matchers ...*labels.Matcher,
) storage.SeriesSet {
	if hints == nil {
		return storage.EmptySeriesSet()
	}
	tenantID, matchers, ok := ql.ParseTenantID(db, source, matchers...)
	if !ok {
		return storage.EmptySeriesSet()
	}

	filter := filters.Compile(db, tenantID, matchers)
	if filter == nil {
		return storage.EmptySeriesSet()
	}

	shards, err := shards.ReadShardsForTenant(db, tenantID)
	if err != nil {
		return storage.EmptySeriesSet()
	}
	if len(shards) == 0 {
		return storage.EmptySeriesSet()
	}

	matchedSeriesData := newMatchSet()

	work.Work(ctx, shards, selectShard(db, hints.Start, hints.End, matchedSeriesData, tenantID, filter))

	return matchedSeriesData.Compile(db, tenantID)
}

func selectShard(db *pebble.DB, mint, maxt int64, result *matchSet, tenantID uint64, filter bitmaps.Filter) func(ctx context.Context, shard []uint64) {
	return func(ctx context.Context, shards []uint64) {
		if len(shards) == 0 {
			return
		}
		iter, err := db.NewIter(nil)
		if err != nil {
			return
		}
		defer iter.Close()

		cu := cursor.New(iter, tenantID)

		for i := range shards {
			shard := shards[i]
			match := filter.Apply(cu, shard)
			if !match.Any() {
				continue
			}
			match = cu.TimeRange(shard, match, mint, maxt,
				encoding.SeriesFloat,
				encoding.SeriesFloatHistogram,
				encoding.SeriesFloatHistogram)
			if !match.Any() {
				continue
			}
			data := readData(cu, shard, match)
			result.Put(data)
		}
	}
}

func readData(r *cursor.Cursor, shard uint64, match *roaring.Bitmap) (da *Data) {
	return &Data{
		Rows:      match,
		Kind:      r.ReadEquality(fields.Kind, shard, match),
		ID:        r.ReadBSI(fields.Series.Hash(), shard, match),
		Timestamp: r.ReadBSI(fields.Timestamp.Hash(), shard, match),
		Value:     r.ReadBSI(fields.Value.Hash(), shard, match),
	}
}
