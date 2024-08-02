package cppbridge_test

import (
	//"fmt"
	"fmt"
	"os"
	"testing"

	// "github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/require"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/stretchr/testify/suite"
)

type IndexWriterSuite struct {
	suite.Suite
	lssStorage          *cppbridge.LabelSetStorage
	chunk_metadata_list [][]cppbridge.ChunkMetadata
}

func TestIndexWriterSuite(t *testing.T) {
	suite.Run(t, new(IndexWriterSuite))
}

func (s *IndexWriterSuite) SetupTest() {
	s.lssStorage = cppbridge.NewQueryableLssStorage()

	s.chunk_metadata_list = make([][]cppbridge.ChunkMetadata, 3)
	for i := 0; i < 3; i++ {
		s.chunk_metadata_list[i] = make([]cppbridge.ChunkMetadata, 2)
		s.chunk_metadata_list[i][0] = cppbridge.ChunkMetadata{MinTimestamp: 1000, MaxTimestamp: 2001, Size: 100}
		s.chunk_metadata_list[i][1] = cppbridge.ChunkMetadata{MinTimestamp: 2002, MaxTimestamp: 4004, Size: 125}
	}
}

func (s *IndexWriterSuite) TestInvalidLss() {
	lss := cppbridge.NewLssStorage()
	writer, err := cppbridge.NewIndexWriter(lss.Pointer(), &s.chunk_metadata_list)

	s.EqualError(err, "Invalid lss")
	s.Nil(writer)
}

func (s *IndexWriterSuite) TestFullWrite() {
	writer, err := cppbridge.NewIndexWriter(s.lssStorage.Pointer(), &s.chunk_metadata_list)
	if err != nil {
		fmt.Println(err)
		return
	}

	file, _ := os.Create("/pp/performance_tests/data/01HDRCKGAQKHYT8NE2ESPQ5QJ5/go_index")
	defer file.Close()

	file.Write(writer.WriteHeader())
	file.Write(writer.WriteSymbols())

	for {
		data, has_more_data := writer.WriteNextSeriesBatch(1)
		file.Write(data)
		if !has_more_data {
			break
		}
	}

	file.Write(writer.WriteLabelIndices())

	for {
		data, has_more_data := writer.WriteNextPostingsBatch(16)
		file.Write(data)
		if !has_more_data {
			break
		}
	}

	file.Write(writer.WriteLabelIndicesTable())
	file.Write(writer.WritePostingsTableOffsets())
	file.Write(writer.WriteTableOfContents())

	s.chunk_metadata_list[0][1].MaxTimestamp = 100023
}
