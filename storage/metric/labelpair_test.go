// Copyright 2013 The Prometheus Authors
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
	"sort"
	"testing"
)

func testLabelPairs(t testing.TB) {
	var scenarios = []struct {
		in  LabelPairs
		out LabelPairs
	}{
		{
			in: LabelPairs{
				{
					Name:  "AAA",
					Value: "aaa",
				},
			},
			out: LabelPairs{
				{
					Name:  "AAA",
					Value: "aaa",
				},
			},
		},
		{
			in: LabelPairs{
				{
					Name:  "aaa",
					Value: "aaa",
				},
				{
					Name:  "ZZZ",
					Value: "aaa",
				},
			},
			out: LabelPairs{
				{
					Name:  "ZZZ",
					Value: "aaa",
				},
				{
					Name:  "aaa",
					Value: "aaa",
				},
			},
		},
	}

	for i, scenario := range scenarios {
		sort.Sort(scenario.in)

		for j, expected := range scenario.out {
			if !expected.Equal(scenario.in[j]) {
				t.Errorf("%d.%d expected %s, got %s", i, j, expected, scenario.in[j])
			}
		}
	}
}

func TestLabelPairs(t *testing.T) {
	testLabelPairs(t)
}

func BenchmarkLabelPairs(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLabelPairs(b)
	}
}
