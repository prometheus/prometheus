// Copyright 2019 The Prometheus Authors
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

	i := 0
	for ; i < 4; i++ {
		msg, err := r.Next()
		require.NoError(t, err)
		require.Less(t, i, len(msgs), "more messages then expected")
		require.Equal(t, msgs[i], msg)
	}

	// Empty byte slice is skipped.
	i++

	msg, err := r.Next()
	require.NoError(t, err)
	require.Less(t, i, len(msgs), "more messages then expected")
	require.Equal(t, msgs[i], msg)

	_, err = r.Next()
	require.Error(t, err, "expected io.EOF")
	require.Equal(t, io.EOF, err)

	require.Equal(t, 5, f.flushed)
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
	require.Equal(t, "chunkedReader: message size exceeded the limit 11 bytes; got: 12 bytes", err.Error())
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
	require.Equal(t, "chunkedReader: corrupted frame; checksum mismatch", err.Error())
}
