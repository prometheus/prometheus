package cppbridge_test

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/stretchr/testify/suite"
)

type ChunkRecoderSuite struct {
	suite.Suite
	dataStorage *cppbridge.HeadDataStorage
	encoder     *cppbridge.HeadEncoder
}

func TestChunkRecoderSuite(t *testing.T) {
	suite.Run(t, new(ChunkRecoderSuite))
}

func (s *ChunkRecoderSuite) SetupTest() {
	s.dataStorage = cppbridge.NewHeadDataStorage()
	s.encoder = cppbridge.NewHeadEncoderWithDataStorage(s.dataStorage)
}

func (s *ChunkRecoderSuite) TestEmptyStorage() {
	// Arrange
	recoder := cppbridge.NewChunkRecoder(s.dataStorage)

	// Act
	chunk := recoder.RecodeNextChunk()

	// Assert
	s.Equal(cppbridge.RecodedChunk{
		MinT:        0,
		MaxT:        0,
		SeriesId:    cppbridge.InvalidSeriesId,
		HasMoreData: false,
		ChunkData:   nil,
	}, chunk)
}

func (s *ChunkRecoderSuite) TestStorageWithOneChunk() {
	// Arrange
	s.encoder.Encode(0, 1, 1.0)
	s.encoder.Encode(0, 2, 1.0)
	recoder := cppbridge.NewChunkRecoder(s.dataStorage)

	// Act
	chunk1 := recoder.RecodeNextChunk()
	chunk1.ChunkData = append([]byte(nil), chunk1.ChunkData...)
	chunk2 := recoder.RecodeNextChunk()

	// Assert
	s.Equal(cppbridge.RecodedChunk{
		MinT:        1,
		MaxT:        2,
		SeriesId:    0,
		HasMoreData: false,
		ChunkData:   []byte{0x00, 0x02, 0x02, 0x3f, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00},
	}, chunk1)
	s.Equal(cppbridge.RecodedChunk{
		MinT:        0,
		MaxT:        0,
		SeriesId:    cppbridge.InvalidSeriesId,
		HasMoreData: false,
		ChunkData:   []byte{},
	}, chunk2)
}

func (s *ChunkRecoderSuite) TestStorageWithTwoChunks() {
	// Arrange
	s.encoder.Encode(0, 1, 1.0)
	s.encoder.Encode(0, 2, 1.0)
	s.encoder.Encode(1, 3, 2.0)
	s.encoder.Encode(1, 4, 2.0)
	recoder := cppbridge.NewChunkRecoder(s.dataStorage)

	// Act
	chunk1 := recoder.RecodeNextChunk()
	chunk1.ChunkData = append([]byte(nil), chunk1.ChunkData...)
	chunk2 := recoder.RecodeNextChunk()

	// Assert
	s.Equal(cppbridge.RecodedChunk{
		MinT:        1,
		MaxT:        2,
		SeriesId:    0,
		HasMoreData: true,
		ChunkData:   []byte{0x00, 0x02, 0x02, 0x3f, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00},
	}, chunk1)
	s.Equal(cppbridge.RecodedChunk{
		MinT:        3,
		MaxT:        4,
		SeriesId:    1,
		HasMoreData: false,
		ChunkData:   []byte{0x00, 0x02, 0x06, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00},
	}, chunk2)
}
