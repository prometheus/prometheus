package remote

import (
	"fmt"
	math_bits "math/bits"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_sizeVarint(t *testing.T) {
	for i := 0; i < 63; i++ {
		x := uint64(1) << i
		t.Run(fmt.Sprint(x), func(t *testing.T) {
			want := (math_bits.Len64(x|1) + 6) / 7 // Implementation used by protoc.
			got := sizeVarint(x)
			require.Equal(t, want, got)
		})
	}
}
