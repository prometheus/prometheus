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
	"math"

	"github.com/gernest/roaring"

	"github.com/prometheus/prometheus/tsdbv2/cache"
	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
	"github.com/prometheus/prometheus/tsdbv2/fields"
)

func (ba *Batch) WriteSeriesID(tenant, id, value uint64) {
	ba.encodeBSI(tenant, fields.Series.Hash(), id, int64(value))
}

func (ba *Batch) WriteLabels(ca cache.Series, tenant, id uint64) {
	ca.Iter(func(field, value uint64) {
		ba.encodeEquality(tenant, field, id, value)
	})
}

func (ba *Batch) WriteCachedLabels(tenant, id uint64) {
	ba.WriteLabels(ba.cache.series, tenant, id)
}

func (ba *Batch) WriteKind(tenant, id, value uint64) {
	ba.encodeEquality(tenant, fields.Kind.Hash(), id, value)
}

func (ba *Batch) WriteTimestamp(tenant, id uint64, value int64) {
	ba.encodeBSI(tenant, fields.Timestamp.Hash(), id, value)
}

func (ba *Batch) WriteFloatValue(tenant, id uint64, value float64) {
	ba.encodeBSI(tenant, fields.Value.Hash(), id, int64(math.Float64bits(value)))
}

func (ba *Batch) WriteValue(tenant, id, value uint64) {
	ba.encodeBSI(tenant, fields.Value.Hash(), id, int64(value))
}

func (ba *Batch) encodeEquality(tenant, field, id, value uint64) {
	bitmaps.Equality(
		ba.get(batchKey{
			tenant: tenant,
			field:  field,
			shard:  ba.shard[tenant],
		}), id, value)
	bitmaps.SetExistenceBit(
		ba.get(batchKey{
			tenant:    tenant,
			field:     field,
			shard:     ba.shard[tenant],
			existence: true,
		}), id)
}

func (ba *Batch) encodeBSI(tenant, field, id uint64, svalue int64) {
	bitmaps.BitSliced(
		ba.get(batchKey{
			tenant: tenant,
			field:  field,
			shard:  ba.shard[tenant],
		}),
		id, svalue)
}

func (ba *Batch) get(key batchKey) *roaring.Bitmap {
	ra, ok := ba.fields[key]
	if !ok {
		ra = roaring.NewBitmap()
		ba.fields[key] = ra
	}
	return ra
}
