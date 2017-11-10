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
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/labels"
)

// BlockReader provides reading access to a data block.
type BlockReader interface {
	// Index returns an IndexReader over the block's data.
	Index() (IndexReader, error)

	// Chunks returns a ChunkReader over the block's data.
	Chunks() (ChunkReader, error)

	// Tombstones returns a TombstoneReader over the block's deleted data.
	Tombstones() (TombstoneReader, error)
}

// Appendable defines an entity to which data can be appended.
type Appendable interface {
	// Appender returns a new Appender against an underlying store.
	Appender() Appender
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
}

// BlockStats contains stats about contents of a block.
type BlockStats struct {
	NumSamples    uint64 `json:"numSamples,omitempty"`
	NumSeries     uint64 `json:"numSeries,omitempty"`
	NumChunks     uint64 `json:"numChunks,omitempty"`
	NumTombstones uint64 `json:"numTombstones,omitempty"`
}

// BlockMetaCompaction holds information about compactions a block went through.
type BlockMetaCompaction struct {
	// Maximum number of compaction cycles any source block has
	// gone through.
	Level int `json:"level"`
	// ULIDs of all source head blocks that went into the block.
	Sources []ulid.ULID `json:"sources,omitempty"`
}

const (
	flagNone = 0
	flagStd  = 1
)

type blockMeta struct {
	Version int `json:"version"`

	*BlockMeta
}

const metaFilename = "meta.json"

func readMetaFile(dir string) (*BlockMeta, error) {
	b, err := ioutil.ReadFile(filepath.Join(dir, metaFilename))
	if err != nil {
		return nil, err
	}
	var m blockMeta

	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Version != 1 {
		return nil, errors.Errorf("unexpected meta file version %d", m.Version)
	}

	return m.BlockMeta, nil
}

func writeMetaFile(dir string, meta *BlockMeta) error {
	// Make any changes to the file appear atomic.
	path := filepath.Join(dir, metaFilename)
	tmp := path + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "\t")

	var merr MultiError
	if merr.Add(enc.Encode(&blockMeta{Version: 1, BlockMeta: meta})); merr.Err() != nil {
		merr.Add(f.Close())
		return merr.Err()
	}
	if err := f.Close(); err != nil {
		return err
	}
	return renameFile(tmp, path)
}

// Block represents a directory of time series data covering a continuous time range.
type Block struct {
	mtx            sync.RWMutex
	closing        bool
	pendingReaders sync.WaitGroup

	dir  string
	meta BlockMeta

	chunkr ChunkReader
	indexr IndexReader

	tombstones tombstoneReader
}

// OpenBlock opens the block in the directory. It can be passed a chunk pool, which is used
// to instantiate chunk structs.
func OpenBlock(dir string, pool chunks.Pool) (*Block, error) {
	meta, err := readMetaFile(dir)
	if err != nil {
		return nil, err
	}

	cr, err := NewDirChunkReader(chunkDir(dir), pool)
	if err != nil {
		return nil, err
	}
	ir, err := NewFileIndexReader(filepath.Join(dir, "index"))
	if err != nil {
		return nil, err
	}

	tr, err := readTombstones(dir)
	if err != nil {
		return nil, err
	}

	pb := &Block{
		dir:        dir,
		meta:       *meta,
		chunkr:     cr,
		indexr:     ir,
		tombstones: tr,
	}
	return pb, nil
}

// Close closes the on-disk block. It blocks as long as there are readers reading from the block.
func (pb *Block) Close() error {
	pb.mtx.Lock()
	pb.closing = true
	pb.mtx.Unlock()

	pb.pendingReaders.Wait()

	var merr MultiError

	merr.Add(pb.chunkr.Close())
	merr.Add(pb.indexr.Close())
	merr.Add(pb.tombstones.Close())

	return merr.Err()
}

func (pb *Block) String() string {
	return pb.meta.ULID.String()
}

// Dir returns the directory of the block.
func (pb *Block) Dir() string { return pb.dir }

// Meta returns meta information about the block.
func (pb *Block) Meta() BlockMeta { return pb.meta }

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
	return blockIndexReader{IndexReader: pb.indexr, b: pb}, nil
}

// Chunks returns a new ChunkReader against the block data.
func (pb *Block) Chunks() (ChunkReader, error) {
	if err := pb.startRead(); err != nil {
		return nil, err
	}
	return blockChunkReader{ChunkReader: pb.chunkr, b: pb}, nil
}

// Tombstones returns a new TombstoneReader against the block data.
func (pb *Block) Tombstones() (TombstoneReader, error) {
	if err := pb.startRead(); err != nil {
		return nil, err
	}
	return blockTombstoneReader{TombstoneReader: pb.tombstones, b: pb}, nil
}

type blockIndexReader struct {
	IndexReader
	b *Block
}

func (r blockIndexReader) Close() error {
	r.b.pendingReaders.Done()
	return nil
}

type blockTombstoneReader struct {
	TombstoneReader
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
func (pb *Block) Delete(mint, maxt int64, ms ...labels.Matcher) error {
	pb.mtx.Lock()
	defer pb.mtx.Unlock()

	if pb.closing {
		return ErrClosing
	}

	pr := newPostingsReader(pb.indexr)
	p, absent := pr.Select(ms...)

	ir := pb.indexr

	// Choose only valid postings which have chunks in the time-range.
	stones := map[uint64]Intervals{}

	var lset labels.Labels
	var chks []ChunkMeta

Outer:
	for p.Next() {
		err := ir.Series(p.At(), &lset, &chks)
		if err != nil {
			return err
		}

		for _, abs := range absent {
			if lset.Get(abs) != "" {
				continue Outer
			}
		}

		for _, chk := range chks {
			if intervalOverlap(mint, maxt, chk.MinTime, chk.MaxTime) {
				// Delete only until the current vlaues and not beyond.
				tmin, tmax := clampInterval(mint, maxt, chks[0].MinTime, chks[len(chks)-1].MaxTime)
				stones[p.At()] = Intervals{{tmin, tmax}}
				continue Outer
			}
		}
	}

	if p.Err() != nil {
		return p.Err()
	}

	// Merge the current and new tombstones.
	for k, v := range stones {
		pb.tombstones.add(k, v[0])
	}

	if err := writeTombstoneFile(pb.dir, pb.tombstones); err != nil {
		return err
	}

	pb.meta.Stats.NumTombstones = uint64(len(pb.tombstones))
	return writeMetaFile(pb.dir, &pb.meta)
}

// Snapshot creates snapshot of the block into dir.
func (pb *Block) Snapshot(dir string) error {
	blockDir := filepath.Join(dir, pb.meta.ULID.String())
	if err := os.MkdirAll(blockDir, 0777); err != nil {
		return errors.Wrap(err, "create snapshot block dir")
	}

	chunksDir := chunkDir(blockDir)
	if err := os.MkdirAll(chunksDir, 0777); err != nil {
		return errors.Wrap(err, "create snapshot chunk dir")
	}

	// Hardlink meta, index and tombstones
	for _, fname := range []string{
		metaFilename,
		indexFilename,
		tombstoneFilename,
	} {
		if err := os.Link(filepath.Join(pb.dir, fname), filepath.Join(blockDir, fname)); err != nil {
			return errors.Wrapf(err, "create snapshot %s", fname)
		}
	}

	// Hardlink the chunks
	curChunkDir := chunkDir(pb.dir)
	files, err := ioutil.ReadDir(curChunkDir)
	if err != nil {
		return errors.Wrap(err, "ReadDir the current chunk dir")
	}

	for _, f := range files {
		err := os.Link(filepath.Join(curChunkDir, f.Name()), filepath.Join(chunksDir, f.Name()))
		if err != nil {
			return errors.Wrap(err, "hardlink a chunk")
		}
	}

	return nil
}

func chunkDir(dir string) string { return filepath.Join(dir, "chunks") }
func walDir(dir string) string   { return filepath.Join(dir, "wal") }

func clampInterval(a, b, mint, maxt int64) (int64, int64) {
	if a < mint {
		a = mint
	}
	if b > maxt {
		b = maxt
	}
	return a, b
}

type mmapFile struct {
	f *os.File
	b []byte
}

func openMmapFile(path string) (*mmapFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "try lock file")
	}
	info, err := f.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "stat")
	}

	b, err := mmap(f, int(info.Size()))
	if err != nil {
		return nil, errors.Wrap(err, "mmap")
	}

	return &mmapFile{f: f, b: b}, nil
}

func (f *mmapFile) Close() error {
	err0 := munmap(f.b)
	err1 := f.f.Close()

	if err0 != nil {
		return err0
	}
	return err1
}
