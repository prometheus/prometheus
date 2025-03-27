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
