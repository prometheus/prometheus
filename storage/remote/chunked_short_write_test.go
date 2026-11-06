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
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// shortFrameWriter deliberately violates io.Writer's short-write contract.
// The codec must fail closed rather than report success for an incomplete frame.
type shortFrameWriter struct {
	dest    bytes.Buffer
	shortAt int
	calls   int
}

func (w *shortFrameWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == w.shortAt {
		if len(p) == 0 {
			return 0, nil
		}
		return w.dest.Write(p[:len(p)-1])
	}
	return w.dest.Write(p)
}

func TestChunkedWriterRejectsShortFrameWrites(t *testing.T) {
	for _, tc := range []struct {
		name string
		call int
	}{
		{"length-delimiter", 1},
		{"checksum", 2},
		{"payload", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &shortFrameWriter{shortAt: tc.call}
			cw := NewChunkedWriter(w, &mockedFlusher{})
			n, err := cw.Write([]byte("hello"))
			require.ErrorIs(t, err, io.ErrShortWrite)
			if tc.call < 3 {
				require.Zero(t, n)
				require.Equal(t, tc.call, w.calls)
			} else {
				require.Equal(t, 4, n)
				require.Equal(t, 3, w.calls)
			}
		})
	}
}

func TestChunkedWriterFullFrameStillRoundTrips(t *testing.T) {
	w := &shortFrameWriter{}
	cw := NewChunkedWriter(w, &mockedFlusher{})
	msg := []byte("complete-frame")
	n, err := cw.Write(msg)
	require.NoError(t, err)
	require.Equal(t, len(msg), n)
	got, err := NewChunkedReader(&w.dest, 100, nil).Next()
	require.NoError(t, err)
	require.Equal(t, msg, got)
}
