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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/util/stats"
	"github.com/prometheus/prometheus/util/testutil"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/route"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/teststorage"
)

var testEngine = promql.NewEngine(promql.EngineOpts{
	Logger:                   nil,
	Reg:                      nil,
	MaxSamples:               10000,
	Timeout:                  100 * time.Second,
	NoStepSubqueryIntervalFn: func(int64) int64 { return 60 * 1000 },
	EnableAtModifier:         true,
	EnableNegativeOffset:     true,
	EnablePerStepStats:       true,
})

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
	Labels           labels.Labels
	DiscoveredLabels labels.Labels
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

var scrapeStart = time.Now().Add(-11 * time.Second)

func (t testTargetRetriever) TargetsActive() map[string][]*scrape.Target {
	return t.activeTargets
}

func (t testTargetRetriever) TargetsDropped() map[string][]*scrape.Target {
	return t.droppedTargets
}

func (t testTargetRetriever) TargetsDroppedCounts() map[string]int {
	r := make(map[string]int)
	for k, v := range t.droppedTargets {
		r[k] = len(v)
	}
	return r
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
	alertingRules []*rules.AlertingRule
	ruleGroups    []*rules.Group
	testing       *testing.T
}

func (m *rulesRetrieverMock) CreateAlertingRules() {
	expr1, err := parser.ParseExpr(`absent(test_metric3) != 1`)
	require.NoError(m.testing, err)
	expr2, err := parser.ParseExpr(`up == 1`)
	require.NoError(m.testing, err)
	expr3, err := parser.ParseExpr(`vector(1)`)
	require.NoError(m.testing, err)

	rule1 := rules.NewAlertingRule(
		"test_metric3",
		expr1,
		time.Second,
		0,
		labels.Labels{},
		labels.Labels{},
		labels.Labels{},
		"",
		true,
		log.NewNopLogger(),
	)
	rule2 := rules.NewAlertingRule(
		"test_metric4",
		expr2,
		time.Second,
		0,
		labels.Labels{},
		labels.Labels{},
		labels.Labels{},
		"",
		true,
		log.NewNopLogger(),
	)
	rule3 := rules.NewAlertingRule(
		"test_metric5",
		expr3,
		time.Second,
		0,
		labels.FromStrings("name", "tm5"),
		labels.Labels{},
		labels.FromStrings("name", "tm5"),
		"",
		false,
		log.NewNopLogger(),
	)

	var r []*rules.AlertingRule
	r = append(r, rule1)
	r = append(r, rule2)
	r = append(r, rule3)
	m.alertingRules = r
}

func (m *rulesRetrieverMock) CreateRuleGroups() {
	m.CreateAlertingRules()
	arules := m.AlertingRules()
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
		NotifyFunc: func(ctx context.Context, expr string, alerts ...*rules.Alert) {},
	}

	var r []rules.Rule

	for _, alertrule := range arules {
		r = append(r, alertrule)
	}

	recordingExpr, err := parser.ParseExpr(`vector(1)`)
	require.NoError(m.testing, err, "unable to parse alert expression")
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
	m.ruleGroups = []*rules.Group{group}
}

func (m *rulesRetrieverMock) AlertingRules() []*rules.AlertingRule {
	return m.alertingRules
}

func (m *rulesRetrieverMock) RuleGroups() []*rules.Group {
	return m.ruleGroups
}

func (m *rulesRetrieverMock) toFactory() func(context.Context) RulesRetriever {
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
	storage := promql.LoadedStorage(t, `
		load 1m
			test_metric1{foo="bar"} 0+100x100
			test_metric1{foo="boo"} 1+0x100
			test_metric2{foo="boo"} 1+0x100
			test_metric3{foo="bar", dup="1"} 1+0x100
			test_metric3{foo="boo", dup="1"} 1+0x100
			test_metric4{foo="bar", dup="1"} 1+0x100
			test_metric4{foo="boo", dup="1"} 1+0x100
			test_metric4{foo="boo"} 1+0x100
	`)
	t.Cleanup(func() { storage.Close() })

	start := time.Unix(0, 0)
	exemplars := []exemplar.QueryResult{
		{
			SeriesLabels: labels.FromStrings("__name__", "test_metric3", "foo", "boo", "dup", "1"),
			Exemplars: []exemplar.Exemplar{
				{
					Labels: labels.FromStrings("id", "abc"),
					Value:  10,
					Ts:     timestamp.FromTime(start.Add(2 * time.Second)),
				},
			},
		},
		{
			SeriesLabels: labels.FromStrings("__name__", "test_metric4", "foo", "bar", "dup", "1"),
			Exemplars: []exemplar.Exemplar{
				{
					Labels: labels.FromStrings("id", "lul"),
					Value:  10,
					Ts:     timestamp.FromTime(start.Add(4 * time.Second)),
				},
			},
		},
		{
			SeriesLabels: labels.FromStrings("__name__", "test_metric3", "foo", "boo", "dup", "1"),
			Exemplars: []exemplar.Exemplar{
				{
					Labels: labels.FromStrings("id", "abc2"),
					Value:  10,
					Ts:     timestamp.FromTime(start.Add(4053 * time.Millisecond)),
				},
			},
		},
		{
			SeriesLabels: labels.FromStrings("__name__", "test_metric4", "foo", "bar", "dup", "1"),
			Exemplars: []exemplar.Exemplar{
				{
					Labels: labels.FromStrings("id", "lul2"),
					Value:  10,
					Ts:     timestamp.FromTime(start.Add(4153 * time.Millisecond)),
				},
			},
		},
	}
	for _, ed := range exemplars {
		_, err := storage.AppendExemplar(0, ed.SeriesLabels, ed.Exemplars[0])
		require.NoError(t, err, "failed to add exemplar: %+v", ed.Exemplars[0])
	}

	now := time.Now()

	t.Run("local", func(t *testing.T) {
		algr := rulesRetrieverMock{}
		algr.testing = t

		algr.CreateAlertingRules()
		algr.CreateRuleGroups()

		g := algr.RuleGroups()
		g[0].Eval(context.Background(), time.Now())

		testTargetRetriever := setupTestTargetRetriever(t)

		api := &API{
			Queryable:             storage,
			QueryEngine:           testEngine,
			ExemplarQueryable:     storage.ExemplarQueryable(),
			targetRetriever:       testTargetRetriever.toFactory(),
			alertmanagerRetriever: testAlertmanagerRetriever{}.toFactory(),
			flagsMap:              sampleFlagMap,
			now:                   func() time.Time { return now },
			config:                func() config.Config { return samplePrometheusCfg },
			ready:                 func(f http.HandlerFunc) http.HandlerFunc { return f },
			rulesRetriever:        algr.toFactory(),
		}
		testEndpoints(t, api, testTargetRetriever, storage, true)
	})

	// Run all the API tests against an API that is wired to forward queries via
	// the remote read client to a test server, which in turn sends them to the
	// data from the test storage.
	t.Run("remote", func(t *testing.T) {
		server := setupRemote(storage)
		defer server.Close()

		u, err := url.Parse(server.URL)
		require.NoError(t, err)

		al := promlog.AllowedLevel{}
		require.NoError(t, al.Set("debug"))

		af := promlog.AllowedFormat{}
		require.NoError(t, af.Set("logfmt"))

		promlogConfig := promlog.Config{
			Level:  &al,
			Format: &af,
		}

		dbDir := t.TempDir()

		remote := remote.NewStorage(promlog.New(&promlogConfig), prometheus.DefaultRegisterer, func() (int64, error) {
			return 0, nil
		}, dbDir, 1*time.Second, nil)

		err = remote.ApplyConfig(&config.Config{
			RemoteReadConfigs: []*config.RemoteReadConfig{
				{
					URL:           &config_util.URL{URL: u},
					RemoteTimeout: model.Duration(1 * time.Second),
					ReadRecent:    true,
				},
			},
		})
		require.NoError(t, err)

		algr := rulesRetrieverMock{}
		algr.testing = t

		algr.CreateAlertingRules()
		algr.CreateRuleGroups()

		g := algr.RuleGroups()
		g[0].Eval(context.Background(), time.Now())

		testTargetRetriever := setupTestTargetRetriever(t)

		api := &API{
			Queryable:             remote,
			QueryEngine:           testEngine,
			ExemplarQueryable:     storage.ExemplarQueryable(),
			targetRetriever:       testTargetRetriever.toFactory(),
			alertmanagerRetriever: testAlertmanagerRetriever{}.toFactory(),
			flagsMap:              sampleFlagMap,
			now:                   func() time.Time { return now },
			config:                func() config.Config { return samplePrometheusCfg },
			ready:                 func(f http.HandlerFunc) http.HandlerFunc { return f },
			rulesRetriever:        algr.toFactory(),
		}
		testEndpoints(t, api, testTargetRetriever, storage, false)
	})
}

type byLabels []labels.Labels

func (b byLabels) Len() int           { return len(b) }
func (b byLabels) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byLabels) Less(i, j int) bool { return labels.Compare(b[i], b[j]) < 0 }

func TestGetSeries(t *testing.T) {
	// TestEndpoints doesn't have enough label names to test api.labelNames
	// endpoint properly. Hence we test it separately.
	storage := promql.LoadedStorage(t, `
		load 1m
			test_metric1{foo1="bar", baz="abc"} 0+100x100
			test_metric1{foo2="boo"} 1+0x100
			test_metric2{foo="boo"} 1+0x100
			test_metric2{foo="boo", xyz="qwerty"} 1+0x100
			test_metric2{foo="baz", abc="qwerty"} 1+0x100
	`)
	t.Cleanup(func() { storage.Close() })
	api := &API{
		Queryable: storage,
	}
	request := func(method string, matchers ...string) (*http.Request, error) {
		u, err := url.Parse("http://example.com")
		require.NoError(t, err)
		q := u.Query()
		for _, matcher := range matchers {
			q.Add("match[]", matcher)
		}
		u.RawQuery = q.Encode()

		r, err := http.NewRequest(method, u.String(), nil)
		if method == http.MethodPost {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		return r, err
	}

	for _, tc := range []struct {
		name              string
		api               *API
		matchers          []string
		expected          []labels.Labels
		expectedErrorType errorType
	}{
		{
			name:              "no matchers",
			expectedErrorType: errorBadData,
			api:               api,
		},
		{
			name:     "non empty label matcher",
			matchers: []string{`{foo=~".+"}`},
			expected: []labels.Labels{
				labels.FromStrings("__name__", "test_metric2", "abc", "qwerty", "foo", "baz"),
				labels.FromStrings("__name__", "test_metric2", "foo", "boo"),
				labels.FromStrings("__name__", "test_metric2", "foo", "boo", "xyz", "qwerty"),
			},
			api: api,
		},
		{
			name:     "exact label matcher",
			matchers: []string{`{foo="boo"}`},
			expected: []labels.Labels{
				labels.FromStrings("__name__", "test_metric2", "foo", "boo"),
				labels.FromStrings("__name__", "test_metric2", "foo", "boo", "xyz", "qwerty"),
			},
			api: api,
		},
		{
			name:     "two matchers",
			matchers: []string{`{foo="boo"}`, `{foo="baz"}`},
			expected: []labels.Labels{
				labels.FromStrings("__name__", "test_metric2", "abc", "qwerty", "foo", "baz"),
				labels.FromStrings("__name__", "test_metric2", "foo", "boo"),
				labels.FromStrings("__name__", "test_metric2", "foo", "boo", "xyz", "qwerty"),
			},
			api: api,
		},
		{
			name:              "exec error type",
			matchers:          []string{`{foo="boo"}`, `{foo="baz"}`},
			expectedErrorType: errorExec,
			api: &API{
				Queryable: errorTestQueryable{err: fmt.Errorf("generic")},
			},
		},
		{
			name:              "storage error type",
			matchers:          []string{`{foo="boo"}`, `{foo="baz"}`},
			expectedErrorType: errorInternal,
			api: &API{
				Queryable: errorTestQueryable{err: promql.ErrStorage{Err: fmt.Errorf("generic")}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			req, err := request(http.MethodGet, tc.matchers...)
			require.NoError(t, err)
			res := tc.api.series(req.WithContext(ctx))
			assertAPIError(t, res.err, tc.expectedErrorType)
			if tc.expectedErrorType == errorNone {
				r := res.data.([]labels.Labels)
				sort.Sort(byLabels(tc.expected))
				sort.Sort(byLabels(r))
				testutil.RequireEqual(t, tc.expected, r)
			}
		})
	}
}

func TestQueryExemplars(t *testing.T) {
	start := time.Unix(0, 0)
	storage := promql.LoadedStorage(t, `
		load 1m
			test_metric1{foo="bar"} 0+100x100
			test_metric1{foo="boo"} 1+0x100
			test_metric2{foo="boo"} 1+0x100
			test_metric3{foo="bar", dup="1"} 1+0x100
			test_metric3{foo="boo", dup="1"} 1+0x100
			test_metric4{foo="bar", dup="1"} 1+0x100
			test_metric4{foo="boo", dup="1"} 1+0x100
			test_metric4{foo="boo"} 1+0x100
	`)
	t.Cleanup(func() { storage.Close() })

	api := &API{
		Queryable:         storage,
		QueryEngine:       testEngine,
		ExemplarQueryable: storage.ExemplarQueryable(),
	}

	request := func(method string, qs url.Values) (*http.Request, error) {
		u, err := url.Parse("http://example.com")
		require.NoError(t, err)
		u.RawQuery = qs.Encode()
		r, err := http.NewRequest(method, u.String(), nil)
		if method == http.MethodPost {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		return r, err
	}

	for _, tc := range []struct {
		name              string
		query             url.Values
		exemplars         []exemplar.QueryResult
		api               *API
		expectedErrorType errorType
	}{
		{
			name: "no error",
			api:  api,
			query: url.Values{
				"query": []string{`test_metric3{foo="boo"} - test_metric4{foo="bar"}`},
				"start": []string{"0"},
				"end":   []string{"4"},
			},
			exemplars: []exemplar.QueryResult{
				{
					SeriesLabels: labels.FromStrings("__name__", "test_metric3", "foo", "boo", "dup", "1"),
					Exemplars: []exemplar.Exemplar{
						{
							Labels: labels.FromStrings("id", "abc"),
							Value:  10,
							Ts:     timestamp.FromTime(start.Add(0 * time.Second)),
						},
					},
				},
				{
					SeriesLabels: labels.FromStrings("__name__", "test_metric4", "foo", "bar", "dup", "1"),
					Exemplars: []exemplar.Exemplar{
						{
							Labels: labels.FromStrings("id", "lul"),
							Value:  10,
							Ts:     timestamp.FromTime(start.Add(3 * time.Second)),
						},
					},
				},
			},
		},
		{
			name:              "should return errorExec upon genetic error",
			expectedErrorType: errorExec,
			api: &API{
				ExemplarQueryable: errorTestQueryable{err: fmt.Errorf("generic")},
			},
			query: url.Values{
				"query": []string{`test_metric3{foo="boo"} - test_metric4{foo="bar"}`},
				"start": []string{"0"},
				"end":   []string{"4"},
			},
		},
		{
			name:              "should return errorInternal err type is ErrStorage",
			expectedErrorType: errorInternal,
			api: &API{
				ExemplarQueryable: errorTestQueryable{err: promql.ErrStorage{Err: fmt.Errorf("generic")}},
			},
			query: url.Values{
				"query": []string{`test_metric3{foo="boo"} - test_metric4{foo="bar"}`},
				"start": []string{"0"},
				"end":   []string{"4"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			es := storage
			ctx := context.Background()

			for _, te := range tc.exemplars {
				for _, e := range te.Exemplars {
					_, err := es.AppendExemplar(0, te.SeriesLabels, e)
					require.NoError(t, err)
				}
			}

			req, err := request(http.MethodGet, tc.query)
			require.NoError(t, err)
			res := tc.api.queryExemplars(req.WithContext(ctx))
			assertAPIError(t, res.err, tc.expectedErrorType)

			if tc.expectedErrorType == errorNone {
				assertAPIResponse(t, res.data, tc.exemplars)
			}
		})
	}
}

func TestLabelNames(t *testing.T) {
	// TestEndpoints doesn't have enough label names to test api.labelNames
	// endpoint properly. Hence we test it separately.
	storage := promql.LoadedStorage(t, `
		load 1m
			test_metric1{foo1="bar", baz="abc"} 0+100x100
			test_metric1{foo2="boo"} 1+0x100
			test_metric2{foo="boo"} 1+0x100
			test_metric2{foo="boo", xyz="qwerty"} 1+0x100
			test_metric2{foo="baz", abc="qwerty"} 1+0x100
	`)
	t.Cleanup(func() { storage.Close() })
	api := &API{
		Queryable: storage,
	}
	request := func(method string, matchers ...string) (*http.Request, error) {
		u, err := url.Parse("http://example.com")
		require.NoError(t, err)
		q := u.Query()
		for _, matcher := range matchers {
			q.Add("match[]", matcher)
		}
		u.RawQuery = q.Encode()

		r, err := http.NewRequest(method, u.String(), nil)
		if method == http.MethodPost {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		return r, err
	}

	for _, tc := range []struct {
		name              string
		api               *API
		matchers          []string
		expected          []string
		expectedErrorType errorType
	}{
		{
			name:     "no matchers",
			expected: []string{"__name__", "abc", "baz", "foo", "foo1", "foo2", "xyz"},
			api:      api,
		},
		{
			name:     "non empty label matcher",
			matchers: []string{`{foo=~".+"}`},
			expected: []string{"__name__", "abc", "foo", "xyz"},
			api:      api,
		},
		{
			name:     "exact label matcher",
			matchers: []string{`{foo="boo"}`},
			expected: []string{"__name__", "foo", "xyz"},
			api:      api,
		},
		{
			name:     "two matchers",
			matchers: []string{`{foo="boo"}`, `{foo="baz"}`},
			expected: []string{"__name__", "abc", "foo", "xyz"},
			api:      api,
		},
		{
			name:              "exec error type",
			matchers:          []string{`{foo="boo"}`, `{foo="baz"}`},
			expectedErrorType: errorExec,
			api: &API{
				Queryable: errorTestQueryable{err: fmt.Errorf("generic")},
			},
		},
		{
			name:              "storage error type",
			matchers:          []string{`{foo="boo"}`, `{foo="baz"}`},
			expectedErrorType: errorInternal,
			api: &API{
				Queryable: errorTestQueryable{err: promql.ErrStorage{Err: fmt.Errorf("generic")}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				ctx := context.Background()
				req, err := request(method, tc.matchers...)
				require.NoError(t, err)
				res := tc.api.labelNames(req.WithContext(ctx))
				assertAPIError(t, res.err, tc.expectedErrorType)
				if tc.expectedErrorType == errorNone {
					assertAPIResponse(t, res.data, tc.expected)
				}
			}
		})
	}
}

type testStats struct {
	Custom string `json:"custom"`
}

func (testStats) Builtin() (_ stats.BuiltinStats) {
	return
}

func TestStats(t *testing.T) {
	storage := teststorage.New(t)
	t.Cleanup(func() { storage.Close() })

	api := &API{
		Queryable:   storage,
		QueryEngine: testEngine,
		now: func() time.Time {
			return time.Unix(123, 0)
		},
	}
	request := func(method, param string) (*http.Request, error) {
		u, err := url.Parse("http://example.com")
		require.NoError(t, err)
		q := u.Query()
		q.Add("stats", param)
		q.Add("query", "up")
		q.Add("start", "0")
		q.Add("end", "100")
		q.Add("step", "10")
		u.RawQuery = q.Encode()

		r, err := http.NewRequest(method, u.String(), nil)
		if method == http.MethodPost {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		return r, err
	}

	for _, tc := range []struct {
		name     string
		renderer StatsRenderer
		param    string
		expected func(*testing.T, interface{})
	}{
		{
			name:  "stats is blank",
			param: "",
			expected: func(t *testing.T, i interface{}) {
				require.IsType(t, &QueryData{}, i)
				qd := i.(*QueryData)
				require.Nil(t, qd.Stats)
			},
		},
		{
			name:  "stats is true",
			param: "true",
			expected: func(t *testing.T, i interface{}) {
				require.IsType(t, &QueryData{}, i)
				qd := i.(*QueryData)
				require.NotNil(t, qd.Stats)
				qs := qd.Stats.Builtin()
				require.NotNil(t, qs.Timings)
				require.Greater(t, qs.Timings.EvalTotalTime, float64(0))
				require.NotNil(t, qs.Samples)
				require.NotNil(t, qs.Samples.TotalQueryableSamples)
				require.Nil(t, qs.Samples.TotalQueryableSamplesPerStep)
			},
		},
		{
			name:  "stats is all",
			param: "all",
			expected: func(t *testing.T, i interface{}) {
				require.IsType(t, &QueryData{}, i)
				qd := i.(*QueryData)
				require.NotNil(t, qd.Stats)
				qs := qd.Stats.Builtin()
				require.NotNil(t, qs.Timings)
				require.Greater(t, qs.Timings.EvalTotalTime, float64(0))
				require.NotNil(t, qs.Samples)
				require.NotNil(t, qs.Samples.TotalQueryableSamples)
				require.NotNil(t, qs.Samples.TotalQueryableSamplesPerStep)
			},
		},
		{
			name: "custom handler with known value",
			renderer: func(ctx context.Context, s *stats.Statistics, p string) stats.QueryStats {
				if p == "known" {
					return testStats{"Custom Value"}
				}
				return nil
			},
			param: "known",
			expected: func(t *testing.T, i interface{}) {
				require.IsType(t, &QueryData{}, i)
				qd := i.(*QueryData)
				require.NotNil(t, qd.Stats)
				j, err := json.Marshal(qd.Stats)
				require.NoError(t, err)
				require.JSONEq(t, `{"custom":"Custom Value"}`, string(j))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := api.statsRenderer
			defer func() { api.statsRenderer = before }()
			api.statsRenderer = tc.renderer

			for _, method := range []string{http.MethodGet, http.MethodPost} {
				ctx := context.Background()
				req, err := request(method, tc.param)
				require.NoError(t, err)
				res := api.query(req.WithContext(ctx))
				assertAPIError(t, res.err, "")
				tc.expected(t, res.data)

				res = api.queryRange(req.WithContext(ctx))
				assertAPIError(t, res.err, "")
				tc.expected(t, res.data)
			}
		})
	}
}

func setupTestTargetRetriever(t *testing.T) *testTargetRetriever {
	t.Helper()

	targets := []*testTargetParams{
		{
			Identifier: "test",
			Labels: labels.FromMap(map[string]string{
				model.SchemeLabel:         "http",
				model.AddressLabel:        "example.com:8080",
				model.MetricsPathLabel:    "/metrics",
				model.JobLabel:            "test",
				model.ScrapeIntervalLabel: "15s",
				model.ScrapeTimeoutLabel:  "5s",
			}),
			DiscoveredLabels: labels.EmptyLabels(),
			Params:           url.Values{},
			Reports:          []*testReport{{scrapeStart, 70 * time.Millisecond, nil}},
			Active:           true,
		},
		{
			Identifier: "blackbox",
			Labels: labels.FromMap(map[string]string{
				model.SchemeLabel:         "http",
				model.AddressLabel:        "localhost:9115",
				model.MetricsPathLabel:    "/probe",
				model.JobLabel:            "blackbox",
				model.ScrapeIntervalLabel: "20s",
				model.ScrapeTimeoutLabel:  "10s",
			}),
			DiscoveredLabels: labels.EmptyLabels(),
			Params:           url.Values{"target": []string{"example.com"}},
			Reports:          []*testReport{{scrapeStart, 100 * time.Millisecond, errors.New("failed")}},
			Active:           true,
		},
		{
			Identifier: "blackbox",
			Labels:     labels.EmptyLabels(),
			DiscoveredLabels: labels.FromMap(map[string]string{
				model.SchemeLabel:         "http",
				model.AddressLabel:        "http://dropped.example.com:9115",
				model.MetricsPathLabel:    "/probe",
				model.JobLabel:            "blackbox",
				model.ScrapeIntervalLabel: "30s",
				model.ScrapeTimeoutLabel:  "15s",
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

			querier, err := s.Querier(query.StartTimestampMs, query.EndTimestampMs)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer querier.Close()

			set := querier.Select(r.Context(), false, hints, matchers...)
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

func testEndpoints(t *testing.T, api *API, tr *testTargetRetriever, es storage.ExemplarStorage, testLabelAPI bool) {
	start := time.Unix(0, 0)

	type targetMetadata struct {
		identifier string
		metadata   []scrape.MetricMetadata
	}

	type test struct {
		endpoint              apiFunc
		params                map[string]string
		query                 url.Values
		response              interface{}
		responseLen           int // If nonzero, check only the length; `response` is ignored.
		responseMetadataTotal int
		responseAsJSON        string
		errType               errorType
		sorter                func(interface{})
		metadata              []targetMetadata
		exemplars             []exemplar.QueryResult
		zeroFunc              func(interface{})
	}

	rulesZeroFunc := func(i interface{}) {
		if i != nil {
			v := i.(*RuleDiscovery)
			for _, ruleGroup := range v.RuleGroups {
				ruleGroup.EvaluationTime = float64(0)
				ruleGroup.LastEvaluation = time.Time{}
				for k, rule := range ruleGroup.Rules {
					switch r := rule.(type) {
					case AlertingRule:
						r.LastEvaluation = time.Time{}
						r.EvaluationTime = float64(0)
						r.LastError = ""
						r.Health = "ok"
						for _, alert := range r.Alerts {
							alert.ActiveAt = nil
						}
						ruleGroup.Rules[k] = r
					case RecordingRule:
						r.LastEvaluation = time.Time{}
						r.EvaluationTime = float64(0)
						r.LastError = ""
						r.Health = "ok"
						ruleGroup.Rules[k] = r
					}
				}
			}
		}
	}

	tests := []test{
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"2"},
				"time":  []string{"123.4"},
			},
			response: &QueryData{
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
			response: &QueryData{
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
			response: &QueryData{
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
			response: &QueryData{
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
			response: &QueryData{
				ResultType: parser.ValueTypeMatrix,
				Result: promql.Matrix{
					promql.Series{
						Floats: []promql.FPoint{
							{F: 0, T: timestamp.FromTime(start)},
							{F: 1, T: timestamp.FromTime(start.Add(1 * time.Second))},
							{F: 2, T: timestamp.FromTime(start.Add(2 * time.Second))},
						},
						// No Metric returned - use zero value for comparison.
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
			endpoint: api.formatQuery,
			query: url.Values{
				"query": []string{"foo+bar"},
			},
			response: "foo + bar",
		},
		{
			endpoint: api.formatQuery,
			query: url.Values{
				"query": []string{"invalid_expression/"},
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
				"match[]": []string{`{foo=""}`},
			},
			errType: errorBadData,
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
		// Try to overlap the selected series set as much as possible to test the result de-duplication works well.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric4{foo=~".+o$"}`, `test_metric4{dup=~"^1"}`},
			},
			response: []labels.Labels{
				labels.FromStrings("__name__", "test_metric4", "dup", "1", "foo", "bar"),
				labels.FromStrings("__name__", "test_metric4", "dup", "1", "foo", "boo"),
				labels.FromStrings("__name__", "test_metric4", "foo", "boo"),
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
		// Series request with limit.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{"test_metric1"},
				"limit":   []string{"1"},
			},
			responseLen: 1, // API does not specify which particular value will come back.
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
						DiscoveredLabels:   labels.FromStrings(),
						Labels:             labels.FromStrings("job", "blackbox"),
						ScrapePool:         "blackbox",
						ScrapeURL:          "http://localhost:9115/probe?target=example.com",
						GlobalURL:          "http://localhost:9115/probe?target=example.com",
						Health:             "down",
						LastError:          "failed: missing port in address",
						LastScrape:         scrapeStart,
						LastScrapeDuration: 0.1,
						ScrapeInterval:     "20s",
						ScrapeTimeout:      "10s",
					},
					{
						DiscoveredLabels:   labels.FromStrings(),
						Labels:             labels.FromStrings("job", "test"),
						ScrapePool:         "test",
						ScrapeURL:          "http://example.com:8080/metrics",
						GlobalURL:          "http://example.com:8080/metrics",
						Health:             "up",
						LastError:          "",
						LastScrape:         scrapeStart,
						LastScrapeDuration: 0.07,
						ScrapeInterval:     "15s",
						ScrapeTimeout:      "5s",
					},
				},
				DroppedTargets: []*DroppedTarget{
					{
						DiscoveredLabels: labels.FromStrings(
							"__address__", "http://dropped.example.com:9115",
							"__metrics_path__", "/probe",
							"__scheme__", "http",
							"job", "blackbox",
							"__scrape_interval__", "30s",
							"__scrape_timeout__", "15s",
						),
					},
				},
				DroppedTargetCounts: map[string]int{"blackbox": 1},
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
						DiscoveredLabels:   labels.FromStrings(),
						Labels:             labels.FromStrings("job", "blackbox"),
						ScrapePool:         "blackbox",
						ScrapeURL:          "http://localhost:9115/probe?target=example.com",
						GlobalURL:          "http://localhost:9115/probe?target=example.com",
						Health:             "down",
						LastError:          "failed: missing port in address",
						LastScrape:         scrapeStart,
						LastScrapeDuration: 0.1,
						ScrapeInterval:     "20s",
						ScrapeTimeout:      "10s",
					},
					{
						DiscoveredLabels:   labels.FromStrings(),
						Labels:             labels.FromStrings("job", "test"),
						ScrapePool:         "test",
						ScrapeURL:          "http://example.com:8080/metrics",
						GlobalURL:          "http://example.com:8080/metrics",
						Health:             "up",
						LastError:          "",
						LastScrape:         scrapeStart,
						LastScrapeDuration: 0.07,
						ScrapeInterval:     "15s",
						ScrapeTimeout:      "5s",
					},
				},
				DroppedTargets: []*DroppedTarget{
					{
						DiscoveredLabels: labels.FromStrings(
							"__address__", "http://dropped.example.com:9115",
							"__metrics_path__", "/probe",
							"__scheme__", "http",
							"job", "blackbox",
							"__scrape_interval__", "30s",
							"__scrape_timeout__", "15s",
						),
					},
				},
				DroppedTargetCounts: map[string]int{"blackbox": 1},
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
						DiscoveredLabels:   labels.FromStrings(),
						Labels:             labels.FromStrings("job", "blackbox"),
						ScrapePool:         "blackbox",
						ScrapeURL:          "http://localhost:9115/probe?target=example.com",
						GlobalURL:          "http://localhost:9115/probe?target=example.com",
						Health:             "down",
						LastError:          "failed: missing port in address",
						LastScrape:         scrapeStart,
						LastScrapeDuration: 0.1,
						ScrapeInterval:     "20s",
						ScrapeTimeout:      "10s",
					},
					{
						DiscoveredLabels:   labels.FromStrings(),
						Labels:             labels.FromStrings("job", "test"),
						ScrapePool:         "test",
						ScrapeURL:          "http://example.com:8080/metrics",
						GlobalURL:          "http://example.com:8080/metrics",
						Health:             "up",
						LastError:          "",
						LastScrape:         scrapeStart,
						LastScrapeDuration: 0.07,
						ScrapeInterval:     "15s",
						ScrapeTimeout:      "5s",
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
						DiscoveredLabels: labels.FromStrings(
							"__address__", "http://dropped.example.com:9115",
							"__metrics_path__", "/probe",
							"__scheme__", "http",
							"job", "blackbox",
							"__scrape_interval__", "30s",
							"__scrape_timeout__", "15s",
						),
					},
				},
				DroppedTargetCounts: map[string]int{"blackbox": 1},
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
							Type:   model.MetricTypeGauge,
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
					Type: model.MetricTypeGauge,
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
							Type:   model.MetricTypeGauge,
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
					Type:   model.MetricTypeGauge,
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
							Type:   model.MetricTypeGauge,
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
							Type:   model.MetricTypeGauge,
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
					Type:   model.MetricTypeGauge,
					Unit:   "",
				},
				{
					Target: labels.FromMap(map[string]string{
						"job": "blackbox",
					}),
					Metric: "prometheus_tsdb_storage_blocks_bytes",
					Help:   "The number of bytes that are currently used for local storage by all blocks.",
					Type:   model.MetricTypeGauge,
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
							Type:   model.MetricTypeSummary,
							Help:   "Query timings",
							Unit:   "",
						},
						{
							Metric: "go_info",
							Type:   model.MetricTypeGauge,
							Help:   "Information about the Go environment.",
							Unit:   "",
						},
					},
				},
			},
			response: map[string][]metadata.Metadata{
				"prometheus_engine_query_duration_seconds": {{Type: model.MetricTypeSummary, Help: "Query timings", Unit: ""}},
				"go_info": {{Type: model.MetricTypeGauge, Help: "Information about the Go environment.", Unit: ""}},
			},
			responseAsJSON: `{"prometheus_engine_query_duration_seconds":[{"type":"summary","unit":"",
"help":"Query timings"}], "go_info":[{"type":"gauge","unit":"",
"help":"Information about the Go environment."}]}`,
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
							Type:   model.MetricTypeGauge,
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
							Type:   model.MetricTypeGauge,
							Help:   "Number of OS threads created",
							Unit:   "",
						},
					},
				},
			},
			response: map[string][]metadata.Metadata{
				"go_threads": {{Type: model.MetricTypeGauge, Help: "Number of OS threads created"}},
			},
			responseAsJSON: `{"go_threads": [{"type":"gauge","unit":"",
"help":"Number of OS threads created"}]}`,
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
							Type:   model.MetricTypeGauge,
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
							Type:   model.MetricTypeGauge,
							Help:   "Number of OS threads that were created.",
							Unit:   "",
						},
					},
				},
			},
			response: map[string][]metadata.Metadata{
				"go_threads": {
					{Type: model.MetricTypeGauge, Help: "Number of OS threads created"},
					{Type: model.MetricTypeGauge, Help: "Number of OS threads that were created."},
				},
			},
			responseAsJSON: `{"go_threads": [{"type":"gauge","unit":"",
"help":"Number of OS threads created"},{"type":"gauge","unit":"",
"help":"Number of OS threads that were created."}]}`,
			sorter: func(m interface{}) {
				v := m.(map[string][]metadata.Metadata)["go_threads"]

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
							Type:   model.MetricTypeGauge,
							Help:   "Number of OS threads created",
							Unit:   "",
						},
						{
							Metric: "prometheus_engine_query_duration_seconds",
							Type:   model.MetricTypeSummary,
							Help:   "Query Timings.",
							Unit:   "",
						},
					},
				},
				{
					identifier: "blackbox",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_gc_duration_seconds",
							Type:   model.MetricTypeSummary,
							Help:   "A summary of the GC invocation durations.",
							Unit:   "",
						},
					},
				},
			},
			responseLen: 2,
		},
		// With a limit for the number of metadata per metric.
		{
			endpoint: api.metricMetadata,
			query:    url.Values{"limit_per_metric": []string{"1"}},
			metadata: []targetMetadata{
				{
					identifier: "test",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_threads",
							Type:   model.MetricTypeGauge,
							Help:   "Number of OS threads created",
							Unit:   "",
						},
						{
							Metric: "go_threads",
							Type:   model.MetricTypeGauge,
							Help:   "Repeated metadata",
							Unit:   "",
						},
						{
							Metric: "go_gc_duration_seconds",
							Type:   model.MetricTypeSummary,
							Help:   "A summary of the GC invocation durations.",
							Unit:   "",
						},
					},
				},
			},
			response: map[string][]metadata.Metadata{
				"go_threads": {
					{Type: model.MetricTypeGauge, Help: "Number of OS threads created"},
				},
				"go_gc_duration_seconds": {
					{Type: model.MetricTypeSummary, Help: "A summary of the GC invocation durations."},
				},
			},
			responseAsJSON: `{"go_gc_duration_seconds":[{"help":"A summary of the GC invocation durations.","type":"summary","unit":""}],"go_threads": [{"type":"gauge","unit":"","help":"Number of OS threads created"}]}`,
		},
		// With a limit for the number of metadata per metric and per metric.
		{
			endpoint: api.metricMetadata,
			query:    url.Values{"limit_per_metric": []string{"1"}, "limit": []string{"1"}},
			metadata: []targetMetadata{
				{
					identifier: "test",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_threads",
							Type:   model.MetricTypeGauge,
							Help:   "Number of OS threads created",
							Unit:   "",
						},
						{
							Metric: "go_threads",
							Type:   model.MetricTypeGauge,
							Help:   "Repeated metadata",
							Unit:   "",
						},
						{
							Metric: "go_gc_duration_seconds",
							Type:   model.MetricTypeSummary,
							Help:   "A summary of the GC invocation durations.",
							Unit:   "",
						},
					},
				},
			},
			responseLen:           1,
			responseMetadataTotal: 1,
		},

		// With a limit for the number of metadata per metric and per metric, while having multiple targets.
		{
			endpoint: api.metricMetadata,
			query:    url.Values{"limit_per_metric": []string{"1"}, "limit": []string{"1"}},
			metadata: []targetMetadata{
				{
					identifier: "test",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_threads",
							Type:   model.MetricTypeGauge,
							Help:   "Number of OS threads created",
							Unit:   "",
						},
						{
							Metric: "go_threads",
							Type:   model.MetricTypeGauge,
							Help:   "Repeated metadata",
							Unit:   "",
						},
						{
							Metric: "go_gc_duration_seconds",
							Type:   model.MetricTypeSummary,
							Help:   "A summary of the GC invocation durations.",
							Unit:   "",
						},
					},
				},
				{
					identifier: "secondTarget",
					metadata: []scrape.MetricMetadata{
						{
							Metric: "go_threads",
							Type:   model.MetricTypeGauge,
							Help:   "Number of OS threads created, but from a different target",
							Unit:   "",
						},
						{
							Metric: "go_gc_duration_seconds",
							Type:   model.MetricTypeSummary,
							Help:   "A summary of the GC invocation durations, but from a different target.",
							Unit:   "",
						},
					},
				},
			},
			responseLen:           1,
			responseMetadataTotal: 1,
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
							Type:   model.MetricTypeGauge,
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
							Type:   model.MetricTypeSummary,
							Help:   "A summary of the GC invocation durations.",
							Unit:   "",
						},
						{
							Metric: "go_threads",
							Type:   model.MetricTypeGauge,
							Help:   "Number of OS threads that were created.",
							Unit:   "",
						},
					},
				},
			},
			response: map[string][]metadata.Metadata{
				"go_threads": {
					{Type: model.MetricTypeGauge, Help: "Number of OS threads created"},
					{Type: model.MetricTypeGauge, Help: "Number of OS threads that were created."},
				},
			},
			responseAsJSON: `{"go_threads": [{"type":"gauge","unit":"","help":"Number of OS threads created"},{"type":"gauge","unit":"","help":"Number of OS threads that were created."}]}`,
			sorter: func(m interface{}) {
				v := m.(map[string][]metadata.Metadata)["go_threads"]

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
							Type:   model.MetricTypeGauge,
							Help:   "Number of OS threads created",
							Unit:   "",
						},
					},
				},
			},
			response: map[string][]metadata.Metadata{},
		},
		// With no available metadata.
		{
			endpoint: api.metricMetadata,
			response: map[string][]metadata.Metadata{},
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
				Alerts: []*Alert{
					{
						Labels:      labels.FromStrings("alertname", "test_metric5", "name", "tm5"),
						Annotations: labels.Labels{},
						State:       "pending",
						Value:       "1e+00",
					},
				},
			},
			zeroFunc: func(i interface{}) {
				if i != nil {
					v := i.(*AlertDiscovery)
					for _, alert := range v.Alerts {
						alert.ActiveAt = nil
					}
				}
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
						Limit:    0,
						Rules: []Rule{
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric3",
								Query:       "absent(test_metric3) != 1",
								Duration:    1,
								Labels:      labels.Labels{},
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "ok",
								Type:        "alerting",
							},
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric4",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.Labels{},
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "ok",
								Type:        "alerting",
							},
							AlertingRule{
								State:       "pending",
								Name:        "test_metric5",
								Query:       "vector(1)",
								Duration:    1,
								Labels:      labels.FromStrings("name", "tm5"),
								Annotations: labels.Labels{},
								Alerts: []*Alert{
									{
										Labels:      labels.FromStrings("alertname", "test_metric5", "name", "tm5"),
										Annotations: labels.Labels{},
										State:       "pending",
										Value:       "1e+00",
									},
								},
								Health: "ok",
								Type:   "alerting",
							},
							RecordingRule{
								Name:   "recording-rule-1",
								Query:  "vector(1)",
								Labels: labels.Labels{},
								Health: "ok",
								Type:   "recording",
							},
						},
					},
				},
			},
			zeroFunc: rulesZeroFunc,
		},
		{
			endpoint: api.rules,
			query: url.Values{
				"exclude_alerts": []string{"true"},
			},
			response: &RuleDiscovery{
				RuleGroups: []*RuleGroup{
					{
						Name:     "grp",
						File:     "/path/to/file",
						Interval: 1,
						Limit:    0,
						Rules: []Rule{
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric3",
								Query:       "absent(test_metric3) != 1",
								Duration:    1,
								Labels:      labels.Labels{},
								Annotations: labels.Labels{},
								Alerts:      nil,
								Health:      "ok",
								Type:        "alerting",
							},
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric4",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.Labels{},
								Annotations: labels.Labels{},
								Alerts:      nil,
								Health:      "ok",
								Type:        "alerting",
							},
							AlertingRule{
								State:       "pending",
								Name:        "test_metric5",
								Query:       "vector(1)",
								Duration:    1,
								Labels:      labels.FromStrings("name", "tm5"),
								Annotations: labels.Labels{},
								Alerts:      nil,
								Health:      "ok",
								Type:        "alerting",
							},
							RecordingRule{
								Name:   "recording-rule-1",
								Query:  "vector(1)",
								Labels: labels.Labels{},
								Health: "ok",
								Type:   "recording",
							},
						},
					},
				},
			},
			zeroFunc: rulesZeroFunc,
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
						Limit:    0,
						Rules: []Rule{
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric3",
								Query:       "absent(test_metric3) != 1",
								Duration:    1,
								Labels:      labels.Labels{},
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "ok",
								Type:        "alerting",
							},
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric4",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.Labels{},
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "ok",
								Type:        "alerting",
							},
							AlertingRule{
								State:       "pending",
								Name:        "test_metric5",
								Query:       "vector(1)",
								Duration:    1,
								Labels:      labels.FromStrings("name", "tm5"),
								Annotations: labels.Labels{},
								Alerts: []*Alert{
									{
										Labels:      labels.FromStrings("alertname", "test_metric5", "name", "tm5"),
										Annotations: labels.Labels{},
										State:       "pending",
										Value:       "1e+00",
									},
								},
								Health: "ok",
								Type:   "alerting",
							},
						},
					},
				},
			},
			zeroFunc: rulesZeroFunc,
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
						Limit:    0,
						Rules: []Rule{
							RecordingRule{
								Name:   "recording-rule-1",
								Query:  "vector(1)",
								Labels: labels.Labels{},
								Health: "ok",
								Type:   "recording",
							},
						},
					},
				},
			},
			zeroFunc: rulesZeroFunc,
		},
		{
			endpoint: api.rules,
			query:    url.Values{"rule_name[]": []string{"test_metric4"}},
			response: &RuleDiscovery{
				RuleGroups: []*RuleGroup{
					{
						Name:     "grp",
						File:     "/path/to/file",
						Interval: 1,
						Limit:    0,
						Rules: []Rule{
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric4",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.Labels{},
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "ok",
								Type:        "alerting",
							},
						},
					},
				},
			},
			zeroFunc: rulesZeroFunc,
		},
		{
			endpoint: api.rules,
			query:    url.Values{"rule_group[]": []string{"respond-with-nothing"}},
			response: &RuleDiscovery{RuleGroups: []*RuleGroup{}},
		},
		{
			endpoint: api.rules,
			query:    url.Values{"file[]": []string{"/path/to/file"}, "rule_name[]": []string{"test_metric4"}},
			response: &RuleDiscovery{
				RuleGroups: []*RuleGroup{
					{
						Name:     "grp",
						File:     "/path/to/file",
						Interval: 1,
						Limit:    0,
						Rules: []Rule{
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric4",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.Labels{},
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "ok",
								Type:        "alerting",
							},
						},
					},
				},
			},
			zeroFunc: rulesZeroFunc,
		},
		{
			endpoint: api.queryExemplars,
			query: url.Values{
				"query": []string{`test_metric3{foo="boo"} - test_metric4{foo="bar"}`},
				"start": []string{"0"},
				"end":   []string{"4"},
			},
			// Note extra integer length of timestamps for exemplars because of millisecond preservation
			// of timestamps within Prometheus (see timestamp package).

			response: []exemplar.QueryResult{
				{
					SeriesLabels: labels.FromStrings("__name__", "test_metric3", "foo", "boo", "dup", "1"),
					Exemplars: []exemplar.Exemplar{
						{
							Labels: labels.FromStrings("id", "abc"),
							Value:  10,
							Ts:     timestamp.FromTime(start.Add(2 * time.Second)),
						},
					},
				},
				{
					SeriesLabels: labels.FromStrings("__name__", "test_metric4", "foo", "bar", "dup", "1"),
					Exemplars: []exemplar.Exemplar{
						{
							Labels: labels.FromStrings("id", "lul"),
							Value:  10,
							Ts:     timestamp.FromTime(start.Add(4 * time.Second)),
						},
					},
				},
			},
		},
		{
			endpoint: api.queryExemplars,
			query: url.Values{
				"query": []string{`{foo="boo"}`},
				"start": []string{"4"},
				"end":   []string{"4.1"},
			},
			response: []exemplar.QueryResult{
				{
					SeriesLabels: labels.FromStrings("__name__", "test_metric3", "foo", "boo", "dup", "1"),
					Exemplars: []exemplar.Exemplar{
						{
							Labels: labels.FromStrings("id", "abc2"),
							Value:  10,
							Ts:     4053,
						},
					},
				},
			},
		},
		{
			endpoint: api.queryExemplars,
			query: url.Values{
				"query": []string{`{foo="boo"}`},
			},
			response: []exemplar.QueryResult{
				{
					SeriesLabels: labels.FromStrings("__name__", "test_metric3", "foo", "boo", "dup", "1"),
					Exemplars: []exemplar.Exemplar{
						{
							Labels: labels.FromStrings("id", "abc"),
							Value:  10,
							Ts:     2000,
						},
						{
							Labels: labels.FromStrings("id", "abc2"),
							Value:  10,
							Ts:     4053,
						},
					},
				},
			},
		},
		{
			endpoint: api.queryExemplars,
			query: url.Values{
				"query": []string{`{__name__="test_metric5"}`},
			},
			response: []exemplar.QueryResult{},
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
					"test_metric3",
					"test_metric4",
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
			// Label values with bad matchers.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"match[]": []string{`{foo=""`, `test_metric2`},
				},
				errType: errorBadData,
			},
			// Label values with empty matchers.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"match[]": []string{`{foo=""}`},
				},
				errType: errorBadData,
			},
			// Label values with matcher.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"match[]": []string{`test_metric2`},
				},
				response: []string{
					"boo",
				},
			},
			// Label values with matcher.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"match[]": []string{`test_metric1`},
				},
				response: []string{
					"bar",
					"boo",
				},
			},
			// Label values with matcher using label filter.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"match[]": []string{`test_metric1{foo="bar"}`},
				},
				response: []string{
					"bar",
				},
			},
			// Label values with matcher and time range.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"match[]": []string{`test_metric1`},
					"start":   []string{"1"},
					"end":     []string{"100000000"},
				},
				response: []string{
					"bar",
					"boo",
				},
			},
			// Try to overlap the selected series set as much as possible to test that the value de-duplication works.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"match[]": []string{`test_metric4{dup=~"^1"}`, `test_metric4{foo=~".+o$"}`},
				},
				response: []string{
					"bar",
					"boo",
				},
			},
			// Label values with limit.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "__name__",
				},
				query: url.Values{
					"limit": []string{"2"},
				},
				responseLen: 2, // API does not specify which particular values will come back.
			},
			// Label names.
			{
				endpoint: api.labelNames,
				response: []string{"__name__", "dup", "foo"},
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
				response: []string{"__name__", "dup", "foo"},
			},
			// Start before Label names, end within Label names.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"start": []string{"-1"},
					"end":   []string{"10"},
				},
				response: []string{"__name__", "dup", "foo"},
			},

			// Start before Label names starts, end after Label names ends.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"start": []string{"-1"},
					"end":   []string{"100000"},
				},
				response: []string{"__name__", "dup", "foo"},
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
				response: []string{"__name__", "dup", "foo"},
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
				response: []string{"__name__", "dup", "foo"},
			},
			// Only provide End within Label names, don't provide a start time.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"end": []string{"20"},
				},
				response: []string{"__name__", "dup", "foo"},
			},
			// Label names with bad matchers.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"match[]": []string{`{foo=""`, `test_metric2`},
				},
				errType: errorBadData,
			},
			// Label values with empty matchers.
			{
				endpoint: api.labelNames,
				params: map[string]string{
					"name": "foo",
				},
				query: url.Values{
					"match[]": []string{`{foo=""}`},
				},
				errType: errorBadData,
			},
			// Label names with matcher.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"match[]": []string{`test_metric2`},
				},
				response: []string{"__name__", "foo"},
			},
			// Label names with matcher.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"match[]": []string{`test_metric3`},
				},
				response: []string{"__name__", "dup", "foo"},
			},
			// Label names with matcher using label filter.
			// There is no matching series.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"match[]": []string{`test_metric1{foo="test"}`},
				},
				response: []string{},
			},
			// Label names with matcher and time range.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"match[]": []string{`test_metric2`},
					"start":   []string{"1"},
					"end":     []string{"100000000"},
				},
				response: []string{"__name__", "foo"},
			},
			// Label names with limit.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"limit": []string{"2"},
				},
				responseLen: 2, // API does not specify which particular values will come back.
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
		t.Run(fmt.Sprintf("run %d %s %q", i, describeAPIFunc(test.endpoint), test.query.Encode()), func(t *testing.T) {
			for _, method := range methods(test.endpoint) {
				t.Run(method, func(t *testing.T) {
					// Build a context with the correct request params.
					ctx := context.Background()
					for p, v := range test.params {
						ctx = route.WithParam(ctx, p, v)
					}

					req, err := request(method, test.query)
					require.NoError(t, err)

					tr.ResetMetadataStore()
					for _, tm := range test.metadata {
						tr.SetMetadataStoreForTargets(tm.identifier, &testMetaStore{Metadata: tm.metadata})
					}

					for _, te := range test.exemplars {
						for _, e := range te.Exemplars {
							_, err := es.AppendExemplar(0, te.SeriesLabels, e)
							require.NoError(t, err)
						}
					}

					res := test.endpoint(req.WithContext(ctx))
					assertAPIError(t, res.err, test.errType)

					if test.sorter != nil {
						test.sorter(res.data)
					}

					if test.responseLen != 0 {
						assertAPIResponseLength(t, res.data, test.responseLen)
						if test.responseMetadataTotal != 0 {
							assertAPIResponseMetadataLen(t, res.data, test.responseMetadataTotal)
						}
					} else {
						if test.zeroFunc != nil {
							test.zeroFunc(res.data)
						}
						assertAPIResponse(t, res.data, test.response)
					}

					if test.responseAsJSON != "" {
						s, err := json.Marshal(res.data)
						require.NoError(t, err)
						require.JSONEq(t, test.responseAsJSON, string(s))
					}
				})
			}
		})
	}
}

func describeAPIFunc(f apiFunc) string {
	name := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
	return strings.Split(name[strings.LastIndex(name, ".")+1:], "-")[0]
}

func assertAPIError(t *testing.T, got *apiError, exp errorType) {
	t.Helper()

	if exp == errorNone {
		require.Nil(t, got)
	} else {
		require.NotNil(t, got)
		require.Equal(t, exp, got.typ, "(%q)", got)
	}
}

func assertAPIResponse(t *testing.T, got, exp interface{}) {
	t.Helper()

	testutil.RequireEqual(t, exp, got)
}

func assertAPIResponseLength(t *testing.T, got interface{}, expLen int) {
	t.Helper()

	gotLen := reflect.ValueOf(got).Len()
	require.Equal(t, expLen, gotLen, "Response length does not match")
}

func assertAPIResponseMetadataLen(t *testing.T, got interface{}, expLen int) {
	t.Helper()

	var gotLen int
	response := got.(map[string][]metadata.Metadata)
	for _, m := range response {
		gotLen += len(m)
	}

	require.Equal(t, expLen, gotLen, "Amount of metadata in the response does not match")
}

type fakeDB struct {
	err error
}

func (f *fakeDB) CleanTombstones() error                                         { return f.err }
func (f *fakeDB) Delete(context.Context, int64, int64, ...*labels.Matcher) error { return f.err }
func (f *fakeDB) Snapshot(string, bool) error                                    { return f.err }
func (f *fakeDB) Stats(statsByLabelName string, limit int) (_ *tsdb.Stats, retErr error) {
	dbDir, err := os.MkdirTemp("", "tsdb-api-ready")
	if err != nil {
		return nil, err
	}
	defer func() {
		err := os.RemoveAll(dbDir)
		if retErr != nil {
			retErr = err
		}
	}()
	opts := tsdb.DefaultHeadOptions()
	opts.ChunkRange = 1000
	h, _ := tsdb.NewHead(nil, nil, nil, nil, opts, nil)
	return h.Stats(statsByLabelName, limit), nil
}

func (f *fakeDB) WALReplayStatus() (tsdb.WALReplayStatus, error) {
	return tsdb.WALReplayStatus{}, nil
}

func TestAdminEndpoints(t *testing.T) {
	tsdb, tsdbWithError, tsdbNotReady := &fakeDB{}, &fakeDB{err: errors.New("some error")}, &fakeDB{err: fmt.Errorf("wrap: %w", tsdb.ErrNotReady)}
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
			dir := t.TempDir()

			api := &API{
				db:          tc.db,
				dbDir:       dir,
				ready:       func(f http.HandlerFunc) http.HandlerFunc { return f },
				enableAdmin: tc.enableAdmin,
			}

			endpoint := tc.endpoint(api)
			req, err := http.NewRequest(tc.method, fmt.Sprintf("?%s", tc.values.Encode()), nil)
			require.NoError(t, err)

			res := setUnavailStatusOnTSDBNotReady(endpoint(req))
			assertAPIError(t, res.err, tc.errType)
		})
	}
}

func TestRespondSuccess(t *testing.T) {
	api := API{
		logger: log.NewNopLogger(),
	}

	api.ClearCodecs()
	api.InstallCodec(JSONCodec{})
	api.InstallCodec(&testCodec{contentType: MIMEType{"test", "cannot-encode"}, canEncode: false})
	api.InstallCodec(&testCodec{contentType: MIMEType{"test", "can-encode"}, canEncode: true})
	api.InstallCodec(&testCodec{contentType: MIMEType{"test", "can-encode-2"}, canEncode: true})

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.respond(w, r, "test", nil, "")
	}))
	defer s.Close()

	for _, tc := range []struct {
		name                string
		acceptHeader        string
		expectedContentType string
		expectedBody        string
	}{
		{
			name:                "no Accept header",
			expectedContentType: "application/json",
			expectedBody:        `{"status":"success","data":"test"}`,
		},
		{
			name:                "Accept header with single content type which is suitable",
			acceptHeader:        "test/can-encode",
			expectedContentType: "test/can-encode",
			expectedBody:        `response from test/can-encode codec`,
		},
		{
			name:                "Accept header with single content type which is not available",
			acceptHeader:        "test/not-registered",
			expectedContentType: "application/json",
			expectedBody:        `{"status":"success","data":"test"}`,
		},
		{
			name:                "Accept header with single content type which cannot encode the response payload",
			acceptHeader:        "test/cannot-encode",
			expectedContentType: "application/json",
			expectedBody:        `{"status":"success","data":"test"}`,
		},
		{
			name:                "Accept header with multiple content types, all of which are suitable",
			acceptHeader:        "test/can-encode, test/can-encode-2",
			expectedContentType: "test/can-encode",
			expectedBody:        `response from test/can-encode codec`,
		},
		{
			name:                "Accept header with multiple content types, only one of which is available",
			acceptHeader:        "test/not-registered, test/can-encode",
			expectedContentType: "test/can-encode",
			expectedBody:        `response from test/can-encode codec`,
		},
		{
			name:                "Accept header with multiple content types, only one of which can encode the response payload",
			acceptHeader:        "test/cannot-encode, test/can-encode",
			expectedContentType: "test/can-encode",
			expectedBody:        `response from test/can-encode codec`,
		},
		{
			name:                "Accept header with multiple content types, none of which are available",
			acceptHeader:        "test/not-registered, test/also-not-registered",
			expectedContentType: "application/json",
			expectedBody:        `{"status":"success","data":"test"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, s.URL, nil)
			require.NoError(t, err)

			if tc.acceptHeader != "" {
				req.Header.Set("Accept", tc.acceptHeader)
			}

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			body, err := io.ReadAll(resp.Body)
			defer resp.Body.Close()
			require.NoError(t, err)

			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, tc.expectedContentType, resp.Header.Get("Content-Type"))
			require.Equal(t, tc.expectedBody, string(body))
		})
	}
}

func TestRespondSuccess_DefaultCodecCannotEncodeResponse(t *testing.T) {
	api := API{
		logger: log.NewNopLogger(),
	}

	api.ClearCodecs()
	api.InstallCodec(&testCodec{contentType: MIMEType{"application", "default-format"}, canEncode: false})

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.respond(w, r, "test", nil, "")
	}))
	defer s.Close()

	req, err := http.NewRequest(http.MethodGet, s.URL, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	require.NoError(t, err)

	require.Equal(t, http.StatusNotAcceptable, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Equal(t, `{"status":"error","errorType":"not_acceptable","error":"cannot encode response as application/default-format"}`, string(body))
}

func TestRespondError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api := API{}
		api.respondError(w, &apiError{errorTimeout, errors.New("message")}, "test")
	}))
	defer s.Close()

	resp, err := http.Get(s.URL)
	require.NoError(t, err, "Error on test request")
	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	require.NoError(t, err, "Error reading response body")
	want, have := http.StatusServiceUnavailable, resp.StatusCode
	require.Equal(t, want, have, "Return code %d expected in error response but got %d", want, have)
	h := resp.Header.Get("Content-Type")
	require.Equal(t, "application/json", h, "Expected Content-Type %q but got %q", "application/json", h)

	var res Response
	err = json.Unmarshal(body, &res)
	require.NoError(t, err, "Error unmarshaling JSON body")

	exp := &Response{
		Status:    statusError,
		Data:      "test",
		ErrorType: errorTimeout,
		Error:     "message",
	}
	require.Equal(t, exp, &res)
}

func TestParseTimeParam(t *testing.T) {
	type resultType struct {
		asTime  time.Time
		asError func() error
	}

	ts, err := parseTime("1582468023986")
	require.NoError(t, err)

	tests := []struct {
		paramName    string
		paramValue   string
		defaultValue time.Time
		result       resultType
	}{
		{ // When data is valid.
			paramName:    "start",
			paramValue:   "1582468023986",
			defaultValue: MinTime,
			result: resultType{
				asTime:  ts,
				asError: nil,
			},
		},
		{ // When data is empty string.
			paramName:    "end",
			paramValue:   "",
			defaultValue: MaxTime,
			result: resultType{
				asTime:  MaxTime,
				asError: nil,
			},
		},
		{ // When data is not valid.
			paramName:    "foo",
			paramValue:   "baz",
			defaultValue: MaxTime,
			result: resultType{
				asTime: time.Time{},
				asError: func() error {
					_, err := parseTime("baz")
					return fmt.Errorf("Invalid time value for '%s': %w", "foo", err)
				},
			},
		},
	}

	for _, test := range tests {
		req, err := http.NewRequest("GET", "localhost:42/foo?"+test.paramName+"="+test.paramValue, nil)
		require.NoError(t, err)

		result := test.result
		asTime, err := parseTimeParam(req, test.paramName, test.defaultValue)

		if err != nil {
			require.EqualError(t, err, result.asError().Error())
		} else {
			require.True(t, asTime.Equal(result.asTime), "time as return value: %s not parsed correctly. Expected %s. Actual %s", test.paramValue, result.asTime, asTime)
		}
	}
}

func TestParseTime(t *testing.T) {
	ts, err := time.Parse(time.RFC3339Nano, "2015-06-03T13:21:58.555Z")
	if err != nil {
		panic(err)
	}

	tests := []struct {
		input  string
		fail   bool
		result time.Time
	}{
		{
			input: "",
			fail:  true,
		},
		{
			input: "abc",
			fail:  true,
		},
		{
			input: "30s",
			fail:  true,
		},
		{
			input:  "123",
			result: time.Unix(123, 0),
		},
		{
			input:  "123.123",
			result: time.Unix(123, 123000000),
		},
		{
			input:  "2015-06-03T13:21:58.555Z",
			result: ts,
		},
		{
			input:  "2015-06-03T14:21:58.555+01:00",
			result: ts,
		},
		{
			// Test float rounding.
			input:  "1543578564.705",
			result: time.Unix(1543578564, 705*1e6),
		},
		{
			input:  MinTime.Format(time.RFC3339Nano),
			result: MinTime,
		},
		{
			input:  MaxTime.Format(time.RFC3339Nano),
			result: MaxTime,
		},
	}

	for _, test := range tests {
		ts, err := parseTime(test.input)
		if !test.fail {
			require.NoError(t, err, "Unexpected error for %q", test.input)
			require.NotNil(t, ts)
			require.True(t, ts.Equal(test.result), "Expected time %v for input %q but got %v", test.result, test.input, ts)
			continue
		}
		require.Error(t, err, "Expected error for %q but got none", test.input)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
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
		if !test.fail {
			require.NoError(t, err, "Unexpected error for %q", test.input)
			require.Equal(t, test.result, d, "Expected duration %v for input %q but got %v", test.result, test.input, d)
			continue
		}
		require.Error(t, err, "Expected error for %q but got none", test.input)
	}
}

func TestOptionsMethod(t *testing.T) {
	r := route.New()
	api := &API{ready: func(f http.HandlerFunc) http.HandlerFunc { return f }}
	api.Register(r)

	s := httptest.NewServer(r)
	defer s.Close()

	req, err := http.NewRequest("OPTIONS", s.URL+"/any_path", nil)
	require.NoError(t, err, "Error creating OPTIONS request")
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err, "Error executing OPTIONS request")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
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
			errType:  errorNone,
		},
		{
			db:       tsdb,
			endpoint: tsdbStatusAPI,
			values:   map[string][]string{"limit": {"20"}},
			errType:  errorNone,
		},
		{
			db:       tsdb,
			endpoint: tsdbStatusAPI,
			values:   map[string][]string{"limit": {"0"}},
			errType:  errorBadData,
		},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			api := &API{db: tc.db, gatherer: prometheus.DefaultGatherer}
			endpoint := tc.endpoint(api)
			req, err := http.NewRequest(tc.method, fmt.Sprintf("?%s", tc.values.Encode()), nil)
			require.NoError(t, err, "Error when creating test request")
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
			err:      fmt.Errorf("wrapped: %w", promql.ErrStorage{Err: errors.New("storage error")}),
			expected: errorInternal,
		}, {
			err:      promql.ErrQueryTimeout("timeout error"),
			expected: errorTimeout,
		}, {
			err:      fmt.Errorf("wrapped: %w", promql.ErrQueryTimeout("timeout error")),
			expected: errorTimeout,
		}, {
			err:      promql.ErrQueryCanceled("canceled error"),
			expected: errorCanceled,
		}, {
			err:      fmt.Errorf("wrapped: %w", promql.ErrQueryCanceled("canceled error")),
			expected: errorCanceled,
		}, {
			err:      errors.New("exec error"),
			expected: errorExec,
		},
	}

	for ix, c := range cases {
		actual := returnAPIError(c.err)
		require.Error(t, actual, ix)
		require.Equal(t, c.expected, actual.typ, ix)
	}
}

// This is a global to avoid the benchmark being optimized away.
var testResponseWriter = httptest.ResponseRecorder{}

func BenchmarkRespond(b *testing.B) {
	points := []promql.FPoint{}
	for i := 0; i < 10000; i++ {
		points = append(points, promql.FPoint{F: float64(i * 1000000), T: int64(i)})
	}
	matrix := promql.Matrix{}
	for i := 0; i < 1000; i++ {
		matrix = append(matrix, promql.Series{
			Metric: labels.FromStrings("__name__", fmt.Sprintf("series%v", i),
				"label", fmt.Sprintf("series%v", i),
				"label2", fmt.Sprintf("series%v", i)),
			Floats: points[:10],
		})
	}
	series := []labels.Labels{}
	for i := 0; i < 1000; i++ {
		series = append(series, labels.FromStrings("__name__", fmt.Sprintf("series%v", i),
			"label", fmt.Sprintf("series%v", i),
			"label2", fmt.Sprintf("series%v", i)))
	}

	cases := []struct {
		name     string
		response interface{}
	}{
		{name: "10000 points no labels", response: &QueryData{
			ResultType: parser.ValueTypeMatrix,
			Result: promql.Matrix{
				promql.Series{
					Floats: points,
					Metric: labels.EmptyLabels(),
				},
			},
		}},
		{name: "1000 labels", response: series},
		{name: "1000 series 10 points", response: &QueryData{
			ResultType: parser.ValueTypeMatrix,
			Result:     matrix,
		}},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			request, err := http.NewRequest(http.MethodGet, "/does-not-matter", nil)
			require.NoError(b, err)
			b.ResetTimer()
			api := API{}
			api.InstallCodec(JSONCodec{})
			for n := 0; n < b.N; n++ {
				api.respond(&testResponseWriter, request, c.response, nil, "")
			}
		})
	}
}

func TestGetGlobalURL(t *testing.T) {
	mustParseURL := func(t *testing.T, u string) *url.URL {
		parsed, err := url.Parse(u)
		require.NoError(t, err)
		return parsed
	}

	testcases := []struct {
		input    *url.URL
		opts     GlobalURLOptions
		expected *url.URL
		errorful bool
	}{
		{
			mustParseURL(t, "http://127.0.0.1:9090"),
			GlobalURLOptions{
				ListenAddress: "127.0.0.1:9090",
				Host:          "127.0.0.1:9090",
				Scheme:        "http",
			},
			mustParseURL(t, "http://127.0.0.1:9090"),
			false,
		},
		{
			mustParseURL(t, "http://127.0.0.1:9090"),
			GlobalURLOptions{
				ListenAddress: "127.0.0.1:9090",
				Host:          "prometheus.io",
				Scheme:        "https",
			},
			mustParseURL(t, "https://prometheus.io"),
			false,
		},
		{
			mustParseURL(t, "http://exemple.com"),
			GlobalURLOptions{
				ListenAddress: "127.0.0.1:9090",
				Host:          "prometheus.io",
				Scheme:        "https",
			},
			mustParseURL(t, "http://exemple.com"),
			false,
		},
		{
			mustParseURL(t, "http://localhost:8080"),
			GlobalURLOptions{
				ListenAddress: "127.0.0.1:9090",
				Host:          "prometheus.io",
				Scheme:        "https",
			},
			mustParseURL(t, "http://prometheus.io:8080"),
			false,
		},
		{
			mustParseURL(t, "http://[::1]:8080"),
			GlobalURLOptions{
				ListenAddress: "127.0.0.1:9090",
				Host:          "prometheus.io",
				Scheme:        "https",
			},
			mustParseURL(t, "http://prometheus.io:8080"),
			false,
		},
		{
			mustParseURL(t, "http://localhost"),
			GlobalURLOptions{
				ListenAddress: "127.0.0.1:9090",
				Host:          "prometheus.io",
				Scheme:        "https",
			},
			mustParseURL(t, "http://prometheus.io"),
			false,
		},
		{
			mustParseURL(t, "http://localhost:9091"),
			GlobalURLOptions{
				ListenAddress: "[::1]:9090",
				Host:          "[::1]",
				Scheme:        "https",
			},
			mustParseURL(t, "http://[::1]:9091"),
			false,
		},
		{
			mustParseURL(t, "http://localhost:9091"),
			GlobalURLOptions{
				ListenAddress: "[::1]:9090",
				Host:          "[::1]:9090",
				Scheme:        "https",
			},
			mustParseURL(t, "http://[::1]:9091"),
			false,
		},
	}

	for i, tc := range testcases {
		t.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
			output, err := getGlobalURL(tc.input, tc.opts)
			if tc.errorful {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, output)
		})
	}
}

type testCodec struct {
	contentType MIMEType
	canEncode   bool
}

func (t *testCodec) ContentType() MIMEType {
	return t.contentType
}

func (t *testCodec) CanEncode(_ *Response) bool {
	return t.canEncode
}

func (t *testCodec) Encode(_ *Response) ([]byte, error) {
	return []byte(fmt.Sprintf("response from %v codec", t.contentType)), nil
}

func TestExtractQueryOpts(t *testing.T) {
	tests := []struct {
		name   string
		form   url.Values
		expect promql.QueryOpts
		err    error
	}{
		{
			name: "with stats all",
			form: url.Values{
				"stats": []string{"all"},
			},
			expect: promql.NewPrometheusQueryOpts(true, 0),

			err: nil,
		},
		{
			name: "with stats none",
			form: url.Values{
				"stats": []string{"none"},
			},
			expect: promql.NewPrometheusQueryOpts(false, 0),
			err:    nil,
		},
		{
			name: "with lookback delta",
			form: url.Values{
				"stats":          []string{"all"},
				"lookback_delta": []string{"30s"},
			},
			expect: promql.NewPrometheusQueryOpts(true, 30*time.Second),
			err:    nil,
		},
		{
			name: "with invalid lookback delta",
			form: url.Values{
				"lookback_delta": []string{"invalid"},
			},
			expect: nil,
			err:    errors.New(`error parsing lookback delta duration: cannot parse "invalid" to a valid duration`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := &http.Request{Form: test.form}
			opts, err := extractQueryOpts(req)
			require.Equal(t, test.expect, opts)
			if test.err == nil {
				require.NoError(t, err)
			} else {
				require.Equal(t, test.err.Error(), err.Error())
			}
		})
	}
}

// Test query timeout parameter.
func TestQueryTimeout(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 1m
			test_metric1{foo="bar"} 0+100x100
	`)
	t.Cleanup(func() {
		_ = storage.Close()
	})

	now := time.Now()

	for _, tc := range []struct {
		name   string
		method string
	}{
		{
			name:   "GET method",
			method: http.MethodGet,
		},
		{
			name:   "POST method",
			method: http.MethodPost,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &fakeEngine{}
			api := &API{
				Queryable:             storage,
				QueryEngine:           engine,
				ExemplarQueryable:     storage.ExemplarQueryable(),
				alertmanagerRetriever: testAlertmanagerRetriever{}.toFactory(),
				flagsMap:              sampleFlagMap,
				now:                   func() time.Time { return now },
				config:                func() config.Config { return samplePrometheusCfg },
				ready:                 func(f http.HandlerFunc) http.HandlerFunc { return f },
			}

			query := url.Values{
				"query":   []string{"2"},
				"timeout": []string{"1s"},
			}
			ctx := context.Background()
			req, err := http.NewRequest(tc.method, fmt.Sprintf("http://example.com?%s", query.Encode()), nil)
			require.NoError(t, err)
			req.RemoteAddr = "127.0.0.1:20201"

			res := api.query(req.WithContext(ctx))
			assertAPIError(t, res.err, errorNone)

			require.Len(t, engine.query.execCalls, 1)
			deadline, ok := engine.query.execCalls[0].Deadline()
			require.True(t, ok)
			require.Equal(t, now.Add(time.Second), deadline)
		})
	}
}

// fakeEngine is a fake QueryEngine implementation.
type fakeEngine struct {
	query fakeQuery
}

func (e *fakeEngine) SetQueryLogger(promql.QueryLogger) {}

func (e *fakeEngine) NewInstantQuery(ctx context.Context, q storage.Queryable, opts promql.QueryOpts, qs string, ts time.Time) (promql.Query, error) {
	return &e.query, nil
}

func (e *fakeEngine) NewRangeQuery(ctx context.Context, q storage.Queryable, opts promql.QueryOpts, qs string, start, end time.Time, interval time.Duration) (promql.Query, error) {
	return &e.query, nil
}

// fakeQuery is a fake Query implementation.
type fakeQuery struct {
	query     string
	execCalls []context.Context
}

func (q *fakeQuery) Exec(ctx context.Context) *promql.Result {
	q.execCalls = append(q.execCalls, ctx)
	return &promql.Result{
		Value: &parser.StringLiteral{
			Val: "test",
		},
	}
}

func (q *fakeQuery) Close() {}

func (q *fakeQuery) Statement() parser.Statement {
	return nil
}

func (q *fakeQuery) Stats() *stats.Statistics {
	return nil
}

func (q *fakeQuery) Cancel() {}

func (q *fakeQuery) String() string {
	return q.query
}
