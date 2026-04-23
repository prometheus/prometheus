// Copyright 2024 The Prometheus Authors
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

package kahansum

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsInf(t *testing.T) {
	for _, tc := range []struct {
		f    float64
		want bool
	}{
		{math.Inf(1), true},
		{math.Inf(-1), true},
		{math.MaxFloat64, false},
		{-math.MaxFloat64, false},
		{1.0, false},
		{-1.0, false},
		{0, false},
		{math.NaN(), false},
	} {
		got := isInf(tc.f)
		require.Equal(t, tc.want, got, "%e", tc.f)
	}
}
