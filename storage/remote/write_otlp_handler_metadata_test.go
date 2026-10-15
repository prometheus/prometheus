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
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

func openOTLPMetadataDB(tb testing.TB, dir string, enabled bool) *tsdb.DB {
	tb.Helper()
	opts := tsdb.DefaultOptions()
	opts.EnableMetadataWALRecords = enabled
	opts.WALCompression = compression.Snappy
	db, err := tsdb.Open(dir, nil, nil, opts, nil)
	require.NoError(tb, err)
	db.DisableCompactions()
	return db
}

func newOTLPMetadataHandler(db *tsdb.DB) http.Handler {
	return NewOTLPWriteHandler(slog.New(slog.DiscardHandler), nil, db, func() config.Config {
		return config.Config{OTLPConfig: config.DefaultOTLPConfig}
	}, OTLPOptions{})
}

func otlpMetadataDescriptions(stress bool) (string, string) {
	if !stress {
		a := strings.Repeat("Measured worker queue depth at the observation timestamp. ", 3)[:128]
		b := strings.Repeat("Updated worker queue depth at the observation timestamp. ", 3)[:128]
		return a, b
	}
	// Deterministic varied text keeps the large-description stress case separate
	// from the compressibility of ordinary help text.
	rng := rand.New(rand.NewPCG(1, 2))
	buf := make([]byte, 64<<10)
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 "
	for i := range buf {
		buf[i] = alphabet[rng.IntN(len(alphabet))]
	}
	return "A" + string(buf[1:]), "B" + string(buf[1:])
}

func otlpMetadataTimestamp(point int) int64 {
	return 946684800000 + int64(point)*1000
}

func otlpMetadataRequest(series, observations, first int, descriptions []string, packed bool) pmetricotlp.ExportRequest {
	request := pmetricotlp.NewExportRequest()
	appendSnapshot := func(metrics pmetric.Metrics, begin, end int, description string) {
		rm := metrics.ResourceMetrics().AppendEmpty()
		// Identifying attributes do not create a target_info series.
		rm.Resource().Attributes().PutStr("service.name", "metadata-test")
		rm.Resource().Attributes().PutStr("service.instance.id", "instance")
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("metadata-test")
		metric := sm.Metrics().AppendEmpty()
		metric.SetName("otlp_metadata_queue_depth")
		metric.SetDescription(description)
		points := metric.SetEmptyGauge().DataPoints()
		for point := begin; point < end; point++ {
			for s := range series {
				dp := points.AppendEmpty()
				dp.Attributes().PutStr("series", strconv.Itoa(s))
				dp.SetTimestamp(pcommon.Timestamp(otlpMetadataTimestamp(point) * 1_000_000))
				dp.SetDoubleValue(float64(point*series + s))
			}
		}
	}
	if packed {
		appendSnapshot(request.Metrics(), first, first+observations, descriptions[0])
		return request
	}
	// Collector batching concatenates ResourceMetrics from successive snapshots.
	// Each snapshot here has one observation per series in a stable Metric.
	for i := range observations {
		description := descriptions[0]
		if len(descriptions) > 1 {
			description = descriptions[i]
		}
		snapshot := pmetric.NewMetrics()
		appendSnapshot(snapshot, first+i, first+i+1, description)
		snapshot.ResourceMetrics().MoveAndAppendTo(request.Metrics().ResourceMetrics())
	}
	return request
}

func serveOTLPMetadata(tb testing.TB, handler http.Handler, request pmetricotlp.ExportRequest) {
	tb.Helper()
	payload, err := request.MarshalProto()
	require.NoError(tb, err)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, newOTLPWriteHandlerRequest(payload).WithContext(tb.Context()))
	require.Equal(tb, http.StatusOK, response.Code, response.Body.String())
}

func readOTLPMetadataWAL(tb testing.TB, db *tsdb.DB) []record.RefMetadata {
	tb.Helper()
	segments, err := wlog.NewSegmentsReader(filepath.Join(db.Dir(), "wal"))
	require.NoError(tb, err)
	defer func() { require.NoError(tb, segments.Close()) }()
	reader := wlog.NewReader(segments)
	decoder := record.NewDecoder(labels.NewSymbolTable(), slog.New(slog.DiscardHandler))
	var entries []record.RefMetadata
	for reader.Next() {
		if decoder.Type(reader.Record()) == record.Metadata {
			entries, err = decoder.Metadata(reader.Record(), entries)
			require.NoError(tb, err)
		}
	}
	require.NoError(tb, reader.Err())
	return entries
}

func requireOTLPMetadataSamples(tb testing.TB, db *tsdb.DB, series, observations int) {
	tb.Helper()
	require.Equal(tb, uint64(series), db.Head().NumSeries())
	querier, err := db.Querier(otlpMetadataTimestamp(0), otlpMetadataTimestamp(observations))
	require.NoError(tb, err)
	defer func() { require.NoError(tb, querier.Close()) }()
	set := querier.Select(tb.Context(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "__name__", "otlp_metadata_queue_depth"))
	seen := make(map[int]bool, series)
	for set.Next() {
		s := set.At()
		index, err := strconv.Atoi(s.Labels().Get("series"))
		require.NoError(tb, err)
		require.GreaterOrEqual(tb, index, 0)
		require.Less(tb, index, series)
		require.False(tb, seen[index])
		seen[index] = true
		it := s.Iterator(nil)
		point := 0
		for typ := it.Next(); typ != chunkenc.ValNone; typ = it.Next() {
			require.Equal(tb, chunkenc.ValFloat, typ)
			ts, value := it.At()
			require.Equal(tb, otlpMetadataTimestamp(point), ts)
			require.Equal(tb, float64(point*series+index), value)
			point++
		}
		require.NoError(tb, it.Err())
		require.Equal(tb, observations, point)
	}
	require.NoError(tb, set.Err())
	require.Len(tb, seen, series)
}

func BenchmarkOTLPMetadataWAL(b *testing.B) {
	for _, shape := range []struct{ series, observations int }{{1024, 1}, {256, 4}, {64, 16}} {
		for _, state := range []string{"new", "changed", "unchanged", "disabled"} {
			if state == "disabled" && shape.observations != 1 {
				continue
			}
			b.Run(fmt.Sprintf("S%d/K%d/%s", shape.series, shape.observations, state), func(b *testing.B) {
				b.StopTimer()
				b.ReportAllocs()
				parent := b.TempDir()
				a, changed := otlpMetadataDescriptions(false)
				var db *tsdb.DB
				var handler http.Handler
				if state != "new" {
					db = openOTLPMetadataDB(b, parent, state != "disabled")
					defer func() { require.NoError(b, db.Close()) }()
					handler = newOTLPMetadataHandler(db)
					serveOTLPMetadata(b, handler, otlpMetadataRequest(shape.series, shape.observations, 0, []string{a}, false))
				}
				var walBytes int64
				for i := 0; i < b.N; i++ {
					first := (i + 1) * shape.observations
					if state == "new" {
						dir, err := os.MkdirTemp(parent, "db-")
						require.NoError(b, err)
						db = openOTLPMetadataDB(b, dir, true)
						handler = newOTLPMetadataHandler(db)
						first = 0
					}
					description := a
					if state == "changed" && i%2 == 0 {
						description = changed
					}
					payload, err := otlpMetadataRequest(shape.series, shape.observations, first, []string{description}, false).MarshalProto()
					require.NoError(b, err)
					request := newOTLPWriteHandlerRequest(payload).WithContext(b.Context())
					response := httptest.NewRecorder()
					before, err := fileutil.DirSize(filepath.Join(db.Dir(), "wal"))
					require.NoError(b, err)
					b.StartTimer()
					handler.ServeHTTP(response, request)
					b.StopTimer()
					require.Equal(b, http.StatusOK, response.Code, response.Body.String())
					after, err := fileutil.DirSize(filepath.Join(db.Dir(), "wal"))
					require.NoError(b, err)
					walBytes += after - before
					if i == b.N-1 {
						requireOTLPMetadataSamples(b, db, shape.series, first+shape.observations)
					}
					if state == "new" {
						dir := db.Dir()
						require.NoError(b, db.Close())
						require.NoError(b, os.RemoveAll(dir))
					}
				}
				// Measure committed compressed bytes before Close pads the WAL page.
				b.ReportMetric(float64(walBytes)/float64(b.N), "wal-B/op")
				b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*shape.series*shape.observations), "ns/sample")
			})
		}
	}
}
