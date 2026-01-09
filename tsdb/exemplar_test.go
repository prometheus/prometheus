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

package tsdb

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

var eMetrics = NewExemplarMetrics(prometheus.DefaultRegisterer)

// Tests the same exemplar cases as AddExemplar, but specifically the ValidateExemplar function so it can be relied on externally.
func TestValidateExemplar(t *testing.T) {
	exs, err := NewCircularExemplarStorage(2, eMetrics, 0)
	require.NoError(t, err)
	es := exs.(*CircularExemplarStorage)

	l := labels.FromStrings("service", "asdf")
	e := exemplar.Exemplar{
		Labels: labels.FromStrings("trace_id", "qwerty"),
		Value:  0.1,
		Ts:     1,
	}

	require.NoError(t, es.ValidateExemplar(l, e))
	require.NoError(t, es.AddExemplar(l, e))

	e2 := exemplar.Exemplar{
		Labels: labels.FromStrings("trace_id", "zxcvb"),
		Value:  0.1,
		Ts:     2,
	}

	require.NoError(t, es.ValidateExemplar(l, e2))
	require.NoError(t, es.AddExemplar(l, e2))

	require.Equal(t, es.ValidateExemplar(l, e2), storage.ErrDuplicateExemplar, "error is expected attempting to validate duplicate exemplar")

	e3 := e2
	e3.Ts = 3
	require.Equal(t, es.ValidateExemplar(l, e3), storage.ErrDuplicateExemplar, "error is expected when attempting to add duplicate exemplar, even with different timestamp")

	e3.Ts = 1
	e3.Value = 0.3
	require.Equal(t, es.ValidateExemplar(l, e3), storage.ErrOutOfOrderExemplar)

	e4 := exemplar.Exemplar{
		Labels: labels.FromStrings("a", strings.Repeat("b", exemplar.ExemplarMaxLabelSetLength)),
		Value:  0.1,
		Ts:     2,
	}
	require.Equal(t, storage.ErrExemplarLabelLength, es.ValidateExemplar(l, e4))
}

func TestCircularExemplarStorage_AddExemplar(t *testing.T) {
	series1 := labels.FromStrings("trace_id", "foo")
	series2 := labels.FromStrings("trace_id", "bar")

	series1Matcher := []*labels.Matcher{{
		Type:  labels.MatchEqual,
		Name:  "trace_id",
		Value: series1.Get("trace_id"),
	}}

	series2Matcher := []*labels.Matcher{{
		Type:  labels.MatchEqual,
		Name:  "trace_id",
		Value: series2.Get("trace_id"),
	}}

	testCases := []struct {
		name          string
		size          int64
		exemplars     []exemplar.Exemplar
		wantExemplars []exemplar.Exemplar
		matcher       []*labels.Matcher
		wantError     error
	}{
		{
			name: "insert after newest",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
			},
			matcher: series1Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
			},
		},
		{
			name: "insert before oldest",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 2},
				{Labels: series1, Value: 0.2, Ts: 1},
			},
			matcher: series1Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.2, Ts: 1},
				{Labels: series1, Value: 0.1, Ts: 2},
			},
		},
		{
			name: "insert in between",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 3},
				{Labels: series1, Value: 0.3, Ts: 2},
			},
			matcher: series1Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.3, Ts: 2},
				{Labels: series1, Value: 0.2, Ts: 3},
			},
		},
		{
			name: "insert after newest with overflow",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.4, Ts: 4},
			},
			matcher: series1Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.4, Ts: 4},
			},
		},
		{
			name: "insert before oldest with overflow",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.4, Ts: 0},
			},
			matcher: series1Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.4, Ts: 0},
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
			},
		},
		{
			name: "insert between with overflow",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 3},
				{Labels: series1, Value: 0.3, Ts: 4},
				{Labels: series1, Value: 0.4, Ts: 2},
			},
			matcher: series1Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.4, Ts: 2},
				{Labels: series1, Value: 0.2, Ts: 3},
				{Labels: series1, Value: 0.3, Ts: 4},
			},
		},
		{
			name: "insert out of the OOO window",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 200},
				{Labels: series1, Value: 0.2, Ts: 1},
			},
			wantError: storage.ErrOutOfOrderExemplar,
		},
		{
			name: "insert multiple series",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 3},
				{Labels: series2, Value: 0.3, Ts: 4},
			},
			matcher: series1Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 3},
			},
		},
		{
			name: "insert multiple series with overflow",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: series2, Value: 0.1, Ts: 1},
				{Labels: series2, Value: 0.2, Ts: 2},
				{Labels: series2, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.4, Ts: 4},
			},
			matcher: series2Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series2, Value: 0.2, Ts: 2},
				{Labels: series2, Value: 0.3, Ts: 3},
			},
		},
		{
			name: "series1 overflows series2 out-of-order",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: series2, Value: 0.1, Ts: 3},
				{Labels: series2, Value: 0.2, Ts: 2},
				{Labels: series2, Value: 0.3, Ts: 4},
				{Labels: series1, Value: 0.4, Ts: 4},
				{Labels: series1, Value: 0.5, Ts: 1},
			},
			matcher: series2Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series2, Value: 0.3, Ts: 4},
			},
		},
		{
			name: "ignore duplicate exemplars",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 3},
				{Labels: series1, Value: 0.1, Ts: 3},
			},
			matcher: series1Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 3},
			},
		},
		{
			name: "ignore duplicate exemplars when buffer is full",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 3},
				{Labels: series1, Value: 0.2, Ts: 4},
				{Labels: series1, Value: 0.3, Ts: 5},
				{Labels: series1, Value: 0.3, Ts: 5},
			},
			matcher: series1Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 3},
				{Labels: series1, Value: 0.2, Ts: 4},
				{Labels: series1, Value: 0.3, Ts: 5},
			},
		},
		{
			name: "empty timestamps are valid",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 0},
				{Labels: series1, Value: 0.2, Ts: 0},
			},
			matcher: series1Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 0},
				{Labels: series1, Value: 0.2, Ts: 0},
			},
		},
		{
			name: "exemplar label length exceeds maximum",
			size: 3,
			exemplars: []exemplar.Exemplar{
				{Labels: labels.FromStrings("a", strings.Repeat("b", exemplar.ExemplarMaxLabelSetLength)), Value: 0.1, Ts: 2},
			},
			wantError: storage.ErrExemplarLabelLength,
		},
		{
			name: "native histograms",
			size: 6,
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
			},
			matcher: series1Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
			},
		},
		{
			name: "evict only exemplar for series then re-add",
			size: 2,
			exemplars: []exemplar.Exemplar{
				// series1 at index 0, series2 at index 1, then series1 evicts its own only exemplar
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series2, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
			},
			matcher: series1Matcher,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.3, Ts: 3},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			exs, err := NewCircularExemplarStorage(tc.size, eMetrics, 100)
			require.NoError(t, err)
			es := exs.(*CircularExemplarStorage)

			// Add exemplars and compare tc.wantErr against the first exemplar failing.
			var addError error
			for i, ex := range tc.exemplars {
				addError = es.AddExemplar(ex.Labels, ex)
				if addError != nil {
					break
				}
				if testing.Verbose() {
					t.Logf("Buffer[%d]:\n%s", i, debugCircularBuffer(es))
				}
			}
			if tc.wantError == nil {
				require.NoError(t, addError)
			} else {
				require.ErrorIs(t, addError, tc.wantError)
			}
			if addError != nil {
				return
			}

			// Ensure exemplars are returned correctly and in-order.
			gotExemplars, err := es.Select(0, 1000, tc.matcher)
			require.NoError(t, err)
			if len(tc.wantExemplars) == 0 {
				require.Empty(t, gotExemplars)
			} else {
				require.Len(t, gotExemplars, 1)
				require.Equal(t, tc.wantExemplars, gotExemplars[0].Exemplars)
			}
		})
	}
}

func TestCircularExemplarStorage_Resize(t *testing.T) {
	series1 := labels.FromStrings("trace_id", "foo")
	series2 := labels.FromStrings("trace_id", "bar")
	matcher1 := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchRegexp, "trace_id", "(foo|bar)"),
	}

	testCases := []struct {
		name          string
		exemplars     []exemplar.Exemplar
		resize        int64
		wantExemplars []exemplar.Exemplar
		wantNextIndex int
		wantError     error
	}{
		{
			name: "in-order, grow",
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
			},
			resize: 10,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
			},
			wantNextIndex: 2,
		},
		{
			name: "in-order, shrink",
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
			},
			resize: 2,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
			},
			wantNextIndex: 0,
		},
		{
			name: "out-of-order, shrink",
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.1, Ts: 1},
			},
			resize: 2,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
			},
			wantNextIndex: 0,
		},
		{
			name: "out-of-order, grow",
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.2, Ts: 2},
			},
			resize: 5,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
			},
			wantNextIndex: 2,
		},
		{
			name: "duplicate timestamps",
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 1},
				{Labels: series1, Value: 0.3, Ts: 2},
			},
			resize: 3,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 1},
				{Labels: series1, Value: 0.3, Ts: 2},
			},
		},
		{
			name:          "empty input, grow",
			exemplars:     []exemplar.Exemplar{},
			resize:        10,
			wantExemplars: []exemplar.Exemplar{},
			wantNextIndex: 0,
		},
		{
			name:          "empty input, shrink",
			exemplars:     []exemplar.Exemplar{},
			resize:        1,
			wantExemplars: []exemplar.Exemplar{},
			wantNextIndex: 0,
		},
		{
			name: "shrink to zero",
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
			},
			resize:        0,
			wantExemplars: []exemplar.Exemplar{},
			wantNextIndex: 0,
		},
		{
			name: "multiple series, shrink",
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series2, Value: 1.1, Ts: 2},
				{Labels: series1, Value: 0.2, Ts: 3},
				{Labels: series2, Value: 1.2, Ts: 4},
			},
			resize: 2,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.2, Ts: 3},
				{Labels: series2, Value: 1.2, Ts: 4},
			},
			wantNextIndex: 0,
		},
		{
			name: "shrink to one",
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
			},
			resize: 1,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.2, Ts: 2},
			},
			wantNextIndex: 0,
		},
		{
			name: "shrink to two",
			exemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
			},
			resize: 2,
			wantExemplars: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
			},
			wantNextIndex: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			exs, err := NewCircularExemplarStorage(3, eMetrics, 100)
			require.NoError(t, err)
			es := exs.(*CircularExemplarStorage)

			for _, ex := range tc.exemplars {
				require.NoError(t, es.AddExemplar(ex.Labels, ex))
			}

			// Resize the circular buffer.
			if testing.Verbose() {
				t.Logf("Buffer[before-resize]:\n%s", debugCircularBuffer(es))
			}
			es.Resize(tc.resize)
			if testing.Verbose() {
				t.Logf("Buffer[after-resize]:\n%s", debugCircularBuffer(es))
			}

			// Ensure exemplars are returned correctly and in-order.
			gotExemplars, err := es.Select(0, 1000, matcher1)
			require.NoError(t, err)
			flat := make([]exemplar.Exemplar, 0)
			for _, group := range gotExemplars {
				flat = append(flat, group.Exemplars...)
			}
			sort.Slice(flat, func(i, j int) bool {
				return flat[i].Ts < flat[j].Ts
			})
			require.Equal(t, tc.wantExemplars, flat, "exemplar mismatch")
			require.Equal(t, tc.wantNextIndex, es.nextIndex, "next index mismatch")
		})
	}

	resizeTwiceCases := []struct {
		name           string
		addExemplars1  []exemplar.Exemplar
		resize1        int64
		wantExemplars1 []exemplar.Exemplar
		resize2        int64
		addExemplars2  []exemplar.Exemplar
		wantExemplars2 []exemplar.Exemplar
	}{
		{
			name: "shrink then grow ordered",
			addExemplars1: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.4, Ts: 4},
			},
			resize1: 2,
			wantExemplars1: []exemplar.Exemplar{
				{Labels: series1, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.4, Ts: 4},
			},
			resize2: 5,
			addExemplars2: []exemplar.Exemplar{
				{Labels: series1, Value: 0.5, Ts: 5},
				{Labels: series1, Value: 0.6, Ts: 6},
				{Labels: series1, Value: 0.7, Ts: 7},
			},
			wantExemplars2: []exemplar.Exemplar{
				{Labels: series1, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.4, Ts: 4},
				{Labels: series1, Value: 0.5, Ts: 5},
				{Labels: series1, Value: 0.6, Ts: 6},
				{Labels: series1, Value: 0.7, Ts: 7},
			},
		},
		{
			name: "shrink then grow out-of-order",
			addExemplars1: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.4, Ts: 4},
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
			},
			resize1: 2,
			wantExemplars1: []exemplar.Exemplar{
				// We delete in the order of ingestion, not temporally.
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
			},
			resize2: 5,
			addExemplars2: []exemplar.Exemplar{
				{Labels: series1, Value: 0.7, Ts: 7},
				{Labels: series1, Value: 0.6, Ts: 6},
				{Labels: series1, Value: 0.5, Ts: 5},
			},
			wantExemplars2: []exemplar.Exemplar{
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.5, Ts: 5},
				{Labels: series1, Value: 0.6, Ts: 6},
				{Labels: series1, Value: 0.7, Ts: 7},
			},
		},
		{
			name: "grow then shrink ordered",
			addExemplars1: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.4, Ts: 4},
			},
			resize1: 5,
			wantExemplars1: []exemplar.Exemplar{
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.4, Ts: 4},
			},
			resize2: 2,
			addExemplars2: []exemplar.Exemplar{
				{Labels: series1, Value: 0.5, Ts: 5},
				{Labels: series1, Value: 0.6, Ts: 6},
				{Labels: series1, Value: 0.7, Ts: 7},
			},
			wantExemplars2: []exemplar.Exemplar{
				{Labels: series1, Value: 0.6, Ts: 6},
				{Labels: series1, Value: 0.7, Ts: 7},
			},
		},
		{
			name: "grow then shrink out-of-order",
			addExemplars1: []exemplar.Exemplar{
				{Labels: series1, Value: 0.1, Ts: 1},
				{Labels: series1, Value: 0.4, Ts: 4},
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
			},
			resize1: 5,
			wantExemplars1: []exemplar.Exemplar{
				// We delete in the order of ingestion, not temporally.
				{Labels: series1, Value: 0.2, Ts: 2},
				{Labels: series1, Value: 0.3, Ts: 3},
				{Labels: series1, Value: 0.4, Ts: 4},
			},
			resize2: 2,
			addExemplars2: []exemplar.Exemplar{
				{Labels: series1, Value: 0.7, Ts: 7},
				{Labels: series1, Value: 0.5, Ts: 5},
				{Labels: series1, Value: 0.6, Ts: 6},
			},
			wantExemplars2: []exemplar.Exemplar{
				{Labels: series1, Value: 0.5, Ts: 5},
				{Labels: series1, Value: 0.6, Ts: 6},
			},
		},
	}

	for _, tc := range resizeTwiceCases {
		t.Run(tc.name, func(t *testing.T) {
			exs, err := NewCircularExemplarStorage(3, eMetrics, 100)
			require.NoError(t, err)
			es := exs.(*CircularExemplarStorage)
			for _, ex := range tc.addExemplars1 {
				require.NoError(t, es.AddExemplar(ex.Labels, ex))
			}
			es.Resize(tc.resize1)
			gotExemplars, err := es.Select(0, 1000, matcher1)
			require.NoError(t, err)
			require.Len(t, gotExemplars, 1)
			require.Equal(t, tc.wantExemplars1, gotExemplars[0].Exemplars)
			es.Resize(tc.resize2)
			for _, ex := range tc.addExemplars2 {
				require.NoError(t, es.AddExemplar(ex.Labels, ex))
			}
			if testing.Verbose() {
				t.Logf("Buffer[after-resize2]:\n%s", debugCircularBuffer(es))
			}
			gotExemplars, err = es.Select(0, 1000, matcher1)
			require.NoError(t, err)
			require.Len(t, gotExemplars, 1)
			require.Equal(t, tc.wantExemplars2, gotExemplars[0].Exemplars)
		})
	}
}

func TestStorageOverflow(t *testing.T) {
	// Test that circular buffer index and assignment
	// works properly, adding more exemplars than can
	// be stored and then querying for them.
	exs, err := NewCircularExemplarStorage(5, eMetrics, 0)
	require.NoError(t, err)
	es := exs.(*CircularExemplarStorage)

	lName, lValue := "service", "asdf"
	l := labels.FromStrings(lName, lValue)

	var eList []exemplar.Exemplar
	for i := 0; i < len(es.exemplars)+1; i++ {
		e := exemplar.Exemplar{
			Labels: labels.FromStrings("trace_id", "a"),
			Value:  float64(i+1) / 10,
			Ts:     int64(101 + i),
		}
		es.AddExemplar(l, e)
		eList = append(eList, e)
	}
	require.True(t, (es.exemplars[0].exemplar.Ts == 106), "exemplar was not stored correctly")

	m, err := labels.NewMatcher(labels.MatchEqual, lName, lValue)
	require.NoError(t, err, "error creating label matcher for exemplar query")
	ret, err := es.Select(100, 110, []*labels.Matcher{m})
	require.NoError(t, err)
	require.Len(t, ret, 1, "select should have returned samples for a single series only")

	require.True(t, reflect.DeepEqual(eList[1:], ret[0].Exemplars), "select did not return expected exemplars\n\texpected: %+v\n\tactual: %+v\n", eList[1:], ret[0].Exemplars)
}

func TestSelectExemplar(t *testing.T) {
	exs, err := NewCircularExemplarStorage(5, eMetrics, 0)
	require.NoError(t, err)
	es := exs.(*CircularExemplarStorage)

	lName, lValue := "service", "asdf"
	l := labels.FromStrings(lName, lValue)
	e := exemplar.Exemplar{
		Labels: labels.FromStrings("trace_id", "querty"),
		Value:  0.1,
		Ts:     12,
	}

	err = es.AddExemplar(l, e)
	require.NoError(t, err, "adding exemplar failed")
	require.True(t, reflect.DeepEqual(es.exemplars[0].exemplar, e), "exemplar was not stored correctly")

	m, err := labels.NewMatcher(labels.MatchEqual, lName, lValue)
	require.NoError(t, err, "error creating label matcher for exemplar query")
	ret, err := es.Select(0, 100, []*labels.Matcher{m})
	require.NoError(t, err)
	require.Len(t, ret, 1, "select should have returned samples for a single series only")

	expectedResult := []exemplar.Exemplar{e}
	require.True(t, reflect.DeepEqual(expectedResult, ret[0].Exemplars), "select did not return expected exemplars\n\texpected: %+v\n\tactual: %+v\n", expectedResult, ret[0].Exemplars)
}

func TestSelectExemplar_MultiSeries(t *testing.T) {
	exs, err := NewCircularExemplarStorage(5, eMetrics, 0)
	require.NoError(t, err)
	es := exs.(*CircularExemplarStorage)

	l1Name := "test_metric"
	l1 := labels.FromStrings(labels.MetricName, l1Name, "service", "asdf")
	l2Name := "test_metric2"
	l2 := labels.FromStrings(labels.MetricName, l2Name, "service", "qwer")

	for i := 0; i < len(es.exemplars); i++ {
		e1 := exemplar.Exemplar{
			Labels: labels.FromStrings("trace_id", "a"),
			Value:  float64(i+1) / 10,
			Ts:     int64(101 + i),
		}
		err = es.AddExemplar(l1, e1)
		require.NoError(t, err)

		e2 := exemplar.Exemplar{
			Labels: labels.FromStrings("trace_id", "b"),
			Value:  float64(i+1) / 10,
			Ts:     int64(101 + i),
		}
		err = es.AddExemplar(l2, e2)
		require.NoError(t, err)
	}

	m, err := labels.NewMatcher(labels.MatchEqual, labels.MetricName, l2Name)
	require.NoError(t, err, "error creating label matcher for exemplar query")
	ret, err := es.Select(100, 200, []*labels.Matcher{m})
	require.NoError(t, err)
	require.Len(t, ret, 1, "select should have returned samples for a single series only")
	require.Len(t, ret[0].Exemplars, 3, "didn't get expected 8 exemplars, got %d", len(ret[0].Exemplars))

	m, err = labels.NewMatcher(labels.MatchEqual, labels.MetricName, l1Name)
	require.NoError(t, err, "error creating label matcher for exemplar query")
	ret, err = es.Select(100, 200, []*labels.Matcher{m})
	require.NoError(t, err)
	require.Len(t, ret, 1, "select should have returned samples for a single series only")
	require.Len(t, ret[0].Exemplars, 2, "didn't get expected 8 exemplars, got %d", len(ret[0].Exemplars))
}

func TestSelectExemplar_TimeRange(t *testing.T) {
	var lenEs int64 = 5
	exs, err := NewCircularExemplarStorage(lenEs, eMetrics, 0)
	require.NoError(t, err)
	es := exs.(*CircularExemplarStorage)

	lName, lValue := "service", "asdf"
	l := labels.FromStrings(lName, lValue)

	for i := 0; int64(i) < lenEs; i++ {
		err := es.AddExemplar(l, exemplar.Exemplar{
			Labels: labels.FromStrings("trace_id", strconv.Itoa(i)),
			Value:  0.1,
			Ts:     int64(101 + i),
		})
		require.NoError(t, err)
		require.Equal(t, es.index[string(l.Bytes(nil))].newest, i, "exemplar was not stored correctly")
	}

	m, err := labels.NewMatcher(labels.MatchEqual, lName, lValue)
	require.NoError(t, err, "error creating label matcher for exemplar query")
	ret, err := es.Select(102, 104, []*labels.Matcher{m})
	require.NoError(t, err)
	require.Len(t, ret, 1, "select should have returned samples for a single series only")
	require.Len(t, ret[0].Exemplars, 3, "didn't get expected two exemplars %d, %+v", len(ret[0].Exemplars), ret)
}

// Test to ensure that even though a series matches more than one matcher from the
// query that it's exemplars are only included in the result a single time.
func TestSelectExemplar_DuplicateSeries(t *testing.T) {
	exs, err := NewCircularExemplarStorage(4, eMetrics, 0)
	require.NoError(t, err)
	es := exs.(*CircularExemplarStorage)

	e := exemplar.Exemplar{
		Labels: labels.FromStrings("trace_id", "qwerty"),
		Value:  0.1,
		Ts:     12,
	}

	lName0, lValue0 := "service", "asdf"
	lName1, lValue1 := "cluster", "us-central1"
	l := labels.FromStrings(lName0, lValue0, lName1, lValue1)

	// Lets just assume somehow the PromQL expression generated two separate lists of matchers,
	// both of which can select this particular series.
	m := [][]*labels.Matcher{
		{
			labels.MustNewMatcher(labels.MatchEqual, lName0, lValue0),
		},
		{
			labels.MustNewMatcher(labels.MatchEqual, lName1, lValue1),
		},
	}

	err = es.AddExemplar(l, e)
	require.NoError(t, err, "adding exemplar failed")
	require.True(t, reflect.DeepEqual(es.exemplars[0].exemplar, e), "exemplar was not stored correctly")

	ret, err := es.Select(0, 100, m...)
	require.NoError(t, err)
	require.Len(t, ret, 1, "select should have returned samples for a single series only")
}

func TestIndexOverwrite(t *testing.T) {
	exs, err := NewCircularExemplarStorage(2, eMetrics, 0)
	require.NoError(t, err)
	es := exs.(*CircularExemplarStorage)

	l1 := labels.FromStrings("service", "asdf")
	l2 := labels.FromStrings("service", "qwer")

	err = es.AddExemplar(l1, exemplar.Exemplar{Value: 1, Ts: 1})
	require.NoError(t, err)
	err = es.AddExemplar(l2, exemplar.Exemplar{Value: 2, Ts: 2})
	require.NoError(t, err)
	err = es.AddExemplar(l2, exemplar.Exemplar{Value: 3, Ts: 3})
	require.NoError(t, err)

	// Ensure index GC'ing is taking place, there should no longer be any
	// index entry for series l1 since we just wrote two exemplars for series l2.
	_, ok := es.index[string(l1.Bytes(nil))]
	require.False(t, ok)
	require.Equal(t, &indexEntry{1, 0, l2}, es.index[string(l2.Bytes(nil))])

	err = es.AddExemplar(l1, exemplar.Exemplar{Value: 4, Ts: 4})
	require.NoError(t, err)

	i := es.index[string(l2.Bytes(nil))]
	require.Equal(t, &indexEntry{0, 0, l2}, i)
}

func TestResize(t *testing.T) {
	testCases := []struct {
		name              string
		startSize         int64
		newCount          int64
		expectedSeries    []int
		notExpectedSeries []int
		expectedMigrated  int
	}{
		{
			name:              "Grow",
			startSize:         100,
			newCount:          200,
			expectedSeries:    []int{99, 98, 1, 0},
			notExpectedSeries: []int{100},
			expectedMigrated:  100,
		},
		{
			name:              "Shrink",
			startSize:         100,
			newCount:          50,
			expectedSeries:    []int{99, 98, 50},
			notExpectedSeries: []int{49, 1, 0},
			expectedMigrated:  50,
		},
		{
			name:              "ShrinkToZero",
			startSize:         100,
			newCount:          0,
			expectedSeries:    []int{},
			notExpectedSeries: []int{},
			expectedMigrated:  0,
		},
		{
			name:              "Negative",
			startSize:         100,
			newCount:          -1,
			expectedSeries:    []int{},
			notExpectedSeries: []int{},
			expectedMigrated:  0,
		},
		{
			name:              "NegativeToNegative",
			startSize:         -1,
			newCount:          -2,
			expectedSeries:    []int{},
			notExpectedSeries: []int{},
			expectedMigrated:  0,
		},
		{
			name:              "GrowFromZero",
			startSize:         0,
			newCount:          10,
			expectedSeries:    []int{},
			notExpectedSeries: []int{},
			expectedMigrated:  0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			exs, err := NewCircularExemplarStorage(tc.startSize, eMetrics, 0)
			require.NoError(t, err)
			es := exs.(*CircularExemplarStorage)

			for i := 0; int64(i) < tc.startSize; i++ {
				err = es.AddExemplar(labels.FromStrings("service", strconv.Itoa(i)), exemplar.Exemplar{
					Value: float64(i),
					Ts:    int64(i),
				})
				require.NoError(t, err)
			}

			if testing.Verbose() {
				t.Logf("Buffer[before-resize]:\n%s", debugCircularBuffer(es))
			}
			resized := es.Resize(tc.newCount)
			if testing.Verbose() {
				t.Logf("Buffer[after-resize]:\n%s", debugCircularBuffer(es))
			}

			require.Equal(t, tc.expectedMigrated, resized)

			q, err := es.Querier(context.TODO())
			require.NoError(t, err)

			matchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "service", "")}

			for _, expected := range tc.expectedSeries {
				matchers[0].Value = strconv.Itoa(expected)
				ex, err := q.Select(0, math.MaxInt64, matchers)
				require.NoError(t, err)
				require.NotEmpty(t, ex)
			}

			for _, notExpected := range tc.notExpectedSeries {
				matchers[0].Value = strconv.Itoa(notExpected)
				ex, err := q.Select(0, math.MaxInt64, matchers)
				require.NoError(t, err)
				require.Empty(t, ex)
			}
		})
	}
}

func BenchmarkAddExemplar(b *testing.B) {
	// We need to include these labels since we do length calculation
	// before adding.
	exLabels := labels.FromStrings("trace_id", "89620921")

	for _, capacity := range []int{1000, 10000, 100000} {
		for _, n := range []int{10000, 100000, 1000000} {
			b.Run(fmt.Sprintf("%d/%d", n, capacity), func(b *testing.B) {
				for b.Loop() {
					b.StopTimer()
					exs, err := NewCircularExemplarStorage(int64(capacity), eMetrics, 0)
					require.NoError(b, err)
					es := exs.(*CircularExemplarStorage)
					var l labels.Labels
					b.StartTimer()

					for i := range n {
						if i%100 == 0 {
							l = labels.FromStrings("service", strconv.Itoa(i))
						}
						err = es.AddExemplar(l, exemplar.Exemplar{Value: float64(i), Ts: int64(i), Labels: exLabels})
						if err != nil {
							require.NoError(b, err)
						}
					}
				}
			})
		}
	}
}

func BenchmarkAddExemplar_OutOfOrder(b *testing.B) {
	// We need to include these labels since we do length calculation
	// before adding.
	exLabels := labels.FromStrings("trace_id", "89620921")

	const (
		capacity = 5000
	)

	fillOneSeries := func(es *CircularExemplarStorage) {
		for i := range capacity {
			e := exemplar.Exemplar{Value: float64(i), Ts: int64(i), Labels: exLabels}
			if err := es.AddExemplar(exLabels, e); err != nil {
				panic(err)
			}
		}
	}

	fillMultipleSeries := func(es *CircularExemplarStorage) {
		for i := range capacity {
			l := labels.FromStrings("service", strconv.Itoa(i))
			e := exemplar.Exemplar{Value: float64(i), Ts: int64(i), Labels: l}
			if err := es.AddExemplar(l, e); err != nil {
				panic(err)
			}
		}
	}

	outOfOrder := func(ts *int64, _ *labels.Labels) {
		switch *ts % 3 {
		case 0:
			return
		case 1:
			*ts = capacity - *ts
		case 2:
			*ts = (capacity - *ts) + 100
		}
	}

	reverseOrder := func(ts *int64, _ *labels.Labels) {
		*ts = capacity - *ts
	}

	multipleSeries := func(f func(*int64, *labels.Labels)) func(*int64, *labels.Labels) {
		return func(ts *int64, l *labels.Labels) {
			f(ts, l)
			*l = labels.FromStrings("service", strconv.Itoa(int(*ts)))
		}
	}

	for fillName, setup := range map[string]func(es *CircularExemplarStorage){
		"empty":         func(*CircularExemplarStorage) {},
		"full-one":      fillOneSeries,
		"full-multiple": fillMultipleSeries,
	} {
		for orderName, forEach := range map[string]func(ts *int64, l *labels.Labels){
			"in-order":           func(*int64, *labels.Labels) {},
			"reverse":            reverseOrder,
			"out-of-order":       outOfOrder,
			"multi-in-order":     multipleSeries(func(*int64, *labels.Labels) {}),
			"multi-reverse":      multipleSeries(reverseOrder),
			"multi-out-of-order": multipleSeries(outOfOrder),
		} {
			b.Run(fmt.Sprintf("%s/%s", fillName, orderName), func(b *testing.B) {
				exs, err := NewCircularExemplarStorage(int64(capacity), eMetrics, 100000)
				require.NoError(b, err)
				es := exs.(*CircularExemplarStorage)
				l := labels.FromStrings("service", "0")
				setup(es)
				b.ResetTimer()
				for b.Loop() {
					for i := range capacity {
						ts := int64(i)
						forEach(&ts, &l)
						err = es.AddExemplar(l, exemplar.Exemplar{Value: float64(i), Ts: ts, Labels: l})
						if err != nil {
							b.Fatalf("Failed to insert item %d %s: %v", i, l, err)
						}
					}
				}
			})
		}
	}
}

func BenchmarkResizeExemplars(b *testing.B) {
	testCases := []struct {
		name         string
		startSize    int64
		endSize      int64
		numExemplars int
	}{
		{
			name:         "grow",
			startSize:    100000,
			endSize:      200000,
			numExemplars: 150000,
		},
		{
			name:         "shrink",
			startSize:    100000,
			endSize:      50000,
			numExemplars: 100000,
		},
		{
			name:         "grow",
			startSize:    1000000,
			endSize:      2000000,
			numExemplars: 1500000,
		},
		{
			name:         "shrink",
			startSize:    1000000,
			endSize:      500000,
			numExemplars: 1000000,
		},
	}

	for _, tc := range testCases {
		b.Run(fmt.Sprintf("%s-%d-to-%d", tc.name, tc.startSize, tc.endSize), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				exs, err := NewCircularExemplarStorage(tc.startSize, eMetrics, 0)
				require.NoError(b, err)
				es := exs.(*CircularExemplarStorage)

				var l labels.Labels
				for i := 0; i < int(float64(tc.startSize)*float64(1.5)); i++ {
					if i%100 == 0 {
						l = labels.FromStrings("service", strconv.Itoa(i))
					}

					err = es.AddExemplar(l, exemplar.Exemplar{Value: float64(i), Ts: int64(i)})
					if err != nil {
						require.NoError(b, err)
					}
				}
				b.StartTimer()
				es.Resize(tc.endSize)
			}
		})
	}
}

// TestCircularExemplarStorage_Concurrent_AddExemplar_Resize tries to provoke a data race between AddExemplar and Resize.
// Run with race detection enabled.
func TestCircularExemplarStorage_Concurrent_AddExemplar_Resize(t *testing.T) {
	exs, err := NewCircularExemplarStorage(0, eMetrics, 0)
	require.NoError(t, err)
	es := exs.(*CircularExemplarStorage)

	l := labels.FromStrings("service", "asdf")
	e := exemplar.Exemplar{
		Labels: labels.FromStrings("trace_id", "qwerty"),
		Value:  0.1,
		Ts:     1,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	t.Cleanup(wg.Wait)

	started := make(chan struct{})

	go func() {
		defer wg.Done()

		<-started
		for range 100 {
			require.NoError(t, es.AddExemplar(l, e))
		}
	}()

	for i := range 100 {
		es.Resize(int64(i + 1))
		if i == 0 {
			close(started)
		}
	}
}

// debugCircularBuffer iterates all exemplars in the circular exemplar storage
// and returns them as a string. The textual representation contains index
// pointers and helps debugging exemplar storage.
func debugCircularBuffer(ce *CircularExemplarStorage) string {
	var sb strings.Builder
	for i, e := range ce.exemplars {
		if e.ref == nil {
			continue
		}
		sb.WriteString(fmt.Sprintf(
			"i: %d, ts: %d, next: %d, prev: %d",
			i, e.exemplar.Ts, e.next, e.prev,
		))
		for _, idx := range ce.index {
			if i == idx.newest {
				sb.WriteString(" <- newest " + idx.seriesLabels.String())
			}
			if i == idx.oldest {
				sb.WriteString(" <- oldest " + idx.seriesLabels.String())
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString(fmt.Sprintf("Next index: %d\n", ce.nextIndex))
	return sb.String()
}
