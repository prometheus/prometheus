package chunkenc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteMixedHeader(t *testing.T) {
	b := make([]byte, 2)
	writeMixedHeaderSTNotConst(b)
	writeMixedHeaderSampleNum(b, 30000)
	isSTConst, numSamples := readMixedHeader(b)
	require.False(t, isSTConst)
	require.Equal(t, uint16(30000), numSamples)

	b = make([]byte, 2)
	writeMixedHeaderSampleNum(b, 30000)
	isSTConst, numSamples = readMixedHeader(b)
	require.True(t, isSTConst)
	require.Equal(t, uint16(30000), numSamples)

	b = make([]byte, 2)
	writeMixedHeaderSampleNum(b, 300)
	writeMixedHeaderSTNotConst(b)
	isSTConst, numSamples = readMixedHeader(b)
	require.False(t, isSTConst)
	require.Equal(t, uint16(300), numSamples)
}
