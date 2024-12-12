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
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdbv2/cache"
	"github.com/prometheus/prometheus/tsdbv2/encoding"
)

func (ba *Batch) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	tenantID, l, err := ba.GetTenantID(l)
	if err != nil {
		return 0, err
	}

	value, err := ba.GetMetadataID(tenantID, &m)
	if err != nil {
		return 0, err
	}
	id, err := ba.nextSeq(tenantID, 0)
	if err != nil {
		return 0, err
	}
	kind := encoding.SeriesMetadata
	if ref != 0 {
		err := ba.tr.GetRef(tenantID, ref, func(val []byte) error {
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
		ba.WriteValue(tenantID, id, value)
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
	ba.WriteValue(tenantID, id, value)
	return ba.cache.series.Ref(), nil
}
