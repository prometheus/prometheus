// Copyright 2025 The Prometheus Authors
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

package rewrite

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/parser"
)

func TestTransformClassicHistograms(t *testing.T) {
	testCases := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{
			input: `4 * rate(hist[5m])`,
			want:  `4 * rate(hist[5m])`,
		},
		// histogram_quantile(X, sum by (le) (rate(Y_bucket[Z]))) -> histogram_quantile(X, sum (rate(Y[Z])))
		{
			input: `histogram_quantile(0.9, sum by (le) (rate(hist_bucket[5m])))`,
			want:  `histogram_quantile(0.9, sum(rate(hist[5m])))`,
		},
		{
			input: `histogram_quantile(0.9, sum by (le, f) (rate(hist_bucket[5m])))`,
			want:  `histogram_quantile(0.9, sum by (f) (rate(hist[5m])))`,
		},
		{
			input: `histogram_quantile(0.9, sum (rate(hist_bucket[5m])) by (f, le))`,
			want:  `histogram_quantile(0.9, sum by (f) (rate(hist[5m])))`,
		},
		{
			input: `histogram_quantile(0.9, sum (irate(hist_bucket[5m])) by (f, le))`,
			want:  `histogram_quantile(0.9, sum by (f) (irate(hist[5m])))`,
		},
		{
			input:   `histogram_quantile(0.9, sum (deriv(hist_bucket[5m])) by (f, le))`,
			want:    `histogram_quantile(0.9, sum (deriv(hist_bucket[5m])) by (f, le))`,
			wantErr: true,
		},
		// X_bucket{le="Y"} -> histogram_count(X) * histogram_fraction(-Inf, Y, X)
		{
			input: `hist_bucket{le="0.99"}`,
			want:  `histogram_count(hist) * histogram_fraction(-Inf, 0.99, hist)`,
		},
		{
			input: `hist_bucket{a="b", le="0.8"}`,
			want:  `histogram_count(hist{a="b"}) * histogram_fraction(-Inf, 0.8, hist{a="b"})`,
		},
		//  X_sum / X_count -> histogram_avg(X)
		{
			input: `hist_sum / hist_count`,
			want:  `histogram_avg(hist)`,
		},
		// X_count -> histogram_count(X)
		{
			input: `hist_count`,
			want:  `histogram_count(hist)`,
		},
		{
			input: `2 * hist_count{a=~"b"}`,
			want:  `2 * histogram_count(hist{a=~"b"})`,
		},
		// X_sum -> histogram_sum(X)
		{
			input: `hist_sum`,
			want:  `histogram_sum(hist)`,
		},
		{
			input: `2 * hist_sum{a=~"b"}`,
			want:  `2 * histogram_sum(hist{a=~"b"})`,
		},
		{
			input: `hist_sum{a="a"} * hist_sum{a="b"}`,
			want:  `histogram_sum(hist{a="a"}) * histogram_sum(hist{a="b"})`,
		},
	}
	f := TransformClassicHistograms([]string{
		"hist",
		"hist2",
	})
	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.input)
			require.NoError(t, err)
			got, err := Rewrite(expr, f)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, tc.want, got.String())
		})
	}
}
