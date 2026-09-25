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
	"testing"

	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
)

// TestNHCBAsClassicCompatLayer covers the promql-nhcb-as-classic feature flag,
// which lets queries for classic histogram series also read NHCB data.
func TestNHCBAsClassicCompatLayer(t *testing.T) {
	newStorage := func(t testing.TB) storage.Storage {
		return storage.NewNHCBAsClassicStorage(teststorage.New(t))
	}

	for _, tc := range []struct {
		name  string
		input string
	}{
		{
			name: "NHCB only, classic queries are served from the NHCB",
			input: `
load 1m
	rpc_latency_seconds{job="a"}	{{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}x5

eval instant at 2m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{job="a", le="1.0"} 1
	rpc_latency_seconds_bucket{job="a", le="2.0"} 3
	rpc_latency_seconds_bucket{job="a", le="+Inf"} 4

eval instant at 2m rpc_latency_seconds_count
	rpc_latency_seconds_count{job="a"} 4

eval instant at 2m rpc_latency_seconds_sum
	rpc_latency_seconds_sum{job="a"} 6

eval instant at 2m histogram_quantile(0.5, rpc_latency_seconds_bucket)
	{job="a"} 1.5

# The NHCB itself is still queryable under its own name.
eval instant at 2m rpc_latency_seconds
	rpc_latency_seconds{job="a"} {{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}
`,
		},
		{
			// The le label of a converted bucket is rendered in the OpenMetrics
			// float format, which does not necessarily match how the classic
			// histogram used to expose it.
			name: "NHCB only, le matchers must use the normalized bucket boundary",
			input: `
load 1m
	rpc_latency_seconds{job="a"}	{{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}x5

eval instant at 2m rpc_latency_seconds_bucket{le="1.0"}
	rpc_latency_seconds_bucket{job="a", le="1.0"} 1

# A dashboard written against the classic exposition, which used le="1", finds nothing.
eval instant at 2m rpc_latency_seconds_bucket{le="1"}
`,
		},
		{
			name: "NHCB only, converted series go stale with the NHCB",
			input: `
load 1m
	rpc_latency_seconds{job="a"}	{{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}x2 stale

eval instant at 2m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{job="a", le="1.0"} 1
	rpc_latency_seconds_bucket{job="a", le="2.0"} 3
	rpc_latency_seconds_bucket{job="a", le="+Inf"} 4

# Without the stale markers, the converted series would be returned until the
# end of the lookback window.
eval instant at 3m rpc_latency_seconds_bucket

eval instant at 3m rpc_latency_seconds_count

eval instant at 3m rpc_latency_seconds_sum

eval instant at 3m rpc_latency_seconds
`,
		},
		{
			name: "NHCB only, bucket layout change",
			input: `
load 1m
	rpc_latency_seconds{job="a"}	{{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}x2 {{schema:-53 sum:6 count:4 custom_values:[1 4] buckets:[1 2 1]}}x2

eval instant at 1m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{job="a", le="1.0"} 1
	rpc_latency_seconds_bucket{job="a", le="2.0"} 3
	rpc_latency_seconds_bucket{job="a", le="+Inf"} 4

# The le="2.0" bucket, which the NHCB does not have anymore, is stale.
eval instant at 4m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{job="a", le="1.0"} 1
	rpc_latency_seconds_bucket{job="a", le="4.0"} 3
	rpc_latency_seconds_bucket{job="a", le="+Inf"} 4

eval instant at 4m histogram_quantile(0.5, rpc_latency_seconds_bucket)
	{job="a"} 2.5
`,
		},
		{
			// Regression test: a partially migrated metric must not hide the
			// series that only exist in the other representation.
			name: "partially migrated metric, classic and NHCB series are both returned",
			input: `
load 1m
	rpc_latency_seconds_bucket{job="classic", le="1"}	1x5
	rpc_latency_seconds_bucket{job="classic", le="+Inf"}	4x5
	rpc_latency_seconds{job="native"}	{{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}x5

eval instant at 2m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{job="classic", le="1"} 1
	rpc_latency_seconds_bucket{job="classic", le="+Inf"} 4
	rpc_latency_seconds_bucket{job="native", le="1.0"} 1
	rpc_latency_seconds_bucket{job="native", le="+Inf"} 4
`,
		},
		{
			// load_with_nhcb writes the classic series and the equivalent NHCB
			// at the same timestamps, i.e. the worst case of a migration.
			name: "classic and NHCB overlap in storage",
			input: `
load_with_nhcb 1m
	rpc_latency_seconds_bucket{le="1"}	1x5
	rpc_latency_seconds_bucket{le="+Inf"}	4x5
	rpc_latency_seconds_sum	6x5
	rpc_latency_seconds_count	4x5

# The stored +Inf bucket and the converted one are the same series at the same
# timestamp, so the query fails.
eval instant at 2m rpc_latency_seconds_bucket
	expect fail msg: vector cannot contain metrics with the same labelset

eval instant at 2m rpc_latency_seconds_count
	expect fail msg: vector cannot contain metrics with the same labelset

eval instant at 2m rate(rpc_latency_seconds_sum[3m])
	expect fail msg: vector cannot contain metrics with the same labelset

# Aggregations do not collide, they silently count both representations:
# 2 stored bucket series plus 2 converted ones instead of 2.
eval instant at 2m count(rpc_latency_seconds_bucket)
	{} 4

# Restricting the query to the finite buckets avoids the collision, but the
# result mixes the stored and the converted bucket boundaries.
eval instant at 2m rpc_latency_seconds_bucket{le!="+Inf"}
	rpc_latency_seconds_bucket{le="1"} 1
	rpc_latency_seconds_bucket{le="1.0"} 1
`,
		},
		{
			// The classic exposition was dropped at 2m, the NHCB starts at 8m,
			// i.e. the two representations never share a timestamp.
			name: "classic and NHCB are disjoint in time",
			input: `
load 1m
	rpc_latency_seconds_bucket{le="1"}	1 1 1
	rpc_latency_seconds_bucket{le="+Inf"}	4 4 4
	rpc_latency_seconds	_ _ _ _ _ _ _ _ {{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}x2

# Only the stored classic series is within the lookback window.
eval instant at 2m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{le="1"} 1
	rpc_latency_seconds_bucket{le="+Inf"} 4

# Only the NHCB is within the lookback window.
eval instant at 8m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{le="1.0"} 1
	rpc_latency_seconds_bucket{le="+Inf"} 4

# A range that only covers the NHCB is fine.
eval instant at 10m count_over_time(rpc_latency_seconds_bucket[3m])
	{le="1.0"} 3
	{le="+Inf"} 3

# A range that covers both representations collides on the +Inf bucket.
eval instant at 10m count_over_time(rpc_latency_seconds_bucket[11m])
	expect fail msg: vector cannot contain metrics with the same labelset
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			promqltest.RunTestWithStorage(t, tc.input, newTestEngine(t), newStorage)
		})
	}
}

// TestClassicAsNHCBCompatLayer covers the promql-classic-as-nhcb feature flag,
// which lets queries for native histograms also read classic histogram data.
func TestClassicAsNHCBCompatLayer(t *testing.T) {
	newStorage := func(t testing.TB) storage.Storage {
		return storage.NewClassicAsNHCBStorage(teststorage.New(t))
	}

	for _, tc := range []struct {
		name  string
		input string
	}{
		{
			name: "classic only, native queries are served from the classic series",
			input: `
load 1m
	rpc_latency_seconds_bucket{job="a", le="1"}	1x5
	rpc_latency_seconds_bucket{job="a", le="2"}	3x5
	rpc_latency_seconds_bucket{job="a", le="+Inf"}	4x5
	rpc_latency_seconds_sum{job="a"}	6x5
	rpc_latency_seconds_count{job="a"}	4x5

eval instant at 2m rpc_latency_seconds
	rpc_latency_seconds{job="a"} {{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}

eval instant at 2m histogram_quantile(0.5, rpc_latency_seconds)
	{job="a"} 1.5

eval instant at 2m histogram_count(rpc_latency_seconds)
	{job="a"} 4

eval instant at 2m histogram_sum(rpc_latency_seconds)
	{job="a"} 6

# The classic series themselves are not touched.
eval instant at 2m rpc_latency_seconds_bucket{le="2"}
	rpc_latency_seconds_bucket{job="a", le="2"} 3

eval instant at 2m rpc_latency_seconds_count
	rpc_latency_seconds_count{job="a"} 4

# Native histograms have no le label, so a le matcher disables the conversion.
eval instant at 2m rpc_latency_seconds{le="1"}
`,
		},
		{
			name: "classic only, count and sum are optional",
			input: `
load 1m
	rpc_latency_seconds_bucket{le="1"}	1x5
	rpc_latency_seconds_bucket{le="+Inf"}	4x5

# The count is taken from the +Inf bucket, the sum stays at 0.
eval instant at 2m rpc_latency_seconds
	rpc_latency_seconds{} {{schema:-53 count:4 custom_values:[1] buckets:[1 3]}}
`,
		},
		{
			name: "classic histogram that cannot be converted",
			input: `
load 1m
	rpc_latency_seconds_bucket{le="+Inf"}	5x5
	rpc_latency_seconds_count	7x5

# The _count contradicts the +Inf bucket, so no NHCB can be built.
eval instant at 2m rpc_latency_seconds
	expect warn regex: .*classic histogram could not be converted.*count mismatch.*

# The classic series are still queryable.
eval instant at 2m rpc_latency_seconds_count
	rpc_latency_seconds_count{} 7
`,
		},
		{
			name: "classic only, the converted NHCB goes stale with the classic series",
			input: `
load 1m
	rpc_latency_seconds_bucket{le="1"}	1x2 stale
	rpc_latency_seconds_bucket{le="+Inf"}	4x2 stale
	rpc_latency_seconds_sum	6x2 stale
	rpc_latency_seconds_count	4x2 stale

eval instant at 2m rpc_latency_seconds
	expect no_warn
	rpc_latency_seconds{} {{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}

# Without the stale marker, the converted NHCB would be returned until the end
# of the lookback window.
eval instant at 3m rpc_latency_seconds
	expect no_warn
`,
		},
		{
			name: "classic only, bucket layout change",
			input: `
load 1m
	rpc_latency_seconds_bucket{le="1"}	1x5
	rpc_latency_seconds_bucket{le="2"}	3x2 stale
	rpc_latency_seconds_bucket{le="+Inf"}	4x5
	rpc_latency_seconds_sum	6x5
	rpc_latency_seconds_count	4x5

eval instant at 1m rpc_latency_seconds
	expect no_warn
	rpc_latency_seconds{} {{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}

# The le="2" bucket went stale at 3m, the other series are converted.
eval instant at 3m rpc_latency_seconds
	expect no_warn
	rpc_latency_seconds{} {{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}
`,
		},
		{
			// Regression test: a partially migrated metric must not hide the
			// series that only exist in the other representation.
			name: "partially migrated metric, classic and NHCB series are both returned",
			input: `
load 1m
	rpc_latency_seconds_bucket{job="classic", le="1"}	1x5
	rpc_latency_seconds_bucket{job="classic", le="+Inf"}	4x5
	rpc_latency_seconds{job="native"}	{{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}x5

eval instant at 2m rpc_latency_seconds
	rpc_latency_seconds{job="classic"} {{schema:-53 count:4 custom_values:[1] buckets:[1 3]}}
	rpc_latency_seconds{job="native"} {{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}
`,
		},
		{
			name: "classic and NHCB overlap in storage",
			input: `
load_with_nhcb 1m
	rpc_latency_seconds_bucket{le="1"}	1x5
	rpc_latency_seconds_bucket{le="+Inf"}	4x5
	rpc_latency_seconds_sum	6x5
	rpc_latency_seconds_count	4x5

# The stored NHCB and the one converted from the classic series are the same
# series at the same timestamp, so the query fails.
eval instant at 2m rpc_latency_seconds
	expect fail msg: vector cannot contain metrics with the same labelset

eval instant at 2m histogram_quantile(0.5, rpc_latency_seconds)
	expect fail msg: vector cannot contain metrics with the same labelset

# Aggregations do not collide, they silently count both representations.
eval instant at 2m count(rpc_latency_seconds)
	{} 2

# Classic queries keep working, they are never converted.
eval instant at 2m rpc_latency_seconds_count
	rpc_latency_seconds_count{} 4
`,
		},
		{
			// The classic exposition was dropped at 2m, the NHCB starts at 8m,
			// i.e. the two representations never share a timestamp.
			name: "classic and NHCB are disjoint in time",
			input: `
load 1m
	rpc_latency_seconds_bucket{le="1"}	1 1 1
	rpc_latency_seconds_bucket{le="+Inf"}	4 4 4
	rpc_latency_seconds	_ _ _ _ _ _ _ _ {{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}x2

# Only the converted classic series is within the lookback window.
eval instant at 2m rpc_latency_seconds
	rpc_latency_seconds{} {{schema:-53 count:4 custom_values:[1] buckets:[1 3]}}

# Only the stored NHCB is within the lookback window.
eval instant at 8m rpc_latency_seconds
	rpc_latency_seconds{} {{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}

# A range that only covers the NHCB is fine.
eval instant at 10m count_over_time(rpc_latency_seconds[3m])
	{} 3

# A range that covers both representations collides.
eval instant at 10m count_over_time(rpc_latency_seconds[11m])
	expect fail msg: vector cannot contain metrics with the same labelset
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			promqltest.RunTestWithStorage(t, tc.input, newTestEngine(t), newStorage)
		})
	}
}

// TestNHClassicCompatLayer covers the promql-nh-classic-compat feature flag,
// which makes native and classic histograms interchangeable in queries: classic
// histogram queries also read native histograms, both NHCB and exponential
// ones, and native histogram queries also read classic histograms.
func TestNHClassicCompatLayer(t *testing.T) {
	newStorage := func(t testing.TB) storage.Storage {
		return storage.NewNHClassicCompatStorage(teststorage.New(t))
	}

	for _, tc := range []struct {
		name  string
		input string
	}{
		{
			name: "classic only",
			input: `
load 1m
	rpc_latency_seconds_bucket{job="a", le="1"}	1x5
	rpc_latency_seconds_bucket{job="a", le="2"}	3x5
	rpc_latency_seconds_bucket{job="a", le="+Inf"}	4x5
	rpc_latency_seconds_sum{job="a"}	6x5
	rpc_latency_seconds_count{job="a"}	4x5

# Regression test: the classic series are returned once, not a second time
# converted to NHCB and back.
eval instant at 2m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{job="a", le="1"} 1
	rpc_latency_seconds_bucket{job="a", le="2"} 3
	rpc_latency_seconds_bucket{job="a", le="+Inf"} 4

eval instant at 2m rpc_latency_seconds_count
	rpc_latency_seconds_count{job="a"} 4

eval instant at 2m histogram_quantile(0.5, rpc_latency_seconds_bucket)
	{job="a"} 1.5

eval instant at 2m rpc_latency_seconds
	rpc_latency_seconds{job="a"} {{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}

eval instant at 2m histogram_quantile(0.5, rpc_latency_seconds)
	{job="a"} 1.5
`,
		},
		{
			name: "NHCB only",
			input: `
load 1m
	rpc_latency_seconds{job="a"}	{{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}x5

eval instant at 2m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{job="a", le="1.0"} 1
	rpc_latency_seconds_bucket{job="a", le="2.0"} 3
	rpc_latency_seconds_bucket{job="a", le="+Inf"} 4

eval instant at 2m rpc_latency_seconds_sum
	rpc_latency_seconds_sum{job="a"} 6

eval instant at 2m histogram_quantile(0.5, rpc_latency_seconds_bucket)
	{job="a"} 1.5

# The NHCB is returned once, not a second time converted to classic series and
# back.
eval instant at 2m rpc_latency_seconds
	rpc_latency_seconds{job="a"} {{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}
`,
		},
		{
			// The buckets are (0.5,1], (1,2] and (2,4].
			name: "exponential only",
			input: `
load 1m
	rpc_latency_seconds{job="a"}	{{schema:0 sum:6 count:4 buckets:[1 2 1]}}+{{schema:0 sum:6 count:4 buckets:[1 2 1]}}x10

# The lower boundary of the lowest bucket is a classic bucket, too.
eval instant at 10m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{job="a", le="0.5"} 0
	rpc_latency_seconds_bucket{job="a", le="1.0"} 11
	rpc_latency_seconds_bucket{job="a", le="2.0"} 33
	rpc_latency_seconds_bucket{job="a", le="4.0"} 44
	rpc_latency_seconds_bucket{job="a", le="+Inf"} 44

eval instant at 10m rpc_latency_seconds_count
	rpc_latency_seconds_count{job="a"} 44

eval instant at 10m rpc_latency_seconds_sum
	rpc_latency_seconds_sum{job="a"} 66

eval instant at 10m rate(rpc_latency_seconds_count[5m])
	{job="a"} 0.06666666666666667

# histogram_quantile interpolates linearly within a classic bucket, but
# exponentially within an exponential one.
eval instant at 10m histogram_quantile(0.5, sum by (le) (rate(rpc_latency_seconds_bucket[5m])))
	{} 1.5

eval instant at 10m histogram_quantile(0.5, sum(rate(rpc_latency_seconds[5m])))
	{} 1.4142135623730951

eval instant at 10m rpc_latency_seconds
	rpc_latency_seconds{job="a"} {{schema:0 sum:66 count:44 buckets:[11 22 11]}}
`,
		},
		{
			name: "partially migrated metric, classic, NHCB and exponential histograms",
			input: `
load 1m
	rpc_latency_seconds_bucket{job="classic", le="1"}	1x5
	rpc_latency_seconds_bucket{job="classic", le="2"}	3x5
	rpc_latency_seconds_bucket{job="classic", le="+Inf"}	4x5
	rpc_latency_seconds_sum{job="classic"}	6x5
	rpc_latency_seconds_count{job="classic"}	4x5
	rpc_latency_seconds{job="nhcb"}	{{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}x5
	rpc_latency_seconds{job="exponential"}	{{schema:0 sum:6 count:4 buckets:[1 2 1]}}x5

eval instant at 2m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{job="classic", le="1"} 1
	rpc_latency_seconds_bucket{job="classic", le="2"} 3
	rpc_latency_seconds_bucket{job="classic", le="+Inf"} 4
	rpc_latency_seconds_bucket{job="nhcb", le="1.0"} 1
	rpc_latency_seconds_bucket{job="nhcb", le="2.0"} 3
	rpc_latency_seconds_bucket{job="nhcb", le="+Inf"} 4
	rpc_latency_seconds_bucket{job="exponential", le="0.5"} 0
	rpc_latency_seconds_bucket{job="exponential", le="1.0"} 1
	rpc_latency_seconds_bucket{job="exponential", le="2.0"} 3
	rpc_latency_seconds_bucket{job="exponential", le="4.0"} 4
	rpc_latency_seconds_bucket{job="exponential", le="+Inf"} 4

eval instant at 2m histogram_quantile(0.5, rpc_latency_seconds_bucket)
	{job="classic"} 1.5
	{job="nhcb"} 1.5
	{job="exponential"} 1.5

eval instant at 2m sum(rpc_latency_seconds_count)
	{} 12

eval instant at 2m rpc_latency_seconds
	rpc_latency_seconds{job="classic"} {{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}
	rpc_latency_seconds{job="nhcb"} {{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}
	rpc_latency_seconds{job="exponential"} {{schema:0 sum:6 count:4 buckets:[1 2 1]}}

eval instant at 2m histogram_quantile(0.5, rpc_latency_seconds)
	{job="classic"} 1.5
	{job="nhcb"} 1.5
	{job="exponential"} 1.4142135623730951

eval instant at 2m sum(histogram_count(rpc_latency_seconds))
	{} 12
`,
		},
		{
			// The buckets of job a are (0.5,1] and (1,2], the one of job b is
			// (2,4].
			name: "exponential histograms with different buckets can be aggregated by le",
			input: `
load 1m
	rpc_latency_seconds{job="a"}	{{schema:0 sum:4 count:3 buckets:[1 2]}}x5
	rpc_latency_seconds{job="b"}	{{schema:0 sum:3 count:1 offset:2 buckets:[1]}}x5

# Both are converted with the union of their buckets.
eval instant at 2m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{job="a", le="0.5"} 0
	rpc_latency_seconds_bucket{job="a", le="1.0"} 1
	rpc_latency_seconds_bucket{job="a", le="2.0"} 3
	rpc_latency_seconds_bucket{job="a", le="4.0"} 3
	rpc_latency_seconds_bucket{job="a", le="+Inf"} 3
	rpc_latency_seconds_bucket{job="b", le="0.5"} 0
	rpc_latency_seconds_bucket{job="b", le="1.0"} 0
	rpc_latency_seconds_bucket{job="b", le="2.0"} 0
	rpc_latency_seconds_bucket{job="b", le="4.0"} 1
	rpc_latency_seconds_bucket{job="b", le="+Inf"} 1

eval instant at 2m sum by (le) (rpc_latency_seconds_bucket)
	{le="0.5"} 0
	{le="1.0"} 1
	{le="2.0"} 3
	{le="4.0"} 4
	{le="+Inf"} 4

eval instant at 2m histogram_quantile(0.9, sum by (le) (rpc_latency_seconds_bucket))
	expect no_info
	{} 3.2

eval instant at 2m histogram_quantile(0.9, sum(rpc_latency_seconds))
	{} 3.0314331330207964
`,
		},
		{
			// The resolution is reduced from schema 0 to -1 at 3m. The buckets
			// of schema -1 are (0.25,1] and (1,4].
			name: "exponential histogram with a schema change",
			input: `
load 1m
	rpc_latency_seconds	{{schema:0 sum:6 count:4 buckets:[1 2 1]}}x2 {{schema:-1 sum:6 count:4 buckets:[1 3]}}x5

# Only schema 0 samples are selected.
eval instant at 1m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{le="0.5"} 0
	rpc_latency_seconds_bucket{le="1.0"} 1
	rpc_latency_seconds_bucket{le="2.0"} 3
	rpc_latency_seconds_bucket{le="4.0"} 4
	rpc_latency_seconds_bucket{le="+Inf"} 4

eval instant at 1m histogram_quantile(0.5, rpc_latency_seconds_bucket)
	{} 1.5

# As soon as a schema -1 sample is selected, all samples are converted with
# the buckets of schema -1, which lowers the resolution.
eval instant at 4m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{le="0.25"} 0
	rpc_latency_seconds_bucket{le="1.0"} 1
	rpc_latency_seconds_bucket{le="4.0"} 4
	rpc_latency_seconds_bucket{le="+Inf"} 4

eval instant at 4m histogram_quantile(0.5, rpc_latency_seconds_bucket)
	{} 2

eval range from 0 to 4m step 1m rpc_latency_seconds_bucket
	rpc_latency_seconds_bucket{le="0.25"} 0x4
	rpc_latency_seconds_bucket{le="1.0"} 1x4
	rpc_latency_seconds_bucket{le="4.0"} 4x4
	rpc_latency_seconds_bucket{le="+Inf"} 4x4
`,
		},
		{
			name: "converted series go stale with the stored ones",
			input: `
load 1m
	rpc_latency_seconds_bucket{job="classic", le="1"}	1x2 stale
	rpc_latency_seconds_bucket{job="classic", le="+Inf"}	4x2 stale
	rpc_latency_seconds_sum{job="classic"}	6x2 stale
	rpc_latency_seconds_count{job="classic"}	4x2 stale
	rpc_latency_seconds{job="exponential"}	{{schema:0 sum:6 count:4 buckets:[1 2 1]}}x2 stale

eval instant at 2m rpc_latency_seconds_count
	rpc_latency_seconds_count{job="classic"} 4
	rpc_latency_seconds_count{job="exponential"} 4

eval instant at 2m rpc_latency_seconds
	rpc_latency_seconds{job="classic"} {{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}
	rpc_latency_seconds{job="exponential"} {{schema:0 sum:6 count:4 buckets:[1 2 1]}}

eval instant at 3m rpc_latency_seconds_bucket
	expect no_warn

eval instant at 3m rpc_latency_seconds_count
	expect no_warn

eval instant at 3m rpc_latency_seconds
	expect no_warn
`,
		},
		{
			// load_with_nhcb writes the classic series and the equivalent NHCB
			// at the same timestamps, i.e. the worst case of a migration.
			name: "classic and NHCB overlap in storage",
			input: `
load_with_nhcb 1m
	rpc_latency_seconds_bucket{le="1"}	1x5
	rpc_latency_seconds_bucket{le="+Inf"}	4x5
	rpc_latency_seconds_sum	6x5
	rpc_latency_seconds_count	4x5

# The converted series are returned in addition to the stored ones, in both
# directions.
eval instant at 2m rpc_latency_seconds_bucket
	expect fail msg: vector cannot contain metrics with the same labelset

eval instant at 2m rpc_latency_seconds
	expect fail msg: vector cannot contain metrics with the same labelset
`,
		},
		{
			// E.g. with always_scrape_classic_histograms enabled.
			name: "classic and exponential histograms scraped side by side",
			input: `
load 1m
	rpc_latency_seconds_bucket{le="1"}	1x5
	rpc_latency_seconds_bucket{le="2"}	3x5
	rpc_latency_seconds_bucket{le="+Inf"}	4x5
	rpc_latency_seconds_sum	6x5
	rpc_latency_seconds_count	4x5
	rpc_latency_seconds	{{schema:0 sum:6 count:4 buckets:[1 2 1]}}x5

eval instant at 2m rpc_latency_seconds_count
	expect fail msg: vector cannot contain metrics with the same labelset

eval instant at 2m rpc_latency_seconds
	expect fail msg: vector cannot contain metrics with the same labelset

# The finite buckets do not collide, but mix both bucket layouts.
eval instant at 2m rpc_latency_seconds_bucket{le!="+Inf"}
	rpc_latency_seconds_bucket{le="1"} 1
	rpc_latency_seconds_bucket{le="2"} 3
	rpc_latency_seconds_bucket{le="0.5"} 0
	rpc_latency_seconds_bucket{le="1.0"} 1
	rpc_latency_seconds_bucket{le="2.0"} 3
	rpc_latency_seconds_bucket{le="4.0"} 4
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			promqltest.RunTestWithStorage(t, tc.input, newTestEngine(t), newStorage)
		})
	}
}
