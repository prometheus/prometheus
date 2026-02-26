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

package tsdb

import (
	"testing"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/stretchr/testify/require"
)

func TestMemSeriesAppendUsesFeatureFlaggedFloatEncoding(t *testing.T) {
	expected := chunkenc.ValFloat.ChunkEncoding()

	s := &memSeries{}
	ok, created := s.append(1000, 1.0, 0, chunkOpts{
		chunkRange:      2 * 60 * 60 * 1000,
		samplesPerChunk: 120,
	})
	require.True(t, ok)
	require.True(t, created)
	require.NotNil(t, s.headChunks)
	require.Equal(t, expected, s.headChunks.chunk.Encoding())
}

func TestOOOChunkToEncodedChunksUsesFeatureFlaggedFloatEncoding(t *testing.T) {
	expected := chunkenc.ValFloat.ChunkEncoding()

	c := NewOOOChunk()
	require.True(t, c.Insert(1000, 1.0, nil, nil))
	require.True(t, c.Insert(2000, 2.0, nil, nil))

	chks, err := c.ToEncodedChunks(0, 3000)
	require.NoError(t, err)
	require.Len(t, chks, 1)
	require.Equal(t, expected, chks[0].chunk.Encoding())
}

func TestChunkSnapshotEncodeDecodeSupportsXOR2(t *testing.T) {
	expected := chunkenc.ValFloat.ChunkEncoding()

	chk := chunkenc.NewFloatChunk()

	app, err := chk.Appender()
	require.NoError(t, err)
	app.Append(0, 1000, 1.5)

	s := &memSeries{
		ref: 1,
		headChunks: &memChunk{
			chunk:   chk,
			minTime: 1000,
			maxTime: 1000,
		},
		lastValue: 1.5,
	}

	encoded := s.encodeToSnapshotRecord(nil)
	decoded, err := decodeSeriesFromChunkSnapshot(&record.Decoder{}, encoded)
	require.NoError(t, err)
	require.NotNil(t, decoded.mc)
	require.Equal(t, expected, decoded.mc.chunk.Encoding())
	require.Equal(t, 1.5, decoded.lastValue)
}
