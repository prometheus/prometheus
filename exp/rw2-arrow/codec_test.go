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

package rw2arrow

import (
	"bytes"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestRoundTripCodecs(t *testing.T) {
	batch := GenerateBatch(50, 3) // 150 samples
	expectedSamples := 150

	t.Run("RW2", func(t *testing.T) {
		raw, err := EncodeRW2(batch, nil)
		require.NoError(t, err)
		n, err := DecodeRW2(raw)
		require.NoError(t, err)
		require.Equal(t, expectedSamples, n)

		sn := compressSnappy(nil, raw)
		dec, err := decompressSnappy(nil, sn)
		require.NoError(t, err)
		n, err = DecodeRW2(dec)
		require.NoError(t, err)
		require.Equal(t, expectedSamples, n)

		zs := compressZstd(nil, raw)
		dec, err = decompressZstd(nil, zs)
		require.NoError(t, err)
		n, err = DecodeRW2(dec)
		require.NoError(t, err)
		require.Equal(t, expectedSamples, n)
	})

	t.Run("ArrowColumnarStateless", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, EncodeArrowColumnarStateless(batch, &buf))
		n, err := DecodeArrowColumnar(buf.Bytes())
		require.NoError(t, err)
		require.Equal(t, expectedSamples, n)
	})

	t.Run("ArrowColumnarPooled", func(t *testing.T) {
		enc := NewPooledArrowColumnarEncoder(collectSortedLabelNames(batch))
		defer enc.Release()

		var buf bytes.Buffer
		for range 3 {
			require.NoError(t, enc.Encode(batch, &buf))
			n, err := DecodeArrowColumnar(buf.Bytes())
			require.NoError(t, err)
			require.Equal(t, expectedSamples, n)
		}
	})

	t.Run("ArrowListStateless_Heterogeneous", func(t *testing.T) {
		heterogeneous := append(GenerateBatch(20, 2), SeriesData{
			Labels:  labels.FromStrings("__name__", "custom_gauge", "region", "us-east-1"),
			Samples: []SamplePoint{{Timestamp: 1700000000000, Value: 42.5}},
		})
		var buf bytes.Buffer
		require.NoError(t, EncodeArrowListStateless(heterogeneous, &buf))
		n, err := DecodeArrowList(buf.Bytes())
		require.NoError(t, err)
		require.Equal(t, 41, n)
	})

	t.Run("ArrowStatefulStream", func(t *testing.T) {
		stream, err := NewStatefulArrowStream(collectSortedLabelNames(batch))
		require.NoError(t, err)
		defer func() { _ = stream.Close() }()

		var fullWire bytes.Buffer
		for range 3 {
			chunk, err := stream.WriteBatch(batch)
			require.NoError(t, err)
			fullWire.Write(chunk)
		}
		n, err := DecodeArrowColumnar(fullWire.Bytes())
		require.NoError(t, err)
		require.Equal(t, expectedSamples*3, n)
	})
}

func TestWireSizeComparison(t *testing.T) {
	cases := []struct {
		name             string
		numSeries        int
		samplesPerSeries int
	}{
		{"200x1 (200 samples)", 200, 1},
		{"500x10 (5000 samples)", 500, 10},
		{"5000x1 (5000 samples)", 5000, 1},
	}

	for _, tc := range cases {
		batch := GenerateBatch(tc.numSeries, tc.samplesPerSeries)

		rw2Raw, err := EncodeRW2(batch, nil)
		require.NoError(t, err)
		rw2Snappy := compressSnappy(nil, rw2Raw)
		rw2Zstd := compressZstd(nil, rw2Raw)

		var arrowColRaw, arrowColZstd, arrowListRaw, arrowListZstd bytes.Buffer
		require.NoError(t, EncodeArrowColumnarStateless(batch, &arrowColRaw))
		require.NoError(t, EncodeArrowColumnarStateless(batch, &arrowColZstd, ipc.WithZstd()))
		require.NoError(t, EncodeArrowListStateless(batch, &arrowListRaw))
		require.NoError(t, EncodeArrowListStateless(batch, &arrowListZstd, ipc.WithZstd()))

		stream, err := NewStatefulArrowStream(collectSortedLabelNames(batch))
		require.NoError(t, err)
		_, err = stream.WriteBatch(batch) // first batch includes schema
		require.NoError(t, err)
		statefulBatch2, err := stream.WriteBatch(batch) // steady-state batch without schema
		require.NoError(t, err)
		statefulBatch2Len := len(statefulBatch2)
		_ = stream.Close()

		t.Logf("%-22s | RW2 raw=%6d snappy=%6d zstd=%6d | ArrowCol raw=%6d zstd=%6d stateful=%6d | ArrowList raw=%6d zstd=%6d",
			tc.name,
			len(rw2Raw), len(rw2Snappy), len(rw2Zstd),
			arrowColRaw.Len(), arrowColZstd.Len(), statefulBatch2Len,
			arrowListRaw.Len(), arrowListZstd.Len(),
		)
	}
}

func benchmarkEncodeDecode(b *testing.B, numSeries, samplesPerSeries int) {
	batch := GenerateBatch(numSeries, samplesPerSeries)
	labelNames := collectSortedLabelNames(batch)

	rw2Raw, err := EncodeRW2(batch, nil)
	require.NoError(b, err)
	rw2Snappy := compressSnappy(nil, rw2Raw)
	rw2Zstd := compressZstd(nil, rw2Raw)

	var arrowColRaw, arrowColZstd, arrowListRaw, arrowListZstd bytes.Buffer
	require.NoError(b, EncodeArrowColumnarStateless(batch, &arrowColRaw))
	require.NoError(b, EncodeArrowColumnarStateless(batch, &arrowColZstd, ipc.WithZstd()))
	require.NoError(b, EncodeArrowListStateless(batch, &arrowListRaw))
	require.NoError(b, EncodeArrowListStateless(batch, &arrowListZstd, ipc.WithZstd()))

	b.Run("RW2_Encode_Raw", func(b *testing.B) {
		var dst []byte
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			var err error
			dst, err = EncodeRW2(batch, dst[:0])
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("RW2_Encode_Snappy", func(b *testing.B) {
		var dst, comp []byte
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			var err error
			dst, err = EncodeRW2(batch, dst[:0])
			if err != nil {
				b.Fatal(err)
			}
			comp = compressSnappy(comp[:0], dst)
		}
	})

	b.Run("RW2_Encode_Zstd", func(b *testing.B) {
		var dst, comp []byte
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			var err error
			dst, err = EncodeRW2(batch, dst[:0])
			if err != nil {
				b.Fatal(err)
			}
			comp = compressZstd(comp[:0], dst)
		}
	})

	b.Run("RW2_Decode_Snappy", func(b *testing.B) {
		var dec []byte
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			var err error
			dec, err = decompressSnappy(dec[:0], rw2Snappy)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := DecodeRW2(dec); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("RW2_Decode_Zstd", func(b *testing.B) {
		var dec []byte
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			var err error
			dec, err = decompressZstd(dec[:0], rw2Zstd)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := DecodeRW2(dec); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ArrowCol_Encode_Stateless_Raw", func(b *testing.B) {
		var buf bytes.Buffer
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if err := EncodeArrowColumnarStateless(batch, &buf); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ArrowCol_Encode_Pooled_Raw", func(b *testing.B) {
		enc := NewPooledArrowColumnarEncoder(labelNames)
		defer enc.Release()
		var buf bytes.Buffer
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if err := enc.Encode(batch, &buf); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ArrowCol_Encode_Pooled_Zstd", func(b *testing.B) {
		enc := NewPooledArrowColumnarEncoder(labelNames)
		defer enc.Release()
		var buf bytes.Buffer
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if err := enc.Encode(batch, &buf, ipc.WithZstd()); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ArrowCol_Encode_Stateful_Raw", func(b *testing.B) {
		stream, err := NewStatefulArrowStream(labelNames)
		if err != nil {
			b.Fatal(err)
		}
		defer func() { _ = stream.Close() }()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := stream.WriteBatch(batch); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ArrowCol_Decode_Raw", func(b *testing.B) {
		raw := arrowColRaw.Bytes()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := DecodeArrowColumnar(raw); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ArrowCol_Decode_Zstd", func(b *testing.B) {
		zs := arrowColZstd.Bytes()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := DecodeArrowColumnar(zs); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ArrowList_Encode_Stateless_Raw", func(b *testing.B) {
		var buf bytes.Buffer
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if err := EncodeArrowListStateless(batch, &buf); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ArrowList_Decode_Raw", func(b *testing.B) {
		raw := arrowListRaw.Bytes()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := DecodeArrowList(raw); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ArrowList_Decode_Zstd", func(b *testing.B) {
		zs := arrowListZstd.Bytes()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := DecodeArrowList(zs); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkSmall200(b *testing.B) {
	benchmarkEncodeDecode(b, 200, 1)
}

func BenchmarkMed500x10(b *testing.B) {
	benchmarkEncodeDecode(b, 500, 10)
}

func BenchmarkLarge5000x1(b *testing.B) {
	benchmarkEncodeDecode(b, 5000, 1)
}
