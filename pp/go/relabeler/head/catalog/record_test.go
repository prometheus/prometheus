package catalog

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReferenceCounter_IncDecValue(t *testing.T) {
	r := NewRecord()
	require.Equal(t, int64(0), r.ReferenceCount())
	release := r.Acquire()
	require.Equal(t, int64(1), r.ReferenceCount())
	release()
	require.Equal(t, int64(0), r.ReferenceCount())
	release()
	require.Equal(t, int64(0), r.ReferenceCount())
}
