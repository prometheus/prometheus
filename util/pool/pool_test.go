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

package pool

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func makeFunc(size int) any {
	return make([]int, 0, size)
}

func TestPool(t *testing.T) {
	testPool := New(1, 8, 2, makeFunc)
	cases := []struct {
		size        int
		expectedCap int
	}{
		{
			size:        -1,
			expectedCap: 1,
		},
		{
			size:        3,
			expectedCap: 4,
		},
		{
			size:        10,
			expectedCap: 10,
		},
	}
	for _, c := range cases {
		ret := testPool.Get(c.size)
		require.Equal(t, c.expectedCap, cap(ret.([]int)))
		testPool.Put(ret)
	}
}

func TestPoolRejectsFactorAtOrBelowOne(t *testing.T) {
	// A factor of 1 would repeat the same size forever, and anything smaller
	// would not grow the bucket sizes at all.
	require.Panics(t, func() { New(1, 10, 1, makeFunc) })
	require.Panics(t, func() { New(1, 10, 0.5, makeFunc) })
}

func TestPoolAdvancesWithFractionalFactor(t *testing.T) {
	// A factor only slightly above 1 rounds back to the same integer, which would
	// make the loop run forever; the sequence must advance by at least one.
	p := New(1, 5, 1.01, makeFunc)
	require.Equal(t, []int{1, 2, 3, 4, 5}, p.sizes)
}
