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

package histogramconv_test

import (
	"testing"
	"time"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage/histogramconv"
)

// TestPromQL covers PromQL queries with query-time histogram conversion. The
// case names start with the representations converted from.
func TestPromQL(t *testing.T) {
	for _, tc := range []struct {
		name        string
		convertFrom []histogramconv.Representation
		// disabled disables query-time histogram conversion.
		disabled bool
		input    string
	}{
		{
			name:        "disabled: nothing is converted if the feature is disabled",
			convertFrom: histogramconv.Representations(),
			disabled:    true,
			input: `
load 1m
	rpc_latency_seconds{job="a"}	{{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}x5
	rpc_latency_seconds_count{job="b"}	4x5

eval instant at 2m rpc_latency_seconds_count
	rpc_latency_seconds_count{job="b"} 4

eval instant at 2m rpc_latency_seconds
	rpc_latency_seconds{job="a"} {{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}

# Control matchers are matched against the stored series, which do not have
# the control labels.
eval instant at 2m rpc_latency_seconds_count{__convert_stored_as__="classic"}

eval instant at 2m rpc_latency_seconds_count{__debug_stored_as__="true"}
`,
		},
		{
			name: "none: nothing is converted by default",
			input: `
load 1m
	rpc_latency_seconds{job="a"}	{{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}x5
	rpc_latency_seconds_count{job="b"}	4x5

eval instant at 2m rpc_latency_seconds_count
	rpc_latency_seconds_count{job="b"} 4

eval instant at 2m rpc_latency_seconds
	rpc_latency_seconds{job="a"} {{schema:-53 sum:6 count:4 custom_values:[1 2] buckets:[1 2 1]}}

# A control matcher enables conversions that the flag does not list.
eval instant at 2m rpc_latency_seconds_count{__convert_stored_as__=~"classic|nhcb"}
	rpc_latency_seconds_count{job="a"} 4
	rpc_latency_seconds_count{job="b"} 4
`,
		},
		{
			// The representation of the stored series changes at 3m.
			name: "none: debug splits a stored series by representation",
			input: `
load 1m
	rpc_latency_seconds{job="a"}	{{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}x2 {{schema:0 sum:6 count:4 buckets:[1 2 1]}}x2

eval instant at 2m rpc_latency_seconds{__debug_stored_as__="true"}
	rpc_latency_seconds{job="a", __stored_as__="nhcb"} {{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}

# The NHCB part is marked stale where the exponential histograms start.
eval instant at 3m rpc_latency_seconds{__debug_stored_as__="true"}
	rpc_latency_seconds{job="a", __stored_as__="nhe"} {{schema:0 sum:6 count:4 buckets:[1 2 1]}}

eval instant at 2m rpc_latency_seconds{__convert_stored_as__="nhcb"}
	rpc_latency_seconds{job="a"} {{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}

eval instant at 3m rpc_latency_seconds{__convert_stored_as__="nhcb"}

eval range from 0 to 5m step 1m count_over_time(rpc_latency_seconds{__debug_stored_as__="true"}[2m])
	{job="a", __stored_as__="nhcb"} 1 2 2 1 _ _
	{job="a", __stored_as__="nhe"} _ _ _ 1 2 2
`,
		},
		{
			name:        "nhcb: NHCB only, classic queries are served from the NHCB",
			convertFrom: []histogramconv.Representation{histogramconv.NHCB},
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
			name:        "nhcb: NHCB only, le matchers must use the normalized bucket boundary",
			convertFrom: []histogramconv.Representation{histogramconv.NHCB},
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
			name:        "nhcb: NHCB only, converted series go stale with the NHCB",
			convertFrom: []histogramconv.Representation{histogramconv.NHCB},
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
			name:        "nhcb: NHCB only, bucket layout change",
			convertFrom: []histogramconv.Representation{histogramconv.NHCB},
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
			name:        "nhcb: partially migrated metric, classic and NHCB series are both returned",
			convertFrom: []histogramconv.Representation{histogramconv.NHCB},
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
			name:        "nhcb: classic and NHCB overlap in storage",
			convertFrom: []histogramconv.Representation{histogramconv.NHCB},
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
			name:        "nhcb: classic and NHCB are disjoint in time",
			convertFrom: []histogramconv.Representation{histogramconv.NHCB},
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
		{
			// The rows of the selector table in PROPOSAL.md.
			name:        "nhcb: control matchers select the representations per selector",
			convertFrom: []histogramconv.Representation{histogramconv.NHCB},
			input: `
load 1m
	rpc_latency_seconds_bucket{job="classic", le="1"}	1x5
	rpc_latency_seconds_bucket{job="classic", le="+Inf"}	4x5
	rpc_latency_seconds_count{job="classic"}	4x5
	rpc_latency_seconds{job="nhcb"}	{{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}x5
	rpc_latency_seconds{job="nhe"}	{{schema:0 sum:6 count:4 buckets:[1 2 1]}}x5

# Stored series, plus the conversions of the flag.
eval instant at 2m rpc_latency_seconds_count
	rpc_latency_seconds_count{job="classic"} 4
	rpc_latency_seconds_count{job="nhcb"} 4

# Stored series only.
eval instant at 2m rpc_latency_seconds_count{__convert_stored_as__="classic"}
	rpc_latency_seconds_count{job="classic"} 4

# Stored series, plus the series converted from NHCB.
eval instant at 2m rpc_latency_seconds_count{__convert_stored_as__=~"classic|nhcb"}
	rpc_latency_seconds_count{job="classic"} 4
	rpc_latency_seconds_count{job="nhcb"} 4

# Only the series converted from NHCB.
eval instant at 2m rpc_latency_seconds_count{__convert_stored_as__="nhcb"}
	rpc_latency_seconds_count{job="nhcb"} 4

# Only the series converted from exponential histograms, although the flag
# does not list them.
eval instant at 2m rpc_latency_seconds_count{__convert_stored_as__="nhe"}
	rpc_latency_seconds_count{job="nhe"} 4

# Stored native histograms only.
eval instant at 2m rpc_latency_seconds{__convert_stored_as__=~"nhcb|nhe"}
	rpc_latency_seconds{job="nhcb"} {{schema:-53 sum:6 count:4 custom_values:[1] buckets:[1 3]}}
	rpc_latency_seconds{job="nhe"} {{schema:0 sum:6 count:4 buckets:[1 2 1]}}

# Stored exponential histograms only.
eval instant at 2m rpc_latency_seconds{__convert_stored_as__="nhe"}
	rpc_latency_seconds{job="nhe"} {{schema:0 sum:6 count:4 buckets:[1 2 1]}}

# Only the NHCB converted from classic series.
eval instant at 2m rpc_latency_seconds{__convert_stored_as__="classic"}
	rpc_latency_seconds{job="classic"} {{schema:-53 count:4 custom_values:[1] buckets:[1 3]}}

# Same as rpc_latency_seconds_count, with __stored_as__ on every series.
eval instant at 2m rpc_latency_seconds_count{__debug_stored_as__="true"}
	rpc_latency_seconds_count{job="classic", __stored_as__="classic"} 4
	rpc_latency_seconds_count{job="nhcb", __stored_as__="nhcb"} 4

# Stored series, plus the series converted from every representation, with
# __stored_as__.
eval instant at 2m rpc_latency_seconds_count{__convert_stored_as__=~".*", __debug_stored_as__="true"}
	rpc_latency_seconds_count{job="classic", __stored_as__="classic"} 4
	rpc_latency_seconds_count{job="nhcb", __stored_as__="nhcb"} 4
	rpc_latency_seconds_count{job="nhe", __stored_as__="nhe"} 4

# A matcher that matches no representation selects nothing.
eval instant at 2m rpc_latency_seconds_count{__convert_stored_as__="nh"}

# __stored_as__ is a normal label: aggregations drop it, unless it is listed.
eval instant at 2m sum(rpc_latency_seconds_count{__convert_stored_as__=~".*", __debug_stored_as__="true"})
	{} 12

eval instant at 2m histogram_quantile(0.5, sum by (le, __stored_as__) (rpc_latency_seconds_bucket{__convert_stored_as__=~".*", __debug_stored_as__="true"}))
	{__stored_as__="classic"} 1
	{__stored_as__="nhcb"} 1
	{__stored_as__="nhe"} 1.5

# Matchers on __stored_as__ are matched against the stored series.
eval instant at 2m rpc_latency_seconds_count{__stored_as__="nhcb"}

# At least one matcher besides the control matchers must not match the empty
# value.
eval instant at 2m {__convert_stored_as__="nhcb"}
	expect fail msg: expanding series: vector selector must contain at least one non-empty matcher besides __convert_stored_as__ and __debug_stored_as__
`,
		},
		{
			name:        "classic: classic only, native queries are served from the classic series",
			convertFrom: []histogramconv.Representation{histogramconv.Classic},
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
			name:        "classic: classic only, count and sum are optional",
			convertFrom: []histogramconv.Representation{histogramconv.Classic},
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
			name:        "classic: classic histogram that cannot be converted",
			convertFrom: []histogramconv.Representation{histogramconv.Classic},
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
			name:        "classic: classic only, the converted NHCB goes stale with the classic series",
			convertFrom: []histogramconv.Representation{histogramconv.Classic},
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
			name:        "classic: classic only, bucket layout change",
			convertFrom: []histogramconv.Representation{histogramconv.Classic},
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
			name:        "classic: partially migrated metric, classic and NHCB series are both returned",
			convertFrom: []histogramconv.Representation{histogramconv.Classic},
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
			name:        "classic: classic and NHCB overlap in storage",
			convertFrom: []histogramconv.Representation{histogramconv.Classic},
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
			name:        "classic: classic and NHCB are disjoint in time",
			convertFrom: []histogramconv.Representation{histogramconv.Classic},
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
		{
			name:        "all: classic only",
			convertFrom: histogramconv.Representations(),
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
			name:        "all: NHCB only",
			convertFrom: histogramconv.Representations(),
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
			name:        "all: exponential only",
			convertFrom: histogramconv.Representations(),
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
			name:        "all: partially migrated metric, classic, NHCB and exponential histograms",
			convertFrom: histogramconv.Representations(),
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
			name:        "all: exponential histograms with different buckets can be aggregated by le",
			convertFrom: histogramconv.Representations(),
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
			name:        "all: exponential histogram with a schema change",
			convertFrom: histogramconv.Representations(),
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
			name:        "all: converted series go stale with the stored ones",
			convertFrom: histogramconv.Representations(),
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
			name:        "all: classic and NHCB overlap in storage",
			convertFrom: histogramconv.Representations(),
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
			name:        "all: classic and exponential histograms scraped side by side",
			convertFrom: histogramconv.Representations(),
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
			promqltest.RunTest(t, tc.input, promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
				MaxSamples:                promqltest.DefaultMaxSamplesPerQuery,
				Timeout:                   100 * time.Second,
				NoStepSubqueryIntervalFn:  func(int64) int64 { return time.Minute.Milliseconds() },
				EnableAtModifier:          true,
				EnableNegativeOffset:      true,
				EnableDelayedNameRemoval:  true,
				UseStartTimestamps:        true,
				EnableHistogramConversion: !tc.disabled,
				HistogramConversionFrom:   tc.convertFrom,
				Parser:                    parser.NewParser(promqltest.TestParserOpts),
			}))
		})
	}
}
