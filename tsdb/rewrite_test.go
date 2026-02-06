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
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestRewriteBlock(t *testing.T) {
	for _, tc := range []struct {
		name           string
		targetEncoding chunkenc.Encoding
	}{
		{
			name:           "xor to xor2",
			targetEncoding: chunkenc.EncXOR2,
		},
		{
			name:           "xor to xor (noop)",
			targetEncoding: chunkenc.EncXOR,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			logger := promslog.NewNopLogger()

			// Create a block with XOR-encoded float data.
			series := genSeries(10, 2, 1000, 10000)
			blockDir := createBlock(t, dir, series)
			blockID := filepath.Base(blockDir)

			// Read original block data for comparison.
			origSamples := readBlockSamples(t, dir, blockID)
			require.Greater(t, len(origSamples), 0)

			// Rewrite the block.
			err := RewriteBlock(logger, dir, blockID, tc.targetEncoding)
			require.NoError(t, err)

			// The original block should be gone.
			_, err = os.Stat(blockDir)
			require.True(t, os.IsNotExist(err))

			// Find the new block.
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			var newBlockID string
			for _, e := range entries {
				if e.IsDir() && e.Name() != blockID {
					newBlockID = e.Name()
					break
				}
			}
			require.NotEmpty(t, newBlockID, "new block should exist")

			// Verify data integrity.
			newSamples := readBlockSamples(t, dir, newBlockID)
			require.Equal(t, origSamples, newSamples)

			// Verify encoding of chunks.
			verifyBlockChunkEncoding(t, dir, newBlockID, tc.targetEncoding)
		})
	}
}

func TestRewriteBlockXOR2ToXOR(t *testing.T) {
	dir := t.TempDir()
	logger := promslog.NewNopLogger()

	// Create a block with XOR data first.
	series := genSeries(5, 2, 1000, 5000)
	blockDir := createBlock(t, dir, series)
	blockID := filepath.Base(blockDir)

	// Rewrite to XOR2.
	err := RewriteBlock(logger, dir, blockID, chunkenc.EncXOR2)
	require.NoError(t, err)

	// Find the XOR2 block.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var xor2BlockID string
	for _, e := range entries {
		if e.IsDir() {
			xor2BlockID = e.Name()
			break
		}
	}
	require.NotEmpty(t, xor2BlockID)

	// Read the XOR2 block samples.
	xor2Samples := readBlockSamples(t, dir, xor2BlockID)

	// Rewrite back to XOR.
	err = RewriteBlock(logger, dir, xor2BlockID, chunkenc.EncXOR)
	require.NoError(t, err)

	// Find the new XOR block.
	entries, err = os.ReadDir(dir)
	require.NoError(t, err)
	var xorBlockID string
	for _, e := range entries {
		if e.IsDir() {
			xorBlockID = e.Name()
			break
		}
	}
	require.NotEmpty(t, xorBlockID)

	// Verify data integrity after round-trip.
	xorSamples := readBlockSamples(t, dir, xorBlockID)
	require.Equal(t, xor2Samples, xorSamples)

	// Verify encoding.
	verifyBlockChunkEncoding(t, dir, xorBlockID, chunkenc.EncXOR)
}

type seriesSample struct {
	labels string
	t      int64
	v      float64
}

// readBlockSamples reads all float samples from a block.
func readBlockSamples(t *testing.T, dbPath, blockID string) []seriesSample {
	t.Helper()
	blockDir := filepath.Join(dbPath, blockID)
	pool := chunkenc.NewPool()
	block, err := OpenBlock(promslog.NewNopLogger(), blockDir, pool, DefaultPostingsDecoderFactory)
	require.NoError(t, err)
	defer block.Close()

	indexr, err := block.Index()
	require.NoError(t, err)
	defer indexr.Close()

	chunkr, err := block.Chunks()
	require.NoError(t, err)
	defer chunkr.Close()

	var result []seriesSample
	all := AllSortedPostings(context.Background(), indexr)
	var builder labels.ScratchBuilder
	var chks []chunks.Meta

	for all.Next() {
		require.NoError(t, indexr.Series(all.At(), &builder, &chks))
		lset := builder.Labels().String()

		for _, chk := range chks {
			c, _, err := chunkr.ChunkOrIterable(chk)
			require.NoError(t, err)

			if !isFloatEncoding(c.Encoding()) {
				continue
			}

			it := c.Iterator(nil)
			for it.Next() == chunkenc.ValFloat {
				ts, v := it.At()
				result = append(result, seriesSample{labels: lset, t: ts, v: v})
			}
			require.NoError(t, it.Err())
		}
	}
	require.NoError(t, all.Err())
	return result
}

// verifyBlockChunkEncoding verifies that all float chunks in a block use the expected encoding.
func verifyBlockChunkEncoding(t *testing.T, dbPath, blockID string, expectedEncoding chunkenc.Encoding) {
	t.Helper()
	blockDir := filepath.Join(dbPath, blockID)
	pool := chunkenc.NewPool()
	block, err := OpenBlock(promslog.NewNopLogger(), blockDir, pool, DefaultPostingsDecoderFactory)
	require.NoError(t, err)
	defer block.Close()

	indexr, err := block.Index()
	require.NoError(t, err)
	defer indexr.Close()

	chunkr, err := block.Chunks()
	require.NoError(t, err)
	defer chunkr.Close()

	all := AllSortedPostings(context.Background(), indexr)
	var builder labels.ScratchBuilder
	var chks []chunks.Meta

	for all.Next() {
		require.NoError(t, indexr.Series(all.At(), &builder, &chks))

		for _, chk := range chks {
			c, _, err := chunkr.ChunkOrIterable(chk)
			require.NoError(t, err)

			if isFloatEncoding(c.Encoding()) {
				require.Equal(t, expectedEncoding, c.Encoding(),
					"chunk encoding mismatch for series %s", builder.Labels().String())
			}
		}
	}
	require.NoError(t, all.Err())
}

