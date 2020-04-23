// Copyright 2020 The Prometheus Authors
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

	"github.com/prometheus/prometheus/util/testutil"
)

func makeFunc(size int) interface{} {
	return size
}

func TestGet(t *testing.T) {
	testPool := New(1, 8, 2, makeFunc)
	cases := []struct {
		size     int
		expected int
	}{
		{
			size:     -1,
			expected: 1,
		},
		{
			size:     3,
			expected: 4,
		},
		{
			size:     10,
			expected: 10,
		},
	}
	for _, c := range cases {
		ret := testPool.Get(c.size)
		testutil.Equals(t, c.expected, ret)
	}
}

func TestPut(t *testing.T) {
	testPool := New(1, 8, 2, makeFunc)
	cases := []struct {
		slice    []int
		size     int
		expected interface{}
	}{
		{
			slice:    make([]int, 1),
			size:     1,
			expected: []int{},
		},
		{
			slice:    make([]int, 8),
			size:     8,
			expected: []int{},
		},
		{
			slice:    nil,
			size:     2,
			expected: 2,
		},
	}
	for _, c := range cases {
		testPool.Put(c.slice)
		testutil.Equals(t, c.expected, testPool.Get(c.size))
	}
}
