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

package semconv

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

type topKTestSeries struct {
	labels       labels.Labels
	contributors int
}

func (s *topKTestSeries) Labels() labels.Labels {
	return s.labels
}

func collectTopKTestSeries(input []*topKTestSeries, limit int) ([]*topKTestSeries, int) {
	position := -1
	got := collectAndChainTopK(
		func() bool {
			position++
			return position < len(input)
		},
		func() *topKTestSeries { return input[position] },
		limit,
		func(series ...*topKTestSeries) *topKTestSeries {
			merged := &topKTestSeries{labels: series[0].labels}
			for _, item := range series {
				merged.contributors += item.contributors
			}
			return merged
		},
	)
	return got, position
}

func TestCollectAndChainTopK(t *testing.T) {
	series := func(value string) *topKTestSeries {
		return &topKTestSeries{
			labels:       labels.FromStrings("instance", value),
			contributors: 1,
		}
	}

	got, drained := collectTopKTestSeries([]*topKTestSeries{
		series("z"), series("a"), series("c"), series("a"), series("b"), series("d"),
	}, 2)
	require.Equal(t, 6, drained)
	require.Equal(t, []labels.Labels{
		labels.FromStrings("instance", "a"),
		labels.FromStrings("instance", "b"),
	}, []labels.Labels{got[0].Labels(), got[1].Labels()})
	require.Equal(t, 2, got[0].contributors)
	require.Equal(t, 1, got[1].contributors)
}

func TestCollectAndChainTopKDoesNotPreallocateFromLimit(t *testing.T) {
	input := []*topKTestSeries{{
		labels:       labels.FromStrings("instance", "a"),
		contributors: 1,
	}}
	got, drained := collectTopKTestSeries(input, math.MaxInt)
	require.Equal(t, 1, drained)
	require.Equal(t, input, got)
	require.LessOrEqual(t, cap(got), len(input))
}
