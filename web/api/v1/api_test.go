// Copyright 2016 The Prometheus Authors
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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/route"
	tsdbLabels "github.com/prometheus/tsdb/labels"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/gate"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/util/testutil"
)

type testTargetRetriever struct{}

func (t testTargetRetriever) TargetsActive() map[string][]*scrape.Target {
	return map[string][]*scrape.Target{
		"test": {
			scrape.NewTarget(
				labels.FromMap(map[string]string{
					model.SchemeLabel:      "http",
					model.AddressLabel:     "example.com:8080",
					model.MetricsPathLabel: "/metrics",
					model.JobLabel:         "test",
				}),
				nil,
				url.Values{},
			),
		},
		"blackbox": {
			scrape.NewTarget(
				labels.FromMap(map[string]string{
					model.SchemeLabel:      "http",
					model.AddressLabel:     "localhost:9115",
					model.MetricsPathLabel: "/probe",
					model.JobLabel:         "blackbox",
				}),
				nil,
				url.Values{"target": []string{"example.com"}},
			),
		},
	}
}
func (t testTargetRetriever) TargetsDropped() map[string][]*scrape.Target {
	return map[string][]*scrape.Target{
		"blackbox": {
			scrape.NewTarget(
				nil,
				labels.FromMap(map[string]string{
					model.AddressLabel:     "http://dropped.example.com:9115",
					model.MetricsPathLabel: "/probe",
					model.SchemeLabel:      "http",
					model.JobLabel:         "blackbox",
				}),
				url.Values{},
			),
		},
	}
}

type testAlertmanagerRetriever struct{}

func (t testAlertmanagerRetriever) Alertmanagers() []*url.URL {
	return []*url.URL{
		{
			Scheme: "http",
			Host:   "alertmanager.example.com:8080",
			Path:   "/api/v1/alerts",
		},
	}
}

func (t testAlertmanagerRetriever) DroppedAlertmanagers() []*url.URL {
	return []*url.URL{
		{
			Scheme: "http",
			Host:   "dropped.alertmanager.example.com:8080",
			Path:   "/api/v1/alerts",
		},
	}
}

type rulesRetrieverMock struct {
	testing *testing.T
}

func (m rulesRetrieverMock) AlertingRules() []*rules.AlertingRule {
	expr1, err := promql.ParseExpr(`absent(test_metric3) != 1`)
	if err != nil {
		m.testing.Fatalf("unable to parse alert expression: %s", err)
	}
	expr2, err := promql.ParseExpr(`up == 1`)
	if err != nil {
		m.testing.Fatalf("Unable to parse alert expression: %s", err)
	}

	rule1 := rules.NewAlertingRule(
		"test_metric3",
		expr1,
		time.Second,
		labels.Labels{},
		labels.Labels{},
		true,
		log.NewNopLogger(),
	)
	rule2 := rules.NewAlertingRule(
		"test_metric4",
		expr2,
		time.Second,
		labels.Labels{},
		labels.Labels{},
		true,
		log.NewNopLogger(),
	)
	var r []*rules.AlertingRule
	r = append(r, rule1)
	r = append(r, rule2)
	return r
}

func (m rulesRetrieverMock) RuleGroups() []*rules.Group {
	var ar rulesRetrieverMock
	arules := ar.AlertingRules()
	storage := testutil.NewStorage(m.testing)
	defer storage.Close()

	engineOpts := promql.EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxConcurrent: 10,
		MaxSamples:    10,
		Timeout:       100 * time.Second,
	}

	engine := promql.NewEngine(engineOpts)
	opts := &rules.ManagerOptions{
		QueryFunc:  rules.EngineQueryFunc(engine, storage),
		Appendable: storage,
		Context:    context.Background(),
		Logger:     log.NewNopLogger(),
	}

	var r []rules.Rule

	for _, alertrule := range arules {
		r = append(r, alertrule)
	}

	recordingExpr, err := promql.ParseExpr(`vector(1)`)
	if err != nil {
		m.testing.Fatalf("unable to parse alert expression: %s", err)
	}
	recordingRule := rules.NewRecordingRule("recording-rule-1", recordingExpr, labels.Labels{})
	r = append(r, recordingRule)

	group := rules.NewGroup("grp", "/path/to/file", time.Second, r, false, opts)
	return []*rules.Group{group}
}

var samplePrometheusCfg = config.Config{
	GlobalConfig:       config.GlobalConfig{},
	AlertingConfig:     config.AlertingConfig{},
	RuleFiles:          []string{},
	ScrapeConfigs:      []*config.ScrapeConfig{},
	RemoteWriteConfigs: []*config.RemoteWriteConfig{},
	RemoteReadConfigs:  []*config.RemoteReadConfig{},
}

var sampleFlagMap = map[string]string{
	"flag1": "value1",
	"flag2": "value2",
}

func TestEndpoints(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			test_metric1{foo="bar"} 0+100x100
			test_metric1{foo="boo"} 1+0x100
			test_metric2{foo="boo"} 1+0x100
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer suite.Close()

	if err := suite.Run(); err != nil {
		t.Fatal(err)
	}

	now := time.Now()

	var algr rulesRetrieverMock
	algr.testing = t
	algr.AlertingRules()
	algr.RuleGroups()

	t.Run("local", func(t *testing.T) {
		var algr rulesRetrieverMock
		algr.testing = t

		algr.AlertingRules()

		algr.RuleGroups()

		api := &API{
			Queryable:             suite.Storage(),
			QueryEngine:           suite.QueryEngine(),
			targetRetriever:       testTargetRetriever{},
			alertmanagerRetriever: testAlertmanagerRetriever{},
			flagsMap:              sampleFlagMap,
			now:                   func() time.Time { return now },
			config:                func() config.Config { return samplePrometheusCfg },
			ready:                 func(f http.HandlerFunc) http.HandlerFunc { return f },
			rulesRetriever:        algr,
		}

		testEndpoints(t, api, true)
	})

	// Run all the API tests against a API that is wired to forward queries via
	// the remote read client to a test server, which in turn sends them to the
	// data from the test suite.
	t.Run("remote", func(t *testing.T) {
		server := setupRemote(suite.Storage())
		defer server.Close()

		u, err := url.Parse(server.URL)
		if err != nil {
			t.Fatal(err)
		}

		al := promlog.AllowedLevel{}
		al.Set("debug")
		af := promlog.AllowedFormat{}
		al.Set("logfmt")
		promlogConfig := promlog.Config{
			Level:  &al,
			Format: &af,
		}

		dbDir, err := ioutil.TempDir("", "tsdb-api-ready")
		testutil.Ok(t, err)
		defer os.RemoveAll(dbDir)

		testutil.Ok(t, err)
		remote := remote.NewStorage(promlog.New(&promlogConfig), prometheus.DefaultRegisterer, func() (int64, error) {
			return 0, nil
		}, dbDir, 1*time.Second)

		err = remote.ApplyConfig(&config.Config{
			RemoteReadConfigs: []*config.RemoteReadConfig{
				{
					URL:           &config_util.URL{URL: u},
					RemoteTimeout: model.Duration(1 * time.Second),
					ReadRecent:    true,
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		var algr rulesRetrieverMock
		algr.testing = t

		algr.AlertingRules()

		algr.RuleGroups()

		api := &API{
			Queryable:             remote,
			QueryEngine:           suite.QueryEngine(),
			targetRetriever:       testTargetRetriever{},
			alertmanagerRetriever: testAlertmanagerRetriever{},
			flagsMap:              sampleFlagMap,
			now:                   func() time.Time { return now },
			config:                func() config.Config { return samplePrometheusCfg },
			ready:                 func(f http.HandlerFunc) http.HandlerFunc { return f },
			rulesRetriever:        algr,
		}

		testEndpoints(t, api, false)
	})

}

func TestLabelNames(t *testing.T) {
	// TestEndpoints doesn't have enough label names to test api.labelNames
	// endpoint properly. Hence we test it separately.
	suite, err := promql.NewTest(t, `
		load 1m
			test_metric1{foo1="bar", baz="abc"} 0+100x100
			test_metric1{foo2="boo"} 1+0x100
			test_metric2{foo="boo"} 1+0x100
			test_metric2{foo="boo", xyz="qwerty"} 1+0x100
	`)
	testutil.Ok(t, err)
	defer suite.Close()
	testutil.Ok(t, suite.Run())

	api := &API{
		Queryable: suite.Storage(),
	}
	request := func(m string) (*http.Request, error) {
		if m == http.MethodPost {
			r, err := http.NewRequest(m, "http://example.com", nil)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return r, err
		}
		return http.NewRequest(m, "http://example.com", nil)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		ctx := context.Background()
		req, err := request(method)
		testutil.Ok(t, err)
		res := api.labelNames(req.WithContext(ctx))
		assertAPIError(t, res.err, "")
		assertAPIResponse(t, res.data, []string{"__name__", "baz", "foo", "foo1", "foo2", "xyz"})
	}
}

func setupRemote(s storage.Storage) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := remote.DecodeReadRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := prompb.ReadResponse{
			Results: make([]*prompb.QueryResult, len(req.Queries)),
		}
		for i, query := range req.Queries {
			from, through, matchers, selectParams, err := remote.FromQuery(query)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			querier, err := s.Querier(r.Context(), from, through)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer querier.Close()

			set, _, err := querier.Select(selectParams, matchers...)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			resp.Results[i], err = remote.ToQueryResult(set, 1e6)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if err := remote.EncodeReadResponse(&resp, w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	return httptest.NewServer(handler)
}

func testEndpoints(t *testing.T, api *API, testLabelAPI bool) {
	start := time.Unix(0, 0)

	type test struct {
		endpoint apiFunc
		params   map[string]string
		query    url.Values
		response interface{}
		errType  errorType
	}

	var tests = []test{
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"2"},
				"time":  []string{"123.4"},
			},
			response: &queryData{
				ResultType: promql.ValueTypeScalar,
				Result: promql.Scalar{
					V: 2,
					T: timestamp.FromTime(start.Add(123*time.Second + 400*time.Millisecond)),
				},
			},
		},
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"0.333"},
				"time":  []string{"1970-01-01T00:02:03Z"},
			},
			response: &queryData{
				ResultType: promql.ValueTypeScalar,
				Result: promql.Scalar{
					V: 0.333,
					T: timestamp.FromTime(start.Add(123 * time.Second)),
				},
			},
		},
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"0.333"},
				"time":  []string{"1970-01-01T01:02:03+01:00"},
			},
			response: &queryData{
				ResultType: promql.ValueTypeScalar,
				Result: promql.Scalar{
					V: 0.333,
					T: timestamp.FromTime(start.Add(123 * time.Second)),
				},
			},
		},
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"0.333"},
			},
			response: &queryData{
				ResultType: promql.ValueTypeScalar,
				Result: promql.Scalar{
					V: 0.333,
					T: timestamp.FromTime(api.now()),
				},
			},
		},
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"start": []string{"0"},
				"end":   []string{"2"},
				"step":  []string{"1"},
			},
			response: &queryData{
				ResultType: promql.ValueTypeMatrix,
				Result: promql.Matrix{
					promql.Series{
						Points: []promql.Point{
							{V: 0, T: timestamp.FromTime(start)},
							{V: 1, T: timestamp.FromTime(start.Add(1 * time.Second))},
							{V: 2, T: timestamp.FromTime(start.Add(2 * time.Second))},
						},
						Metric: nil,
					},
				},
			},
		},
		// Missing query params in range queries.
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"end":   []string{"2"},
				"step":  []string{"1"},
			},
			errType: errorBadData,
		},
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"start": []string{"0"},
				"step":  []string{"1"},
			},
			errType: errorBadData,
		},
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"start": []string{"0"},
				"end":   []string{"2"},
			},
			errType: errorBadData,
		},
		// Bad query expression.
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"invalid][query"},
				"time":  []string{"1970-01-01T01:02:03+01:00"},
			},
			errType: errorBadData,
		},
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"invalid][query"},
				"start": []string{"0"},
				"end":   []string{"100"},
				"step":  []string{"1"},
			},
			errType: errorBadData,
		},
		// Invalid step.
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"start": []string{"1"},
				"end":   []string{"2"},
				"step":  []string{"0"},
			},
			errType: errorBadData,
		},
		// Start after end.
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"start": []string{"2"},
				"end":   []string{"1"},
				"step":  []string{"1"},
			},
			errType: errorBadData,
		},
		// Start overflows int64 internally.
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"start": []string{"148966367200.372"},
				"end":   []string{"1489667272.372"},
				"step":  []string{"1"},
			},
			errType: errorBadData,
		},
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
			},
			response: []labels.Labels{
				labels.FromStrings("__name__", "test_metric2", "foo", "boo"),
			},
		},
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric1{foo=~".+o"}`},
			},
			response: []labels.Labels{
				labels.FromStrings("__name__", "test_metric1", "foo", "boo"),
			},
		},
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric1{foo=~".+o$"}`, `test_metric1{foo=~".+o"}`},
			},
			response: []labels.Labels{
				labels.FromStrings("__name__", "test_metric1", "foo", "boo"),
			},
		},
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric1{foo=~".+o"}`, `none`},
			},
			response: []labels.Labels{
				labels.FromStrings("__name__", "test_metric1", "foo", "boo"),
			},
		},
		// Start and end before series starts.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
				"start":   []string{"-2"},
				"end":     []string{"-1"},
			},
			response: []labels.Labels{},
		},
		// Start and end after series ends.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
				"start":   []string{"100000"},
				"end":     []string{"100001"},
			},
			response: []labels.Labels{},
		},
		// Start before series starts, end after series ends.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
				"start":   []string{"-1"},
				"end":     []string{"100000"},
			},
			response: []labels.Labels{
				labels.FromStrings("__name__", "test_metric2", "foo", "boo"),
			},
		},
		// Start and end within series.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
				"start":   []string{"1"},
				"end":     []string{"100"},
			},
			response: []labels.Labels{
				labels.FromStrings("__name__", "test_metric2", "foo", "boo"),
			},
		},
		// Start within series, end after.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
				"start":   []string{"1"},
				"end":     []string{"100000"},
			},
			response: []labels.Labels{
				labels.FromStrings("__name__", "test_metric2", "foo", "boo"),
			},
		},
		// Start before series, end within series.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
				"start":   []string{"-1"},
				"end":     []string{"1"},
			},
			response: []labels.Labels{
				labels.FromStrings("__name__", "test_metric2", "foo", "boo"),
			},
		},
		// Missing match[] query params in series requests.
		{
			endpoint: api.series,
			errType:  errorBadData,
		},
		{
			endpoint: api.dropSeries,
			errType:  errorInternal,
		},
		{
			endpoint: api.targets,
			response: &TargetDiscovery{
				ActiveTargets: []*Target{
					{
						DiscoveredLabels: map[string]string{},
						Labels: map[string]string{
							"job": "blackbox",
						},
						ScrapeURL: "http://localhost:9115/probe?target=example.com",
						Health:    "unknown",
					},
					{
						DiscoveredLabels: map[string]string{},
						Labels: map[string]string{
							"job": "test",
						},
						ScrapeURL: "http://example.com:8080/metrics",
						Health:    "unknown",
					},
				},
				DroppedTargets: []*DroppedTarget{
					{
						DiscoveredLabels: map[string]string{
							"__address__":      "http://dropped.example.com:9115",
							"__metrics_path__": "/probe",
							"__scheme__":       "http",
							"job":              "blackbox",
						},
					},
				},
			},
		},
		{
			endpoint: api.alertmanagers,
			response: &AlertmanagerDiscovery{
				ActiveAlertmanagers: []*AlertmanagerTarget{
					{
						URL: "http://alertmanager.example.com:8080/api/v1/alerts",
					},
				},
				DroppedAlertmanagers: []*AlertmanagerTarget{
					{
						URL: "http://dropped.alertmanager.example.com:8080/api/v1/alerts",
					},
				},
			},
		},
		{
			endpoint: api.serveConfig,
			response: &prometheusConfig{
				YAML: samplePrometheusCfg.String(),
			},
		},
		{
			endpoint: api.serveFlags,
			response: sampleFlagMap,
		},
		{
			endpoint: api.alerts,
			response: &AlertDiscovery{
				Alerts: []*Alert{},
			},
		},
		{
			endpoint: api.rules,
			response: &RuleDiscovery{
				RuleGroups: []*RuleGroup{
					{
						Name:     "grp",
						File:     "/path/to/file",
						Interval: 1,
						Rules: []rule{
							alertingRule{
								Name:        "test_metric3",
								Query:       "absent(test_metric3) != 1",
								Duration:    1,
								Labels:      labels.Labels{},
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "unknown",
								Type:        "alerting",
							},
							alertingRule{
								Name:        "test_metric4",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.Labels{},
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "unknown",
								Type:        "alerting",
							},
							recordingRule{
								Name:   "recording-rule-1",
								Query:  "vector(1)",
								Labels: labels.Labels{},
								Health: "unknown",
								Type:   "recording",
							},
						},
					},
				},
			},
		},
	}

	if testLabelAPI {
		tests = append(tests, []test{
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "__name__",
				},
				response: []string{
					"test_metric1",
					"test_metric2",
				},
			},
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				response: []string{
					"bar",
					"boo",
				},
			},
			// Bad name parameter.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "not!!!allowed",
				},
				errType: errorBadData,
			},
			// Label names.
			{
				endpoint: api.labelNames,
				response: []string{"__name__", "foo"},
			},
		}...)
	}

	methods := func(f apiFunc) []string {
		fp := reflect.ValueOf(f).Pointer()
		if fp == reflect.ValueOf(api.query).Pointer() || fp == reflect.ValueOf(api.queryRange).Pointer() || fp == reflect.ValueOf(api.series).Pointer() {
			return []string{http.MethodGet, http.MethodPost}
		}
		return []string{http.MethodGet}
	}

	request := func(m string, q url.Values) (*http.Request, error) {
		if m == http.MethodPost {
			r, err := http.NewRequest(m, "http://example.com", strings.NewReader(q.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return r, err
		}
		return http.NewRequest(m, fmt.Sprintf("http://example.com?%s", q.Encode()), nil)
	}

	for i, test := range tests {
		for _, method := range methods(test.endpoint) {
			// Build a context with the correct request params.
			ctx := context.Background()
			for p, v := range test.params {
				ctx = route.WithParam(ctx, p, v)
			}
			t.Logf("run %d\t%s\t%q", i, method, test.query.Encode())

			req, err := request(method, test.query)
			if err != nil {
				t.Fatal(err)
			}
			res := test.endpoint(req.WithContext(ctx))
			assertAPIError(t, res.err, test.errType)
			assertAPIResponse(t, res.data, test.response)
		}
	}
}

func assertAPIError(t *testing.T, got *apiError, exp errorType) {
	t.Helper()

	if got != nil {
		if exp == errorNone {
			t.Fatalf("Unexpected error: %s", got)
		}
		if exp != got.typ {
			t.Fatalf("Expected error of type %q but got type %q (%q)", exp, got.typ, got)
		}
		return
	}
	if got == nil && exp != errorNone {
		t.Fatalf("Expected error of type %q but got none", exp)
	}
}

func assertAPIResponse(t *testing.T, got interface{}, exp interface{}) {
	if !reflect.DeepEqual(exp, got) {
		respJSON, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("failed to marshal response as JSON: %v", err.Error())
		}

		expectedRespJSON, err := json.Marshal(exp)
		if err != nil {
			t.Fatalf("failed to marshal expected response as JSON: %v", err.Error())
		}

		t.Fatalf(
			"Response does not match, expected:\n%+v\ngot:\n%+v",
			string(expectedRespJSON),
			string(respJSON),
		)
	}
}

func TestReadEndpoint(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			test_metric1{foo="bar",baz="qux"} 1
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer suite.Close()

	if err := suite.Run(); err != nil {
		t.Fatal(err)
	}

	api := &API{
		Queryable:   suite.Storage(),
		QueryEngine: suite.QueryEngine(),
		config: func() config.Config {
			return config.Config{
				GlobalConfig: config.GlobalConfig{
					ExternalLabels: labels.Labels{
						{Name: "baz", Value: "a"},
						{Name: "b", Value: "c"},
						{Name: "d", Value: "e"},
					},
				},
			}
		},
		remoteReadSampleLimit: 1e6,
		remoteReadGate:        gate.New(1),
	}

	// Encode the request.
	matcher1, err := labels.NewMatcher(labels.MatchEqual, "__name__", "test_metric1")
	if err != nil {
		t.Fatal(err)
	}
	matcher2, err := labels.NewMatcher(labels.MatchEqual, "d", "e")
	if err != nil {
		t.Fatal(err)
	}
	query, err := remote.ToQuery(0, 1, []*labels.Matcher{matcher1, matcher2}, &storage.SelectParams{Step: 0, Func: "avg"})
	if err != nil {
		t.Fatal(err)
	}
	req := &prompb.ReadRequest{Queries: []*prompb.Query{query}}
	data, err := proto.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	compressed := snappy.Encode(nil, data)
	request, err := http.NewRequest("POST", "", bytes.NewBuffer(compressed))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	api.remoteRead(recorder, request)

	if recorder.Code/100 != 2 {
		t.Fatal(recorder.Code)
	}

	// Decode the response.
	compressed, err = ioutil.ReadAll(recorder.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	uncompressed, err := snappy.Decode(nil, compressed)
	if err != nil {
		t.Fatal(err)
	}

	var resp prompb.ReadResponse
	err = proto.Unmarshal(uncompressed, &resp)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(resp.Results))
	}

	result := resp.Results[0]
	expected := &prompb.QueryResult{
		Timeseries: []*prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "test_metric1"},
					{Name: "b", Value: "c"},
					{Name: "baz", Value: "qux"},
					{Name: "d", Value: "e"},
					{Name: "foo", Value: "bar"},
				},
				Samples: []prompb.Sample{{Value: 1, Timestamp: 0}},
			},
		},
	}
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("Expected response \n%v\n but got \n%v\n", result, expected)
	}
}

type fakeDB struct {
	err    error
	closer func()
}

func (f *fakeDB) CleanTombstones() error                                  { return f.err }
func (f *fakeDB) Delete(mint, maxt int64, ms ...tsdbLabels.Matcher) error { return f.err }
func (f *fakeDB) Dir() string {
	dir, _ := ioutil.TempDir("", "fakeDB")
	f.closer = func() {
		os.RemoveAll(dir)
	}
	return dir
}
func (f *fakeDB) Snapshot(dir string, withHead bool) error { return f.err }

func TestAdminEndpoints(t *testing.T) {
	tsdb, tsdbWithError := &fakeDB{}, &fakeDB{err: errors.New("some error")}
	snapshotAPI := func(api *API) apiFunc { return api.snapshot }
	cleanAPI := func(api *API) apiFunc { return api.cleanTombstones }
	deleteAPI := func(api *API) apiFunc { return api.deleteSeries }

	for i, tc := range []struct {
		db          *fakeDB
		enableAdmin bool
		endpoint    func(api *API) apiFunc
		method      string
		values      url.Values

		errType errorType
	}{
		// Tests for the snapshot endpoint.
		{
			db:          tsdb,
			enableAdmin: false,
			endpoint:    snapshotAPI,

			errType: errorUnavailable,
		},
		{
			db:          tsdb,
			enableAdmin: true,
			endpoint:    snapshotAPI,

			errType: errorNone,
		},
		{
			db:          tsdb,
			enableAdmin: true,
			endpoint:    snapshotAPI,
			values:      map[string][]string{"skip_head": {"true"}},

			errType: errorNone,
		},
		{
			db:          tsdb,
			enableAdmin: true,
			endpoint:    snapshotAPI,
			values:      map[string][]string{"skip_head": {"xxx"}},

			errType: errorBadData,
		},
		{
			db:          tsdbWithError,
			enableAdmin: true,
			endpoint:    snapshotAPI,

			errType: errorInternal,
		},
		{
			db:          nil,
			enableAdmin: true,
			endpoint:    snapshotAPI,

			errType: errorUnavailable,
		},
		// Tests for the cleanTombstones endpoint.
		{
			db:          tsdb,
			enableAdmin: false,
			endpoint:    cleanAPI,

			errType: errorUnavailable,
		},
		{
			db:          tsdb,
			enableAdmin: true,
			endpoint:    cleanAPI,

			errType: errorNone,
		},
		{
			db:          tsdbWithError,
			enableAdmin: true,
			endpoint:    cleanAPI,

			errType: errorInternal,
		},
		{
			db:          nil,
			enableAdmin: true,
			endpoint:    cleanAPI,

			errType: errorUnavailable,
		},
		// Tests for the deleteSeries endpoint.
		{
			db:          tsdb,
			enableAdmin: false,
			endpoint:    deleteAPI,

			errType: errorUnavailable,
		},
		{
			db:          tsdb,
			enableAdmin: true,
			endpoint:    deleteAPI,

			errType: errorBadData,
		},
		{
			db:          tsdb,
			enableAdmin: true,
			endpoint:    deleteAPI,
			values:      map[string][]string{"match[]": {"123"}},

			errType: errorBadData,
		},
		{
			db:          tsdb,
			enableAdmin: true,
			endpoint:    deleteAPI,
			values:      map[string][]string{"match[]": {"up"}, "start": {"xxx"}},

			errType: errorBadData,
		},
		{
			db:          tsdb,
			enableAdmin: true,
			endpoint:    deleteAPI,
			values:      map[string][]string{"match[]": {"up"}, "end": {"xxx"}},

			errType: errorBadData,
		},
		{
			db:          tsdb,
			enableAdmin: true,
			endpoint:    deleteAPI,
			values:      map[string][]string{"match[]": {"up"}},

			errType: errorNone,
		},
		{
			db:          tsdb,
			enableAdmin: true,
			endpoint:    deleteAPI,
			values:      map[string][]string{"match[]": {"up{job!=\"foo\"}", "{job=~\"bar.+\"}", "up{instance!~\"fred.+\"}"}},

			errType: errorNone,
		},
		{
			db:          tsdbWithError,
			enableAdmin: true,
			endpoint:    deleteAPI,
			values:      map[string][]string{"match[]": {"up"}},

			errType: errorInternal,
		},
		{
			db:          nil,
			enableAdmin: true,
			endpoint:    deleteAPI,

			errType: errorUnavailable,
		},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			api := &API{
				db: func() TSDBAdmin {
					if tc.db != nil {
						return tc.db
					}
					return nil
				},
				ready:       func(f http.HandlerFunc) http.HandlerFunc { return f },
				enableAdmin: tc.enableAdmin,
			}
			defer func() {
				if tc.db != nil && tc.db.closer != nil {
					tc.db.closer()
				}
			}()

			endpoint := tc.endpoint(api)
			req, err := http.NewRequest(tc.method, fmt.Sprintf("?%s", tc.values.Encode()), nil)
			if err != nil {
				t.Fatalf("Error when creating test request: %s", err)
			}
			res := endpoint(req)
			assertAPIError(t, res.err, tc.errType)
		})
	}
}

func TestRespondSuccess(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api := API{}
		api.respond(w, "test", nil)
	}))
	defer s.Close()

	resp, err := http.Get(s.URL)
	if err != nil {
		t.Fatalf("Error on test request: %s", err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		t.Fatalf("Error reading response body: %s", err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("Return code %d expected in success response but got %d", 200, resp.StatusCode)
	}
	if h := resp.Header.Get("Content-Type"); h != "application/json" {
		t.Fatalf("Expected Content-Type %q but got %q", "application/json", h)
	}

	var res response
	if err = json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatalf("Error unmarshaling JSON body: %s", err)
	}

	exp := &response{
		Status: statusSuccess,
		Data:   "test",
	}
	if !reflect.DeepEqual(&res, exp) {
		t.Fatalf("Expected response \n%v\n but got \n%v\n", res, exp)
	}
}

func TestRespondError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api := API{}
		api.respondError(w, &apiError{errorTimeout, errors.New("message")}, "test")
	}))
	defer s.Close()

	resp, err := http.Get(s.URL)
	if err != nil {
		t.Fatalf("Error on test request: %s", err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		t.Fatalf("Error reading response body: %s", err)
	}

	if want, have := http.StatusServiceUnavailable, resp.StatusCode; want != have {
		t.Fatalf("Return code %d expected in error response but got %d", want, have)
	}
	if h := resp.Header.Get("Content-Type"); h != "application/json" {
		t.Fatalf("Expected Content-Type %q but got %q", "application/json", h)
	}

	var res response
	if err = json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatalf("Error unmarshaling JSON body: %s", err)
	}

	exp := &response{
		Status:    statusError,
		Data:      "test",
		ErrorType: errorTimeout,
		Error:     "message",
	}
	if !reflect.DeepEqual(&res, exp) {
		t.Fatalf("Expected response \n%v\n but got \n%v\n", res, exp)
	}
}

func TestParseTime(t *testing.T) {
	ts, err := time.Parse(time.RFC3339Nano, "2015-06-03T13:21:58.555Z")
	if err != nil {
		panic(err)
	}

	var tests = []struct {
		input  string
		fail   bool
		result time.Time
	}{
		{
			input: "",
			fail:  true,
		}, {
			input: "abc",
			fail:  true,
		}, {
			input: "30s",
			fail:  true,
		}, {
			input:  "123",
			result: time.Unix(123, 0),
		}, {
			input:  "123.123",
			result: time.Unix(123, 123000000),
		}, {
			input:  "2015-06-03T13:21:58.555Z",
			result: ts,
		}, {
			input:  "2015-06-03T14:21:58.555+01:00",
			result: ts,
		}, {
			// Test float rounding.
			input:  "1543578564.705",
			result: time.Unix(1543578564, 705*1e6),
		},
	}

	for _, test := range tests {
		ts, err := parseTime(test.input)
		if err != nil && !test.fail {
			t.Errorf("Unexpected error for %q: %s", test.input, err)
			continue
		}
		if err == nil && test.fail {
			t.Errorf("Expected error for %q but got none", test.input)
			continue
		}
		if !test.fail && !ts.Equal(test.result) {
			t.Errorf("Expected time %v for input %q but got %v", test.result, test.input, ts)
		}
	}
}

func TestParseDuration(t *testing.T) {
	var tests = []struct {
		input  string
		fail   bool
		result time.Duration
	}{
		{
			input: "",
			fail:  true,
		}, {
			input: "abc",
			fail:  true,
		}, {
			input: "2015-06-03T13:21:58.555Z",
			fail:  true,
		}, {
			// Internal int64 overflow.
			input: "-148966367200.372",
			fail:  true,
		}, {
			// Internal int64 overflow.
			input: "148966367200.372",
			fail:  true,
		}, {
			input:  "123",
			result: 123 * time.Second,
		}, {
			input:  "123.333",
			result: 123*time.Second + 333*time.Millisecond,
		}, {
			input:  "15s",
			result: 15 * time.Second,
		}, {
			input:  "5m",
			result: 5 * time.Minute,
		},
	}

	for _, test := range tests {
		d, err := parseDuration(test.input)
		if err != nil && !test.fail {
			t.Errorf("Unexpected error for %q: %s", test.input, err)
			continue
		}
		if err == nil && test.fail {
			t.Errorf("Expected error for %q but got none", test.input)
			continue
		}
		if !test.fail && d != test.result {
			t.Errorf("Expected duration %v for input %q but got %v", test.result, test.input, d)
		}
	}
}

func TestOptionsMethod(t *testing.T) {
	r := route.New()
	api := &API{ready: func(f http.HandlerFunc) http.HandlerFunc { return f }}
	api.Register(r)

	s := httptest.NewServer(r)
	defer s.Close()

	req, err := http.NewRequest("OPTIONS", s.URL+"/any_path", nil)
	if err != nil {
		t.Fatalf("Error creating OPTIONS request: %s", err)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Error executing OPTIONS request: %s", err)
	}

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Expected status %d, got %d", http.StatusNoContent, resp.StatusCode)
	}
}

func TestRespond(t *testing.T) {
	cases := []struct {
		response interface{}
		expected string
	}{
		{
			response: &queryData{
				ResultType: promql.ValueTypeMatrix,
				Result: promql.Matrix{
					promql.Series{
						Points: []promql.Point{{V: 1, T: 1000}},
						Metric: labels.FromStrings("__name__", "foo"),
					},
				},
			},
			expected: `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"foo"},"values":[[1,"1"]]}]}}`,
		},
		{
			response: promql.Point{V: 0, T: 0},
			expected: `{"status":"success","data":[0,"0"]}`,
		},
		{
			response: promql.Point{V: 20, T: 1},
			expected: `{"status":"success","data":[0.001,"20"]}`,
		},
		{
			response: promql.Point{V: 20, T: 10},
			expected: `{"status":"success","data":[0.010,"20"]}`,
		},
		{
			response: promql.Point{V: 20, T: 100},
			expected: `{"status":"success","data":[0.100,"20"]}`,
		},
		{
			response: promql.Point{V: 20, T: 1001},
			expected: `{"status":"success","data":[1.001,"20"]}`,
		},
		{
			response: promql.Point{V: 20, T: 1010},
			expected: `{"status":"success","data":[1.010,"20"]}`,
		},
		{
			response: promql.Point{V: 20, T: 1100},
			expected: `{"status":"success","data":[1.100,"20"]}`,
		},
		{
			response: promql.Point{V: 20, T: 12345678123456555},
			expected: `{"status":"success","data":[12345678123456.555,"20"]}`,
		},
		{
			response: promql.Point{V: 20, T: -1},
			expected: `{"status":"success","data":[-0.001,"20"]}`,
		},
		{
			response: promql.Point{V: math.NaN(), T: 0},
			expected: `{"status":"success","data":[0,"NaN"]}`,
		},
		{
			response: promql.Point{V: math.Inf(1), T: 0},
			expected: `{"status":"success","data":[0,"+Inf"]}`,
		},
		{
			response: promql.Point{V: math.Inf(-1), T: 0},
			expected: `{"status":"success","data":[0,"-Inf"]}`,
		},
		{
			response: promql.Point{V: 1.2345678e6, T: 0},
			expected: `{"status":"success","data":[0,"1234567.8"]}`,
		},
		{
			response: promql.Point{V: 1.2345678e-6, T: 0},
			expected: `{"status":"success","data":[0,"0.0000012345678"]}`,
		},
		{
			response: promql.Point{V: 1.2345678e-67, T: 0},
			expected: `{"status":"success","data":[0,"1.2345678e-67"]}`,
		},
	}

	for _, c := range cases {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			api := API{}
			api.respond(w, c.response, nil)
		}))
		defer s.Close()

		resp, err := http.Get(s.URL)
		if err != nil {
			t.Fatalf("Error on test request: %s", err)
		}
		body, err := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			t.Fatalf("Error reading response body: %s", err)
		}

		if string(body) != c.expected {
			t.Fatalf("Expected response \n%v\n but got \n%v\n", c.expected, string(body))
		}
	}
}

// This is a global to avoid the benchmark being optimized away.
var testResponseWriter = httptest.ResponseRecorder{}

func BenchmarkRespond(b *testing.B) {
	b.ReportAllocs()
	points := []promql.Point{}
	for i := 0; i < 10000; i++ {
		points = append(points, promql.Point{V: float64(i * 1000000), T: int64(i)})
	}
	response := &queryData{
		ResultType: promql.ValueTypeMatrix,
		Result: promql.Matrix{
			promql.Series{
				Points: points,
				Metric: nil,
			},
		},
	}
	b.ResetTimer()
	api := API{}
	for n := 0; n < b.N; n++ {
		api.respond(&testResponseWriter, response, nil)
	}
}
