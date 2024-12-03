package remotewriter

import (
	"github.com/stretchr/testify/require"
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

	crw, err := NewCursorReadWriter(filePath)
	require.NoError(t, err)

	cursor, err := crw.Read()
	require.NoError(t, err)
	require.Nil(t, cursor.SegmentID)
	require.Nil(t, cursor.ConfigCRC32)

	var configCRC32 uint32 = 5

	require.NoError(t, crw.Write(nil, &configCRC32))

	cursor, err = crw.Read()
	require.NoError(t, err)
	require.Nil(t, cursor.SegmentID)
	require.NotNil(t, cursor.ConfigCRC32)
	require.Equal(t, configCRC32, *cursor.ConfigCRC32)

	var segmentID uint32 = 32

	require.NoError(t, crw.Write(&segmentID, &configCRC32))

	cursor, err = crw.Read()
	require.NoError(t, err)
	require.NotNil(t, cursor.SegmentID)
	require.Equal(t, segmentID, *cursor.SegmentID)
	require.NotNil(t, cursor.ConfigCRC32)
	require.Equal(t, configCRC32, *cursor.ConfigCRC32)

	require.NoError(t, crw.Close())

	crw, err = NewCursorReadWriter(filePath)
	require.NoError(t, err)

	cursor, err = crw.Read()
	require.NoError(t, err)
	require.NotNil(t, cursor.SegmentID)
	require.Equal(t, segmentID, *cursor.SegmentID)
	require.NotNil(t, cursor.ConfigCRC32)
	require.Equal(t, configCRC32, *cursor.ConfigCRC32)
}
