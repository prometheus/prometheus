// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0.

package index

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/encoding"
)

func writeSymbolDirectoryFixture(tb testing.TB, count, width int) ([]byte, []string, *TOC) {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "index")
	w, err := NewWriterWithOptions(tb.Context(), path, WriterOptions{FormatVersion: FormatV4})
	require.NoError(tb, err)
	symbols := make([]string, count)
	for i := range symbols {
		symbols[i] = fmt.Sprintf("%08d%s", i, strings.Repeat("x", width))
		require.NoError(tb, w.AddSymbol(symbols[i]))
	}
	require.NoError(tb, w.Close())
	data, err := os.ReadFile(path)
	require.NoError(tb, err)
	toc, err := NewTOCFromByteSlice(realByteSlice(data))
	require.NoError(tb, err)
	return data, symbols, toc
}

func TestIndexV4SymbolDirectory(t *testing.T) {
	for _, count := range []int{0, 1, 31, 32, 33, 64, 65, 1025} {
		for _, width := range []int{0, 1000} {
			t.Run(fmt.Sprintf("count=%d/width=%d", count, width), func(t *testing.T) {
				data, want, toc := writeSymbolDirectoryFixture(t, count, width)
				symbols, err := NewSymbols(realByteSlice(data), FormatV4, int(toc.Symbols))
				require.NoError(t, err)
				require.Len(t, symbols.offsets, (count+31)/32)
				require.Equal(t, int(toc.Series), symbols.end)
				for i, s := range want {
					got, err := symbols.Lookup(uint32(i))
					require.NoError(t, err)
					require.Equal(t, s, got)
					ref, err := symbols.ReverseLookup(s)
					require.NoError(t, err)
					require.Equal(t, uint32(i), ref)
				}
				it := symbols.Iter()
				for _, s := range want {
					require.True(t, it.Next())
					require.Equal(t, s, it.At())
				}
				require.False(t, it.Next())
				require.NoError(t, it.Err())
				_, err = symbols.Lookup(uint32(count))
				require.EqualError(t, err, fmt.Sprintf("unknown symbol offset %d", count))
				_, err = symbols.ReverseLookup("missing")
				require.Error(t, err)
				r, err := NewReader(realByteSlice(data), DecodePostingsRaw)
				require.NoError(t, err)
				require.NoError(t, r.Close())
			})
		}
	}
}

func TestIndexV4SymbolDirectoryCorruption(t *testing.T) {
	data, _, toc := writeSymbolDirectoryFixture(t, 65, 4)
	start := int(toc.Symbols) + 8 + int(binary.BigEndian.Uint64(data[toc.Symbols:])) + 4
	payloadSize := int(binary.BigEndian.Uint32(data[start:]))
	tests := []struct {
		name     string
		mutate   func([]byte)
		checksum bool
	}{
		{"checksum", func(b []byte) { b[start+8] ^= 1 }, false},
		{"truncated length", func(b []byte) { binary.BigEndian.PutUint32(b[start:], math.MaxUint32) }, false},
		{"sampling interval", func(b []byte) { binary.BigEndian.PutUint32(b[start+4:], 64) }, true},
		{"first offset", func(b []byte) { b[start+8] = 13 }, true},
		{"zero delta", func(b []byte) { b[start+9] = 0 }, true},
		{"overflowing delta", func(b []byte) {
			for i := start + 9; i < start+4+payloadSize; i++ {
				b[i] = 255
			}
		}, true},
		{"symbol checksum", func(b []byte) { b[toc.Symbols+12] ^= 1 }, false},
		{"incorrect count", func(b []byte) {
			binary.BigEndian.PutUint32(b[toc.Symbols+8:], 66)
			end := int(toc.Symbols) + 8 + int(binary.BigEndian.Uint64(b[toc.Symbols:]))
			binary.BigEndian.PutUint32(b[end:], crc32.Checksum(b[toc.Symbols+8:uint64(end)], castagnoliTable))
		}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bad := slices.Clone(data)
			tc.mutate(bad)
			if tc.checksum {
				binary.BigEndian.PutUint32(bad[start+4+payloadSize:], crc32.Checksum(bad[start+4:start+4+payloadSize], castagnoliTable))
			}
			_, err := NewReader(realByteSlice(bad), DecodePostingsRaw)
			require.Error(t, err)
		})
	}
	// An index from the earlier PoC has no directory and must not silently open as the revised format.
	_, err := NewSymbols(realByteSlice(data[:start]), FormatV4, int(toc.Symbols))
	require.Error(t, err)
}

func FuzzIndexV4SymbolDirectory(f *testing.F) {
	data, _, toc := writeSymbolDirectoryFixture(f, 65, 4)
	start := int(toc.Symbols) + 8 + int(binary.BigEndian.Uint64(data[toc.Symbols:])) + 4
	size := int(binary.BigEndian.Uint32(data[start:]))
	f.Add(slices.Clone(data[start+4 : start+4+size]))
	f.Add([]byte{0, 0, 0, 32, 12})
	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) > 4096 {
			t.Skip()
		}
		b := slices.Clone(data[:start])
		b = binary.BigEndian.AppendUint32(b, uint32(len(payload)))
		b = append(b, payload...)
		b = binary.BigEndian.AppendUint32(b, crc32.Checksum(payload, castagnoliTable))
		s, err := NewSymbols(realByteSlice(b), FormatV4, int(toc.Symbols))
		if err != nil {
			return
		}
		for i := 0; i < s.seen; i++ {
			_, _ = s.Lookup(uint32(i))
		}
		_, _ = s.ReverseLookup("missing")
		it := s.Iter()
		for it.Next() {
		}
		_ = it.Err()
	})
}

var symbolDirectoryBenchmarkSink []uint64

func BenchmarkSymbolDirectoryEncoding(b *testing.B) {
	for _, width := range []uint64{17, 65, 1025} {
		offsets := make([]uint64, 31250)
		for i := range offsets {
			offsets[i] = 12 + uint64(i)*32*width
		}
		for _, codec := range []string{"fixed64", "absolute_varint", "delta_varint"} {
			var data []byte
			var previous uint64
			for _, offset := range offsets {
				switch codec {
				case "fixed64":
					data = binary.BigEndian.AppendUint64(data, offset)
				case "absolute_varint":
					data = binary.AppendUvarint(data, offset)
				case "delta_varint":
					data = binary.AppendUvarint(data, offset-previous)
				}
				previous = offset
			}
			b.Run(fmt.Sprintf("bytes_per_symbol=%d/%s", width, codec), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				b.ReportMetric(float64(len(data)), "encoded_bytes")
				for i := 0; i < b.N; i++ {
					out := make([]uint64, len(offsets))
					d := encoding.Decbuf{B: data}
					var previous uint64
					for j := range out {
						if codec == "fixed64" {
							out[j] = d.Be64()
						} else {
							out[j] = d.Uvarint64()
							if codec == "delta_varint" {
								out[j] += previous
							}
						}
						previous = out[j]
					}
					if d.Err() != nil || d.Len() != 0 || out[len(out)-1] != offsets[len(offsets)-1] {
						b.Fatal("invalid round trip")
					}
					symbolDirectoryBenchmarkSink = out
				}
			})
		}
	}
}
