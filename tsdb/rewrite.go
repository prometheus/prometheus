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
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/oklog/ulid/v2"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

// RewriteBlock rewrites a block in the given database directory, converting
// float chunks to the specified encoding. Non-float chunks are passed through
// unchanged. The original block is replaced atomically.
func RewriteBlock(logger *slog.Logger, dbPath, blockID string, targetEncoding chunkenc.Encoding) error {
	if targetEncoding != chunkenc.EncXOR && targetEncoding != chunkenc.EncXOR2 {
		return fmt.Errorf("unsupported target float chunk encoding: %s", targetEncoding)
	}

	blockDir := filepath.Join(dbPath, blockID)
	if _, err := os.Stat(blockDir); os.IsNotExist(err) {
		return fmt.Errorf("block directory does not exist: %s", blockDir)
	}

	pool := chunkenc.NewPool()
	block, err := OpenBlock(logger, blockDir, pool, DefaultPostingsDecoderFactory)
	if err != nil {
		return fmt.Errorf("open block: %w", err)
	}
	defer func() {
		if cerr := block.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	meta := block.Meta()
	newULID := ulid.MustNew(ulid.Now(), rand.Reader)
	newMeta := BlockMeta{
		ULID:       newULID,
		MinTime:    meta.MinTime,
		MaxTime:    meta.MaxTime,
		Compaction: meta.Compaction,
	}
	newMeta.Compaction.Sources = append([]ulid.ULID{}, meta.Compaction.Sources...)

	tmp := filepath.Join(dbPath, newULID.String()+tmpForCreationBlockDirSuffix)
	if err := os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("remove existing tmp dir: %w", err)
	}
	if err := os.MkdirAll(tmp, 0o777); err != nil {
		return fmt.Errorf("create tmp dir: %w", err)
	}
	defer func() {
		// Clean up tmp dir on error.
		if err != nil {
			if rerr := os.RemoveAll(tmp); rerr != nil {
				logger.Error("Failed to remove tmp dir after error", "err", rerr)
			}
		}
	}()

	if err := rewriteBlockData(context.Background(), block, tmp, targetEncoding, pool, &newMeta); err != nil {
		return fmt.Errorf("rewrite block data: %w", err)
	}

	if newMeta.Stats.NumSamples == 0 {
		return fmt.Errorf("rewritten block has no samples")
	}

	if _, err := writeMetaFile(logger, tmp, &newMeta); err != nil {
		return fmt.Errorf("write meta file: %w", err)
	}

	if _, err := tombstones.WriteFile(logger, tmp, tombstones.NewMemTombstones()); err != nil {
		return fmt.Errorf("write tombstones file: %w", err)
	}

	df, err := fileutil.OpenDir(tmp)
	if err != nil {
		return fmt.Errorf("open tmp dir: %w", err)
	}
	if err := df.Sync(); err != nil {
		df.Close()
		return fmt.Errorf("sync tmp dir: %w", err)
	}
	if err := df.Close(); err != nil {
		return fmt.Errorf("close tmp dir: %w", err)
	}

	// Move new block to final location.
	finalDir := filepath.Join(dbPath, newULID.String())
	if err := fileutil.Replace(tmp, finalDir); err != nil {
		return fmt.Errorf("rename block dir: %w", err)
	}

	// Remove old block atomically.
	tmpToDelete := filepath.Join(dbPath, fmt.Sprintf("%s%s", blockID, tmpForDeletionBlockDirSuffix))
	if err := fileutil.Replace(blockDir, tmpToDelete); err != nil {
		return fmt.Errorf("rename old block for deletion: %w", err)
	}
	if err := os.RemoveAll(tmpToDelete); err != nil {
		logger.Warn("Failed to remove old block after rewrite", "err", err)
	}

	logger.Info("Block rewritten successfully", "old", blockID, "new", newULID.String(), "encoding", targetEncoding)
	return nil
}

// rewriteBlockData reads all series and chunks from the source block, re-encodes
// float chunks to the target encoding, and writes them to the destination directory.
func rewriteBlockData(ctx context.Context, block *Block, destDir string, targetEncoding chunkenc.Encoding, pool chunkenc.Pool, meta *BlockMeta) error {
	indexr, err := block.Index()
	if err != nil {
		return fmt.Errorf("open index reader: %w", err)
	}
	defer indexr.Close()

	chunkr, err := block.Chunks()
	if err != nil {
		return fmt.Errorf("open chunk reader: %w", err)
	}
	defer chunkr.Close()

	chunkw, err := chunks.NewWriter(chunkDir(destDir), chunks.WithSegmentSize(0))
	if err != nil {
		return fmt.Errorf("open chunk writer: %w", err)
	}

	indexw, err := index.NewWriter(ctx, filepath.Join(destDir, indexFilename))
	if err != nil {
		chunkw.Close()
		return fmt.Errorf("open index writer: %w", err)
	}

	var closers []io.Closer
	closers = append(closers, chunkw, indexw)
	defer func() {
		errs := tsdb_errors.NewMulti()
		for _, c := range closers {
			errs.Add(c.Close())
		}
	}()

	// Write all symbols.
	syms := indexr.Symbols()
	for syms.Next() {
		if err := indexw.AddSymbol(syms.At()); err != nil {
			return fmt.Errorf("add symbol: %w", err)
		}
	}
	if err := syms.Err(); err != nil {
		return fmt.Errorf("symbols iterator: %w", err)
	}

	// Iterate all series.
	k, v := index.AllPostingsKey()
	all, err := indexr.Postings(ctx, k, v)
	if err != nil {
		return fmt.Errorf("get all postings: %w", err)
	}
	all = indexr.SortedPostings(all)

	var (
		ref     = storage.SeriesRef(0)
		builder labels.ScratchBuilder
		chks    []chunks.Meta
	)

	for all.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := indexr.Series(all.At(), &builder, &chks); err != nil {
			return fmt.Errorf("read series: %w", err)
		}

		newChks := make([]chunks.Meta, 0, len(chks))
		for _, chk := range chks {
			c, _, err := chunkr.ChunkOrIterable(chk)
			if err != nil {
				return fmt.Errorf("read chunk: %w", err)
			}

			// Re-encode float chunks if needed.
			if isFloatEncoding(c.Encoding()) && c.Encoding() != targetEncoding {
				reencoded, err := reencodeFloatChunk(c, targetEncoding)
				if err != nil {
					return fmt.Errorf("re-encode chunk: %w", err)
				}
				for _, rc := range reencoded {
					newChks = append(newChks, chunks.Meta{
						Chunk:   rc.chunk,
						MinTime: rc.minTime,
						MaxTime: rc.maxTime,
					})
				}
			} else {
				newChks = append(newChks, chunks.Meta{
					Chunk:   c,
					MinTime: chk.MinTime,
					MaxTime: chk.MaxTime,
				})
			}
		}

		if len(newChks) == 0 {
			continue
		}

		if err := chunkw.WriteChunks(newChks...); err != nil {
			return fmt.Errorf("write chunks: %w", err)
		}
		if err := indexw.AddSeries(ref, builder.Labels(), newChks...); err != nil {
			return fmt.Errorf("add series: %w", err)
		}

		meta.Stats.NumChunks += uint64(len(newChks))
		meta.Stats.NumSeries++
		for _, chk := range newChks {
			samples := uint64(chk.Chunk.NumSamples())
			meta.Stats.NumSamples += samples
			switch chk.Chunk.Encoding() {
			case chunkenc.EncHistogram, chunkenc.EncFloatHistogram:
				meta.Stats.NumHistogramSamples += samples
			case chunkenc.EncXOR, chunkenc.EncXOR2:
				meta.Stats.NumFloatSamples += samples
			}
		}

		for _, chk := range newChks {
			if err := pool.Put(chk.Chunk); err != nil {
				return fmt.Errorf("put chunk: %w", err)
			}
		}
		ref++
	}
	if err := all.Err(); err != nil {
		return fmt.Errorf("postings iterator: %w", err)
	}

	// Explicitly close writers to check for errors.
	errs := tsdb_errors.NewMulti()
	for _, c := range closers {
		errs.Add(c.Close())
	}
	closers = closers[:0]
	return errs.Err()
}

type reencodedChunk struct {
	chunk   chunkenc.Chunk
	minTime int64
	maxTime int64
}

// isFloatEncoding returns true if the encoding is a float chunk encoding.
func isFloatEncoding(e chunkenc.Encoding) bool {
	return e == chunkenc.EncXOR || e == chunkenc.EncXOR2
}

// reencodeFloatChunk re-encodes a float chunk to the target encoding.
func reencodeFloatChunk(src chunkenc.Chunk, targetEncoding chunkenc.Encoding) ([]reencodedChunk, error) {
	iter := src.Iterator(nil)

	newChunk, err := chunkenc.NewEmptyChunk(targetEncoding)
	if err != nil {
		return nil, err
	}
	app, err := newChunk.Appender()
	if err != nil {
		return nil, err
	}

	var (
		result  []reencodedChunk
		minTime int64
		lastTime int64
		first   = true
	)

	for iter.Next() == chunkenc.ValFloat {
		t, v := iter.At()
		// Check if the chunk is full (reached the max byte size).
		if newChunk.NumSamples() > 0 && len(newChunk.Bytes()) >= chunkenc.MaxBytesPerXORChunk {
			result = append(result, reencodedChunk{
				chunk:   newChunk,
				minTime: minTime,
				maxTime: lastTime,
			})
			newChunk, err = chunkenc.NewEmptyChunk(targetEncoding)
			if err != nil {
				return nil, err
			}
			app, err = newChunk.Appender()
			if err != nil {
				return nil, err
			}
			first = true
		}

		if first {
			minTime = t
			first = false
		}
		app.Append(0, t, v)
		lastTime = t
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	if newChunk.NumSamples() > 0 {
		result = append(result, reencodedChunk{
			chunk:   newChunk,
			minTime: minTime,
			maxTime: lastTime,
		})
	}

	return result, nil
}
