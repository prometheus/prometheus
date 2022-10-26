// Copyright 2022 The Prometheus Authors
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

package histogram

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetBound(t *testing.T) {
	scenarios := []struct {
		idx    int32
		schema int32
		want   float64
	}{
		{
			idx:    -1,
			schema: -1,
			want:   0.25,
		},
		{
			idx:    0,
			schema: -1,
			want:   1,
		},
		{
			idx:    1,
			schema: -1,
			want:   4,
		},
		{
			idx:    512,
			schema: -1,
			want:   math.MaxFloat64,
		},
		{
			idx:    513,
			schema: -1,
			want:   math.Inf(+1),
		},
		{
			idx:    -1,
			schema: 0,
			want:   0.5,
		},
		{
			idx:    0,
			schema: 0,
			want:   1,
		},
		{
			idx:    1,
			schema: 0,
			want:   2,
		},
		{
			idx:    1024,
			schema: 0,
			want:   math.MaxFloat64,
		},
		{
			idx:    1025,
			schema: 0,
			want:   math.Inf(+1),
		},
		{
			idx:    -1,
			schema: 2,
			want:   0.8408964152537144,
		},
		{
			idx:    0,
			schema: 2,
			want:   1,
		},
		{
			idx:    1,
			schema: 2,
			want:   1.189207115002721,
		},
		{
			idx:    4096,
			schema: 2,
			want:   math.MaxFloat64,
		},
		{
			idx:    4097,
			schema: 2,
			want:   math.Inf(+1),
		},
	}

	for _, s := range scenarios {
		got := getBound(s.idx, s.schema)
		if s.want != got {
			require.Equal(t, s.want, got, "idx %d, schema %d", s.idx, s.schema)
		}
	}
}
