// Copyright 2025 The Prometheus Authors
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

package record_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/compression"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/util/testrecord"
)

func TestEncodeDecode(t *testing.T) {
	for _, tcase := range []testrecord.RefSamplesCase{
		testrecord.Realistic1000Samples,
		testrecord.Realistic1000WithCTSamples,
		testrecord.WorstCase1000Samples,
	} {
		var (
			enc record.Encoder
			dec record.Decoder
			buf []byte
		)

		s := testrecord.GenTestRefSamplesCase(t, tcase)

		{
			got, err := dec.Samples(enc.Samples(s, nil), nil)
			require.NoError(t, err)
			require.Equal(t, s, got)
		}

		//  With byte buffer (append!)
		{
			buf = make([]byte, 10, 1e5)
			got, err := dec.Samples(enc.Samples(s, buf)[10:], nil)
			require.NoError(t, err)
			require.Equal(t, s, got)
		}

		// With sample slice
		{
			samples := make([]record.RefSample, 0, len(s)+1)
			got, err := dec.Samples(enc.Samples(s, nil), samples)
			require.NoError(t, err)
			require.Equal(t, s, got)
		}

		// With compression.
		{
			buf := enc.Samples(s, nil)

			cEnc, err := compression.NewEncoder()
			require.NoError(t, err)
			buf, _, err = cEnc.Encode(compression.Zstd, buf, nil)
			require.NoError(t, err)

			buf, err = compression.NewDecoder().Decode(compression.Zstd, buf, nil)
			require.NoError(t, err)

			got, err := dec.Samples(buf, nil)
			require.NoError(t, err)
			require.Equal(t, s, got)
		}
	}
}

var (
	compressions = []compression.Type{compression.None, compression.Snappy, compression.Zstd}
	dataCases    = []testrecord.RefSamplesCase{
		testrecord.Realistic1000Samples,
		testrecord.Realistic1000WithCTSamples,
		testrecord.WorstCase1000Samples,
	}
)

/*
	export bench=encode-v1 && go test ./tsdb/record/... \
		-run '^$' -bench '^BenchmarkEncode_Samples' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee ${bench}.txt
*/
func BenchmarkEncode_Samples(b *testing.B) {
	for _, compr := range compressions {
		for _, data := range dataCases {
			b.Run(fmt.Sprintf("compr=%v/data=%v", compr, data), func(b *testing.B) {
				var (
					samples = testrecord.GenTestRefSamplesCase(b, data)
					enc     record.Encoder
					buf     []byte
					cBuf    []byte
				)

				cEnc, err := compression.NewEncoder()
				require.NoError(b, err)

				// Warm up.
				buf = enc.Samples(samples, buf[:0])
				cBuf, _, err = cEnc.Encode(compr, buf, cBuf[:0])
				require.NoError(b, err)

				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					buf = enc.Samples(samples, buf[:0])
					b.ReportMetric(float64(len(buf)), "B/rec")

					cBuf, _, _ = cEnc.Encode(compr, buf, cBuf[:0])
					b.ReportMetric(float64(len(cBuf)), "B/compressed-rec")
				}
			})
		}
	}
}

/*
	export bench=decode-v1 && go test ./tsdb/record/... \
		-run '^$' -bench '^BenchmarkDecode_Samples' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee ${bench}.txt
*/
func BenchmarkDecode_Samples(b *testing.B) {
	for _, compr := range compressions {
		for _, data := range dataCases {
			b.Run(fmt.Sprintf("compr=%v/data=%v", compr, data), func(b *testing.B) {
				var (
					samples    = testrecord.GenTestRefSamplesCase(b, data)
					enc        record.Encoder
					dec        record.Decoder
					cDec       = compression.NewDecoder()
					cBuf       []byte
					samplesBuf []record.RefSample
				)

				buf := enc.Samples(samples, nil)

				cEnc, err := compression.NewEncoder()
				require.NoError(b, err)

				buf, _, err = cEnc.Encode(compr, buf, nil)
				require.NoError(b, err)

				// Warm up.
				cBuf, err = cDec.Decode(compr, buf, cBuf[:0])
				require.NoError(b, err)
				samplesBuf, err = dec.Samples(cBuf, samplesBuf[:0])
				require.NoError(b, err)

				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					cBuf, _ = cDec.Decode(compr, buf, cBuf[:0])
					samplesBuf, _ = dec.Samples(cBuf, samplesBuf[:0])
				}
			})
		}
	}
}
