// Copyright The Prometheus Authors
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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestSampledReadEndpoint(t *testing.T) {
	store := promqltest.LoadedStorage(t, `
		load 1m
			test_metric1{foo="bar",baz="qux"} 1
	`)
	defer store.Close()

	addNativeHistogramsToTestSuite(t, store, 1)

	h := NewReadHandler(nil, nil, store, func() config.Config {
		return config.Config{
			GlobalConfig: config.GlobalConfig{
				// We expect external labels to be added, with the source labels honored.
				ExternalLabels: labels.FromStrings("b", "c", "baz", "a", "d", "e"),
			},
		}
	}, 1e6, 1, 0)

	// Encode the request.
	matcher1, err := labels.NewMatcher(labels.MatchEqual, "__name__", "test_metric1")
	require.NoError(t, err)

	matcher2, err := labels.NewMatcher(labels.MatchEqual, "d", "e")
	require.NoError(t, err)

	matcher3, err := labels.NewMatcher(labels.MatchEqual, "__name__", "test_histogram_metric1")
	require.NoError(t, err)

	matcher4, err := labels.NewMatcher(labels.MatchEqual, "__name__", "test_nhcb_metric1")
	require.NoError(t, err)

	query1, err := ToQuery(0, 1, []*labels.Matcher{matcher1, matcher2}, &storage.SelectHints{Step: 0, Func: "avg"})
	require.NoError(t, err)

	query2, err := ToQuery(0, 1, []*labels.Matcher{matcher3, matcher2}, &storage.SelectHints{Step: 0, Func: "avg"})
	require.NoError(t, err)

	query3, err := ToQuery(0, 1, []*labels.Matcher{matcher4, matcher2}, &storage.SelectHints{Step: 0, Func: "avg"})
	require.NoError(t, err)

	req := &prompb.ReadRequest{Queries: []*prompb.Query{query1, query2, query3}}
	data, err := proto.Marshal(req)
	require.NoError(t, err)

	compressed := snappy.Encode(nil, data)
	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer(compressed))
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, request)

	require.Equal(t, 2, recorder.Code/100)

	require.Equal(t, "application/x-protobuf", recorder.Result().Header.Get("Content-Type"))
	require.Equal(t, "snappy", recorder.Result().Header.Get("Content-Encoding"))

	// Decode the response.
	compressed, err = io.ReadAll(recorder.Result().Body)
	require.NoError(t, err)

	uncompressed, err := snappy.Decode(nil, compressed)
	require.NoError(t, err)

	var resp prompb.ReadResponse
	err = proto.Unmarshal(uncompressed, &resp)
	require.NoError(t, err)

	require.Len(t, resp.Results, 3, "Expected 3 results.")

	require.Equal(t, &prompb.QueryResult{
		Timeseries: []*prompb.TimeSeries{
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
		},
	}, resp.Results[0])

	require.Equal(t, &prompb.QueryResult{
		Timeseries: []*prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "test_histogram_metric1"},
					{Name: "b", Value: "c"},
					{Name: "baz", Value: "qux"},
					{Name: "d", Value: "e"},
				},
				Histograms: []prompb.Histogram{
					prompb.FromFloatHistogram(0, tsdbutil.GenerateTestFloatHistogram(0)),
				},
			},
		},
	}, resp.Results[1])

	require.Equal(t, &prompb.QueryResult{
		Timeseries: []*prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "test_nhcb_metric1"},
					{Name: "b", Value: "c"},
					{Name: "baz", Value: "qux"},
					{Name: "d", Value: "e"},
				},
				Histograms: []prompb.Histogram{{
					// We cannot use prompb.FromFloatHistogram as that's one
					// of the things we are testing here.
					Schema:    histogram.CustomBucketsSchema,
					Count:     &prompb.Histogram_CountFloat{CountFloat: 5},
					Sum:       18.4,
					ZeroCount: &prompb.Histogram_ZeroCountFloat{},
					PositiveSpans: []prompb.BucketSpan{
						{Offset: 0, Length: 2},
						{Offset: 1, Length: 2},
					},
					PositiveCounts: []float64{1, 2, 1, 1},
					CustomValues:   []float64{0, 1, 2, 3, 4},
				}},
			},
		},
	}, resp.Results[2])
}

func BenchmarkStreamReadEndpoint(b *testing.B) {
	store := promqltest.LoadedStorage(b, `
	load 1m
		test_metric1{foo="bar1",baz="qux"} 0+100x119
		test_metric1{foo="bar2",baz="qux"} 0+100x120
		test_metric1{foo="bar3",baz="qux"} 0+100x240
	`)

	b.Cleanup(func() { store.Close() })

	api := NewReadHandler(nil, nil, store, func() config.Config {
		return config.Config{}
	},
		0, 1, 0,
	)

	matcher, err := labels.NewMatcher(labels.MatchEqual, "__name__", "test_metric1")
	require.NoError(b, err)

	query, err := ToQuery(0, 14400001, []*labels.Matcher{matcher}, &storage.SelectHints{
		Step:  1,
		Func:  "sum",
		Start: 0,
		End:   14400001,
	})
	require.NoError(b, err)

	req := &prompb.ReadRequest{
		Queries:               []*prompb.Query{query},
		AcceptedResponseTypes: []prompb.ReadRequest_ResponseType{prompb.ReadRequest_STREAMED_XOR_CHUNKS},
	}
	data, err := proto.Marshal(req)
	require.NoError(b, err)

	b.ReportAllocs()

	for b.Loop() {
		compressed := snappy.Encode(nil, data)
		request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer(compressed))
		require.NoError(b, err)

		recorder := httptest.NewRecorder()
		api.ServeHTTP(recorder, request)

		require.Equal(b, 2, recorder.Code/100)

		var results []*prompb.ChunkedReadResponse
		stream := NewChunkedReader(recorder.Result().Body, config.DefaultChunkedReadLimit, nil)

		for {
			res := &prompb.ChunkedReadResponse{}
			err := stream.NextProto(res)
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(b, err)
			results = append(results, res)
		}

		require.Len(b, results, 6, "Expected 6 results.")
	}
}

func TestStreamReadEndpoint(t *testing.T) {
	// First with 120 float samples. We expect 1 frame with 1 chunk.
	// Second with 121 float samples, We expect 1 frame with 2 chunks.
	// Third with 241 float samples. We expect 1 frame with 2 chunks, and 1 frame with 1 chunk for the same series due to bytes limit.
	// Fourth with 25 histogram samples. We expect 1 frame with 1 chunk.
	store := promqltest.LoadedStorage(t, `
		load 1m
			test_metric1{foo="bar1",baz="qux"} 0+100x119
			test_metric1{foo="bar2",baz="qux"} 0+100x120
			test_metric1{foo="bar3",baz="qux"} 0+100x240
	`)
	defer store.Close()

	addNativeHistogramsToTestSuite(t, store, 25)

	api := NewReadHandler(nil, nil, store, func() config.Config {
		return config.Config{
			GlobalConfig: config.GlobalConfig{
				// We expect external labels to be added, with the source labels honored.
				ExternalLabels: labels.FromStrings("baz", "a", "b", "c", "d", "e"),
			},
		}
	},
		1e6, 1,
		// Labelset has 57 bytes. Full chunk in test data has roughly 240 bytes. This allows us to have at max 2 chunks in this test.
		57+480,
	)

	// Encode the request.
	matcher1, err := labels.NewMatcher(labels.MatchEqual, "__name__", "test_metric1")
	require.NoError(t, err)

	matcher2, err := labels.NewMatcher(labels.MatchEqual, "d", "e")
	require.NoError(t, err)

	matcher3, err := labels.NewMatcher(labels.MatchEqual, "foo", "bar1")
	require.NoError(t, err)

	matcher4, err := labels.NewMatcher(labels.MatchEqual, "__name__", "test_histogram_metric1")
	require.NoError(t, err)

	query1, err := ToQuery(0, 14400001, []*labels.Matcher{matcher1, matcher2}, &storage.SelectHints{
		Step:  1,
		Func:  "avg",
		Start: 0,
		End:   14400001,
	})
	require.NoError(t, err)

	query2, err := ToQuery(0, 14400001, []*labels.Matcher{matcher1, matcher3}, &storage.SelectHints{
		Step:  1,
		Func:  "avg",
		Start: 0,
		End:   14400001,
	})
	require.NoError(t, err)

	query3, err := ToQuery(0, 14400001, []*labels.Matcher{matcher4}, &storage.SelectHints{
		Step:  1,
		Func:  "avg",
		Start: 0,
		End:   14400001,
	})
	require.NoError(t, err)

	req := &prompb.ReadRequest{
		Queries:               []*prompb.Query{query1, query2, query3},
		AcceptedResponseTypes: []prompb.ReadRequest_ResponseType{prompb.ReadRequest_STREAMED_XOR_CHUNKS},
	}
	data, err := proto.Marshal(req)
	require.NoError(t, err)

	compressed := snappy.Encode(nil, data)
	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer(compressed))
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	api.ServeHTTP(recorder, request)

	require.Equal(t, 2, recorder.Code/100)

	require.Equal(t, "application/x-streamed-protobuf; proto=prometheus.ChunkedReadResponse", recorder.Result().Header.Get("Content-Type"))
	require.Empty(t, recorder.Result().Header.Get("Content-Encoding"))

	var results []*prompb.ChunkedReadResponse
	stream := NewChunkedReader(recorder.Result().Body, config.DefaultChunkedReadLimit, nil)
	for {
		res := &prompb.ChunkedReadResponse{}
		err := stream.NextProto(res)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		results = append(results, res)
	}

	require.Len(t, results, 6, "Expected 6 results.")

	type expectedFrame struct {
		labels     []prompb.Label
		chunkCount int
		queryIndex int64
	}
	expectedFrames := []expectedFrame{
		{
			labels: []prompb.Label{
				{Name: "__name__", Value: "test_metric1"},
				{Name: "b", Value: "c"},
				{Name: "baz", Value: "qux"},
				{Name: "d", Value: "e"},
				{Name: "foo", Value: "bar1"},
			},
			chunkCount: 1,
		},
		{
			labels: []prompb.Label{
				{Name: "__name__", Value: "test_metric1"},
				{Name: "b", Value: "c"},
				{Name: "baz", Value: "qux"},
				{Name: "d", Value: "e"},
				{Name: "foo", Value: "bar2"},
			},
			chunkCount: 2,
		},
		{
			labels: []prompb.Label{
				{Name: "__name__", Value: "test_metric1"},
				{Name: "b", Value: "c"},
				{Name: "baz", Value: "qux"},
				{Name: "d", Value: "e"},
				{Name: "foo", Value: "bar3"},
			},
			chunkCount: 2,
		},
		{
			labels: []prompb.Label{
				{Name: "__name__", Value: "test_metric1"},
				{Name: "b", Value: "c"},
				{Name: "baz", Value: "qux"},
				{Name: "d", Value: "e"},
				{Name: "foo", Value: "bar3"},
			},
			chunkCount: 1,
		},
		{
			labels: []prompb.Label{
				{Name: "__name__", Value: "test_metric1"},
				{Name: "b", Value: "c"},
				{Name: "baz", Value: "qux"},
				{Name: "d", Value: "e"},
				{Name: "foo", Value: "bar1"},
			},
			chunkCount: 1,
			queryIndex: 1,
		},
		{
			labels: []prompb.Label{
				{Name: "__name__", Value: "test_histogram_metric1"},
				{Name: "b", Value: "c"},
				{Name: "baz", Value: "qux"},
				{Name: "d", Value: "e"},
			},
			chunkCount: 1,
			queryIndex: 2,
		},
	}

	for i, exp := range expectedFrames {
		require.Len(t, results[i].ChunkedSeries, 1, "frame %d: expected 1 chunked series", i)
		series := results[i].ChunkedSeries[0]
		require.Equal(t, exp.labels, series.Labels, "frame %d: labels mismatch", i)
		require.Len(t, series.Chunks, exp.chunkCount, "frame %d: chunk count mismatch", i)
		require.Equal(t, exp.queryIndex, results[i].QueryIndex, "frame %d: query index mismatch", i)

		// Verify chunks have non-empty data and valid encoding.
		for j, chk := range series.Chunks {
			require.NotEmpty(t, chk.Data, "frame %d chunk %d: empty data", i, j)
			// Decode the chunk and verify we get the expected number of samples.
			c, err := chunkenc.FromData(chunkenc.Encoding(chk.Type), chk.Data)
			require.NoError(t, err, "frame %d chunk %d: failed to decode", i, j)
			iter := c.Iterator(nil)
			count := 0
			for iter.Next() != chunkenc.ValNone {
				count++
			}
			require.NoError(t, iter.Err(), "frame %d chunk %d: iterator error", i, j)
			require.Greater(t, count, 0, "frame %d chunk %d: no samples", i, j)
		}
	}
}

func addNativeHistogramsToTestSuite(t *testing.T, storage *teststorage.TestStorage, n int) {
	lbls := labels.FromStrings("__name__", "test_histogram_metric1", "baz", "qux")

	app := storage.Appender(t.Context())
	for i, fh := range tsdbutil.GenerateTestFloatHistograms(n) {
		_, err := app.AppendHistogram(0, lbls, int64(i)*int64(60*time.Second/time.Millisecond), nil, fh)
		require.NoError(t, err)
	}

	lbls = labels.FromStrings("__name__", "test_nhcb_metric1", "baz", "qux")
	for i, fh := range tsdbutil.GenerateTestCustomBucketsFloatHistograms(n) {
		_, err := app.AppendHistogram(0, lbls, int64(i)*int64(60*time.Second/time.Millisecond), nil, fh)
		require.NoError(t, err)
	}

	require.NoError(t, app.Commit())
}
