package web

import (
	"net/http"

	"github.com/golang/protobuf/proto"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

type Federation struct {
	Storage local.Storage
}

func (fed *Federation) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()

	metrics := map[model.Fingerprint]model.COWMetric{}

	for _, s := range req.Form["match[]"] {
		matchers, err := promql.ParseMetricSelector(s)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for fp, met := range fed.Storage.MetricsForLabelMatchers(matchers...) {
			metrics[fp] = met
		}
	}

	format := expfmt.Negotiate(req.Header)
	w.Header().Set("Content-Type", string(format))

	enc := expfmt.NewEncoder(w, format)

	protMetric := &dto.Metric{
		Label:   []*dto.LabelPair{},
		Untyped: &dto.Untyped{},
	}
	protMetricFam := &dto.MetricFamily{
		Metric: []*dto.Metric{protMetric},
		Type:   dto.MetricType_UNTYPED.Enum(),
	}

	for fp, met := range metrics {
		sp := fed.Storage.LastSamplePairForFingerprint(fp)
		if sp == nil {
			continue
		}

		// Reset label slice.
		protMetric.Label = protMetric.Label[:0]

		for ln, lv := range met.Metric {
			if ln == model.MetricNameLabel {
				protMetricFam.Name = proto.String(string(lv))
				continue
			}
			protMetric.Label = append(protMetric.Label, &dto.LabelPair{
				Name:  proto.String(string(ln)),
				Value: proto.String(string(lv)),
			})
		}
		protMetric.TimestampMs = (*int64)(&sp.Timestamp)
		protMetric.Untyped.Value = (*float64)(&sp.Value)

		if err := enc.Encode(protMetricFam); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return

		}
	}
}
