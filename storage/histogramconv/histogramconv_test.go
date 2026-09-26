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

package histogramconv_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/storage/histogramconv"
)

func TestParseRepresentations(t *testing.T) {
	for _, tc := range []struct {
		name        string
		lists       []string
		expected    []histogramconv.Representation
		expectedErr string
	}{
		{
			name: "nothing",
		},
		{
			name:  "empty elements",
			lists: []string{"", ",", " , "},
		},
		{
			name:     "all",
			lists:    []string{"classic,nhcb,nhe"},
			expected: histogramconv.Representations(),
		},
		{
			name:     "several lists with duplicates and spaces",
			lists:    []string{"nhe, nhcb", "nhe", "classic,nhcb"},
			expected: []histogramconv.Representation{histogramconv.NHE, histogramconv.NHCB, histogramconv.Classic},
		},
		{
			name:        "unknown representation",
			lists:       []string{"nhcb", "exponential"},
			expectedErr: `unknown histogram representation "exponential", valid ones are classic, nhcb and nhe`,
		},
		{
			name:        "representations are case sensitive",
			lists:       []string{"NHCB"},
			expectedErr: `unknown histogram representation "NHCB"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := histogramconv.ParseRepresentations(tc.lists...)
			if tc.expectedErr != "" {
				require.ErrorContains(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}
