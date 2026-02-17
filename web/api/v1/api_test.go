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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/grafana/regexp"
	jsoniter "github.com/json-iterator/go"
	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/route"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/stats"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/prometheus/prometheus/util/testutil"
)

var testParser = parser.NewParser(parser.Options{})

func testEngine(t *testing.T) *promql.Engine {
	t.Helper()
	return promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
		Logger:                   nil,
		Reg:                      nil,
		MaxSamples:               10000,
		Timeout:                  100 * time.Second,
		NoStepSubqueryIntervalFn: func(int64) int64 { return 60 * 1000 },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
		EnablePerStepStats:       true,
	})
}

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
		if metric == m.MetricFamily {
			return m, true
		}
	}

	return scrape.MetricMetadata{}, false
}

func (*testMetaStore) SizeMetadata() int   { return 0 }
func (*testMetaStore) LengthMetadata() int { return 0 }

// testTargetRetriever represents a list of targets to scrape.
// It is used to represent targets as part of test cases.
type testTargetRetriever struct {
	activeTargets  map[string][]*scrape.Target
	droppedTargets map[string][]*scrape.Target
}

type testTargetParams struct {
	Identifier   string
	Labels       labels.Labels
	targetLabels model.LabelSet
	Params       url.Values
	Reports      []*testReport
	Active       bool
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
		nt := scrape.NewTarget(t.Labels, &config.ScrapeConfig{Params: t.Params}, t.targetLabels, nil)

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

func (testTargetRetriever) ScrapePoolConfig(pool string) (*config.ScrapeConfig, error) {
	cfg := &config.ScrapeConfig{
		RelabelConfigs: []*relabel.Config{
			{
				Action:               relabel.Replace,
				Replacement:          "example.com:443",
				TargetLabel:          "__address__",
				Regex:                relabel.MustNewRegexp(""),
				NameValidationScheme: model.LegacyValidation,
			},
			{
				Action:       relabel.Drop,
				SourceLabels: []model.LabelName{"__address__"},
				Regex:        relabel.MustNewRegexp(`example\.com:.*`),
			},
		},
	}
	if pool == "testpool3" {
		cfg.RelabelConfigs = append(cfg.RelabelConfigs, &relabel.Config{
			Action:      relabel.Replace,
			TargetLabel: "job",
			Regex:       relabel.MustNewRegexp(".*"),
			Replacement: "should_not_apply",
		})
	}
	return cfg, nil
}

func (t *testTargetRetriever) SetMetadataStoreForTargets(identifier string, metadata scrape.MetricMetadataStore) error {
	targets, ok := t.activeTargets[identifier]
	if !ok {
		return fmt.Errorf("no active target for %v", identifier)
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

func (testAlertmanagerRetriever) Alertmanagers() []*url.URL {
	return []*url.URL{
		{
			Scheme: "http",
			Host:   "alertmanager.example.com:8080",
			Path:   "/api/v1/alerts",
		},
	}
}

func (testAlertmanagerRetriever) DroppedAlertmanagers() []*url.URL {
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
	expr1, err := testParser.ParseExpr(`absent(test_metric3) != 1`)
	require.NoError(m.testing, err)
	expr2, err := testParser.ParseExpr(`up == 1`)
	require.NoError(m.testing, err)
	expr3, err := testParser.ParseExpr(`vector(1)`)
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
		promslog.NewNopLogger(),
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
		promslog.NewNopLogger(),
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
		promslog.NewNopLogger(),
	)
	rule4 := rules.NewAlertingRule(
		"test_metric6",
		expr2,
		time.Second,
		0,
		labels.FromStrings("testlabel", "rule"),
		labels.Labels{},
		labels.Labels{},
		"",
		true,
		promslog.NewNopLogger(),
	)
	rule5 := rules.NewAlertingRule(
		"test_metric7",
		expr2,
		time.Second,
		0,
		labels.FromStrings("templatedlabel", "{{ $externalURL }}"),
		labels.Labels{},
		labels.Labels{},
		"",
		true,
		promslog.NewNopLogger(),
	)
	var r []*rules.AlertingRule
	r = append(r, rule1)
	r = append(r, rule2)
	r = append(r, rule3)
	r = append(r, rule4)
	r = append(r, rule5)
	m.alertingRules = r
}

func (m *rulesRetrieverMock) CreateRuleGroups() {
	m.CreateAlertingRules()
	arules := m.AlertingRules()
	// Create separate storage for recordings to not pollute the main one.
	s := teststorage.New(m.testing)

	engineOpts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    100 * time.Second,
	}
	engine := promqltest.NewTestEngineWithOpts(m.testing, engineOpts)
	opts := &rules.ManagerOptions{
		QueryFunc:  rules.EngineQueryFunc(engine, s),
		Appendable: s,
		Context:    context.Background(),
		Logger:     promslog.NewNopLogger(),
		NotifyFunc: func(context.Context, string, ...*rules.Alert) {},
	}

	var r []rules.Rule

	for _, alertrule := range arules {
		r = append(r, alertrule)
	}

	recordingExpr, err := testParser.ParseExpr(`vector(1)`)
	require.NoError(m.testing, err, "unable to parse alert expression")
	recordingRule := rules.NewRecordingRule("recording-rule-1", recordingExpr, labels.Labels{})
	recordingRule2 := rules.NewRecordingRule("recording-rule-2", recordingExpr, labels.FromStrings("testlabel", "rule"))
	r = append(r, recordingRule)
	r = append(r, recordingRule2)

	group := rules.NewGroup(rules.GroupOptions{
		Name:          "grp",
		File:          "/path/to/file",
		Interval:      time.Second,
		Rules:         r,
		ShouldRestore: false,
		Opts:          opts,
	})
	group2 := rules.NewGroup(rules.GroupOptions{
		Name:          "grp2",
		File:          "/path/to/file",
		Interval:      time.Second,
		Rules:         []rules.Rule{r[0]},
		ShouldRestore: false,
		Opts:          opts,
	})
	m.ruleGroups = []*rules.Group{group, group2}
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
	OTLPConfig:         config.OTLPConfig{},
}

var sampleFlagMap = map[string]string{
	"flag1": "value1",
	"flag2": "value2",
}

func appendExemplars(t testing.TB, s storage.Storage, ex []exemplar.QueryResult) {
	t.Helper()

	// TODO(bwplotka): Use AppenderV2.AppendExemplar per series flow
	// once its implemented: https://github.com/prometheus/prometheus/issues/17632#issuecomment-3759315095
	app := s.Appender(t.Context())
	for _, ed := range ex {
		for _, e := range ed.Exemplars {
			_, err := app.AppendExemplar(0, ed.SeriesLabels, e)
			require.NoError(t, err)
		}
	}
	require.NoError(t, app.Commit())
}

func TestEndpoints(t *testing.T) {
	s := promqltest.LoadedStorage(t, `
		load 1m
			test_metric1{foo="bar"} 0+100x100
			test_metric1{foo="boo"} 1+0x100
			test_metric2{foo="boo"} 1+0x100
			test_metric3{foo="bar", dup="1"} 1+0x100
			test_metric3{foo="boo", dup="1"} 1+0x100
			test_metric4{foo="bar", dup="1"} 1+0x100
			test_metric4{foo="boo", dup="1"} 1+0x100
			test_metric4{foo="boo"} 1+0x100
			test_metric5{"host.name"="localhost"} 1+0x100
			test_metric5{"junk\n{},=:  chars"="bar"} 1+0x100
	`)

	// Add exemplar testdata here, given promqltest does not support exemplars.
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
	appendExemplars(t, s, exemplars)

	now := time.Now()
	ng := testEngine(t)
	t.Run("local", func(t *testing.T) {
		algr := rulesRetrieverMock{testing: t}

		algr.CreateAlertingRules()
		algr.CreateRuleGroups()

		g := algr.RuleGroups()
		g[0].Eval(context.Background(), time.Now())

		testTargetRetriever := setupTestTargetRetriever(t)

		api := &API{
			Queryable:             s,
			QueryEngine:           ng,
			ExemplarQueryable:     s,
			targetRetriever:       testTargetRetriever.toFactory(),
			alertmanagerRetriever: testAlertmanagerRetriever{}.toFactory(),
			flagsMap:              sampleFlagMap,
			now:                   func() time.Time { return now },
			config:                func() config.Config { return samplePrometheusCfg },
			ready:                 func(f http.HandlerFunc) http.HandlerFunc { return f },
			rulesRetriever:        algr.toFactory(),
			parser:                testParser,
		}
		testEndpoints(t, api, testTargetRetriever, true)
	})

	// Run all the API tests against an API that is wired to forward queries via
	// the remote read client to a test server, which in turn sends them to the
	// data from the test storage.
	t.Run("remote", func(t *testing.T) {
		server := setupRemote(s)
		defer server.Close()

		u, err := url.Parse(server.URL)
		require.NoError(t, err)

		al := promslog.NewLevel()
		require.NoError(t, al.Set("debug"))

		af := promslog.NewFormat()
		require.NoError(t, af.Set("logfmt"))

		promslogConfig := promslog.Config{
			Level:  al,
			Format: af,
		}

		dbDir := t.TempDir()

		remote := remote.NewStorage(promslog.New(&promslogConfig), prometheus.DefaultRegisterer, func() (int64, error) {
			return 0, nil
		}, dbDir, 1*time.Second, nil, false)
		t.Cleanup(func() { _ = remote.Close() })

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

		algr := rulesRetrieverMock{testing: t}

		algr.CreateAlertingRules()
		algr.CreateRuleGroups()

		g := algr.RuleGroups()
		g[0].Eval(context.Background(), time.Now())

		testTargetRetriever := setupTestTargetRetriever(t)

		api := &API{
			Queryable:             remote,
			QueryEngine:           ng,
			ExemplarQueryable:     s,
			targetRetriever:       testTargetRetriever.toFactory(),
			alertmanagerRetriever: testAlertmanagerRetriever{}.toFactory(),
			flagsMap:              sampleFlagMap,
			now:                   func() time.Time { return now },
			config:                func() config.Config { return samplePrometheusCfg },
			ready:                 func(f http.HandlerFunc) http.HandlerFunc { return f },
			rulesRetriever:        algr.toFactory(),
			parser:                testParser,
		}
		testEndpoints(t, api, testTargetRetriever, false)
	})
}

type byLabels []labels.Labels

func (b byLabels) Len() int           { return len(b) }
func (b byLabels) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byLabels) Less(i, j int) bool { return labels.Compare(b[i], b[j]) < 0 }

func TestGetSeries(t *testing.T) {
	// TestEndpoints doesn't have enough label names to test api.labelNames
	// endpoint properly. Hence we test it separately.
	s := promqltest.LoadedStorage(t, `
		load 1m
			test_metric1{foo1="bar", baz="abc"} 0+100x100
			test_metric1{foo2="boo"} 1+0x100
			test_metric2{foo="boo"} 1+0x100
			test_metric2{foo="boo", xyz="qwerty"} 1+0x100
			test_metric2{foo="baz", abc="qwerty"} 1+0x100
	`)

	api := &API{
		Queryable: s,
		parser:    testParser,
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
				Queryable: errorTestQueryable{err: errors.New("generic")},
				parser:    testParser,
			},
		},
		{
			name:              "storage error type",
			matchers:          []string{`{foo="boo"}`, `{foo="baz"}`},
			expectedErrorType: errorInternal,
			api: &API{
				Queryable: errorTestQueryable{err: promql.ErrStorage{Err: errors.New("generic")}},
				parser:    testParser,
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
	s := promqltest.LoadedStorage(t, `
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

	api := &API{
		Queryable:         s,
		QueryEngine:       testEngine(t),
		ExemplarQueryable: s,
		parser:            testParser,
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
				ExemplarQueryable: errorTestQueryable{err: errors.New("generic")},
				parser:            testParser,
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
				ExemplarQueryable: errorTestQueryable{err: promql.ErrStorage{Err: errors.New("generic")}},
				parser:            testParser,
			},
			query: url.Values{
				"query": []string{`test_metric3{foo="boo"} - test_metric4{foo="bar"}`},
				"start": []string{"0"},
				"end":   []string{"4"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			es := s
			ctx := context.Background()

			appendExemplars(t, es, tc.exemplars)

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
	s := promqltest.LoadedStorage(t, `
		load 1m
			test_metric1{foo1="bar", baz="abc"} 0+100x100
			test_metric1{foo2="boo"} 1+0x100
			test_metric2{foo="boo"} 1+0x100
			test_metric2{foo="boo", xyz="qwerty"} 1+0x100
			test_metric2{foo="baz", abc="qwerty"} 1+0x100
	`)

	api := &API{
		Queryable: s,
		parser:    testParser,
	}
	request := func(method, limit string, matchers ...string) (*http.Request, error) {
		u, err := url.Parse("http://example.com")
		require.NoError(t, err)
		q := u.Query()
		for _, matcher := range matchers {
			q.Add("match[]", matcher)
		}
		if limit != "" {
			q.Add("limit", limit)
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
		limit             string
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
			name:     "non empty label matcher with limit",
			matchers: []string{`{foo=~".+"}`},
			expected: []string{"__name__", "abc"},
			limit:    "2",
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
				Queryable: errorTestQueryable{err: errors.New("generic")},
				parser:    testParser,
			},
		},
		{
			name:              "storage error type",
			matchers:          []string{`{foo="boo"}`, `{foo="baz"}`},
			expectedErrorType: errorInternal,
			api: &API{
				Queryable: errorTestQueryable{err: promql.ErrStorage{Err: errors.New("generic")}},
				parser:    testParser,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				ctx := context.Background()
				req, err := request(method, tc.limit, tc.matchers...)
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
	s := teststorage.New(t)

	api := &API{
		Queryable:   s,
		QueryEngine: testEngine(t),
		parser:      testParser,
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
		expected func(*testing.T, any)
	}{
		{
			name:  "stats is blank",
			param: "",
			expected: func(t *testing.T, i any) {
				require.IsType(t, &QueryData{}, i)
				qd := i.(*QueryData)
				require.Nil(t, qd.Stats)
			},
		},
		{
			name:  "stats is true",
			param: "true",
			expected: func(t *testing.T, i any) {
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
			expected: func(t *testing.T, i any) {
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
			renderer: func(_ context.Context, _ *stats.Statistics, p string) stats.QueryStats {
				if p == "known" {
					return testStats{"Custom Value"}
				}
				return nil
			},
			param: "known",
			expected: func(t *testing.T, i any) {
				require.IsType(t, &QueryData{}, i)
				qd := i.(*QueryData)
				require.NotNil(t, qd.Stats)
				json := jsoniter.ConfigCompatibleWithStandardLibrary
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
				assertAPIError(t, res.err, errorNone)
				tc.expected(t, res.data)

				res = api.queryRange(req.WithContext(ctx))
				assertAPIError(t, res.err, errorNone)
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
			Params:  url.Values{},
			Reports: []*testReport{{scrapeStart, 70 * time.Millisecond, nil}},
			Active:  true,
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
			Params:  url.Values{"target": []string{"example.com"}},
			Reports: []*testReport{{scrapeStart, 100 * time.Millisecond, errors.New("failed")}},
			Active:  true,
		},
		{
			Identifier: "blackbox",
			Labels:     labels.EmptyLabels(),
			targetLabels: model.LabelSet{
				model.SchemeLabel:         "http",
				model.AddressLabel:        "http://dropped.example.com:9115",
				model.MetricsPathLabel:    "/probe",
				model.JobLabel:            "blackbox",
				model.ScrapeIntervalLabel: "30s",
				model.ScrapeTimeoutLabel:  "15s",
			},
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

		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Header().Set("Content-Encoding", "snappy")

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
		endpoint              apiFunc
		params                map[string]string
		query                 url.Values
		response              any
		responseLen           int // If nonzero, check only the length; `response` is ignored.
		responseMetadataTotal int
		responseAsJSON        string
		warningsCount         int
		errType               errorType
		sorter                func(any)
		metadata              []targetMetadata
		zeroFunc              func(any)
	}

	rulesZeroFunc := func(i any) {
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
		// Only matrix and vector responses are limited/truncated. String and scalar responses aren't truncated.
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"2"},
				"time":  []string{"123.4"},
				"limit": []string{"1"},
			},
			response: &QueryData{
				ResultType: parser.ValueTypeScalar,
				Result: promql.Scalar{
					V: 2,
					T: timestamp.FromTime(start.Add(123*time.Second + 400*time.Millisecond)),
				},
			},
			warningsCount: 0,
		},
		// When limit = 0, limit is disabled.
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"2"},
				"time":  []string{"123.4"},
				"limit": []string{"0"},
			},
			response: &QueryData{
				ResultType: parser.ValueTypeScalar,
				Result: promql.Scalar{
					V: 2,
					T: timestamp.FromTime(start.Add(123*time.Second + 400*time.Millisecond)),
				},
			},
			warningsCount: 0,
		},
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"2"},
				"time":  []string{"123.4"},
				"limit": []string{"-1"},
			},
			errType: errorBadData,
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
			endpoint: api.query,
			query: url.Values{
				"query": []string{
					`label_replace(vector(42), "foo", "bar", "", "") or label_replace(vector(3.1415), "dings", "bums", "", "")`,
				},
				"time":  []string{"123.4"},
				"limit": []string{"2"},
			},
			warningsCount: 0,
			responseAsJSON: `{
		"resultType": "vector",
		"result": [
			{
				"metric": {
					"foo": "bar"
				},
				"value": [123.4, "42"]
			},
			{
				"metric": {
					"dings": "bums"
				},
				"value": [123.4, "3.1415"]
			}
		]
	}`,
		},
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{
					`label_replace(vector(42), "foo", "bar", "", "") or label_replace(vector(3.1415), "dings", "bums", "", "")`,
				},
				"time":  []string{"123.4"},
				"limit": []string{"1"},
			},
			warningsCount: 1,
			responseAsJSON: `{
		"resultType": "vector",
		"result": [
			{
				"metric": {
					"foo": "bar"
				},
				"value": [123.4, "42"]
			}
		]
	}`,
		},
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{
					`label_replace(vector(42), "foo", "bar", "", "") or label_replace(vector(3.1415), "dings", "bums", "", "")`,
				},
				"time":  []string{"123.4"},
				"limit": []string{"0"},
			},
			responseAsJSON: `{
		"resultType": "vector",
		"result": [
			{
				"metric": {
					"foo": "bar"
				},
				"value": [123.4, "42"]
			},
			{
				"metric": {
					"dings": "bums"
				},
				"value": [123.4, "3.1415"]
			}
		]
	}`,
			warningsCount: 0,
		},
		// limit=0 means no limit.
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{
					`label_replace(vector(42), "foo", "bar", "", "") or label_replace(vector(3.1415), "dings", "bums", "", "")`,
				},
				"start": []string{"0"},
				"end":   []string{"2"},
				"step":  []string{"1"},
				"limit": []string{"0"},
			},
			response: &QueryData{
				ResultType: parser.ValueTypeMatrix,
				Result: promql.Matrix{
					promql.Series{
						Metric: labels.FromMap(map[string]string{"dings": "bums"}),
						Floats: []promql.FPoint{
							{F: 3.1415, T: timestamp.FromTime(start)},
							{F: 3.1415, T: timestamp.FromTime(start.Add(1 * time.Second))},
							{F: 3.1415, T: timestamp.FromTime(start.Add(2 * time.Second))},
						},
					},
					promql.Series{
						Metric: labels.FromMap(map[string]string{"foo": "bar"}),
						Floats: []promql.FPoint{
							{F: 42, T: timestamp.FromTime(start)},
							{F: 42, T: timestamp.FromTime(start.Add(1 * time.Second))},
							{F: 42, T: timestamp.FromTime(start.Add(2 * time.Second))},
						},
					},
				},
			},
			warningsCount: 0,
		},
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{
					`label_replace(vector(42), "foo", "bar", "", "") or label_replace(vector(3.1415), "dings", "bums", "", "")`,
				},
				"start": []string{"0"},
				"end":   []string{"2"},
				"step":  []string{"1"},
				"limit": []string{"1"},
			},
			response: &QueryData{
				ResultType: parser.ValueTypeMatrix,
				Result: promql.Matrix{
					promql.Series{
						Metric: labels.FromMap(map[string]string{"dings": "bums"}),
						Floats: []promql.FPoint{
							{F: 3.1415, T: timestamp.FromTime(start)},
							{F: 3.1415, T: timestamp.FromTime(start.Add(1 * time.Second))},
							{F: 3.1415, T: timestamp.FromTime(start.Add(2 * time.Second))},
						},
					},
				},
			},
			warningsCount: 1,
		},
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{
					`label_replace(vector(42), "foo", "bar", "", "") or label_replace(vector(3.1415), "dings", "bums", "", "")`,
				},
				"start": []string{"0"},
				"end":   []string{"2"},
				"step":  []string{"1"},
				"limit": []string{"2"},
			},
			response: &QueryData{
				ResultType: parser.ValueTypeMatrix,
				Result: promql.Matrix{
					promql.Series{
						Metric: labels.FromMap(map[string]string{"dings": "bums"}),
						Floats: []promql.FPoint{
							{F: 3.1415, T: timestamp.FromTime(start)},
							{F: 3.1415, T: timestamp.FromTime(start.Add(1 * time.Second))},
							{F: 3.1415, T: timestamp.FromTime(start.Add(2 * time.Second))},
						},
					},
					promql.Series{
						Metric: labels.FromMap(map[string]string{"foo": "bar"}),
						Floats: []promql.FPoint{
							{F: 42, T: timestamp.FromTime(start)},
							{F: 42, T: timestamp.FromTime(start.Add(1 * time.Second))},
							{F: 42, T: timestamp.FromTime(start.Add(2 * time.Second))},
						},
					},
				},
			},
			warningsCount: 0,
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
					},
				},
			},
		},
		// Test empty vector result
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"bottomk(2, notExists)"},
			},
			responseAsJSON: `{"resultType":"vector","result":[]}`,
		},
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"bottomk(2, notExists)"},
				"start": []string{"0"},
				"end":   []string{"2"},
				"step":  []string{"1"},
				"limit": []string{"-1"},
			},
			errType: errorBadData,
		},
		// Test empty matrix result
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"bottomk(2, notExists)"},
				"start": []string{"0"},
				"end":   []string{"2"},
				"step":  []string{"1"},
			},
			responseAsJSON: `{"resultType":"matrix","result":[]}`,
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
			responseLen:   1, // API does not specify which particular value will come back.
			warningsCount: 1,
		},
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{"test_metric1"},
				"limit":   []string{"2"},
			},
			responseLen:   2, // API does not specify which particular value will come back.
			warningsCount: 0, // No warnings if limit isn't exceeded.
		},
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{"test_metric1"},
				"limit":   []string{"0"},
			},
			responseLen:   2, // API does not specify which particular value will come back.
			warningsCount: 0, // No warnings if limit isn't exceeded.
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
						DiscoveredLabels:   labels.FromStrings("__param_target", "example.com", "__scrape_interval__", "0s", "__scrape_timeout__", "0s"),
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
						DiscoveredLabels:   labels.FromStrings("__scrape_interval__", "0s", "__scrape_timeout__", "0s"),
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
						ScrapePool: "blackbox",
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
						DiscoveredLabels:   labels.FromStrings("__param_target", "example.com", "__scrape_interval__", "0s", "__scrape_timeout__", "0s"),
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
						DiscoveredLabels:   labels.FromStrings("__scrape_interval__", "0s", "__scrape_timeout__", "0s"),
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
						ScrapePool: "blackbox",
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
						DiscoveredLabels:   labels.FromStrings("__param_target", "example.com", "__scrape_interval__", "0s", "__scrape_timeout__", "0s"),
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
						DiscoveredLabels:   labels.FromStrings("__scrape_interval__", "0s", "__scrape_timeout__", "0s"),
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
						ScrapePool: "blackbox",
					},
				},
				DroppedTargetCounts: map[string]int{"blackbox": 1},
			},
		},
		{
			endpoint: api.targetRelabelSteps,
			query:    url.Values{"scrapePool": []string{"testpool"}, "labels": []string{`{"job":"test","__address__":"localhost:9090"}`}},
			response: &RelabelStepsResponse{
				Steps: []RelabelStep{
					{
						Rule: &relabel.Config{
							Action:               relabel.Replace,
							Replacement:          "example.com:443",
							TargetLabel:          "__address__",
							Regex:                relabel.MustNewRegexp(""),
							NameValidationScheme: model.LegacyValidation,
						},
						Output: labels.FromMap(map[string]string{
							"job":         "test",
							"__address__": "example.com:443",
						}),
						Keep: true,
					},
					{
						Rule: &relabel.Config{
							Action:       relabel.Drop,
							SourceLabels: []model.LabelName{"__address__"},
							Regex:        relabel.MustNewRegexp(`example\.com:.*`),
						},
						Output: labels.EmptyLabels(),
						Keep:   false,
					},
				},
			},
		},
		{
			endpoint: api.targetRelabelSteps,
			query:    url.Values{"scrapePool": []string{"testpool3"}, "labels": []string{`{"job":"test","__address__":"localhost:9090"}`}},
			response: &RelabelStepsResponse{
				Steps: []RelabelStep{
					{
						Rule: &relabel.Config{
							Action:               relabel.Replace,
							Replacement:          "example.com:443",
							TargetLabel:          "__address__",
							Regex:                relabel.MustNewRegexp(""),
							NameValidationScheme: model.LegacyValidation,
						},
						Output: labels.FromMap(map[string]string{
							"job":         "test",
							"__address__": "example.com:443",
						}),
						Keep: true,
					},
					{
						Rule: &relabel.Config{
							Action:       relabel.Drop,
							SourceLabels: []model.LabelName{"__address__"},
							Regex:        relabel.MustNewRegexp(`example\.com:.*`),
						},
						Output: labels.EmptyLabels(),
						Keep:   false,
					},
					{
						Rule: &relabel.Config{
							Action:      relabel.Replace,
							TargetLabel: "job",
							Regex:       relabel.MustNewRegexp(".*"),
							Replacement: "should_not_apply",
						},
						Output: labels.EmptyLabels(),
						Keep:   false,
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
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads created.",
							Unit:         "",
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
							MetricFamily: "prometheus_tsdb_storage_blocks_bytes",
							Type:         model.MetricTypeGauge,
							Help:         "The number of bytes that are currently used for local storage by all blocks.",
							Unit:         "",
						},
					},
				},
			},
			response: []metricMetadata{
				{
					Target: labels.FromMap(map[string]string{
						"job": "blackbox",
					}),
					MetricFamily: "prometheus_tsdb_storage_blocks_bytes",
					Help:         "The number of bytes that are currently used for local storage by all blocks.",
					Type:         model.MetricTypeGauge,
					Unit:         "",
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
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads created.",
							Unit:         "",
						},
					},
				},
				{
					identifier: "blackbox",
					metadata: []scrape.MetricMetadata{
						{
							MetricFamily: "prometheus_tsdb_storage_blocks_bytes",
							Type:         model.MetricTypeGauge,
							Help:         "The number of bytes that are currently used for local storage by all blocks.",
							Unit:         "",
						},
					},
				},
			},
			response: []metricMetadata{
				{
					Target: labels.FromMap(map[string]string{
						"job": "test",
					}),
					MetricFamily: "go_threads",
					Help:         "Number of OS threads created.",
					Type:         model.MetricTypeGauge,
					Unit:         "",
				},
				{
					Target: labels.FromMap(map[string]string{
						"job": "blackbox",
					}),
					MetricFamily: "prometheus_tsdb_storage_blocks_bytes",
					Help:         "The number of bytes that are currently used for local storage by all blocks.",
					Type:         model.MetricTypeGauge,
					Unit:         "",
				},
			},
			sorter: func(m any) {
				sort.Slice(m.([]metricMetadata), func(i, j int) bool {
					mm := m.([]metricMetadata)
					return mm[i].MetricFamily < mm[j].MetricFamily
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
							MetricFamily: "prometheus_engine_query_duration_seconds",
							Type:         model.MetricTypeSummary,
							Help:         "Query timings",
							Unit:         "",
						},
						{
							MetricFamily: "go_info",
							Type:         model.MetricTypeGauge,
							Help:         "Information about the Go environment.",
							Unit:         "",
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
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads created",
							Unit:         "",
						},
					},
				},
				{
					identifier: "blackbox",
					metadata: []scrape.MetricMetadata{
						{
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads created",
							Unit:         "",
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
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads created",
							Unit:         "",
						},
					},
				},
				{
					identifier: "blackbox",
					metadata: []scrape.MetricMetadata{
						{
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads that were created.",
							Unit:         "",
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
			sorter: func(m any) {
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
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads created",
							Unit:         "",
						},
						{
							MetricFamily: "prometheus_engine_query_duration_seconds",
							Type:         model.MetricTypeSummary,
							Help:         "Query Timings.",
							Unit:         "",
						},
					},
				},
				{
					identifier: "blackbox",
					metadata: []scrape.MetricMetadata{
						{
							MetricFamily: "go_gc_duration_seconds",
							Type:         model.MetricTypeSummary,
							Help:         "A summary of the GC invocation durations.",
							Unit:         "",
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
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads created",
							Unit:         "",
						},
						{
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Repeated metadata",
							Unit:         "",
						},
						{
							MetricFamily: "go_gc_duration_seconds",
							Type:         model.MetricTypeSummary,
							Help:         "A summary of the GC invocation durations.",
							Unit:         "",
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
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads created",
							Unit:         "",
						},
						{
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Repeated metadata",
							Unit:         "",
						},
						{
							MetricFamily: "go_gc_duration_seconds",
							Type:         model.MetricTypeSummary,
							Help:         "A summary of the GC invocation durations.",
							Unit:         "",
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
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads created",
							Unit:         "",
						},
						{
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Repeated metadata",
							Unit:         "",
						},
						{
							MetricFamily: "go_gc_duration_seconds",
							Type:         model.MetricTypeSummary,
							Help:         "A summary of the GC invocation durations.",
							Unit:         "",
						},
					},
				},
				{
					identifier: "secondTarget",
					metadata: []scrape.MetricMetadata{
						{
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads created, but from a different target",
							Unit:         "",
						},
						{
							MetricFamily: "go_gc_duration_seconds",
							Type:         model.MetricTypeSummary,
							Help:         "A summary of the GC invocation durations, but from a different target.",
							Unit:         "",
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
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads created",
							Unit:         "",
						},
					},
				},
				{
					identifier: "blackbox",
					metadata: []scrape.MetricMetadata{
						{
							MetricFamily: "go_gc_duration_seconds",
							Type:         model.MetricTypeSummary,
							Help:         "A summary of the GC invocation durations.",
							Unit:         "",
						},
						{
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads that were created.",
							Unit:         "",
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
			sorter: func(m any) {
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
							MetricFamily: "go_threads",
							Type:         model.MetricTypeGauge,
							Help:         "Number of OS threads created",
							Unit:         "",
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
			zeroFunc: func(i any) {
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
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric6",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.FromStrings("testlabel", "rule"),
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "ok",
								Type:        "alerting",
							},
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric7",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.FromStrings("templatedlabel", "{{ $externalURL }}"),
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
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
							RecordingRule{
								Name:   "recording-rule-2",
								Query:  "vector(1)",
								Labels: labels.FromStrings("testlabel", "rule"),
								Health: "ok",
								Type:   "recording",
							},
						},
					},
					{
						Name:     "grp2",
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
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric6",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.FromStrings("testlabel", "rule"),
								Annotations: labels.Labels{},
								Alerts:      nil,
								Health:      "ok",
								Type:        "alerting",
							},
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric7",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.FromStrings("templatedlabel", "{{ $externalURL }}"),
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
							RecordingRule{
								Name:   "recording-rule-2",
								Query:  "vector(1)",
								Labels: labels.FromStrings("testlabel", "rule"),
								Health: "ok",
								Type:   "recording",
							},
						},
					},
					{
						Name:     "grp2",
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
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric6",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.FromStrings("testlabel", "rule"),
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "ok",
								Type:        "alerting",
							},
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric7",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.FromStrings("templatedlabel", "{{ $externalURL }}"),
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "ok",
								Type:        "alerting",
							},
						},
					},
					{
						Name:     "grp2",
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
							RecordingRule{
								Name:   "recording-rule-2",
								Query:  "vector(1)",
								Labels: labels.FromStrings("testlabel", "rule"),
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
			endpoint: api.rules,
			query: url.Values{
				"match[]": []string{`{testlabel="rule"}`},
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
								Name:        "test_metric6",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.FromStrings("testlabel", "rule"),
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "ok",
								Type:        "alerting",
							},
							RecordingRule{
								Name:   "recording-rule-2",
								Query:  "vector(1)",
								Labels: labels.FromStrings("testlabel", "rule"),
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
				"type":    []string{"alert"},
				"match[]": []string{`{templatedlabel="{{ $externalURL }}"}`},
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
								Name:        "test_metric7",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.FromStrings("templatedlabel", "{{ $externalURL }}"),
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
			query: url.Values{
				"match[]": []string{`{testlabel="abc"}`},
			},
			response: &RuleDiscovery{
				RuleGroups: []*RuleGroup{},
			},
		},
		// This is testing OR condition, the api response should return rule if it matches one of the label selector
		{
			endpoint: api.rules,
			query: url.Values{
				"match[]": []string{`{testlabel="abc"}`, `{testlabel="rule"}`},
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
								Name:        "test_metric6",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.FromStrings("testlabel", "rule"),
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "ok",
								Type:        "alerting",
							},
							RecordingRule{
								Name:   "recording-rule-2",
								Query:  "vector(1)",
								Labels: labels.FromStrings("testlabel", "rule"),
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
				"type":    []string{"record"},
				"match[]": []string{`{testlabel="rule"}`},
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
								Name:   "recording-rule-2",
								Query:  "vector(1)",
								Labels: labels.FromStrings("testlabel", "rule"),
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
				"type":    []string{"alert"},
				"match[]": []string{`{testlabel="rule"}`},
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
								Name:        "test_metric6",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.FromStrings("testlabel", "rule"),
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
			query: url.Values{
				"group_limit": []string{"1"},
			},
			response: &RuleDiscovery{
				GroupNextToken: getRuleGroupNextToken("/path/to/file", "grp2"),
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
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric6",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.FromStrings("testlabel", "rule"),
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
								Health:      "ok",
								Type:        "alerting",
							},
							AlertingRule{
								State:       "inactive",
								Name:        "test_metric7",
								Query:       "up == 1",
								Duration:    1,
								Labels:      labels.FromStrings("templatedlabel", "{{ $externalURL }}"),
								Annotations: labels.Labels{},
								Alerts:      []*Alert{},
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
							RecordingRule{
								Name:   "recording-rule-2",
								Query:  "vector(1)",
								Labels: labels.FromStrings("testlabel", "rule"),
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
				"group_limit":      []string{"1"},
				"group_next_token": []string{getRuleGroupNextToken("/path/to/file", "grp2")},
			},
			response: &RuleDiscovery{
				RuleGroups: []*RuleGroup{
					{
						Name:     "grp2",
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
						},
					},
				},
			},
			zeroFunc: rulesZeroFunc,
		},
		{ // invalid pagination request
			endpoint: api.rules,
			query: url.Values{
				"group_next_token": []string{getRuleGroupNextToken("/path/to/file", "grp2")},
			},
			errType:  errorBadData,
			zeroFunc: rulesZeroFunc,
		},
		{ // invalid group_limit
			endpoint: api.rules,
			query: url.Values{
				"group_limit":      []string{"0"},
				"group_next_token": []string{getRuleGroupNextToken("/path/to/file", "grp2")},
			},
			errType:  errorBadData,
			zeroFunc: rulesZeroFunc,
		},
		{ // Pagination token is invalid due to changes in the rule groups
			endpoint: api.rules,
			query: url.Values{
				"group_limit":      []string{"1"},
				"group_next_token": []string{getRuleGroupNextToken("/removed/file", "notfound")},
			},
			errType:  errorBadData,
			zeroFunc: rulesZeroFunc,
		},
		{ // groupNextToken should not be in empty response
			endpoint: api.rules,
			query: url.Values{
				"match[]":     []string{`{testlabel="abc-cannot-find"}`},
				"group_limit": []string{"1"},
			},
			responseAsJSON: `{"groups":[]}`,
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
					"test_metric5",
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
			// Bad name parameter
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "host.name\xff",
				},
				errType: errorBadData,
			},
			// Valid utf8 name parameter for utf8 validation.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "host.name",
				},
				response: []string{
					"localhost",
				},
			},
			// Valid escaped utf8 name parameter for utf8 validation.
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "U__junk_0a__7b__7d__2c__3d_:_20__20_chars",
				},
				response: []string{
					"bar",
				},
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
				responseLen:   2, // API does not specify which particular values will come back.
				warningsCount: 1,
			},
			{
				endpoint: api.labelValues,
				params: map[string]string{
					"name": "__name__",
				},
				query: url.Values{
					"limit": []string{"5"},
				},
				responseLen:   5, // API does not specify which particular values will come back.
				warningsCount: 0, // No warnings if limit isn't exceeded.
			},
			// Label names.
			{
				endpoint: api.labelNames,
				response: []string{"__name__", "dup", "foo", "host.name", "junk\n{},=:  chars"},
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
				response: []string{"__name__", "dup", "foo", "host.name", "junk\n{},=:  chars"},
			},
			// Start before Label names, end within Label names.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"start": []string{"-1"},
					"end":   []string{"10"},
				},
				response: []string{"__name__", "dup", "foo", "host.name", "junk\n{},=:  chars"},
			},

			// Start before Label names starts, end after Label names ends.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"start": []string{"-1"},
					"end":   []string{"100000"},
				},
				response: []string{"__name__", "dup", "foo", "host.name", "junk\n{},=:  chars"},
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
				response: []string{"__name__", "dup", "foo", "host.name", "junk\n{},=:  chars"},
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
				response: []string{"__name__", "dup", "foo", "host.name", "junk\n{},=:  chars"},
			},
			// Only provide End within Label names, don't provide a start time.
			{
				endpoint: api.labelNames,
				query: url.Values{
					"end": []string{"20"},
				},
				response: []string{"__name__", "dup", "foo", "host.name", "junk\n{},=:  chars"},
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
				responseLen:   2, // API does not specify which particular values will come back.
				warningsCount: 1,
			},
			{
				endpoint: api.labelNames,
				query: url.Values{
					"limit": []string{"5"},
				},
				responseLen:   5, // API does not specify which particular values will come back.
				warningsCount: 0, // No warnings if limit isn't exceeded.
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
						ctx = route.WithParam(ctx, p, v) //nolint:fatcontext // This is intentional to provide the route params.
					}

					req, err := request(method, test.query)
					require.NoError(t, err)

					tr.ResetMetadataStore()
					for _, tm := range test.metadata {
						// TODO: Check error and fixed broken test/bug.
						// TestEndpoints/local/run_60_metricMetadata_"limit=1&limit_per_metric=1"/GET fails if we check the error.
						_ = tr.SetMetadataStoreForTargets(tm.identifier, &testMetaStore{Metadata: tm.metadata})
					}

					res := test.endpoint(req.WithContext(ctx))
					if res.finalizer != nil {
						// Finalizers were added to ensure closed readers on API panics, ensure they are closed here too.
						res.finalizer()
					}
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
						if test.response != nil {
							assertAPIResponse(t, res.data, test.response)
						}
					}

					if test.responseAsJSON != "" {
						json := jsoniter.ConfigCompatibleWithStandardLibrary
						s, err := json.Marshal(res.data)
						require.NoError(t, err)
						require.JSONEq(t, test.responseAsJSON, string(s))
					}

					require.Len(t, res.warnings, test.warningsCount)
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

	if exp.num == ErrorNone {
		require.Nil(t, got)
	} else {
		require.NotNil(t, got)
		require.Equal(t, exp, got.typ, "(%q)", got)
	}
}

func assertAPIResponse(t *testing.T, got, exp any) {
	t.Helper()

	testutil.RequireEqualWithOptions(t, exp, got, []cmp.Option{
		cmpopts.IgnoreUnexported(regexp.Regexp{}),
	})
}

func assertAPIResponseLength(t *testing.T, got any, expLen int) {
	t.Helper()

	gotLen := reflect.ValueOf(got).Len()
	require.Equal(t, expLen, gotLen, "Response length does not match")
}

func assertAPIResponseMetadataLen(t *testing.T, got any, expLen int) {
	t.Helper()

	var gotLen int
	response := got.(map[string][]metadata.Metadata)
	for _, m := range response {
		gotLen += len(m)
	}

	require.Equal(t, expLen, gotLen, "Amount of metadata in the response does not match")
}

type fakeDB struct {
	err        error
	blockMetas []tsdb.BlockMeta
}

func (f *fakeDB) CleanTombstones() error { return f.err }

func (f *fakeDB) BlockMetas() ([]tsdb.BlockMeta, error) {
	return f.blockMetas, nil
}
func (f *fakeDB) Delete(context.Context, int64, int64, ...*labels.Matcher) error { return f.err }
func (f *fakeDB) Snapshot(string, bool) error                                    { return f.err }
func (*fakeDB) Stats(statsByLabelName string, limit int) (_ *tsdb.Stats, retErr error) {
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

func (*fakeDB) WALReplayStatus() (tsdb.WALReplayStatus, error) {
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
		t.Run("", func(t *testing.T) {
			dir := t.TempDir()

			api := &API{
				db:          tc.db,
				dbDir:       dir,
				ready:       func(f http.HandlerFunc) http.HandlerFunc { return f },
				enableAdmin: tc.enableAdmin,
				parser:      testParser,
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
		logger: promslog.NewNopLogger(),
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
		logger: promslog.NewNopLogger(),
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
	require.JSONEq(t, `{"status":"error","errorType":"not_acceptable","error":"cannot encode response as application/default-format"}`, string(body))
}

func TestServeTSDBBlocks(t *testing.T) {
	blockMeta := tsdb.BlockMeta{
		ULID:    ulid.MustNew(ulid.Now(), nil),
		MinTime: 0,
		MaxTime: 1000,
		Stats: tsdb.BlockStats{
			NumSeries: 10,
		},
	}

	db := &fakeDB{
		blockMetas: []tsdb.BlockMeta{blockMeta},
	}

	api := &API{
		db: db,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status/tsdb/blocks", nil)
	w := httptest.NewRecorder()

	result := api.serveTSDBBlocks(req)

	json.NewEncoder(w).Encode(result.data)

	resp := w.Result()
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var resultData struct {
		Blocks []tsdb.BlockMeta `json:"blocks"`
	}
	err := json.NewDecoder(resp.Body).Decode(&resultData)
	require.NoError(t, err)
	require.Len(t, resultData.Blocks, 1)
	require.Equal(t, blockMeta, resultData.Blocks[0])
}

func TestRespondError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	require.JSONEq(t, `{"status": "error", "data": "test", "errorType": "timeout", "error": "message"}`, string(body))
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
					return fmt.Errorf("invalid time value for '%s': %w", "foo", err)
				},
			},
		},
	}

	for _, test := range tests {
		req, err := http.NewRequest(http.MethodGet, "localhost:42/foo?"+test.paramName+"="+test.paramValue, nil)
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

	req, err := http.NewRequest(http.MethodOptions, s.URL+"/any_path", nil)
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
		{
			db:       tsdb,
			endpoint: tsdbStatusAPI,
			values:   map[string][]string{"limit": {"10000"}},
			errType:  errorNone,
		},
		{
			db:       tsdb,
			endpoint: tsdbStatusAPI,
			values:   map[string][]string{"limit": {"10001"}},
			errType:  errorBadData,
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
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
		}, {
			err:      context.Canceled,
			expected: errorCanceled,
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
	for i := range 10000 {
		points = append(points, promql.FPoint{F: float64(i * 1000000), T: int64(i)})
	}
	matrix := promql.Matrix{}
	for i := range 1000 {
		matrix = append(matrix, promql.Series{
			Metric: labels.FromStrings("__name__", fmt.Sprintf("series%v", i),
				"label", fmt.Sprintf("series%v", i),
				"label2", fmt.Sprintf("series%v", i)),
			Floats: points[:10],
		})
	}
	series := []labels.Labels{}
	for i := range 1000 {
		series = append(series, labels.FromStrings("__name__", fmt.Sprintf("series%v", i),
			"label", fmt.Sprintf("series%v", i),
			"label2", fmt.Sprintf("series%v", i)))
	}

	cases := []struct {
		name     string
		response any
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
			for b.Loop() {
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
			mustParseURL(t, "http://example.com"),
			GlobalURLOptions{
				ListenAddress: "127.0.0.1:9090",
				Host:          "prometheus.io",
				Scheme:        "https",
			},
			mustParseURL(t, "http://example.com"),
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

func (t *testCodec) CanEncode(*Response) bool {
	return t.canEncode
}

func (t *testCodec) Encode(*Response) ([]byte, error) {
	return fmt.Appendf(nil, "response from %v codec", t.contentType), nil
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
				require.EqualError(t, err, test.err.Error())
			}
		})
	}
}

// Test query timeout parameter.
func TestQueryTimeout(t *testing.T) {
	s := promqltest.LoadedStorage(t, `
		load 1m
			test_metric1{foo="bar"} 0+100x100
	`)

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
				Queryable:             s,
				QueryEngine:           engine,
				ExemplarQueryable:     s,
				alertmanagerRetriever: testAlertmanagerRetriever{}.toFactory(),
				flagsMap:              sampleFlagMap,
				now:                   func() time.Time { return now },
				config:                func() config.Config { return samplePrometheusCfg },
				ready:                 func(f http.HandlerFunc) http.HandlerFunc { return f },
				parser:                testParser,
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

func (e *fakeEngine) NewInstantQuery(context.Context, storage.Queryable, promql.QueryOpts, string, time.Time) (promql.Query, error) {
	return &e.query, nil
}

func (e *fakeEngine) NewRangeQuery(context.Context, storage.Queryable, promql.QueryOpts, string, time.Time, time.Time, time.Duration) (promql.Query, error) {
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

func (*fakeQuery) Close() {}

func (*fakeQuery) Statement() parser.Statement {
	return nil
}

func (*fakeQuery) Stats() *stats.Statistics {
	return nil
}

func (*fakeQuery) Cancel() {}

func (q *fakeQuery) String() string {
	return q.query
}
