package agent

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/chunks"

	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

const checkpointFixturesDir = "./testdata/db-fixtures/"

func BenchmarkCheckpoint(b *testing.B) {
}

func TestCreateCheckpointFixtures(t *testing.T) {
	createCheckpointFixtures(t, checkpointFixtureParams{
		dir:         checkpointFixturesDir,
		numSegments: 512,
		numSeries:   32,
		dtDelta:     10000,
		segmentSize: 32 << 10, // must be aligned to page size
	})
}

type checkpointFixtureParams struct {
	dir         string
	numSegments int
	numSeries   int
	segmentSize int
	dtDelta     int64
}

func createCheckpointFixtures(t testing.TB, p checkpointFixtureParams) {
	// Make a segment to put initial data
	var enc record.Encoder

	// Create first dummy segment to bump the start segment number.
	seg, err := wlog.CreateSegment(p.dir, 100)
	require.NoError(t, err)
	require.NoError(t, seg.Close())

	w, err := wlog.NewSize(promslog.NewNopLogger(), nil, p.dir, p.segmentSize, DefaultOptions().WALCompression)
	require.NoError(t, err)

	series := make([]record.RefSeries, 0, p.numSeries)
	meta := make([]record.RefMetadata, 0, p.numSeries)
	for i := range p.numSeries {
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: labels.FromStrings("foo", "bar", "bar", strconv.Itoa(i)),
		})
		meta = append(meta, record.RefMetadata{
			Ref:  chunks.HeadSeriesRef(i),
			Unit: "unit",
			Help: "help",
		})
	}

	var dt int64
	samples := make([]record.RefSample, 0, len(series))
	for i := range p.numSegments {
		if i == 0 {
			// Write series required for samples
			b := enc.Series(series, nil)
			require.NoError(t, w.Log(b))

			b = enc.Metadata(meta, nil)
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

func makeHistogram(i int) *histogram.Histogram {
	return &histogram.Histogram{
		Count:         5 + uint64(i*4),
		ZeroCount:     2 + uint64(i),
		ZeroThreshold: 0.001,
		Sum:           18.4 * float64(i+1),
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{int64(i + 1), 1, -1, 0},
	}
}

func makeCustomBucketHistogram(i int) *histogram.Histogram {
	return &histogram.Histogram{
		Count:         5 + uint64(i*4),
		ZeroCount:     2 + uint64(i),
		ZeroThreshold: 0.001,
		Sum:           18.4 * float64(i+1),
		Schema:        -53,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		CustomValues: []float64{0, 1, 2, 3, 4},
	}
}

func makeFloatHistogram(i int) *histogram.FloatHistogram {
	return &histogram.FloatHistogram{
		Count:         5 + float64(i*4),
		ZeroCount:     2 + float64(i),
		ZeroThreshold: 0.001,
		Sum:           18.4 * float64(i+1),
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []float64{float64(i + 1), 1, -1, 0},
	}
}

func makeCustomBucketFloatHistogram(i int) *histogram.FloatHistogram {
	return &histogram.FloatHistogram{
		Count:         5 + float64(i*4),
		ZeroCount:     2 + float64(i),
		ZeroThreshold: 0.001,
		Sum:           18.4 * float64(i+1),
		Schema:        -53,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		CustomValues: []float64{0, 1, 2, 3, 4},
	}
}
