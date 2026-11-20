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

package main

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/promqltest"
)

func TestCheckClassicHistograms(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
		load 1m
			ok_bucket{le="1"} 5
			ok_bucket{le="2"} 15
			ok_bucket{le="+Inf"} 42
			ok_count 42
			nonmono_bucket{le="1"} 5
			nonmono_bucket{le="2"} 15
			nonmono_bucket{le="3"} 35
			nonmono_bucket{le="+Inf"} 7
			nonmono_count 7
			mismatch_bucket{le="1"} 5
			mismatch_bucket{le="+Inf"} 35
			mismatch_count 42
			badle_bucket{le="NaN"} 5
			badle_bucket{le="+Inf"} 5
			badle_count 5
			noinf_bucket{le="1"} 3
			noinf_bucket{le="2"} 7
			noinf_count 7
			migrated_bucket{le="1"} 5 _
			migrated_bucket{le="1.0"} _ 6
			migrated_bucket{le="+Inf"} 9 9
			migrated_count 9 9
			duple_bucket{le="1"} 5
			duple_bucket{le="1.0"} 6
			duple_bucket{le="+Inf"} 9
			duple_count 9
			weird_bucket_extra{le="5"} 1
	`)
	t.Cleanup(func() { storage.Close() })

	findings, err := checkClassicHistograms(context.Background(), storage.Dir(), "", math.MinInt64, math.MaxInt64)
	require.NoError(t, err)

	kindsByMetric := map[string]map[string]int{}
	for _, f := range findings {
		if kindsByMetric[f.metric] == nil {
			kindsByMetric[f.metric] = map[string]int{}
		}
		kindsByMetric[f.metric][f.kind]++
	}

	tests := []struct {
		metric   string
		expected map[string]int
	}{
		// The control: a conforming histogram yields no findings at all.
		{metric: "ok", expected: nil},
		// Cumulative counts decrease at le="+Inf" (35 -> 7).
		{metric: "nonmono", expected: map[string]int{findingNonMonotonic: 1}},
		// le="+Inf" (35) disagrees with _count (42).
		{metric: "mismatch", expected: map[string]int{findingCountMismatch: 1}},
		// A bucket label that does not parse as a float.
		{metric: "badle", expected: map[string]int{findingUnparsableLe: 1}},
		// No le="+Inf" bucket stored at all.
		{metric: "noinf", expected: map[string]int{findingMissingInf: 1}},
		// le="1" and le="1.0" on disjoint timestamps: the v3 spelling
		// migration shape, not an inconsistency.
		{metric: "migrated", expected: nil},
		// The same two spellings carrying different values at the same
		// timestamp is a real conflict.
		{metric: "duple", expected: map[string]int{findingDuplicateLe: 1}},
		// A name where _bucket is not the suffix must not be scanned at
		// all: the selector regexp is anchored.
		{metric: "weird_bucket_extra", expected: nil},
	}
	for _, tt := range tests {
		t.Run(tt.metric, func(t *testing.T) {
			if tt.expected == nil {
				require.Empty(t, kindsByMetric[tt.metric])
				return
			}
			require.Equal(t, tt.expected, kindsByMetric[tt.metric])
		})
	}
}

func TestPrintClassicHistogramChecks(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
		load 1m
			bad_bucket{le="1"} 9
			bad_bucket{le="+Inf"} 3
			bad_count 3
	`)
	t.Cleanup(func() { storage.Close() })

	err := printClassicHistogramChecks(context.Background(), storage.Dir(), "", math.MinInt64, math.MaxInt64)
	require.ErrorContains(t, err, "classic histogram inconsistencies")

	clean := promqltest.LoadedStorage(t, `
		load 1m
			good_bucket{le="1"} 1
			good_bucket{le="+Inf"} 3
			good_count 3
	`)
	t.Cleanup(func() { clean.Close() })
	require.NoError(t, printClassicHistogramChecks(context.Background(), clean.Dir(), "", math.MinInt64, math.MaxInt64))
}
