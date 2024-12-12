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
	"sync"

	"github.com/cockroachdb/pebble"
	"github.com/gernest/roaring"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/tsdbv2/cursor"
	"github.com/prometheus/prometheus/tsdbv2/encoding"
	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
	"github.com/prometheus/prometheus/tsdbv2/encoding/blob"
	"github.com/prometheus/prometheus/tsdbv2/fields"
	"github.com/prometheus/prometheus/tsdbv2/query/filters"
	ql "github.com/prometheus/prometheus/tsdbv2/query/labels"
	"github.com/prometheus/prometheus/tsdbv2/shards"
	"github.com/prometheus/prometheus/tsdbv2/tenants"
	"github.com/prometheus/prometheus/tsdbv2/work"
)

func Exemplar(db *pebble.DB, source tenants.Sounce, start, end int64, matchers ...[]*labels.Matcher) ([]exemplar.QueryResult, error) {
	tenantID, matchers, ok := ql.ParseTenantIDFromMany(db, source, matchers...)
	if !ok {
		return nil, nil
	}
	var filter bitmaps.Filter
	for i := range matchers {
		fs := filters.Compile(db, tenantID, matchers[i])
		if fs == nil {
			continue
		}
		filter = &bitmaps.Or{Left: filter, Right: fs}
	}
	if filter == nil {
		return nil, nil
	}

	shards, err := shards.ReadShardsForTenant(db, tenantID)
	if err != nil {
		return nil, nil
	}
	if len(shards) == 0 {
		return nil, nil
	}

	matchSet := newBlobSet()

	work.Work(context.Background(), shards,
		selectExemplarShard(db, start, end, matchSet, tenantID, filter))

	return matchSet.Compile(db, tenantID)
}

type blobsSet struct {
	series [][]uint64
	value  [][]uint64
	mu     sync.RWMutex
}

func newBlobSet() *blobsSet {
	return &blobsSet{}
}

func (b *blobsSet) Put(series, value []uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.series = append(b.series, series)
	b.value = append(b.value, value)
}

func (b *blobsSet) Compile(db *pebble.DB, tenant uint64) (result []exemplar.QueryResult, err error) {
	series := make(map[uint64]*roaring.Bitmap)

	for i := range b.series {
		sx := b.series[i]
		vx := b.value[i]

		for j := range sx {
			sid := sx[j]
			ra, ok := series[sid]
			if !ok {
				ra = roaring.NewBitmap()
				series[sid] = ra
			}
			_, _ = ra.Add(vx[j])
		}
	}

	var ts prompb.TimeSeries
	var exa prompb.Exemplar
	var buf []byte
	result = make([]exemplar.QueryResult, 0, len(b.series))
	for k, v := range series {
		buf = encoding.AppendBlobTranslationID(tenant, fields.Series.Hash(), k, buf[:0])
		ts.Reset()
		err = get(db, buf, ts.Unmarshal)
		if err != nil {
			return
		}
		ex := make([]exemplar.Exemplar, 0, v.Count())
		err = v.ForEach(func(u uint64) error {
			buf = encoding.AppendBlobTranslationID(tenant, fields.Exemplar.Hash(), u, buf[:0])
			exa.Reset()
			err = get(db, buf, exa.Unmarshal)
			if err != nil {
				return err
			}
			ex = blob.ProtoToExemplar(&exa, ex)
			return nil
		})
		if err != nil {
			return
		}
		result = append(result, exemplar.QueryResult{
			SeriesLabels: blob.ProtoToLabels(ts.Labels, nil),
			Exemplars:    ex,
		})
	}
	return
}

func get(db *pebble.DB, key []byte, f func([]byte) error) error {
	value, done, err := db.Get(key)
	if err != nil {
		return err
	}
	defer done.Close()
	return f(value)
}

func selectExemplarShard(db *pebble.DB, mint, maxt int64,
	result *blobsSet,
	tenantID uint64, filter bitmaps.Filter,
) func(ctx context.Context, shard []uint64) {
	return func(ctx context.Context, shards []uint64) {
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
			match = cu.TimeRange(shard, match, mint, maxt, encoding.SeriesExemplar)
			if !match.Any() {
				continue
			}
			series := cu.ReadBSI(fields.Series.Hash(), shard, match)
			blobs := cu.ReadBSI(fields.Value.Hash(), shard, match)
			result.Put(series, blobs)
		}
	}
}
