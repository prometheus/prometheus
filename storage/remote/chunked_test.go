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
package remote

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockedFlusher struct {
	flushed int
}

func (f *mockedFlusher) Flush() {
	f.flushed++
}

func TestChunkedReaderCanReadFromChunkedWriter(t *testing.T) {
	b := &bytes.Buffer{}
	f := &mockedFlusher{}
	w := NewChunkedWriter(b, f)

	r := NewChunkedReader(b, 20, nil)

	msgs := [][]byte{
		[]byte("test1"),
		[]byte("test2"),
		[]byte("test3"),
		[]byte("test4"),
		{}, // This is ignored by writer.
		[]byte("test5-after-empty"),
	}

	for _, msg := range msgs {
		n, err := w.Write(msg)
		require.NoError(t, err)
		require.Len(t, msg, n)
	}
	w.Close()

	i := 0
	for ; i < 4; i++ {
		msg, err := r.Next()
		require.NoError(t, err)
		require.Less(t, i, len(msgs), "more messages than expected")
		require.Equal(t, msgs[i], msg)
	}

	// Empty byte slice is skipped.
	i++

	msg, err := r.Next()
	require.NoError(t, err)
	require.Less(t, i, len(msgs), "more messages than expected")
	require.Equal(t, msgs[i], msg)

	_, err = r.Next()
	require.Error(t, err, "expected io.EOF")
	require.Equal(t, io.EOF, err)

	require.Equal(t, 1, f.flushed)
}

func TestChunkedReader_Overflow(t *testing.T) {
	b := &bytes.Buffer{}
	_, err := NewChunkedWriter(b, &mockedFlusher{}).Write([]byte("twelve bytes"))
	require.NoError(t, err)

	b2 := make([]byte, 12)
	copy(b2, b.Bytes())

	ret, err := NewChunkedReader(b, 12, nil).Next()
	require.NoError(t, err)
	require.Equal(t, "twelve bytes", string(ret))

	_, err = NewChunkedReader(bytes.NewReader(b2), 11, nil).Next()
	require.Error(t, err, "expect exceed limit error")
	require.EqualError(t, err, "chunkedReader: message size exceeded the limit 11 bytes; got: 12 bytes")
}

func TestChunkedReader_CorruptedFrame(t *testing.T) {
	b := &bytes.Buffer{}
	w := NewChunkedWriter(b, &mockedFlusher{})

	n, err := w.Write([]byte("test1"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	bs := b.Bytes()
	bs[9] = 1 // Malform the frame by changing one byte.

	_, err = NewChunkedReader(bytes.NewReader(bs), 20, nil).Next()
	require.Error(t, err, "expected malformed frame")
	require.EqualError(t, err, "chunkedReader: corrupted frame; checksum mismatch")
}

type errReader struct {
	err error
}

func (r errReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestChunkedReader_EOF(t *testing.T) {
	b := &bytes.Buffer{}
	w := NewChunkedWriter(b, &mockedFlusher{})
	_, err := w.Write([]byte("hello"))
	require.NoError(t, err)
	frame := b.Bytes() // 1-byte uvarint (5) + 4-byte CRC32 + 5-byte payload.
	require.Len(t, frame, 10)

	t.Run("clean EOF on empty stream", func(t *testing.T) {
		_, err := NewChunkedReader(bytes.NewReader(nil), 20, nil).Next()
		require.ErrorIs(t, err, io.EOF)
	})

	t.Run("clean EOF after complete frame", func(t *testing.T) {
		r := NewChunkedReader(bytes.NewReader(frame), 20, nil)

		msg, err := r.Next()
		require.NoError(t, err)
		require.Equal(t, []byte("hello"), msg)

		_, err = r.Next()
		require.ErrorIs(t, err, io.EOF)
	})

	t.Run("valid zero-length frame followed by clean EOF", func(t *testing.T) {
		// 1-byte uvarint (0) + 4-byte Castagnoli CRC32 of empty slice (0).
		r := NewChunkedReader(bytes.NewReader([]byte{0x00, 0x00, 0x00, 0x00, 0x00}), 20, nil)

		msg, err := r.Next()
		require.NoError(t, err)
		require.Empty(t, msg)

		_, err = r.Next()
		require.ErrorIs(t, err, io.EOF)
	})

	truncatedCases := []struct {
		name string
		data []byte
	}{
		{
			name: "truncated uvarint",
			data: []byte{0x80},
		},
		{
			name: "truncated after size before crc",
			data: frame[:1],
		},
		{
			name: "truncated mid-crc (1 byte)",
			data: frame[:2],
		},
		{
			name: "truncated mid-crc (3 bytes)",
			data: frame[:4],
		},
		{
			name: "truncated after crc before payload",
			data: frame[:5],
		},
		{
			name: "truncated mid-payload",
			data: frame[:7],
		},
		{
			name: "truncated zero-length frame before crc",
			data: []byte{0x00},
		},
	}

	for _, tc := range truncatedCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewChunkedReader(bytes.NewReader(tc.data), 20, nil).Next()
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
		})
	}

	t.Run("preserves non-EOF read errors", func(t *testing.T) {
		readErr := errors.New("connection reset")

		_, err := NewChunkedReader(io.MultiReader(bytes.NewReader(frame[:1]), errReader{err: readErr}), 20, nil).Next()
		require.ErrorIs(t, err, readErr)

		_, err = NewChunkedReader(io.MultiReader(bytes.NewReader(frame[:5]), errReader{err: readErr}), 20, nil).Next()
		require.ErrorIs(t, err, readErr)
	})
}
