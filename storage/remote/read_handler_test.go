// Copyright 2021 The Prometheus Authors
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
	"context"
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
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestSampledReadEndpoint(t *testing.T) {
	store := promql.LoadedStorage(t, `
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

	query1, err := ToQuery(0, 1, []*labels.Matcher{matcher1, matcher2}, &storage.SelectHints{Step: 0, Func: "avg"})
	require.NoError(t, err)

	query2, err := ToQuery(0, 1, []*labels.Matcher{matcher3, matcher2}, &storage.SelectHints{Step: 0, Func: "avg"})
	require.NoError(t, err)

	req := &prompb.ReadRequest{Queries: []*prompb.Query{query1, query2}}
	data, err := proto.Marshal(req)
	require.NoError(t, err)

	compressed := snappy.Encode(nil, data)
	request, err := http.NewRequest("POST", "", bytes.NewBuffer(compressed))
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

	require.Equal(t, 2, len(resp.Results), "Expected 2 results.")

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
					FloatHistogramToHistogramProto(0, tsdbutil.GenerateTestFloatHistogram(0)),
				},
			},
		},
	}, resp.Results[1])
}

func BenchmarkStreamReadEndpoint(b *testing.B) {
	store := promql.LoadedStorage(b, `
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

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		compressed := snappy.Encode(nil, data)
		request, err := http.NewRequest("POST", "", bytes.NewBuffer(compressed))
		require.NoError(b, err)

		recorder := httptest.NewRecorder()
		api.ServeHTTP(recorder, request)

		require.Equal(b, 2, recorder.Code/100)

		var results []*prompb.ChunkedReadResponse
		stream := NewChunkedReader(recorder.Result().Body, DefaultChunkedReadLimit, nil)

		for {
			res := &prompb.ChunkedReadResponse{}
			err := stream.NextProto(res)
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(b, err)
			results = append(results, res)
		}

		require.Equal(b, 6, len(results), "Expected 6 results.")
	}
}

func TestStreamReadEndpoint(t *testing.T) {
	// First with 120 samples. We expect 1 frame with 1 chunk.
	// Second with 121 samples, We expect 1 frame with 2 chunks.
	// Third with 241 samples. We expect 1 frame with 2 chunks, and 1 frame with 1 chunk for the same series due to bytes limit.
	// Fourth with 120 histogram samples. We expect 1 frame with 1 chunk.
	store := promql.LoadedStorage(t, `
		load 1m
			test_metric1{foo="bar1",baz="qux"} 0+100x119
			test_metric1{foo="bar2",baz="qux"} 0+100x120
			test_metric1{foo="bar3",baz="qux"} 0+100x240
	`)
	defer store.Close()

	addNativeHistogramsToTestSuite(t, store, 120)

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
	request, err := http.NewRequest("POST", "", bytes.NewBuffer(compressed))
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	api.ServeHTTP(recorder, request)

	require.Equal(t, 2, recorder.Code/100)

	require.Equal(t, "application/x-streamed-protobuf; proto=prometheus.ChunkedReadResponse", recorder.Result().Header.Get("Content-Type"))
	require.Equal(t, "", recorder.Result().Header.Get("Content-Encoding"))

	var results []*prompb.ChunkedReadResponse
	stream := NewChunkedReader(recorder.Result().Body, DefaultChunkedReadLimit, nil)
	for {
		res := &prompb.ChunkedReadResponse{}
		err := stream.NextProto(res)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		results = append(results, res)
	}

	require.Equal(t, 6, len(results), "Expected 6 results.")

	require.Equal(t, []*prompb.ChunkedReadResponse{
		{
			ChunkedSeries: []*prompb.ChunkedSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_metric1"},
						{Name: "b", Value: "c"},
						{Name: "baz", Value: "qux"},
						{Name: "d", Value: "e"},
						{Name: "foo", Value: "bar1"},
					},
					Chunks: []prompb.Chunk{
						{
							Type:      prompb.Chunk_XOR,
							MaxTimeMs: 7140000,
							Data:      []byte("\000x\000\000\000\000\000\000\000\000\000\340\324\003\302|\005\224\000\301\254}\351z2\320O\355\264n[\007\316\224\243md\371\320\375\032Pm\nS\235\016Q\255\006P\275\250\277\312\201Z\003(3\240R\207\332\005(\017\240\322\201\332=(\023\2402\203Z\007(w\2402\201Z\017(\023\265\227\364P\033@\245\007\364\nP\033C\245\002t\036P+@e\036\364\016Pk@e\002t:P;A\245\001\364\nS\373@\245\006t\006P+C\345\002\364\006Pk@\345\036t\nP\033A\245\003\364:P\033@\245\006t\016ZJ\377\\\205\313\210\327\270\017\345+F[\310\347E)\355\024\241\366\342}(v\215(N\203)\326\207(\336\203(V\332W\362\202t4\240m\005(\377AJ\006\320\322\202t\374\240\255\003(oA\312:\3202"),
						},
					},
				},
			},
		},
		{
			ChunkedSeries: []*prompb.ChunkedSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_metric1"},
						{Name: "b", Value: "c"},
						{Name: "baz", Value: "qux"},
						{Name: "d", Value: "e"},
						{Name: "foo", Value: "bar2"},
					},
					Chunks: []prompb.Chunk{
						{
							Type:      prompb.Chunk_XOR,
							MaxTimeMs: 7140000,
							Data:      []byte("\000x\000\000\000\000\000\000\000\000\000\340\324\003\302|\005\224\000\301\254}\351z2\320O\355\264n[\007\316\224\243md\371\320\375\032Pm\nS\235\016Q\255\006P\275\250\277\312\201Z\003(3\240R\207\332\005(\017\240\322\201\332=(\023\2402\203Z\007(w\2402\201Z\017(\023\265\227\364P\033@\245\007\364\nP\033C\245\002t\036P+@e\036\364\016Pk@e\002t:P;A\245\001\364\nS\373@\245\006t\006P+C\345\002\364\006Pk@\345\036t\nP\033A\245\003\364:P\033@\245\006t\016ZJ\377\\\205\313\210\327\270\017\345+F[\310\347E)\355\024\241\366\342}(v\215(N\203)\326\207(\336\203(V\332W\362\202t4\240m\005(\377AJ\006\320\322\202t\374\240\255\003(oA\312:\3202"),
						},
						{
							Type:      prompb.Chunk_XOR,
							MinTimeMs: 7200000,
							MaxTimeMs: 7200000,
							Data:      []byte("\000\001\200\364\356\006@\307p\000\000\000\000\000\000"),
						},
					},
				},
			},
		},
		{
			ChunkedSeries: []*prompb.ChunkedSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_metric1"},
						{Name: "b", Value: "c"},
						{Name: "baz", Value: "qux"},
						{Name: "d", Value: "e"},
						{Name: "foo", Value: "bar3"},
					},
					Chunks: []prompb.Chunk{
						{
							Type:      prompb.Chunk_XOR,
							MaxTimeMs: 7140000,
							Data:      []byte("\000x\000\000\000\000\000\000\000\000\000\340\324\003\302|\005\224\000\301\254}\351z2\320O\355\264n[\007\316\224\243md\371\320\375\032Pm\nS\235\016Q\255\006P\275\250\277\312\201Z\003(3\240R\207\332\005(\017\240\322\201\332=(\023\2402\203Z\007(w\2402\201Z\017(\023\265\227\364P\033@\245\007\364\nP\033C\245\002t\036P+@e\036\364\016Pk@e\002t:P;A\245\001\364\nS\373@\245\006t\006P+C\345\002\364\006Pk@\345\036t\nP\033A\245\003\364:P\033@\245\006t\016ZJ\377\\\205\313\210\327\270\017\345+F[\310\347E)\355\024\241\366\342}(v\215(N\203)\326\207(\336\203(V\332W\362\202t4\240m\005(\377AJ\006\320\322\202t\374\240\255\003(oA\312:\3202"),
						},
						{
							Type:      prompb.Chunk_XOR,
							MinTimeMs: 7200000,
							MaxTimeMs: 14340000,
							Data:      []byte("\000x\200\364\356\006@\307p\000\000\000\000\000\340\324\003\340>\224\355\260\277\322\200\372\005(=\240R\207:\003(\025\240\362\201z\003(\365\240r\203:\005(\r\241\322\201\372\r(\r\240R\237:\007(5\2402\201z\037(\025\2402\203:\005(\375\240R\200\372\r(\035\241\322\201:\003(5\240r\326g\364\271\213\227!\253q\037\312N\340GJ\033E)\375\024\241\266\362}(N\217(V\203)\336\207(\326\203(N\334W\322\203\2644\240}\005(\373AJ\031\3202\202\264\374\240\275\003(kA\3129\320R\201\2644\240\375\264\277\322\200\332\005(3\240r\207Z\003(\027\240\362\201Z\003(\363\240R\203\332\005(\017\241\322\201\332\r(\023\2402\237Z\007(7\2402\201Z\037(\023\240\322\200\332\005(\377\240R\200\332\r "),
						},
					},
				},
			},
		},
		{
			ChunkedSeries: []*prompb.ChunkedSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_metric1"},
						{Name: "b", Value: "c"},
						{Name: "baz", Value: "qux"},
						{Name: "d", Value: "e"},
						{Name: "foo", Value: "bar3"},
					},
					Chunks: []prompb.Chunk{
						{
							Type:      prompb.Chunk_XOR,
							MinTimeMs: 14400000,
							MaxTimeMs: 14400000,
							Data:      []byte("\000\001\200\350\335\r@\327p\000\000\000\000\000\000"),
						},
					},
				},
			},
		},
		{
			ChunkedSeries: []*prompb.ChunkedSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_metric1"},
						{Name: "b", Value: "c"},
						{Name: "baz", Value: "qux"},
						{Name: "d", Value: "e"},
						{Name: "foo", Value: "bar1"},
					},
					Chunks: []prompb.Chunk{
						{
							Type:      prompb.Chunk_XOR,
							MaxTimeMs: 7140000,
							Data:      []byte("\000x\000\000\000\000\000\000\000\000\000\340\324\003\302|\005\224\000\301\254}\351z2\320O\355\264n[\007\316\224\243md\371\320\375\032Pm\nS\235\016Q\255\006P\275\250\277\312\201Z\003(3\240R\207\332\005(\017\240\322\201\332=(\023\2402\203Z\007(w\2402\201Z\017(\023\265\227\364P\033@\245\007\364\nP\033C\245\002t\036P+@e\036\364\016Pk@e\002t:P;A\245\001\364\nS\373@\245\006t\006P+C\345\002\364\006Pk@\345\036t\nP\033A\245\003\364:P\033@\245\006t\016ZJ\377\\\205\313\210\327\270\017\345+F[\310\347E)\355\024\241\366\342}(v\215(N\203)\326\207(\336\203(V\332W\362\202t4\240m\005(\377AJ\006\320\322\202t\374\240\255\003(oA\312:\3202"),
						},
					},
				},
			},
			QueryIndex: 1,
		},
		{
			ChunkedSeries: []*prompb.ChunkedSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "test_histogram_metric1"},
						{Name: "b", Value: "c"},
						{Name: "baz", Value: "qux"},
						{Name: "d", Value: "e"},
					},
					Chunks: []prompb.Chunk{
						{
							Type:      prompb.Chunk_FLOAT_HISTOGRAM,
							MaxTimeMs: 7140000,
							Data:      []byte("\000x\000\377?PbM\322\361\251\374\214\244\224e$\242@(\000\000\000\000\000\000@\000\000\000\000\000\000\000@2ffffff?\360\000\000\000\000\000\000@\000\000\000\000\000\000\000?\360\000\000\000\000\000\000?\360\000\000\000\000\000\000?\360\000\000\000\000\000\000@\000\000\000\000\000\000\000?\360\000\000\000\000\000\000?\360\000\000\000\000\000\000\370\352`\326/v\003\322\037\302_\377\330\017\t\177\377\t\177\377\t\177\377`<%\377\374%\377\364\275a{4\237\377\377\377\377\377\375\200\365\205\354\007\260\036\300z\302\366\003\330\r\244\217\275\240\365\353\237\377\377\377\377\377\375a{A\353\013\326\027\254/h=az\302\320\276\300\270\254\314\314\314\314\314\333A\354\013\332\017h=\240\366\005\355\007\264\032\t\231\22333333;\002\347`^\300\275\201s\260/`Z8\324'\352\037\274\352\023\346gP\2372\334$\356\007\273)\377\377\377\377\377\377P\237\270\036\241?P\237\250O\334\017P\237\250Om\027\373B\366\347\265UUUUUw\003\332\027\270\036\340{\201\355\013\334\017p4\231\235\231\231\231\231\231\236\320\271\332\027\264/h\\\355\013\332\026\3307{\003\3732\344\314\314\314\314\314\347`~fv\007\346Q1\335\225\252\252\252\252\252\253\260?\035\201\375\201\375\201\370\354\017\354\017\245\347\267<\377\377\377\377\377\377\031\3061\234b\324O\306:\365\357\377\377\377\377\377\374\3439\3163\232\005\353\023\340\254\314\314\314\314\314\307X\237\030\307X\237\030\240\316\360p\311\231\231\231\231\231\265\211\373\301\353\023\365\211\372\304\375\340\365\211\372\304\364\017\334\027\007\252\252\252\252\252\253x=\301{\301\357\007\274\036\340\275\340\367\203C\231\216\177\377\377\377\377\377\270.w\005\356\013\334\027;\202\367\005\240^\320\374\026\252\252\252\252\252\251\332\037\231\235\241\371\224\031\214-\231\231\231\231\231\235\241\370\355\017\355\017\355\017\307h\177h}\035\347<\231\231\231\231\231\2323\214c8\305\002c\005\252\252\252\252\252\252q\234\347\031\315\006\366\t\360\317\377\377\377\377\377\343\260O\214c\260O\214P&\030=UUUUU[\004\370v\t\373\004\375\202|;\004\375\202z\037\307\034\231\231\231\231\231\232\030\341\206\030\341\205\002a\202\26333333\0341\307\0341\306\203s\302\352\252\252\252\252\252\206xa\206xa@\230tq\376*\252\252\252\252\253<3\317<3\316\237q\300\026\252\252\252\252\252\260\307\0140\307\014-\342\346\030\007\263333328c\2168c\215/H\377\340\t333338t\217\376\030a\322?\370an\033\236\000\340;UUUUU]#\377\340\017H\377\364\217\377H\377\370\003\322?\375#\377G\357\013\200'\377\377\377\377\377\370\003\336\027\300\037\000|\001\357\013\340\017\2004\314\300.\252\252\252\252\252\273\302\347x^\360\275\341s\274/xZ/p~\007\254\314\314\314\314\314\235\301\371\231\334\037\231m\037\314`\t33333=\301\370\356\017\356\017\356\017\307p\177p}\027\234\003\352\252\252\252\252\253\031\3061\234b\204\306\000\237\377\377\377\377\377\234g9\306sN\366\211\360\035\252\252\252\252\252\254v\211\361\214v\211\361\212\023\014\002\331\231\231\231\231\231\264O\207h\237\264O\332'\303\264O\332'\243\361\300\022fffffp\307\0140\307\014(L0}\252\252\252\252\252\254p\307\034p\307\033a\037s\300\023\377\377\377\377\377\360\317\0140\317\014(&\030\007\325UUUUVxg\236xg\235\013\307\000I\231\231\231\231\231\303\0340\303\0340\243\230`:\314\314\314\314\314\310\341\2168\341\2164\027\260_\300.\252\252\252\252\252\260\354\027\360\303\016\301\177\014(f\014\001?\377\377\377\377\377\260_\301\330/\366\013\375\202\376\016\301\177\260_\240\370p=\252\252\252\252\252\254\030p`\301\207\006\ny\203\000I\231\231\231\231\231\303\203\016\03480\341\240\270\360\017fffffd\030\360`\301\217\006\n\031\203\000Z\252\252\252\252\252\307\203\036<x1\343\243xp\034\377\377\377\377\377\374\030p`\301\207\006\n\t\203\000\272\252\252\252\252\252\303\203\016\03480\341\241\271\360\004\231\231\231\231\231\234\031\360`\301\237\006\n\t\203\017\30333333O\203>|\3703\347\264\031\3770\340\007\252\252\252\252\252\2500\340\301\203\016\014\027\021&\014\001\237\377\377\377\377\377\016\0148p\340\303\206\340.\343\300\013UUUUUPc\301\203\006<\030)0`\033fffffh\360c\307\217\006<v\361\267\207\000d\314\314\314\314\314\301\207\006\014\030p`\246`\300\013UUUUUP\340\303\207\016\0148h\275c\177\001\347\377\377\377\377\377\301\3267\360`\301\3267\360`\267\017\347\2108\001\352\252\252\252\252\253X\337\361\007\254o\365\215\376\261\277\342\017X\337\353\033\364?\200\\\001\254\314\314\314\314\315\304\037\000\276 \370\203\342\017\200_\020|A\2433\000$\314\314\314\314\314\360\013\236\001|\002\370\005\317\000\276\001h^\360\374\003uUUUUU;\303\3633\274?2\234\306\000\317\377\377\377\377\377\336\037\216\360\376\360\376\360\374w\207\367\207\321y\300\013UUUUUQ\234c\031\306(L`\177fffffi\306s\234g6\322>\367\t\360\002L\314\314\314\314\314w\t\361\214w\t\361\212\t\206\000\332\252\252\252\252\252\334'\303\270O\334'\356\023\341\334'\356\023\320\374p\002\177\377\377\377\377\3741\303\0141\303\n\t\206\001\272\252\252\252\252\252\216\030\343\216\030\343Gs\300\032\314\314\314\314\314\320\317\0140\317\014(&\030\001&fffffxg\236xg\235\013\307\001\375UUUUUC\0340\303\0340\247\230`\004\377\377\377\377\377\370\341\2168\341\2164\027\264_\300\033UUUUUP\355\027\360\303\016\321\177\014(f\014\003l\314\314\314\314\315\264_\301\332/\366\213\375\242\376\016\321\177\264_\240\370p\002L\314\314\314\314\314\030p`\301\207\006\n9\203\000mUUUUUC\203\016\03480\341\240\270\360\002\177\377\377\377\377\374\030\360`\301\217\006\n\031\203\007\375UUUUUG\203\036<x1\343\266\023\367\207\000$\314\314\314\314\314\301\207\006\014\030p`\240L\030\003Y\231\231\231\231\232\034\030p\341\301\207\r\006\347\3007UUUUUPg\301\203\006|\030(\023\006\000O\377\377\377\377\377\237\006|\371\360g\317C\370p\006\325UUUUT\030p`\301\207\006\n\004\301\200\022fffffa\301\207\016\034\030p\320n<\007\354\314\314\314\314\315\006<\0300c\301\202\2010`\005\252\252\252\252\252\250\360c\307\217\006<tw\207\000g\377\377\377\377\377\301\207\006\014\030p`\240\314\030\006\352\252\252\252\252\252\034\030p\341\301\207\r\002\366\r\374\000\22333333\007`\337\301\203\007`\337\301\202\20700\006\26333336\301\277\201\3307\373\006\377`\337\300\354\033\375\203~\201\360p\003\325UUUUT\014\034\014\014\014\034\014\n\014\300\300\371\377\377\377\377\377\360p0ppp0ph\027\017\000-UUUUU@\303\300\300\300\303\300\300\247\314\014\001\22333333\017\003\017\017\017\003\017\016\202\360p\r\26333334\014\034\014\014\014\034\014\n\004\300\300\013UUUUUPp0ppp0phw\037\000g\377\377\377\377\377\300\307\300\300\300\307\300\300\240L\014\000\365UUUUU\037\003\037\037\037\003\037\036\203\360p\036L\314\314\314\314\314\014\034\014\014\014\034\014\n\004\300\300\n\314\314\314\314\314\320p0ppp0ph\367\017\000]UUUUU@\303\300\300\300\303\300\300\240L\014\003\237\377\377\377\377\377\017\003\017\017\017\003\017\016\202\360p\002\325UUUUT\014\034\014\014\014\034\014\n\034\300\300\031333330p0ppp0ph\027?\000;33333@\317\300\300\300\317\300\300\240\314\014\177\340\000\000\000\000\001?\003???\003?>\201\360p\001?\377\377\377\377\374\014\034\014\014\014\034\014\013Y\177\366\006\000]UUUUU\203\201\203\203\203\201\203\203@.\036\000\35333333\201\207\201\201\201\207\201\201@f\006\000$\314\314\314\314\314\207\201\207\207\207\201\207\207@\336\016\000}UUUUU\201\203\201\201\201\203\201\201@&\006\000'\377\377\377\377\377\203\201\203\203\203\201\203\203@n>\001\355UUUUU\201\217\201\201\201\217\201\201@&\006\000[33333\217\201\217\217\217\201\217\217"),
						},
					},
				},
			},
			QueryIndex: 2,
		},
	}, results)
}

func addNativeHistogramsToTestSuite(t *testing.T, storage *teststorage.TestStorage, n int) {
	lbls := labels.FromStrings("__name__", "test_histogram_metric1", "baz", "qux")

	app := storage.Appender(context.TODO())
	for i, fh := range tsdbutil.GenerateTestFloatHistograms(n) {
		_, err := app.AppendHistogram(0, lbls, int64(i)*int64(60*time.Second/time.Millisecond), nil, fh)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
}
