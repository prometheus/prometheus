// This micro-benchmark is intentionally written against ONLY the gogo-compatible
// generated API surface (value-typed repeated fields + Size/Marshal/
// MarshalToSizedBuffer/Unmarshal). It therefore compiles UNCHANGED against both
// the gogo baseline (main) and the wiresmith branch, so the two can be compared
// directly with benchstat. It deliberately avoids oneof fields (Histogram
// count/zero_count) and pointer fields, whose Go shape may differ between the two
// codegens.
package remote

import (
	"testing"

	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

// buildMicroV1Request builds a representative Remote-Write 1.0 WriteRequest from
// value-typed repeated fields only (Labels/Samples/Exemplars/Histograms).
func buildMicroV1Request(nSeries int) *prompb.WriteRequest {
	req := &prompb.WriteRequest{Timeseries: make([]prompb.TimeSeries, nSeries)}
	for i := range req.Timeseries {
		lbls := []prompb.Label{
			{Name: "__name__", Value: "http_request_duration_seconds_bucket"},
			{Name: "cluster", Value: "some-cluster-0"},
			{Name: "container", Value: "prometheus"},
			{Name: "instance", Value: "10.0.0.1:9090"},
			{Name: "job", Value: "some-namespace/prometheus"},
			{Name: "le", Value: "0.25"},
			{Name: "namespace", Value: "some-namespace"},
			{Name: "pod", Value: "prometheus-0"},
		}
		ts := prompb.TimeSeries{
			Labels:  lbls,
			Samples: []prompb.Sample{{Value: float64(i) * 1.5, Timestamp: 1700000000000 + int64(i)}},
			Exemplars: []prompb.Exemplar{{
				Labels:    []prompb.Label{{Name: "trace_id", Value: "abcdef0123456789"}},
				Value:     float64(i),
				Timestamp: 1700000000000 + int64(i),
			}},
		}
		// Every 4th series carries a native histogram, populated with
		// NON-oneof fields only (Count/ZeroCount oneofs are intentionally omitted
		// to keep this source portable across gogo and wiresmith).
		if i%4 == 0 {
			ts.Histograms = []prompb.Histogram{{
				Sum:            float64(i) * 3.0,
				Schema:         2,
				ZeroThreshold:  1e-128,
				PositiveSpans:  []prompb.BucketSpan{{Offset: 0, Length: 2}, {Offset: 1, Length: 2}},
				PositiveDeltas: []int64{1, 2, -1, 3},
				NegativeSpans:  []prompb.BucketSpan{{Offset: 0, Length: 1}},
				NegativeDeltas: []int64{2},
				Timestamp:      1700000000000 + int64(i),
			}}
		}
		req.Timeseries[i] = ts
	}
	return req
}

// buildMicroV2Request builds a representative Remote-Write 2.0 Request using the
// symbol table + LabelsRefs shape, value-typed samples only (no histogram oneofs).
func buildMicroV2Request(nSeries int) *writev2.Request {
	symbols := []string{
		"", "__name__", "http_request_duration_seconds_bucket",
		"cluster", "some-cluster-0", "container", "prometheus",
		"instance", "10.0.0.1:9090", "job", "some-namespace/prometheus",
		"le", "0.25", "namespace", "some-namespace", "pod", "prometheus-0",
	}
	req := &writev2.Request{Symbols: symbols, Timeseries: make([]writev2.TimeSeries, nSeries)}
	for i := range req.Timeseries {
		req.Timeseries[i] = writev2.TimeSeries{
			LabelsRefs: []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			Samples:    []writev2.Sample{{Value: float64(i) * 1.5, Timestamp: 1700000000000 + int64(i)}},
		}
	}
	return req
}

var microSeriesCounts = []struct {
	name string
	n    int
}{
	{"100series", 100},
	{"1000series", 1000},
}

func BenchmarkWiresmithV1Size(b *testing.B) {
	for _, tc := range microSeriesCounts {
		req := buildMicroV1Request(tc.n)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			var total int
			for b.Loop() {
				total += req.Size()
			}
			_ = total
		})
	}
}

func BenchmarkWiresmithV1Marshal(b *testing.B) {
	for _, tc := range microSeriesCounts {
		req := buildMicroV1Request(tc.n)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := req.Marshal(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkWiresmithV1MarshalReuse exercises the alloc-free hot path: Size() +
// MarshalToSizedBuffer into a single reused buffer (this mirrors what the
// migrated queue_manager buildWriteRequest should do).
func BenchmarkWiresmithV1MarshalReuse(b *testing.B) {
	for _, tc := range microSeriesCounts {
		req := buildMicroV1Request(tc.n)
		buf := make([]byte, 0, req.Size())
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sz := req.Size()
				if cap(buf) < sz {
					buf = make([]byte, sz)
				}
				buf = buf[:sz]
				if _, err := req.MarshalToSizedBuffer(buf); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkWiresmithV1Unmarshal(b *testing.B) {
	for _, tc := range microSeriesCounts {
		req := buildMicroV1Request(tc.n)
		data, err := req.Marshal()
		if err != nil {
			b.Fatal(err)
		}
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var out prompb.WriteRequest
				if err := out.Unmarshal(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkWiresmithV2Size(b *testing.B) {
	for _, tc := range microSeriesCounts {
		req := buildMicroV2Request(tc.n)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			var total int
			for b.Loop() {
				total += req.Size()
			}
			_ = total
		})
	}
}

func BenchmarkWiresmithV2Marshal(b *testing.B) {
	for _, tc := range microSeriesCounts {
		req := buildMicroV2Request(tc.n)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := req.Marshal(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkWiresmithV2Unmarshal(b *testing.B) {
	for _, tc := range microSeriesCounts {
		req := buildMicroV2Request(tc.n)
		data, err := req.Marshal()
		if err != nil {
			b.Fatal(err)
		}
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var out writev2.Request
				if err := out.Unmarshal(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
