// Copyright 2017 The Prometheus Authors
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

package almost

import (
	"testing"
	"math"
)

func TestEqual(t *testing.T) {
	tests := []struct {
		a       float64
		b       float64
		epsilon float64
		expected bool
	}{
		{1.0, 1.0, 0.0001, true},
		{1.0, 1.0001, 0.0001, true},
		{1.0, 1.001, 0.0001, false},
		{0, 0, 0.0001, true},
		{1.0, 0, 0.0001, false},
		{-1.0, -1.0001, 0.0001, true},
		{math.NaN(), math.NaN(), 0.0001, true},
		{math.Inf(-1), math.Inf(-1), 0.0001, true},
	}

	for _, test := range tests {
		result := Equal(test.a, test.b, test.epsilon)
		if result != test.expected {
			t.Errorf("Equal(%v, %v, %v) = %t, expected %t", test.a, test.b, test.epsilon, result, test.expected)
		}
	}
}