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
)

func TestSampledReadEndpoint(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			test_metric1{foo="bar",baz="qux"} 1
	`)
	require.NoError(t, err)
	defer suite.Close()

	addNativeHistogramsToTestSuite(t, suite, 1)

	err = suite.Run()
	require.NoError(t, err)

	h := NewReadHandler(nil, nil, suite.Storage(), func() config.Config {
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
	suite, err := promql.NewTest(b, `
	load 1m
		test_metric1{foo="bar1",baz="qux"} 0+100x119
		test_metric1{foo="bar2",baz="qux"} 0+100x120
		test_metric1{foo="bar3",baz="qux"} 0+100x240
	`)
	require.NoError(b, err)

	defer suite.Close()

	require.NoError(b, suite.Run())

	api := NewReadHandler(nil, nil, suite.Storage(), func() config.Config {
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
	// Fourth with 30 histogram samples. We expect 1 frame with 1 chunk.
	suite, err := promql.NewTest(t, `
		load 1m
			test_metric1{foo="bar1",baz="qux"} 0+100x119
			test_metric1{foo="bar2",baz="qux"} 0+100x120
			test_metric1{foo="bar3",baz="qux"} 0+100x240
	`)
	require.NoError(t, err)
	defer suite.Close()

	addNativeHistogramsToTestSuite(t, suite, 30)

	require.NoError(t, suite.Run())

	api := NewReadHandler(nil, nil, suite.Storage(), func() config.Config {
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
							MaxTimeMs: 1740000,
							Data:      []byte("\x00\x1e\x00\xff?PbM\xd2\xf1\xa9\xfc\x8c\xa4\x94e$\xa2@$\x00\x00\x00\x00\x00\x00@\x00\x00\x00\x00\x00\x00\x00@2ffffff?\xf0\x00\x00\x00\x00\x00\x00@\x00\x00\x00\x00\x00\x00\x00?\xf0\x00\x00\x00\x00\x00\x00?\xf0\x00\x00\x00\x00\x00\x00?\xf0\x00\x00\x00\x00\x00\x00@\x00\x00\x00\x00\x00\x00\x00?\xf0\x00\x00\x00\x00\x00\x00?\xf0\x00\x00\x00\x00\x00\x00\xf8\xea`\xd6%\xec\a\xa4?\x84\xbf\xff\xb0\x1e\x12\xff\xfe\x12\xff\xfe\x12\xff\xfe\xc0xK\xff\xf8K\xff\xe95\x85\xec\xd2\x7f\xff\xff\xff\xff\xff\xf6\x03\xd6\x17\xb0\x1e\xc0{\x01\xeb\v\xd8\x0f`6\x91\xfd\xed\a\xaf\\\xff\xff\xff\xff\xff\xff\xeb\v\xda\x0fX^\xb0\xbda{A\xeb\v\xd6\x16\x82l\v\x8a\xcc\xcc\xcc\xcc\xccʹ\x1e\xc0\xbd\xa0\xf6\x83\xda\x0f`^\xd0{A\xa1\x932fffffg`\\\xec\v\xd8\x17\xb0.v\x05\xec\vA5\t\xfa\x87\xef:\x84\xf9\x99\xd4'̵\x8d\xde\xe0{\xb2\x9f\xff\xff\xff\xff\xff\xf5\t\xfb\x81\xea\x13\xf5\t\xfa\x84\xfd\xc0\xf5\t\xfa\x84\xf4&н\xb9\xedUUUUU]\xc0\xf6\x85\xee\a\xb8\x1e\xe0{B\xf7\x03\xdc\r\x193\xb333333\xda\x17;B\xf6\x85\xed\v\x9d\xa1{BЛ\x03\xfb2\xe4\xcc\xcc\xcc\xcc\xcc\xe7`~fv\a\xe6S\x91ݕ\xaa\xaa\xaa\xaa\xaa\xab\xb0?\x1d\x81\xfd\x81\xfd\x81\xf8\xec\x0f\xec\x0f\xa1'\xb7<\xff\xff\xff\xff\xff\xff\x19\xc61\x9cb\x8c\x8e\xbd{\xff\xff\xff\xff\xff\xff8\xces\x8c\xe6\x84\xd6'\xc1Y\x99\x99\x99\x99\x99\x8e\xb1>1\x8e\xb1>1j#\xefx8d\xcc\xcc\xcc\xcc\xcc\xda\xc4\xfd\xe0\xf5\x89\xfa\xc4\xfdb~\xf0z\xc4\xfdbz\x04\xdc\x17\a\xaa\xaa\xaa\xaa\xaa\xabx=\xc1{\xc1\xef\a\xbc\x1e\xe0\xbd\xe0\xf7\x83A\x93\x1c\xff\xff\xff\xff\xff\xffp\\\xee\v\xdc\x17\xb8.w\x05\xee\v@\x9bC\xf0Z\xaa\xaa\xaa\xaa\xaa\xa7h~fv\x87\xe6P\xe4al\xcc\xcc\xcc\xcc\xcc\xed\x0f\xc7h\x7fh\x7fh~;C\xfbC\xe8\x12sə\x99\x99\x99\x99\xa38\xc63\x8cPd`\xb5UUUUUN3\x9c\xe39\xa0M\x82|3\xff\xff\xff\xff\xff\xf8\xec\x13\xe3\x18\xec\x13\xe3\x14y\f\x1e\xaa\xaa\xaa\xaa\xaa\xad\x82|;\x04\xfd\x82~\xc1>\x1d\x82~\xc1=\x02G\x1c\x99\x99\x99\x99\x99\x9a\x18\xe1\x86\x18\xe1\x85\x06C\x05ffffff8c\x8e8c\x8d\x02O\v\xaa\xaa\xaa\xaa\xaa\xaa\x19\xe1\x86\x19\xe1\x85\x0eC\xa3\x8f\xf1UUUUUY\xe1\x9ey\xe1\x9et\t\x1c\x01j\xaa\xaa\xaa\xaa\xab\fp\xc3\fp\u0083!\x80{33333#\x868\xe3\x868\xc0"),
						},
					},
				},
			},
			QueryIndex: 2,
		},
	}, results)
}

func addNativeHistogramsToTestSuite(t *testing.T, pqlTest *promql.Test, n int) {
	lbls := labels.FromStrings("__name__", "test_histogram_metric1", "baz", "qux")

	app := pqlTest.Storage().Appender(context.TODO())
	for i, fh := range tsdbutil.GenerateTestFloatHistograms(n) {
		_, err := app.AppendHistogram(0, lbls, int64(i)*int64(60*time.Second/time.Millisecond), nil, fh)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
}
