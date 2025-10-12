// Copyright 2017 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package remote

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/annotations"
)

var (
	testHistogram = histogram.Histogram{
		Schema:          2,
		ZeroThreshold:   1e-128,
		ZeroCount:       0,
		Count:           3,
		Sum:             20,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
		PositiveBuckets: []int64{1},
		NegativeSpans:   []histogram.Span{{Offset: 0, Length: 1}},
		NegativeBuckets: []int64{2},
	}

	writeRequestFixture = &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "test_metric1"},
					{Name: "b", Value: "c"},
					{Name: "baz", Value: "qux"},
					{Name: "d", Value: "e"},
					{Name: "foo", Value: "bar"},
				},
				Samples:    []prompb.Sample{{Value: 1, Timestamp: 1}},
				Exemplars:  []prompb.Exemplar{{Labels: []prompb.Label{{Name: "f", Value: "g"}}, Value: 1, Timestamp: 1}},
				Histograms: []prompb.Histogram{prompb.FromIntHistogram(1, &testHistogram), prompb.FromFloatHistogram(2, testHistogram.ToFloat(nil))},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "test_metric1"},
					{Name: "b", Value: "c"},
					{Name: "baz", Value: "qux"},
					{Name: "d", Value: "e"},
					{Name: "foo", Value: "bar"},
				},
				Samples:    []prompb.Sample{{Value: 2, Timestamp: 2}},
				Exemplars:  []prompb.Exemplar{{Labels: []prompb.Label{{Name: "h", Value: "i"}}, Value: 2, Timestamp: 2}},
				Histograms: []prompb.Histogram{prompb.FromIntHistogram(3, &testHistogram), prompb.FromFloatHistogram(4, testHistogram.ToFloat(nil))},
			},
		},
	}

	writeV2RequestSeries1Metadata = metadata.Metadata{
		Type: model.MetricTypeGauge,
		Help: "Test gauge for test purposes",
		Unit: "Maybe op/sec who knows (:",
	}
	writeV2RequestSeries2Metadata = metadata.Metadata{
		Type: model.MetricTypeCounter,
		Help: "Test counter for test purposes",
	}

	testHistogramCustomBuckets = histogram.Histogram{
		Schema:          histogram.CustomBucketsSchema,
		Count:           16,
		Sum:             20,
		PositiveSpans:   []histogram.Span{{Offset: 1, Length: 2}},
		PositiveBuckets: []int64{10, -4},     // Means 10 observations for upper bound 1.0 and 6 for upper bound +Inf.
		CustomValues:    []float64{0.1, 1.0}, // +Inf is implied.
	}

	// writeV2RequestFixture represents the same request as writeRequestFixture,
	// but using the v2 representation, plus includes writeV2RequestSeries1Metadata and writeV2RequestSeries2Metadata.
	// NOTE: Use TestWriteV2RequestFixture and copy the diff to regenerate if needed.
	writeV2RequestFixture = &writev2.Request{
		Symbols: []string{"", "__name__", "test_metric1", "b", "c", "baz", "qux", "d", "e", "foo", "bar", "f", "g", "h", "i", "Test gauge for test purposes", "Maybe op/sec who knows (:", "Test counter for test purposes"},
		Timeseries: []writev2.TimeSeries{
			{
				LabelsRefs: []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, // Symbolized writeRequestFixture.Timeseries[0].Labels
				Metadata: writev2.Metadata{
					Type: writev2.Metadata_METRIC_TYPE_GAUGE, // writeV2RequestSeries1Metadata.Type.

					HelpRef: 15, // Symbolized writeV2RequestSeries1Metadata.Help.
					UnitRef: 16, // Symbolized writeV2RequestSeries1Metadata.Unit.
				},
				Samples:   []writev2.Sample{{Value: 1, Timestamp: 10}},
				Exemplars: []writev2.Exemplar{{LabelsRefs: []uint32{11, 12}, Value: 1, Timestamp: 10}},
				Histograms: []writev2.Histogram{
					writev2.FromIntHistogram(10, &testHistogram),
					writev2.FromFloatHistogram(20, testHistogram.ToFloat(nil)),
					writev2.FromIntHistogram(30, &testHistogramCustomBuckets),
					writev2.FromFloatHistogram(40, testHistogramCustomBuckets.ToFloat(nil)),
				},
				CreatedTimestamp: 1, // CT needs to be lower than the sample's timestamp.
			},
			{
				LabelsRefs: []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, // Same series as first.
				Metadata: writev2.Metadata{
					Type: writev2.Metadata_METRIC_TYPE_COUNTER, // writeV2RequestSeries2Metadata.Type.

					HelpRef: 17, // Symbolized writeV2RequestSeries2Metadata.Help.
					// No unit.
				},
				Samples:   []writev2.Sample{{Value: 2, Timestamp: 20}},
				Exemplars: []writev2.Exemplar{{LabelsRefs: []uint32{13, 14}, Value: 2, Timestamp: 20}},
				Histograms: []writev2.Histogram{
					writev2.FromIntHistogram(50, &testHistogram),
					writev2.FromFloatHistogram(60, testHistogram.ToFloat(nil)),
					writev2.FromIntHistogram(70, &testHistogramCustomBuckets),
					writev2.FromFloatHistogram(80, testHistogramCustomBuckets.ToFloat(nil)),
				},
			},
		},
	}
)

func TestHistogramFixtureValid(t *testing.T) {
	for _, ts := range writeRequestFixture.Timeseries {
		for _, h := range ts.Histograms {
			if h.IsFloatHistogram() {
				require.NoError(t, h.ToFloatHistogram().Validate())
			} else {
				require.NoError(t, h.ToIntHistogram().Validate())
			}
		}
	}
	for _, ts := range writeV2RequestFixture.Timeseries {
		for _, h := range ts.Histograms {
			if h.IsFloatHistogram() {
				require.NoError(t, h.ToFloatHistogram().Validate())
			} else {
				require.NoError(t, h.ToIntHistogram().Validate())
			}
		}
	}
}

func TestWriteV2RequestFixture(t *testing.T) {
	// Generate dynamically writeV2RequestFixture, reusing v1 fixture elements.
	st := writev2.NewSymbolTable()
	b := labels.NewScratchBuilder(0)
	labelRefs := st.SymbolizeLabels(writeRequestFixture.Timeseries[0].ToLabels(&b, nil), nil)
	exemplar1LabelRefs := st.SymbolizeLabels(writeRequestFixture.Timeseries[0].Exemplars[0].ToExemplar(&b, nil).Labels, nil)
	exemplar2LabelRefs := st.SymbolizeLabels(writeRequestFixture.Timeseries[1].Exemplars[0].ToExemplar(&b, nil).Labels, nil)
	expected := &writev2.Request{
		Timeseries: []writev2.TimeSeries{
			{
				LabelsRefs: labelRefs,
				Metadata: writev2.Metadata{
					Type:    writev2.Metadata_METRIC_TYPE_GAUGE,
					HelpRef: st.Symbolize(writeV2RequestSeries1Metadata.Help),
					UnitRef: st.Symbolize(writeV2RequestSeries1Metadata.Unit),
				},
				Samples:   []writev2.Sample{{Value: 1, Timestamp: 10}},
				Exemplars: []writev2.Exemplar{{LabelsRefs: exemplar1LabelRefs, Value: 1, Timestamp: 10}},
				Histograms: []writev2.Histogram{
					writev2.FromIntHistogram(10, &testHistogram),
					writev2.FromFloatHistogram(20, testHistogram.ToFloat(nil)),
					writev2.FromIntHistogram(30, &testHistogramCustomBuckets),
					writev2.FromFloatHistogram(40, testHistogramCustomBuckets.ToFloat(nil)),
				},
				CreatedTimestamp: 1,
			},
			{
				LabelsRefs: labelRefs,
				Metadata: writev2.Metadata{
					Type:    writev2.Metadata_METRIC_TYPE_COUNTER,
					HelpRef: st.Symbolize(writeV2RequestSeries2Metadata.Help),
					// No unit.
				},
				Samples:   []writev2.Sample{{Value: 2, Timestamp: 20}},
				Exemplars: []writev2.Exemplar{{LabelsRefs: exemplar2LabelRefs, Value: 2, Timestamp: 20}},
				Histograms: []writev2.Histogram{
					writev2.FromIntHistogram(50, &testHistogram),
					writev2.FromFloatHistogram(60, testHistogram.ToFloat(nil)),
					writev2.FromIntHistogram(70, &testHistogramCustomBuckets),
					writev2.FromFloatHistogram(80, testHistogramCustomBuckets.ToFloat(nil)),
				},
			},
		},
		Symbols: st.Symbols(),
	}
	// Check if it matches static writeV2RequestFixture.
	require.Equal(t, expected, writeV2RequestFixture)
}

func TestValidateLabelsAndMetricName(t *testing.T) {
	tests := []struct {
		input       []prompb.Label
		expectedErr string
		description string
	}{
		{
			input: []prompb.Label{
				{Name: "__name__", Value: "name"},
				{Name: "labelName", Value: "labelValue"},
			},
			expectedErr: "",
			description: "regular labels",
		},
		{
			input: []prompb.Label{
				{Name: "__name__", Value: "name"},
				{Name: "_labelName", Value: "labelValue"},
			},
			expectedErr: "",
			description: "label name with _",
		},
		{
			input: []prompb.Label{
				{Name: "__name__", Value: "name"},
				{Name: "@labelName\xff", Value: "labelValue"},
			},
			expectedErr: "invalid label name: @labelName\xff",
			description: "label name with \xff",
		},
		{
			input: []prompb.Label{
				{Name: "__name__", Value: "name"},
				{Name: "", Value: "labelValue"},
			},
			expectedErr: "invalid label name: ",
			description: "label name is empty string",
		},
		{
			input: []prompb.Label{
				{Name: "__name__", Value: "name"},
				{Name: "labelName", Value: string([]byte{0xff})},
			},
			expectedErr: "invalid label value: " + string([]byte{0xff}),
			description: "label value is an invalid UTF-8 value",
		},
		{
			input: []prompb.Label{
				{Name: "__name__", Value: "invalid_name\xff"},
			},
			expectedErr: "invalid metric name: invalid_name\xff",
			description: "metric name has invalid utf8",
		},
		{
			input: []prompb.Label{
				{Name: "__name__", Value: "name1"},
				{Name: "__name__", Value: "name2"},
			},
			expectedErr: "duplicate label with name: __name__",
			description: "duplicate label names",
		},
		{
			input: []prompb.Label{
				{Name: "label1", Value: "name"},
				{Name: "label2", Value: "name"},
			},
			expectedErr: "",
			description: "duplicate label values",
		},
		{
			input: []prompb.Label{
				{Name: "", Value: "name"},
				{Name: "label2", Value: "name"},
			},
			expectedErr: "invalid label name: ",
			description: "don't report as duplicate label name",
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			err := validateLabelsAndMetricName(test.input)
			if test.expectedErr != "" {
				require.EqualError(t, err, test.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConcreteSeriesSet(t *testing.T) {
	series1 := &concreteSeries{
		labels: labels.FromStrings("foo", "bar"),
		floats: []prompb.Sample{{Value: 1, Timestamp: 2}},
	}
	series2 := &concreteSeries{
		labels: labels.FromStrings("foo", "baz"),
		floats: []prompb.Sample{{Value: 3, Timestamp: 4}},
	}
	c := &concreteSeriesSet{
		series: []storage.Series{series1, series2},
	}
	require.True(t, c.Next(), "Expected Next() to be true.")
	require.Equal(t, series1, c.At(), "Unexpected series returned.")
	require.True(t, c.Next(), "Expected Next() to be true.")
	require.Equal(t, series2, c.At(), "Unexpected series returned.")
	require.False(t, c.Next(), "Expected Next() to be false.")
}

func TestConcreteSeriesClonesLabels(t *testing.T) {
	lbls := labels.FromStrings("a", "b", "c", "d")
	cs := concreteSeries{
		labels: lbls,
	}

	gotLabels := cs.Labels()
	require.Equal(t, lbls, gotLabels)

	gotLabels.CopyFrom(labels.FromStrings("a", "foo", "c", "foo"))

	gotLabels = cs.Labels()
	require.Equal(t, lbls, gotLabels)
}

func TestConcreteSeriesIterator_FloatSamples(t *testing.T) {
	series := &concreteSeries{
		labels: labels.FromStrings("foo", "bar"),
		floats: []prompb.Sample{
			{Value: 1, Timestamp: 1},
			{Value: 1.5, Timestamp: 1},
			{Value: 2, Timestamp: 2},
			{Value: 3, Timestamp: 3},
			{Value: 4, Timestamp: 4},
		},
	}
	it := series.Iterator(nil)

	// Seek to the first sample with ts=1.
	require.Equal(t, chunkenc.ValFloat, it.Seek(1))
	ts, v := it.At()
	require.Equal(t, int64(1), ts)
	require.Equal(t, 1., v)

	// Seek one further, next sample still has ts=1.
	require.Equal(t, chunkenc.ValFloat, it.Next())
	ts, v = it.At()
	require.Equal(t, int64(1), ts)
	require.Equal(t, 1.5, v)

	// Seek again to 1 and make sure we stay where we are.
	require.Equal(t, chunkenc.ValFloat, it.Seek(1))
	ts, v = it.At()
	require.Equal(t, int64(1), ts)
	require.Equal(t, 1.5, v)

	// Another seek.
	require.Equal(t, chunkenc.ValFloat, it.Seek(3))
	ts, v = it.At()
	require.Equal(t, int64(3), ts)
	require.Equal(t, 3., v)

	// And we don't go back.
	require.Equal(t, chunkenc.ValFloat, it.Seek(2))
	ts, v = it.At()
	require.Equal(t, int64(3), ts)
	require.Equal(t, 3., v)

	// Seek beyond the end.
	require.Equal(t, chunkenc.ValNone, it.Seek(5))
	// And we don't go back. (This exposes issue #10027.)
	require.Equal(t, chunkenc.ValNone, it.Seek(2))
}

func TestConcreteSeriesIterator_HistogramSamples(t *testing.T) {
	histograms := tsdbutil.GenerateTestHistograms(5)
	histProtos := make([]prompb.Histogram, len(histograms))
	for i, h := range histograms {
		// Results in ts sequence of 1, 1, 2, 3, 4.
		var ts int64
		if i == 0 {
			ts = 1
		} else {
			ts = int64(i)
		}
		histProtos[i] = prompb.FromIntHistogram(ts, h)
	}
	series := &concreteSeries{
		labels:     labels.FromStrings("foo", "bar"),
		histograms: histProtos,
	}
	it := series.Iterator(nil)

	// Seek to the first sample with ts=1.
	require.Equal(t, chunkenc.ValHistogram, it.Seek(1))
	ts, v := it.AtHistogram(nil)
	require.Equal(t, int64(1), ts)
	require.Equal(t, histograms[0], v)

	// Seek one further, next sample still has ts=1.
	require.Equal(t, chunkenc.ValHistogram, it.Next())
	ts, v = it.AtHistogram(nil)
	require.Equal(t, int64(1), ts)
	require.Equal(t, histograms[1], v)

	// Seek again to 1 and make sure we stay where we are.
	require.Equal(t, chunkenc.ValHistogram, it.Seek(1))
	ts, v = it.AtHistogram(nil)
	require.Equal(t, int64(1), ts)
	require.Equal(t, histograms[1], v)

	// Another seek.
	require.Equal(t, chunkenc.ValHistogram, it.Seek(3))
	ts, v = it.AtHistogram(nil)
	require.Equal(t, int64(3), ts)
	require.Equal(t, histograms[3], v)

	// And we don't go back.
	require.Equal(t, chunkenc.ValHistogram, it.Seek(2))
	ts, v = it.AtHistogram(nil)
	require.Equal(t, int64(3), ts)
	require.Equal(t, histograms[3], v)

	// Seek beyond the end.
	require.Equal(t, chunkenc.ValNone, it.Seek(5))
	// And we don't go back. (This exposes issue #10027.)
	require.Equal(t, chunkenc.ValNone, it.Seek(2))
}

func TestConcreteSeriesIterator_FloatAndHistogramSamples(t *testing.T) {
	// Series starts as histograms, then transitions to floats at ts=8 (with an overlap from ts=8 to ts=10), then
	// transitions back to histograms at ts=16.
	histograms := tsdbutil.GenerateTestHistograms(15)
	histProtos := make([]prompb.Histogram, len(histograms))
	for i, h := range histograms {
		if i < 10 {
			histProtos[i] = prompb.FromIntHistogram(int64(i+1), h)
		} else {
			histProtos[i] = prompb.FromIntHistogram(int64(i+6), h)
		}
	}
	series := &concreteSeries{
		labels: labels.FromStrings("foo", "bar"),
		floats: []prompb.Sample{
			{Value: 1, Timestamp: 8},
			{Value: 2, Timestamp: 9},
			{Value: 3, Timestamp: 10},
			{Value: 4, Timestamp: 11},
			{Value: 5, Timestamp: 12},
			{Value: 6, Timestamp: 13},
			{Value: 7, Timestamp: 14},
			{Value: 8, Timestamp: 15},
		},
		histograms: histProtos,
	}
	it := series.Iterator(nil)

	var (
		ts int64
		v  float64
		h  *histogram.Histogram
		fh *histogram.FloatHistogram
	)
	require.Equal(t, chunkenc.ValHistogram, it.Next())
	ts, h = it.AtHistogram(nil)
	require.Equal(t, int64(1), ts)
	require.Equal(t, histograms[0], h)

	require.Equal(t, chunkenc.ValHistogram, it.Next())
	ts, h = it.AtHistogram(nil)
	require.Equal(t, int64(2), ts)
	require.Equal(t, histograms[1], h)

	// Seek to the first float/histogram sample overlap at ts=8 (should prefer the float sample).
	require.Equal(t, chunkenc.ValFloat, it.Seek(8))
	ts, v = it.At()
	require.Equal(t, int64(8), ts)
	require.Equal(t, 1., v)

	// Attempting to seek backwards should do nothing.
	require.Equal(t, chunkenc.ValFloat, it.Seek(1))
	ts, v = it.At()
	require.Equal(t, int64(8), ts)
	require.Equal(t, 1., v)

	// Seeking to 8 again should also do nothing.
	require.Equal(t, chunkenc.ValFloat, it.Seek(8))
	ts, v = it.At()
	require.Equal(t, int64(8), ts)
	require.Equal(t, 1., v)

	// Again, should prefer the float sample.
	require.Equal(t, chunkenc.ValFloat, it.Next())
	ts, v = it.At()
	require.Equal(t, int64(9), ts)
	require.Equal(t, 2., v)

	// Seek to ts=11 where there are only float samples.
	require.Equal(t, chunkenc.ValFloat, it.Seek(11))
	ts, v = it.At()
	require.Equal(t, int64(11), ts)
	require.Equal(t, 4., v)

	// Seek to ts=15 right before the transition back to histogram samples.
	require.Equal(t, chunkenc.ValFloat, it.Seek(15))
	ts, v = it.At()
	require.Equal(t, int64(15), ts)
	require.Equal(t, 8., v)

	require.Equal(t, chunkenc.ValHistogram, it.Next())
	ts, h = it.AtHistogram(nil)
	require.Equal(t, int64(16), ts)
	require.Equal(t, histograms[10], h)

	// Getting a float histogram from an int histogram works.
	require.Equal(t, chunkenc.ValHistogram, it.Next())
	ts, fh = it.AtFloatHistogram(nil)
	require.Equal(t, int64(17), ts)
	expected := prompb.FromIntHistogram(int64(17), histograms[11]).ToFloatHistogram()
	require.Equal(t, expected, fh)

	// Keep calling Next() until the end.
	for range 3 {
		require.Equal(t, chunkenc.ValHistogram, it.Next())
	}

	// The iterator is exhausted.
	require.Equal(t, chunkenc.ValNone, it.Next())
	require.Equal(t, chunkenc.ValNone, it.Next())
	// Should also not be able to seek backwards again.
	require.Equal(t, chunkenc.ValNone, it.Seek(1))
}

func TestConcreteSeriesIterator_InvalidHistogramSamples(t *testing.T) {
	for _, schema := range []int32{-100, 100} {
		t.Run(fmt.Sprintf("schema=%d", schema), func(t *testing.T) {
			h := prompb.FromIntHistogram(2, &testHistogram)
			h.Schema = schema
			fh := prompb.FromFloatHistogram(4, testHistogram.ToFloat(nil))
			fh.Schema = schema
			series := &concreteSeries{
				labels: labels.FromStrings("foo", "bar"),
				floats: []prompb.Sample{
					{Value: 1, Timestamp: 0},
					{Value: 2, Timestamp: 3},
				},
				histograms: []prompb.Histogram{
					h,
					fh,
				},
			}
			it := series.Iterator(nil)
			require.Equal(t, chunkenc.ValFloat, it.Next())
			require.Equal(t, chunkenc.ValNone, it.Next())
			require.Error(t, it.Err())
			require.ErrorIs(t, it.Err(), histogram.ErrHistogramsUnknownSchema)

			it = series.Iterator(it)
			require.Equal(t, chunkenc.ValFloat, it.Next())
			require.Equal(t, chunkenc.ValNone, it.Next())
			require.ErrorIs(t, it.Err(), histogram.ErrHistogramsUnknownSchema)

			it = series.Iterator(it)
			require.Equal(t, chunkenc.ValNone, it.Seek(1))
			require.ErrorIs(t, it.Err(), histogram.ErrHistogramsUnknownSchema)

			it = series.Iterator(it)
			require.Equal(t, chunkenc.ValFloat, it.Seek(3))
			require.Equal(t, chunkenc.ValNone, it.Next())
			require.ErrorIs(t, it.Err(), histogram.ErrHistogramsUnknownSchema)

			it = series.Iterator(it)
			require.Equal(t, chunkenc.ValNone, it.Seek(4))
			require.ErrorIs(t, it.Err(), histogram.ErrHistogramsUnknownSchema)
		})
	}
}

func TestConcreteSeriesIterator_ReducesHighResolutionHistograms(t *testing.T) {
	for _, schema := range []int32{9, 52} {
		t.Run(fmt.Sprintf("schema=%d", schema), func(t *testing.T) {
			h := testHistogram.Copy()
			h.Schema = schema
			fh := h.ToFloat(nil)
			series := &concreteSeries{
				labels: labels.FromStrings("foo", "bar"),
				histograms: []prompb.Histogram{
					prompb.FromIntHistogram(1, h),
					prompb.FromFloatHistogram(2, fh),
				},
			}
			it := series.Iterator(nil)
			require.Equal(t, chunkenc.ValHistogram, it.Next())
			_, gotH := it.AtHistogram(nil)
			require.Equal(t, histogram.ExponentialSchemaMax, gotH.Schema)
			_, gotFH := it.AtFloatHistogram(nil)
			require.Equal(t, histogram.ExponentialSchemaMax, gotFH.Schema)
			require.Equal(t, chunkenc.ValFloatHistogram, it.Next())
			_, gotFH = it.AtFloatHistogram(nil)
			require.Equal(t, histogram.ExponentialSchemaMax, gotFH.Schema)
			require.Equal(t, chunkenc.ValNone, it.Next())
			require.NoError(t, it.Err())
		})
	}
}

func TestFromQueryResultWithDuplicates(t *testing.T) {
	ts1 := prompb.TimeSeries{
		Labels: []prompb.Label{
			{Name: "foo", Value: "bar"},
			{Name: "foo", Value: "def"},
		},
		Samples: []prompb.Sample{
			{Value: 0.0, Timestamp: 0},
		},
	}

	res := prompb.QueryResult{
		Timeseries: []*prompb.TimeSeries{
			&ts1,
		},
	}

	series := FromQueryResult(false, &res)

	errSeries, isErrSeriesSet := series.(errSeriesSet)

	require.True(t, isErrSeriesSet, "Expected resulting series to be an errSeriesSet")
	errMessage := errSeries.Err().Error()
	require.Equalf(t, "duplicate label with name: foo", errMessage, "Expected error to be from duplicate label, but got: %s", errMessage)
}

func TestNegotiateResponseType(t *testing.T) {
	r, err := NegotiateResponseType([]prompb.ReadRequest_ResponseType{
		prompb.ReadRequest_STREAMED_XOR_CHUNKS,
		prompb.ReadRequest_SAMPLES,
	})
	require.NoError(t, err)
	require.Equal(t, prompb.ReadRequest_STREAMED_XOR_CHUNKS, r)

	r2, err := NegotiateResponseType([]prompb.ReadRequest_ResponseType{
		prompb.ReadRequest_SAMPLES,
		prompb.ReadRequest_STREAMED_XOR_CHUNKS,
	})
	require.NoError(t, err)
	require.Equal(t, prompb.ReadRequest_SAMPLES, r2)

	r3, err := NegotiateResponseType([]prompb.ReadRequest_ResponseType{})
	require.NoError(t, err)
	require.Equal(t, prompb.ReadRequest_SAMPLES, r3)

	_, err = NegotiateResponseType([]prompb.ReadRequest_ResponseType{20})
	require.Error(t, err, "expected error due to not supported requested response types")
	require.EqualError(t, err, "server does not support any of the requested response types: [20]; supported: map[SAMPLES:{} STREAMED_XOR_CHUNKS:{}]")
}

func TestMergeLabels(t *testing.T) {
	for _, tc := range []struct {
		primary, secondary, expected []prompb.Label
	}{
		{
			primary:   []prompb.Label{{Name: "aaa", Value: "foo"}, {Name: "bbb", Value: "foo"}, {Name: "ddd", Value: "foo"}},
			secondary: []prompb.Label{{Name: "bbb", Value: "bar"}, {Name: "ccc", Value: "bar"}},
			expected:  []prompb.Label{{Name: "aaa", Value: "foo"}, {Name: "bbb", Value: "foo"}, {Name: "ccc", Value: "bar"}, {Name: "ddd", Value: "foo"}},
		},
		{
			primary:   []prompb.Label{{Name: "bbb", Value: "bar"}, {Name: "ccc", Value: "bar"}},
			secondary: []prompb.Label{{Name: "aaa", Value: "foo"}, {Name: "bbb", Value: "foo"}, {Name: "ddd", Value: "foo"}},
			expected:  []prompb.Label{{Name: "aaa", Value: "foo"}, {Name: "bbb", Value: "bar"}, {Name: "ccc", Value: "bar"}, {Name: "ddd", Value: "foo"}},
		},
	} {
		require.Equal(t, tc.expected, MergeLabels(tc.primary, tc.secondary))
	}
}

func TestDecodeWriteRequest(t *testing.T) {
	buf, _, _, err := buildWriteRequest(nil, writeRequestFixture.Timeseries, nil, nil, nil, nil, "snappy")
	require.NoError(t, err)

	actual, err := DecodeWriteRequest(bytes.NewReader(buf))
	require.NoError(t, err)
	require.Equal(t, writeRequestFixture, actual)
}

func TestDecodeWriteV2Request(t *testing.T) {
	buf, _, _, err := buildV2WriteRequest(promslog.NewNopLogger(), writeV2RequestFixture.Timeseries, writeV2RequestFixture.Symbols, nil, nil, nil, "snappy")
	require.NoError(t, err)

	actual, err := DecodeWriteV2Request(bytes.NewReader(buf))
	require.NoError(t, err)
	require.Equal(t, writeV2RequestFixture, actual)
}

func TestStreamResponse(t *testing.T) {
	lbs1 := prompb.FromLabels(labels.FromStrings("instance", "localhost1", "job", "demo1"), nil)
	lbs2 := prompb.FromLabels(labels.FromStrings("instance", "localhost2", "job", "demo2"), nil)
	chunk := prompb.Chunk{
		Type: prompb.Chunk_XOR,
		Data: make([]byte, 100),
	}
	lbSize, chunkSize := 0, chunk.Size()
	for _, lb := range lbs1 {
		lbSize += lb.Size()
	}
	maxBytesInFrame := lbSize + chunkSize*2
	testData := []*prompb.ChunkedSeries{{
		Labels: lbs1,
		Chunks: []prompb.Chunk{chunk, chunk, chunk, chunk},
	}, {
		Labels: lbs2,
		Chunks: []prompb.Chunk{chunk, chunk, chunk, chunk},
	}}
	css := newMockChunkSeriesSet(testData)
	writer := mockWriter{}
	warning, err := StreamChunkedReadResponses(&writer, 0,
		css,
		nil,
		maxBytesInFrame,
		&sync.Pool{})
	require.Nil(t, warning)
	require.NoError(t, err)
	expectData := []*prompb.ChunkedSeries{{
		Labels: lbs1,
		Chunks: []prompb.Chunk{chunk, chunk},
	}, {
		Labels: lbs1,
		Chunks: []prompb.Chunk{chunk, chunk},
	}, {
		Labels: lbs2,
		Chunks: []prompb.Chunk{chunk, chunk},
	}, {
		Labels: lbs2,
		Chunks: []prompb.Chunk{chunk, chunk},
	}}
	require.Equal(t, expectData, writer.actual)
}

type mockWriter struct {
	actual []*prompb.ChunkedSeries
}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	cr := &prompb.ChunkedReadResponse{}
	if err := proto.Unmarshal(p, cr); err != nil {
		return 0, fmt.Errorf("unmarshaling: %w", err)
	}
	m.actual = append(m.actual, cr.ChunkedSeries...)
	return len(p), nil
}

type mockChunkSeriesSet struct {
	chunkedSeries []*prompb.ChunkedSeries
	index         int
	builder       labels.ScratchBuilder
}

func newMockChunkSeriesSet(ss []*prompb.ChunkedSeries) storage.ChunkSeriesSet {
	return &mockChunkSeriesSet{chunkedSeries: ss, index: -1, builder: labels.NewScratchBuilder(0)}
}

func (c *mockChunkSeriesSet) Next() bool {
	c.index++
	return c.index < len(c.chunkedSeries)
}

func (c *mockChunkSeriesSet) At() storage.ChunkSeries {
	return &storage.ChunkSeriesEntry{
		Lset: c.chunkedSeries[c.index].ToLabels(&c.builder, nil),
		ChunkIteratorFn: func(chunks.Iterator) chunks.Iterator {
			return &mockChunkIterator{
				chunks: c.chunkedSeries[c.index].Chunks,
				index:  -1,
			}
		},
	}
}

func (*mockChunkSeriesSet) Warnings() annotations.Annotations { return nil }

func (*mockChunkSeriesSet) Err() error {
	return nil
}

type mockChunkIterator struct {
	chunks []prompb.Chunk
	index  int
}

func (c *mockChunkIterator) At() chunks.Meta {
	one := c.chunks[c.index]
	chunk, err := chunkenc.FromData(chunkenc.Encoding(one.Type), one.Data)
	if err != nil {
		panic(err)
	}
	return chunks.Meta{
		Chunk:   chunk,
		MinTime: one.MinTimeMs,
		MaxTime: one.MaxTimeMs,
	}
}

func (c *mockChunkIterator) Next() bool {
	c.index++
	return c.index < len(c.chunks)
}

func (*mockChunkIterator) Err() error {
	return nil
}

func TestChunkedSeriesIterator(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		chks := buildTestChunks(t)

		it := newChunkedSeriesIterator(chks, 2000, 12000)

		require.NoError(t, it.err)
		require.NotNil(t, it.cur)

		// Initial next; advance to first valid sample of first chunk.
		res := it.Next()
		require.Equal(t, chunkenc.ValFloat, res)
		require.NoError(t, it.Err())

		ts, v := it.At()
		require.Equal(t, int64(2000), ts)
		require.Equal(t, float64(2), v)

		// Next to the second sample of the first chunk.
		res = it.Next()
		require.Equal(t, chunkenc.ValFloat, res)
		require.NoError(t, it.Err())

		ts, v = it.At()
		require.Equal(t, int64(3000), ts)
		require.Equal(t, float64(3), v)

		// Attempt to seek to the first sample of the first chunk (should return current sample).
		res = it.Seek(0)
		require.Equal(t, chunkenc.ValFloat, res)

		ts, v = it.At()
		require.Equal(t, int64(3000), ts)
		require.Equal(t, float64(3), v)

		// Seek to the end of the first chunk.
		res = it.Seek(4000)
		require.Equal(t, chunkenc.ValFloat, res)

		ts, v = it.At()
		require.Equal(t, int64(4000), ts)
		require.Equal(t, float64(4), v)

		// Next to the first sample of the second chunk.
		res = it.Next()
		require.Equal(t, chunkenc.ValFloat, res)
		require.NoError(t, it.Err())

		ts, v = it.At()
		require.Equal(t, int64(5000), ts)
		require.Equal(t, float64(1), v)

		// Seek to the second sample of the third chunk.
		res = it.Seek(10999)
		require.Equal(t, chunkenc.ValFloat, res)
		require.NoError(t, it.Err())

		ts, v = it.At()
		require.Equal(t, int64(11000), ts)
		require.Equal(t, float64(3), v)

		// Attempt to seek to something past the last sample (should return false and exhaust the iterator).
		res = it.Seek(99999)
		require.Equal(t, chunkenc.ValNone, res)
		require.NoError(t, it.Err())

		// Attempt to next past the last sample (should return false as the iterator is exhausted).
		res = it.Next()
		require.Equal(t, chunkenc.ValNone, res)
		require.NoError(t, it.Err())
	})

	t.Run("invalid chunk encoding error", func(t *testing.T) {
		chks := buildTestChunks(t)

		// Set chunk type to an invalid value.
		chks[0].Type = 8

		it := newChunkedSeriesIterator(chks, 0, 14000)

		res := it.Next()
		require.Equal(t, chunkenc.ValNone, res)

		res = it.Seek(1000)
		require.Equal(t, chunkenc.ValNone, res)

		require.ErrorContains(t, it.err, "invalid chunk encoding")
		require.Nil(t, it.cur)
	})

	t.Run("empty chunks", func(t *testing.T) {
		emptyChunks := make([]prompb.Chunk, 0)

		it1 := newChunkedSeriesIterator(emptyChunks, 0, 1000)
		require.Equal(t, chunkenc.ValNone, it1.Next())
		require.Equal(t, chunkenc.ValNone, it1.Seek(1000))
		require.NoError(t, it1.Err())

		var nilChunks []prompb.Chunk

		it2 := newChunkedSeriesIterator(nilChunks, 0, 1000)
		require.Equal(t, chunkenc.ValNone, it2.Next())
		require.Equal(t, chunkenc.ValNone, it2.Seek(1000))
		require.NoError(t, it2.Err())
	})
}

func TestChunkedSeries(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		chks := buildTestChunks(t)

		s := chunkedSeries{
			ChunkedSeries: prompb.ChunkedSeries{
				Labels: []prompb.Label{
					{Name: "foo", Value: "bar"},
					{Name: "asdf", Value: "zxcv"},
				},
				Chunks: chks,
			},
		}

		require.Equal(t, labels.FromStrings("asdf", "zxcv", "foo", "bar"), s.Labels())

		it := s.Iterator(nil)
		res := it.Next() // Behavior is undefined w/o the initial call to Next.

		require.Equal(t, chunkenc.ValFloat, res)
		require.NoError(t, it.Err())

		ts, v := it.At()
		require.Equal(t, int64(0), ts)
		require.Equal(t, float64(0), v)
	})
}

func TestChunkedSeriesSet(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		buf := &bytes.Buffer{}
		flusher := &mockFlusher{}

		w := NewChunkedWriter(buf, flusher)
		wrappedReader := newOneShotCloser(buf)
		r := NewChunkedReader(wrappedReader, config.DefaultChunkedReadLimit, nil)

		chks := buildTestChunks(t)
		l := []prompb.Label{
			{Name: "foo", Value: "bar"},
		}

		for i, c := range chks {
			cSeries := prompb.ChunkedSeries{Labels: l, Chunks: []prompb.Chunk{c}}
			readResp := prompb.ChunkedReadResponse{
				ChunkedSeries: []*prompb.ChunkedSeries{&cSeries},
				QueryIndex:    int64(i),
			}

			b, err := proto.Marshal(&readResp)
			require.NoError(t, err)

			_, err = w.Write(b)
			require.NoError(t, err)
		}

		ss := NewChunkedSeriesSet(r, wrappedReader, 0, 14000, func(error) {})
		require.NoError(t, ss.Err())
		require.Nil(t, ss.Warnings())

		res := ss.Next()
		require.True(t, res)
		require.NoError(t, ss.Err())

		s := ss.At()
		require.Equal(t, 1, s.Labels().Len())
		require.True(t, s.Labels().Has("foo"))
		require.Equal(t, "bar", s.Labels().Get("foo"))

		it := s.Iterator(nil)
		it.Next()
		ts, v := it.At()
		require.Equal(t, int64(0), ts)
		require.Equal(t, float64(0), v)

		numResponses := 1
		for ss.Next() {
			numResponses++
		}
		require.Equal(t, numTestChunks, numResponses)
		require.NoError(t, ss.Err())

		require.False(t, ss.Next(), "Next() should still return false after it previously returned false")
		require.NoError(t, ss.Err(), "Err() should not return an error if Next() is called again after it previously returned false")
	})

	t.Run("chunked reader error", func(t *testing.T) {
		buf := &bytes.Buffer{}
		flusher := &mockFlusher{}

		w := NewChunkedWriter(buf, flusher)
		r := NewChunkedReader(buf, config.DefaultChunkedReadLimit, nil)

		chks := buildTestChunks(t)
		l := []prompb.Label{
			{Name: "foo", Value: "bar"},
		}

		for i, c := range chks {
			cSeries := prompb.ChunkedSeries{Labels: l, Chunks: []prompb.Chunk{c}}
			readResp := prompb.ChunkedReadResponse{
				ChunkedSeries: []*prompb.ChunkedSeries{&cSeries},
				QueryIndex:    int64(i),
			}

			b, err := proto.Marshal(&readResp)
			require.NoError(t, err)

			b[0] = 0xFF // Corruption!

			_, err = w.Write(b)
			require.NoError(t, err)
		}

		ss := NewChunkedSeriesSet(r, io.NopCloser(buf), 0, 14000, func(error) {})
		require.NoError(t, ss.Err())
		require.Nil(t, ss.Warnings())

		res := ss.Next()
		require.False(t, res)
		require.ErrorContains(t, ss.Err(), "proto: illegal wireType 7")
	})
}

// mockFlusher implements http.Flusher.
type mockFlusher struct{}

func (*mockFlusher) Flush() {}

type oneShotCloser struct {
	r      io.Reader
	closed bool
}

func newOneShotCloser(r io.Reader) io.ReadCloser {
	return &oneShotCloser{r, false}
}

func (c *oneShotCloser) Read(p []byte) (n int, err error) {
	if c.closed {
		return 0, errors.New("already closed")
	}

	return c.r.Read(p)
}

func (c *oneShotCloser) Close() error {
	if c.closed {
		return errors.New("already closed")
	}

	c.closed = true
	return nil
}

const (
	numTestChunks          = 3
	numSamplesPerTestChunk = 5
)

func buildTestChunks(t *testing.T) []prompb.Chunk {
	startTime := int64(0)
	chks := make([]prompb.Chunk, 0, numTestChunks)

	time := startTime

	for i := range numTestChunks {
		c := chunkenc.NewXORChunk()

		a, err := c.Appender()
		require.NoError(t, err)

		minTimeMs := time

		for j := range numSamplesPerTestChunk {
			a.Append(time, float64(i+j))
			time += int64(1000)
		}

		chks = append(chks, prompb.Chunk{
			MinTimeMs: minTimeMs,
			MaxTimeMs: time,
			Type:      prompb.Chunk_XOR,
			Data:      c.Bytes(),
		})
	}

	return chks
}
