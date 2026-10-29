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
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/storage"
)

// benchValues returns a slice of N pseudo-metric names used as Filter inputs.
// The shape mirrors typical metric_name cardinality: a small number of common
// prefixes followed by a descriptor.
func benchValues(n int) []string {
	prefixes := []string{
		"http_requests_total",
		"go_gc_duration_seconds",
		"go_memstats_alloc_bytes",
		"prometheus_tsdb_head_series",
		"node_cpu_seconds_total",
		"node_memory_MemAvailable_bytes",
		"container_cpu_usage_seconds_total",
		"kube_pod_status_phase",
	}
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s_%07d", prefixes[i%len(prefixes)], i)
	}
	return out
}

func BenchmarkSubstringFilter(b *testing.B) {
	values := benchValues(10000)
	filter := NewSubstringFilter("requests")
	b.ReportAllocs()
	for b.Loop() {
		for _, v := range values {
			_, _ = filter.Accept(v)
		}
	}
}

func BenchmarkFuzzyFilter(b *testing.B) {
	values := benchValues(10000)
	filter := NewFuzzyFilter("requests", 0.6)
	b.ReportAllocs()
	for b.Loop() {
		for _, v := range values {
			_, _ = filter.Accept(v)
		}
	}
}

func BenchmarkSubsequenceFilter(b *testing.B) {
	values := benchValues(10000)
	filter := NewSubsequenceFilter("rqts", 0.0)
	b.ReportAllocs()
	for b.Loop() {
		for _, v := range values {
			_, _ = filter.Accept(v)
		}
	}
}

// BenchmarkFilterChainAcrossBlocks models the multi-block case that motivates
// memoization: each value is scored once per simulated block. The "blocks"
// dimension is the multiplier that the cache eliminates for shared values.
//
// Each filter type has paired sub-benchmarks — one with the raw filter and one
// wrapped in memoizingFilter — so benchstat can quantify the memoization win
// for the same scoring algorithm rather than comparing across algorithms.
func BenchmarkFilterChainAcrossBlocks(b *testing.B) {
	const blocks = 24
	values := benchValues(2000)

	type filterCase struct {
		name string
		raw  func() storage.Filter
	}
	cases := []filterCase{
		{"substring", func() storage.Filter { return NewSubstringFilter("requests") }},
		{"jarowinkler", func() storage.Filter { return NewFuzzyFilter("requests", 0.6) }},
		{"subsequence", func() storage.Filter { return NewSubsequenceFilter("rqts", 0.0) }},
	}

	run := func(b *testing.B, build func() storage.Filter) {
		b.ReportAllocs()
		for b.Loop() {
			filter := build()
			for range blocks {
				for _, v := range values {
					_, _ = filter.Accept(v)
				}
			}
		}
	}

	for _, c := range cases {
		b.Run(c.name+"/raw", func(b *testing.B) {
			run(b, c.raw)
		})
		b.Run(c.name+"/memo", func(b *testing.B) {
			run(b, func() storage.Filter { return newMemoizingFilter(c.raw()) })
		})
	}
}
