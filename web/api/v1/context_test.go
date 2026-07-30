// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"math"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/seriesmetadata"
)

// fakeResourceQuerier is a storage.Querier that answers GetResourceAt from a
// canned per-hash version list, mimicking VersionAt's forward-fill: the latest
// version whose MinTime <= ts wins.
type fakeResourceQuerier struct {
	storage.Querier
	byHash map[uint64][]*seriesmetadata.ResourceVersion
}

func (q *fakeResourceQuerier) GetResourceAt(hash uint64, ts int64) (*seriesmetadata.ResourceVersion, bool) {
	var chosen *seriesmetadata.ResourceVersion
	for _, v := range q.byHash[hash] {
		if ts >= v.MinTime {
			chosen = v
		}
	}
	if chosen == nil {
		return nil, false
	}
	return chosen, true
}

func (q *fakeResourceQuerier) IterUniqueAttributeNames(func(string)) error { return nil }

type fakeResourceQueryable struct{ q storage.Querier }

func (f fakeResourceQueryable) Querier(_, _ int64) (storage.Querier, error) { return f.q, nil }
func (f fakeResourceQueryable) ChunkQuerier(_, _ int64) (storage.ChunkQuerier, error) {
	return nil, nil
}

func newContextTestAPI(byHash map[uint64][]*seriesmetadata.ResourceVersion) *API {
	q := &fakeResourceQuerier{Querier: storage.NoopQuerier(), byHash: byHash}
	return &API{Queryable: fakeResourceQueryable{q: q}, enableNativeMetadata: true}
}

func mustMarshalJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestParseContextSelector(t *testing.T) {
	for _, tc := range []struct {
		in      string
		nilSel  bool
		all     bool
		matches map[string]bool
	}{
		{in: "", nilSel: true},
		{in: "   ", nilSel: true},
		{in: "*", all: true, matches: map[string]bool{"resource.service.name": true, "semconv.url": true}},
		{in: "resource.*", matches: map[string]bool{"resource.service.name": true, "resource.k8s.pod.name": true, "semconv.url": false}},
		{in: "resource.k8s.*", matches: map[string]bool{"resource.k8s.pod.name": true, "resource.service.name": false}},
		{in: "resource.service.name", matches: map[string]bool{"resource.service.name": true, "resource.service.namespace": false}},
	} {
		t.Run(tc.in, func(t *testing.T) {
			sel, err := parseContextSelector(tc.in)
			require.NoError(t, err)
			if tc.nilSel {
				require.Nil(t, sel)
				return
			}
			require.NotNil(t, sel)
			require.Equal(t, tc.all, sel.all)
			for k, want := range tc.matches {
				require.Equalf(t, want, sel.matchKey(k), "matchKey(%q)", k)
			}
		})
	}
}

func TestEnrichWithContext_InstantVector(t *testing.T) {
	lblsGET := labels.FromStrings("__name__", "http_requests_total", "job", "api", "method", "GET")
	lblsPOST := labels.FromStrings("__name__", "http_requests_total", "job", "api", "method", "POST")
	lblsNoRes := labels.FromStrings("__name__", "orphan", "job", "api")

	rv := seriesmetadata.NewResourceVersion(
		map[string]string{"service.name": "payment", "service.namespace": "prod"},
		map[string]string{"k8s.pod.name": "pod-a", "cloud.region": "us-west-2"},
		0, math.MaxInt64,
	)
	api := newContextTestAPI(map[uint64][]*seriesmetadata.ResourceVersion{
		labels.StableHash(lblsGET):  {rv},
		labels.StableHash(lblsPOST): {rv},
	})

	const ts = int64(1500)
	v := promql.Vector{
		{Metric: lblsGET, T: ts, F: 42},
		{Metric: lblsPOST, T: ts, F: 17},
		{Metric: lblsNoRes, T: ts, F: 1},
	}
	sel, err := parseContextSelector("resource.*")
	require.NoError(t, err)

	enriched, table, err := api.enrichWithContext(v, sel, ts, ts)
	require.NoError(t, err)
	require.Len(t, table, 1, "the two series sharing a resource must dedup to one entry")

	qd := &QueryData{ResultType: enriched.Type(), Result: enriched, Contexts: table}
	require.JSONEq(t, `{
		"resultType":"vector",
		"result":[
			{"metric":{"__name__":"http_requests_total","job":"api","method":"GET"},"value":[1.5,"42"],"context":"r1"},
			{"metric":{"__name__":"http_requests_total","job":"api","method":"POST"},"value":[1.5,"17"],"context":"r1"},
			{"metric":{"__name__":"orphan","job":"api"},"value":[1.5,"1"]}
		],
		"contexts":{
			"r1":{"resource":{
				"identifying":{"service.name":"payment","service.namespace":"prod"},
				"descriptive":{"k8s.pod.name":"pod-a","cloud.region":"us-west-2"}
			}}
		}
	}`, mustMarshalJSON(t, qd))
}

func TestEnrichWithContext_InstantVectorNoResources(t *testing.T) {
	lbls := labels.FromStrings("__name__", "orphan", "job", "api")
	api := newContextTestAPI(map[uint64][]*seriesmetadata.ResourceVersion{})

	const ts = int64(1500)
	v := promql.Vector{{Metric: lbls, T: ts, F: 1}}
	sel, err := parseContextSelector("resource.*")
	require.NoError(t, err)

	enriched, table, err := api.enrichWithContext(v, sel, ts, ts)
	require.NoError(t, err)
	require.Nil(t, table)
	require.IsType(t, promql.Vector{}, enriched, "unenriched result must stay a plain vector")

	qd := &QueryData{ResultType: enriched.Type(), Result: enriched, Contexts: table}
	require.JSONEq(t, `{
		"resultType":"vector",
		"result":[{"metric":{"__name__":"orphan","job":"api"},"value":[1.5,"1"]}]
	}`, mustMarshalJSON(t, qd))
}

func TestEnrichWithContext_RangeMatrixVersionChange(t *testing.T) {
	lbls := labels.FromStrings("__name__", "http_requests_total", "job", "api")
	hash := labels.StableHash(lbls)

	podA := seriesmetadata.NewResourceVersion(
		map[string]string{"service.name": "payment"},
		map[string]string{"k8s.pod.name": "pod-a"},
		0, 199,
	)
	podB := seriesmetadata.NewResourceVersion(
		map[string]string{"service.name": "payment"},
		map[string]string{"k8s.pod.name": "pod-b"},
		200, math.MaxInt64,
	)
	api := newContextTestAPI(map[uint64][]*seriesmetadata.ResourceVersion{hash: {podA, podB}})

	m := promql.Matrix{{
		Metric: lbls,
		Floats: []promql.FPoint{{T: 50, F: 1}, {T: 150, F: 2}, {T: 250, F: 3}},
	}}
	sel, err := parseContextSelector("resource.*")
	require.NoError(t, err)

	enriched, table, err := api.enrichWithContext(m, sel, 0, 300)
	require.NoError(t, err)
	require.Len(t, table, 2)

	qd := &QueryData{ResultType: enriched.Type(), Result: enriched, Contexts: table}
	require.JSONEq(t, `{
		"resultType":"matrix",
		"result":[
			{"metric":{"__name__":"http_requests_total","job":"api"},
			 "values":[[0.05,"1"],[0.15,"2"],[0.25,"3"]],
			 "context":[{"i":0,"id":"r1"},{"i":2,"id":"r2"}]}
		],
		"contexts":{
			"r1":{"resource":{"identifying":{"service.name":"payment"},"descriptive":{"k8s.pod.name":"pod-a"}}},
			"r2":{"resource":{"identifying":{"service.name":"payment"},"descriptive":{"k8s.pod.name":"pod-b"}}}
		}
	}`, mustMarshalJSON(t, qd))
}

func TestEnrichWithContext_RangeMatrixStableCollapsesToBareID(t *testing.T) {
	lbls := labels.FromStrings("__name__", "up", "job", "api")
	hash := labels.StableHash(lbls)
	rv := seriesmetadata.NewResourceVersion(
		map[string]string{"service.name": "payment"},
		nil,
		0, math.MaxInt64,
	)
	api := newContextTestAPI(map[uint64][]*seriesmetadata.ResourceVersion{hash: {rv}})

	m := promql.Matrix{{
		Metric: lbls,
		Floats: []promql.FPoint{{T: 50, F: 1}, {T: 150, F: 1}},
	}}
	sel, err := parseContextSelector("resource.*")
	require.NoError(t, err)

	enriched, table, err := api.enrichWithContext(m, sel, 0, 200)
	require.NoError(t, err)

	qd := &QueryData{ResultType: enriched.Type(), Result: enriched, Contexts: table}
	require.JSONEq(t, `{
		"resultType":"matrix",
		"result":[
			{"metric":{"__name__":"up","job":"api"},
			 "values":[[0.05,"1"],[0.15,"1"]],
			 "context":"r1"}
		],
		"contexts":{"r1":{"resource":{"identifying":{"service.name":"payment"}}}}
	}`, mustMarshalJSON(t, qd))
}

func TestEnrichWithContext_Projection(t *testing.T) {
	lbls := labels.FromStrings("__name__", "up", "job", "api")
	hash := labels.StableHash(lbls)
	rv := seriesmetadata.NewResourceVersion(
		map[string]string{"service.name": "payment", "service.namespace": "prod"},
		map[string]string{"k8s.pod.name": "pod-a", "cloud.region": "us-west-2"},
		0, math.MaxInt64,
	)
	api := newContextTestAPI(map[uint64][]*seriesmetadata.ResourceVersion{hash: {rv}})

	const ts = int64(1500)
	v := promql.Vector{{Metric: lbls, T: ts, F: 1}}
	sel, err := parseContextSelector("resource.k8s.*")
	require.NoError(t, err)

	enriched, table, err := api.enrichWithContext(v, sel, ts, ts)
	require.NoError(t, err)
	require.Len(t, table, 1)

	qd := &QueryData{ResultType: enriched.Type(), Result: enriched, Contexts: table}
	require.JSONEq(t, `{
		"resultType":"vector",
		"result":[{"metric":{"__name__":"up","job":"api"},"value":[1.5,"1"],"context":"r1"}],
		"contexts":{"r1":{"resource":{"descriptive":{"k8s.pod.name":"pod-a"}}}}
	}`, mustMarshalJSON(t, qd))
}
