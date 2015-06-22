package web

import (
	"io"
	"net/http"

	"bitbucket.org/ww/goautoneg"
	"github.com/golang/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/text"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"

	clientmodel "github.com/prometheus/client_golang/model"
	dto "github.com/prometheus/client_model/go"
)

type Federation struct {
	Storage local.Storage
}

func (fed *Federation) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()

	metrics := map[clientmodel.Fingerprint]clientmodel.COWMetric{}

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

	enc, contentType := chooseEncoder(req)
	w.Header().Set("Content-Type", contentType)

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
			if ln == clientmodel.MetricNameLabel {
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

		if _, err := enc(w, protMetricFam); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

type encoder func(w io.Writer, p *dto.MetricFamily) (int, error)

func chooseEncoder(req *http.Request) (encoder, string) {
	accepts := goautoneg.ParseAccept(req.Header.Get("Accept"))
	for _, accept := range accepts {
		switch {
		case accept.Type == "application" &&
			accept.SubType == "vnd.google.protobuf" &&
			accept.Params["proto"] == "io.prometheus.client.MetricFamily":
			switch accept.Params["encoding"] {
			case "delimited":
				return text.WriteProtoDelimited, prometheus.DelimitedTelemetryContentType
			case "text":
				return text.WriteProtoText, prometheus.ProtoTextTelemetryContentType
			case "compact-text":
				return text.WriteProtoCompactText, prometheus.ProtoCompactTextTelemetryContentType
			default:
				continue
			}
		case accept.Type == "text" &&
			accept.SubType == "plain" &&
			(accept.Params["version"] == "0.0.4" || accept.Params["version"] == ""):
			return text.MetricFamilyToText, prometheus.TextTelemetryContentType
		default:
			continue
		}
	}
	return text.MetricFamilyToText, prometheus.TextTelemetryContentType
}
