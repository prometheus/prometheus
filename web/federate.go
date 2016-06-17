// Copyright 2015 The Prometheus Authors
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

package web

import (
	"net/http"

	"github.com/golang/protobuf/proto"

	"github.com/prometheus/prometheus/promql"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	dto "github.com/prometheus/client_model/go"
)

func (h *Handler) federation(w http.ResponseWriter, req *http.Request) {
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	req.ParseForm()

	fps := map[model.Fingerprint]struct{}{}

	for _, s := range req.Form["match[]"] {
		matchers, err := promql.ParseMetricSelector(s)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for fp := range h.storage.MetricsForLabelMatchers(
			model.Now().Add(-promql.StalenessDelta), model.Latest,
			matchers...,
		) {
			fps[fp] = struct{}{}
		}
	}

	var (
		minTimestamp = model.Now().Add(-promql.StalenessDelta)
		format       = expfmt.Negotiate(req.Header)
		enc          = expfmt.NewEncoder(w, format)
	)
	w.Header().Set("Content-Type", string(format))

	protMetric := &dto.Metric{
		Label:   []*dto.LabelPair{},
		Untyped: &dto.Untyped{},
	}
	protMetricFam := &dto.MetricFamily{
		Metric: []*dto.Metric{protMetric},
		Type:   dto.MetricType_UNTYPED.Enum(),
	}

	for fp := range fps {
		globalUsed := map[model.LabelName]struct{}{}

		s := h.storage.LastSampleForFingerprint(fp)
		// Discard if sample does not exist or lays before the staleness interval.
		if s.Timestamp.Before(minTimestamp) {
			continue
		}

		// Reset label slice.
		protMetric.Label = protMetric.Label[:0]

		for ln, lv := range s.Metric {
			if ln == model.MetricNameLabel {
				protMetricFam.Name = proto.String(string(lv))
				continue
			}
			protMetric.Label = append(protMetric.Label, &dto.LabelPair{
				Name:  proto.String(string(ln)),
				Value: proto.String(string(lv)),
			})
			if _, ok := h.externalLabels[ln]; ok {
				globalUsed[ln] = struct{}{}
			}
		}

		// Attach global labels if they do not exist yet.
		for ln, lv := range h.externalLabels {
			if _, ok := globalUsed[ln]; !ok {
				protMetric.Label = append(protMetric.Label, &dto.LabelPair{
					Name:  proto.String(string(ln)),
					Value: proto.String(string(lv)),
				})
			}
		}

		protMetric.TimestampMs = proto.Int64(int64(s.Timestamp))
		protMetric.Untyped.Value = proto.Float64(float64(s.Value))

		if err := enc.Encode(protMetricFam); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
