package labels

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStableHash tests that StableHash is stable.
// The hashes this test asserts should not be changed.
func TestStableHash(t *testing.T) {
	for expectedHash, lbls := range map[uint64]Labels{
		0xef46db3751d8e999: EmptyLabels(),
		0x347c8ee7a9e29708: FromStrings("hello", "world"),
		0xcbab40540f26097d: FromStrings(MetricName, "metric", "label", "value"),
	} {
		require.Equal(t, expectedHash, StableHash(lbls))
	}
}
