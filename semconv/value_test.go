// Copyright 2025 The Prometheus Authors
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

package semconv

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValueTransformer(t *testing.T) {
	v, err := valueTransformer{}.AddPromQL("value{} / 1000")
	require.NoError(t, err)
	require.Equal(t, float64(4), v.Transform(4000))
	require.Equal(t, float64(4), v.Transform(4000))

	v, err = valueTransformer{}.AddPromQL("whatever{foo=\"bar\"} * 1024")
	require.NoError(t, err)
	require.Equal(t, float64(81920), v.Transform(80))
	require.Equal(t, float64(81920), v.Transform(80))

	v, err = valueTransformer{}.AddPromQL("a{} + 15 - 44")
	require.NoError(t, err)
	require.Equal(t, float64(-27), v.Transform(2))
	require.Equal(t, float64(-27), v.Transform(2))

	// Chain things up.
	v, err = valueTransformer{}.AddPromQL("value{} / 1000")
	require.NoError(t, err)
	v, err = v.AddPromQL("whatever{foo=\"bar\"} * 1024")
	require.NoError(t, err)
	require.Equal(t, float64(2048), v.Transform(2000))
}
