package cppbridge_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

type PrometheusScraperHashdexSuite struct {
	suite.Suite
	hasdex *cppbridge.WALPrometheusScraperHashdex
}

func TestPrometheusScraperHashdexSuite(t *testing.T) {
	suite.Run(t, new(PrometheusScraperHashdexSuite))
}

func (s *PrometheusScraperHashdexSuite) SetupTest() {
	s.hasdex = cppbridge.NewPrometheusScraperHashdex()
}

func (s *PrometheusScraperHashdexSuite) TestParseOk() {
	// Arrange
	input := `# HELP go_gc_duration_seconds A summary of the GC invocation durations.
# 	TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 4.9351e-05
go_gc_duration_seconds{quantile="0.25",} 7.424100000000001e-05
go_gc_duration_seconds{quantile="0.5",a="b"} 8.3835e-05
go_gc_duration_seconds{quantile="0.8", a="b"} 8.3835e-05
go_gc_duration_seconds{ quantile="0.9", a="b"} 8.3835e-05
# Hrandom comment starting with prefix of HELP
#
wind_speed{A="2",c="3"} 12345
# comment with escaped \n newline
# comment with escaped \ escape character
# HELP nohelp1
# HELP nohelp2
go_gc_duration_seconds{ quantile="1.0", a="b" } 8.3835e-05
go_gc_duration_seconds { quantile="1.0", a="b" } 8.3835e-05
go_gc_duration_seconds { quantile= "1.0", a= "b", } 8.3835e-05
go_gc_duration_seconds { quantile = "1.0", a = "b" } 8.3835e-05
go_gc_duration_seconds { quantile = "2.0" a = "b" } 8.3835e-05
go_gc_duration_seconds_count 99
some:aggregate:rate5m{a_b="c"}	1
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 33  	123123
_metric_starting_with_underscore 1
testmetric{_label_starting_with_underscore="foo"} 1
testmetric{label="\"bar\""} 1`
	input += "\n# HELP metric foo\x00bar"
	input += "\nnull_byte_metric{a=\"abc\x00\"} 1\n"

	// Act
	scraped, err := s.hasdex.Parse([]byte(input), -1)
	expectedMetadata := []cppbridge.WALScraperHashdexMetadata{
		{MetricName: "go_gc_duration_seconds", Text: "A summary of the GC invocation durations.", Type: cppbridge.HashdexMetadataHelp},
		{MetricName: "go_gc_duration_seconds", Text: "summary", Type: cppbridge.HashdexMetadataType},
		{MetricName: "nohelp1", Text: "", Type: cppbridge.HashdexMetadataHelp},
		{MetricName: "nohelp2", Text: "", Type: cppbridge.HashdexMetadataHelp},
		{MetricName: "go_goroutines", Text: "Number of goroutines that currently exist.", Type: cppbridge.HashdexMetadataHelp},
		{MetricName: "go_goroutines", Text: "gauge", Type: cppbridge.HashdexMetadataType},
		{MetricName: "metric", Text: "foo\x00bar", Type: cppbridge.HashdexMetadataHelp},
	}
	actualMetadata := make([]cppbridge.WALScraperHashdexMetadata, 0, len(expectedMetadata))
	s.hasdex.RangeMetadata(func(md cppbridge.WALScraperHashdexMetadata) bool {
		actualMetadata = append(actualMetadata, md)
		return true
	})

	// Assert
	s.NoError(err)
	s.Equal(uint32(18), scraped)
	s.Equal(expectedMetadata, actualMetadata)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperParseUnexpectedToken() {
	// Arrange
	input := []byte("a{b='c'} 1\n")

	// Act
	scraped, err := s.hasdex.Parse(input, -1)

	// Assert
	s.ErrorIs(err, cppbridge.ErrScraperParseUnexpectedToken)
	s.Equal(uint32(0), scraped)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperParseNoMetricName() {
	// Arrange
	input := []byte("{b=\"c\"} 1\n")

	// Act
	scraped, err := s.hasdex.Parse(input, -1)

	// Assert
	s.ErrorIs(err, cppbridge.ErrScraperParseNoMetricName)
	s.Equal(uint32(0), scraped)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperInvalidUtf8() {
	// Arrange
	input := []byte("a{b=\"\x80\"} 1\n")

	// Act
	scraped, err := s.hasdex.Parse(input, -1)

	// Assert
	s.ErrorIs(err, cppbridge.ErrScraperInvalidUtf8)
	s.Equal(uint32(0), scraped)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperParseInvalidValue() {
	// Arrange
	input := []byte("a{b=\"c\"} v\n")

	// Act
	scraped, err := s.hasdex.Parse(input, -1)

	// Assert
	s.ErrorIs(err, cppbridge.ErrScraperParseInvalidValue)
	s.Equal(uint32(0), scraped)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperParseInvalidTimestamp() {
	// Arrange
	input := []byte("a{b=\"c\"} 1 9223372036854775808\n")

	// Act
	scraped, err := s.hasdex.Parse(input, -1)

	// Assert
	s.ErrorIs(err, cppbridge.ErrScraperParseInvalidTimestamp)
	s.Equal(uint32(0), scraped)
}

func (s *PrometheusScraperHashdexSuite) TestParseEmptyInput() {
	// Arrange
	input := []byte{}

	// Act
	scraped, err := s.hasdex.Parse(input, -1)
	var actualMetadata []cppbridge.WALScraperHashdexMetadata
	s.hasdex.RangeMetadata(func(md cppbridge.WALScraperHashdexMetadata) bool {
		actualMetadata = append(actualMetadata, md)
		return true
	})

	// Assert
	s.NoError(err)
	s.Equal([]cppbridge.WALScraperHashdexMetadata(nil), actualMetadata)
	s.Equal(uint32(0), scraped)
}
