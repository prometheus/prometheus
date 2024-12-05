package catalog

import (
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
)

func TestReferenceCounter_IncDecValue(t *testing.T) {
	global := atomic.Int64{}
	refCounter := NewReferenceCounter(&global)

	refCounter.Inc()
	require.Equal(t, int64(1), global.Load())
	require.Equal(t, global.Load(), refCounter.Value())
	refCounter.Dec()
	require.Equal(t, int64(0), global.Load())
	require.Equal(t, global.Load(), refCounter.Value())

	refCounter.Dec()
	require.Equal(t, int64(0), global.Load())
	require.Equal(t, global.Load(), refCounter.Value())

	refCounter.Inc()
	require.Equal(t, int64(0), global.Load())
	require.Equal(t, global.Load(), refCounter.Value())
	refCounter.Inc()
	require.Equal(t, int64(1), global.Load())
	require.Equal(t, global.Load(), refCounter.Value())
	refCounter.Inc()
	require.Equal(t, int64(2), global.Load())
	require.Equal(t, global.Load(), refCounter.Value())
}
