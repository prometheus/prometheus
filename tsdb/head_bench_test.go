// Copyright 2018 The Prometheus Authors
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
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

func BenchmarkHeadStripeSeriesCreate(b *testing.B) {
	// Put a series, select it. GC it and then access it.
	h, err := NewHead(nil, nil, nil, 1000, DefaultStripeSize)
	testutil.Ok(b, err)
	defer h.Close()

	for i := 0; i < b.N; i++ {
		h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(i)))
	}
}

func BenchmarkHeadStripeSeriesCreateParallel(b *testing.B) {
	// Put a series, select it. GC it and then access it.
	h, err := NewHead(nil, nil, nil, 1000, DefaultStripeSize)
	testutil.Ok(b, err)
	defer h.Close()

	var count int64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&count, 1)
			h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(int(i))))
		}
	})
}

func BenchmarkHeadSeries(b *testing.B) {
	h, err := NewHead(nil, nil, nil, 1000)
	testutil.Ok(b, err)
	defer h.Close()
	app := h.Appender()
	numSeries := 1000000
	for i := 0; i < numSeries; i++ {
		app.Add(labels.FromStrings("foo", "bar", "i", strconv.Itoa(i)), int64(i), 0)
	}
	testutil.Ok(b, app.Commit())

	matcher := labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")

	for s := 1; s <= numSeries; s *= 10 {
		b.Run(fmt.Sprintf("%dof%d", s, numSeries), func(b *testing.B) {
			q, err := NewBlockQuerier(h, 0, int64(s-1))
			testutil.Ok(b, err)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ss, err := q.Select(matcher)
				testutil.Ok(b, err)
				for ss.Next() {
				}
				testutil.Ok(b, ss.Err())
			}
			q.Close()
		})

	}
}
