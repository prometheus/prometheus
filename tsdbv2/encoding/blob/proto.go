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

package blob

import (
	"slices"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/prompb"
)

func ExemplarToProto(ex *exemplar.Exemplar) ([]byte, error) {
	px := &prompb.Exemplar{
		Value:     ex.Value,
		Timestamp: ex.Ts,
		Labels:    prompb.FromLabels(ex.Labels, nil),
	}
	return px.Marshal()
}

func ProtoToExemplar(ex *prompb.Exemplar, o []exemplar.Exemplar) []exemplar.Exemplar {
	return append(o, exemplar.Exemplar{
		Value:  ex.Value,
		Ts:     ex.Timestamp,
		Labels: ProtoToLabels(ex.Labels, nil),
	})
}

func LabelsToProto(la labels.Labels) ([]byte, error) {
	e := &prompb.TimeSeries{
		Labels: prompb.FromLabels(la, make([]prompb.Label, len(la))),
	}
	return e.Marshal()
}

func ProtoToLabels(la []prompb.Label, e labels.Labels) labels.Labels {
	e = slices.Grow(e, len(la))
	for i := range la {
		e = append(e, labels.Label{
			Name:  la[i].Name,
			Value: la[i].Value,
		})
	}
	return e
}

func MetadataToProto(me *metadata.Metadata) ([]byte, error) {
	px := &prompb.MetricMetadata{
		Type: prompb.FromMetadataType(me.Type),
		Help: me.Help,
		Unit: me.Unit,
	}
	return px.Marshal()
}

func HistogramToProto(t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) ([]byte, error) {
	if h != nil {
		e := prompb.FromIntHistogram(t, h)
		return e.Marshal()
	}
	e := prompb.FromFloatHistogram(t, fh)
	return e.Marshal()
}
