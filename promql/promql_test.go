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

package promql_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/util/teststorage"
)

func newTestEngine(t *testing.T) *promql.Engine {
	return promqltest.NewTestEngine(t, false, 0, promqltest.DefaultMaxSamplesPerQuery)
}

func TestEvaluations(t *testing.T) {
	promqltest.RunBuiltinTests(t, newTestEngine(t))
}

// Run a lot of queries at the same time, to check for race conditions.
func TestConcurrentRangeQueries(t *testing.T) {
	stor := teststorage.New(t)

	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 50000000,
		Timeout:    100 * time.Second,
	}
	// Enable experimental functions testing
	parser.EnableExperimentalFunctions = true
	parser.EnableExtendedRangeSelectors = true
	t.Cleanup(func() {
		parser.EnableExperimentalFunctions = false
		parser.EnableExtendedRangeSelectors = false
	})
	engine := promqltest.NewTestEngineWithOpts(t, opts)

	const interval = 10000 // 10s interval.
	// A day of data plus 10k steps.
	numIntervals := 8640 + 10000
	err := setupRangeQueryTestData(stor, engine, interval, numIntervals)
	require.NoError(t, err)

	cases := rangeQueryCases()

	// Limit the number of queries running at the same time.
	const numConcurrent = 4
	sem := make(chan struct{}, numConcurrent)
	for range numConcurrent {
		sem <- struct{}{}
	}
	var g errgroup.Group
	for _, c := range cases {
		if strings.Contains(c.expr, "count_values") && c.steps > 10 {
			continue // This test is too big to run with -race.
		}
		if strings.Contains(c.expr, "[1d]") && c.steps > 100 {
			continue // This test is too slow.
		}
		<-sem
		g.Go(func() error {
			defer func() {
				sem <- struct{}{}
			}()
			ctx := context.Background()
			qry, err := engine.NewRangeQuery(
				ctx, stor, nil, c.expr,
				time.Unix(int64((numIntervals-c.steps)*10), 0),
				time.Unix(int64(numIntervals*10), 0), time.Second*10)
			if err != nil {
				return err
			}
			res := qry.Exec(ctx)
			if res.Err != nil {
				t.Logf("Query: %q, steps: %d, result: %s", c.expr, c.steps, res.Err)
				return res.Err
			}
			qry.Close()
			return nil
		})
	}

	err = g.Wait()
	require.NoError(t, err)
}
