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
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdbv2/cache"
	"github.com/prometheus/prometheus/tsdbv2/encoding"
	"github.com/prometheus/prometheus/tsdbv2/hash"
)

func (ba *Batch) AppendCTZeroSample(ref storage.SeriesRef, l labels.Labels, t, ct int64) (storage.SeriesRef, error) {
	return ba.append(ref, l, ct, 0)
}

func (ba *Batch) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	return ba.append(ref, l, t, v)
}

func (ba *Batch) append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	tenantID, l, err := ba.GetTenantID(l)
	if err != nil {
		return 0, err
	}

	id, err := ba.nextSeq(tenantID, 0)
	if err != nil {
		return 0, err
	}

	kind := encoding.SeriesFloat

	if ref != 0 {
		err = ba.tr.GetRef(tenantID, ref, func(val []byte) error {
			srs := cache.Decode(val)
			sid := srs.ID()
			ba.WriteLabels(srs, tenantID, id)
			ba.WriteSeriesID(tenantID, id, sid)
			return nil
		})
		if err != nil {
			return 0, err
		}

		ba.WriteKind(tenantID, id, uint64(kind))
		ba.WriteTimestamp(tenantID, id, t)
		ba.WriteFloatValue(tenantID, id, v)
		return ref, nil
	}

	seriesID, l, err := ba.GetSeriesID(tenantID, l)
	if err != nil {
		return 0, err
	}

	err = ba.processLabels(tenantID, id, l)
	if err != nil {
		return 0, err
	}

	ba.WriteCachedLabels(tenantID, id)
	ba.WriteSeriesID(tenantID, id, seriesID)
	ba.WriteKind(tenantID, id, uint64(kind))
	ba.WriteFloatValue(tenantID, id, v)
	ba.WriteTimestamp(tenantID, id, t)
	return ba.cache.series.Ref(), nil
}

func (ba *Batch) processLabels(tenant, id uint64, l labels.Labels) (err error) {
	shard := ba.shard[tenant]
	ba.cache.fields = ba.cache.fields[:0]
	ba.cache.ids = ba.cache.ids[:0]
	var text []byte
	for i := range l {
		text = append(text[:0], []byte(l[i].Name)...)
		field := hash.Bytes(text)
		ba.baseFieldMapping.Set(tenant, shard, field)
		text = append(text, encoding.SepBytes...)
		text = append(text, []byte(l[i].Value)...)
		value, err := ba.tr.TranslateText(tenant, field, text)
		if err != nil {
			return err
		}
		ba.cache.fields = append(ba.cache.fields, field)
		ba.cache.ids = append(ba.cache.ids, value)
	}
	ba.cache.series = cache.New(id, ba.cache.fields, ba.cache.ids, ba.cache.series)
	return ba.tr.SetRef(tenant, ba.cache.series)
}
