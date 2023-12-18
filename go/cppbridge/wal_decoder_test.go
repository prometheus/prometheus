package cppbridge_test

import (
	"context"
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames/framestest"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecoderSuite(t *testing.T) {
	ctx := context.Background()
	dec := cppbridge.NewWALDecoder()

	segment := [...]byte{
		// version
		1,
		// UUID
		96, 20, 232, 115,
		79, 106, 75, 235,
		170, 193, 201, 111,
		43, 239, 97, 199,
		// shard ID
		0, 0,
		// pow of two of total shards
		0,
		// segment number
		0, 0, 0, 0,
		// created_at
		0, 0, 0, 0, 0, 0, 0, 0,
		// encoded_at
		0, 0, 0, 0, 0, 0, 0, 0,
		// data
		1, 2, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 0, 0, 0, 0, 7, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 1, 2, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0, 1, 2, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 8, 0, 0, 0, 8, 0, 0, 0, 8, 0, 0, 0, 16, 0, 0, 0, 3, 0, 0, 0, 19, 0, 0, 0, 3, 0, 0, 0, 22, 0, 0, 0, 4, 0, 0, 0, 1, 2, 0, 0, 0, 0, 26, 0, 0, 0, 95, 95, 110, 97, 109, 101, 95, 95, 105, 110, 115, 116, 97, 110, 99, 101, 106, 111, 98, 108, 111, 119, 122, 101, 114, 111, 5, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 1, 1, 5, 0, 0, 0, 116, 101, 115, 116, 48, 1, 0, 0, 0, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 10, 0, 0, 0, 1, 1, 10, 0, 0, 0, 98, 108, 97, 98, 108, 97, 98, 108, 97, 48, 2, 0, 0, 0, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0, 1, 1, 7, 0, 0, 0, 116, 101, 115, 116, 101, 114, 48, 3, 0, 0, 0, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 1, 1, 6, 0, 0, 0, 98, 97, 110, 97, 110, 48, 4, 0, 0, 0, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 9, 0, 0, 0, 1, 1, 9, 0, 0, 0, 110, 111, 110, 95, 122, 101, 114, 111, 48, 1, 1, 1, 2, 0, 0, 0, 4, 2, 1, 255, 255, 255, 255, 160, 220, 88, 62, 129, 1, 0, 0, 1, 1, 37, 0, 0, 0, 0, 0, 0, 0, 4, 76, 29, 0, 0, 1, 18, 166, 48, 76, 1, 84, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 112, 197, 2, 109, 133, 151, 198, 15, 1, 114, 37, 16, 239}
	expectedProtob := [...]byte{10, 147, 1, 10, 17, 10, 8, 95, 95, 110, 97, 109, 101, 95, 95, 18, 5, 116, 101, 115, 116, 48, 10, 22, 10, 8, 105, 110, 115, 116, 97, 110, 99, 101, 18, 10, 98, 108, 97, 98, 108, 97, 98, 108, 97, 48, 10, 14, 10, 3, 106, 111, 98, 18, 7, 116, 101, 115, 116, 101, 114, 48, 10, 13, 10, 3, 108, 111, 119, 18, 6, 98, 97, 110, 97, 110, 48, 10, 17, 10, 4, 122, 101, 114, 111, 18, 9, 110, 111, 110, 95, 122, 101, 114, 111, 48, 18, 16, 9, 0, 0, 0, 0, 0, 92, 177, 64, 16, 160, 185, 227, 242, 147, 48, 18, 16, 9, 0, 0, 0, 0, 0, 95, 177, 64, 16, 128, 142, 231, 242, 147, 48, 18, 16, 9, 0, 0, 0, 0, 0, 96, 177, 64, 16, 224, 226, 234, 242, 147, 48}
	expectedString := "timeseries:<labels:<name:\"__name__\" value:\"test0\" > labels:<name:\"instance\" value:\"blablabla0\" > labels:<name:\"job\" value:\"tester0\" > labels:<name:\"low\" value:\"banan0\" > labels:<name:\"zero\" value:\"non_zero0\" > samples:<value:4444 timestamp:1654608420000 > samples:<value:4447 timestamp:1654608480000 > samples:<value:4448 timestamp:1654608540000 > > "

	protocont, err := dec.Decode(ctx, segment[:])
	require.NoError(t, err)

	assert.EqualValues(t, 0, protocont.CreatedAt())
	assert.EqualValues(t, 0, protocont.EncodedAt())
	assert.EqualValues(t, 3, protocont.Samples())
	assert.EqualValues(t, 1, protocont.Series())
	assert.EqualValues(t, 0, protocont.SegmentID())

	t.Log("compare income and outcome protobuf")

	buf, err := framestest.ReadPayload(protocont)
	require.NoError(t, err)
	assert.Equal(t, expectedProtob[:], buf)

	actualWr := &prompb.WriteRequest{}
	assert.NoError(t, protocont.UnmarshalTo(actualWr))
	assert.Equal(t, expectedString, actualWr.String())
}

func TestDecodeWithError(t *testing.T) {
	dec := cppbridge.NewWALDecoder()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := dec.Decode(ctx, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestRestoreFromStreamWithError(t *testing.T) {
	dec := cppbridge.NewWALDecoder()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := dec.RestoreFromStream(ctx, nil, 0)
	require.ErrorIs(t, err, context.Canceled)
}

func BenchmarkDecoder(b *testing.B) {
	ctx := context.Background()
	segment := [...]byte{
		// version
		1,
		// UUID
		96, 20, 232, 115,
		79, 106, 75, 235,
		170, 193, 201, 111,
		43, 239, 97, 199,
		// shard ID
		0, 0,
		// pow of two of total shards
		0,
		// segment number
		0, 0, 0, 0,
		// created_at
		0, 0, 0, 0, 0, 0, 0, 0,
		// written_at
		0, 0, 0, 0, 0, 0, 0, 0,
		// data
		1, 2, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 0, 0, 0, 0, 7, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 1, 2, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0, 1, 2, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 8, 0, 0, 0, 8, 0, 0, 0, 8, 0, 0, 0, 16, 0, 0, 0, 3, 0, 0, 0, 19, 0, 0, 0, 3, 0, 0, 0, 22, 0, 0, 0, 4, 0, 0, 0, 1, 2, 0, 0, 0, 0, 26, 0, 0, 0, 95, 95, 110, 97, 109, 101, 95, 95, 105, 110, 115, 116, 97, 110, 99, 101, 106, 111, 98, 108, 111, 119, 122, 101, 114, 111, 5, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 1, 1, 5, 0, 0, 0, 116, 101, 115, 116, 48, 1, 0, 0, 0, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 10, 0, 0, 0, 1, 1, 10, 0, 0, 0, 98, 108, 97, 98, 108, 97, 98, 108, 97, 48, 2, 0, 0, 0, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0, 1, 1, 7, 0, 0, 0, 116, 101, 115, 116, 101, 114, 48, 3, 0, 0, 0, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 1, 1, 6, 0, 0, 0, 98, 97, 110, 97, 110, 48, 4, 0, 0, 0, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 9, 0, 0, 0, 1, 1, 9, 0, 0, 0, 110, 111, 110, 95, 122, 101, 114, 111, 48, 1, 1, 1, 2, 0, 0, 0, 4, 2, 1, 255, 255, 255, 255, 160, 220, 88, 62, 129, 1, 0, 0, 1, 1, 37, 0, 0, 0, 0, 0, 0, 0, 4, 76, 29, 0, 0, 1, 18, 166, 48, 76, 1, 84, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 112, 197, 2, 109, 133, 151, 198, 15, 1, 114, 37, 16, 239}

	for i := 0; i < b.N; i++ {
		dec := cppbridge.NewWALDecoder()
		protocont, _ := dec.Decode(ctx, segment[:])
		_ = protocont
	}
}
