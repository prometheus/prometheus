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

package tsdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/oklog/ulid/v2"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

// IndexWriter serializes the index for a block of series data.
// The methods must be called in the order they are specified in.
type IndexWriter interface {
	// AddSymbol registers a single symbol.
	// Symbols must be registered in sorted order.
	AddSymbol(sym string) error

	// AddSeries populates the index writer with a series and its offsets
	// of chunks that the index can reference.
	// Implementations may require series to be insert in strictly increasing order by
	// their labels. The reference numbers are used to resolve entries in postings lists
	// that are added later.
	AddSeries(ref storage.SeriesRef, l labels.Labels, chunks ...chunks.Meta) error

	// Close writes any finalization and closes the resources associated with
	// the underlying writer.
	Close() error
}

// IndexReader provides reading access of serialized index data.
type IndexReader interface {
	// Symbols return an iterator over sorted string symbols that may occur in
	// series' labels and indices. It is not safe to use the returned strings
	// beyond the lifetime of the index reader.
	Symbols() index.StringIter

	// SortedLabelValues returns sorted possible label values.
	SortedLabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, error)

	// LabelValues returns possible label values which may not be sorted.
	LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, error)

	// Postings returns the postings list iterator for the label pairs.
	// The Postings here contain the offsets to the series inside the index.
	// Found IDs are not strictly required to point to a valid Series, e.g.
	// during background garbage collections.
	Postings(ctx context.Context, name string, values ...string) (index.Postings, error)

	// PostingsForLabelMatching returns a sorted iterator over postings having a label with the given name and a value for which match returns true.
	// If no postings are found having at least one matching label, an empty iterator is returned.
	PostingsForLabelMatching(ctx context.Context, name string, match func(value string) bool) index.Postings

	// PostingsForAllLabelValues returns a sorted iterator over all postings having a label with the given name.
	// If no postings are found with the label in question, an empty iterator is returned.
	PostingsForAllLabelValues(ctx context.Context, name string) index.Postings

	// SortedPostings returns a postings list that is reordered to be sorted
	// by the label set of the underlying series.
	SortedPostings(index.Postings) index.Postings

	// ShardedPostings returns a postings list filtered by the provided shardIndex
	// out of shardCount. For a given posting, its shard MUST be computed hashing
	// the series labels mod shardCount, using a hash function which is consistent over time.
	ShardedPostings(p index.Postings, shardIndex, shardCount uint64) index.Postings

	// Series populates the given builder and chunk metas for the series identified
	// by the reference.
	// Returns storage.ErrNotFound if the ref does not resolve to a known series.
	Series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error

	// LabelNames returns all the unique label names present in the index in sorted order.
	LabelNames(ctx context.Context, matchers ...*labels.Matcher) ([]string, error)

	// LabelNamesFor returns all the label names for the series referred to by the postings.
	// The names returned are sorted.
	LabelNamesFor(ctx context.Context, postings index.Postings) ([]string, error)

	// Close releases the underlying resources of the reader.
	Close() error
}

// ChunkWriter serializes a time block of chunked series data.
type ChunkWriter interface {
	// WriteChunks writes several chunks. The Chunk field of the ChunkMetas
	// must be populated.
	// After returning successfully, the Ref fields in the ChunkMetas
	// are set and can be used to retrieve the chunks from the written data.
	WriteChunks(chunks ...chunks.Meta) error

	// Close writes any required finalization and closes the resources
	// associated with the underlying writer.
	Close() error
}

// ChunkReader provides reading access of serialized time series data.
type ChunkReader interface {
	// ChunkOrIterable returns the series data for the given chunks.Meta.
	// Either a single chunk will be returned, or an iterable.
	// A single chunk should be returned if chunks.Meta maps to a chunk that
	// already exists and doesn't need modifications.
	// An iterable should be returned if chunks.Meta maps to a subset of the
	// samples in a stored chunk, or multiple chunks. (E.g. OOOHeadChunkReader
	// could return an iterable where multiple histogram samples have counter
	// resets. There can only be one counter reset per histogram chunk so
	// multiple chunks would be created from the iterable in this case.)
	// Only one of chunk or iterable should be returned. In some cases you may
	// always expect a chunk to be returned. You can check that iterable is nil
	// in those cases.
	ChunkOrIterable(meta chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, error)

	// Close releases all underlying resources of the reader.
	Close() error
}

// BlockReader provides reading access to a data block.
type BlockReader interface {
	// Index returns an IndexReader over the block's data.
	Index() (IndexReader, error)

	// Chunks returns a ChunkReader over the block's data.
	Chunks() (ChunkReader, error)

	// Tombstones returns a tombstones.Reader over the block's deleted data.
	Tombstones() (tombstones.Reader, error)

	// Meta provides meta information about the block reader.
	Meta() BlockMeta

	// Size returns the number of bytes that the block takes up on disk.
	Size() int64
}

// BlockMeta provides meta information about a block.
type BlockMeta struct {
	// Unique identifier for the block and its contents. Changes on compaction.
	ULID ulid.ULID `json:"ulid"`

	// MinTime and MaxTime specify the time range all samples
	// in the block are in.
	MinTime int64 `json:"minTime"`
	MaxTime int64 `json:"maxTime"`

	// Stats about the contents of the block.
	Stats BlockStats `json:"stats,omitempty"`

	// Information on compactions the block was created from.
	Compaction BlockMetaCompaction `json:"compaction"`

	// Version of the index format.
	Version int `json:"version"`
}

// BlockStats contains stats about contents of a block.
type BlockStats struct {
	NumSamples          uint64 `json:"numSamples,omitempty"`
	NumFloatSamples     uint64 `json:"numFloatSamples,omitempty"`
	NumHistogramSamples uint64 `json:"numHistogramSamples,omitempty"`
	NumSeries           uint64 `json:"numSeries,omitempty"`
	NumChunks           uint64 `json:"numChunks,omitempty"`
	NumTombstones       uint64 `json:"numTombstones,omitempty"`
}

// BlockDesc describes a block by ULID and time range.
type BlockDesc struct {
	ULID    ulid.ULID `json:"ulid"`
	MinTime int64     `json:"minTime"`
	MaxTime int64     `json:"maxTime"`
}

// BlockMetaCompaction holds information about compactions a block went through.
type BlockMetaCompaction struct {
	// Maximum number of compaction cycles any source block has
	// gone through.
	Level int `json:"level"`
	// ULIDs of all source head blocks that went into the block.
	Sources []ulid.ULID `json:"sources,omitempty"`
	// Indicates that during compaction it resulted in a block without any samples
	// so it should be deleted on the next reloadBlocks.
	Deletable bool `json:"deletable,omitempty"`
	// Short descriptions of the direct blocks that were used to create
	// this block.
	Parents []BlockDesc `json:"parents,omitempty"`
	Failed  bool        `json:"failed,omitempty"`
	// Additional information about the compaction, for example, block created from out-of-order chunks.
	Hints []string `json:"hints,omitempty"`
}

func (bm *BlockMetaCompaction) SetOutOfOrder() {
	if bm.FromOutOfOrder() {
		return
	}
	bm.Hints = append(bm.Hints, CompactionHintFromOutOfOrder)
	slices.Sort(bm.Hints)
}

func (bm *BlockMetaCompaction) FromOutOfOrder() bool {
	return slices.Contains(bm.Hints, CompactionHintFromOutOfOrder)
}

const (
	indexFilename = "index"
	metaFilename  = "meta.json"
	metaVersion1  = 1

	// CompactionHintFromOutOfOrder is a hint noting that the block
	// was created from out-of-order chunks.
	CompactionHintFromOutOfOrder = "from-out-of-order"
)

func chunkDir(dir string) string { return filepath.Join(dir, "chunks") }

func readMetaFile(dir string) (*BlockMeta, int64, error) {
	b, err := os.ReadFile(filepath.Join(dir, metaFilename))
	if err != nil {
		return nil, 0, err
	}
	var m BlockMeta

	if err := json.Unmarshal(b, &m); err != nil {
		return nil, 0, err
	}
	if m.Version != metaVersion1 {
		return nil, 0, fmt.Errorf("unexpected meta file version %d", m.Version)
	}

	return &m, int64(len(b)), nil
}

func writeMetaFile(logger *slog.Logger, dir string, meta *BlockMeta) (int64, error) {
	meta.Version = metaVersion1

	// Make any changes to the file appear atomic.
	path := filepath.Join(dir, metaFilename)
	tmp := path + ".tmp"
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			logger.Error("remove tmp file", "err", err.Error())
		}
	}()

	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}

	jsonMeta, err := json.MarshalIndent(meta, "", "\t")
	if err != nil {
		return 0, err
	}

	n, err := f.Write(jsonMeta)
	if err != nil {
		return 0, tsdb_errors.NewMulti(err, f.Close()).Err()
	}

	// Force the kernel to persist the file on disk to avoid data loss if the host crashes.
	if err := f.Sync(); err != nil {
		return 0, tsdb_errors.NewMulti(err, f.Close()).Err()
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	return int64(n), fileutil.Replace(tmp, path)
}

// Block represents a directory of time series data covering a continuous time range.
type Block struct {
	mtx            sync.RWMutex
	closing        bool
	pendingReaders sync.WaitGroup

	dir  string
	meta BlockMeta

	// Symbol Table Size in bytes.
	// We maintain this variable to avoid recalculation every time.
	symbolTableSize uint64

	chunkr     ChunkReader
	indexr     IndexReader
	tombstones tombstones.Reader

	logger *slog.Logger

	numBytesChunks    int64
	numBytesIndex     int64
	numBytesTombstone int64
	numBytesMeta      int64
}

// OpenBlock opens the block in the directory. It can be passed a chunk pool, which is used
// to instantiate chunk structs.
func OpenBlock(logger *slog.Logger, dir string, pool chunkenc.Pool, postingsDecoderFactory PostingsDecoderFactory) (pb *Block, err error) {
	if logger == nil {
		logger = promslog.NewNopLogger()
	}
	var closers []io.Closer
	defer func() {
		if err != nil {
			err = tsdb_errors.NewMulti(err, tsdb_errors.CloseAll(closers)).Err()
		}
	}()
	meta, sizeMeta, err := readMetaFile(dir)
	if err != nil {
		return nil, err
	}

	cr, err := chunks.NewDirReader(chunkDir(dir), pool)
	if err != nil {
		return nil, err
	}
	closers = append(closers, cr)

	decoder := index.DecodePostingsRaw
	if postingsDecoderFactory != nil {
		decoder = postingsDecoderFactory(meta)
	}
	ir, err := index.NewFileReader(filepath.Join(dir, indexFilename), decoder)
	if err != nil {
		return nil, err
	}
	closers = append(closers, ir)

	tr, sizeTomb, err := tombstones.ReadTombstones(dir)
	if err != nil {
		return nil, err
	}
	closers = append(closers, tr)

	pb = &Block{
		dir:               dir,
		meta:              *meta,
		chunkr:            cr,
		indexr:            ir,
		tombstones:        tr,
		symbolTableSize:   ir.SymbolTableSize(),
		logger:            logger,
		numBytesChunks:    cr.Size(),
		numBytesIndex:     ir.Size(),
		numBytesTombstone: sizeTomb,
		numBytesMeta:      sizeMeta,
	}
	return pb, nil
}

// Close closes the on-disk block. It blocks as long as there are readers reading from the block.
func (pb *Block) Close() error {
	pb.mtx.Lock()
	pb.closing = true
	pb.mtx.Unlock()

	pb.pendingReaders.Wait()

	return tsdb_errors.NewMulti(
		pb.chunkr.Close(),
		pb.indexr.Close(),
		pb.tombstones.Close(),
	).Err()
}

func (pb *Block) String() string {
	return pb.meta.ULID.String()
}

// Dir returns the directory of the block.
func (pb *Block) Dir() string { return pb.dir }

// Meta returns meta information about the block.
func (pb *Block) Meta() BlockMeta { return pb.meta }

// MinTime returns the min time of the meta.
func (pb *Block) MinTime() int64 { return pb.meta.MinTime }

// MaxTime returns the max time of the meta.
func (pb *Block) MaxTime() int64 { return pb.meta.MaxTime }

// Size returns the number of bytes that the block takes up.
func (pb *Block) Size() int64 {
	return pb.numBytesChunks + pb.numBytesIndex + pb.numBytesTombstone + pb.numBytesMeta
}

// ErrClosing is returned when a block is in the process of being closed.
var ErrClosing = errors.New("block is closing")

func (pb *Block) startRead() error {
	pb.mtx.RLock()
	defer pb.mtx.RUnlock()

	if pb.closing {
		return ErrClosing
	}
	pb.pendingReaders.Add(1)
	return nil
}

// Index returns a new IndexReader against the block data.
func (pb *Block) Index() (IndexReader, error) {
	if err := pb.startRead(); err != nil {
		return nil, err
	}
	return blockIndexReader{ir: pb.indexr, b: pb}, nil
}

// Chunks returns a new ChunkReader against the block data.
func (pb *Block) Chunks() (ChunkReader, error) {
	if err := pb.startRead(); err != nil {
		return nil, err
	}
	return blockChunkReader{ChunkReader: pb.chunkr, b: pb}, nil
}

// Tombstones returns a new TombstoneReader against the block data.
func (pb *Block) Tombstones() (tombstones.Reader, error) {
	if err := pb.startRead(); err != nil {
		return nil, err
	}
	return blockTombstoneReader{Reader: pb.tombstones, b: pb}, nil
}

// GetSymbolTableSize returns the Symbol Table Size in the index of this block.
func (pb *Block) GetSymbolTableSize() uint64 {
	return pb.symbolTableSize
}

func (pb *Block) setCompactionFailed() error {
	pb.meta.Compaction.Failed = true
	n, err := writeMetaFile(pb.logger, pb.dir, &pb.meta)
	if err != nil {
		return err
	}
	pb.numBytesMeta = n
	return nil
}

type blockIndexReader struct {
	ir IndexReader
	b  *Block
}

func (r blockIndexReader) Symbols() index.StringIter {
	return r.ir.Symbols()
}

func (r blockIndexReader) SortedLabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, error) {
	var st []string
	var err error

	if len(matchers) == 0 {
		st, err = r.ir.SortedLabelValues(ctx, name, hints)
	} else {
		st, err = r.LabelValues(ctx, name, hints, matchers...)
		if err == nil {
			slices.Sort(st)
		}
	}
	if err != nil {
		return st, fmt.Errorf("block: %s: %w", r.b.Meta().ULID, err)
	}
	return st, nil
}

func (r blockIndexReader) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, error) {
	if len(matchers) == 0 {
		st, err := r.ir.LabelValues(ctx, name, hints)
		if err != nil {
			return st, fmt.Errorf("block: %s: %w", r.b.Meta().ULID, err)
		}
		return st, nil
	}

	return labelValuesWithMatchers(ctx, r.ir, name, hints, matchers...)
}

func (r blockIndexReader) LabelNames(ctx context.Context, matchers ...*labels.Matcher) ([]string, error) {
	if len(matchers) == 0 {
		return r.b.LabelNames(ctx)
	}

	return labelNamesWithMatchers(ctx, r.ir, matchers...)
}

func (r blockIndexReader) Postings(ctx context.Context, name string, values ...string) (index.Postings, error) {
	p, err := r.ir.Postings(ctx, name, values...)
	if err != nil {
		return p, fmt.Errorf("block: %s: %w", r.b.Meta().ULID, err)
	}
	return p, nil
}

func (r blockIndexReader) PostingsForLabelMatching(ctx context.Context, name string, match func(string) bool) index.Postings {
	return r.ir.PostingsForLabelMatching(ctx, name, match)
}

func (r blockIndexReader) PostingsForAllLabelValues(ctx context.Context, name string) index.Postings {
	return r.ir.PostingsForAllLabelValues(ctx, name)
}

func (r blockIndexReader) SortedPostings(p index.Postings) index.Postings {
	return r.ir.SortedPostings(p)
}

func (r blockIndexReader) ShardedPostings(p index.Postings, shardIndex, shardCount uint64) index.Postings {
	return r.ir.ShardedPostings(p, shardIndex, shardCount)
}

func (r blockIndexReader) Series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	if err := r.ir.Series(ref, builder, chks); err != nil {
		return fmt.Errorf("block: %s: %w", r.b.Meta().ULID, err)
	}
	return nil
}

func (r blockIndexReader) Close() error {
	r.b.pendingReaders.Done()
	return nil
}

// LabelNamesFor returns all the label names for the series referred to by the postings.
// The names returned are sorted.
func (r blockIndexReader) LabelNamesFor(ctx context.Context, postings index.Postings) ([]string, error) {
	return r.ir.LabelNamesFor(ctx, postings)
}

type blockTombstoneReader struct {
	tombstones.Reader
	b *Block
}

func (r blockTombstoneReader) Close() error {
	r.b.pendingReaders.Done()
	return nil
}

type blockChunkReader struct {
	ChunkReader
	b *Block
}

func (r blockChunkReader) Close() error {
	r.b.pendingReaders.Done()
	return nil
}

// Delete matching series between mint and maxt in the block.
func (pb *Block) Delete(ctx context.Context, mint, maxt int64, ms ...*labels.Matcher) error {
	pb.mtx.Lock()
	defer pb.mtx.Unlock()

	if pb.closing {
		return ErrClosing
	}

	p, err := PostingsForMatchers(ctx, pb.indexr, ms...)
	if err != nil {
		return fmt.Errorf("select series: %w", err)
	}

	ir := pb.indexr

	// Choose only valid postings which have chunks in the time-range.
	stones := tombstones.NewMemTombstones()

	var chks []chunks.Meta
	var builder labels.ScratchBuilder

Outer:
	for p.Next() {
		err := ir.Series(p.At(), &builder, &chks)
		if err != nil {
			return err
		}

		for _, chk := range chks {
			if chk.OverlapsClosedInterval(mint, maxt) {
				// Delete only until the current values and not beyond.
				tmin, tmax := clampInterval(mint, maxt, chks[0].MinTime, chks[len(chks)-1].MaxTime)
				stones.AddInterval(p.At(), tombstones.Interval{Mint: tmin, Maxt: tmax})
				continue Outer
			}
		}
	}

	if p.Err() != nil {
		return p.Err()
	}

	err = pb.tombstones.Iter(func(id storage.SeriesRef, ivs tombstones.Intervals) error {
		for _, iv := range ivs {
			stones.AddInterval(id, iv)
		}
		return nil
	})
	if err != nil {
		return err
	}
	pb.tombstones = stones
	pb.meta.Stats.NumTombstones = pb.tombstones.Total()

	n, err := tombstones.WriteFile(pb.logger, pb.dir, pb.tombstones)
	if err != nil {
		return err
	}
	pb.numBytesTombstone = n
	n, err = writeMetaFile(pb.logger, pb.dir, &pb.meta)
	if err != nil {
		return err
	}
	pb.numBytesMeta = n
	return nil
}

// CleanTombstones will remove the tombstones and rewrite the block (only if there are any tombstones).
// If there was a rewrite, then it returns the ULID of new blocks written, else nil.
// If a resultant block is empty (tombstones covered the whole block), then it returns an empty slice.
// It returns a boolean indicating if the parent block can be deleted safely of not.
func (pb *Block) CleanTombstones(dest string, c Compactor) ([]ulid.ULID, bool, error) {
	numStones := 0

	if err := pb.tombstones.Iter(func(_ storage.SeriesRef, ivs tombstones.Intervals) error {
		numStones += len(ivs)
		return nil
	}); err != nil {
		// This should never happen, as the iteration function only returns nil.
		panic(err)
	}
	if numStones == 0 {
		return nil, false, nil
	}

	meta := pb.Meta()
	uids, err := c.Write(dest, pb, pb.meta.MinTime, pb.meta.MaxTime, &meta)
	if err != nil {
		return nil, false, err
	}

	return uids, true, nil
}

// Snapshot creates snapshot of the block into dir.
func (pb *Block) Snapshot(dir string) error {
	blockDir := filepath.Join(dir, pb.meta.ULID.String())
	if err := os.MkdirAll(blockDir, 0o777); err != nil {
		return fmt.Errorf("create snapshot block dir: %w", err)
	}

	chunksDir := chunkDir(blockDir)
	if err := os.MkdirAll(chunksDir, 0o777); err != nil {
		return fmt.Errorf("create snapshot chunk dir: %w", err)
	}

	// Hardlink meta, index and tombstones
	for _, fname := range []string{
		metaFilename,
		indexFilename,
		tombstones.TombstonesFilename,
	} {
		if err := os.Link(filepath.Join(pb.dir, fname), filepath.Join(blockDir, fname)); err != nil {
			return fmt.Errorf("create snapshot %s: %w", fname, err)
		}
	}

	// Hardlink the chunks
	curChunkDir := chunkDir(pb.dir)
	files, err := os.ReadDir(curChunkDir)
	if err != nil {
		return fmt.Errorf("ReadDir the current chunk dir: %w", err)
	}

	for _, f := range files {
		err := os.Link(filepath.Join(curChunkDir, f.Name()), filepath.Join(chunksDir, f.Name()))
		if err != nil {
			return fmt.Errorf("hardlink a chunk: %w", err)
		}
	}

	return nil
}

// OverlapsClosedInterval returns true if the block overlaps [mint, maxt].
func (pb *Block) OverlapsClosedInterval(mint, maxt int64) bool {
	// The block itself is a half-open interval
	// [pb.meta.MinTime, pb.meta.MaxTime).
	return pb.meta.MinTime <= maxt && mint < pb.meta.MaxTime
}

// LabelNames returns all the unique label names present in the Block in sorted order.
func (pb *Block) LabelNames(ctx context.Context) ([]string, error) {
	return pb.indexr.LabelNames(ctx)
}

func clampInterval(a, b, mint, maxt int64) (int64, int64) {
	if a < mint {
		a = mint
	}
	if b > maxt {
		b = maxt
	}
	return a, b
}
