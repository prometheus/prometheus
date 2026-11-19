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
	"strconv"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/teststorage"
)

// Encode the shard fields independently of the generated Go bindings, so these
// requests also verify the field numbers used by other remote-read clients.
func shardReadHints(count, index uint64) *prompb.ReadHints {
	fields := append([]byte{8 << 3}, proto.EncodeVarint(count)...)
	fields = append(fields, 9<<3)
	fields = append(fields, proto.EncodeVarint(index)...)
	return &prompb.ReadHints{StartMs: 0, EndMs: 1000, XXX_unrecognized: fields}
}

func executeShardedRead(
	t *testing.T,
	handler http.Handler,
	responseType prompb.ReadRequest_ResponseType,
	queries ...*prompb.Query,
) (*httptest.ResponseRecorder, map[string]labels.Labels) {
	t.Helper()
	data, err := proto.Marshal(&prompb.ReadRequest{
		Queries:               queries,
		AcceptedResponseTypes: []prompb.ReadRequest_ResponseType{responseType},
	})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/read", bytes.NewReader(snappy.Encode(nil, data)))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		return recorder, nil
	}

	series := map[string]labels.Labels{}
	builder := labels.NewScratchBuilder(0)
	if responseType == prompb.ReadRequest_SAMPLES {
		body, err := snappy.Decode(nil, recorder.Body.Bytes())
		require.NoError(t, err)
		var response prompb.ReadResponse
		require.NoError(t, proto.Unmarshal(body, &response))
		for _, result := range response.Results {
			for _, s := range result.Timeseries {
				ls := s.ToLabels(&builder, nil).Copy()
				series[ls.String()] = ls
			}
		}
	} else {
		reader := NewChunkedReader(recorder.Body, config.DefaultChunkedReadLimit, nil)
		for {
			var response prompb.ChunkedReadResponse
			if err := reader.NextProto(&response); errors.Is(err, io.EOF) {
				break
			} else {
				require.NoError(t, err)
			}
			for _, s := range response.ChunkedSeries {
				ls := s.ToLabels(&builder, nil).Copy()
				series[ls.String()] = ls
			}
		}
	}
	return recorder, series
}

func shardingTestStorage(t *testing.T, enabled bool) *teststorage.TestStorage {
	t.Helper()
	store := teststorage.New(t, func(opts *tsdb.Options) { opts.EnableSharding = enabled })
	app := store.AppenderV2(t.Context())
	for i := range 16 {
		ls := labels.FromStrings("__name__", "shard_test", "instance", strconv.Itoa(i))
		_, err := app.Append(0, ls, 0, 0, float64(i), nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
	addNativeHistogramsToTestSuite(t, store, 1)
	return store
}

func TestRemoteReadSharding(t *testing.T) {
	store := shardingTestStorage(t, true)
	matcher := labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+")
	querier, err := store.Querier(0, 1000)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, querier.Close()) })
	expected := map[string]uint64{}
	set := querier.Select(t.Context(), true, nil, matcher)
	for set.Next() {
		ls := set.At().Labels()
		expected[ls.String()] = labels.StableHash(ls)
	}
	require.NoError(t, set.Err())
	require.Len(t, expected, 18)

	handler := NewReadHandler(nil, nil, store, func() config.Config {
		return config.Config{GlobalConfig: config.GlobalConfig{ExternalLabels: labels.FromStrings("cluster", "remote")}}
	}, 1e6, 1, 1<<20)
	for _, responseType := range []prompb.ReadRequest_ResponseType{prompb.ReadRequest_SAMPLES, prompb.ReadRequest_STREAMED_XOR_CHUNKS} {
		t.Run(responseType.String(), func(t *testing.T) {
			for _, count := range []uint64{0, 1, 2, 3, 32} {
				t.Run(strconv.FormatUint(count, 10), func(t *testing.T) {
					seen := map[string]bool{}
					for index := range max(count, 1) {
						query := &prompb.Query{
							StartTimestampMs: 0,
							EndTimestampMs:   1000,
							Matchers:         []*prompb.LabelMatcher{{Type: prompb.LabelMatcher_RE, Name: labels.MetricName, Value: ".+"}},
							Hints:            shardReadHints(count, index),
						}
						recorder, result := executeShardedRead(t, handler, responseType, query)
						require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
						wantCount := 0
						for _, hash := range expected {
							if count == 0 || hash%count == index {
								wantCount++
							}
						}
						require.Len(t, result, wantCount, "shard %d of %d", index, count)
						for _, ls := range result {
							require.Equal(t, "remote", ls.Get("cluster"))
							builder := labels.NewBuilder(ls)
							builder.Del("cluster")
							key := builder.Labels().String()
							hash, ok := expected[key]
							require.True(t, ok)
							if count > 0 {
								require.Equal(t, index, hash%count)
							}
							require.False(t, seen[key], "series appeared in multiple shards")
							seen[key] = true
						}
					}
					require.Len(t, seen, len(expected))
				})
			}
		})
	}
}

func TestRemoteReadShardingDisabled(t *testing.T) {
	store := shardingTestStorage(t, false)
	handler := NewReadHandler(nil, nil, store, func() config.Config { return config.Config{} }, 0, 1, 1<<20)
	for _, responseType := range []prompb.ReadRequest_ResponseType{prompb.ReadRequest_SAMPLES, prompb.ReadRequest_STREAMED_XOR_CHUNKS} {
		t.Run(responseType.String(), func(t *testing.T) {
			query := &prompb.Query{
				StartTimestampMs: 0,
				EndTimestampMs:   1000,
				Matchers:         []*prompb.LabelMatcher{{Type: prompb.LabelMatcher_RE, Name: labels.MetricName, Value: ".+"}},
				Hints:            shardReadHints(2, 0),
			}
			recorder, _ := executeShardedRead(t, handler, responseType, query)
			require.Equal(t, http.StatusInternalServerError, recorder.Code)
			require.Contains(t, recorder.Body.String(), "sharding is disabled")
			query.Hints = shardReadHints(0, 0)
			recorder, result := executeShardedRead(t, handler, responseType, query)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Len(t, result, 18)
		})
	}
}

func TestRemoteReadRejectsInvalidShardHints(t *testing.T) {
	store := shardingTestStorage(t, false)
	handler := NewReadHandler(nil, nil, store, func() config.Config { return config.Config{} }, 0, 1, 1<<20)
	for _, responseType := range []prompb.ReadRequest_ResponseType{prompb.ReadRequest_SAMPLES, prompb.ReadRequest_STREAMED_XOR_CHUNKS} {
		t.Run(responseType.String(), func(t *testing.T) {
			for _, tc := range []struct {
				name         string
				count, index uint64
			}{
				{name: "disabled with index", index: 1},
				{name: "index equals count", count: 2, index: 2},
				{name: "index exceeds count", count: 2, index: 3},
			} {
				t.Run(tc.name, func(t *testing.T) {
					valid := &prompb.Query{
						StartTimestampMs: 0,
						EndTimestampMs:   1000,
						Matchers:         []*prompb.LabelMatcher{{Type: prompb.LabelMatcher_EQ, Name: labels.MetricName, Value: "shard_test"}},
					}
					invalid := &prompb.Query{Hints: shardReadHints(tc.count, tc.index)}
					recorder, _ := executeShardedRead(t, handler, responseType, valid, invalid)
					require.Equal(t, http.StatusBadRequest, recorder.Code)
					require.Contains(t, recorder.Body.String(), "shard")
					require.NotContains(t, recorder.Header().Get("Content-Type"), "protobuf")
				})
			}
		})
	}
}
