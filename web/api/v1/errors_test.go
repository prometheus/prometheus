// Copyright 2022 The Prometheus Authors
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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/regexp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/route"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

func TestApiStatusCodes(t *testing.T) {
	for name, tc := range map[string]struct {
		err            error
		expectedString string
		expectedCode   int
	}{
		"random error": {
			err:            errors.New("some random error"),
			expectedString: "some random error",
			expectedCode:   http.StatusUnprocessableEntity,
		},

		"promql.ErrTooManySamples": {
			err:            promql.ErrTooManySamples("some error"),
			expectedString: "too many samples",
			expectedCode:   http.StatusUnprocessableEntity,
		},

		"promql.ErrQueryCanceled": {
			err:            promql.ErrQueryCanceled("some error"),
			expectedString: "query was canceled",
			expectedCode:   statusClientClosedConnection,
		},

		"promql.ErrQueryTimeout": {
			err:            promql.ErrQueryTimeout("some error"),
			expectedString: "query timed out",
			expectedCode:   http.StatusServiceUnavailable,
		},

		"context.DeadlineExceeded": {
			err:            context.DeadlineExceeded,
			expectedString: "context deadline exceeded",
			expectedCode:   http.StatusUnprocessableEntity,
		},

		"context.Canceled": {
			err:            context.Canceled,
			expectedString: "context canceled",
			expectedCode:   statusClientClosedConnection,
		},
	} {
		for k, q := range map[string]storage.SampleAndChunkQueryable{
			"error from queryable": errorTestQueryable{err: tc.err},
			"error from querier":   errorTestQueryable{q: errorTestQuerier{err: tc.err}},
			"error from seriesset": errorTestQueryable{q: errorTestQuerier{s: errorTestSeriesSet{err: tc.err}}},
		} {
			t.Run(fmt.Sprintf("%s/%s", name, k), func(t *testing.T) {
				r := createPrometheusAPI(q)
				rec := httptest.NewRecorder()

				req := httptest.NewRequest(http.MethodGet, "/api/v1/query?query=up", nil)

				r.ServeHTTP(rec, req)

				require.Equal(t, tc.expectedCode, rec.Code)
				require.Contains(t, rec.Body.String(), tc.expectedString)
			})
		}
	}
}

func createPrometheusAPI(q storage.SampleAndChunkQueryable) *route.Router {
	engine := promql.NewEngine(promql.EngineOpts{
		Logger:             log.NewNopLogger(),
		Reg:                nil,
		ActiveQueryTracker: nil,
		MaxSamples:         100,
		Timeout:            5 * time.Second,
	})

	api := NewAPI(
		engine,
		q,
		nil,
		nil,
		func(context.Context) ScrapePoolsRetriever { return &DummyScrapePoolsRetriever{} },
		func(context.Context) TargetRetriever { return &DummyTargetRetriever{} },
		func(context.Context) AlertmanagerRetriever { return &DummyAlertmanagerRetriever{} },
		func() config.Config { return config.Config{} },
		map[string]string{}, // TODO: include configuration flags
		GlobalURLOptions{},
		func(f http.HandlerFunc) http.HandlerFunc { return f },
		nil,   // Only needed for admin APIs.
		"",    // This is for snapshots, which is disabled when admin APIs are disabled. Hence empty.
		false, // Disable admin APIs.
		log.NewNopLogger(),
		func(context.Context) RulesRetriever { return &DummyRulesRetriever{} },
		0, 0, 0, // Remote read samples and concurrency limit.
		false, // Not an agent.
		regexp.MustCompile(".*"),
		func() (RuntimeInfo, error) { return RuntimeInfo{}, errors.New("not implemented") },
		&PrometheusVersion{},
		prometheus.DefaultGatherer,
		nil,
		nil,
		false,
		false,
	)

	promRouter := route.New().WithPrefix("/api/v1")
	api.Register(promRouter)

	return promRouter
}

type errorTestQueryable struct {
	storage.ExemplarQueryable
	q   storage.Querier
	err error
}

func (t errorTestQueryable) ExemplarQuerier(ctx context.Context) (storage.ExemplarQuerier, error) {
	return nil, t.err
}

func (t errorTestQueryable) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	return nil, t.err
}

func (t errorTestQueryable) Querier(mint, maxt int64) (storage.Querier, error) {
	if t.q != nil {
		return t.q, nil
	}
	return nil, t.err
}

type errorTestQuerier struct {
	s   storage.SeriesSet
	err error
}

func (t errorTestQuerier) LabelValues(context.Context, string, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, t.err
}

func (t errorTestQuerier) LabelNames(context.Context, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, t.err
}

func (t errorTestQuerier) Close() error {
	return nil
}

func (t errorTestQuerier) Select(_ context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	if t.s != nil {
		return t.s
	}
	return storage.ErrSeriesSet(t.err)
}

type errorTestSeriesSet struct {
	err error
}

func (t errorTestSeriesSet) Next() bool {
	return false
}

func (t errorTestSeriesSet) At() storage.Series {
	return nil
}

func (t errorTestSeriesSet) Err() error {
	return t.err
}

func (t errorTestSeriesSet) Warnings() annotations.Annotations {
	return nil
}

// DummyTargetRetriever implements github.com/prometheus/prometheus/web/api/v1.ScrapePoolsRetriever.
type DummyScrapePoolsRetriever struct{}

func (DummyScrapePoolsRetriever) ScrapePools() []string {
	return []string{}
}

// DummyTargetRetriever implements github.com/prometheus/prometheus/web/api/v1.targetRetriever.
type DummyTargetRetriever struct{}

// TargetsActive implements targetRetriever.
func (DummyTargetRetriever) TargetsActive() map[string][]*scrape.Target {
	return map[string][]*scrape.Target{}
}

// TargetsDropped implements targetRetriever.
func (DummyTargetRetriever) TargetsDropped() map[string][]*scrape.Target {
	return map[string][]*scrape.Target{}
}

// TargetsDroppedCounts implements targetRetriever.
func (DummyTargetRetriever) TargetsDroppedCounts() map[string]int {
	return nil
}

// DummyAlertmanagerRetriever implements AlertmanagerRetriever.
type DummyAlertmanagerRetriever struct{}

// Alertmanagers implements AlertmanagerRetriever.
func (DummyAlertmanagerRetriever) Alertmanagers() []*url.URL { return nil }

// DroppedAlertmanagers implements AlertmanagerRetriever.
func (DummyAlertmanagerRetriever) DroppedAlertmanagers() []*url.URL { return nil }

// DummyRulesRetriever implements RulesRetriever.
type DummyRulesRetriever struct{}

// RuleGroups implements RulesRetriever.
func (DummyRulesRetriever) RuleGroups() []*rules.Group {
	return nil
}

// AlertingRules implements RulesRetriever.
func (DummyRulesRetriever) AlertingRules() []*rules.AlertingRule {
	return nil
}
