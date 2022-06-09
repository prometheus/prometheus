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
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
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
		externalLabels: labels.Labels{{Name: "foo", Value: "baz"}, {Name: "zone", Value: "ie"}},
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
		externalLabels: labels.Labels{{Name: "instance", Value: "baz"}},
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
	suite, err := promql.NewTest(t, `
		load 1m
			test_metric1{foo="bar",instance="i"}    0+100x100
			test_metric1{foo="boo",instance="i"}    1+0x100
			test_metric2{foo="boo",instance="i"}    1+0x100
			test_metric_without_labels 1+10x100
			test_metric_stale                       1+10x99 stale
			test_metric_old                         1+10x98
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer suite.Close()

	if err := suite.Run(); err != nil {
		t.Fatal(err)
	}

	h := &Handler{
		localStorage:  &dbAdapter{suite.TSDB()},
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

func (notReadyReadStorage) Querier(context.Context, int64, int64) (storage.Querier, error) {
	return nil, errors.Wrap(tsdb.ErrNotReady, "wrap")
}

func (notReadyReadStorage) StartTime() (int64, error) {
	return 0, errors.Wrap(tsdb.ErrNotReady, "wrap")
}

func (notReadyReadStorage) Stats(string) (*tsdb.Stats, error) {
	return nil, errors.Wrap(tsdb.ErrNotReady, "wrap")
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
