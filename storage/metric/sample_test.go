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
)

type appendBatchScenario struct {
	in  []SampleSet
	out AppendBatch
}

func (s *appendBatchScenario) test(t *testing.T, set int) {
	b := AppendBatch{}

	for _, e := range s.in {
		b.Add(e)
	}

	if len(s.out) != len(b) {
		t.Fatalf("%d. expected length of %d, got %d", set, len(s.out), len(b))
	}

	for k, ss := range s.out {
		actual, ok := b[k]
		if !ok {
			t.Fatalf("%d. expected fingerprint of %s, missing", set, k)
		}

		if !ss.Values.Equal(actual.Values) {
			t.Fatalf("%d. expected values of %s, got %s", set, ss.Values, actual.Values)
		}
	}
}

var testTime = time.Now()

func TestAppendBatch(t *testing.T) {
	var scenarios = []appendBatchScenario{
		// Single Element
		{
			in: []SampleSet{
				{
					Metric: clientmodel.Metric{"my": "metric"},
					Values: Values{
						{
							Timestamp: testTime,
							Value:     0,
						},
						{
							Timestamp: testTime.Add(time.Second),
							Value:     1,
						},
					},
				},
			},
			out: AppendBatch{
				clientmodel.Fingerprint{8025694300047475580, "m", 8, "c"}: SampleSet{
					Metric: clientmodel.Metric{"my": "metric"},
					Values: Values{
						&SamplePair{
							Timestamp: testTime,
							Value:     0,
						},
						&SamplePair{
							Timestamp: testTime.Add(time.Second),
							Value:     1,
						},
					},
				},
			},
		},
		// Two Elements
		{
			in: []SampleSet{
				{
					Metric: clientmodel.Metric{"my": "metric"},
					Values: Values{
						{
							Timestamp: testTime,
							Value:     0,
						},
					},
				},
				{
					Metric: clientmodel.Metric{"my": "metric"},
					Values: Values{
						{
							Timestamp: testTime.Add(time.Second),
							Value:     1,
						},
					},
				},
			},
			out: AppendBatch{
				clientmodel.Fingerprint{8025694300047475580, "m", 8, "c"}: SampleSet{
					Metric: clientmodel.Metric{"my": "metric"},
					Values: Values{
						&SamplePair{
							Timestamp: testTime,
							Value:     0,
						},
						&SamplePair{
							Timestamp: testTime.Add(time.Second),
							Value:     1,
						},
					},
				},
			},
		},
		// Two Metrics
		{
			in: []SampleSet{
				{
					Metric: clientmodel.Metric{"my": "metric"},
					Values: Values{
						{
							Timestamp: testTime,
							Value:     0,
						},
					},
				},
				{
					Metric: clientmodel.Metric{"your": "metric"},
					Values: Values{
						{
							Timestamp: testTime,
							Value:     10,
						},
					},
				},
			},
			out: AppendBatch{
				clientmodel.Fingerprint{8025694300047475580, "m", 8, "c"}: SampleSet{
					Metric: clientmodel.Metric{"my": "metric"},
					Values: Values{
						&SamplePair{
							Timestamp: testTime,
							Value:     0,
						},
					},
				},
				clientmodel.Fingerprint{8978127942843029426, "y", 0, "c"}: SampleSet{
					Metric: clientmodel.Metric{"your": "metric"},
					Values: Values{
						&SamplePair{
							Timestamp: testTime,
							Value:     10,
						},
					},
				},
			},
		},
	}

	for i, scenario := range scenarios {
		scenario.test(t, i)
	}
}
