package cppbridge_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

type OpenMetricsScraperHashdexSuite struct {
	suite.Suite
	hasdex *cppbridge.WALOpenMetricsScraperHashdex
}

func TestOpenMetricsScraperHashdexSuite(t *testing.T) {
	suite.Run(t, new(OpenMetricsScraperHashdexSuite))
}

func (s *OpenMetricsScraperHashdexSuite) SetupTest() {
	s.hasdex = cppbridge.NewOpenMetricsScraperHashdex()
}

func (s *OpenMetricsScraperHashdexSuite) TestParseOk() {
	// Arrange
	input := `# HELP go_gc_duration_seconds A summary of the GC invocation durations.
# TYPE go_gc_duration_seconds summary
# UNIT go_gc_duration_seconds seconds
go_gc_duration_seconds{quantile="0"} 4.9351e-05
go_gc_duration_seconds{quantile="0.25"} 7.424100000000001e-05
go_gc_duration_seconds{quantile="0.5",a="b"} 8.3835e-05
# HELP nohelp1 
# HELP help2 escape \ \n \\ \" \x chars
# UNIT nounit 
go_gc_duration_seconds{quantile="1.0",a="b"} 8.3835e-05
go_gc_duration_seconds_count 99
some:aggregate:rate5m{a_b="c"} 1
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 33 123.123
# TYPE hh histogram
hh_bucket{le="+Inf"} 1
# TYPE gh gaugehistogram
gh_bucket{le="+Inf"} 1
# TYPE hhh histogram
hhh_bucket{le="+Inf"} 1 # {id="histogram-bucket-test"} 4
hhh_count 1 # {id="histogram-count-test"} 4
# TYPE ggh gaugehistogram
ggh_bucket{le="+Inf"} 1 # {id="gaugehistogram-bucket-test",xx="yy"} 4 123.123
ggh_count 1 # {id="gaugehistogram-count-test",xx="yy"} 4 123.123
# TYPE smr_seconds summary
smr_seconds_count 2.0 # {id="summary-count-test"} 1 123.321
smr_seconds_sum 42.0 # {id="summary-sum-test"} 1 123.321
# TYPE ii info
ii{foo="bar"} 1
# TYPE ss stateset
ss{ss="foo"} 1
ss{ss="bar"} 0
ss{A="a"} 0
# TYPE un unknown
_metric_starting_with_underscore 1
testmetric{_label_starting_with_underscore="foo"} 1
testmetric{label="\"bar\""} 1
# TYPE foo counter
foo_total 17.0 1520879607.789 # {id="counter-test"} 5`

	input += "\n# HELP metric foo\x00bar"
	input += "\nnull_byte_metric{a=\"abc\x00\"} 1"
	input += "\n# EOF\n"

	// Act
	scraped, err := s.hasdex.Parse([]byte(input), -1)
	expectedMetadata := []cppbridge.WALScraperHashdexMetadata{
		{MetricName: "go_gc_duration_seconds", Text: "A summary of the GC invocation durations.", Type: cppbridge.HashdexMetadataHelp},
		{MetricName: "go_gc_duration_seconds", Text: "summary", Type: cppbridge.HashdexMetadataType},
		{MetricName: "go_gc_duration_seconds", Text: "seconds", Type: cppbridge.HashdexMetadataUnit},
		{MetricName: "nohelp1", Text: "", Type: cppbridge.HashdexMetadataHelp},
		{MetricName: "help2", Text: `escape \ \n \\ \" \x chars`, Type: cppbridge.HashdexMetadataHelp},
		{MetricName: "nounit", Text: "", Type: cppbridge.HashdexMetadataUnit},
		{MetricName: "go_goroutines", Text: "Number of goroutines that currently exist.", Type: cppbridge.HashdexMetadataHelp},
		{MetricName: "go_goroutines", Text: "gauge", Type: cppbridge.HashdexMetadataType},
		{MetricName: "hh", Text: "histogram", Type: cppbridge.HashdexMetadataType},
		{MetricName: "gh", Text: "gaugehistogram", Type: cppbridge.HashdexMetadataType},
		{MetricName: "hhh", Text: "histogram", Type: cppbridge.HashdexMetadataType},
		{MetricName: "ggh", Text: "gaugehistogram", Type: cppbridge.HashdexMetadataType},
		{MetricName: "smr_seconds", Text: "summary", Type: cppbridge.HashdexMetadataType},
		{MetricName: "ii", Text: "info", Type: cppbridge.HashdexMetadataType},
		{MetricName: "ss", Text: "stateset", Type: cppbridge.HashdexMetadataType},
		{MetricName: "un", Text: "unknown", Type: cppbridge.HashdexMetadataType},
		{MetricName: "foo", Text: "counter", Type: cppbridge.HashdexMetadataType},
		{MetricName: "metric", Text: "foo\x00bar", Type: cppbridge.HashdexMetadataHelp},
	}
	actualMetadata := make([]cppbridge.WALScraperHashdexMetadata, 0, len(expectedMetadata))
	s.hasdex.RangeMetadata(func(md cppbridge.WALScraperHashdexMetadata) bool {
		actualMetadata = append(actualMetadata, md)
		return true
	})

	// Assert
	s.NoError(err)
	s.Equal(uint32(24), scraped)
	s.Equal(expectedMetadata, actualMetadata)
}
