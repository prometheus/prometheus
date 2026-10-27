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
	"slices"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/semconv"
	"github.com/prometheus/prometheus/util/teststorage"
)

func newPromQLEngine(t *testing.T) *promql.Engine {
	t.Helper()
	engine := promql.NewEngine(promql.EngineOpts{
		Logger:     promslog.NewNopLogger(),
		MaxSamples: 50000,
		Timeout:    time.Minute,
	})
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}

func evalInstant(t *testing.T, engine *promql.Engine, q storage.Queryable, expr string, ts time.Time) promql.Vector {
	t.Helper()
	qry, err := engine.NewInstantQuery(t.Context(), q, nil, expr, ts)
	require.NoError(t, err)
	defer qry.Close()
	res := qry.Exec(t.Context())
	require.NoError(t, res.Err)
	vec, ok := res.Value.(promql.Vector)
	require.True(t, ok, "expected promql.Vector, got %T", res.Value)
	return vec
}

func TestRewritePromQLQuery_SideBySideWithStorageTranslation(t *testing.T) {
	underlying := teststorage.New(t)
	t.Cleanup(func() { _ = underlying.Close() })
	wrapped := semconv.AwareStorage(underlying)
	engine := newPromQLEngine(t)

	evalTime := time.UnixMilli(1000)

	// 1. HTTP 1.20.0 and 1.23.1 series.
	appendSeries(t, underlying, "http.server.duration", 1000, 42.0,
		"http.method", "POST",
		"http.status_code", "503",
		"net.protocol.version", "2.0",
		"http.scheme", "https",
		"instance", "api-1",
	)
	appendSeries(t, underlying, "http.server.request.duration", 1000, 18.0,
		"http.request.method", "POST",
		"http.response.status_code", "503",
		"network.protocol.version", "2",
		"url.scheme", "https",
		"error.type", "503",
		"instance", "api-2",
	)

	// 2. K8s 1.18.0 and 1.44.0 series (resource quota split + pod phase DATAPOINT_FIELD_VALUE).
	appendSeries(t, underlying, "k8s.resource_quota.hard_limit", 1000, 8.0,
		"k8s.resource_quota.name", "hp-quota",
		"resource", "requests.hugepages-2Mi",
	)
	appendSeries(t, underlying, "k8s.pod.phase", 1000, 2.0,
		"k8s.pod.name", "pod-a",
	)
	appendSeries(t, underlying, "k8s.pod.status.phase", 1000, 1.0,
		"k8s.pod.name", "pod-b",
		"k8s.pod.status.phase", "Running",
	)

	// 3. RPC 1.37.0 and gRPC A66 -> 1.45.0 series across two instances.
	appendSeries(t, underlying, "rpc.client.duration", 1000, 15.0,
		"rpc.system", "grpc",
		"rpc.service", "checkout.CheckoutService",
		"rpc.method", "PlaceOrder",
		"rpc.grpc.status_code", "0",
		"instance", "client-1",
	)
	appendSeries(t, underlying, "grpc.client.call.duration", 1000, 25.0,
		"grpc.method", "checkout.CheckoutService/PlaceOrder",
		"grpc.status", "OK",
		"instance", "client-2",
	)

	// 4. Classic Prometheus -> OTel series.
	appendSeries(t, underlying, "container_network_receive_bytes_total", 1000, 512.0,
		"pod", "web-0",
		"namespace", "prod",
		"interface", "eth0",
	)

	cases := []struct {
		name  string
		query string
	}{
		{
			name:  "HTTP 1.20.0 -> 1.23.1 aggregation with projected attributes",
			query: `sum by ("http.request.method", "network.protocol.version", "error.type") ({"http.server.request.duration", __schema_url__="registry/http_1.20.0_to_1.23.1.yaml", "http.request.method"="POST", "network.protocol.version"="2", "error.type"="503"})`,
		},
		{
			name:  "K8s 1.18.0 -> 1.44.0 hugepage split and regex extraction",
			query: `sum by ("k8s.hugepage.size", "k8s.resourcequota.name") ({"k8s.resourcequota.hugepage_count.request.hard", __schema_url__="registry/k8s_1.18.0_to_1.44.0.yaml", "k8s.hugepage.size"="2Mi"})`,
		},
		{
			name:  "K8s 1.18.0 -> 1.44.0 DATAPOINT_FIELD_VALUE pod phase",
			query: `sum by ("k8s.pod.name", "k8s.pod.status.phase") ({"k8s.pod.status.phase", __schema_url__="registry/k8s_1.18.0_to_1.44.0.yaml", "k8s.pod.status.phase"="Running"})`,
		},
		{
			name:  "Multi-policy target schema_url 1.45.0 combining gRPC A66 and RPC 1.37.0 via label_join",
			query: `sum by ("rpc.system.name", "rpc.method", "rpc.status_code") ({"rpc.client.call.duration", __schema_url__="registry/1.45.0", "rpc.method"="checkout.CheckoutService/PlaceOrder"})`,
		},
		{
			name:  "Prometheus classic cAdvisor to OTel k8s.pod.network.io",
			query: `sum by ("k8s.pod.name", "network.io.direction") ({"k8s.pod.network.io", __schema_url__="registry/prometheus_to_otel.yaml", "network.io.direction"="receive"})`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rewritten, annos, err := semconv.RewritePromQLQuery(tc.query, "")
			require.NoError(t, err)
			require.Empty(t, annos)
			require.NotContains(t, rewritten, "__schema_url__")

			// Evaluate the rewritten query on raw underlying storage (no AwareStorage wrapper).
			gotRewritten := evalInstant(t, engine, underlying, rewritten, evalTime)
			// Evaluate the original query on AwareStorage.
			gotStorage := evalInstant(t, engine, wrapped, tc.query, evalTime)

			sortVec := func(v promql.Vector) {
				slices.SortFunc(v, func(a, b promql.Sample) int {
					return labels.Compare(a.Metric, b.Metric)
				})
			}
			sortVec(gotRewritten)
			sortVec(gotStorage)

			require.NotEmpty(t, gotStorage)
			require.Equal(t, len(gotStorage), len(gotRewritten), "rewritten query: %s", rewritten)
			for i := range gotStorage {
				require.Equal(t, gotStorage[i].Metric, gotRewritten[i].Metric, "rewritten query: %s", rewritten)
				require.InDelta(t, gotStorage[i].F, gotRewritten[i].F, 1e-9, "rewritten query: %s", rewritten)
			}
		})
	}
}

func TestCounterRateAcrossSchemaBoundary_StorageVsASTRewriter(t *testing.T) {
	// Demonstrate why query-time storage chaining (AwareStorage) is exact on counter
	// rate() across a schema upgrade boundary whereas authoring-time AST `or` union
	// evaluates rate() on each metric name separately.
	underlying := teststorage.New(t)
	t.Cleanup(func() { _ = underlying.Close() })
	wrapped := semconv.AwareStorage(underlying)
	engine := newPromQLEngine(t)

	// Counter increments by +10 every 60s across a schema upgrade at t=120s:
	// t=0s, 60s under cAdvisor classic name; t=120s, 180s under OTel 1.44.0 name.
	appendSeries(t, underlying, "container_network_receive_bytes_total", 0, 100.0,
		"pod", "web-0", "namespace", "prod", "interface", "eth0",
	)
	appendSeries(t, underlying, "container_network_receive_bytes_total", 60_000, 110.0,
		"pod", "web-0", "namespace", "prod", "interface", "eth0",
	)
	appendSeries(t, underlying, "k8s.pod.network.io", 120_000, 120.0,
		"k8s.pod.name", "web-0", "k8s.namespace.name", "prod", "network.interface.name", "eth0", "network.io.direction", "receive",
	)
	appendSeries(t, underlying, "k8s.pod.network.io", 180_000, 130.0,
		"k8s.pod.name", "web-0", "k8s.namespace.name", "prod", "network.interface.name", "eth0", "network.io.direction", "receive",
	)

	query := `rate({"k8s.pod.network.io", __schema_url__="registry/prometheus_to_otel.yaml", "network.io.direction"="receive"}[3m])`
	evalTime := time.UnixMilli(180_000)

	// Storage-level translation chains all 4 samples into a single counter series before
	// PromQL computes rate([3m]), seeing Δ=30 over 180s = 0.166667/s.
	storageVec := evalInstant(t, engine, wrapped, query, evalTime)
	require.Len(t, storageVec, 1)
	require.InDelta(t, 30.0/180.0, storageVec[0].F, 1e-6)

	// Authoring-time AST rewrite wraps rate([3m]) per branch and unions with `or`.
	// Because the counter transition from t=60s (110) to t=120s (120) is split across two
	// metric names, the AST branch only observes 2 samples spanning 60s of the 180s window
	// and under-reports the rate by half (15.0/180.0 = 0.083333/s).
	rewritten, _, err := semconv.RewritePromQLQuery(query, "")
	require.NoError(t, err)
	astVec := evalInstant(t, engine, underlying, rewritten, evalTime)
	require.Len(t, astVec, 1)
	require.InDelta(t, 15.0/180.0, astVec[0].F, 1e-6)
}
