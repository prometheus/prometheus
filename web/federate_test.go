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

package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/teststorage"
)

var scenarios = map[string]struct {
	params         string
	externalLabels labels.Labels
	code           int
	body           string
}{
	"empty": {
		params: "",
		code:   200,
		body:   ``,
	},
	"match nothing": {
		params: "match[]=does_not_match_anything",
		code:   200,
		body:   ``,
	},
	"invalid params from the beginning": {
		params: "match[]=-not-a-valid-metric-name",
		code:   400,
		body: `1:1: parse error: unexpected <op:->
`,
	},
	"invalid params somewhere in the middle": {
		params: "match[]=not-a-valid-metric-name",
		code:   400,
		body: `1:4: parse error: unexpected <op:->
`,
	},
	"test_metric1": {
		params: "match[]=test_metric1",
		code:   200,
		body: `# TYPE test_metric1 untyped
test_metric1{foo="bar",instance="i"} 10000 6000000
test_metric1{foo="boo",instance="i"} 1 6000000
`,
	},
	"test_metric2": {
		params: "match[]=test_metric2",
		code:   200,
		body: `# TYPE test_metric2 untyped
test_metric2{foo="boo",instance="i"} 1 6000000
`,
	},
	"test_metric_without_labels": {
		params: "match[]=test_metric_without_labels",
		code:   200,
		body: `# TYPE test_metric_without_labels untyped
test_metric_without_labels{instance=""} 1001 6000000
`,
	},
	"test_stale_metric": {
		params: "match[]=test_metric_stale",
		code:   200,
		body:   ``,
	},
	"test_old_metric": {
		params: "match[]=test_metric_old",
		code:   200,
		body: `# TYPE test_metric_old untyped
test_metric_old{instance=""} 981 5880000
`,
	},
	"{foo='boo'}": {
		params: "match[]={foo='boo'}",
		code:   200,
		body: `# TYPE test_metric1 untyped
test_metric1{foo="boo",instance="i"} 1 6000000
# TYPE test_metric2 untyped
test_metric2{foo="boo",instance="i"} 1 6000000
`,
	},
	"two matchers": {
		params: "match[]=test_metric1&match[]=test_metric2",
		code:   200,
		body: `# TYPE test_metric1 untyped
test_metric1{foo="bar",instance="i"} 10000 6000000
test_metric1{foo="boo",instance="i"} 1 6000000
# TYPE test_metric2 untyped
test_metric2{foo="boo",instance="i"} 1 6000000
`,
	},
	"two matchers with overlap": {
		params: "match[]={__name__=~'test_metric1'}&match[]={foo='bar'}",
		code:   200,
		body: `# TYPE test_metric1 untyped
test_metric1{foo="bar",instance="i"} 10000 6000000
test_metric1{foo="boo",instance="i"} 1 6000000
`,
	},
	"everything": {
		params: "match[]={__name__=~'.%2b'}", // '%2b' is an URL-encoded '+'.
		code:   200,
		body: `# TYPE test_metric1 untyped
test_metric1{foo="bar",instance="i"} 10000 6000000
test_metric1{foo="boo",instance="i"} 1 6000000
# TYPE test_metric2 untyped
test_metric2{foo="boo",instance="i"} 1 6000000
# TYPE test_metric_old untyped
test_metric_old{instance=""} 981 5880000
# TYPE test_metric_without_labels untyped
test_metric_without_labels{instance=""} 1001 6000000
`,
	},
	"empty label value matches everything that doesn't have that label": {
		params: "match[]={foo='',__name__=~'.%2b'}",
		code:   200,
		body: `# TYPE test_metric_old untyped
test_metric_old{instance=""} 981 5880000
# TYPE test_metric_without_labels untyped
test_metric_without_labels{instance=""} 1001 6000000
`,
	},
	"empty label value for a label that doesn't exist at all, matches everything": {
		params: "match[]={bar='',__name__=~'.%2b'}",
		code:   200,
		body: `# TYPE test_metric1 untyped
test_metric1{foo="bar",instance="i"} 10000 6000000
test_metric1{foo="boo",instance="i"} 1 6000000
# TYPE test_metric2 untyped
test_metric2{foo="boo",instance="i"} 1 6000000
# TYPE test_metric_old untyped
test_metric_old{instance=""} 981 5880000
# TYPE test_metric_without_labels untyped
test_metric_without_labels{instance=""} 1001 6000000
`,
	},
	"external labels are added if not already present": {
		params:         "match[]={__name__=~'.%2b'}", // '%2b' is an URL-encoded '+'.
		externalLabels: labels.FromStrings("foo", "baz", "zone", "ie"),
		code:           200,
		body: `# TYPE test_metric1 untyped
test_metric1{foo="bar",instance="i",zone="ie"} 10000 6000000
test_metric1{foo="boo",instance="i",zone="ie"} 1 6000000
# TYPE test_metric2 untyped
test_metric2{foo="boo",instance="i",zone="ie"} 1 6000000
# TYPE test_metric_old untyped
test_metric_old{foo="baz",instance="",zone="ie"} 981 5880000
# TYPE test_metric_without_labels untyped
test_metric_without_labels{foo="baz",instance="",zone="ie"} 1001 6000000
`,
	},
	"instance is an external label": {
		// This makes no sense as a configuration, but we should
		// know what it does anyway.
		params:         "match[]={__name__=~'.%2b'}", // '%2b' is an URL-encoded '+'.
		externalLabels: labels.FromStrings("instance", "baz"),
		code:           200,
		body: `# TYPE test_metric1 untyped
test_metric1{foo="bar",instance="i"} 10000 6000000
test_metric1{foo="boo",instance="i"} 1 6000000
# TYPE test_metric2 untyped
test_metric2{foo="boo",instance="i"} 1 6000000
# TYPE test_metric_old untyped
test_metric_old{instance="baz"} 981 5880000
# TYPE test_metric_without_labels untyped
test_metric_without_labels{instance="baz"} 1001 6000000
`,
	},
}

func TestFederation(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 1m
			test_metric1{foo="bar",instance="i"}    0+100x100
			test_metric1{foo="boo",instance="i"}    1+0x100
			test_metric2{foo="boo",instance="i"}    1+0x100
			test_metric_without_labels 1+10x100
			test_metric_stale                       1+10x99 stale
			test_metric_old                         1+10x98
	`)
	t.Cleanup(func() { storage.Close() })

	h := &Handler{
		localStorage:  &dbAdapter{storage.DB},
		lookbackDelta: 5 * time.Minute,
		now:           func() model.Time { return 101 * 60 * 1000 }, // 101min after epoch.
		config: &config.Config{
			GlobalConfig: config.GlobalConfig{},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			h.config.GlobalConfig.ExternalLabels = scenario.externalLabels
			req := httptest.NewRequest("GET", "http://example.org/federate?"+scenario.params, nil)
			res := httptest.NewRecorder()

			h.federation(res, req)
			require.Equal(t, scenario.code, res.Code)
			require.Equal(t, scenario.body, normalizeBody(res.Body))
		})
	}
}

type notReadyReadStorage struct {
	LocalStorage
}

func (notReadyReadStorage) Querier(int64, int64) (storage.Querier, error) {
	return nil, fmt.Errorf("wrap: %w", tsdb.ErrNotReady)
}

func (notReadyReadStorage) StartTime() (int64, error) {
	return 0, fmt.Errorf("wrap: %w", tsdb.ErrNotReady)
}

func (notReadyReadStorage) Stats(string, int) (*tsdb.Stats, error) {
	return nil, fmt.Errorf("wrap: %w", tsdb.ErrNotReady)
}

// Regression test for https://github.com/prometheus/prometheus/issues/7181.
func TestFederation_NotReady(t *testing.T) {
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			h := &Handler{
				localStorage:  notReadyReadStorage{},
				lookbackDelta: 5 * time.Minute,
				now:           func() model.Time { return 101 * 60 * 1000 }, // 101min after epoch.
				config: &config.Config{
					GlobalConfig: config.GlobalConfig{
						ExternalLabels: scenario.externalLabels,
					},
				},
			}

			req := httptest.NewRequest("GET", "http://example.org/federate?"+scenario.params, nil)
			res := httptest.NewRecorder()

			h.federation(res, req)
			if scenario.code == http.StatusBadRequest {
				// Request are expected to be checked before DB readiness.
				require.Equal(t, http.StatusBadRequest, res.Code)
				return
			}
			require.Equal(t, http.StatusServiceUnavailable, res.Code)
		})
	}
}

// normalizeBody sorts the lines within a metric to make it easy to verify the body.
// (Federation is not taking care of sorting within a metric family.)
func normalizeBody(body *bytes.Buffer) string {
	var (
		lines    []string
		lastHash int
	)
	for line, err := body.ReadString('\n'); err == nil; line, err = body.ReadString('\n') {
		if line[0] == '#' && len(lines) > 0 {
			sort.Strings(lines[lastHash+1:])
			lastHash = len(lines)
		}
		lines = append(lines, line)
	}
	if len(lines) > 0 {
		sort.Strings(lines[lastHash+1:])
	}
	return strings.Join(lines, "")
}

func TestFederationWithNativeHistograms(t *testing.T) {
	storage := teststorage.New(t)
	t.Cleanup(func() { storage.Close() })

	var expVec promql.Vector

	db := storage.DB
	hist := &histogram.Histogram{
		Count:         12,
		ZeroCount:     2,
		ZeroThreshold: 0.001,
		Sum:           39.4,
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{1, 1, -1, 0},
		NegativeSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		NegativeBuckets: []int64{1, 1, -1, 0},
	}
	histWithoutZeroBucket := &histogram.Histogram{
		Count:  20,
		Sum:    99.23,
		Schema: 1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{2, 2, -2, 0},
		NegativeSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		NegativeBuckets: []int64{2, 2, -2, 0},
	}
	app := db.Appender(context.Background())
	for i := 0; i < 6; i++ {
		l := labels.FromStrings("__name__", "test_metric", "foo", fmt.Sprintf("%d", i))
		expL := labels.FromStrings("__name__", "test_metric", "instance", "", "foo", fmt.Sprintf("%d", i))
		var err error
		switch i {
		case 0, 3:
			_, err = app.Append(0, l, 100*60*1000, float64(i*100))
			expVec = append(expVec, promql.Sample{
				T:      100 * 60 * 1000,
				F:      float64(i * 100),
				Metric: expL,
			})
		case 4:
			_, err = app.AppendHistogram(0, l, 100*60*1000, histWithoutZeroBucket.Copy(), nil)
			expVec = append(expVec, promql.Sample{
				T:      100 * 60 * 1000,
				H:      histWithoutZeroBucket.ToFloat(nil),
				Metric: expL,
			})
		default:
			hist.ZeroCount++
			hist.Count++
			_, err = app.AppendHistogram(0, l, 100*60*1000, hist.Copy(), nil)
			expVec = append(expVec, promql.Sample{
				T:      100 * 60 * 1000,
				H:      hist.ToFloat(nil),
				Metric: expL,
			})
		}
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	h := &Handler{
		localStorage:  &dbAdapter{db},
		lookbackDelta: 5 * time.Minute,
		now:           func() model.Time { return 101 * 60 * 1000 }, // 101min after epoch.
		config: &config.Config{
			GlobalConfig: config.GlobalConfig{},
		},
	}

	req := httptest.NewRequest("GET", "http://example.org/federate?match[]=test_metric", nil)
	req.Header.Add("Accept", `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited,application/openmetrics-text;version=1.0.0;q=0.8,application/openmetrics-text;version=0.0.1;q=0.75,text/plain;version=0.0.4;q=0.5,*/*;q=0.1`)
	res := httptest.NewRecorder()

	h.federation(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	p := textparse.NewProtobufParser(body, false)
	var actVec promql.Vector
	metricFamilies := 0
	l := labels.Labels{}
	for {
		et, err := p.Next()
		if err != nil && errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if et == textparse.EntryHistogram || et == textparse.EntrySeries {
			p.Metric(&l)
		}
		switch et {
		case textparse.EntryHelp:
			metricFamilies++
		case textparse.EntryHistogram:
			_, parsedTimestamp, h, fh := p.Histogram()
			require.Nil(t, h)
			actVec = append(actVec, promql.Sample{
				T:      *parsedTimestamp,
				H:      fh,
				Metric: l,
			})
		case textparse.EntrySeries:
			_, parsedTimestamp, f := p.Series()
			actVec = append(actVec, promql.Sample{
				T:      *parsedTimestamp,
				F:      f,
				Metric: l,
			})
		}
	}

	// TODO(codesome): Once PromQL is able to set the CounterResetHint on histograms,
	// test it with switching histogram types for metric families.
	require.Equal(t, 4, metricFamilies)
	require.Equal(t, expVec, actVec)
}
