package cppbridge_test

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/stretchr/testify/suite"
)

type HeadStatusSuite struct {
	suite.Suite
	dataStorage *cppbridge.HeadDataStorage
	encoder     *cppbridge.HeadEncoder
	lssStorage  *cppbridge.LabelSetStorage
}

func TestHeadStatusSuite(t *testing.T) {
	suite.Run(t, new(HeadStatusSuite))
}

func (s *HeadStatusSuite) SetupTest() {
	s.dataStorage = cppbridge.NewHeadDataStorage()
	s.encoder = cppbridge.NewHeadEncoderWithDataStorage(s.dataStorage)
	s.lssStorage = cppbridge.NewQueryableLssStorage()
}

func (s *HeadStatusSuite) TestSomeData() {
	// Arrange
	label_sets := []model.LabelSet{
		model.NewLabelSetBuilder().Set("job", "cron").Set("server", "localhost").Set("__name__", "php").Build(),
		model.NewLabelSetBuilder().Set("job", "cron").Set("server", "127.0.0.1:8000").Set("__name__", "c++").Build(),
		model.NewLabelSetBuilder().Set("job", "cro1").Set("ip", "127.0.0.1").Set("__name__", "nodejs").Build(),
	}
	for _, label_set := range label_sets {
		s.lssStorage.FindOrEmplace(label_set)
	}

	s.encoder.Encode(0, 1, 1.0)
	s.encoder.Encode(1, 3, 1.0)
	s.encoder.Encode(2, 3, 1.0)

	// Act
	status := cppbridge.GetHeadStatus(s.lssStorage.Pointer(), s.dataStorage.Pointer())

	// Assert
	s.Equal(cppbridge.HeadStatus{
		TimeInterval: struct {
			Min int64
			Max int64
		}{Min: 1, Max: 3},
		LabelValueCountByLabelName: []struct {
			Name  string
			Count uint32
		}{{Name: "__name__", Count: 3},
			{Name: "job", Count: 2},
			{Name: "server", Count: 2},
			{Name: "ip", Count: 1}},
		SeriesCountByMetricName: []struct {
			Name  string
			Count uint32
		}{{Name: "php", Count: 1},
			{Name: "c++", Count: 1},
			{Name: "nodejs", Count: 1}},
		MemoryInBytesByLabelName: []struct {
			Name string
			Size uint32
		}{{Name: "server", Size: 23},
			{Name: "__name__", Size: 12},
			{Name: "job", Size: 12},
			{Name: "ip", Size: 9}},
		SeriesCountByLabelValuePair: []struct {
			Name, Value string
			Count       uint32
		}{{Name: "job", Value: "cron", Count: 2},
			{Name: "__name__", Value: "php", Count: 1},
			{Name: "__name__", Value: "c++", Count: 1},
			{Name: "__name__", Value: "nodejs", Count: 1},
			{Name: "job", Value: "cro1", Count: 1},
			{Name: "server", Value: "localhost", Count: 1},
			{Name: "server", Value: "127.0.0.1:8000", Count: 1},
			{Name: "ip", Value: "127.0.0.1", Count: 1}},
		NumSeries:     3,
		ChunkCount:    3,
		NumLabelPairs: 8}, *status)
}
