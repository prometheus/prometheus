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

package kahansum

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsInf(t *testing.T) {
	for _, f := range []float64{
		math.Inf(1),
		math.Inf(-1),
		math.MaxFloat64,
		-math.MaxFloat64,
		1.0,
		-1.0,
		0,
		math.NaN(),
	} {
		require.Equal(t, math.IsInf(f, 0), isInf(f), "%e", f)
	}
}
