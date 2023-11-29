// Copyright 2023 The Prometheus Authors
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
package prompb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptimizedMarshal(t *testing.T) {
	var got []byte

	tests := []struct {
		name string
		m    *MinimizedWriteRequestStr
	}{
		// {
		// 	name: "empty",
		// 	m:    &MinimizedWriteRequestStr{},
		// },
		{
			name: "simple",
			m: &MinimizedWriteRequestStr{
				Timeseries: []MinimizedTimeSeriesStr{
					{
						LabelSymbols: []uint32{
							0, 1,
							2, 3,
							4, 5,
							6, 7,
							8, 9,
							10, 11,
							12, 13,
							14, 15,
						},

						Samples:    []Sample{{Value: 1, Timestamp: 0}},
						Exemplars:  []Exemplar{{Labels: []Label{{Name: "f", Value: "g"}}, Value: 1, Timestamp: 0}},
						Histograms: nil,
					},
					{
						LabelSymbols: []uint32{
							0, 1,
							2, 3,
							4, 5,
							6, 7,
							8, 9,
							10, 11,
							12, 13,
							14, 15,
						},
						Samples:    []Sample{{Value: 2, Timestamp: 1}},
						Exemplars:  []Exemplar{{Labels: []Label{{Name: "h", Value: "i"}}, Value: 2, Timestamp: 1}},
						Histograms: nil,
					},
				},
				Symbols: []string{
					"a", "b",
					"c", "d",
					"e", "f",
					"g", "h",
					"i", "j",
					"k", "l",
					"m", "n",
					"o", "p",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got = got[:0]
			// should be the same as the standard marshal
			expected, err := tt.m.Marshal()
			require.NoError(t, err)
			got, err = tt.m.OptimizedMarshal(got)
			require.NoError(t, err)
			require.Equal(t, expected, got)

			// round trip
			m := &MinimizedWriteRequestStr{}
			require.NoError(t, m.Unmarshal(got))
			require.Equal(t, tt.m, m)
		})
	}
}
