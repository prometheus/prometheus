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

package semconv_test

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/semconv"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestTransformPolicyHTTPForwardAndReverse(t *testing.T) {
	wrapped, underlying := newAwareStorage(t)

	// v1.20.0 series: old metric name, old attribute keys, protocol version "2.0", 200 OK.
	appendSeries(t, underlying, "http.server.duration", 1, 10.0,
		"http.method", "GET",
		"http.status_code", "200",
		"http.scheme", "https",
		"net.protocol.version", "2.0",
		"net.host.name", "api.example.com",
	)
	// v1.21.0 intermediate series: old metric name, already using stable attribute keys.
	appendSeries(t, underlying, "http.server.duration", 2, 20.0,
		"http.request.method", "GET",
		"http.response.status_code", "200",
		"url.scheme", "https",
		"network.protocol.version", "2",
		"server.address", "api.example.com",
	)
	// v1.23.1 stable series: target metric name and stable attribute keys.
	appendSeries(t, underlying, "http.server.request.duration", 3, 30.0,
		"http.request.method", "GET",
		"http.response.status_code", "200",
		"url.scheme", "https",
		"network.protocol.version", "2",
		"server.address", "api.example.com",
	)
	// v1.20.0 series with a 503 status code and a non-standard HTTP method.
	appendSeries(t, underlying, "http.server.duration", 1, 50.0,
		"http.method", "PURGE_CACHE",
		"http.status_code", "503",
		"net.protocol.version", "3.0",
	)
	// Same-name metric with 1.20.0 attributes.
	appendSeries(t, underlying, "http.server.active_requests", 1, 4.0,
		"http.method", "POST",
		"http.scheme", "http",
		"net.host.name", "api.example.com",
	)

	t.Run("forward 1.20.0 to 1.23.1 merges eras and maps values without __semconv_url__", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.request.duration"),
			labels.MustNewMatcher(labels.MatchEqual, "http.request.method", "GET"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/http_1.20.0_to_1.23.1.yaml"),
		}

		for _, mode := range []string{"samples", "chunks"} {
			t.Run(mode, func(t *testing.T) {
				var set storage.SeriesSet
				if mode == "samples" {
					q, err := wrapped.Querier(0, 10)
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, q.Close()) })
					set = q.Select(t.Context(), true, nil, matchers...)
				} else {
					cq, err := wrapped.ChunkQuerier(0, 10)
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, cq.Close()) })
					set = storage.NewSeriesSetFromChunkSeriesSet(cq.Select(t.Context(), true, nil, matchers...))
				}

				counts := collectSeriesSampleCounts(t, set)
				require.Empty(t, set.Warnings())
				wantLabels := `{__name__="http.server.request.duration", "http.request.method"="GET", "http.response.status_code"="200", "network.protocol.version"="2", "server.address"="api.example.com", "url.scheme"="https"}`
				require.Equal(t, map[string]int{wantLabels: 3}, counts)
			})
		}
	})

	t.Run("forward 1.20.0 to 1.23.1 normalizes unknown method to _OTHER and derives error.type on 5xx", func(t *testing.T) {
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.request.duration"),
			labels.MustNewMatcher(labels.MatchEqual, "http.request.method", "_OTHER"),
			labels.MustNewMatcher(labels.MatchEqual, "error.type", "503"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/http_1.20.0_to_1.23.1.yaml"),
		)
		got := collectSeries(t, set)
		wantLabels := `{__name__="http.server.request.duration", "error.type"="503", "http.request.method"="_OTHER", "http.response.status_code"="503", "network.protocol.version"="3"}`
		require.Equal(t, map[string]float64{wantLabels: 50.0}, got)
	})

	t.Run("forward 1.20.0 to 1.23.1 transforms attributes in-place on same-name metric", func(t *testing.T) {
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.active_requests"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/http_1.20.0_to_1.23.1.yaml"),
		)
		got := collectSeries(t, set)
		wantLabels := `{__name__="http.server.active_requests", "http.request.method"="POST", "server.address"="api.example.com", "url.scheme"="http"}`
		require.Equal(t, map[string]float64{wantLabels: 4.0}, got)
	})

	t.Run("LabelNames and LabelValues reflect renamed and mapped attributes", func(t *testing.T) {
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.request.duration"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/http_1.20.0_to_1.23.1.yaml"),
		}
		names, anns, err := q.LabelNames(t.Context(), nil, matchers...)
		require.NoError(t, err)
		require.Empty(t, anns)
		require.Contains(t, names, "http.request.method")
		require.Contains(t, names, "network.protocol.version")
		require.NotContains(t, names, "http.method")
		require.NotContains(t, names, "net.protocol.version")

		protoVals, anns, err := q.LabelValues(t.Context(), "network.protocol.version", nil, matchers...)
		require.NoError(t, err)
		require.Empty(t, anns)
		require.Equal(t, []string{"2", "3"}, protoVals)
	})

	t.Run("reverse 1.23.1 to 1.20.0 translates stable series back to legacy shape", func(t *testing.T) {
		revWrapped, revUnderlying := newAwareStorage(t)
		appendSeries(t, revUnderlying, "http.server.request.duration", 1, 15.0,
			"http.request.method", "GET",
			"http.response.status_code", "200",
			"url.scheme", "https",
			"network.protocol.version", "2",
			"server.address", "api.example.com",
		)

		q, err := revWrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.duration"),
			labels.MustNewMatcher(labels.MatchEqual, "http.method", "GET"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/http_1.23.1_to_1.20.0.yaml"),
		)
		got := collectSeries(t, set)
		wantLabels := `{__name__="http.server.duration", "http.method"="GET", "http.scheme"="https", "http.status_code"="200", "net.host.name"="api.example.com", "net.protocol.name"="http", "net.protocol.version"="2.0"}`
		require.Equal(t, map[string]float64{wantLabels: 15.0}, got)
	})
}

func TestTransformPolicyK8sSplitsAndEntityTransforms(t *testing.T) {
	wrapped, underlying := newAwareStorage(t)

	// ResourceQuota series under the old Collector metric name with different `resource` values.
	appendSeries(t, underlying, "k8s.resource_quota.hard_limit", 1, 1024.0,
		"k8s.resource_quota.name", "compute-quota",
		"resource", "limits.memory",
	)
	appendSeries(t, underlying, "k8s.resource_quota.hard_limit", 1, 4.0,
		"k8s.resource_quota.name", "compute-quota",
		"resource", "requests.cpu",
	)
	appendSeries(t, underlying, "k8s.resource_quota.hard_limit", 1, 500.0,
		"k8s.resource_quota.name", "storage-quota",
		"resource", "fast.storageclass.storage.k8s.io/requests.storage",
	)
	appendSeries(t, underlying, "k8s.resource_quota.hard_limit", 1, 8.0,
		"k8s.resource_quota.name", "hugepage-quota",
		"resource", "requests.hugepages-2Mi",
	)
	// Pod network metric with legacy attributes + entity pod label prefix.
	appendSeries(t, underlying, "k8s.pod.network.io", 1, 4096.0,
		"k8s.pod.name", "frontend-abc",
		"k8s.pod.labels.app", "frontend",
		"k8s.pod.annotations.team", "observability",
		"interface", "eth0",
		"direction", "receive",
	)
	// Pod phase metric with value-encoded phase (2.0 = Running).
	appendSeries(t, underlying, "k8s.pod.phase", 1, 2.0,
		"k8s.pod.name", "frontend-abc",
	)

	t.Run("metric split by equals predicate removes pinned attribute and applies entity_transform", func(t *testing.T) {
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "k8s.resourcequota.memory.limit.hard"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/k8s_1.18.0_to_1.44.0.yaml"),
		)
		got := collectSeries(t, set)
		wantLabels := `{__name__="k8s.resourcequota.memory.limit.hard", "k8s.resourcequota.name"="compute-quota"}`
		require.Equal(t, map[string]float64{wantLabels: 1024.0}, got)
	})

	t.Run("split and discard extracts storageclass name from resource attribute", func(t *testing.T) {
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "k8s.resourcequota.storage.request.hard"),
			labels.MustNewMatcher(labels.MatchEqual, "k8s.storageclass.name", "fast"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/k8s_1.18.0_to_1.44.0.yaml"),
		)
		got := collectSeries(t, set)
		wantLabels := `{__name__="k8s.resourcequota.storage.request.hard", "k8s.resourcequota.name"="storage-quota", "k8s.storageclass.name"="fast"}`
		require.Equal(t, map[string]float64{wantLabels: 500.0}, got)
	})

	t.Run("split and discard extracts hugepage size from resource attribute", func(t *testing.T) {
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "k8s.resourcequota.hugepage_count.request.hard"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/k8s_1.18.0_to_1.44.0.yaml"),
		)
		got := collectSeries(t, set)
		wantLabels := `{__name__="k8s.resourcequota.hugepage_count.request.hard", "k8s.hugepage.size"="2Mi", "k8s.resourcequota.name"="hugepage-quota"}`
		require.Equal(t, map[string]float64{wantLabels: 8.0}, got)
	})

	t.Run("entity_transform rename_prefix rewrites pod labels and annotations", func(t *testing.T) {
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "k8s.pod.network.io"),
			labels.MustNewMatcher(labels.MatchEqual, "k8s.pod.label.app", "frontend"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/k8s_1.18.0_to_1.44.0.yaml"),
		)
		got := collectSeries(t, set)
		wantLabels := `{__name__="k8s.pod.network.io", "k8s.pod.annotation.team"="observability", "k8s.pod.label.app"="frontend", "k8s.pod.name"="frontend-abc", "network.interface.name"="eth0", "network.io.direction"="receive"}`
		require.Equal(t, map[string]float64{wantLabels: 4096.0}, got)
	})

	t.Run("translates DATAPOINT_FIELD_VALUE state metrics to attribute and sample value 1", func(t *testing.T) {
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "k8s.pod.status.phase"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/k8s_1.18.0_to_1.44.0.yaml"),
		)
		got := collectSeries(t, set)
		require.Empty(t, warningStrings(set.Warnings()))
		wantLabels := `{__name__="k8s.pod.status.phase", "k8s.pod.name"="frontend-abc", "k8s.pod.status.phase"="Running"}`
		require.Equal(t, map[string]float64{wantLabels: 1.0}, got)
	})
}

func TestTransformPolicyGRPCAndRPCAndGenAI(t *testing.T) {
	t.Run("gRPC A66 to OTel RPC 1.45.0 adds rpc.system.name and derives error.type", func(t *testing.T) {
		wrapped, underlying := newAwareStorage(t)
		appendSeries(t, underlying, "grpc.client.call.duration", 1, 0.042,
			"grpc.method", "other",
			"grpc.status", "UNAVAILABLE",
			"grpc.target", "dns:///payments.svc:443",
		)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "rpc.client.call.duration"),
			labels.MustNewMatcher(labels.MatchEqual, "rpc.system.name", "grpc"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/grpc_to_1.45.0.yaml"),
		)
		got := collectSeries(t, set)
		wantLabels := `{__name__="rpc.client.call.duration", "error.type"="UNAVAILABLE", "rpc.method"="_OTHER", "rpc.status_code"="UNAVAILABLE", "rpc.system.name"="grpc", "server.address"="dns:///payments.svc:443"}`
		require.Equal(t, map[string]float64{wantLabels: 0.042}, got)
	})

	t.Run("RPC 1.37.0 to 1.45.0 joins service and method, maps int status codes, and discards service", func(t *testing.T) {
		wrapped, underlying := newAwareStorage(t)
		// Error call (status 14 = UNAVAILABLE): should get error.type="UNAVAILABLE".
		appendSeries(t, underlying, "rpc.server.duration", 1, 12.0,
			"rpc.system", "apache_dubbo",
			"rpc.service", "checkout.CheckoutService",
			"rpc.method", "PlaceOrder",
			"rpc.grpc.status_code", "14",
		)
		// Successful call (status 0 = OK): unmapped: remove drops error.type.
		appendSeries(t, underlying, "rpc.server.duration", 1, 5.0,
			"rpc.system", "connect_rpc",
			"rpc.service", "checkout.CheckoutService",
			"rpc.method", "GetCart",
			"rpc.grpc.status_code", "0",
		)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "rpc.server.call.duration"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/rpc_1.37.0_to_1.45.0.yaml"),
		)
		got := collectSeries(t, set)
		wantErrSeries := `{__name__="rpc.server.call.duration", "error.type"="UNAVAILABLE", "rpc.method"="checkout.CheckoutService/PlaceOrder", "rpc.status_code"="UNAVAILABLE", "rpc.system.name"="dubbo"}`
		wantOKSeries := `{__name__="rpc.server.call.duration", "rpc.method"="checkout.CheckoutService/GetCart", "rpc.status_code"="OK", "rpc.system.name"="connectrpc"}`
		require.Equal(t, map[string]float64{
			wantErrSeries: 12.0,
			wantOKSeries:  5.0,
		}, got)
	})

	t.Run("GenAI 1.41.0 to 1.42.0-dev splits input and output token usage", func(t *testing.T) {
		wrapped, underlying := newAwareStorage(t)
		appendSeries(t, underlying, "gen_ai.client.token.usage", 1, 1200.0,
			"gen_ai.operation.name", "chat",
			"gen_ai.token.type", "input",
			"gen_ai.request.model", "gemini-2.5-pro",
		)
		appendSeries(t, underlying, "gen_ai.client.token.usage", 1, 350.0,
			"gen_ai.operation.name", "chat",
			"gen_ai.token.type", "output",
			"gen_ai.request.model", "gemini-2.5-pro",
		)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "gen_ai.client.inference.operation.input_tokens"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/genai_1.41.0_to_1.42.0.yaml"),
		)
		got := collectSeries(t, set)
		wantLabels := `{__name__="gen_ai.client.inference.operation.input_tokens", "gen_ai.operation.name"="chat", "gen_ai.request.model"="gemini-2.5-pro"}`
		require.Equal(t, map[string]float64{wantLabels: 1200.0}, got)
	})

	t.Run("metric_filter ACTION_DROP drops matching series", func(t *testing.T) {
		customRegistry := map[string][]byte{
			"filter_policy.yaml": []byte(`file_format: transform/2.0
source:
  schema_url: https://example.com/schemas/1.0.0
target:
  schema_url: https://example.com/schemas/2.0.0
policies:
  - id: drop-debug-env
    metric_filter:
      matches:
        - target: {descriptor_field: METRIC_DESCRIPTOR_FIELD_NAME}
          predicate: {equals: app.requests}
        - target: {datapoint_attribute: [env]}
          predicate: {equals: debug}
      action: ACTION_DROP
`),
		}
		underlying := teststorage.New(t)
		wrapped, err := semconv.AwareStorageWithRegistry(underlying, customRegistry)
		require.NoError(t, err)

		appendSeries(t, underlying, "app.requests", 1, 10.0, "env", "prod")
		appendSeries(t, underlying, "app.requests", 1, 99.0, "env", "debug")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "app.requests"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/filter_policy.yaml"),
		)
		got := collectSeries(t, set)
		require.Equal(t, map[string]float64{`{__name__="app.requests", env="prod"}`: 10.0}, got)
	})

	t.Run("target schema_url discovery fans out across both gRPC A66 and RPC 1.37.0 to 1.45.0", func(t *testing.T) {
		wrapped, underlying := newAwareStorage(t)
		// Era 1: gRPC A66 native metric.
		appendSeries(t, underlying, "grpc.client.call.duration", 1, 0.015,
			"grpc.method", "checkout.CheckoutService/PlaceOrder",
			"grpc.status", "OK",
			"grpc.target", "dns:///checkout.svc:443",
		)
		// Era 2: OTel RPC 1.37.0 metric.
		appendSeries(t, underlying, "rpc.client.duration", 2, 0.025,
			"rpc.system", "grpc",
			"rpc.service", "checkout.CheckoutService",
			"rpc.method", "PlaceOrder",
			"rpc.grpc.status_code", "0",
			"server.address", "dns:///checkout.svc:443",
		)
		// Era 3: OTel RPC 1.45.0 stable metric.
		appendSeries(t, underlying, "rpc.client.call.duration", 3, 0.035,
			"rpc.system.name", "grpc",
			"rpc.method", "checkout.CheckoutService/PlaceOrder",
			"rpc.status_code", "OK",
			"server.address", "dns:///checkout.svc:443",
		)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		for _, schemaTarget := range []string{"registry/1.45.0", "https://opentelemetry.io/schemas/1.45.0"} {
			set := q.Select(t.Context(), true, nil,
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "rpc.client.call.duration"),
				labels.MustNewMatcher(labels.MatchEqual, "rpc.method", "checkout.CheckoutService/PlaceOrder"),
				labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", schemaTarget),
			)
			var seriesCount, sampleCount int
			for set.Next() {
				seriesCount++
				it := set.At().Iterator(nil)
				for it.Next() != 0 {
					sampleCount++
				}
			}
			require.NoError(t, set.Err())
			require.Equal(t, 1, seriesCount, "expected all 3 eras to merge into 1 series for %s", schemaTarget)
			require.Equal(t, 3, sampleCount, "expected 3 chained samples across all 3 eras for %s", schemaTarget)
		}
	})

	t.Run("reverse K8s 1.44.0 to 1.18.0 translates attribute phase back to DATAPOINT_FIELD_VALUE", func(t *testing.T) {
		wrapped, underlying := newAwareStorage(t)
		appendSeries(t, underlying, "k8s.pod.status.phase", 1, 0.0,
			"k8s.pod.name", "web-0",
			"k8s.pod.status.phase", "Pending",
		)
		appendSeries(t, underlying, "k8s.pod.status.phase", 1, 1.0,
			"k8s.pod.name", "web-0",
			"k8s.pod.status.phase", "Running",
		)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "k8s.pod.phase"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/k8s_1.44.0_to_1.18.0.yaml"),
		)
		got := collectSeries(t, set)
		require.Equal(t, map[string]float64{`{__name__="k8s.pod.phase", "k8s.pod.name"="web-0"}`: 2.0}, got)
	})

	t.Run("Prometheus classic to OTel and reverse policies bridge metric, label, and histogram names", func(t *testing.T) {
		wrapped, underlying := newAwareStorage(t)
		appendSeries(t, underlying, "http_server_request_duration_seconds_bucket", 1, 10.0,
			"method", "GET",
			"status_code", "500",
			"scheme", "https",
			"protocol_version", "2.0",
			"le", "0.5",
		)
		appendSeries(t, underlying, "container_network_receive_bytes_total", 1, 2048.0,
			"pod", "api-0",
			"namespace", "default",
			"interface", "eth0",
		)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		// Query classic histogram _bucket using OTel canonical names.
		histSet := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.request.duration_bucket"),
			labels.MustNewMatcher(labels.MatchEqual, "http.request.method", "GET"),
			labels.MustNewMatcher(labels.MatchEqual, "le", "0.5"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/prometheus_to_otel.yaml"),
		)
		gotHist := collectSeries(t, histSet)
		wantHist := `{__name__="http.server.request.duration_bucket", "error.type"="500", "http.request.method"="GET", "http.response.status_code"="500", le="0.5", "network.protocol.version"="2", "url.scheme"="https"}`
		require.Equal(t, map[string]float64{wantHist: 10.0}, gotHist)

		// Query cAdvisor network counter using OTel k8s.pod.network.io.
		netSet := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "k8s.pod.network.io"),
			labels.MustNewMatcher(labels.MatchEqual, "network.io.direction", "receive"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/prometheus_to_otel.yaml"),
		)
		gotNet := collectSeries(t, netSet)
		wantNet := `{__name__="k8s.pod.network.io", "k8s.namespace.name"="default", "k8s.pod.name"="api-0", "network.interface.name"="eth0", "network.io.direction"="receive"}`
		require.Equal(t, map[string]float64{wantNet: 2048.0}, gotNet)
	})
}
