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

package fuzzing

import (
	"bytes"
	"encoding/binary"
	"math"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	dto "github.com/prometheus/prometheus/prompb/io/prometheus/client"
)

func serializeProtobufSeed(families ...dto.MetricFamily) ([]byte, error) {
	varintBuf := make([]byte, binary.MaxVarintLen64)
	var out []byte
	for i := range families {
		b, err := proto.Marshal(&families[i])
		if err != nil {
			return nil, err
		}
		n := binary.PutUvarint(varintBuf, uint64(len(b)))
		out = append(out, varintBuf[:n]...)
		out = append(out, b...)
	}
	return out, nil
}

func appendSerializedProtobufSeed(dst [][]byte, families ...dto.MetricFamily) ([][]byte, error) {
	b, err := serializeProtobufSeed(families...)
	if err != nil {
		return nil, err
	}
	return append(dst, b), nil
}

// appendTextProtobufSeeds parses text-format proto strings (one MetricFamily
// per string) and appends the serialized binary payloads to dst. This lets the
// corpus reuse the same text-format fixtures that the parser unit tests use.
func appendTextProtobufSeeds(dst [][]byte, textFamilies ...string) ([][]byte, error) {
	for _, text := range textFamilies {
		pb := &dto.MetricFamily{}
		if err := proto.UnmarshalText(text, pb); err != nil {
			return nil, err
		}
		b, err := serializeProtobufSeed(*pb)
		if err != nil {
			return nil, err
		}
		dst = append(dst, b)
	}
	return dst, nil
}

func protobufCorruptSeeds() ([][]byte, error) {
	result := [][]byte{
		{},
		{0x00},
		{0xff},
		bytes.Repeat([]byte{0xff}, 10),
		{0x01, 0x00},
		{0x64, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0xde, 0xad, 0xbe, 0xef},
	}

	truncBase, err := serializeProtobufSeed(dto.MetricFamily{
		Name: "truncated_metric",
		Help: "This payload will be truncated.",
		Type: dto.MetricType_GAUGE,
		Metric: []dto.Metric{
			{
				Label: []dto.LabelPair{{Name: "foo", Value: "bar"}},
				Gauge: &dto.Gauge{Value: 42},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	result = append(result, truncBase[:len(truncBase)/2])

	validGauge, err := serializeProtobufSeed(dto.MetricFamily{
		Name:   "gauge_then_garbage",
		Type:   dto.MetricType_GAUGE,
		Metric: []dto.Metric{{Gauge: &dto.Gauge{Value: 1}}},
	})
	if err != nil {
		return nil, err
	}
	result = append(result, append(validGauge, 0xde, 0xad, 0xbe, 0xef))

	return result, nil
}

// ProtobufCorpusSeed is a single seed entry for FuzzParseProtobuf, pairing a
// binary payload with a specific set of parser options.
type ProtobufCorpusSeed struct {
	Data         []byte
	IgnoreNative bool
	ParseClassic bool
	ConvertNHCB  bool
	TypeAndUnit  bool
}

// GetCorpusForFuzzParseProtobuf returns the seed corpus for FuzzParseProtobuf.
// Each entry pairs a length-prefixed binary protobuf payload in the Prometheus
// protobuf exposition format (uvarint-length followed by a marshaled
// MetricFamily message) with a specific combination of parser options. The
// corpus covers all metric types and their variants, multi-metric and
// multi-family payloads, edge cases in valid data, structurally corrupt
// inputs, and key combinations of parser boolean options that gate distinct
// code paths (ignoreNative, parseClassic, convertNHCB, typeAndUnit).
func GetCorpusForFuzzParseProtobuf() ([]ProtobufCorpusSeed, error) {
	rawCorpus, err := getRawProtobufCorpus()
	if err != nil {
		return nil, err
	}

	// flagCombos lists the parser-option combinations that gate distinct code
	// paths in Next() and related helpers. Each combination is applied to every
	// raw payload so the fuzzer starts with coverage of those branches rather
	// than having to discover them by random bit-flipping.
	//
	//  IgnoreNative=true  routes native histograms through the classic/series
	//                     path instead of EntryHistogram.
	//  ParseClassic=true  triggers the redoClassic state machine so both native
	//                     and classic representations are emitted.
	//  ConvertNHCB=true   activates convertToNHCB(), producing schema -53
	//                     (custom-bucket) native histograms from classic data.
	//  TypeAndUnit=true   exercises schema.Metadata.AddToLabels() in
	//                     onSeriesOrHistogramUpdate().
	type flagSet struct {
		ignoreNative, parseClassic, convertNHCB, typeAndUnit bool
	}
	flagCombos := []flagSet{
		{false, false, false, false}, // baseline — all paths off.
		{true, false, false, false},  // ignoreNative: native→classic routing.
		{false, true, true, false},   // parseClassic+convertNHCB: NHCB with classic emit.
		{false, false, true, true},   // convertNHCB+typeAndUnit: NHCB conversion with metadata labels.
	}

	seeds := make([]ProtobufCorpusSeed, 0, len(rawCorpus)*len(flagCombos))
	for _, data := range rawCorpus {
		for _, f := range flagCombos {
			seeds = append(seeds, ProtobufCorpusSeed{
				Data:         data,
				IgnoreNative: f.ignoreNative,
				ParseClassic: f.parseClassic,
				ConvertNHCB:  f.convertNHCB,
				TypeAndUnit:  f.typeAndUnit,
			})
		}
	}
	return seeds, nil
}

func getRawProtobufCorpus() ([][]byte, error) {
	var result [][]byte

	// --- GAUGE ---

	// Minimal gauge: no labels, no help, no timestamp.
	var err error
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name:   "up",
		Type:   dto.MetricType_GAUGE,
		Metric: []dto.Metric{{Gauge: &dto.Gauge{Value: 1}}},
	}); err != nil {
		return nil, err
	}

	// Gauge: multiple labels, with timestamp.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "go_build_info",
		Help: "Build information about the main Go module.",
		Type: dto.MetricType_GAUGE,
		Metric: []dto.Metric{
			{
				Label: []dto.LabelPair{
					{Name: "checksum", Value: ""},
					{Name: "path", Value: "github.com/prometheus/client_golang"},
					{Name: "version", Value: "(devel)"},
				},
				Gauge:       &dto.Gauge{Value: 1},
				TimestampMs: 1395066363000,
			},
		},
	}); err != nil {
		return nil, err
	}

	// Gauge: with unit.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "node_cpu_seconds",
		Help: "CPU time spent.",
		Unit: "seconds",
		Type: dto.MetricType_GAUGE,
		Metric: []dto.Metric{
			{
				Label: []dto.LabelPair{{Name: "cpu", Value: "0"}, {Name: "mode", Value: "idle"}},
				Gauge: &dto.Gauge{Value: 123.456},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Gauge: very long metric name and many labels.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "very_long_gauge_metric_name_to_test_string_handling_in_the_parser",
		Help: "A metric with a long name and many labels to stress string handling in the parser.",
		Type: dto.MetricType_GAUGE,
		Metric: []dto.Metric{
			{
				Label: []dto.LabelPair{
					{Name: "label_a", Value: "value_a"},
					{Name: "label_b", Value: "value_b"},
					{Name: "label_c", Value: "value_c"},
					{Name: "label_d", Value: "value_d"},
					{Name: "label_e", Value: "value_e"},
					{Name: "label_f", Value: "a_very_long_label_value_to_test_label_value_handling"},
				},
				Gauge: &dto.Gauge{Value: 0},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Gauge: multiple metrics in family with different label sets.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "http_server_connections",
		Help: "Current number of HTTP server connections.",
		Type: dto.MetricType_GAUGE,
		Metric: []dto.Metric{
			{Label: []dto.LabelPair{{Name: "state", Value: "active"}}, Gauge: &dto.Gauge{Value: 42}},
			{Label: []dto.LabelPair{{Name: "state", Value: "idle"}}, Gauge: &dto.Gauge{Value: 100}},
			{Label: []dto.LabelPair{{Name: "state", Value: "hijacked"}}, Gauge: &dto.Gauge{Value: 3}},
		},
	}); err != nil {
		return nil, err
	}

	// Gauge: empty label value.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "metric_empty_label",
		Type: dto.MetricType_GAUGE,
		Metric: []dto.Metric{
			{
				Label: []dto.LabelPair{{Name: "env", Value: ""}},
				Gauge: &dto.Gauge{Value: 7},
			},
		},
	}); err != nil {
		return nil, err
	}

	// --- COUNTER ---

	// Counter: minimal.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name:   "http_requests_total",
		Type:   dto.MetricType_COUNTER,
		Metric: []dto.Metric{{Counter: &dto.Counter{Value: 0}}},
	}); err != nil {
		return nil, err
	}

	// Counter: with labels and timestamp.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "http_requests_total",
		Help: "Total HTTP requests.",
		Type: dto.MetricType_COUNTER,
		Metric: []dto.Metric{
			{
				Label:       []dto.LabelPair{{Name: "code", Value: "200"}, {Name: "method", Value: "GET"}},
				Counter:     &dto.Counter{Value: 1027},
				TimestampMs: 1395066363000,
			},
		},
	}); err != nil {
		return nil, err
	}

	// Counter: with exemplar (has timestamp).
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "go_memstats_alloc_bytes_total",
		Help: "Total number of bytes allocated.",
		Type: dto.MetricType_COUNTER,
		Metric: []dto.Metric{
			{
				Counter: &dto.Counter{
					Value: 1546544,
					Exemplar: &dto.Exemplar{
						Label:     []dto.LabelPair{{Name: "dummyID", Value: "42"}},
						Value:     12,
						Timestamp: &types.Timestamp{Seconds: 1625851151, Nanos: 233181499},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Counter: with start_timestamp.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "process_cpu_seconds_total",
		Help: "Total user and system CPU time spent.",
		Type: dto.MetricType_COUNTER,
		Metric: []dto.Metric{
			{
				Counter: &dto.Counter{
					Value:          12.5,
					StartTimestamp: &types.Timestamp{Seconds: 1625000000},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Counter: multiple metrics in family.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "rpc_calls_total",
		Help: "Total RPC calls by method.",
		Type: dto.MetricType_COUNTER,
		Metric: []dto.Metric{
			{Label: []dto.LabelPair{{Name: "method", Value: "Foo"}}, Counter: &dto.Counter{Value: 10}},
			{Label: []dto.LabelPair{{Name: "method", Value: "Bar"}}, Counter: &dto.Counter{Value: 20}},
			{Label: []dto.LabelPair{{Name: "method", Value: "Baz"}}, Counter: &dto.Counter{Value: 0}},
		},
	}); err != nil {
		return nil, err
	}

	// --- SUMMARY ---

	// Summary: no quantiles. Exercises the getMagicLabel bounds-check path.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "summary_no_quantiles",
		Help: "Summary with no quantile objectives.",
		Type: dto.MetricType_SUMMARY,
		Metric: []dto.Metric{
			{Summary: &dto.Summary{SampleCount: 500, SampleSum: 2500}},
		},
	}); err != nil {
		return nil, err
	}

	// Summary: single quantile.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "summary_one_quantile",
		Type: dto.MetricType_SUMMARY,
		Metric: []dto.Metric{
			{
				Summary: &dto.Summary{
					SampleCount: 100,
					SampleSum:   500,
					Quantile:    []dto.Quantile{{Quantile: 0.5, Value: 4.2}},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Summary: multiple quantiles.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "rpc_duration_seconds",
		Help: "RPC duration in seconds.",
		Type: dto.MetricType_SUMMARY,
		Metric: []dto.Metric{
			{
				Summary: &dto.Summary{
					SampleCount: 1000,
					SampleSum:   5000.5,
					Quantile: []dto.Quantile{
						{Quantile: 0.5, Value: 4.2},
						{Quantile: 0.9, Value: 8.3},
						{Quantile: 0.99, Value: 12.1},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Summary: with start_timestamp.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "summary_with_ct",
		Help: "Summary with start timestamp.",
		Type: dto.MetricType_SUMMARY,
		Metric: []dto.Metric{
			{
				Summary: &dto.Summary{
					SampleCount:    200,
					SampleSum:      1000,
					StartTimestamp: &types.Timestamp{Seconds: 1625000000},
					Quantile: []dto.Quantile{
						{Quantile: 0.5, Value: 5},
						{Quantile: 0.99, Value: 10},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Summary: multiple metrics in family.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "http_request_duration_seconds",
		Help: "HTTP request latency by handler.",
		Type: dto.MetricType_SUMMARY,
		Metric: []dto.Metric{
			{
				Label: []dto.LabelPair{{Name: "handler", Value: "/api"}},
				Summary: &dto.Summary{
					SampleCount: 100,
					SampleSum:   50,
					Quantile:    []dto.Quantile{{Quantile: 0.5, Value: 0.4}, {Quantile: 0.99, Value: 1.2}},
				},
			},
			{
				Label: []dto.LabelPair{{Name: "handler", Value: "/health"}},
				Summary: &dto.Summary{
					SampleCount: 5000,
					SampleSum:   5,
					Quantile:    []dto.Quantile{{Quantile: 0.5, Value: 0.001}, {Quantile: 0.99, Value: 0.005}},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// --- CLASSIC HISTOGRAM ---

	// Classic histogram: no buckets.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name:   "histogram_no_buckets",
		Type:   dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{{Histogram: &dto.Histogram{SampleCount: 0, SampleSum: 0}}},
	}); err != nil {
		return nil, err
	}

	// Classic histogram: single explicit +Inf bucket. Exercises the
	// getMagicLabel terminal case where math.IsInf(upper_bound, +1) is true.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "histogram_inf_only",
		Help: "Classic histogram with only the explicit +Inf bucket.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount: 100,
					SampleSum:   42,
					Bucket:      []dto.Bucket{{CumulativeCount: 100, UpperBound: math.Inf(1)}},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Classic histogram: many finite buckets.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "http_request_duration_seconds",
		Help: "Request duration histogram.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount: 144320,
					SampleSum:   53423,
					Bucket: []dto.Bucket{
						{CumulativeCount: 24054, UpperBound: 0.5},
						{CumulativeCount: 33444, UpperBound: 1},
						{CumulativeCount: 100392, UpperBound: 2.5},
						{CumulativeCount: 129389, UpperBound: 5},
						{CumulativeCount: 133988, UpperBound: 10},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Classic histogram: bucket with exemplar.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "histogram_with_exemplar",
		Help: "Classic histogram with an exemplar on a bucket.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount: 1000,
					SampleSum:   500,
					Bucket: []dto.Bucket{
						{
							CumulativeCount: 900,
							UpperBound:      1,
							Exemplar: &dto.Exemplar{
								Label:     []dto.LabelPair{{Name: "trace_id", Value: "abc123"}},
								Value:     0.9,
								Timestamp: &types.Timestamp{Seconds: 1625851155},
							},
						},
						{CumulativeCount: 1000, UpperBound: 5},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Classic histogram: float cumulative counts.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "histogram_float_counts",
		Help: "Classic histogram using floating-point cumulative counts.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCountFloat: 100.5,
					SampleSum:        500,
					Bucket: []dto.Bucket{
						{CumulativeCountFloat: 30, UpperBound: 1},
						{CumulativeCountFloat: 80, UpperBound: 5},
						{CumulativeCountFloat: 100.5, UpperBound: 10},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Classic histogram: multiple metrics in family.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "request_size_bytes",
		Help: "Request payload size in bytes.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Label: []dto.LabelPair{{Name: "method", Value: "GET"}},
				Histogram: &dto.Histogram{
					SampleCount: 1000,
					SampleSum:   50000,
					Bucket: []dto.Bucket{
						{CumulativeCount: 100, UpperBound: 100},
						{CumulativeCount: 900, UpperBound: 1000},
					},
				},
			},
			{
				Label: []dto.LabelPair{{Name: "method", Value: "POST"}},
				Histogram: &dto.Histogram{
					SampleCount: 500,
					SampleSum:   250000,
					Bucket: []dto.Bucket{
						{CumulativeCount: 50, UpperBound: 100},
						{CumulativeCount: 500, UpperBound: 10000},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// --- NATIVE HISTOGRAM (INTEGER) ---

	// Native histogram: positive spans only.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "native_pos_only",
		Help: "Native integer histogram with positive spans only.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount:   100,
					SampleSum:     50,
					Schema:        3,
					ZeroThreshold: 0.001,
					ZeroCount:     5,
					PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 3}},
					PositiveDelta: []int64{10, 5, -3},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Native histogram: both positive and negative spans, with timestamp.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "test_native_histogram",
		Help: "A native integer histogram.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount:   175,
					SampleSum:     0.0008280461746287094,
					Schema:        3,
					ZeroThreshold: 2.938735877055719e-39,
					ZeroCount:     2,
					PositiveSpan:  []dto.BucketSpan{{Offset: -161, Length: 1}, {Offset: 8, Length: 3}},
					PositiveDelta: []int64{1, 2, -1, -1},
					NegativeSpan:  []dto.BucketSpan{{Offset: -162, Length: 1}, {Offset: 23, Length: 4}},
					NegativeDelta: []int64{1, 3, -2, -1, 1},
				},
				TimestampMs: 1234568,
			},
		},
	}); err != nil {
		return nil, err
	}

	// Native histogram: with exemplars in the native part.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "native_with_exemplars",
		Help: "Native histogram with exemplars attached to the native part.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount:   50,
					SampleSum:     25,
					Schema:        1,
					ZeroThreshold: 0.001,
					ZeroCount:     1,
					PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 2}},
					PositiveDelta: []int64{20, 29},
					Exemplars: []*dto.Exemplar{
						{
							Label:     []dto.LabelPair{{Name: "trace_id", Value: "xyz789"}},
							Value:     1.5,
							Timestamp: &types.Timestamp{Seconds: 1625851200},
						},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// --- NATIVE HISTOGRAM (FLOAT) ---

	// Native float histogram.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "test_float_histogram",
		Help: "A native float histogram.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCountFloat: 175,
					SampleSum:        0.0008280461746287094,
					Schema:           3,
					ZeroThreshold:    2.938735877055719e-39,
					ZeroCountFloat:   2,
					PositiveSpan:     []dto.BucketSpan{{Offset: -161, Length: 1}, {Offset: 8, Length: 3}},
					PositiveCount:    []float64{1, 3, 2, 1},
					NegativeSpan:     []dto.BucketSpan{{Offset: -162, Length: 1}, {Offset: 23, Length: 4}},
					NegativeCount:    []float64{1, 4, 2, 1, 2},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// --- GAUGE HISTOGRAM (NATIVE) ---

	// Gauge histogram: native integer.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "test_gauge_histogram",
		Help: "A native gauge histogram.",
		Type: dto.MetricType_GAUGE_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount:   175,
					SampleSum:     0.0008280461746287094,
					Schema:        3,
					ZeroThreshold: 2.938735877055719e-39,
					ZeroCount:     2,
					PositiveSpan:  []dto.BucketSpan{{Offset: -161, Length: 1}},
					PositiveDelta: []int64{1},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// --- MIXED HISTOGRAM (NATIVE + CLASSIC BUCKETS) ---

	// Mixed: native spans and classic buckets in the same metric.
	// Exercises parseClassicHistograms, redoClassic, and NHCB conversion paths.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "mixed_histogram",
		Help: "Histogram carrying both native spans and classic buckets.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount:   175,
					SampleSum:     0.0008280461746287094,
					Schema:        3,
					ZeroThreshold: 2.938735877055719e-39,
					ZeroCount:     2,
					PositiveSpan:  []dto.BucketSpan{{Offset: -161, Length: 1}, {Offset: 8, Length: 3}},
					PositiveDelta: []int64{1, 2, -1, -1},
					Bucket: []dto.Bucket{
						{CumulativeCount: 2, UpperBound: -0.0004899999999999998},
						{CumulativeCount: 4, UpperBound: -0.0003899999999999998},
						{CumulativeCount: 16, UpperBound: -0.0002899999999999998},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Mixed: multiple metrics, some with and some without classic buckets.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "mixed_multi_metric",
		Help: "Native histogram family with mixed metrics.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Label: []dto.LabelPair{{Name: "shard", Value: "0"}},
				Histogram: &dto.Histogram{
					SampleCount:   100,
					SampleSum:     50,
					Schema:        2,
					ZeroThreshold: 0.001,
					ZeroCount:     5,
					PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 2}},
					PositiveDelta: []int64{40, 55},
					Bucket:        []dto.Bucket{{CumulativeCount: 95, UpperBound: 10}},
				},
			},
			{
				Label: []dto.LabelPair{{Name: "shard", Value: "1"}},
				Histogram: &dto.Histogram{
					SampleCount:   200,
					SampleSum:     100,
					Schema:        2,
					ZeroThreshold: 0.001,
					ZeroCount:     10,
					PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 2}},
					PositiveDelta: []int64{80, 110},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// --- UNTYPED ---

	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name:   "something_untyped",
		Help:   "Just to test the untyped type.",
		Type:   dto.MetricType_UNTYPED,
		Metric: []dto.Metric{{Untyped: &dto.Untyped{Value: 42}, TimestampMs: 1234567}},
	}); err != nil {
		return nil, err
	}

	// --- MULTI-FAMILY PAYLOADS ---

	// All six metric types in a single payload.
	if result, err = appendSerializedProtobufSeed(result,
		dto.MetricFamily{
			Name:   "multi_gauge",
			Type:   dto.MetricType_GAUGE,
			Metric: []dto.Metric{{Gauge: &dto.Gauge{Value: 1}}},
		},
		dto.MetricFamily{
			Name:   "multi_counter",
			Type:   dto.MetricType_COUNTER,
			Metric: []dto.Metric{{Counter: &dto.Counter{Value: 10}}},
		},
		dto.MetricFamily{
			Name:   "multi_summary",
			Type:   dto.MetricType_SUMMARY,
			Metric: []dto.Metric{{Summary: &dto.Summary{SampleCount: 5, SampleSum: 2.5}}},
		},
		dto.MetricFamily{
			Name: "multi_classic_hist",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{
				{Histogram: &dto.Histogram{
					SampleCount: 100,
					SampleSum:   50,
					Bucket:      []dto.Bucket{{CumulativeCount: 100, UpperBound: 1}},
				}},
			},
		},
		dto.MetricFamily{
			Name: "multi_native_hist",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{
				{Histogram: &dto.Histogram{
					SampleCount:   10,
					SampleSum:     5,
					Schema:        1,
					ZeroThreshold: 0.001,
					ZeroCount:     1,
					PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 1}},
					PositiveDelta: []int64{9},
				}},
			},
		},
		dto.MetricFamily{
			Name:   "multi_untyped",
			Type:   dto.MetricType_UNTYPED,
			Metric: []dto.Metric{{Untyped: &dto.Untyped{Value: 99}}},
		},
	); err != nil {
		return nil, err
	}

	// Empty family followed by a real family. The parser skips empty families.
	if result, err = appendSerializedProtobufSeed(result,
		dto.MetricFamily{Name: "empty_family", Type: dto.MetricType_GAUGE},
		dto.MetricFamily{
			Name:   "real_family",
			Type:   dto.MetricType_GAUGE,
			Metric: []dto.Metric{{Gauge: &dto.Gauge{Value: 1}}},
		},
	); err != nil {
		return nil, err
	}

	// Classic histogram followed by native histogram in the same payload.
	if result, err = appendSerializedProtobufSeed(result,
		dto.MetricFamily{
			Name: "classic_in_payload",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{
				{Histogram: &dto.Histogram{
					SampleCount: 10,
					SampleSum:   5,
					Bucket:      []dto.Bucket{{CumulativeCount: 10, UpperBound: 1}},
				}},
			},
		},
		dto.MetricFamily{
			Name: "native_in_payload",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{
				{Histogram: &dto.Histogram{
					SampleCount:   10,
					SampleSum:     5,
					Schema:        1,
					ZeroThreshold: 0.001,
					ZeroCount:     1,
					PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 1}},
					PositiveDelta: []int64{9},
				}},
			},
		},
	); err != nil {
		return nil, err
	}

	// Three families, each with multiple metrics and distinct label sets.
	if result, err = appendSerializedProtobufSeed(result,
		dto.MetricFamily{
			Name: "job_a_requests_total",
			Type: dto.MetricType_COUNTER,
			Metric: []dto.Metric{
				{Label: []dto.LabelPair{{Name: "status", Value: "200"}}, Counter: &dto.Counter{Value: 500}},
				{Label: []dto.LabelPair{{Name: "status", Value: "500"}}, Counter: &dto.Counter{Value: 3}},
			},
		},
		dto.MetricFamily{
			Name: "job_a_latency_seconds",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{
				{Histogram: &dto.Histogram{
					SampleCount: 503,
					SampleSum:   10.6,
					Bucket: []dto.Bucket{
						{CumulativeCount: 400, UpperBound: 0.1},
						{CumulativeCount: 503, UpperBound: 1},
					},
				}},
			},
		},
		dto.MetricFamily{
			Name: "job_a_memory_bytes",
			Type: dto.MetricType_GAUGE,
			Metric: []dto.Metric{
				{Gauge: &dto.Gauge{Value: 1073741824}},
			},
		},
	); err != nil {
		return nil, err
	}

	// --- SEEDS FROM PARSER UNIT TESTS ---
	// These use the same text-format fixtures as model/textparse/protobufparse_test.go,
	// covering cases not otherwise represented: negative upper bounds, schema 0,
	// zero_threshold=0, no-op spans, float gauge histograms, and native exemplars.

	if result, err = appendTextProtobufSeeds(result,
		// Mixed native+classic histogram with negative upper bounds and bucket exemplars.
		`name: "test_histogram"
help: "Test histogram with many buckets removed to keep it manageable in size."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count: 175
    sample_sum: 0.0008280461746287094
    bucket: <
      cumulative_count: 2
      upper_bound: -0.0004899999999999998
    >
    bucket: <
      cumulative_count: 4
      upper_bound: -0.0003899999999999998
      exemplar: <
        label: < name: "dummyID" value: "59727" >
        value: -0.00039
        timestamp: < seconds: 1625851155 nanos: 146848499 >
      >
    >
    bucket: <
      cumulative_count: 16
      upper_bound: -0.0002899999999999998
      exemplar: <
        label: < name: "dummyID" value: "5617" >
        value: -0.00029
      >
    >
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count: 2
    negative_span: < offset: -162 length: 1 >
    negative_span: < offset: 23 length: 4 >
    negative_delta: 1
    negative_delta: 3
    negative_delta: -2
    negative_delta: -1
    negative_delta: 1
    positive_span: < offset: -161 length: 1 >
    positive_span: < offset: 8 length: 3 >
    positive_delta: 1
    positive_delta: 2
    positive_delta: -1
    positive_delta: -1
  >
  timestamp_ms: 1234568
>
`,
		// Same structure as gauge histogram.
		`name: "test_gauge_histogram"
help: "Like test_histogram but as gauge histogram."
type: GAUGE_HISTOGRAM
metric: <
  histogram: <
    sample_count: 175
    sample_sum: 0.0008280461746287094
    bucket: <
      cumulative_count: 2
      upper_bound: -0.0004899999999999998
    >
    bucket: <
      cumulative_count: 4
      upper_bound: -0.0003899999999999998
      exemplar: <
        label: < name: "dummyID" value: "59727" >
        value: -0.00039
        timestamp: < seconds: 1625851155 nanos: 146848499 >
      >
    >
    bucket: <
      cumulative_count: 16
      upper_bound: -0.0002899999999999998
      exemplar: <
        label: < name: "dummyID" value: "5617" >
        value: -0.00029
      >
    >
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count: 2
    negative_span: < offset: -162 length: 1 >
    negative_span: < offset: 23 length: 4 >
    negative_delta: 1
    negative_delta: 3
    negative_delta: -2
    negative_delta: -1
    negative_delta: 1
    positive_span: < offset: -161 length: 1 >
    positive_span: < offset: 8 length: 3 >
    positive_delta: 1
    positive_delta: 2
    positive_delta: -1
    positive_delta: -1
  >
  timestamp_ms: 1234568
>
`,
		// Float native histogram with negative upper bounds and bucket exemplars.
		`name: "test_float_histogram"
help: "Test float histogram with many buckets removed to keep it manageable in size."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count_float: 175.0
    sample_sum: 0.0008280461746287094
    bucket: <
      cumulative_count_float: 2.0
      upper_bound: -0.0004899999999999998
    >
    bucket: <
      cumulative_count_float: 4.0
      upper_bound: -0.0003899999999999998
      exemplar: <
        label: < name: "dummyID" value: "59727" >
        value: -0.00039
        timestamp: < seconds: 1625851155 nanos: 146848499 >
      >
    >
    bucket: <
      cumulative_count_float: 16
      upper_bound: -0.0002899999999999998
      exemplar: <
        label: < name: "dummyID" value: "5617" >
        value: -0.00029
      >
    >
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count_float: 2.0
    negative_span: < offset: -162 length: 1 >
    negative_span: < offset: 23 length: 4 >
    negative_count: 1.0
    negative_count: 3.0
    negative_count: -2.0
    negative_count: -1.0
    negative_count: 1.0
    positive_span: < offset: -161 length: 1 >
    positive_span: < offset: 8 length: 3 >
    positive_count: 1.0
    positive_count: 2.0
    positive_count: -1.0
    positive_count: -1.0
  >
  timestamp_ms: 1234568
>
`,
		// Float gauge histogram with negative upper bounds.
		`name: "test_gauge_float_histogram"
help: "Like test_float_histogram but as gauge histogram."
type: GAUGE_HISTOGRAM
metric: <
  histogram: <
    sample_count_float: 175.0
    sample_sum: 0.0008280461746287094
    bucket: <
      cumulative_count_float: 2.0
      upper_bound: -0.0004899999999999998
    >
    bucket: <
      cumulative_count_float: 4.0
      upper_bound: -0.0003899999999999998
      exemplar: <
        label: < name: "dummyID" value: "59727" >
        value: -0.00039
        timestamp: < seconds: 1625851155 nanos: 146848499 >
      >
    >
    bucket: <
      cumulative_count_float: 16
      upper_bound: -0.0002899999999999998
      exemplar: <
        label: < name: "dummyID" value: "5617" >
        value: -0.00029
      >
    >
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count_float: 2.0
    negative_span: < offset: -162 length: 1 >
    negative_span: < offset: 23 length: 4 >
    negative_count: 1.0
    negative_count: 3.0
    negative_count: -2.0
    negative_count: -1.0
    negative_count: 1.0
    positive_span: < offset: -161 length: 1 >
    positive_span: < offset: 8 length: 3 >
    positive_count: 1.0
    positive_count: 2.0
    positive_count: -1.0
    positive_count: -1.0
  >
  timestamp_ms: 1234568
>
`,
		// Classic-only histogram: schema 0, zero_threshold 0, no spans.
		`name: "test_histogram2"
help: "Similar histogram as before but now without sparse buckets."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count: 175
    sample_sum: 0.000828
    bucket: <
      cumulative_count: 2
      upper_bound: -0.00048
    >
    bucket: <
      cumulative_count: 4
      upper_bound: -0.00038
      exemplar: <
        label: < name: "dummyID" value: "59727" >
        value: -0.00038
        timestamp: < seconds: 1625851153 nanos: 146848499 >
      >
    >
    bucket: <
      cumulative_count: 16
      upper_bound: 1
      exemplar: <
        label: < name: "dummyID" value: "5617" >
        value: -0.000295
      >
    >
    schema: 0
    zero_threshold: 0
  >
>
`,
		// Classic histogram: schema 0, integer buckets with negative upper bound.
		`name: "test_histogram3"
help: "Similar histogram as before but now with integer buckets."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count: 6
    sample_sum: 50
    bucket: <
      cumulative_count: 2
      upper_bound: -20
    >
    bucket: <
      cumulative_count: 4
      upper_bound: 20
      exemplar: <
        label: < name: "dummyID" value: "59727" >
        value: 15
        timestamp: < seconds: 1625851153 nanos: 146848499 >
      >
    >
    bucket: <
      cumulative_count: 6
      upper_bound: 30
      exemplar: <
        label: < name: "dummyID" value: "5617" >
        value: 25
      >
    >
    schema: 0
    zero_threshold: 0
  >
>
`,
		// Family with two mixed native+classic metrics.
		`name: "test_histogram_family"
help: "Test histogram metric family with two very simple histograms."
type: HISTOGRAM
metric: <
  label: < name: "foo" value: "bar" >
  histogram: <
    sample_count: 5
    sample_sum: 12.1
    bucket: < cumulative_count: 2 upper_bound: 1.1 >
    bucket: < cumulative_count: 3 upper_bound: 2.2 >
    schema: 3
    positive_span: < offset: 8 length: 2 >
    positive_delta: 2
    positive_delta: 1
  >
>
metric: <
  label: < name: "foo" value: "baz" >
  histogram: <
    sample_count: 6
    sample_sum: 13.1
    bucket: < cumulative_count: 1 upper_bound: 1.1 >
    bucket: < cumulative_count: 5 upper_bound: 2.2 >
    schema: 3
    positive_span: < offset: 8 length: 2 >
    positive_delta: 1
    positive_delta: 4
  >
>
`,
		// Float native histogram with zero_threshold=0 (no zero bucket).
		`name: "test_float_histogram_with_zerothreshold_zero"
help: "Test float histogram with a zero threshold of zero."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count_float: 5.0
    sample_sum: 12.1
    schema: 3
    positive_span: < offset: 8 length: 2 >
    positive_count: 2.0
    positive_count: 3.0
  >
>
`,
		// Empty native histogram: no-op span identifies it as native, no data.
		`name: "empty_histogram"
help: "A histogram without observations and with a zero threshold of zero but with a no-op span to identify it as a native histogram."
type: HISTOGRAM
metric: <
  histogram: <
    positive_span: < offset: 0 length: 0 >
  >
>
`,
		// Summary with start_timestamp.
		`name: "test_summary_with_createdtimestamp"
help: "A summary with a start timestamp."
type: SUMMARY
metric: <
  summary: <
    sample_count: 42
    sample_sum: 1.234
    start_timestamp: < seconds: 1625851153 nanos: 146848499 >
  >
>
`,
		// Native histogram with start_timestamp.
		`name: "test_histogram_with_createdtimestamp"
help: "A histogram with a start timestamp."
type: HISTOGRAM
metric: <
  histogram: <
    start_timestamp: < seconds: 1625851153 nanos: 146848499 >
    positive_span: < offset: 0 length: 0 >
  >
>
`,
		// Gauge histogram with start_timestamp.
		`name: "test_gaugehistogram_with_createdtimestamp"
help: "A gauge histogram with a start timestamp."
type: GAUGE_HISTOGRAM
metric: <
  histogram: <
    start_timestamp: < seconds: 1625851153 nanos: 146848499 >
    positive_span: < offset: 0 length: 0 >
  >
>
`,
		// Native histogram with exemplars on the native part (Exemplars field).
		`name: "test_histogram_with_native_histogram_exemplars"
help: "A histogram with native histogram exemplars."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count: 175
    sample_sum: 0.0008280461746287094
    bucket: <
      cumulative_count: 2
      upper_bound: -0.0004899999999999998
    >
    bucket: <
      cumulative_count: 4
      upper_bound: -0.0003899999999999998
      exemplar: <
        label: < name: "dummyID" value: "59727" >
        value: -0.00039
        timestamp: < seconds: 1625851155 nanos: 146848499 >
      >
    >
    bucket: <
      cumulative_count: 16
      upper_bound: -0.0002899999999999998
      exemplar: <
        label: < name: "dummyID" value: "5617" >
        value: -0.00029
      >
    >
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count: 2
    negative_span: < offset: -162 length: 1 >
    negative_span: < offset: 23 length: 4 >
    negative_delta: 1
    negative_delta: 3
    negative_delta: -2
    negative_delta: -1
    negative_delta: 1
    positive_span: < offset: -161 length: 1 >
    positive_span: < offset: 8 length: 3 >
    positive_delta: 1
    positive_delta: 2
    positive_delta: -1
    positive_delta: -1
    exemplars: <
      label: < name: "dummyID" value: "59780" >
      value: -0.00039
      timestamp: < seconds: 1625851155 nanos: 146848499 >
    >
    exemplars: <
      label: < name: "dummyID" value: "5617" >
      value: -0.00029
    >
    exemplars: <
      label: < name: "dummyID" value: "59772" >
      value: -0.00052
      timestamp: < seconds: 1625851160 nanos: 156848499 >
    >
  >
  timestamp_ms: 1234568
>
`,
		// Native histogram with a single exemplar on the native part, no bucket exemplars.
		`name: "test_histogram_with_native_histogram_exemplars2"
help: "Another histogram with native histogram exemplars."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count: 175
    sample_sum: 0.0008280461746287094
    bucket: < cumulative_count: 2 upper_bound: -0.0004899999999999998 >
    bucket: < cumulative_count: 4 upper_bound: -0.0003899999999999998 >
    bucket: < cumulative_count: 16 upper_bound: -0.0002899999999999998 >
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count: 2
    negative_span: < offset: -162 length: 1 >
    negative_span: < offset: 23 length: 4 >
    negative_delta: 1
    negative_delta: 3
    negative_delta: -2
    negative_delta: -1
    negative_delta: 1
    positive_span: < offset: -161 length: 1 >
    positive_span: < offset: 8 length: 3 >
    positive_delta: 1
    positive_delta: 2
    positive_delta: -1
    positive_delta: -1
    exemplars: <
      label: < name: "dummyID" value: "59780" >
      value: -0.00039
      timestamp: < seconds: 1625851155 nanos: 146848499 >
    >
  >
  timestamp_ms: 1234568
>
`,
	); err != nil {
		return nil, err
	}

	// --- NATIVE HISTOGRAM: STRUCTURAL VARIATIONS ---

	// Many spans: exercises span-merging and offset-adjustment code paths in compactBuckets.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "native_many_spans",
		Help: "Native histogram with many small spans.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount:   50,
					SampleSum:     25,
					Schema:        1,
					ZeroThreshold: 0.001,
					ZeroCount:     2,
					PositiveSpan: []dto.BucketSpan{
						{Offset: 0, Length: 1},
						{Offset: 1, Length: 1},
						{Offset: 1, Length: 1},
						{Offset: 1, Length: 1},
						{Offset: 1, Length: 1},
					},
					PositiveDelta: []int64{10, 5, 3, 2, 8},
					NegativeSpan: []dto.BucketSpan{
						{Offset: 0, Length: 1},
						{Offset: 2, Length: 1},
						{Offset: 2, Length: 1},
					},
					NegativeDelta: []int64{5, 4, 3},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Empty buckets in middle: exercises the "cut empty from middle of span" path.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "native_empty_middle",
		Help: "Native histogram with empty buckets in the middle of a span.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount:   20,
					SampleSum:     10,
					Schema:        2,
					ZeroThreshold: 0.001,
					ZeroCount:     0,
					PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 6}},
					PositiveDelta: []int64{5, -5, 0, 0, 3, 7},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// All-zero positive buckets: exercises the "cut all empty" code path.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "native_all_zero",
		Help: "Native histogram where all positive buckets are zero.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount:   5,
					SampleSum:     2,
					Schema:        1,
					ZeroThreshold: 0.001,
					ZeroCount:     5,
					PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 3}},
					PositiveDelta: []int64{0, 0, 0},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Negative-only: no positive spans, exercises the negative-side code path exclusively.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "native_neg_only",
		Help: "Native histogram with negative spans only.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount:   30,
					SampleSum:     -15,
					Schema:        1,
					ZeroThreshold: 0.001,
					ZeroCount:     2,
					NegativeSpan:  []dto.BucketSpan{{Offset: 0, Length: 2}, {Offset: 3, Length: 2}},
					NegativeDelta: []int64{10, 8, 5, 5},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Various schemas: -4 through 8.
	for _, schema := range []int32{-4, -3, -2, -1, 0, 1, 2, 4, 8} {
		if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
			Name: "native_schema_varied",
			Help: "Native histogram with varied schema values.",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{
				{
					Histogram: &dto.Histogram{
						SampleCount:   10,
						SampleSum:     5,
						Schema:        schema,
						ZeroThreshold: 0.001,
						ZeroCount:     1,
						PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 2}},
						PositiveDelta: []int64{5, 4},
					},
				},
			},
		}); err != nil {
			return nil, err
		}
	}

	// Float histogram with negative side only.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "float_neg_only",
		Help: "Float histogram with negative spans only.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCountFloat: 30,
					SampleSum:        -15,
					Schema:           1,
					ZeroThreshold:    0.001,
					ZeroCountFloat:   2,
					NegativeSpan:     []dto.BucketSpan{{Offset: 0, Length: 3}},
					NegativeCount:    []float64{10, 8, 10},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Float histogram with empty buckets: exercises compactBuckets float path.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "float_empty_buckets",
		Help: "Float histogram with some zero-count buckets.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCountFloat: 15,
					SampleSum:        7,
					Schema:           2,
					ZeroThreshold:    0.001,
					ZeroCountFloat:   1,
					PositiveSpan:     []dto.BucketSpan{{Offset: 0, Length: 5}},
					PositiveCount:    []float64{5, 0, 0, 3, 7},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Multiple metrics per family: exercises repeated histogram parsing within one family.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "native_multi_metric",
		Help: "Native histogram family with several label sets.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Label: []dto.LabelPair{{Name: "le", Value: "0.1"}},
				Histogram: &dto.Histogram{
					SampleCount:   100,
					SampleSum:     50,
					Schema:        1,
					ZeroThreshold: 0.001,
					ZeroCount:     5,
					PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 2}},
					PositiveDelta: []int64{40, 55},
				},
			},
			{
				Label: []dto.LabelPair{{Name: "le", Value: "1.0"}},
				Histogram: &dto.Histogram{
					SampleCount:   200,
					SampleSum:     100,
					Schema:        1,
					ZeroThreshold: 0.001,
					ZeroCount:     10,
					PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 3}},
					PositiveDelta: []int64{80, 60, 50},
				},
			},
			{
				Label: []dto.LabelPair{{Name: "le", Value: "10.0"}},
				Histogram: &dto.Histogram{
					SampleCount:   300,
					SampleSum:     150,
					Schema:        1,
					ZeroThreshold: 0.001,
					ZeroCount:     15,
					PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 2}, {Offset: 2, Length: 2}},
					PositiveDelta: []int64{100, 80, 60, 45},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Spans with zero offset between them: exercises zero-offset merging path.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "native_zero_offsets",
		Help: "Native histogram with zero offsets between spans.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount:   30,
					SampleSum:     15,
					Schema:        1,
					ZeroThreshold: 0.001,
					ZeroCount:     0,
					PositiveSpan: []dto.BucketSpan{
						{Offset: 2, Length: 2},
						{Offset: 0, Length: 2},
						{Offset: 0, Length: 1},
					},
					PositiveDelta: []int64{10, 5, 3, 7, 5},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// --- NHCB CONVERSION TARGETS ---
	// Classic histograms with varied bucket patterns specifically designed to
	// exercise the convertToNHCB() path (schema -53, custom buckets). These are
	// most useful when paired with ConvertNHCB=true in GetCorpusForFuzzParseProtobuf.

	// Classic histogram: many buckets, varied upper bounds.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "nhcb_many_buckets",
		Help: "Classic histogram with many buckets for NHCB conversion.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount: 1000,
					SampleSum:   500,
					Bucket: []dto.Bucket{
						{CumulativeCount: 10, UpperBound: 0.001},
						{CumulativeCount: 50, UpperBound: 0.01},
						{CumulativeCount: 150, UpperBound: 0.1},
						{CumulativeCount: 300, UpperBound: 0.5},
						{CumulativeCount: 600, UpperBound: 1.0},
						{CumulativeCount: 800, UpperBound: 5.0},
						{CumulativeCount: 950, UpperBound: 10.0},
						{CumulativeCount: 1000, UpperBound: math.Inf(1)},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Classic histogram: single bucket (minimal NHCB conversion).
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "nhcb_single_bucket",
		Help: "Classic histogram with a single bucket.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount: 10,
					SampleSum:   5,
					Bucket:      []dto.Bucket{{CumulativeCount: 10, UpperBound: 1.0}},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Classic histogram: buckets with +Inf explicit upper bound.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "nhcb_explicit_inf",
		Help: "Classic histogram with explicit +Inf bucket.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount: 100,
					SampleSum:   50,
					Bucket: []dto.Bucket{
						{CumulativeCount: 40, UpperBound: 1.0},
						{CumulativeCount: 80, UpperBound: 10.0},
						{CumulativeCount: 100, UpperBound: math.Inf(1)},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Classic gauge histogram: exercises NHCB conversion for gauge type.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "nhcb_gauge",
		Help: "Classic gauge histogram for NHCB conversion.",
		Type: dto.MetricType_GAUGE_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount: 50,
					SampleSum:   25,
					Bucket: []dto.Bucket{
						{CumulativeCount: 20, UpperBound: 1.0},
						{CumulativeCount: 40, UpperBound: 5.0},
						{CumulativeCount: 50, UpperBound: math.Inf(1)},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Classic histogram: multiple metrics in one family (NHCB with label sets).
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "nhcb_multi_label",
		Help: "Classic histogram family with multiple label sets.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Label: []dto.LabelPair{{Name: "handler", Value: "/api/v1/query"}},
				Histogram: &dto.Histogram{
					SampleCount: 200,
					SampleSum:   100,
					Bucket: []dto.Bucket{
						{CumulativeCount: 100, UpperBound: 0.1},
						{CumulativeCount: 180, UpperBound: 1.0},
						{CumulativeCount: 200, UpperBound: math.Inf(1)},
					},
				},
			},
			{
				Label: []dto.LabelPair{{Name: "handler", Value: "/api/v1/series"}},
				Histogram: &dto.Histogram{
					SampleCount: 50,
					SampleSum:   25,
					Bucket: []dto.Bucket{
						{CumulativeCount: 30, UpperBound: 0.1},
						{CumulativeCount: 45, UpperBound: 1.0},
						{CumulativeCount: 50, UpperBound: math.Inf(1)},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// --- NHCB CONVERSION ERROR PATHS ---
	// These seeds trigger error return paths inside convertToNHCB() and the
	// underlying convertnhcb.TempHistogram validator. Seeding them directly
	// ensures the fuzzer reaches the error-handling branches without having
	// to discover them by random byte mutation.

	// Classic histogram with a negative float sample count: triggers errNegativeCount
	// in TempHistogram.SetCount.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "nhcb_negative_count",
		Help: "Classic histogram with a negative float sample count.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCountFloat: -5.0,
					SampleSum:        10,
					Bucket: []dto.Bucket{
						{CumulativeCount: 3, UpperBound: 1.0},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Classic histogram with a non-cumulative bucket sequence: triggers errCountNotCumulative
	// in TempHistogram.SetBucketCount (second bucket count is less than the first).
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "nhcb_decreasing_buckets",
		Help: "Classic histogram where bucket cumulative counts decrease.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount: 100,
					SampleSum:   50,
					Bucket: []dto.Bucket{
						{CumulativeCountFloat: 80, UpperBound: 1.0},
						{CumulativeCountFloat: 40, UpperBound: 5.0}, // Decreasing — invalid.
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Classic histogram where sample count does not match the highest bucket:
	// triggers errCountMismatch in convertToIntegerHistogram.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "nhcb_count_mismatch",
		Help: "Classic histogram where sample count mismatches the highest bucket.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount: 200, // Does not match the 100 in the highest bucket.
					SampleSum:   50,
					Bucket: []dto.Bucket{
						{CumulativeCount: 50, UpperBound: 1.0},
						{CumulativeCount: 100, UpperBound: math.Inf(1)},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// --- EXEMPLAR EDGE CASES ---

	// Native histogram with an exemplar that has no timestamp: exercises the
	// early-exit path in Exemplar() for native histograms with timestamp-less
	// exemplars in the Exemplars field.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "native_exemplar_no_ts",
		Help: "Native histogram with an exemplar missing a timestamp.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount:   50,
					SampleSum:     25,
					Schema:        1,
					ZeroThreshold: 0.001,
					ZeroCount:     2,
					PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 2}},
					PositiveDelta: []int64{20, 28},
					Exemplars: []*dto.Exemplar{
						// No Timestamp field — exercises the nil-timestamp path.
						{
							Label: []dto.LabelPair{{Name: "trace_id", Value: "no-ts-exemplar"}},
							Value: 1.5,
						},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// Native histogram with a mix of timestamped and timestamp-less exemplars:
	// exercises the inner loop that skips exemplars without timestamps.
	if result, err = appendSerializedProtobufSeed(result, dto.MetricFamily{
		Name: "native_exemplar_mixed_ts",
		Help: "Native histogram exemplars with mixed timestamp presence.",
		Type: dto.MetricType_HISTOGRAM,
		Metric: []dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount:   100,
					SampleSum:     50,
					Schema:        1,
					ZeroThreshold: 0.001,
					ZeroCount:     5,
					PositiveSpan:  []dto.BucketSpan{{Offset: 0, Length: 2}},
					PositiveDelta: []int64{40, 55},
					Exemplars: []*dto.Exemplar{
						// No timestamp — skipped by the Exemplar() loop.
						{Label: []dto.LabelPair{{Name: "x", Value: "1"}}, Value: 0.1},
						// Has timestamp — returned.
						{
							Label:     []dto.LabelPair{{Name: "x", Value: "2"}},
							Value:     0.5,
							Timestamp: &types.Timestamp{Seconds: 1625851200},
						},
					},
				},
			},
		},
	}); err != nil {
		return nil, err
	}

	// --- STRUCTURALLY INVALID / CORRUPT INPUTS ---
	// The fuzzer mutates bytes freely, but seeding with known-bad structures
	// helps it reach corrupt-input handling paths faster.

	corruptSeeds, err := protobufCorruptSeeds()
	if err != nil {
		return nil, err
	}
	result = append(result, corruptSeeds...)

	return result, nil
}
