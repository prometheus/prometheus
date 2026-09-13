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

package rules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestGroup_Equals(t *testing.T) {
	tests := map[string]struct {
		first    *Group
		second   *Group
		expected bool
	}{
		"no query offset set on both groups": {
			first: &Group{
				name:     "group-1",
				file:     "file-1",
				interval: time.Minute,
			},
			second: &Group{
				name:     "group-1",
				file:     "file-1",
				interval: time.Minute,
			},
			expected: true,
		},
		"query offset set only on the first group": {
			first: &Group{
				name:        "group-1",
				file:        "file-1",
				interval:    time.Minute,
				queryOffset: pointerOf[time.Duration](time.Minute),
			},
			second: &Group{
				name:     "group-1",
				file:     "file-1",
				interval: time.Minute,
			},
			expected: false,
		},
		"query offset set on both groups to the same value": {
			first: &Group{
				name:        "group-1",
				file:        "file-1",
				interval:    time.Minute,
				queryOffset: pointerOf[time.Duration](time.Minute),
			},
			second: &Group{
				name:        "group-1",
				file:        "file-1",
				interval:    time.Minute,
				queryOffset: pointerOf[time.Duration](time.Minute),
			},
			expected: true,
		},
		"query offset set on both groups to different value": {
			first: &Group{
				name:        "group-1",
				file:        "file-1",
				interval:    time.Minute,
				queryOffset: pointerOf[time.Duration](time.Minute),
			},
			second: &Group{
				name:        "group-1",
				file:        "file-1",
				interval:    time.Minute,
				queryOffset: pointerOf[time.Duration](2 * time.Minute),
			},
			expected: false,
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			require.Equal(t, testData.expected, testData.first.Equals(testData.second))
			require.Equal(t, testData.expected, testData.second.Equals(testData.first))
		})
	}
}

func pointerOf[T any](value T) *T {
	return &value
}

func TestGroup_PartialEvaluationStrategy(t *testing.T) {
	storage := teststorage.New(t)
	engine := testEngine(t)

	firstExpr, err := testParser.ParseExpr("vector(0)")
	require.NoError(t, err)
	secondExpr, err := testParser.ParseExpr("vector(1)")
	require.NoError(t, err)

	tests := []struct {
		name                  string
		strategy              rulefmt.PartialEvaluationStrategy
		expectSecondEvaluated bool
	}{
		{
			name:                  "independent",
			strategy:              rulefmt.PartialEvaluationStrategyIndependent,
			expectSecondEvaluated: true,
		},
		{
			name:                  "abort",
			strategy:              rulefmt.PartialEvaluationStrategyAbort,
			expectSecondEvaluated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var secondEvaluated bool
			base := EngineQueryFunc(engine, storage)
			qf := func(ctx context.Context, q string, ts time.Time) (promql.Vector, error) {
				if q == "vector(0)" {
					return nil, errors.New("boom")
				}
				if q == "vector(1)" {
					secondEvaluated = true
				}
				return base(ctx, q, ts)
			}

			g := NewGroup(GroupOptions{
				Name:                "test",
				Interval:            time.Minute,
				PartialEvalStrategy: tt.strategy,
				Rules: []Rule{
					NewRecordingRule("first", firstExpr, labels.EmptyLabels()),
					NewRecordingRule("second", secondExpr, labels.EmptyLabels()),
				},
				Opts: &ManagerOptions{
					Appendable: storage,
					Queryable:  storage,
					QueryFunc:  qf,
					Logger:     promslog.NewNopLogger(),
				},
			})

			g.Eval(context.Background(), time.Unix(0, 0))
			require.Equal(t, tt.expectSecondEvaluated, secondEvaluated, "second rule evaluation mismatch")
		})
	}
}
