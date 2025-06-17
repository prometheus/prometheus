// Copyright 2017 The Prometheus Authors
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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

func TestReaderWithInvalidBuffer(t *testing.T) {
	b := realByteSlice([]byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81})
	r := &Reader{bs: []ByteSlice{b}}

	_, _, err := r.ChunkOrIterable(Meta{Ref: 0})
	require.Error(t, err)
}

func TestWriterWithDefaultSegmentSize(t *testing.T) {
	chk1, err := ChunkFromSamples([]Sample{
		sample{t: 10, f: 11},
		sample{t: 20, f: 12},
		sample{t: 30, f: 13},
	})
	require.NoError(t, err)

	chk2, err := ChunkFromSamples([]Sample{
		sample{t: 40, h: tsdbutil.GenerateTestHistogram(1)},
		sample{t: 50, h: tsdbutil.GenerateTestHistogram(2)},
		sample{t: 60, h: tsdbutil.GenerateTestHistogram(3)},
	})
	require.NoError(t, err)

	dir := t.TempDir()
	w, err := NewWriter(dir)
	require.NoError(t, err)

	err = w.WriteChunks(chk1, chk2)
	require.NoError(t, err)

	require.NoError(t, w.Close())

	d, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, d, 1, "expected only one segment to be created to hold both chunks")
}
