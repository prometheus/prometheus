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
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"sort"
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

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/gate"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/prometheus/prometheus/util/testutil"
)

// testMetaStore satisfies the scrape.MetricMetadataStore interface.
// It is used to inject specific metadata as part of a test case.
type testMetaStore struct {
	Metadata []scrape.MetricMetadata
}

func (s *testMetaStore) ListMetadata() []scrape.MetricMetadata {
	return s.Metadata
}

func (s *testMetaStore) GetMetadata(metric string) (scrape.MetricMetadata, bool) {
	for _, m := range s.Metadata {
		if metric == m.Metric {
			return m, true
		}
	}

	return scrape.MetricMetadata{}, false
}

func (s *testMetaStore) SizeMetadata() int   { return 0 }
func (s *testMetaStore) LengthMetadata() int { return 0 }

// testTargetRetriever represents a list of targets to scrape.
// It is used to represent targets as part of test cases.
type testTargetRetriever struct {
	activeTargets  map[string][]*scrape.Target
	droppedTargets map[string][]*scrape.Target
}

type testTargetParams struct {
	Identifier       string
	Labels           []labels.Label
	DiscoveredLabels []labels.Label
	Params           url.Values
	Reports          []*testReport
	Active           bool
}

type testReport struct {
	Start    time.Time
	Duration time.Duration
	Error    error
}

func newTestTargetRetriever(targetsInfo []*testTargetParams) *testTargetRetriever {
	var activeTargets map[string][]*scrape.Target
	var droppedTargets map[string][]*scrape.Target
	activeTargets = make(map[string][]*scrape.Target)
	droppedTargets = make(map[string][]*scrape.Target)

	for _, t := range targetsInfo {
		nt := scrape.NewTarget(t.Labels, t.DiscoveredLabels, t.Params)

		for _, r := range t.Reports {
			nt.Report(r.Start, r.Duration, r.Error)
		}

		if t.Active {
			activeTargets[t.Identifier] = []*scrape.Target{nt}
		} else {
			droppedTargets[t.Identifier] = []*scrape.Target{nt}
		}
	}

	return &testTargetRetriever{
		activeTargets:  activeTargets,
		droppedTargets: droppedTargets,
	}
}

var (
	scrapeStart = time.Now().Add(-11 * time.Second)
)

func (t testTargetRetriever) TargetsActive() map[string][]*scrape.Target {
	return t.activeTargets
}

func (t testTargetRetriever) TargetsDropped() map[string][]*scrape.Target {
	return t.droppedTargets
}

func (t *testTargetRetriever) SetMetadataStoreForTargets(identifier string, metadata scrape.MetricMetadataStore) error {
	targets, ok := t.activeTargets[identifier]

	if !ok {
		return errors.New("targets not found")
	}

	for _, at := range targets {
		at.SetMetadataStore(metadata)
	}

	return nil
}

func (t *testTargetRetriever) ResetMetadataStore() {
	for _, at := range t.activeTargets {
		for _, tt := range at {
			tt.SetMetadataStore(&testMetaStore{})
		}
	}
}

func (t *testTargetRetriever) toFactory() func(context.Context) TargetRetriever {
	return func(context.Context) TargetRetriever { return t }
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

func (t testAlertmanagerRetriever) toFactory() func(context.Context) AlertmanagerRetriever {
	return func(context.Context) AlertmanagerRetriever { return t }
}

type rulesRetrieverMock struct {
	testing *testing.T
}

func (m rulesRetrieverMock) AlertingRules() []*rules.AlertingRule {
	expr1, err := parser.ParseExpr(`absent(test_metric3) != 1`)
	if err != nil {
		m.testing.Fatalf("unable to parse alert expression: %s", err)
	}
	expr2, err := parser.ParseExpr(`up == 1`)
	if err != nil {
		m.testing.Fatalf("Unable to parse alert expression: %s", err)
	}

	rule1 := rules.NewAlertingRule(
		"test_metric3",
		expr1,
		time.Second,
		labels.Labels{},
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
	storage := teststorage.New(m.testing)
	defer storage.Close()

	engineOpts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    100 * time.Second,
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

	recordingExpr, err := parser.ParseExpr(`vector(1)`)
	if err != nil {
		m.testing.Fatalf("unable to parse alert expression: %s", err)
	}
	recordingRule := rules.NewRecordingRule("recording-rule-1", recordingExpr, labels.Labels{})
	r = append(r, recordingRule)

	group := rules.NewGroup(rules.GroupOptions{
		Name:          "grp",
		File:          "/path/to/file",
		Interval:      time.Second,
		Rules:         r,
		ShouldRestore: false,
		Opts:          opts,
	})
	return []*rules.Group{group}
}

func (m rulesRetrieverMock) toFactory() func(context.Context) RulesRetriever {
	return func(context.Context) RulesRetriever { return m }
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
	testutil.Ok(t, err)
	defer suite.Close()

	testutil.Ok(t, suite.Run())

	now := time.Now()

	t.Run("local", func(t *testing.T) {
		var algr rulesRetrieverMock
		algr.testing = t

		algr.AlertingRules()

		algr.RuleGroups()

		testTargetRetriever := setupTestTargetRetriever(t)

		api := &API{
			Queryable:             suite.Storage(),
			QueryEngine:           suite.QueryEngine(),
			targetRetriever:       testTargetRetriever.toFactory(),
			alertmanagerRetriever: testAlertmanagerRetriever{}.toFactory(),
			flagsMap:              sampleFlagMap,
			now:                   func() time.Time { return now },
			config:                func() config.Config { return samplePrometheusCfg },
			ready:                 func(f http.HandlerFunc) http.HandlerFunc { return f },
			rulesRetriever:        algr.toFactory(),
		}

		testEndpoints(t, api, testTargetRetriever, true)
	})

	// Run all the API tests against a API that is wired to forward queries via
	// the remote read client to a test server, which in turn sends them to the
	// data from the test suite.
	t.Run("remote", func(t *testing.T) {
		server := setupRemote(suite.Storage())
		defer server.Close()

		u, err := url.Parse(server.URL)
		testutil.Ok(t, err)

		al := promlog.AllowedLevel{}
		testutil.Ok(t, al.Set("debug"))

		af := promlog.AllowedFormat{}
		testutil.Ok(t, af.Set("logfmt"))

		promlogConfig := promlog.Config{
			Level:  &al,
			Format: &af,
		}

		dbDir, err := ioutil.TempDir("", "tsdb-api-ready")
		testutil.Ok(t, err)
		defer os.RemoveAll(dbDir)

		remote := remote.NewStorage(promlog.New(&promlogConfig), prometheus.DefaultRegisterer, nil, dbDir, 1*time.Second)

		err = remote.ApplyConfig(&config.Config{
			RemoteReadConfigs: []*config.RemoteReadConfig{
				{
					URL:           &config_util.URL{URL: u},
					RemoteTimeout: model.Duration(1 * time.Second),
					ReadRecent:    true,
				},
			},
		})
		testutil.Ok(t, err)

		var algr rulesRetrieverMock
		algr.testing = t

		algr.AlertingRules()

		algr.RuleGroups()

		testTargetRetriever := setupTestTargetRetriever(t)

		api := &API{
			Queryable:             remote,
			QueryEngine:           suite.QueryEngine(),
			targetRetriever:       testTargetRetriever.toFactory(),
			alertmanagerRetriever: testAlertmanagerRetriever{}.toFactory(),
			flagsMap:              sampleFlagMap,
			now:                   func() time.Time { return now },
			config:                func() config.Config { return samplePrometheusCfg },
			ready:                 func(f http.HandlerFunc) http.HandlerFunc { return f },
			rulesRetriever:        algr.toFactory(),
		}

		testEndpoints(t, api, testTargetRetriever, false)
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

func setupTestTargetRetriever(t *testing.T) *testTargetRetriever {
	t.Helper()

	targets := []*testTargetParams{
		{
			Identifier: "test",
			Labels: labels.FromMap(map[string]string{
				model.SchemeLabel:      "http",
				model.AddressLabel:     "example.com:8080",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "test",
			}),
			DiscoveredLabels: nil,
			Params:           url.Values{},
			Reports:          []*testReport{{scrapeStart, 70 * time.Millisecond, nil}},
			Active:           true,
		},
		{
			Identifier: "blackbox",
			Labels: labels.FromMap(map[string]string{
				model.SchemeLabel:      "http",
				model.AddressLabel:     "localhost:9115",
				model.MetricsPathLabel: "/probe",
				model.JobLabel:         "blackbox",
			}),
			DiscoveredLabels: nil,
			Params:           url.Values{"target": []string{"example.com"}},
			Reports:          []*testReport{{scrapeStart, 100 * time.Millisecond, errors.New("failed")}},
			Active:           true,
		},
		{
			Identifier: "blackbox",
			Labels:     nil,
			DiscoveredLabels: labels.FromMap(map[string]string{
				model.SchemeLabel:      "http",
				model.AddressLabel:     "http://dropped.example.com:9115",
				model.MetricsPathLabel: "/probe",
				model.JobLabel:         "blackbox",
			}),
			Params: url.Values{},
			Active: false,
		},
	}

	return newTestTargetRetriever(targets)
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
			matchers, err := remote.FromLabelMatchers(query.Matchers)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			var hints *storage.SelectHints
			if query.Hints != nil {
				hints = &storage.SelectHints{
					Start: query.Hints.StartMs,
					End:   query.Hints.EndMs,
					Step:  query.Hints.StepMs,
					Func:  query.Hints.Func,
				}
			}

			querier, err := s.Querier(r.Context(), query.StartTimestampMs, query.EndTimestampMs)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer querier.Close()

			set := querier.Select(false, hints, matchers...)
			resp.Results[i], _, err = remote.ToQueryResult(set, 1e6)
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

func testEndpoints(t *testing.T, api *API, tr *testTargetRetriever, testLabelAPI bool) {
	start := time.Unix(0, 0)

	type targetMetadata struct {
		identifier string
		metadata   []scrape.MetricMetadata
	}

	type test struct {
		endpoint    apiFunc
		params      map[string]string
		query       url.Values
		response    interface{}
		responseLen int
		errType     errorType
		sorter      func(interface{})
		metadata    []targetMetadata
	}

	var tests = []test{
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"2"},
				"time":  []string{"123.4"},
			},
			response: &queryData{
				ResultType: parser.ValueTypeScalar,
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
				ResultType: parser.ValueTypeScalar,
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
				ResultType: parser.ValueTypeScalar,
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
				ResultType: parser.ValueTypeScalar,
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
				ResultType: parser.ValueTypeMatrix,
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
						ScrapePool:         "blackbox",
						ScrapeURL:          "http://localhost:9115/probe?target=example.com",
						GlobalURL:          "http://localhost:9115/probe?target=example.com",
						Health:             "down",
						LastError:          "failed: missing port in address",
						LastScrape:         scrapeStart,
						LastScrapeDuration: 0.1,
					},
					{
						DiscoveredLabels: map[string]string{},
						Labels: map[string]string{
							"job": "test",
						},
						ScrapePool:         "test",
						ScrapeURL:          "http://example.com:8080/metrics",
						GlobalURL:          "http://example.com:8080/metrics",
						Health:             "up",
						LastError:          "",
						LastScrape:         scrapeStart,
						LastScrapeDuration: 0.07,
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
			endpoint: api.targets,
			query: url.Values{
				"state": []string{"any"},
			},
			response: &TargetDiscovery{
				ActiveTargets: []*Target{
					{
						DiscoveredLabels: map[string]string{},
						Labels: map[string]string{
							"job": "blackbox",
						},
						ScrapePool:         "blackbox",
						ScrapeURL:          "http://localhost:9115/probe?target=example.com",
						GlobalURL:          "http://localhost:9115/probe?target=example.com",
						Health:             "down",
						LastError:          "failed: missing port in address",
						LastScrape:         scrapeStart,
						LastScrapeDuration: 0.1,
					},
					{
						DiscoveredLabels: map[string]string{},
						Labels: map[string]string{
							"job": "test",
						},
						ScrapePool:         "test",
						ScrapeURL:          "http://example.com:8080/metrics",
						GlobalURL:          "http://example.com:8080/metrics",
						Health:             "up",
						LastError:          "",
						LastScrape:         scrapeStart,
						LastScrapeDuration: 0.07,
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
			endpoint: api.targets,
			query: url.Values{
				"state": []string{"active"},
			},
			response: &TargetDiscovery{
				ActiveTargets: []*Target{
					{
						DiscoveredLabels: map[string]string{},
						Labels: map[string]string{
							"job": "blackbox",
						},
						ScrapePool:         "blackbox",
						ScrapeURL:          "http://localhost:9115/probe?target=example.com",
						GlobalURL:          "http://localhost:9115/probe?target=example.com",
						Health:             "down",
						LastError:          "failed: missing port in address",
						LastScrape:         scrapeStart,
						LastScrapeDuration: 0.1,
					},
					{
						DiscoveredLabels: map[string]string{},
						Labels: map[string]string{
							"job": "test",
						},
						ScrapePool:         "test",
						ScrapeURL:          "http://example.com:8080/metrics",
						GlobalURL:          "http://example.com:8080/metrics",
						Health:             "up",
						LastError:          "",
						LastScrape:         scrapeStart,
						LastScrapeDuration: 0.07,
					},
				},
				DroppedTargets: []*DroppedTarget{},
			},
		},
		{
			endpoint: api.targets,
			query: url.Values{
				"state": []string{"Dropped"},
			},
			response: &TargetDiscovery{
				ActiveTargets: []*Target{},
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
		// With a matching metric.
		{
			endpoint: api.targetMetadata,
			query: url.Values{
				"metric": []string{"go_threads"},
			},
			metadata: []targetMetadata{
				{
					identifier: "test",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_threads",
							Type:   textparse.MetricTypeGauge,
							Help:   "Number of OS threads created.",
							Unit:   "",
						},
					},
				},
			},
			response: []metricMetadata{
				{
					Target: labels.FromMap(map[string]string{
						"job": "test",
					}),
					Help: "Number of OS threads created.",
					Type: textparse.MetricTypeGauge,
					Unit: "",
				},
			},
		},
		// With a matching target.
		{
			endpoint: api.targetMetadata,
			query: url.Values{
				"match_target": []string{"{job=\"blackbox\"}"},
			},
			metadata: []targetMetadata{
				{
					identifier: "blackbox",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "prometheus_tsdb_storage_blocks_bytes",
							Type:   textparse.MetricTypeGauge,
							Help:   "The number of bytes that are currently used for local storage by all blocks.",
							Unit:   "",
						},
					},
				},
			},
			response: []metricMetadata{
				{
					Target: labels.FromMap(map[string]string{
						"job": "blackbox",
					}),
					Metric: "prometheus_tsdb_storage_blocks_bytes",
					Help:   "The number of bytes that are currently used for local storage by all blocks.",
					Type:   textparse.MetricTypeGauge,
					Unit:   "",
				},
			},
		},
		// Without a target or metric.
		{
			endpoint: api.targetMetadata,
			metadata: []targetMetadata{
				{
					identifier: "test",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_threads",
							Type:   textparse.MetricTypeGauge,
							Help:   "Number of OS threads created.",
							Unit:   "",
						},
					},
				},
				{
					identifier: "blackbox",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "prometheus_tsdb_storage_blocks_bytes",
							Type:   textparse.MetricTypeGauge,
							Help:   "The number of bytes that are currently used for local storage by all blocks.",
							Unit:   "",
						},
					},
				},
			},
			response: []metricMetadata{
				{
					Target: labels.FromMap(map[string]string{
						"job": "test",
					}),
					Metric: "go_threads",
					Help:   "Number of OS threads created.",
					Type:   textparse.MetricTypeGauge,
					Unit:   "",
				},
				{
					Target: labels.FromMap(map[string]string{
						"job": "blackbox",
					}),
					Metric: "prometheus_tsdb_storage_blocks_bytes",
					Help:   "The number of bytes that are currently used for local storage by all blocks.",
					Type:   textparse.MetricTypeGauge,
					Unit:   "",
				},
			},
			sorter: func(m interface{}) {
				sort.Slice(m.([]metricMetadata), func(i, j int) bool {
					s := m.([]metricMetadata)
					return s[i].Metric < s[j].Metric
				})
			},
		},
		// Without a matching metric.
		{
			endpoint: api.targetMetadata,
			query: url.Values{
				"match_target": []string{"{job=\"non-existentblackbox\"}"},
			},
			response: []metricMetadata{},
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
		// With metadata available.
		{
			endpoint: api.metricMetadata,
			metadata: []targetMetadata{
				{
					identifier: "test",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "prometheus_engine_query_duration_seconds",
							Type:   textparse.MetricTypeSummary,
							Help:   "Query timings",
							Unit:   "",
						},
						{
							Metric: "go_info",
							Type:   textparse.MetricTypeGauge,
							Help:   "Information about the Go environment.",
							Unit:   "",
						},
					},
				},
			},
			response: map[string][]metadata{
				"prometheus_engine_query_duration_seconds": {{textparse.MetricTypeSummary, "Query timings", ""}},
				"go_info": {{textparse.MetricTypeGauge, "Information about the Go environment.", ""}},
			},
		},
		// With duplicate metadata for a metric that comes from different targets.
		{
			endpoint: api.metricMetadata,
			metadata: []targetMetadata{
				{
					identifier: "test",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_threads",
							Type:   textparse.MetricTypeGauge,
							Help:   "Number of OS threads created",
							Unit:   "",
						},
					},
				},
				{
					identifier: "blackbox",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_threads",
							Type:   textparse.MetricTypeGauge,
							Help:   "Number of OS threads created",
							Unit:   "",
						},
					},
				},
			},
			response: map[string][]metadata{
				"go_threads": {{textparse.MetricTypeGauge, "Number of OS threads created", ""}},
			},
		},
		// With non-duplicate metadata for the same metric from different targets.
		{
			endpoint: api.metricMetadata,
			metadata: []targetMetadata{
				{
					identifier: "test",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_threads",
							Type:   textparse.MetricTypeGauge,
							Help:   "Number of OS threads created",
							Unit:   "",
						},
					},
				},
				{
					identifier: "blackbox",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_threads",
							Type:   textparse.MetricTypeGauge,
							Help:   "Number of OS threads that were created.",
							Unit:   "",
						},
					},
				},
			},
			response: map[string][]metadata{
				"go_threads": {
					{textparse.MetricTypeGauge, "Number of OS threads created", ""},
					{textparse.MetricTypeGauge, "Number of OS threads that were created.", ""},
				},
			},
			sorter: func(m interface{}) {
				v := m.(map[string][]metadata)["go_threads"]

				sort.Slice(v, func(i, j int) bool {
					return v[i].Help < v[j].Help
				})
			},
		},
		// With a limit for the number of metrics returned.
		{
			endpoint: api.metricMetadata,
			query: url.Values{
				"limit": []string{"2"},
			},
			metadata: []targetMetadata{
				{
					identifier: "test",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_threads",
							Type:   textparse.MetricTypeGauge,
							Help:   "Number of OS threads created",
							Unit:   "",
						},
						{
							Metric: "prometheus_engine_query_duration_seconds",
							Type:   textparse.MetricTypeSummary,
							Help:   "Query Timmings.",
							Unit:   "",
						},
					},
				},
				{
					identifier: "blackbox",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_gc_duration_seconds",
							Type:   textparse.MetricTypeSummary,
							Help:   "A summary of the GC invocation durations.",
							Unit:   "",
						},
					},
				},
			},
			responseLen: 2,
		},
		// When requesting a specific metric that is present.
		{
			endpoint: api.metricMetadata,
			query:    url.Values{"metric": []string{"go_threads"}},
			metadata: []targetMetadata{
				{
					identifier: "test",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_threads",
							Type:   textparse.MetricTypeGauge,
							Help:   "Number of OS threads created",
							Unit:   "",
						},
					},
				},
				{
					identifier: "blackbox",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_gc_duration_seconds",
							Type:   textparse.MetricTypeSummary,
							Help:   "A summary of the GC invocation durations.",
							Unit:   "",
						},
						{
							Metric: "go_threads",
							Type:   textparse.MetricTypeGauge,
							Help:   "Number of OS threads that were created.",
							Unit:   "",
						},
					},
				},
			},
			response: map[string][]metadata{
				"go_threads": {
					{textparse.MetricTypeGauge, "Number of OS threads created", ""},
					{textparse.MetricTypeGauge, "Number of OS threads that were created.", ""},
				},
			},
			sorter: func(m interface{}) {
				v := m.(map[string][]metadata)["go_threads"]

				sort.Slice(v, func(i, j int) bool {
					return v[i].Help < v[j].Help
				})
			},
		},
		// With a specific metric that is not present.
		{
			endpoint: api.metricMetadata,
			query:    url.Values{"metric": []string{"go_gc_duration_seconds"}},
			metadata: []targetMetadata{
				{
					identifier: "test",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_threads",
							Type:   textparse.MetricTypeGauge,
							Help:   "Number of OS threads created",
							Unit:   "",
						},
					},
				},
			},
			response: map[string][]metadata{},
		},
		// With no available metadata.
		{
			endpoint: api.metricMetadata,
			response: map[string][]metadata{},
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
								State:       "inactive",
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
								State:       "inactive",
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
		{
			endpoint: api.rules,
			query: url.Values{
				"type": []string{"alert"},
			},
			response: &RuleDiscovery{
				RuleGroups: []*RuleGroup{
					{
						Name:     "grp",
						File:     "/path/to/file",
						Interval: 1,
						Rules: []rule{
							alertingRule{
								State:       "inactive",
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
								State:       "inactive",
								Name:        "test_metric4",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.Labels{},
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "unknown",
								Type:        "alerting",
							},
						},
					},
				},
			},
		},
		{
			endpoint: api.rules,
			query: url.Values{
				"type": []string{"record"},
			},
			response: &RuleDiscovery{
				RuleGroups: []*RuleGroup{
					{
						Name:     "grp",
						File:     "/path/to/file",
						Interval: 1,
						Rules: []rule{
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
			// Start and end before LabelValues starts.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"start": []string{"-2"},
					"end":   []string{"-1"},
				},
				response: []string{},
			},
			// Start and end within LabelValues.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"start": []string{"1"},
					"end":   []string{"100"},
				},
				response: []string{
					"bar",
					"boo",
				},
			},
			// Start before LabelValues, end within LabelValues.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"start": []string{"-1"},
					"end":   []string{"3"},
				},
				response: []string{
					"bar",
					"boo",
				},
			},
			// Start before LabelValues starts, end after LabelValues ends.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"start": []string{"1969-12-31T00:00:00Z"},
					"end":   []string{"1970-02-01T00:02:03Z"},
				},
				response: []string{
					"bar",
					"boo",
				},
			},
			// Start with bad data, end within LabelValues.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"start": []string{"boop"},
					"end":   []string{"1"},
				},
				errType: errorBadData,
			},
			// Start within LabelValues, end after.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"start": []string{"1"},
					"end":   []string{"100000000"},
				},
				response: []string{
					"bar",
					"boo",
				},
			},
			// Start and end after LabelValues ends.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"start": []string{"148966367200.372"},
					"end":   []string{"148966367200.972"},
				},
				response: []string{},
			},
			// Only provide Start within LabelValues, don't provide an end time.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"start": []string{"2"},
				},
				response: []string{
					"bar",
					"boo",
				},
			},
			// Only provide end within LabelValues, don't provide a start time.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"end": []string{"100"},
				},
				response: []string{
					"bar",
					"boo",
				},
			},

			// Label names.
			{
				endpoint: api.labelNames,
				response: []string{"__name__", "foo"},
			},
			// Start and end before Label names starts.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"start": []string{"-2"},
					"end":   []string{"-1"},
				},
				response: []string{},
			},
			// Start and end within Label names.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"start": []string{"1"},
					"end":   []string{"100"},
				},
				response: []string{"__name__", "foo"},
			},
			// Start before Label names, end within Label names.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"start": []string{"-1"},
					"end":   []string{"10"},
				},
				response: []string{"__name__", "foo"},
			},

			// Start before Label names starts, end after Label names ends.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"start": []string{"-1"},
					"end":   []string{"100000"},
				},
				response: []string{"__name__", "foo"},
			},
			// Start with bad data for Label names, end within Label names.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"start": []string{"boop"},
					"end":   []string{"1"},
				},
				errType: errorBadData,
			},
			// Start within Label names, end after.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"start": []string{"1"},
					"end":   []string{"1000000006"},
				},
				response: []string{"__name__", "foo"},
			},
			// Start and end after Label names ends.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"start": []string{"148966367200.372"},
					"end":   []string{"148966367200.972"},
				},
				response: []string{},
			},
			// Only provide Start within Label names, don't provide an end time.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"start": []string{"4"},
				},
				response: []string{"__name__", "foo"},
			},
			// Only provide End within Label names, don't provide a start time.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"end": []string{"20"},
				},
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
			r.RemoteAddr = "127.0.0.1:20201"
			return r, err
		}
		r, err := http.NewRequest(m, fmt.Sprintf("http://example.com?%s", q.Encode()), nil)
		r.RemoteAddr = "127.0.0.1:20201"
		return r, err
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

			tr.ResetMetadataStore()
			for _, tm := range test.metadata {
				tr.SetMetadataStoreForTargets(tm.identifier, &testMetaStore{Metadata: tm.metadata})
			}

			res := test.endpoint(req.WithContext(ctx))
			assertAPIError(t, res.err, test.errType)

			if test.sorter != nil {
				test.sorter(res.data)
			}

			if test.responseLen != 0 {
				assertAPIResponseLength(t, res.data, test.responseLen)
			} else {
				assertAPIResponse(t, res.data, test.response)
			}
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
	if exp != errorNone {
		t.Fatalf("Expected error of type %q but got none", exp)
	}
}

func assertAPIResponse(t *testing.T, got interface{}, exp interface{}) {
	t.Helper()

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

func assertAPIResponseLength(t *testing.T, got interface{}, expLen int) {
	t.Helper()

	gotLen := reflect.ValueOf(got).Len()
	if gotLen != expLen {
		t.Fatalf(
			"Response length does not match, expected:\n%d\ngot:\n%d",
			expLen,
			gotLen,
		)
	}
}

func TestSampledReadEndpoint(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			test_metric1{foo="bar",baz="qux"} 1
	`)
	testutil.Ok(t, err)

	defer suite.Close()

	err = suite.Run()
	testutil.Ok(t, err)

	api := &API{
		Queryable:   suite.Storage(),
		QueryEngine: suite.QueryEngine(),
		config: func() config.Config {
			return config.Config{
				GlobalConfig: config.GlobalConfig{
					ExternalLabels: labels.Labels{
						// We expect external labels to be added, with the source labels honored.
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
	testutil.Ok(t, err)

	matcher2, err := labels.NewMatcher(labels.MatchEqual, "d", "e")
	testutil.Ok(t, err)

	query, err := remote.ToQuery(0, 1, []*labels.Matcher{matcher1, matcher2}, &storage.SelectHints{Step: 0, Func: "avg"})
	testutil.Ok(t, err)

	req := &prompb.ReadRequest{Queries: []*prompb.Query{query}}
	data, err := proto.Marshal(req)
	testutil.Ok(t, err)

	compressed := snappy.Encode(nil, data)
	request, err := http.NewRequest("POST", "", bytes.NewBuffer(compressed))
	testutil.Ok(t, err)

	recorder := httptest.NewRecorder()
	api.remoteRead(recorder, request)

	if recorder.Code/100 != 2 {
		t.Fatal(recorder.Code)
	}

	testutil.Equals(t, "application/x-protobuf", recorder.Result().Header.Get("Content-Type"))
	testutil.Equals(t, "snappy", recorder.Result().Header.Get("Content-Encoding"))

	// Decode the response.
	compressed, err = ioutil.ReadAll(recorder.Result().Body)
	testutil.Ok(t, err)

	uncompressed, err := snappy.Decode(nil, compressed)
	testutil.Ok(t, err)

	var resp prompb.ReadResponse
	err = proto.Unmarshal(uncompressed, &resp)
	testutil.Ok(t, err)

	if len(resp.Results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(resp.Results))
	}

	testutil.Equals(t, &prompb.QueryResult{
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
	}, resp.Results[0])
}

func TestStreamReadEndpoint(t *testing.T) {
	// First with 120 samples. We expect 1 frame with 1 chunk.
	// Second with 121 samples, We expect 1 frame with 2 chunks.
	// Third with 241 samples. We expect 1 frame with 2 chunks, and 1 frame with 1 chunk for the same series due to bytes limit.
	suite, err := promql.NewTest(t, `
		load 1m
			test_metric1{foo="bar1",baz="qux"} 0+100x119
            test_metric1{foo="bar2",baz="qux"} 0+100x120
            test_metric1{foo="bar3",baz="qux"} 0+100x240
	`)
	testutil.Ok(t, err)

	defer suite.Close()

	testutil.Ok(t, suite.Run())

	api := &API{
		Queryable:   suite.Storage(),
		QueryEngine: suite.QueryEngine(),
		config: func() config.Config {
			return config.Config{
				GlobalConfig: config.GlobalConfig{
					ExternalLabels: labels.Labels{
						// We expect external labels to be added, with the source labels honored.
						{Name: "baz", Value: "a"},
						{Name: "b", Value: "c"},
						{Name: "d", Value: "e"},
					},
				},
			}
		},
		remoteReadSampleLimit: 1e6,
		remoteReadGate:        gate.New(1),
		// Labelset has 57 bytes. Full chunk in test data has roughly 240 bytes. This allows us to have at max 2 chunks in this test.
		remoteReadMaxBytesInFrame: 57 + 480,
	}

	// Encode the request.
	matcher1, err := labels.NewMatcher(labels.MatchEqual, "__name__", "test_metric1")
	testutil.Ok(t, err)

	matcher2, err := labels.NewMatcher(labels.MatchEqual, "d", "e")
	testutil.Ok(t, err)

	matcher3, err := labels.NewMatcher(labels.MatchEqual, "foo", "bar1")
	testutil.Ok(t, err)

	query1, err := remote.ToQuery(0, 14400001, []*labels.Matcher{matcher1, matcher2}, &storage.SelectHints{
		Step:  1,
		Func:  "avg",
		Start: 0,
		End:   14400001,
	})
	testutil.Ok(t, err)

	query2, err := remote.ToQuery(0, 14400001, []*labels.Matcher{matcher1, matcher3}, &storage.SelectHints{
		Step:  1,
		Func:  "avg",
		Start: 0,
		End:   14400001,
	})
	testutil.Ok(t, err)

	req := &prompb.ReadRequest{
		Queries:               []*prompb.Query{query1, query2},
		AcceptedResponseTypes: []prompb.ReadRequest_ResponseType{prompb.ReadRequest_STREAMED_XOR_CHUNKS},
	}
	data, err := proto.Marshal(req)
	testutil.Ok(t, err)

	compressed := snappy.Encode(nil, data)
	request, err := http.NewRequest("POST", "", bytes.NewBuffer(compressed))
	testutil.Ok(t, err)

	recorder := httptest.NewRecorder()
	api.remoteRead(recorder, request)

	if recorder.Code/100 != 2 {
		t.Fatal(recorder.Code)
	}

	testutil.Equals(t, "application/x-streamed-protobuf; proto=prometheus.ChunkedReadResponse", recorder.Result().Header.Get("Content-Type"))
	testutil.Equals(t, "", recorder.Result().Header.Get("Content-Encoding"))

	var results []*prompb.ChunkedReadResponse
	stream := remote.NewChunkedReader(recorder.Result().Body, remote.DefaultChunkedReadLimit, nil)
	for {
		res := &prompb.ChunkedReadResponse{}
		err := stream.NextProto(res)
		if err == io.EOF {
			break
		}
		testutil.Ok(t, err)
		results = append(results, res)
	}

	if len(results) != 5 {
		t.Fatalf("Expected 5 result, got %d", len(results))
	}

	testutil.Equals(t, []*prompb.ChunkedReadResponse{
		{
			ChunkedSeries: []*prompb.ChunkedSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_metric1"},
						{Name: "b", Value: "c"},
						{Name: "baz", Value: "qux"},
						{Name: "d", Value: "e"},
						{Name: "foo", Value: "bar1"},
					},
					Chunks: []prompb.Chunk{
						{
							Type:      prompb.Chunk_XOR,
							MaxTimeMs: 7140000,
							Data:      []byte("\000x\000\000\000\000\000\000\000\000\000\340\324\003\302|\005\224\000\301\254}\351z2\320O\355\264n[\007\316\224\243md\371\320\375\032Pm\nS\235\016Q\255\006P\275\250\277\312\201Z\003(3\240R\207\332\005(\017\240\322\201\332=(\023\2402\203Z\007(w\2402\201Z\017(\023\265\227\364P\033@\245\007\364\nP\033C\245\002t\036P+@e\036\364\016Pk@e\002t:P;A\245\001\364\nS\373@\245\006t\006P+C\345\002\364\006Pk@\345\036t\nP\033A\245\003\364:P\033@\245\006t\016ZJ\377\\\205\313\210\327\270\017\345+F[\310\347E)\355\024\241\366\342}(v\215(N\203)\326\207(\336\203(V\332W\362\202t4\240m\005(\377AJ\006\320\322\202t\374\240\255\003(oA\312:\3202"),
						},
					},
				},
			},
		},
		{
			ChunkedSeries: []*prompb.ChunkedSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_metric1"},
						{Name: "b", Value: "c"},
						{Name: "baz", Value: "qux"},
						{Name: "d", Value: "e"},
						{Name: "foo", Value: "bar2"},
					},
					Chunks: []prompb.Chunk{
						{
							Type:      prompb.Chunk_XOR,
							MaxTimeMs: 7140000,
							Data:      []byte("\000x\000\000\000\000\000\000\000\000\000\340\324\003\302|\005\224\000\301\254}\351z2\320O\355\264n[\007\316\224\243md\371\320\375\032Pm\nS\235\016Q\255\006P\275\250\277\312\201Z\003(3\240R\207\332\005(\017\240\322\201\332=(\023\2402\203Z\007(w\2402\201Z\017(\023\265\227\364P\033@\245\007\364\nP\033C\245\002t\036P+@e\036\364\016Pk@e\002t:P;A\245\001\364\nS\373@\245\006t\006P+C\345\002\364\006Pk@\345\036t\nP\033A\245\003\364:P\033@\245\006t\016ZJ\377\\\205\313\210\327\270\017\345+F[\310\347E)\355\024\241\366\342}(v\215(N\203)\326\207(\336\203(V\332W\362\202t4\240m\005(\377AJ\006\320\322\202t\374\240\255\003(oA\312:\3202"),
						},
						{
							Type:      prompb.Chunk_XOR,
							MinTimeMs: 7200000,
							MaxTimeMs: 7200000,
							Data:      []byte("\000\001\200\364\356\006@\307p\000\000\000\000\000\000"),
						},
					},
				},
			},
		},
		{
			ChunkedSeries: []*prompb.ChunkedSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_metric1"},
						{Name: "b", Value: "c"},
						{Name: "baz", Value: "qux"},
						{Name: "d", Value: "e"},
						{Name: "foo", Value: "bar3"},
					},
					Chunks: []prompb.Chunk{
						{
							Type:      prompb.Chunk_XOR,
							MaxTimeMs: 7140000,
							Data:      []byte("\000x\000\000\000\000\000\000\000\000\000\340\324\003\302|\005\224\000\301\254}\351z2\320O\355\264n[\007\316\224\243md\371\320\375\032Pm\nS\235\016Q\255\006P\275\250\277\312\201Z\003(3\240R\207\332\005(\017\240\322\201\332=(\023\2402\203Z\007(w\2402\201Z\017(\023\265\227\364P\033@\245\007\364\nP\033C\245\002t\036P+@e\036\364\016Pk@e\002t:P;A\245\001\364\nS\373@\245\006t\006P+C\345\002\364\006Pk@\345\036t\nP\033A\245\003\364:P\033@\245\006t\016ZJ\377\\\205\313\210\327\270\017\345+F[\310\347E)\355\024\241\366\342}(v\215(N\203)\326\207(\336\203(V\332W\362\202t4\240m\005(\377AJ\006\320\322\202t\374\240\255\003(oA\312:\3202"),
						},
						{
							Type:      prompb.Chunk_XOR,
							MinTimeMs: 7200000,
							MaxTimeMs: 14340000,
							Data:      []byte("\000x\200\364\356\006@\307p\000\000\000\000\000\340\324\003\340>\224\355\260\277\322\200\372\005(=\240R\207:\003(\025\240\362\201z\003(\365\240r\203:\005(\r\241\322\201\372\r(\r\240R\237:\007(5\2402\201z\037(\025\2402\203:\005(\375\240R\200\372\r(\035\241\322\201:\003(5\240r\326g\364\271\213\227!\253q\037\312N\340GJ\033E)\375\024\241\266\362}(N\217(V\203)\336\207(\326\203(N\334W\322\203\2644\240}\005(\373AJ\031\3202\202\264\374\240\275\003(kA\3129\320R\201\2644\240\375\264\277\322\200\332\005(3\240r\207Z\003(\027\240\362\201Z\003(\363\240R\203\332\005(\017\241\322\201\332\r(\023\2402\237Z\007(7\2402\201Z\037(\023\240\322\200\332\005(\377\240R\200\332\r "),
						},
					},
				},
			},
		},
		{
			ChunkedSeries: []*prompb.ChunkedSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_metric1"},
						{Name: "b", Value: "c"},
						{Name: "baz", Value: "qux"},
						{Name: "d", Value: "e"},
						{Name: "foo", Value: "bar3"},
					},
					Chunks: []prompb.Chunk{
						{
							Type:      prompb.Chunk_XOR,
							MinTimeMs: 14400000,
							MaxTimeMs: 14400000,
							Data:      []byte("\000\001\200\350\335\r@\327p\000\000\000\000\000\000"),
						},
					},
				},
			},
		},
		{
			ChunkedSeries: []*prompb.ChunkedSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_metric1"},
						{Name: "b", Value: "c"},
						{Name: "baz", Value: "qux"},
						{Name: "d", Value: "e"},
						{Name: "foo", Value: "bar1"},
					},
					Chunks: []prompb.Chunk{
						{
							Type:      prompb.Chunk_XOR,
							MaxTimeMs: 7140000,
							Data:      []byte("\000x\000\000\000\000\000\000\000\000\000\340\324\003\302|\005\224\000\301\254}\351z2\320O\355\264n[\007\316\224\243md\371\320\375\032Pm\nS\235\016Q\255\006P\275\250\277\312\201Z\003(3\240R\207\332\005(\017\240\322\201\332=(\023\2402\203Z\007(w\2402\201Z\017(\023\265\227\364P\033@\245\007\364\nP\033C\245\002t\036P+@e\036\364\016Pk@e\002t:P;A\245\001\364\nS\373@\245\006t\006P+C\345\002\364\006Pk@\345\036t\nP\033A\245\003\364:P\033@\245\006t\016ZJ\377\\\205\313\210\327\270\017\345+F[\310\347E)\355\024\241\366\342}(v\215(N\203)\326\207(\336\203(V\332W\362\202t4\240m\005(\377AJ\006\320\322\202t\374\240\255\003(oA\312:\3202"),
						},
					},
				},
			},
			QueryIndex: 1,
		},
	}, results)
}

type fakeDB struct {
	err error
}

func (f *fakeDB) CleanTombstones() error                               { return f.err }
func (f *fakeDB) Delete(mint, maxt int64, ms ...*labels.Matcher) error { return f.err }
func (f *fakeDB) Snapshot(dir string, withHead bool) error             { return f.err }
func (f *fakeDB) Stats(statsByLabelName string) (_ *tsdb.Stats, retErr error) {
	dbDir, err := ioutil.TempDir("", "tsdb-api-ready")
	if err != nil {
		return nil, err
	}
	defer func() {
		err := os.RemoveAll(dbDir)
		if retErr != nil {
			retErr = err
		}
	}()
	h, _ := tsdb.NewHead(nil, nil, nil, 1000, "", nil, tsdb.DefaultStripeSize, nil)
	return h.Stats(statsByLabelName), nil
}

func TestAdminEndpoints(t *testing.T) {
	tsdb, tsdbWithError, tsdbNotReady := &fakeDB{}, &fakeDB{err: errors.New("some error")}, &fakeDB{err: errors.Wrap(tsdb.ErrNotReady, "wrap")}
	snapshotAPI := func(api *API) apiFunc { return api.snapshot }
	cleanAPI := func(api *API) apiFunc { return api.cleanTombstones }
	deleteAPI := func(api *API) apiFunc { return api.deleteSeries }

	for _, tc := range []struct {
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
			db:          tsdbNotReady,
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
			db:          tsdbNotReady,
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
			db:          tsdbNotReady,
			enableAdmin: true,
			endpoint:    deleteAPI,
			values:      map[string][]string{"match[]": {"up"}},

			errType: errorUnavailable,
		},
	} {
		tc := tc
		t.Run("", func(t *testing.T) {
			dir, _ := ioutil.TempDir("", "fakeDB")
			defer testutil.Ok(t, os.RemoveAll(dir))

			api := &API{
				db:          tc.db,
				dbDir:       dir,
				ready:       func(f http.HandlerFunc) http.HandlerFunc { return f },
				enableAdmin: tc.enableAdmin,
			}

			endpoint := tc.endpoint(api)
			req, err := http.NewRequest(tc.method, fmt.Sprintf("?%s", tc.values.Encode()), nil)
			testutil.Ok(t, err)

			res := setUnavailStatusOnTSDBNotReady(endpoint(req))
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

func TestParseTimeParam(t *testing.T) {
	type resultType struct {
		asTime  time.Time
		asError func() error
	}

	ts, err := parseTime("1582468023986")
	testutil.Ok(t, err)

	var tests = []struct {
		paramName    string
		paramValue   string
		defaultValue time.Time
		result       resultType
	}{
		{ // When data is valid.
			paramName:    "start",
			paramValue:   "1582468023986",
			defaultValue: minTime,
			result: resultType{
				asTime:  ts,
				asError: nil,
			},
		},
		{ // When data is empty string.
			paramName:    "end",
			paramValue:   "",
			defaultValue: maxTime,
			result: resultType{
				asTime:  maxTime,
				asError: nil,
			},
		},
		{ // When data is not valid.
			paramName:    "foo",
			paramValue:   "baz",
			defaultValue: maxTime,
			result: resultType{
				asTime: time.Time{},
				asError: func() error {
					_, err := parseTime("baz")
					return errors.Wrapf(err, "Invalid time value for '%s'", "foo")
				},
			},
		},
	}

	for _, test := range tests {
		req, err := http.NewRequest("GET", "localhost:42/foo?"+test.paramName+"="+test.paramValue, nil)
		testutil.Ok(t, err)

		result := test.result
		asTime, err := parseTimeParam(req, test.paramName, test.defaultValue)

		if err != nil {
			testutil.ErrorEqual(t, result.asError(), err)
		} else {
			testutil.Assert(t, asTime.Equal(result.asTime), "time as return value: %s not parsed correctly. Expected %s. Actual %s", test.paramValue, result.asTime, asTime)
		}
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
		{
			input:  minTime.Format(time.RFC3339Nano),
			result: minTime,
		},
		{
			input:  maxTime.Format(time.RFC3339Nano),
			result: maxTime,
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
				ResultType: parser.ValueTypeMatrix,
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

func TestTSDBStatus(t *testing.T) {
	tsdb := &fakeDB{}
	tsdbStatusAPI := func(api *API) apiFunc { return api.serveTSDBStatus }

	for i, tc := range []struct {
		db       *fakeDB
		endpoint func(api *API) apiFunc
		method   string
		values   url.Values

		errType errorType
	}{
		// Tests for the TSDB Status endpoint.
		{
			db:       tsdb,
			endpoint: tsdbStatusAPI,

			errType: errorNone,
		},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			api := &API{db: tc.db}
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

func TestReturnAPIError(t *testing.T) {
	cases := []struct {
		err      error
		expected errorType
	}{
		{
			err:      promql.ErrStorage{Err: errors.New("storage error")},
			expected: errorInternal,
		}, {
			err:      errors.Wrap(promql.ErrStorage{Err: errors.New("storage error")}, "wrapped"),
			expected: errorInternal,
		}, {
			err:      promql.ErrQueryTimeout("timeout error"),
			expected: errorTimeout,
		}, {
			err:      errors.Wrap(promql.ErrQueryTimeout("timeout error"), "wrapped"),
			expected: errorTimeout,
		}, {
			err:      promql.ErrQueryCanceled("canceled error"),
			expected: errorCanceled,
		}, {
			err:      errors.Wrap(promql.ErrQueryCanceled("canceled error"), "wrapped"),
			expected: errorCanceled,
		}, {
			err:      errors.New("exec error"),
			expected: errorExec,
		},
	}

	for _, c := range cases {
		actual := returnAPIError(c.err)
		testutil.NotOk(t, actual)
		testutil.Equals(t, c.expected, actual.typ)
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
		ResultType: parser.ValueTypeMatrix,
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
