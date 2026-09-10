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

package chunks

import (
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/encoding"
)

func TestReaderRetainsSegmentOwner(t *testing.T) {
	cleaned := exerciseOwnedReaderChunk(t)
	require.Eventually(t, func() bool {
		runtime.GC()
		return cleaned.Load()
	}, 2*time.Second, 10*time.Millisecond)
}

func exerciseOwnedReaderChunk(t *testing.T) *atomic.Bool {
	t.Helper()
	r, chunk, pool, segment, cleaned := ownedReaderChunk(t)
	runtime.GC()
	runtime.KeepAlive(r)
	require.False(t, cleaned.Load())

	chunkStart := uintptr(unsafe.Pointer(unsafe.SliceData(chunk.Bytes())))
	segmentStart := uintptr(unsafe.Pointer(unsafe.SliceData(segment)))
	require.GreaterOrEqual(t, chunkStart, segmentStart)
	require.Less(t, chunkStart, segmentStart+uintptr(len(segment)))
	require.Equal(t, chunkenc.ValFloat, chunk.Iterator(nil).Next())

	require.NoError(t, pool.Put(chunk))
	runtime.KeepAlive(r)
	return cleaned
}

func ownedReaderChunk(t *testing.T) (*Reader, chunkenc.Chunk, chunkenc.Pool, []byte, *atomic.Bool) {
	t.Helper()
	dir := t.TempDir()
	w, err := NewWriter(dir)
	require.NoError(t, err)
	c := chunkenc.NewXORChunk()
	app, err := c.Appender()
	require.NoError(t, err)
	app.Append(0, 1000, 1)
	require.NoError(t, w.WriteChunks(Meta{Chunk: c}))
	require.NoError(t, w.Close())

	files, err := sequenceFiles(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	b, err := os.ReadFile(files[0])
	require.NoError(t, err)

	owner := new([64]byte)
	cleaned := &atomic.Bool{}
	runtime.AddCleanup(owner, func(cleaned *atomic.Bool) {
		cleaned.Store(true)
	}, cleaned)
	view := encoding.NewByteViewWithOwner(b, owner)
	pool := chunkenc.NewPool()
	r, err := newReader([]ByteSlice{realByteSlice(b)}, []encoding.ByteView{view}, nil, pool)
	require.NoError(t, err)
	chunk, _, err := r.ChunkOrIterable(Meta{Ref: ChunkRef(NewBlockChunkRef(0, SegmentHeaderSize))})
	require.NoError(t, err)
	return r, chunk, pool, b, cleaned
}
