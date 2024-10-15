package head_test

import (
	"bytes"
	"github.com/prometheus/prometheus/pp/go/relabeler/head"
	"github.com/stretchr/testify/require"
	"hash/crc32"
	"testing"
)

func TestReadWriteHeader(t *testing.T) {
	readWriter := bytes.NewBuffer(nil)
	ffv := uint8(5)
	ev := uint8(42)
	bytesWritten, err := head.WriteHeader(readWriter, ffv, ev)
	require.NoError(t, err)

	rffv, rev, bytesRead, err := head.ReadHeader(readWriter)
	require.NoError(t, err)
	require.Equal(t, ffv, rffv)
	require.Equal(t, ev, rev)
	require.Equal(t, bytesWritten, bytesRead)
}

func TestReadWriteSegment(t *testing.T) {
	readWriter := bytes.NewBuffer(nil)
	data := []byte("lol_kek_chebureck")

	bytesWritten, err := head.WriteSegment(readWriter, data)
	require.NoError(t, err)

	size, crc32Hash, rData, bytesRead, err := head.ReadSegment(readWriter)
	require.NoError(t, err)
	require.Equal(t, bytesWritten, bytesRead)
	require.Equal(t, size, uint64(len(data)))
	require.Equal(t, crc32Hash, crc32.ChecksumIEEE(data))
	require.Equal(t, data, rData)
}
