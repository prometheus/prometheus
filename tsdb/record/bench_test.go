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

package record_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/compression"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/util/testrecord"
)

func zeroOutSTs(samples []record.RefSample) []record.RefSample {
	out := make([]record.RefSample, len(samples))
	for i := range samples {
		out[i] = samples[i]
		out[i].ST = 0
	}
	return out
}

func TestEncodeDecode(t *testing.T) {
	for _, enableSTStorage := range []bool{false, true} {
		for _, tcase := range []testrecord.RefSamplesCase{
			testrecord.Realistic1000Samples,
			testrecord.Realistic1000WithVariableSTSamples,
			testrecord.Realistic1000WithConstSTSamples,
			testrecord.WorstCase1000,
			testrecord.WorstCase1000WithSTSamples,
		} {
			var (
				dec record.Decoder
				buf []byte
				enc = record.Encoder{EnableSTStorage: enableSTStorage}
			)

			s := testrecord.GenTestRefSamplesCase(t, tcase)

			{
				got, err := dec.Samples(enc.Samples(s, nil), nil)
				require.NoError(t, err)
				// if ST is off, we expect all STs to be zero
				expected := s
				if !enableSTStorage {
					expected = zeroOutSTs(s)
				}

				require.Equal(t, expected, got)
			}

			//  With byte buffer (append!)
			{
				buf = make([]byte, 10, 1e5)
				got, err := dec.Samples(enc.Samples(s, buf)[10:], nil)
				require.NoError(t, err)

				expected := s
				if !enableSTStorage {
					expected = zeroOutSTs(s)
				}
				require.Equal(t, expected, got)
			}

			// With sample slice
			{
				samples := make([]record.RefSample, 0, len(s)+1)
				got, err := dec.Samples(enc.Samples(s, nil), samples)
				require.NoError(t, err)
				expected := s
				if !enableSTStorage {
					expected = zeroOutSTs(s)
				}
				require.Equal(t, expected, got)
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
				expected := s
				if !enableSTStorage {
					expected = zeroOutSTs(s)
				}
				require.Equal(t, expected, got)
			}
		}
	}
}

var (
	compressions = []compression.Type{compression.None, compression.Snappy, compression.Zstd}
	dataCases    = []testrecord.RefSamplesCase{
		testrecord.Realistic1000Samples,
		testrecord.Realistic1000WithVariableSTSamples,
		testrecord.Realistic1000WithConstSTSamples,
		testrecord.WorstCase1000,
		testrecord.WorstCase1000WithSTSamples,
	}
	versions = []struct {
		name     string
		enableST bool
	}{
		{"V1", false},
		{"V2", true},
	}
)

//nolint:godot
/*
	go test ./tsdb/record/... \
		-run '^$' -bench '^BenchmarkEncode_Samples' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee encode.txt

benchstat -col /version encode.txt
*/
func BenchmarkEncode_Samples(b *testing.B) {
	for _, ver := range versions {
		for _, compr := range compressions {
			for _, data := range dataCases {
				b.Run(fmt.Sprintf("version=%s/compr=%v/data=%v", ver.name, compr, data), func(b *testing.B) {
					var (
						samples = testrecord.GenTestRefSamplesCase(b, data)
						enc     = record.Encoder{EnableSTStorage: ver.enableST}
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
					for b.Loop() {
						buf = enc.Samples(samples, buf[:0])
						b.ReportMetric(float64(len(buf)), "B/rec")

						cBuf, _, _ = cEnc.Encode(compr, buf, cBuf[:0])
						b.ReportMetric(float64(len(cBuf)), "B/compressed-rec")
					}
				})
			}
		}
	}
}

//nolint:godot
/*
	go test ./tsdb/record/... \
		-run '^$' -bench '^BenchmarkDecode_Samples' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee decode.txt

benchstat -col /version decode.txt
*/
func BenchmarkDecode_Samples(b *testing.B) {
	for _, ver := range versions {
		for _, compr := range compressions {
			for _, data := range dataCases {
				b.Run(fmt.Sprintf("version=%s/compr=%v/data=%v", ver.name, compr, data), func(b *testing.B) {
					var (
						samples    = testrecord.GenTestRefSamplesCase(b, data)
						enc        = record.Encoder{EnableSTStorage: ver.enableST}
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
					for b.Loop() {
						cBuf, _ = cDec.Decode(compr, buf, cBuf[:0])
						samplesBuf, _ = dec.Samples(cBuf, samplesBuf[:0])
					}
				})
			}
		}
	}
}

var (
	histDataCases = testrecord.HistDataCases
	histCounts    = testrecord.HistCounts
)

//nolint:godot
/*
	go test ./tsdb/record/... \
		-run '^$' -bench '^BenchmarkEncode_Histograms' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee encode-hist.txt

benchstat -col /version encode-hist.txt
*/
func BenchmarkEncode_Histograms(b *testing.B) {
	for _, ver := range versions {
		for _, compr := range compressions {
			for _, hcase := range histDataCases {
				for _, stCase := range testrecord.HistSTCases {
					for _, count := range histCounts {
						b.Run(fmt.Sprintf("version=%s/compr=%v/type=%s/st=%s/n=%d", ver.name, compr, hcase.Name, stCase, count), func(b *testing.B) {
							var (
								samples = hcase.Gen(count, stCase)
								enc     = record.Encoder{EnableSTStorage: ver.enableST}
								buf     []byte
								cBuf    []byte
							)

							cEnc, err := compression.NewEncoder()
							require.NoError(b, err)

							// Warm up.
							if hcase.Name == "nhcb" {
								buf = enc.CustomBucketsHistogramSamples(samples, buf[:0])
							} else {
								buf, _ = enc.HistogramSamples(samples, buf[:0])
							}
							cBuf, _, err = cEnc.Encode(compr, buf, cBuf[:0])
							require.NoError(b, err)

							b.ReportAllocs()
							b.ResetTimer()
							for b.Loop() {
								if hcase.Name == "nhcb" {
									buf = enc.CustomBucketsHistogramSamples(samples, buf[:0])
								} else {
									buf, _ = enc.HistogramSamples(samples, buf[:0])
								}
								b.ReportMetric(float64(len(buf)), "B/rec")

								cBuf, _, _ = cEnc.Encode(compr, buf, cBuf[:0])
								b.ReportMetric(float64(len(cBuf)), "B/compressed-rec")
							}
						})
					}
				}
			}
		}
	}
}

//nolint:godot
/*
	go test ./tsdb/record/... \
		-run '^$' -bench '^BenchmarkDecode_Histograms' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee decode-hist.txt

benchstat -col /version decode-hist.txt
*/
func BenchmarkDecode_Histograms(b *testing.B) {
	for _, ver := range versions {
		for _, compr := range compressions {
			for _, hcase := range histDataCases {
				for _, stCase := range testrecord.HistSTCases {
					for _, count := range histCounts {
						b.Run(fmt.Sprintf("version=%s/compr=%v/type=%s/st=%s/n=%d", ver.name, compr, hcase.Name, stCase, count), func(b *testing.B) {
							var (
								samples    = hcase.Gen(count, stCase)
								enc        = record.Encoder{EnableSTStorage: ver.enableST}
								dec        record.Decoder
								cDec       = compression.NewDecoder()
								cBuf       []byte
								samplesBuf []record.RefHistogramSample
							)

							var buf []byte
							if hcase.Name == "nhcb" {
								buf = enc.CustomBucketsHistogramSamples(samples, nil)
							} else {
								buf, _ = enc.HistogramSamples(samples, nil)
							}

							cEnc, err := compression.NewEncoder()
							require.NoError(b, err)
							buf, _, err = cEnc.Encode(compr, buf, nil)
							require.NoError(b, err)

							// Warm up.
							cBuf, err = cDec.Decode(compr, buf, cBuf[:0])
							require.NoError(b, err)
							samplesBuf, err = dec.HistogramSamples(cBuf, samplesBuf[:0])
							require.NoError(b, err)

							b.ReportAllocs()
							b.ResetTimer()
							for b.Loop() {
								cBuf, _ = cDec.Decode(compr, buf, cBuf[:0])
								samplesBuf, _ = dec.HistogramSamples(cBuf, samplesBuf[:0])
							}
						})
					}
				}
			}
		}
	}
}

//nolint:godot
/*
	go test ./tsdb/record/... \
		-run '^$' -bench '^BenchmarkEncode_FloatHistograms' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee encode-fhist.txt

benchstat -col /version encode-fhist.txt
*/
func BenchmarkEncode_FloatHistograms(b *testing.B) {
	for _, ver := range versions {
		for _, compr := range compressions {
			for _, hcase := range histDataCases {
				for _, stCase := range testrecord.HistSTCases {
					for _, count := range histCounts {
						b.Run(fmt.Sprintf("version=%s/compr=%v/type=%s/st=%s/n=%d", ver.name, compr, hcase.Name, stCase, count), func(b *testing.B) {
							var (
								samples = testrecord.GenFloatHistograms(hcase.Gen(count, stCase))
								enc     = record.Encoder{EnableSTStorage: ver.enableST}
								buf     []byte
								cBuf    []byte
							)

							cEnc, err := compression.NewEncoder()
							require.NoError(b, err)

							// Warm up.
							if hcase.Name == "nhcb" {
								buf = enc.CustomBucketsFloatHistogramSamples(samples, buf[:0])
							} else {
								buf, _ = enc.FloatHistogramSamples(samples, buf[:0])
							}
							cBuf, _, err = cEnc.Encode(compr, buf, cBuf[:0])
							require.NoError(b, err)

							b.ReportAllocs()
							b.ResetTimer()
							for b.Loop() {
								if hcase.Name == "nhcb" {
									buf = enc.CustomBucketsFloatHistogramSamples(samples, buf[:0])
								} else {
									buf, _ = enc.FloatHistogramSamples(samples, buf[:0])
								}
								b.ReportMetric(float64(len(buf)), "B/rec")

								cBuf, _, _ = cEnc.Encode(compr, buf, cBuf[:0])
								b.ReportMetric(float64(len(cBuf)), "B/compressed-rec")
							}
						})
					}
				}
			}
		}
	}
}

//nolint:godot
/*
	go test ./tsdb/record/... \
		-run '^$' -bench '^BenchmarkDecode_FloatHistograms' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee decode-fhist.txt

benchstat -col /version decode-fhist.txt
*/
func BenchmarkDecode_FloatHistograms(b *testing.B) {
	for _, ver := range versions {
		for _, compr := range compressions {
			for _, hcase := range histDataCases {
				for _, stCase := range testrecord.HistSTCases {
					for _, count := range histCounts {
						b.Run(fmt.Sprintf("version=%s/compr=%v/type=%s/st=%s/n=%d", ver.name, compr, hcase.Name, stCase, count), func(b *testing.B) {
							var (
								samples    = testrecord.GenFloatHistograms(hcase.Gen(count, stCase))
								enc        = record.Encoder{EnableSTStorage: ver.enableST}
								dec        record.Decoder
								cDec       = compression.NewDecoder()
								cBuf       []byte
								samplesBuf []record.RefFloatHistogramSample
							)

							var buf []byte
							if hcase.Name == "nhcb" {
								buf = enc.CustomBucketsFloatHistogramSamples(samples, nil)
							} else {
								buf, _ = enc.FloatHistogramSamples(samples, nil)
							}

							cEnc, err := compression.NewEncoder()
							require.NoError(b, err)
							buf, _, err = cEnc.Encode(compr, buf, nil)
							require.NoError(b, err)

							// Warm up.
							cBuf, err = cDec.Decode(compr, buf, cBuf[:0])
							require.NoError(b, err)
							samplesBuf, err = dec.FloatHistogramSamples(cBuf, samplesBuf[:0])
							require.NoError(b, err)

							b.ReportAllocs()
							b.ResetTimer()
							for b.Loop() {
								cBuf, _ = cDec.Decode(compr, buf, cBuf[:0])
								samplesBuf, _ = dec.FloatHistogramSamples(cBuf, samplesBuf[:0])
							}
						})
					}
				}
			}
		}
	}
}
