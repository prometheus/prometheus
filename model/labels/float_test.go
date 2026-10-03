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

package labels

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenMetricsFloatFormatting(t *testing.T) {
	tests := []struct {
		f    float64
		want string
	}{
		{f: 0, want: "0.0"},
		{f: math.Copysign(0, -1), want: "0.0"},
		{f: 1, want: "1.0"},
		{f: -1, want: "-1.0"},
		{f: 42, want: "42.0"},
		{f: -100, want: "-100.0"},
		{f: 0.005, want: "0.005"},
		{f: 1.5, want: "1.5"},
		{f: 1e6, want: "1e+06"},
		{f: 1e-5, want: "1e-05"},
		{f: math.NaN(), want: "NaN"},
		{f: math.Inf(+1), want: "+Inf"},
		{f: math.Inf(-1), want: "-Inf"},
	}

	for _, tc := range tests {
		require.Equal(t, tc.want, FormatOpenMetricsFloat(tc.f))
		require.Equal(t, tc.want, string(AppendOpenMetricsFloat(nil, tc.f)))
		// Verify existing "." or "e" bytes in dst prefix do not suppress ".0".
		require.Equal(t, "le.e="+tc.want, string(AppendOpenMetricsFloat([]byte("le.e="), tc.f)))
	}
}
