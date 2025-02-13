// Copyright 2024 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux

package fileutil

import (
	"fmt"
	"io"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func directIORqmtsForTest(tb testing.TB) *directIORqmts {
	f, err := os.OpenFile(path.Join(tb.TempDir(), "foo"), os.O_CREATE|os.O_WRONLY, 0o666)
	require.NoError(tb, err)
	alignmentRqmts, err := fetchDirectIORqmts(f.Fd())
	require.NoError(tb, err)
	return alignmentRqmts
}

func TestDirectIOFile(t *testing.T) {
	tmpDir := t.TempDir()

	f, err := os.OpenFile(path.Join(tmpDir, "test"), os.O_CREATE|os.O_WRONLY, 0o666)
	require.NoError(t, err)

	require.NoError(t, enableDirectIO(f.Fd()))
}

func TestAlignedBlockEarlyPanic(t *testing.T) {
	alignRqmts := directIORqmtsForTest(t)
	cases := []struct {
		desc string
		size int
	}{
		{"Zero size", 0},
		{"Size not multiple of offset alignment", 9973},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			require.Panics(t, func() {
				alignedBlock(tc.size, alignRqmts)
			})
		})
	}
}

func TestAlignedBloc(t *testing.T) {
	alignRqmts := directIORqmtsForTest(t)
	block := alignedBlock(5*alignRqmts.offsetAlign, alignRqmts)
	require.True(t, isAligned(block, alignRqmts))
	require.Len(t, block, 5*alignRqmts.offsetAlign)
	require.False(t, isAligned(block[1:], alignRqmts))
}

func TestDirectIOWriter(t *testing.T) {
	alignRqmts := directIORqmtsForTest(t)
	cases := []struct {
		name          string
		initialOffset int
		bufferSize    int
		dataSize      int
		// writtenBytes should also consider needed zero padding.
		writtenBytes     int
		shouldInvalidate bool
	}{
		{
			name:         "data equal to buffer",
			bufferSize:   8 * alignRqmts.offsetAlign,
			dataSize:     8 * alignRqmts.offsetAlign,
			writtenBytes: 8 * alignRqmts.offsetAlign,
		},
		{
			name:         "data exceeds buffer",
			bufferSize:   4 * alignRqmts.offsetAlign,
			dataSize:     64 * alignRqmts.offsetAlign,
			writtenBytes: 64 * alignRqmts.offsetAlign,
		},
		{
			name:             "data exceeds buffer + final offset unaligned",
			bufferSize:       2 * alignRqmts.offsetAlign,
			dataSize:         4*alignRqmts.offsetAlign + 33,
			writtenBytes:     4*alignRqmts.offsetAlign + alignRqmts.offsetAlign,
			shouldInvalidate: true,
		},
		{
			name:         "data smaller than buffer",
			bufferSize:   8 * alignRqmts.offsetAlign,
			dataSize:     3 * alignRqmts.offsetAlign,
			writtenBytes: 3 * alignRqmts.offsetAlign,
		},
		{
			name:             "data smaller than buffer + final offset unaligned",
			bufferSize:       4 * alignRqmts.offsetAlign,
			dataSize:         alignRqmts.offsetAlign + 70,
			writtenBytes:     alignRqmts.offsetAlign + alignRqmts.offsetAlign,
			shouldInvalidate: true,
		},
		{
			name:          "offset aligned",
			initialOffset: alignRqmts.offsetAlign,
			bufferSize:    8 * alignRqmts.offsetAlign,
			dataSize:      alignRqmts.offsetAlign,
			writtenBytes:  alignRqmts.offsetAlign,
		},
		{
			name:             "initial offset unaligned + final offset unaligned",
			initialOffset:    8,
			bufferSize:       8 * alignRqmts.offsetAlign,
			dataSize:         64 * alignRqmts.offsetAlign,
			writtenBytes:     64*alignRqmts.offsetAlign + (alignRqmts.offsetAlign - 8),
			shouldInvalidate: true,
		},
		{
			name:          "offset unaligned + final offset aligned",
			initialOffset: 8,
			bufferSize:    4 * alignRqmts.offsetAlign,
			dataSize:      4*alignRqmts.offsetAlign + (alignRqmts.offsetAlign - 8),
			writtenBytes:  4*alignRqmts.offsetAlign + (alignRqmts.offsetAlign - 8),
		},
		{
			name:       "empty data",
			bufferSize: 4 * alignRqmts.offsetAlign,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fileName := path.Join(t.TempDir(), "test")

			data := make([]byte, tc.dataSize)
			for i := 0; i < len(data); i++ {
				// Do not use 256 as it may be a divider of requiredAlignment. To avoid patterns.
				data[i] = byte(i % 251)
			}

			f, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, 0o666)
			require.NoError(t, err)

			if tc.initialOffset != 0 {
				_, err = f.Seek(int64(tc.initialOffset), io.SeekStart)
				require.NoError(t, err)
			}

			w, err := newDirectIOWriter(f, tc.bufferSize)
			require.NoError(t, err)

			n, err := w.Write(data)
			require.NoError(t, err)
			require.Equal(t, tc.dataSize, n)
			require.NoError(t, w.Flush())

			// Check the file's final offset.
			currOffset, err := currentFileOffset(f)
			require.NoError(t, err)
			require.Equal(t, tc.dataSize+tc.initialOffset, currOffset)

			// Check the written data.
			fileBytes, err := os.ReadFile(fileName)
			require.NoError(t, err)
			if tc.dataSize > 0 {
				require.Len(t, fileBytes, tc.writtenBytes+tc.initialOffset)
				require.Equal(t, data, fileBytes[tc.initialOffset:tc.dataSize+tc.initialOffset])
			} else {
				require.Empty(t, fileBytes)
			}

			// Check the writer state.
			if tc.shouldInvalidate {
				require.True(t, w.invalid)
				require.Error(t, w.Flush())
				_, err = w.Write([]byte{})
				require.Error(t, err)
			} else {
				require.False(t, w.invalid)
				require.NoError(t, w.Flush())
				_, err = w.Write([]byte{})
				require.NoError(t, err)
			}
		})
	}
}

func BenchmarkBufWriter(b *testing.B) {
	// newFile creates a new file in a temporary directory,
	// seeks 8 bytes forward to simulate pre‐written SegmentHeaderSize.
	newFile := func() *os.File {
		fileName := path.Join(b.TempDir(), "test")
		f, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, 0o666)
		require.NoError(b, err)
		_, err = f.Seek(8, io.SeekStart)
		require.NoError(b, err)
		return f
	}

	chunks := []int{0, 1, 2, 3, 5, 11, 63, 128, 280, 333, 450, 500, 512}

	for _, writer := range []string{"bufioWriter", "directIOWriter"} {
		b.Run(fmt.Sprintf("sample=%s", writer), func(b *testing.B) {
			data := make([]byte, 512*1024*1024+1)
			for i := 0; i < len(data); i++ {
				data[i] = byte(i % 251)
			}
			// Ensure the block is not aligned for directIOWriter’s worst-case scenario.
			if isAligned(data, directIORqmtsForTest(b)) {
				data = data[1:]
			} else {
				data = data[:len(data)-1]
			}

			file := newFile()

			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()

			const bufferSize = 8 * 1024 * 1024
			var bw BufWriter
			var err error

			switch writer {
			case "directIOWriter":
				bw, err = NewDirectIOWriter(file, bufferSize)
				require.NoError(b, err)
			case "bufioWriter":
				bw, err = NewBufioWriterWithSeek(file, bufferSize)
				require.NoError(b, err)
			default:
				b.Fatalf("unknown writer : %s", writer)
			}

			for i := 0; i < b.N; i++ {
				// Write the data in multiple chunks.
				for i := range len(chunks) - 1 {
					_, err := bw.Write(data[chunks[i]*1024*1024 : chunks[i+1]*1024*1024])
					require.NoError(b, err)
				}
				require.NoError(b, bw.Flush())
				require.NoError(b, file.Sync())

				// Reset to a new file.
				b.StopTimer()
				require.NoError(b, file.Close())
				require.NoError(b, os.Remove(file.Name()))
				file = newFile()
				b.StartTimer()

				// Reset the writer to the new file.
				bw.Reset(file)
			}
		})
	}
}
