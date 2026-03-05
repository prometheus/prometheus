// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package almost

import (
	"fmt"
	"math"
	"testing"

	"github.com/prometheus/prometheus/model/value"
)

func TestEqual(t *testing.T) {
	staleNaN := math.Float64frombits(value.StaleNaN)
	tests := []struct {
		a       float64
		b       float64
		epsilon float64
		want    bool
	}{
		{0.0, 0.0, 0.0, true},
		{0.0, 0.1, 0.0, false},
		{1.0, 1.1, 0.1, true},
		{-1.0, -1.1, 0.1, true},
		{math.MaxFloat64, math.MaxFloat64 / 10, 0.1, false},
		{1.0, math.NaN(), 0.1, false},
		{math.NaN(), math.NaN(), 0.1, true},
		{math.NaN(), staleNaN, 0.1, false},
		{staleNaN, math.NaN(), 0.1, false},
		{staleNaN, staleNaN, 0.1, true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v,%v,%v", tt.a, tt.b, tt.epsilon), func(t *testing.T) {
			if got := Equal(tt.a, tt.b, tt.epsilon); got != tt.want {
				t.Errorf("Equal(%v,%v,%v) = %v, want %v", tt.a, tt.b, tt.epsilon, got, tt.want)
			}
		})
	}
}
