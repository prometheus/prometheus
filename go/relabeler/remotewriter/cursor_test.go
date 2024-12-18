package remotewriter

import (
	"bytes"
	"github.com/edsrzf/mmap-go"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"unsafe"
)

func TestNewCursor(t *testing.T) {
	tempFile, err := os.CreateTemp("", "mmaptest")
	require.NoError(t, err)
	tempFileName := tempFile.Name()
	defer func() {
		_ = os.RemoveAll(tempFileName)
	}()
	require.NoError(t, tempFile.Close())

	var numberOfShards uint16 = 2
	func(t *testing.T) {
		var crw *CursorReadWriter
		crw, err = NewCursorReadWriter(tempFileName, numberOfShards)
		require.NoError(t, err)
		defer func() { _ = crw.Close() }()

		require.Equal(t, uint32(0), crw.GetTargetSegmentID())
		require.Equal(t, uint32(0), crw.GetConfigCRC32())
		require.False(t, crw.ShardIsCorrupted(0))
		require.False(t, crw.ShardIsCorrupted(1))
		require.NoError(t, crw.SetTargetSegmentID(42))
		require.NoError(t, crw.SetShardCorrupted(1))
	}(t)

	func(t *testing.T) {
		var crw *CursorReadWriter
		crw, err = NewCursorReadWriter(tempFileName, numberOfShards)
		require.NoError(t, err)
		defer func() { _ = crw.Close() }()

		require.Equal(t, uint32(42), crw.GetTargetSegmentID())
		require.Equal(t, uint32(0), crw.GetConfigCRC32())
		require.False(t, crw.ShardIsCorrupted(0))
		require.True(t, crw.ShardIsCorrupted(1))
	}(t)
}

type CursorV2 struct {
	TargetSegmentID uint32
	ConfigCRC32     uint32
}

func TestMMapFile_Bytes(t *testing.T) {
	tempFile, err := os.CreateTemp("", "mmaptest")
	require.NoError(t, err)
	tempFileName := tempFile.Name()
	defer func() {
		_ = os.RemoveAll(tempFileName)
	}()
	require.NoError(t, tempFile.Close())

	targetSize := unsafe.Sizeof(Cursor{})

	func(t *testing.T) {
		mfile, err := NewMMapFile(tempFileName, os.O_CREATE|os.O_RDWR, 0600, int64(targetSize))
		require.NoError(t, err)
		defer func() {
			_ = mfile.Close()
		}()
		t.Log(mfile.mmap)
		c := (*CursorV2)(unsafe.Pointer(&mfile.mmap[0]))
		c.TargetSegmentID = 1
		require.NoError(t, mfile.Sync())
	}(t)

	func(t *testing.T) {
		mfile, err := NewMMapFile(tempFileName, os.O_CREATE|os.O_RDWR, 0600, int64(targetSize))
		require.NoError(t, err)
		defer func() {
			_ = mfile.Close()
		}()
		c := (*CursorV2)(unsafe.Pointer(&mfile.mmap[0]))

		require.Equal(t, uint32(1), c.TargetSegmentID)
	}(t)

}

func TestMMap(t *testing.T) {
	tempFile, err := os.CreateTemp("", "mmaptest")
	require.NoError(t, err)
	tempFileName := tempFile.Name()
	defer func() {
		_ = os.RemoveAll(tempFileName)
	}()
	require.NoError(t, tempFile.Close())

	testString := "lol + kek = chebureck"

	func(t *testing.T) {
		var file *os.File
		file, err = os.OpenFile(tempFileName, os.O_CREATE|os.O_RDWR, 0755)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, file.Close())
		}()

		require.NoError(t, fileutil.Preallocate(file, int64(len(testString)), true))
		require.NoError(t, file.Sync())

		var mappedFile mmap.MMap
		mappedFile, err = mmap.MapRegion(file, len(testString), mmap.RDWR, 0, 0)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, mappedFile.Unmap())
		}()
		copy(mappedFile, testString)
		require.NoError(t, mappedFile.Flush())
	}(t)

	func(t *testing.T) {
		testFile, err := os.OpenFile(tempFileName, os.O_CREATE|os.O_RDWR, 0755)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, testFile.Close())
		}()

		mappedFile, err := mmap.Map(testFile, mmap.RDWR, 0)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, mappedFile.Unmap())
		}()
		require.True(t, bytes.Equal(mappedFile, []byte(testString)))
	}(t)

	func(t *testing.T) {
		testFile, err := os.OpenFile(tempFileName, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, testFile.Close())
		}()

		_, err = testFile.WriteString(testString)
		require.NoError(t, err)

		mappedFile, err := mmap.Map(testFile, mmap.RDWR, 0)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, mappedFile.Unmap())
		}()

		require.True(t, bytes.Equal(mappedFile, []byte("lol + kek = chebureck")))
		copy(mappedFile, "lol + lol")
		require.NoError(t, mappedFile.Flush())
		require.True(t, bytes.Equal(mappedFile, []byte("lol + lol = chebureck")))
	}(t)
}
