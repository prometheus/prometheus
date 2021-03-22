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
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

var writeRequestFixture = &prompb.WriteRequest{
	Timeseries: []prompb.TimeSeries{
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "test_metric1"},
				{Name: "b", Value: "c"},
				{Name: "baz", Value: "qux"},
				{Name: "d", Value: "e"},
				{Name: "foo", Value: "bar"},
			},
			Samples: []prompb.Sample{{Value: 1, Timestamp: 0}},
		},
		{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "test_metric1"},
				{Name: "b", Value: "c"},
				{Name: "baz", Value: "qux"},
				{Name: "d", Value: "e"},
				{Name: "foo", Value: "bar"},
			},
			Samples: []prompb.Sample{{Value: 2, Timestamp: 1}},
		},
	},
}

func TestValidateLabelsAndMetricName(t *testing.T) {
	tests := []struct {
		input       labels.Labels
		expectedErr string
		description string
	}{
		{
			input: labels.FromStrings(
				"__name__", "name",
				"labelName", "labelValue",
			),
			expectedErr: "",
			description: "regular labels",
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"_labelName", "labelValue",
			),
			expectedErr: "",
			description: "label name with _",
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"@labelName", "labelValue",
			),
			expectedErr: "invalid label name: @labelName",
			description: "label name with @",
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"123labelName", "labelValue",
			),
			expectedErr: "invalid label name: 123labelName",
			description: "label name starts with numbers",
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"", "labelValue",
			),
			expectedErr: "invalid label name: ",
			description: "label name is empty string",
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"labelName", string([]byte{0xff}),
			),
			expectedErr: "invalid label value: " + string([]byte{0xff}),
			description: "label value is an invalid UTF-8 value",
		},
		{
			input: labels.FromStrings(
				"__name__", "@invalid_name",
			),
			expectedErr: "invalid metric name: @invalid_name",
			description: "metric name starts with @",
		},
		{
			input: labels.FromStrings(
				"__name__", "name1",
				"__name__", "name2",
			),
			expectedErr: "duplicate label with name: __name__",
			description: "duplicate label names",
		},
		{
			input: labels.FromStrings(
				"label1", "name",
				"label2", "name",
			),
			expectedErr: "",
			description: "duplicate label values",
		},
		{
			input: labels.FromStrings(
				"", "name",
				"label2", "name",
			),
			expectedErr: "invalid label name: ",
			description: "don't report as duplicate label name",
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			err := validateLabelsAndMetricName(test.input)
			if test.expectedErr != "" {
				require.Error(t, err)
				require.Equal(t, test.expectedErr, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConcreteSeriesSet(t *testing.T) {
	series1 := &concreteSeries{
		labels:  labels.FromStrings("foo", "bar"),
		samples: []prompb.Sample{{Value: 1, Timestamp: 2}},
	}
	series2 := &concreteSeries{
		labels:  labels.FromStrings("foo", "baz"),
		samples: []prompb.Sample{{Value: 3, Timestamp: 4}},
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
	lbls := labels.Labels{
		labels.Label{Name: "a", Value: "b"},
		labels.Label{Name: "c", Value: "d"},
	}
	cs := concreteSeries{
		labels: labels.New(lbls...),
	}

	gotLabels := cs.Labels()
	require.Equal(t, lbls, gotLabels)

	gotLabels[0].Value = "foo"
	gotLabels[1].Value = "bar"

	gotLabels = cs.Labels()
	require.Equal(t, lbls, gotLabels)
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
	require.Equal(t, "duplicate label with name: foo", errMessage, fmt.Sprintf("Expected error to be from duplicate label, but got: %s", errMessage))
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
	require.Equal(t, "server does not support any of the requested response types: [20]; supported: map[SAMPLES:{} STREAMED_XOR_CHUNKS:{}]", err.Error())
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

func TestMetricTypeToMetricTypeProto(t *testing.T) {
	tc := []struct {
		desc     string
		input    textparse.MetricType
		expected prompb.MetricMetadata_MetricType
	}{
		{
			desc:     "with a single-word metric",
			input:    textparse.MetricTypeCounter,
			expected: prompb.MetricMetadata_COUNTER,
		},
		{
			desc:     "with a two-word metric",
			input:    textparse.MetricTypeStateset,
			expected: prompb.MetricMetadata_STATESET,
		},
		{
			desc:     "with an unknown metric",
			input:    "not-known",
			expected: prompb.MetricMetadata_UNKNOWN,
		},
	}

	for _, tt := range tc {
		t.Run(tt.desc, func(t *testing.T) {
			m := metricTypeToMetricTypeProto(tt.input)
			require.Equal(t, tt.expected, m)
		})
	}
}

type TestSample struct {
	t int64
	v float64
}

func (s TestSample) T() int64 {
	return s.t
}

func (s TestSample) V() float64 {
	return s.v
}

func TestChunkSeriesSet(t *testing.T) {
	b := &bytes.Buffer{}
	f := &mockedFlusher{}
	w := NewChunkedWriter(b, f)

	chk1 := tsdbutil.ChunkFromSamples([]tsdbutil.Sample{TestSample{1, 10}, TestSample{2, 20}})
	chk2 := tsdbutil.ChunkFromSamples([]tsdbutil.Sample{TestSample{5, 50}, TestSample{8, 55}})
	chk3 := tsdbutil.ChunkFromSamples([]tsdbutil.Sample{TestSample{1, 100}, TestSample{2, 100}})
	chk4 := tsdbutil.ChunkFromSamples([]tsdbutil.Sample{TestSample{12, 21}, TestSample{14, 41}, TestSample{16, 61}})
	chk5 := tsdbutil.ChunkFromSamples([]tsdbutil.Sample{TestSample{10, 100}})

	chunkedSeries1, _ :=
		proto.Marshal(&prompb.ChunkedReadResponse{
			ChunkedSeries: []*prompb.ChunkedSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_metric1"},
						{Name: "__name__", Value: "test_metric2"},
					},
					Chunks: []prompb.Chunk{
						{
							Data:      chk1.Chunk.Bytes(),
							MinTimeMs: chk1.MinTime,
							MaxTimeMs: chk1.MaxTime,
							Type:      prompb.Chunk_XOR,
						},
						{
							Data:      chk2.Chunk.Bytes(),
							MinTimeMs: chk2.MinTime,
							MaxTimeMs: chk2.MaxTime,
							Type:      prompb.Chunk_XOR,
						},
					},
				},
			},
		})

	chunkedSeries2, _ := proto.Marshal(&prompb.ChunkedReadResponse{
		ChunkedSeries: []*prompb.ChunkedSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "test_metric3"},
				},
				Chunks: []prompb.Chunk{
					{
						Data:      chk3.Chunk.Bytes(),
						MinTimeMs: chk3.MinTime,
						MaxTimeMs: chk3.MaxTime,
						Type:      prompb.Chunk_XOR,
					},
				},
			},
		},
	})

	chunkedSeries3, _ := proto.Marshal(&prompb.ChunkedReadResponse{
		ChunkedSeries: []*prompb.ChunkedSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "test_metric4"},
					{Name: "__name__", Value: "test_metric5"},
				},
				Chunks: []prompb.Chunk{
					{
						Data:      chk4.Chunk.Bytes(),
						MinTimeMs: chk4.MinTime,
						MaxTimeMs: chk4.MaxTime,
						Type:      prompb.Chunk_XOR,
					},
					{
						Data:      chk5.Chunk.Bytes(),
						MinTimeMs: chk5.MinTime,
						MaxTimeMs: chk5.MaxTime,
						Type:      prompb.Chunk_XOR,
					},
				},
			},
		},
	})

	msgs := [][]byte{chunkedSeries1, chunkedSeries2, chunkedSeries3}
	for _, msg := range msgs {
		_, err := w.Write(msg)
		require.NoError(t, err)
	}
	r := NewChunkedReader(b, 5e+7, nil)
	mockHTTPResponse := http.Response{
		Body: ioutil.NopCloser(bytes.NewBufferString("")),
	}
	ss := &chunkedResponseSeriesSet{stream: r, httpResp: &mockHTTPResponse}
	require.Equal(t, true, ss.Next())
	require.Equal(t, labels.Labels{labels.Label{Name: "__name__", Value: "test_metric1"}, labels.Label{Name: "__name__", Value: "test_metric2"}}, ss.At().Labels())
	chk := ss.At().Iterator()
	require.Equal(t, true, chk.Next())
	time, value := chk.At()
	require.Equal(t, int64(1), time)
	require.Equal(t, 10.0, value)
	require.Equal(t, true, chk.Next())
	time, value = chk.At()
	require.Equal(t, int64(2), time)
	require.Equal(t, 20.0, value)
	require.Equal(t, true, chk.Next())

	time, value = chk.At()
	require.Equal(t, int64(5), time)
	require.Equal(t, 50.0, value)
	require.Equal(t, true, chk.Next())
	require.Equal(t, false, chk.Next())

	require.Equal(t, true, ss.Next())
	chk = ss.At().Iterator()
	require.Equal(t, labels.Labels{labels.Label{Name: "__name__", Value: "test_metric3"}}, ss.At().Labels())

	require.Equal(t, true, chk.Next())
	time, value = chk.At()
	require.Equal(t, int64(1), time)
	require.Equal(t, 100.0, value)
	require.Equal(t, true, chk.Next())
	require.Equal(t, false, chk.Next())

	require.Equal(t, true, ss.Next())
	chk = ss.At().Iterator()
	require.Equal(t, labels.Labels{labels.Label{Name: "__name__", Value: "test_metric4"}, labels.Label{Name: "__name__", Value: "test_metric5"}}, ss.At().Labels())

	require.Equal(t, true, chk.Next())
	require.Equal(t, true, chk.Seek(int64(14)))
	time, value = chk.At()
	require.Equal(t, int64(14), time)
	require.Equal(t, 41.0, value)
	require.Equal(t, true, chk.Seek(int64(15)))
	time, value = chk.At()
	require.Equal(t, int64(16), time)
	require.Equal(t, 61.0, value)
	require.Equal(t, labels.Labels{labels.Label{Name: "__name__", Value: "test_metric4"}, labels.Label{Name: "__name__", Value: "test_metric5"}}, ss.At().Labels())
	require.Equal(t, true, chk.Next())
	require.Equal(t, true, chk.Seek(int64(5)))
	time, value = chk.At()
	require.Equal(t, int64(10), time)
	require.Equal(t, 100.0, value)
	require.Equal(t, false, chk.Next())
	require.Equal(t, false, ss.Next())
	require.Equal(t, labels.Labels{}, ss.At().Labels())
}

func TestDecodeWriteRequest(t *testing.T) {
	buf, _, err := buildWriteRequest(writeRequestFixture.Timeseries, nil, nil)
	require.NoError(t, err)

	actual, err := DecodeWriteRequest(bytes.NewReader(buf))
	require.NoError(t, err)
	require.Equal(t, writeRequestFixture, actual)
}
