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

package compression

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

const compressible = `ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa
fsfsdfsfddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa2
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa12
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa1
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa121
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa324
ddddddddddsfpgjsdoadjgfpajdspfgjasfjapddddddddddaaaaaaaa145
`

func TestEncodeDecode(t *testing.T) {
	for _, tcase := range []struct {
		name string

		src               string
		types             []Type
		encBuf            EncodeBuffer
		decBuf            DecodeBuffer
		expectCompression bool
		expectEncErr      error
	}{
		{
			name:              "empty src; no buffers",
			types:             Types(),
			src:               "",
			expectCompression: false,
		},
		{
			name:   "empty src; sync buffers",
			types:  Types(),
			encBuf: NewSyncEncodeBuffer(), decBuf: NewSyncDecodeBuffer(),
			src:               "",
			expectCompression: false,
		},
		{
			name:   "empty src; concurrent buffers",
			types:  Types(),
			encBuf: NewConcurrentEncodeBuffer(), decBuf: NewConcurrentDecodeBuffer(),
			src:               "",
			expectCompression: false,
		},
		{
			name:              "no buffers",
			types:             []Type{None},
			src:               compressible,
			expectCompression: false,
		},
		{
			name:              "no buffers",
			types:             []Type{Snappy},
			src:               compressible,
			expectCompression: true,
		},
		{
			name:         "no buffers",
			types:        []Type{Zstd},
			src:          compressible,
			expectEncErr: errors.New("zstd requested but EncodeBuffer was not provided"),
		},
		{
			name:   "sync buffers",
			types:  []Type{None},
			encBuf: NewSyncEncodeBuffer(), decBuf: NewSyncDecodeBuffer(),
			src:               compressible,
			expectCompression: false,
		},
		{
			name:   "sync buffers",
			types:  Types()[1:], // All but none
			encBuf: NewSyncEncodeBuffer(), decBuf: NewSyncDecodeBuffer(),
			src:               compressible,
			expectCompression: true,
		},
		{
			name:   "concurrent buffers",
			types:  []Type{None},
			encBuf: NewConcurrentEncodeBuffer(), decBuf: NewConcurrentDecodeBuffer(),
			src:               compressible,
			expectCompression: false,
		},
		{
			name:   "concurrent buffers",
			types:  Types()[1:], // All but none
			encBuf: NewConcurrentEncodeBuffer(), decBuf: NewConcurrentDecodeBuffer(),
			src:               compressible,
			expectCompression: true,
		},
	} {
		require.NotEmpty(t, tcase.types, "must specify at least one type")
		for _, typ := range tcase.types {
			t.Run(fmt.Sprintf("case=%v/type=%v", tcase.name, typ), func(t *testing.T) {
				res, err := Encode(typ, []byte(tcase.src), tcase.encBuf)
				if tcase.expectEncErr != nil {
					require.ErrorContains(t, err, tcase.expectEncErr.Error())
					return
				}
				require.NoError(t, err)
				if tcase.expectCompression {
					require.Less(t, len(res), len(tcase.src))
				}

				// Decode back.
				got, err := Decode(typ, res, tcase.decBuf)
				require.NoError(t, err)
				require.Equal(t, tcase.src, string(got))
			})
		}
	}
}

/*
	export bench=encode-v1 && go test ./util/compression/... \
		-run '^$' -bench '^BenchmarkEncode' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee ${bench}.txt
*/
func BenchmarkEncode(b *testing.B) {
	for _, typ := range Types() {
		b.Run(fmt.Sprintf("type=%v", typ), func(b *testing.B) {
			var buf EncodeBuffer
			compressible := []byte(compressible)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if buf == nil {
					buf = NewSyncEncodeBuffer()
				}
				res, err := Encode(typ, compressible, buf)
				require.NoError(b, err)
				b.ReportMetric(float64(len(res)), "B")
			}
		})
	}
}

/*
	export bench=decode-v1 && go test ./util/compression/... \
		-run '^$' -bench '^BenchmarkDecode' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee ${bench}.txt
*/
func BenchmarkDecode(b *testing.B) {
	for _, typ := range Types() {
		b.Run(fmt.Sprintf("type=%v", typ), func(b *testing.B) {
			var buf DecodeBuffer
			res, err := Encode(typ, []byte(compressible), NewConcurrentEncodeBuffer())
			require.NoError(b, err)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if buf == nil {
					buf = NewSyncDecodeBuffer()
				}
				_, err := Decode(typ, res, buf)
				require.NoError(b, err)
			}
		})
	}
}
