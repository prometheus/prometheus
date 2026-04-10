package remote

import (
	"strconv"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

type testSamplesParams struct {
	labelPrefix   string
	numDatapoints int
	numHistograms int
	numSeries     int
}

type testSamples struct {
	datapointLabels  [][]labels.Label
	histogramLabels  [][]labels.Label
	datapointSamples [][]chunks.Sample
	histogramSamples [][]*histogram.Histogram
}

// genTestSamples generates samples and labels to be written by appender.
func genTestSamples(p testSamplesParams) testSamples {
	out := testSamples{
		datapointLabels:  labelsForTest(p.labelPrefix, p.numSeries),
		histogramLabels:  labelsForTest(p.labelPrefix+"_histogram", p.numSeries),
		datapointSamples: make([][]chunks.Sample, 0, p.numSeries),
		histogramSamples: make([][]*histogram.Histogram, 0, p.numSeries),
	}

	for range p.numDatapoints {
		sample := chunks.GenerateSamples(0, 1)
		out.datapointSamples = append(out.datapointSamples, sample)
	}

	for range out.histogramLabels {
		histograms := tsdbutil.GenerateTestHistograms(p.numHistograms)
		out.histogramSamples = append(out.histogramSamples, histograms)
	}

	return out
}

type walFixtureParams struct {
	dir          string
	numSegments  int
	segmentSize  int
	dtDelta      int64
	seriesLabels [][]labels.Label
}

func createWALFixtures(t testing.TB, p walFixtureParams) {
	// Make a segment to put initial data
	var enc record.Encoder

	// Create dummy segment to bump the start segment number.
	// Dummy segment should be zero or agent.Open() will fail.
	seg, err := wlog.CreateSegment(p.dir, 0)
	require.NoError(t, err)
	require.NoError(t, seg.Close())

	w, err := wlog.NewSize(promslog.NewNopLogger(), nil, p.dir, p.segmentSize, compression.None)
	require.NoError(t, err)

	series := make([]record.RefSeries, 0, len(p.seriesLabels))
	for i, lset := range p.seriesLabels {
		// NOTE: don't append RefMetadata as agent.DB doesn't support it during WAL replay.
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: labels.New(lset...),
		})
	}

	var dt int64
	samples := make([]record.RefSample, 0, len(series))
	for i := range p.numSegments {
		if i == 0 {
			// Write series required for samples
			b := enc.Series(series, nil)
			require.NoError(t, w.Log(b))
		}

		samples = samples[:0]
		for j := range len(series) {
			samples = append(samples, record.RefSample{
				Ref: chunks.HeadSeriesRef(j),
				V:   float64(i),
				T:   dt + int64(j+1),
			})
		}
		require.NoError(t, w.Log(enc.Samples(samples, nil)))
		dt += p.dtDelta
	}
	require.NoError(t, w.Close(), "WAL.Close")
}

func labelsForTest(lName string, seriesCount int) [][]labels.Label {
	var series [][]labels.Label

	for i := range seriesCount {
		lset := []labels.Label{
			{Name: "a", Value: lName},
			{Name: "instance", Value: "localhost" + strconv.Itoa(i)},
			{Name: "job", Value: "prometheus"},
		}
		series = append(series, lset)
	}

	return series
}
