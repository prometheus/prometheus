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
	"bufio"
	"io"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDirectIOFile(t *testing.T) {
	tmpDir := t.TempDir()

	f, err := os.OpenFile(path.Join(tmpDir, "test"), os.O_CREATE|os.O_WRONLY, 0o666)
	require.NoError(t, err)

	require.NoError(t, enableDirectIO(f.Fd()))
}

func TestAlignedBlockEarlyPanic(t *testing.T) {
	cases := []struct {
		desc  string
		size  int
		align int
	}{
		{"Zero size", 0, blockSizeUnit},
		{"Size not multiple of block unit", 9999, blockSizeUnit},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			require.Panics(t, func() {
				alignedBlock(tc.size, tc.align)
			})
		})
	}
}

func TestAlignedBloc(t *testing.T) {
	block := alignedBlock(5*blockSizeUnit, blockSizeUnit)
	require.True(t, isAligned(block))
	require.Len(t, block, 5*blockSizeUnit)
	require.False(t, isAligned(block[1:]))
}

func TestDirectIOWriter(t *testing.T) {
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
			bufferSize:   8 * blockSizeUnit,
			dataSize:     8 * blockSizeUnit,
			writtenBytes: 8 * blockSizeUnit,
		},
		{
			name:         "data exceeds buffer",
			bufferSize:   4 * blockSizeUnit,
			dataSize:     64 * blockSizeUnit,
			writtenBytes: 64 * blockSizeUnit,
		},
		{
			name:             "data exceeds buffer + final offset unaligned",
			bufferSize:       2 * blockSizeUnit,
			dataSize:         4*blockSizeUnit + 33,
			writtenBytes:     4*blockSizeUnit + blockSizeUnit,
			shouldInvalidate: true,
		},
		{
			name:         "data smaller than buffer",
			bufferSize:   8 * blockSizeUnit,
			dataSize:     3 * blockSizeUnit,
			writtenBytes: 3 * blockSizeUnit,
		},
		{
			name:             "data smaller than buffer + final offset unaligned",
			bufferSize:       4 * blockSizeUnit,
			dataSize:         blockSizeUnit + 70,
			writtenBytes:     blockSizeUnit + blockSizeUnit,
			shouldInvalidate: true,
		},
		{
			name:          "offset aligned",
			initialOffset: requiredAlignment,
			bufferSize:    8 * blockSizeUnit,
			dataSize:      blockSizeUnit,
			writtenBytes:  blockSizeUnit,
		},
		{
			name:             "initial offset unaligned + final offset unaligned",
			initialOffset:    8,
			bufferSize:       8 * blockSizeUnit,
			dataSize:         64 * blockSizeUnit,
			writtenBytes:     64*blockSizeUnit + (blockSizeUnit - 8),
			shouldInvalidate: true,
		},
		{
			name:          "offset unaligned + final offset aligned",
			initialOffset: 8,
			bufferSize:    4 * blockSizeUnit,
			dataSize:      4*blockSizeUnit + (blockSizeUnit - 8),
			writtenBytes:  4*blockSizeUnit + (blockSizeUnit - 8),
		},
		{
			name:       "empty data",
			bufferSize: 4 * blockSizeUnit,
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

// BenchmarkDirectIOWriter XXX
func BenchmarkDirectIOWriter(b *testing.B) {
	data := make([]byte, 512*1024*1024+1)
	for i := 0; i < len(data); i++ {
		data[i] = byte(i % 251)
	}
	// Ensure the block is not aligned to ensure the worst-case scenario for
	// the Direct I/O Writer.
	// This approach also helps to achieve a deterministic behavior.
	if isAligned(data) {
		data = data[1:]
	}
	chunks := []int{0, 1, 2, 3, 5, 11, 63, 128, 280, 333, 450, 500, 512}

	newFile := func() *os.File {
		fileName := path.Join(b.TempDir(), "test")
		f, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, 0o666)
		require.NoError(b, err)
		// file with initial offset to simulate what happens with segments.
		_, err = f.Seek(8, io.SeekStart)
		require.NoError(b, err)
		return f
	}
	file := newFile()

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	//bw, err := newDirectIOWriter(file, 8*1024*1024)
	//require.NoError(b, err)

	// Uncomment to compare to bufio's Writer.
	bw := bufio.NewWriterSize(file, 8*1024*1024)

	for i := 0; i < b.N; i++ {
		// Write the data in multiple steps.
		for i := range len(chunks) - 1 {
			bw.Write(data[chunks[i]*1024*1024 : chunks[i+1]*1024*1024])
		}
		bw.Flush()
		require.NoError(b, file.Sync())

		// Reset to a new file.
		b.StopTimer()
		os.Remove(file.Name())
		file = newFile()
		b.StartTimer()
		bw.Reset(file)
	}
}
