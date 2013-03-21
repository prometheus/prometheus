// Copyright 2013 Prometheus Team
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

package metric

import (
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/utility/test"
	"testing"
	"time"
)

func testBuilder(t test.Tester) {
	type atTime struct {
		fingerprint string
		time        time.Time
	}

	type atInterval struct {
		fingerprint string
		from        time.Time
		through     time.Time
		interval    time.Duration
	}

	type atRange struct {
		fingerprint string
		from        time.Time
		through     time.Time
	}

	type in struct {
		atTimes     []atTime
		atIntervals []atInterval
		atRanges    []atRange
	}

	type out []struct {
		fingerprint string
		operations  ops
	}

	var scenarios = []struct {
		in  in
		out out
	}{
		// // Ensure that the fingerprint is sorted in proper order.
		{
			in: in{
				atTimes: []atTime{
					{
						fingerprint: "0000000000000001111-a-4-a",
						time:        time.Unix(100, 0),
					},
					{
						fingerprint: "0000000000000000000-a-4-a",
						time:        time.Unix(100, 0),
					},
				},
			},
			out: out{
				{
					fingerprint: "00000000000000000000-a-4-a",
				},
				{
					fingerprint: "00000000000000001111-a-4-a",
				},
			},
		},
		// // Ensure that the fingerprint-timestamp pairs are sorted in proper order.
		{
			in: in{
				atTimes: []atTime{
					{
						fingerprint: "1111-a-4-a",
						time:        time.Unix(100, 0),
					},
					{
						fingerprint: "1111-a-4-a",
						time:        time.Unix(200, 0),
					},
					{
						fingerprint: "0-a-4-a",
						time:        time.Unix(100, 0),
					},
					{
						fingerprint: "0-a-4-a",
						time:        time.Unix(0, 0),
					},
				},
			},
			out: out{
				{
					fingerprint: "00000000000000000000-a-4-a",
				},
				{
					fingerprint: "00000000000000001111-a-4-a",
				},
			},
		},
		// Ensure grouping of operations
		{
			in: in{
				atTimes: []atTime{
					{
						fingerprint: "1111-a-4-a",
						time:        time.Unix(100, 0),
					},
				},
				atRanges: []atRange{
					{
						fingerprint: "1111-a-4-a",
						from:        time.Unix(100, 0),
						through:     time.Unix(1000, 0),
					},
					{
						fingerprint: "1111-a-4-a",
						from:        time.Unix(100, 0),
						through:     time.Unix(9000, 0),
					},
				},
			},
			out: out{
				{
					fingerprint: "00000000000000001111-a-4-a",
				},
			},
		},
	}

	for i, scenario := range scenarios {
		builder := viewRequestBuilder{
			operations: map[model.Fingerprint]ops{},
		}

		for _, atTime := range scenario.in.atTimes {
			fingerprint := model.NewFingerprintFromRowKey(atTime.fingerprint)
			builder.GetMetricAtTime(fingerprint, atTime.time)
		}

		for _, atInterval := range scenario.in.atIntervals {
			fingerprint := model.NewFingerprintFromRowKey(atInterval.fingerprint)
			builder.GetMetricAtInterval(fingerprint, atInterval.from, atInterval.through, atInterval.interval)
		}

		for _, atRange := range scenario.in.atRanges {
			fingerprint := model.NewFingerprintFromRowKey(atRange.fingerprint)
			builder.GetMetricRange(fingerprint, atRange.from, atRange.through)
		}

		jobs := builder.ScanJobs()

		if len(scenario.out) != len(jobs) {
			t.Fatalf("%d. expected job length of %d, got %d\n", i, len(scenario.out), len(jobs))
		}

		for j, job := range scenario.out {
			if jobs[j].fingerprint.ToRowKey() != job.fingerprint {
				t.Fatalf("%d.%d. expected fingerprint %s, got %s\n", i, j, job.fingerprint, jobs[j].fingerprint.ToRowKey())
			}
		}
	}
}

func TestBuilder(t *testing.T) {
	testBuilder(t)
}

func BenchmarkBuilder(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testBuilder(b)
	}
}
