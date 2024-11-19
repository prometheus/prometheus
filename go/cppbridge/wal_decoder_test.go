package cppbridge_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames/framestest"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/suite"
)

type DecoderSuite struct {
	suite.Suite
	baseCtx        context.Context
	startTimestamp int64
	step           int64
}

func TestDecoderSuite(t *testing.T) {
	suite.Run(t, new(DecoderSuite))
}

func (s *DecoderSuite) SetupTest() {
	s.baseCtx = context.Background()
	s.startTimestamp = 1654608420000
	s.step = 60000
}

func (s *DecoderSuite) TestDecodeV1() {
	var defaultEncodersVersion uint8 = 1
	dec := cppbridge.NewWALDecoder(defaultEncodersVersion)

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

	protocont, err := dec.Decode(s.baseCtx, segment[:])
	s.Require().NoError(err)

	s.EqualValues(0, protocont.CreatedAt())
	s.EqualValues(0, protocont.EncodedAt())
	s.EqualValues(3, protocont.Samples())
	s.EqualValues(1, protocont.Series())
	s.EqualValues(0, protocont.SegmentID())
	s.EqualValues(1654608420000, protocont.EarliestBlockSample())
	s.EqualValues(1654608540000, protocont.LatestBlockSample())

	s.T().Log("compare income and outcome protobuf")

	buf, err := framestest.ReadPayload(protocont)
	s.Require().NoError(err)
	s.Equal(expectedProtob[:], buf)

	actualWr := new(prompb.WriteRequest)
	s.Require().NoError(protocont.UnmarshalTo(actualWr))
	s.Equal(expectedString, actualWr.String())
}

func (s *DecoderSuite) TestDecodeV3() {
	var defaultEncodersVersion uint8 = 3
	dec := cppbridge.NewWALDecoder(defaultEncodersVersion)

	segment := [...]byte{4, 34, 77, 24, 64, 64, 192, 246, 0, 0, 0, 162, 130, 101, 7, 17, 76, 7, 176, 23, 80, 231, 8, 0, 134, 1, 2, 0, 0, 0, 0, 1, 0, 1, 0, 2, 18, 0, 28, 7, 17, 0, 4, 35, 0, 21, 5, 18, 0, 20, 5, 32, 0, 48, 0, 0, 0, 17, 0, 93, 3, 0, 0, 0, 4, 30, 0, 23, 8, 4, 0, 19, 16, 38, 0, 19, 19, 8, 0, 25, 22, 50, 0, 245, 15, 26, 0, 0, 0, 95, 95, 110, 97, 109, 101, 95, 95, 105, 110, 115, 116, 97, 110, 99, 101, 106, 111, 98, 108, 111, 119, 122, 101, 114, 111, 110, 0, 25, 1, 138, 0, 16, 1, 6, 0, 105, 116, 101, 115, 116, 48, 1, 29, 0, 17, 10, 14, 0, 0, 6, 0, 50, 98, 108, 97, 3, 0, 41, 48, 2, 34, 0, 17, 7, 14, 0, 19, 7, 63, 0, 73, 101, 114, 48, 3, 31, 0, 17, 6, 14, 0, 0, 6, 0, 121, 98, 97, 110, 97, 110, 48, 4, 30, 0, 17, 9, 14, 0, 0, 6, 0, 64, 110, 111, 110, 95, 160, 0, 49, 48, 1, 1, 199, 0, 208, 4, 2, 1, 255, 255, 255, 255, 160, 220, 88, 62, 129, 1, 37, 0, 19, 37, 51, 0, 195, 4, 76, 29, 0, 0, 1, 18, 166, 48, 76, 1, 84, 19, 0, 240, 1, 2, 0, 0, 112, 197, 2, 109, 133, 151, 198, 15, 1, 114, 37, 16, 239}
	expectedProtob := [...]byte{10, 147, 1, 10, 17, 10, 8, 95, 95, 110, 97, 109, 101, 95, 95, 18, 5, 116, 101, 115, 116, 48, 10, 22, 10, 8, 105, 110, 115, 116, 97, 110, 99, 101, 18, 10, 98, 108, 97, 98, 108, 97, 98, 108, 97, 48, 10, 14, 10, 3, 106, 111, 98, 18, 7, 116, 101, 115, 116, 101, 114, 48, 10, 13, 10, 3, 108, 111, 119, 18, 6, 98, 97, 110, 97, 110, 48, 10, 17, 10, 4, 122, 101, 114, 111, 18, 9, 110, 111, 110, 95, 122, 101, 114, 111, 48, 18, 16, 9, 0, 0, 0, 0, 0, 92, 177, 64, 16, 160, 185, 227, 242, 147, 48, 18, 16, 9, 0, 0, 0, 0, 0, 95, 177, 64, 16, 128, 142, 231, 242, 147, 48, 18, 16, 9, 0, 0, 0, 0, 0, 96, 177, 64, 16, 224, 226, 234, 242, 147, 48}
	expectedString := "timeseries:<labels:<name:\"__name__\" value:\"test0\" > labels:<name:\"instance\" value:\"blablabla0\" > labels:<name:\"job\" value:\"tester0\" > labels:<name:\"low\" value:\"banan0\" > labels:<name:\"zero\" value:\"non_zero0\" > samples:<value:4444 timestamp:1654608420000 > samples:<value:4447 timestamp:1654608480000 > samples:<value:4448 timestamp:1654608540000 > > "

	protocont, err := dec.Decode(s.baseCtx, segment[:])
	s.Require().NoError(err)

	s.EqualValues(1706872282058024322, protocont.CreatedAt())
	s.EqualValues(1706872282058057552, protocont.EncodedAt())
	s.EqualValues(3, protocont.Samples())
	s.EqualValues(1, protocont.Series())
	s.EqualValues(0, protocont.SegmentID())
	s.EqualValues(1654608420000, protocont.EarliestBlockSample())
	s.EqualValues(1654608540000, protocont.LatestBlockSample())

	s.T().Log("compare income and outcome protobuf")
	buf, err := framestest.ReadPayload(protocont)
	s.Require().NoError(err)
	s.Equal(expectedProtob[:], buf)

	actualWr := new(prompb.WriteRequest)
	s.Require().NoError(protocont.UnmarshalTo(actualWr))
	s.Equal(expectedString, actualWr.String())
}

func (s *DecoderSuite) TestDecodeWithError() {
	dec := cppbridge.NewWALDecoder(cppbridge.EncodersVersion())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := dec.Decode(ctx, nil)
	s.Require().ErrorIs(err, context.Canceled)
}

func (s *DecoderSuite) TestRestoreFromStreamWithError() {
	dec := cppbridge.NewWALDecoder(cppbridge.EncodersVersion())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := dec.RestoreFromStream(ctx, nil, 0)
	s.Require().ErrorIs(err, context.Canceled)
}

func BenchmarkDecoderV1(b *testing.B) {
	var defaultEncodersVersion uint8 = 1
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
		dec := cppbridge.NewWALDecoder(defaultEncodersVersion)
		protocont, _ := dec.Decode(ctx, segment[:])
		_ = protocont
	}
}

func BenchmarkDecoderV3(b *testing.B) {
	var defaultEncodersVersion uint8 = 1
	ctx := context.Background()
	segment := [...]byte{4, 34, 77, 24, 64, 64, 192, 246, 0, 0, 0, 162, 130, 101, 7, 17, 76, 7, 176, 23, 80, 231, 8, 0, 134, 1, 2, 0, 0, 0, 0, 1, 0, 1, 0, 2, 18, 0, 28, 7, 17, 0, 4, 35, 0, 21, 5, 18, 0, 20, 5, 32, 0, 48, 0, 0, 0, 17, 0, 93, 3, 0, 0, 0, 4, 30, 0, 23, 8, 4, 0, 19, 16, 38, 0, 19, 19, 8, 0, 25, 22, 50, 0, 245, 15, 26, 0, 0, 0, 95, 95, 110, 97, 109, 101, 95, 95, 105, 110, 115, 116, 97, 110, 99, 101, 106, 111, 98, 108, 111, 119, 122, 101, 114, 111, 110, 0, 25, 1, 138, 0, 16, 1, 6, 0, 105, 116, 101, 115, 116, 48, 1, 29, 0, 17, 10, 14, 0, 0, 6, 0, 50, 98, 108, 97, 3, 0, 41, 48, 2, 34, 0, 17, 7, 14, 0, 19, 7, 63, 0, 73, 101, 114, 48, 3, 31, 0, 17, 6, 14, 0, 0, 6, 0, 121, 98, 97, 110, 97, 110, 48, 4, 30, 0, 17, 9, 14, 0, 0, 6, 0, 64, 110, 111, 110, 95, 160, 0, 49, 48, 1, 1, 199, 0, 208, 4, 2, 1, 255, 255, 255, 255, 160, 220, 88, 62, 129, 1, 37, 0, 19, 37, 51, 0, 195, 4, 76, 29, 0, 0, 1, 18, 166, 48, 76, 1, 84, 19, 0, 240, 1, 2, 0, 0, 112, 197, 2, 109, 133, 151, 198, 15, 1, 114, 37, 16, 239}

	for i := 0; i < b.N; i++ {
		dec := cppbridge.NewWALDecoder(defaultEncodersVersion)
		protocont, _ := dec.Decode(ctx, segment[:])
		_ = protocont
	}
}

func (s *DecoderSuite) TestWALOutputDecoderDump() {
	statelessRelabeler, err := cppbridge.NewStatelessRelabeler([]*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        ".*",
			Action:       cppbridge.Keep,
		},
	})
	s.Require().NoError(err)
	externalLabels := []cppbridge.Label{{"name0", "value0"}, {"name1", "value1"}}
	outputLss := cppbridge.NewLssStorage()

	dec := cppbridge.NewWALOutputDecoder(externalLabels, statelessRelabeler, outputLss, 0, cppbridge.EncodersVersion())

	file := &bytes.Buffer{}
	n, err := dec.WriteTo(file)
	s.Require().NoError(err)
	s.Equal(int64(15), n)
}

func (s *DecoderSuite) TestWALOutputDecoderEmptyLoad() {
	statelessRelabeler, err := cppbridge.NewStatelessRelabeler([]*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			Regex:        ".*",
			Action:       cppbridge.Keep,
		},
	})
	s.Require().NoError(err)
	externalLabels := []cppbridge.Label{{"name0", "value0"}, {"name1", "value1"}}
	outputLss := cppbridge.NewLssStorage()

	dec := cppbridge.NewWALOutputDecoder(externalLabels, statelessRelabeler, outputLss, 0, cppbridge.EncodersVersion())

	file := &bytes.Buffer{}
	n, err := dec.WriteTo(file)
	s.Require().NoError(err)
	s.Equal(int64(15), n)

	err = dec.LoadFrom(file.Bytes())
	s.Require().NoError(err)
}
