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

package discovery

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestK8sClientNativeHistograms(t *testing.T) {
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterK8sClientMetricsWithPrometheus(reg))
	t.Cleanup(func() {
		clientGoRequestLatencyMetricVec.Reset()
		clientGoWorkqueueLatencyMetricVec.Reset()
		clientGoWorkqueueWorkDurationMetricVec.Reset()
	})

	for _, tc := range []struct {
		name       string
		observe    func(float64)
		labelName  string
		labelValue string
	}{
		{
			name: "prometheus_sd_kubernetes_http_request_duration_seconds",
			observe: func(value float64) {
				clientGoRequestMetrics.Observe(context.Background(), "GET", url.URL{Path: "/api/v1/pods"}, time.Duration(value*float64(time.Second)))
			},
			labelName:  "endpoint",
			labelValue: "/api/v1/pods",
		},
		{
			name:       "prometheus_sd_kubernetes_workqueue_latency_seconds",
			observe:    clientGoWorkloadMetrics.NewLatencyMetric("native-histogram-test").Observe,
			labelName:  "queue_name",
			labelValue: "native-histogram-test",
		},
		{
			name:       "prometheus_sd_kubernetes_workqueue_work_duration_seconds",
			observe:    clientGoWorkloadMetrics.NewWorkDurationMetric("native-histogram-test").Observe,
			labelName:  "queue_name",
			labelValue: "native-histogram-test",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, value := range []float64{0.01, 0.25, 1.5} {
				tc.observe(value)
			}
			families, err := reg.Gather()
			require.NoError(t, err)
			var family *dto.MetricFamily
			for _, f := range families {
				if f.GetName() == tc.name {
					family = f
					break
				}
			}
			require.NotNil(t, family)
			require.Equal(t, dto.MetricType_HISTOGRAM, family.GetType())
			require.Len(t, family.Metric, 1)
			metric := family.Metric[0]
			require.Len(t, metric.Label, 1)
			require.Equal(t, tc.labelName, metric.Label[0].GetName())
			require.Equal(t, tc.labelValue, metric.Label[0].GetValue())
			h := metric.GetHistogram()
			require.NotNil(t, h)
			require.NotNil(t, h.Schema)
			require.Equal(t, uint64(3), h.GetSampleCount())
			require.InDelta(t, 1.76, h.GetSampleSum(), 1e-10)
			require.NotEmpty(t, h.PositiveSpan)
		})
	}
}
