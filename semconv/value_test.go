package semconv

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValueTransformer(t *testing.T) {
	v, err := newValueTransformer("value{} / 1000")
	require.NoError(t, err)
	require.Equal(t, float64(4), v.Transform(4000))
	require.Equal(t, float64(4), v.Transform(4000))

	v, err = newValueTransformer("whatever{foo=\"bar\"} * 1024")
	require.NoError(t, err)
	require.Equal(t, float64(81920), v.Transform(80))
	require.Equal(t, float64(81920), v.Transform(80))

	v, err = newValueTransformer("a{} + 15 - 44")
	require.NoError(t, err)
	require.Equal(t, float64(-27), v.Transform(2))
	require.Equal(t, float64(-27), v.Transform(2))
}
