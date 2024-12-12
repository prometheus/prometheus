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
	"fmt"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/tsdbv2/compat"
	"github.com/prometheus/prometheus/tsdbv2/encoding/blob"
	"github.com/prometheus/prometheus/tsdbv2/fields"
)

func (ba *Batch) GetMetadataID(tenantID uint64, e *metadata.Metadata) (uint64, error) {
	data, err := blob.MetadataToProto(e)
	if err != nil {
		return 0, err
	}
	return ba.translateBlob(tenantID, fields.Metadata.Hash(), data)
}

func (ba *Batch) GetHistogramID(tenantID uint64, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (uint64, error) {
	data, err := blob.HistogramToProto(t, h, fh)
	if err != nil {
		return 0, err
	}
	return ba.translateBlob(tenantID, fields.Histogram.Hash(), data)
}

func (ba *Batch) GetExemplarID(tenantID uint64, e *exemplar.Exemplar) (uint64, error) {
	data, err := blob.ExemplarToProto(e)
	if err != nil {
		return 0, err
	}
	return ba.translateBlob(tenantID, fields.Exemplar.Hash(), data)
}

func (ba *Batch) translateBlob(tenant, field uint64, data []byte) (uint64, error) {
	return ba.tr.TranslateBlob(tenant, field, data)
}

func (ba *Batch) GetSeriesID(tenantID uint64, la labels.Labels) (id uint64, lset labels.Labels, err error) {
	lset = la.WithoutEmpty()
	if lset.IsEmpty() {
		return 0, nil, fmt.Errorf("empty labelset: %w", compat.ErrInvalidSample)
	}
	if l, dup := la.HasDuplicateLabelNames(); dup {
		return 0, nil, fmt.Errorf(`label name "%s" is not unique: %w`, l, compat.ErrInvalidSample)
	}
	data, err := blob.LabelsToProto(la)
	if err != nil {
		return 0, nil, err
	}
	id, err = ba.translateBlob(tenantID, fields.Series.Hash(), data)
	if err != nil {
		return
	}
	return
}
