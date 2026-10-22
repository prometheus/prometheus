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

func TestPoolCoversMaxSize(t *testing.T) {
	calls := 0
	countingMake := func(size int) any {
		calls++
		return make([]byte, 0, size)
	}

	// 1000 * 3^k never lands on 1000000; without a bucket for the documented maximum, Get allocates
	// for it and Put drops the buffer.
	p := New(1000, 1000000, 3, countingMake)

	buf := p.Get(1000000)
	require.Equal(t, 1000000, cap(buf.([]byte)))
	p.Put(buf)
	p.Get(1000000)

	require.Equal(t, 1, calls, "the maximum size must be pooled, not reallocated on every Get")
}

func TestPoolRejectsFactorOne(t *testing.T) {
	require.Panics(t, func() { New(1, 10, 1, makeFunc) })
}
