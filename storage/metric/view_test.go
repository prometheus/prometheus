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
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/utility/test"
)

func testBuilder(t test.Tester) {
	type atTime struct {
		fingerprint string
		time        clientmodel.Timestamp
	}

	type atInterval struct {
		fingerprint string
		from        clientmodel.Timestamp
		through     clientmodel.Timestamp
		interval    time.Duration
	}

	type atRange struct {
		fingerprint string
		from        clientmodel.Timestamp
		through     clientmodel.Timestamp
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
		// Ensure that the fingerprint is sorted in proper order.
		{
			in: in{
				atTimes: []atTime{
					{
						fingerprint: "0000000000000001111-a-4-a",
						time:        clientmodel.TimestampFromUnix(100),
					},
					{
						fingerprint: "0000000000000000000-a-4-a",
						time:        clientmodel.TimestampFromUnix(100),
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
						time:        clientmodel.TimestampFromUnix(100),
					},
					{
						fingerprint: "1111-a-4-a",
						time:        clientmodel.TimestampFromUnix(200),
					},
					{
						fingerprint: "0-a-4-a",
						time:        clientmodel.TimestampFromUnix(100),
					},
					{
						fingerprint: "0-a-4-a",
						time:        clientmodel.TimestampFromUnix(0),
					},
				},
			},
			out: out{
				{
					fingerprint: "00000000000000000000-a-4-a",
				},
				{
					fingerprint: "00000000000000000000-a-4-a",
				},
				{
					fingerprint: "00000000000000001111-a-4-a",
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
						time:        clientmodel.TimestampFromUnix(100),
					},
				},
				atRanges: []atRange{
					{
						fingerprint: "1111-a-4-a",
						from:        clientmodel.TimestampFromUnix(100),
						through:     clientmodel.TimestampFromUnix(1000),
					},
					{
						fingerprint: "1111-a-4-a",
						from:        clientmodel.TimestampFromUnix(100),
						through:     clientmodel.TimestampFromUnix(9000),
					},
				},
			},
			out: out{
				{
					fingerprint: "00000000000000001111-a-4-a",
				},
				{
					fingerprint: "00000000000000001111-a-4-a",
				},
				{
					fingerprint: "00000000000000001111-a-4-a",
				},
			},
		},
	}

	for i, scenario := range scenarios {
		builder := NewViewRequestBuilder()

		for _, atTime := range scenario.in.atTimes {
			fingerprint := &clientmodel.Fingerprint{}
			fingerprint.LoadFromString(atTime.fingerprint)
			builder.GetMetricAtTime(fingerprint, atTime.time)
		}

		for _, atInterval := range scenario.in.atIntervals {
			fingerprint := &clientmodel.Fingerprint{}
			fingerprint.LoadFromString(atInterval.fingerprint)
			builder.GetMetricAtInterval(fingerprint, atInterval.from, atInterval.through, atInterval.interval)
		}

		for _, atRange := range scenario.in.atRanges {
			fingerprint := &clientmodel.Fingerprint{}
			fingerprint.LoadFromString(atRange.fingerprint)
			builder.GetMetricRange(fingerprint, atRange.from, atRange.through)
		}

		for j, job := range scenario.out {
			got := builder.PopOp()
			if got.Fingerprint().String() != job.fingerprint {
				t.Errorf("%d.%d. expected fingerprint %s, got %s", i, j, job.fingerprint, got.Fingerprint())
			}
		}
		if builder.HasOp() {
			t.Error("Expected builder to have no scan jobs left.")
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
