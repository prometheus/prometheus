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
	"math"
	"strconv"
	"testing"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/prometheus/prometheus/model/labels"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// BenchmarkIngestionWithNativeMetadata compares RW2 and OTLP ingestion into the
// Head with native metadata enabled and disabled. Metadata remains unchanged
// after warm-up. HTTP handling, wire encoding/decoding, and WAL writes are excluded.
func BenchmarkIngestionWithNativeMetadata(b *testing.B) {
	const numSeries = 1000
	for _, transport := range []string{"rw2", "otlp"} {
		for _, families := range []int{1, 100, 1000} {
			for _, native := range []bool{false, true} {
				b.Run(fmt.Sprintf("transport=%s/values=%d/native=%t", transport, families, native), func(b *testing.B) {
					registry := prometheus.NewRegistry()
					opts := tsdb.DefaultHeadOptions()
					opts.ChunkDirRoot = b.TempDir()
					opts.EnableNativeMetadata = native
					opts.EnableMetadataWALRecords = false
					head, err := tsdb.NewHead(registry, nil, nil, nil, opts, nil)
					if err != nil {
						b.Fatal(err)
					}
					if err := head.Init(math.MinInt64); err != nil {
						b.Fatal(err)
					}
					b.Cleanup(func() {
						if err := head.Close(); err != nil {
							b.Fatal(err)
						}
					})
					var appendRound func(int64)
					if transport == "rw2" {
						symbols := writev2.NewSymbolTable()
						request := &writev2.Request{Timeseries: make([]writev2.TimeSeries, numSeries)}
						for i := range request.Timeseries {
							request.Timeseries[i] = writev2.TimeSeries{
								LabelsRefs: symbols.SymbolizeLabels(labels.FromStrings(labels.MetricName, "family_"+strconv.Itoa(i%families), "instance", strconv.Itoa(i)), nil),
								Metadata:   writev2.Metadata{Type: writev2.Metadata_METRIC_TYPE_GAUGE, HelpRef: symbols.Symbolize(fmt.Sprintf("Metric description %d.", i%families)), UnitRef: symbols.Symbolize("seconds")},
								Samples:    []writev2.Sample{{Value: 1}},
							}
						}
						request.Symbols = symbols.Symbols()
						handler := &writeHandler{logger: promslog.NewNopLogger(), appendMetadata: true, samplesWithInvalidLabelsTotal: prometheus.NewCounter(prometheus.CounterOpts{Name: "invalid_labels"})}
						appendRound = func(timestamp int64) {
							for i := range request.Timeseries {
								request.Timeseries[i].Samples[0].Timestamp = timestamp
							}
							app := &remoteWriteAppenderV2{AppenderV2: head.AppenderV2(b.Context()), maxTime: math.MaxInt64}
							var stats remoteapi.WriteResponseStats
							if _, _, err := handler.appendV2(app, request, &stats); err != nil {
								b.Fatal(err)
							}
							if stats.Samples != numSeries {
								b.Fatalf("accepted %d samples, want %d", stats.Samples, numSeries)
							}
							if err := app.Commit(); err != nil {
								b.Fatal(err)
							}
						}
					} else {
						metrics := pmetric.NewMetrics()
						metricSlice := metrics.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics()
						for family := range families {
							m := metricSlice.AppendEmpty()
							m.SetName("family_" + strconv.Itoa(family))
							m.SetDescription(fmt.Sprintf("Metric description %d.", family))
							m.SetUnit("seconds")
							points := m.SetEmptyGauge().DataPoints()
							for i := range numSeries / families {
								point := points.AppendEmpty()
								point.Attributes().PutStr("instance", strconv.Itoa(i))
								point.SetDoubleValue(1)
							}
						}
						converter := prometheusremotewrite.NewPrometheusConverter(nil)
						appendRound = func(timestamp int64) {
							for i := 0; i < metricSlice.Len(); i++ {
								points := metricSlice.At(i).Gauge().DataPoints()
								for j := 0; j < points.Len(); j++ {
									points.At(j).SetTimestamp(pcommon.Timestamp(timestamp * 1_000_000))
								}
							}
							app := head.AppenderV2(b.Context())
							converter.Reset(app)
							if _, err := converter.FromMetrics(b.Context(), metrics, prometheusremotewrite.Settings{DisableTargetInfo: true}); err != nil {
								b.Fatal(err)
							}
							if err := app.Commit(); err != nil {
								b.Fatal(err)
							}
							converter.Reset(nil)
						}
					}
					appendRound(100)
					b.ReportAllocs()
					iteration := int64(0)
					for b.Loop() {
						appendRound(1000 + iteration)
						iteration++
					}
					if got := head.NumSeries(); got != numSeries {
						b.Fatalf("got %d series, want %d", got, numSeries)
					}
					metrics, err := registry.Gather()
					if err != nil {
						b.Fatal(err)
					}
					foundMetadataGauge := false
					for _, metric := range metrics {
						if metric.GetName() == "prometheus_tsdb_head_native_metric_metadata_series" {
							foundMetadataGauge = true
							want := float64(0)
							if native {
								want = numSeries
							}
							if metric.Metric[0].GetGauge().GetValue() != want {
								b.Fatal("metadata was not retained")
							}
						}
					}
					if native && !foundMetadataGauge {
						b.Fatal("native metadata gauge is missing")
					}
					querier, err := tsdb.NewBlockChunkQuerier(head, math.MinInt64, math.MaxInt64)
					if err != nil {
						b.Fatal(err)
					}
					var samples int64
					var iterator chunks.Iterator
					series := querier.Select(b.Context(), false, nil, labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+"))
					for series.Next() {
						iterator = series.At().Iterator(iterator)
						for iterator.Next() {
							samples += int64(iterator.At().Chunk.NumSamples())
						}
						if err := iterator.Err(); err != nil {
							b.Fatal(err)
						}
					}
					if err := series.Err(); err != nil {
						b.Fatal(err)
					}
					if err := querier.Close(); err != nil {
						b.Fatal(err)
					}
					if want := (iteration + 1) * numSeries; samples != want {
						b.Fatalf("committed %d samples, want %d", samples, want)
					}
				})
			}
		}
	}
}
