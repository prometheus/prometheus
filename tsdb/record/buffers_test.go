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

package record

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
)

func TestBuffersPool_PtrClear(t *testing.T) {
	pool := NewBuffersPool()

	h := pool.GetHistograms(1)
	h = append(h, RefHistogramSample{
		H: &histogram.Histogram{Schema: 1244124},
	})
	pool.PutHistograms(h)

	h2 := pool.GetHistograms(1)
	require.Empty(t, h2)
	require.Equal(t, 1, cap(h2))
	h2 = h2[:1] // extend to capacity to check previously stored item
	require.Nil(t, h2[0].H)

	fh := pool.GetFloatHistograms(1)
	fh = append(fh, RefFloatHistogramSample{
		FH: &histogram.FloatHistogram{Schema: 1244521},
	})
	pool.PutFloatHistograms(fh)

	fh2 := pool.GetFloatHistograms(1)
	require.Empty(t, fh2)
	require.Equal(t, 1, cap(fh2))
	fh2 = fh2[:1] // extend to capacity
	require.Nil(t, fh2[0].FH)
}
