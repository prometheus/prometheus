package cppbridge_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/go-faker/faker/v4"
	"github.com/go-faker/faker/v4/pkg/options"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames/framestest"
	"github.com/prometheus/prometheus/pp/go/model"
)

type HashdexSuite struct {
	suite.Suite
	baseCtx        context.Context
	startTimestamp int64
	step           int64
}

func TestHashdexSuite(t *testing.T) {
	suite.Run(t, new(HashdexSuite))
}

func (s *HashdexSuite) SetupTest() {
	s.baseCtx = context.Background()
	s.startTimestamp = 1654608420000
	s.step = 60000
}

func (s *HashdexSuite) makeData(i int64) []byte {
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
					},
					{
						Name:  "job",
						Value: "tester",
					},
					{
						Name:  "instance",
						Value: "blablabla",
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: s.startTimestamp + (s.step * i),
						Value:     4444,
					},
					{
						Timestamp: s.startTimestamp + (s.step * i * 2),
						Value:     4445,
					},
				},
			},
		},
	}

	b, err := wr.Marshal()
	s.Require().NoError(err)
	return b
}

func (s *HashdexSuite) makeDataWithTwoTimeseries() []byte {
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
					},
					{
						Name:  "job",
						Value: "tester",
					},
					{
						Name:  "instance",
						Value: "blablabla",
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: 1654608420000,
						Value:     4444,
					},
				},
			},
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
					},
					{
						Name:  "job",
						Value: "tester",
					},
					{
						Name:  "instance",
						Value: "blablabla",
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: 1654608420666,
						Value:     4666,
					},
				},
			},
		},
	}

	b, err := wr.Marshal()
	s.Require().NoError(err)
	return b
}

func (s *HashdexSuite) TestCppInvalidDataForHashdex() {
	invalidPbData := []byte("1111")
	hlimits := cppbridge.DefaultWALHashdexLimits()
	h, err := cppbridge.NewWALProtobufHashdex(invalidPbData, hlimits)
	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *HashdexSuite) TestHashdexWithHardLimitsOnLabelNameLength() {
	limits := cppbridge.WALHashdexLimits{
		MaxLabelNameLength: 2,
	}
	data := s.makeData(1)
	h, err := cppbridge.NewWALProtobufHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *HashdexSuite) TestHashdexWithHardLimitsOnLabelValueLength() {
	limits := cppbridge.WALHashdexLimits{
		MaxLabelValueLength: 2,
	}
	data := s.makeData(1)
	h, err := cppbridge.NewWALProtobufHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *HashdexSuite) TestHashdexWithHardLimitsOnLabelsInTimeseries() {
	limits := cppbridge.WALHashdexLimits{
		MaxLabelNamesPerTimeseries: 1,
	}
	data := s.makeData(1)
	h, err := cppbridge.NewWALProtobufHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func (s *HashdexSuite) TestHashdexWithVeryHardLimitsOnTimeseries() {
	limits := cppbridge.WALHashdexLimits{
		MaxTimeseriesCount: 1,
	}
	data := s.makeDataWithTwoTimeseries()
	h, err := cppbridge.NewWALProtobufHashdex(data, limits)

	s.Error(err)
	s.T().Logf("Got an error (it's OK): %s", err.Error())
	_ = h
}

func TestHashdex_ClusterReplica(t *testing.T) {
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
					},
					{
						Name:  "cluster",
						Value: "cluster-0",
					},
					{
						Name:  "__replica__",
						Value: "replica-1",
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: -1654608420000,
						Value:     4444,
					},
				},
			},
		},
	}
	b, err := wr.Marshal()
	require.NoError(t, err)

	hlimits := cppbridge.DefaultWALHashdexLimits()
	h, err := cppbridge.NewWALProtobufHashdex(b, hlimits)
	require.NoError(t, err)
	cluster := h.Cluster()
	replica := h.Replica()
	t.Log("clean source protobuf message")
	copy(b, make([]byte, len(b)))
	t.Log("check that cluster and replica stay unchanged")
	assert.Equal(t, "cluster-0", cluster)
	assert.Equal(t, "replica-1", replica)
}

const (
	clusterLabelName = "cluster"
	replicaLabelName = "__replica__"
)

type GoModelHashdexTestSuite struct {
	suite.Suite
	ctx  context.Context
	rand *rand.Rand
}

func TestGoModelHashdexTestSuite(t *testing.T) {
	suite.Run(t, new(GoModelHashdexTestSuite))
}

func (s *GoModelHashdexTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.rand = rand.New(rand.NewSource(time.Now().UnixNano()))
}

func (s *GoModelHashdexTestSuite) makeTestData(limits cppbridge.WALHashdexLimits, size int) []model.TimeSeries {
	tss := make([]model.TimeSeries, 0, size)

	for i := 0; i < size; i++ {
		lsb := model.NewLabelSetBuilder()
		for j := 0; j < 1+s.rand.Intn(int(limits.MaxLabelNamesPerTimeseries-1)); j++ {
			lsb.Set(
				faker.Word(options.WithRandomStringLength(1+uint(s.rand.Intn(int(limits.MaxLabelNameLength-1))))),
				faker.Word(options.WithRandomStringLength(1+uint(s.rand.Intn(int(limits.MaxLabelValueLength-1))))),
			)
		}
		ts := model.TimeSeries{
			LabelSet:  lsb.Build(),
			Timestamp: uint64(time.Now().Unix()),
			Value:     s.rand.ExpFloat64(),
		}
		tss = append(tss, ts)
	}
	return tss
}

func (s *GoModelHashdexTestSuite) TestHappyPath() {
	encodersVersion := cppbridge.EncodersVersion()
	limits := cppbridge.DefaultWALHashdexLimits()
	dec := cppbridge.NewWALDecoder(encodersVersion)
	enc := cppbridge.NewWALEncoder(0, 0)

	testData := s.makeTestData(limits, 1000)
	hdx, err := cppbridge.NewWALGoModelHashdex(limits, testData)
	s.Require().NoError(err)

	segStat, err := enc.Add(s.ctx, hdx)
	s.Require().NoError(err)

	s.Require().EqualValues(len(testData), segStat.Series())

	_, seg, err := enc.Finalize(s.ctx)
	s.Require().NoError(err)

	buf, err := framestest.ReadPayload(seg)
	s.Require().NoError(err)

	protoData, err := dec.Decode(s.ctx, buf)
	s.Require().NoError(err)

	prometheusWriteRequest := &prompb.WriteRequest{}
	s.Require().NoError(protoData.UnmarshalTo(prometheusWriteRequest))

	s.Require().EqualValues(len(testData), len(prometheusWriteRequest.Timeseries))

	for i := 0; i < len(testData); i++ {
		s.Require().Len(prometheusWriteRequest.Timeseries[i].Samples, 1)
		s.Require().EqualValues(testData[i].Timestamp, prometheusWriteRequest.Timeseries[i].Samples[0].Timestamp)
		s.Require().EqualValues(testData[i].Value, prometheusWriteRequest.Timeseries[i].Samples[0].Value)
		for j := 0; j < testData[i].LabelSet.Len(); j++ {
			expectedName := testData[i].LabelSet.Key(j)
			expectedValue := testData[i].LabelSet.Value(j)
			s.Require().Equal(expectedName, prometheusWriteRequest.Timeseries[i].Labels[j].Name)
			s.Require().Equal(expectedValue, prometheusWriteRequest.Timeseries[i].Labels[j].Value)
		}
	}
}

func (s *GoModelHashdexTestSuite) TestHashdexLabels() {
	cluster := "the_cluster"
	replica := "the_replica"

	// cluster & replica labels
	{
		limits := cppbridge.DefaultWALHashdexLimits()
		ts := model.TimeSeries{
			LabelSet: model.NewLabelSetBuilder().
				Set(clusterLabelName, cluster).
				Set(replicaLabelName, replica).
				Build(),
			Timestamp: 42,
			Value:     25,
		}

		hdx, err := cppbridge.NewWALGoModelHashdex(limits, []model.TimeSeries{ts})
		s.Require().NoError(err)
		require.Equal(s.T(), cluster, hdx.Cluster())
		require.Equal(s.T(), replica, hdx.Replica())
	}

	// max label name size exceeded
	{
		limits := cppbridge.WALHashdexLimits{
			MaxLabelNameLength: 5,
		}

		ts := model.TimeSeries{
			LabelSet:  model.NewLabelSetBuilder().Set("123456", "value").Build(),
			Timestamp: 42,
			Value:     25,
		}
		_, err := cppbridge.NewWALGoModelHashdex(limits, []model.TimeSeries{ts})
		s.Require().Error(err)
	}

	// max label value size exceeded
	{
		limits := cppbridge.WALHashdexLimits{
			MaxLabelValueLength: 5,
		}

		ts := model.TimeSeries{
			LabelSet:  model.NewLabelSetBuilder().Set("123456", "1234567").Build(),
			Timestamp: 42,
			Value:     25,
		}
		_, err := cppbridge.NewWALGoModelHashdex(limits, []model.TimeSeries{ts})
		s.Require().Error(err)
	}

	// max number of time series exceeded
	{
		limits := cppbridge.WALHashdexLimits{
			MaxTimeseriesCount: 1,
		}

		tss := []model.TimeSeries{
			{}, {},
		}

		_, err := cppbridge.NewWALGoModelHashdex(limits, tss)
		s.Require().Error(err)
	}
}

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
	err := s.hasdex.Parse([]byte(input), -1)
	expectedMetadata := []cppbridge.WALScraperHashdexMetadata{
		{MetricName: "go_gc_duration_seconds", Text: "A summary of the GC invocation durations.", Type: cppbridge.ScraperMetadataHelp},
		{MetricName: "go_gc_duration_seconds", Text: "summary", Type: cppbridge.ScraperMetadataType},
		{MetricName: "nohelp1", Text: "", Type: cppbridge.ScraperMetadataHelp},
		{MetricName: "nohelp2", Text: "", Type: cppbridge.ScraperMetadataHelp},
		{MetricName: "go_goroutines", Text: "Number of goroutines that currently exist.", Type: cppbridge.ScraperMetadataHelp},
		{MetricName: "go_goroutines", Text: "gauge", Type: cppbridge.ScraperMetadataType},
		{MetricName: "metric", Text: "foo\x00bar", Type: cppbridge.ScraperMetadataHelp},
	}
	actualMetadata := make([]cppbridge.WALScraperHashdexMetadata, 0, len(expectedMetadata))
	s.hasdex.RangeMetadata(func(md cppbridge.WALScraperHashdexMetadata) bool {
		actualMetadata = append(actualMetadata, md)
		return true
	})

	// Assert
	s.NoError(err)
	s.Equal(expectedMetadata, actualMetadata)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperParseUnexpectedToken() {
	// Arrange
	input := []byte("a{b='c'} 1\n")

	// Act
	err := s.hasdex.Parse(input, -1)

	// Assert
	s.ErrorIs(err, cppbridge.ErrScraperParseUnexpectedToken)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperParseNoMetricName() {
	// Arrange
	input := []byte("{b=\"c\"} 1\n")

	// Act
	err := s.hasdex.Parse(input, -1)

	// Assert
	s.ErrorIs(err, cppbridge.ErrScraperParseNoMetricName)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperInvalidUtf8() {
	// Arrange
	input := []byte("a{b=\"\x80\"} 1\n")

	// Act
	err := s.hasdex.Parse(input, -1)

	// Assert
	s.ErrorIs(err, cppbridge.ErrScraperInvalidUtf8)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperParseInvalidValue() {
	// Arrange
	input := []byte("a{b=\"c\"} v\n")

	// Act
	err := s.hasdex.Parse(input, -1)

	// Assert
	s.ErrorIs(err, cppbridge.ErrScraperParseInvalidValue)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperParseInvalidTimestamp() {
	// Arrange
	input := []byte("a{b=\"c\"} 1 9223372036854775808\n")

	// Act
	err := s.hasdex.Parse(input, -1)

	// Assert
	s.ErrorIs(err, cppbridge.ErrScraperParseInvalidTimestamp)
}

func (s *PrometheusScraperHashdexSuite) TestParseEmptyInput() {
	// Arrange
	input := []byte{}

	// Act
	err := s.hasdex.Parse(input, -1)
	var actualMetadata []cppbridge.WALScraperHashdexMetadata
	s.hasdex.RangeMetadata(func(md cppbridge.WALScraperHashdexMetadata) bool {
		actualMetadata = append(actualMetadata, md)
		return true
	})

	// Assert
	s.NoError(err)
	s.Equal([]cppbridge.WALScraperHashdexMetadata(nil), actualMetadata)
}

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
	err := s.hasdex.Parse([]byte(input), -1)
	expectedMetadata := []cppbridge.WALScraperHashdexMetadata{
		{MetricName: "go_gc_duration_seconds", Text: "A summary of the GC invocation durations.", Type: cppbridge.ScraperMetadataHelp},
		{MetricName: "go_gc_duration_seconds", Text: "summary", Type: cppbridge.ScraperMetadataType},
		{MetricName: "go_gc_duration_seconds", Text: "seconds", Type: cppbridge.ScraperMetadataUnit},
		{MetricName: "nohelp1", Text: "", Type: cppbridge.ScraperMetadataHelp},
		{MetricName: "help2", Text: `escape \ \n \\ \" \x chars`, Type: cppbridge.ScraperMetadataHelp},
		{MetricName: "nounit", Text: "", Type: cppbridge.ScraperMetadataUnit},
		{MetricName: "go_goroutines", Text: "Number of goroutines that currently exist.", Type: cppbridge.ScraperMetadataHelp},
		{MetricName: "go_goroutines", Text: "gauge", Type: cppbridge.ScraperMetadataType},
		{MetricName: "hh", Text: "histogram", Type: cppbridge.ScraperMetadataType},
		{MetricName: "gh", Text: "gaugehistogram", Type: cppbridge.ScraperMetadataType},
		{MetricName: "hhh", Text: "histogram", Type: cppbridge.ScraperMetadataType},
		{MetricName: "ggh", Text: "gaugehistogram", Type: cppbridge.ScraperMetadataType},
		{MetricName: "smr_seconds", Text: "summary", Type: cppbridge.ScraperMetadataType},
		{MetricName: "ii", Text: "info", Type: cppbridge.ScraperMetadataType},
		{MetricName: "ss", Text: "stateset", Type: cppbridge.ScraperMetadataType},
		{MetricName: "un", Text: "unknown", Type: cppbridge.ScraperMetadataType},
		{MetricName: "foo", Text: "counter", Type: cppbridge.ScraperMetadataType},
		{MetricName: "metric", Text: "foo\x00bar", Type: cppbridge.ScraperMetadataHelp},
	}
	actualMetadata := make([]cppbridge.WALScraperHashdexMetadata, 0, len(expectedMetadata))
	s.hasdex.RangeMetadata(func(md cppbridge.WALScraperHashdexMetadata) bool {
		actualMetadata = append(actualMetadata, md)
		return true
	})

	// Assert
	s.NoError(err)
	s.Equal(expectedMetadata, actualMetadata)
}
