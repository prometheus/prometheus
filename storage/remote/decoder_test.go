// Copyright 2018 The Prometheus Authors
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
	"io"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateLabelsAndMetricName(t *testing.T) {
	tests := []struct {
		input       labels.Labels
		expectedErr string
		shouldPass  bool
	}{
		{
			input: labels.FromStrings(
				"__name__", "name",
				"labelName", "labelValue",
			),
			expectedErr: "",
			shouldPass:  true,
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"_labelName", "labelValue",
			),
			expectedErr: "",
			shouldPass:  true,
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"@labelName", "labelValue",
			),
			expectedErr: "Invalid label name: @labelName",
			shouldPass:  false,
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"123labelName", "labelValue",
			),
			expectedErr: "Invalid label name: 123labelName",
			shouldPass:  false,
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"", "labelValue",
			),
			expectedErr: "Invalid label name: ",
			shouldPass:  false,
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"labelName", string([]byte{0xff}),
			),
			expectedErr: "Invalid label value: " + string([]byte{0xff}),
			shouldPass:  false,
		},
		{
			input: labels.FromStrings(
				"__name__", "@invalid_name",
			),
			expectedErr: "Invalid metric name: @invalid_name",
			shouldPass:  false,
		},
	}

	for _, test := range tests {
		err := validateLabelsAndMetricName(test.input)
		if test.shouldPass != (err == nil) {
			if test.shouldPass {
				t.Fatalf("Test should pass, got unexpected error: %v", err)
			} else {
				t.Fatalf("Test should fail, unexpected error, got: %v, expected: %v", err, test.expectedErr)
			}
		}
	}
}

func TestConcreteSeriesSet(t *testing.T) {
	series1 := &concreteSeries{
		labels:  labels.FromStrings("foo", "bar"),
		samples: []*prompb.Sample{&prompb.Sample{Value: 1, Timestamp: 2}},
	}
	series2 := &concreteSeries{
		labels:  labels.FromStrings("foo", "baz"),
		samples: []*prompb.Sample{&prompb.Sample{Value: 3, Timestamp: 4}},
	}
	c := &concreteSeriesSet{
		series: []storage.Series{series1, series2},
	}
	if !c.Next() {
		t.Fatalf("Expected Next() to be true.")
	}
	if c.At() != series1 {
		t.Fatalf("Unexpected series returned.")
	}
	if !c.Next() {
		t.Fatalf("Expected Next() to be true.")
	}
	if c.At() != series2 {
		t.Fatalf("Unexpected series returned.")
	}
	if c.Next() {
		t.Fatalf("Expected Next() to be false.")
	}
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

func TestDecodeResponse(t *testing.T) {
	for _, format := range []prompb.ReadRequest_Response{prompb.ReadRequest_RAW, prompb.ReadRequest_RAW_STREAMED} {
		t.Run(format.String(), func(t *testing.T) {
			b := bytes.Buffer{}

			enc, err := NewEncoder(&b, &prompb.ReadRequest{
				Queries:      make([]*prompb.Query, 2),
				ResponseType: format,
			}, 1e6)
			require.NoError(t, err)

			h := http.Header{}
			for k, v := range enc.ContentHeaders() {
				h.Set(k, v)
			}

			ts1 := []*prompb.TimeSeries{
				{Labels: labelsToLabelsProto(labels.FromStrings("foo", "bar", "a", "b")), Samples: []*prompb.Sample{}},
				{Labels: labelsToLabelsProto(labels.FromStrings("foo", "bar", "a", "b2")), Samples: []*prompb.Sample{}},
			}
			ts2 := []*prompb.TimeSeries{
				{Labels: labelsToLabelsProto(labels.FromStrings("foo", "bar2", "a", "b")), Samples: []*prompb.Sample{}},
				{Labels: labelsToLabelsProto(labels.FromStrings("foo", "bar2", "a", "b2")), Samples: []*prompb.Sample{}},
			}
			require.NoError(t, enc.Encode(ts1, 0))
			require.NoError(t, enc.Encode(ts2, 1))
			require.NoError(t, enc.Close())

			querySet := DecodeReadResponse(h, ioutil.NopCloser(&b))

			assert.True(t, querySet.Next())
			require.NoError(t, querySet.Err())
			res1, err := ToTimeSeries(querySet.At())
			require.NoError(t, err)
			if !proto.Equal(
				&prompb.QueryResult{Timeseries: res1},
				&prompb.QueryResult{Timeseries: ts1},
			) {
				t.Fatalf("unexpected labels for first query; want %v, got %v", ts1, res1)
			}

			assert.False(t, querySet.At().Next())
			require.NoError(t, querySet.Err())

			assert.True(t, querySet.Next())
			require.NoError(t, querySet.Err())
			res2, err := ToTimeSeries(querySet.At())
			require.NoError(t, err)
			if !proto.Equal(
				&prompb.QueryResult{Timeseries: res2},
				&prompb.QueryResult{Timeseries: ts2},
			) {
				t.Fatalf("unexpected labels for first query; want %v, got %v", ts2, res2)
			}

			assert.False(t, querySet.At().Next())
			require.NoError(t, querySet.Err())

			assert.False(t, querySet.Next())
			require.NoError(t, querySet.Err())
		})
	}
}

type fakeReadCloser struct {
	io.Reader
	closed bool
}

func (c *fakeReadCloser) Close() error {
	c.closed = true
	return nil
}

func TestDecodeResponseStreamedFrames(t *testing.T) {
	b := bytes.Buffer{}

	enc, err := NewEncoder(&b, &prompb.ReadRequest{
		Queries:      make([]*prompb.Query, 2),
		ResponseType: prompb.ReadRequest_RAW_STREAMED,
	}, 1e6)
	require.NoError(t, err)

	h := http.Header{}
	for k, v := range enc.ContentHeaders() {
		h.Set(k, v)
	}

	f := fakeReadCloser{Reader: &b}
	querySet := DecodeReadResponse(h, &f)

	ts1 := []*prompb.TimeSeries{
		// Query 1
		{Labels: labelsToLabelsProto(labels.FromStrings("foo", "bar", "a", "b")), Samples: []*prompb.Sample{}},
		{Labels: labelsToLabelsProto(labels.FromStrings("foo", "bar", "a", "b2")), Samples: []*prompb.Sample{}},

		// Query 2
		{Labels: labelsToLabelsProto(labels.FromStrings("foo", "bar2", "a", "b")), Samples: []*prompb.Sample{}},
		{Labels: labelsToLabelsProto(labels.FromStrings("foo", "bar2", "a", "b2")), Samples: []*prompb.Sample{}},
	}

	var ss storage.SeriesSet
	// Iterates over 2 queries, send and access individual series separately.
	for i := 0; i < 2; i++ {
		require.NoError(t, enc.Encode([]*prompb.TimeSeries{ts1[2*i]}, i))

		if ss != nil {
			assert.False(t, ss.Next())
			require.NoError(t, ss.Err())
		}

		assert.True(t, querySet.Next())
		require.NoError(t, querySet.Err())

		ss = querySet.At()

		assert.True(t, ss.Next())
		require.NoError(t, ss.Err())

		got, err := toSingleTimeSeries(ss.At())
		require.NoError(t, err)
		if !proto.Equal(got, ts1[i*2]) {
			t.Fatalf("unexpected labels for first query; want %v, got %v", ts1[i*2], got)
		}

		require.NoError(t, enc.Encode([]*prompb.TimeSeries{ts1[i*2+1]}, i))

		assert.True(t, ss.Next())
		require.NoError(t, ss.Err())

		got, err = toSingleTimeSeries(ss.At())
		require.NoError(t, err)
		if !proto.Equal(got, ts1[i*2+1]) {
			t.Fatalf("unexpected labels for first query; want %v, got %v", ts1[i*2+1], got)
		}

		assert.False(t, f.closed)
	}

	assert.False(t, ss.Next())
	require.NoError(t, ss.Err())

	// End of stream, we should close.
	assert.True(t, f.closed)

	assert.False(t, querySet.Next())
	require.NoError(t, querySet.Err())

	require.NoError(t, enc.Close())
}
