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
	"io"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func directIORqmtsForTest(tb testing.TB) *directIORqmts {
	f, err := os.OpenFile(path.Join(tb.TempDir(), "foo"), os.O_CREATE|os.O_WRONLY, 0o666)
	require.NoError(tb, err)
	alignmentRqmts, err := fileDirectIORqmts(f)
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
			for i := range data {
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
