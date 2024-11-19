package remotewriter

import (
	"github.com/stretchr/testify/require"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestNewCursor(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cursor_test")
	require.NoError(t, err)
	filePath := filepath.Join(tmpDir, "cursor_test")
	defer func() {
		_ = os.RemoveAll(filePath)
	}()

	c, err := NewCursor(filePath)
	require.NoError(t, err)

	data, err := c.Read()
	require.NoError(t, err)
	require.Equal(t, uint32(math.MaxUint32), data.SegmentID)
	require.Equal(t, uint32(math.MaxUint32), data.ConfigCRC32)

	var segmentID uint32 = 1
	var configCRC32 uint32 = 5

	require.NoError(t, c.Write(CursorData{
		SegmentID:   segmentID,
		ConfigCRC32: configCRC32,
	}))

	data, err = c.Read()
	require.NoError(t, err)
	require.Equal(t, segmentID, data.SegmentID)
	require.Equal(t, configCRC32, data.ConfigCRC32)
}
