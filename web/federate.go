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
	"sort"

	"github.com/golang/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/metric"
)

var (
	federationErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_web_federation_errors_total",
		Help: "Total number of errors that occurred while sending federation responses.",
	})
)

func (h *Handler) federation(w http.ResponseWriter, req *http.Request) {
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	req.ParseForm()

	var matcherSets []metric.LabelMatchers
	for _, s := range req.Form["match[]"] {
		matchers, err := promql.ParseMetricSelector(s)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		matcherSets = append(matcherSets, matchers)
	}

	var (
		minTimestamp = h.now().Add(-promql.StalenessDelta)
		format       = expfmt.Negotiate(req.Header)
		enc          = expfmt.NewEncoder(w, format)
	)
	w.Header().Set("Content-Type", string(format))

	q, err := h.storage.Querier()
	if err != nil {
		federationErrors.Inc()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer q.Close()

	vector, err := q.LastSampleForLabelMatchers(h.context, minTimestamp, matcherSets...)
	if err != nil {
		federationErrors.Inc()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sort.Sort(byName(vector))

	externalLabels := h.externalLabels.Clone()
	if _, ok := externalLabels[model.InstanceLabel]; !ok {
		externalLabels[model.InstanceLabel] = ""
	}
	externalLabelNames := make(model.LabelNames, 0, len(externalLabels))
	for ln := range externalLabels {
		externalLabelNames = append(externalLabelNames, ln)
	}
	sort.Sort(externalLabelNames)

	var (
		lastMetricName model.LabelValue
		protMetricFam  *dto.MetricFamily
	)
	for _, s := range vector {
		nameSeen := false
		globalUsed := map[model.LabelName]struct{}{}
		protMetric := &dto.Metric{
			Untyped: &dto.Untyped{},
		}

		// Sort labelnames for unittest consistency.
		labelNames := make(model.LabelNames, 0, len(s.Metric))
		for ln := range s.Metric {
			labelNames = append(labelNames, ln)
		}
		sort.Sort(labelNames)

		for _, ln := range labelNames {
			lv := s.Metric[ln]
			if lv == "" {
				// No value means unset. Never consider those labels.
				// This is also important to protect against nameless metrics.
				continue
			}
			if ln == model.MetricNameLabel {
				nameSeen = true
				if lv == lastMetricName {
					// We already have the name in the current MetricFamily,
					// and we ignore nameless metrics.
					continue
				}
				// Need to start a new MetricFamily. Ship off the old one (if any) before
				// creating the new one.
				if protMetricFam != nil {
					if err := enc.Encode(protMetricFam); err != nil {
						federationErrors.Inc()
						log.With("err", err).Error("federation failed")
						return
					}
				}
				protMetricFam = &dto.MetricFamily{
					Type: dto.MetricType_UNTYPED.Enum(),
					Name: proto.String(string(lv)),
				}
				lastMetricName = lv
				continue
			}
			protMetric.Label = append(protMetric.Label, &dto.LabelPair{
				Name:  proto.String(string(ln)),
				Value: proto.String(string(lv)),
			})
			if _, ok := externalLabels[ln]; ok {
				globalUsed[ln] = struct{}{}
			}
		}
		if !nameSeen {
			log.With("metric", s.Metric).Warn("Ignoring nameless metric during federation.")
			continue
		}
		// Attach global labels if they do not exist yet.
		for _, ln := range externalLabelNames {
			lv := externalLabels[ln]
			if _, ok := globalUsed[ln]; !ok {
				protMetric.Label = append(protMetric.Label, &dto.LabelPair{
					Name:  proto.String(string(ln)),
					Value: proto.String(string(lv)),
				})
			}
		}

		protMetric.TimestampMs = proto.Int64(int64(s.Timestamp))
		protMetric.Untyped.Value = proto.Float64(float64(s.Value))

		protMetricFam.Metric = append(protMetricFam.Metric, protMetric)
	}
	// Still have to ship off the last MetricFamily, if any.
	if protMetricFam != nil {
		if err := enc.Encode(protMetricFam); err != nil {
			federationErrors.Inc()
			log.With("err", err).Error("federation failed")
		}
	}
}

// byName makes a model.Vector sortable by metric name.
type byName model.Vector

func (vec byName) Len() int      { return len(vec) }
func (vec byName) Swap(i, j int) { vec[i], vec[j] = vec[j], vec[i] }

func (vec byName) Less(i, j int) bool {
	ni := vec[i].Metric[model.MetricNameLabel]
	nj := vec[j].Metric[model.MetricNameLabel]
	return ni < nj
}
