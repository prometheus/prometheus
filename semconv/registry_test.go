// Copyright The Prometheus Authors
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

package semconv

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestGroupDescInfo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		group Group
		want  prometheus.DescInfo
	}{
		{
			name: "counter with attributes",
			group: Group{
				MetricName: "prometheus_notifications_sent_total",
				Brief:      "Total number of alerts sent.",
				Instrument: "counter",
				Unit:       "s",
				Attributes: []Attribute{{Ref: "alertmanager"}},
			},
			want: prometheus.DescInfo{
				FQName:         "prometheus_notifications_sent_total",
				Help:           "Total number of alerts sent.",
				Unit:           "s",
				Type:           dto.MetricType_COUNTER,
				VariableLabels: []string{"alertmanager"},
			},
		},
		{
			name: "summary is declared as a histogram",
			group: Group{
				MetricName: "prometheus_notifications_latency_seconds",
				Instrument: "histogram",
				Annotations: Annotations{
					Prometheus: PrometheusAnnotations{HistogramType: "summary"},
				},
			},
			want: prometheus.DescInfo{
				FQName: "prometheus_notifications_latency_seconds",
				Type:   dto.MetricType_SUMMARY,
			},
		},
		{
			name: "attribute definitions are named by id, references by ref",
			group: Group{
				MetricName: "defined",
				Instrument: "gauge",
				Attributes: []Attribute{{ID: "shard"}, {Ref: "alertmanager"}},
			},
			want: prometheus.DescInfo{
				FQName:         "defined",
				Type:           dto.MetricType_GAUGE,
				VariableLabels: []string{"shard", "alertmanager"},
			},
		},
		{
			// The zero value of dto.MetricType is COUNTER, so a missing
			// instrument must not fall through to it.
			name:  "missing instrument is untyped",
			group: Group{MetricName: "no_instrument"},
			want:  prometheus.DescInfo{FQName: "no_instrument", Type: dto.MetricType_UNTYPED},
		},
		{
			name:  "unrecognised instrument is untyped",
			group: Group{MetricName: "bogus", Instrument: "not_a_thing"},
			want:  prometheus.DescInfo{FQName: "bogus", Type: dto.MetricType_UNTYPED},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.RequireEqual(t, tc.want, tc.group.DescInfo())
		})
	}
}

func TestLoad(t *testing.T) {
	r, err := Load([]byte(`
groups:
  - id: metric.some_total
    type: metric
    stability: development
    brief: A metric.
    metric_name: some_total
    instrument: counter
    unit: "{thing}"
    attributes:
      - ref: label
    annotations:
      prometheus:
        only_opts: true
`))
	require.NoError(t, err)
	require.Len(t, r.Groups, 1)

	g := r.Groups[0]
	require.Equal(t, "metric.some_total", g.ID)
	require.Equal(t, "development", g.Stability)
	require.True(t, g.Annotations.Prometheus.OnlyOpts)

	testutil.RequireEqual(t, map[string]prometheus.DescInfo{
		"some_total": {
			FQName:         "some_total",
			Help:           "A metric.",
			Unit:           "{thing}",
			Type:           dto.MetricType_COUNTER,
			VariableLabels: []string{"label"},
		},
	}, r.DescInfos())
}

func TestLoadRejectsInputWithoutMetrics(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "unrelated yaml", input: "hello: world\n"},
		{
			name:  "only attribute groups, no metrics",
			input: "groups:\n  - id: attr.only\n    type: attribute_group\n",
		},
		{
			name:  "otel telemetry schema, not a registry",
			input: "file_format: 1.1.0\nversions:\n  1.0.0: {}\n",
		},
		{
			name:  "metric group without a metric name",
			input: "groups:\n  - id: metric.nameless\n    type: metric\n    brief: No name.\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load([]byte(tc.input))
			require.Error(t, err)
		})
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := LoadFile("testdata/does-not-exist.yaml")
	require.Error(t, err)
}

func TestRegistryDiff(t *testing.T) {
	r := &Registry{Groups: []Group{
		{Type: TypeMetric, MetricName: "kept_total", Brief: "Kept.", Instrument: "counter"},
		{Type: TypeMetric, MetricName: "changed_total", Brief: "Changed.", Instrument: "counter"},
	}}
	unchanged := prometheus.DescInfo{
		FQName: "kept_total", Help: "Kept.", Type: dto.MetricType_COUNTER,
	}

	t.Run("no difference", func(t *testing.T) {
		require.Empty(t, r.Diff(map[string]prometheus.DescInfo{
			"kept_total": unchanged,
			"changed_total": {
				FQName: "changed_total", Help: "Changed.", Type: dto.MetricType_COUNTER,
			},
		}))
	})

	t.Run("reports only what differs", func(t *testing.T) {
		diff := r.Diff(map[string]prometheus.DescInfo{
			"kept_total": unchanged,
			"changed_total": {
				FQName: "changed_total", Help: "Changed.", Type: dto.MetricType_GAUGE,
			},
		})
		require.Contains(t, diff, "changed_total")
		require.Contains(t, diff, "GAUGE")
		require.NotContains(t, diff, "kept_total")
	})

	t.Run("metric missing from the code", func(t *testing.T) {
		diff := r.Diff(map[string]prometheus.DescInfo{"kept_total": unchanged})
		require.Contains(t, diff, "changed_total")
		require.NotContains(t, diff, "kept_total")
	})

	t.Run("metric absent from the registry", func(t *testing.T) {
		declared := map[string]prometheus.DescInfo{
			"kept_total":    unchanged,
			"changed_total": {FQName: "changed_total", Help: "Changed.", Type: dto.MetricType_COUNTER},
			"undeclared_total": {
				FQName: "undeclared_total", Type: dto.MetricType_COUNTER,
			},
		}
		diff := r.Diff(declared)
		require.Contains(t, diff, "undeclared_total")
		require.NotContains(t, diff, "kept_total")
	})

	t.Run("label order does not matter", func(t *testing.T) {
		withLabels := &Registry{Groups: []Group{{
			Type: TypeMetric, MetricName: "labelled", Instrument: "gauge",
			Attributes: []Attribute{{Ref: "b"}, {Ref: "a"}},
		}}}
		require.Empty(t, withLabels.Diff(map[string]prometheus.DescInfo{
			"labelled": {
				FQName: "labelled", Type: dto.MetricType_GAUGE,
				VariableLabels: []string{"a", "b"},
			},
		}))
	})
}
