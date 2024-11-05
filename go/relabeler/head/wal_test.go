package head_test

import (
	"bytes"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/head"
	"github.com/stretchr/testify/require"
	"hash/crc32"
	"io"
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

type TestSegment struct {
	data []byte
	cppbridge.WALEncoderStats
}

func (s *TestSegment) Size() int64 {
	return int64(len(s.data))
}

func (s *TestSegment) CRC32() uint32 {
	return crc32.ChecksumIEEE(s.data)
}

func (s *TestSegment) WriteTo(w io.Writer) (n int64, err error) {
	written, err := w.Write(s.data)
	return int64(written), err
}

func NewTestSegment(data []byte) *TestSegment {
	return &TestSegment{data: data}
}

func TestReadWriteSegment(t *testing.T) {
	readWriter := bytes.NewBuffer(nil)
	data := []byte("lol_kek_chebureck")

	bytesWritten, err := head.WriteSegment(readWriter, NewTestSegment(data))
	require.NoError(t, err)

	decodedSegment, bytesRead, err := head.ReadSegment(readWriter)
	require.NoError(t, err)
	require.Equal(t, bytesWritten, bytesRead)
	require.Equal(t, len(decodedSegment.Data()), len(data))
	require.Equal(t, crc32.ChecksumIEEE(decodedSegment.Data()), crc32.ChecksumIEEE(data))
	require.Equal(t, data, decodedSegment.Data())
}
