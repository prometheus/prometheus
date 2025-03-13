package block_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp-pkg/handler/storage"
	"github.com/prometheus/prometheus/pp-pkg/handler/storage/block"
)

func TestStorage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "storage-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	blockStorage := block.NewStorage(tempDir)

	blockID := uuid.New()
	shardID := uint16(1)

	_, err = blockStorage.Reader(blockID, shardID)
	require.ErrorIs(t, err, storage.ErrNoBlock)

	encodedSegment := model.Segment{
		ID:   0,
		Size: 4,
		CRC:  42,
		Body: make([]byte, 4),
	}

	blockWriter := blockStorage.Writer(blockID, shardID, 0, 1)
	err = blockWriter.Append(encodedSegment)
	require.NoError(t, err)

	blockReader, err := blockStorage.Reader(blockID, shardID)
	require.NoError(t, err)

	rSeg, err := blockReader.Next()
	require.NoError(t, err)

	require.Equal(t, encodedSegment.ID, rSeg.ID)
	require.Equal(t, encodedSegment.Size, rSeg.Size)
	require.Equal(t, encodedSegment.CRC, rSeg.CRC)
	require.EqualValues(t, encodedSegment.Body, rSeg.Body)
}
