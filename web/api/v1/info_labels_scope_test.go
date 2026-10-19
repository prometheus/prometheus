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

package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
)

func TestInfoLabelScopeCancellationAfterEvaluation(t *testing.T) {
	for _, size := range []int{0, maxInfoIdentifyingValues + 1} {
		vector := make(promql.Vector, size)
		for i := range vector {
			vector[i].Metric = labels.FromStrings("job", strconv.Itoa(i))
		}
		for _, tc := range []struct {
			name     string
			cause    error
			code     int
			kind     errorType
			deadline bool
		}{
			{name: "deadline", cause: context.DeadlineExceeded, code: http.StatusServiceUnavailable, kind: errorTimeout},
			{name: "cancellation", cause: context.Canceled, code: 499, kind: errorCanceled},
			{name: "custom cancellation cause", cause: errors.New("request stopped"), code: 499, kind: errorCanceled},
			{name: "custom deadline cause", cause: errors.New("query budget exhausted"), code: http.StatusServiceUnavailable, kind: errorTimeout, deadline: true},
		} {
			for _, path := range []string{"/api/v1/search/info_labels", "/api/v1/search/info_label_values"} {
				for _, method := range []string{http.MethodGet, http.MethodPost} {
					t.Run(strconv.Itoa(size)+"/"+tc.name+"/"+method+path, func(t *testing.T) {
						api, engine, querier := newInfoTimeoutTestAPI(t, time.Minute)
						queryable := &recordingSampleAndChunkQueryable{SampleAndChunkQueryable: api.Queryable}
						api.Queryable = queryable
						engine.query.result.Value = vector
						ctx, cancel := context.WithCancelCause(t.Context())
						defer cancel(nil)
						engine.query.afterExec = func() { cancel(tc.cause) }
						if tc.deadline {
							var stop context.CancelFunc
							ctx, stop = context.WithDeadlineCause(t.Context(), time.Now().Add(-time.Second), tc.cause)
							defer stop()
							engine.query.afterExec = nil
						}
						params := url.Values{"expr": {"metric"}}
						if strings.HasSuffix(path, "info_label_values") {
							params.Set("label", "version")
						}
						rec := httptest.NewRecorder()
						req := searchEndpointRequest(t, method, path, params).WithContext(ctx)
						if strings.HasSuffix(path, "info_label_values") {
							api.infoLabelValues(rec, req)
						} else {
							api.infoLabels(rec, req)
						}

						require.Equal(t, tc.code, rec.Code, rec.Body.String())
						var response Response
						require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
						require.Equal(t, tc.kind.str, response.ErrorType)
						require.Equal(t, tc.cause.Error(), response.Error)
						require.Len(t, engine.query.execCalls, 1)
						require.Empty(t, queryable.querierRanges)
						require.Empty(t, querier.contexts)
					})
				}
			}
		}
	}
}

// Cancel at a context error check after evaluation, allowing scope construction
// to be exercised without timer races.
type scopeCancellationContext struct {
	context.Context
	cancel   context.CancelCauseFunc
	cause    error
	cancelAt int
	checks   int
	armed    bool
}

func (c *scopeCancellationContext) Err() error {
	if c.armed {
		c.checks++
		if c.checks == c.cancelAt {
			c.cancel(c.cause)
		}
	}
	return c.Context.Err()
}

func TestInfoLabelScopeCancellationDuringConstruction(t *testing.T) {
	for _, tc := range []struct {
		name     string
		size     int
		metric   labels.Labels
		unique   bool
		cancelAt int
	}{
		{name: "duplicate identifiers", size: 1024, metric: labels.FromStrings("job", "api"), cancelAt: 3},
		{name: "ignored info samples", size: 1024, metric: labels.FromStrings("__name__", "target_info", "job", "api"), cancelAt: 3},
		{name: "samples without identifiers", size: 1024, metric: labels.EmptyLabels(), cancelAt: 3},
		{name: "value bound failure", size: maxInfoIdentifyingValues + 1, unique: true, cancelAt: 3 + maxInfoIdentifyingValues/128},
		{name: "regexp bound failure", size: 1, metric: labels.FromStrings("job", strings.Repeat("x", maxInfoIdentifyingRegexpBytes+1)), cancelAt: 3},
		{name: "empty scope materialization", cancelAt: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, engine, _ := newInfoTimeoutTestAPI(t, time.Minute)
			vector := make(promql.Vector, tc.size)
			for i := range vector {
				vector[i].Metric = tc.metric
				if tc.unique {
					vector[i].Metric = labels.FromStrings("job", strconv.Itoa(i))
				}
			}
			engine.query.result.Value = vector
			parent, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			ctx := &scopeCancellationContext{Context: parent, cancel: cancel, cause: context.DeadlineExceeded, cancelAt: tc.cancelAt}
			engine.query.afterExec = func() { ctx.armed = true }
			req := searchEndpointRequest(t, http.MethodGet, "/api/v1/search/info_labels", url.Values{"expr": {"metric"}})
			require.NoError(t, req.ParseForm())
			sp, apiErr := api.parseSearchParams(req, true)
			require.Nil(t, apiErr)

			scope, apiErr := api.infoMetricMatcherSets(ctx, req, sp)
			require.NotNil(t, apiErr)
			require.Equal(t, errorTimeout, apiErr.typ)
			require.ErrorIs(t, apiErr.err, context.DeadlineExceeded)
			require.Empty(t, scope.matcherSets)
		})
	}
}
