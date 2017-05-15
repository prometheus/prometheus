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
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/labels"
)

// DiskBlock handles reads against a Block of time series data.
type DiskBlock interface {
	// Directory where block data is stored.
	Dir() string

	// Stats returns statistics about the block.
	Meta() BlockMeta

	// Index returns an IndexReader over the block's data.
	Index() IndexReader

	// Chunks returns a ChunkReader over the block's data.
	Chunks() ChunkReader

	// Delete deletes data from the block.
	Delete(mint, maxt int64, ms ...labels.Matcher) error

	// Close releases all underlying resources of the block.
	Close() error
}

// Block is an interface to a DiskBlock that can also be queried.
type Block interface {
	DiskBlock
	Queryable
}

// headBlock is a regular block that can still be appended to.
type headBlock interface {
	Block
	Appendable
}

// Appendable defines an entity to which data can be appended.
type Appendable interface {
	// Appender returns a new Appender against an underlying store.
	Appender() Appender

	// Busy returns whether there are any currently active appenders.
	Busy() bool
}

// Queryable defines an entity which provides a Querier.
type Queryable interface {
	Querier(mint, maxt int64) Querier
}

// BlockMeta provides meta information about a block.
type BlockMeta struct {
	// Unique identifier for the block and its contents. Changes on compaction.
	ULID ulid.ULID `json:"ulid"`

	// Sequence number of the block.
	Sequence int `json:"sequence"`

	// MinTime and MaxTime specify the time range all samples
	// in the block are in.
	MinTime int64 `json:"minTime"`
	MaxTime int64 `json:"maxTime"`

	// Stats about the contents of the block.
	Stats struct {
		NumSamples uint64 `json:"numSamples,omitempty"`
		NumSeries  uint64 `json:"numSeries,omitempty"`
		NumChunks  uint64 `json:"numChunks,omitempty"`
	} `json:"stats,omitempty"`

	// Information on compactions the block was created from.
	Compaction struct {
		Generation int `json:"generation"`
	} `json:"compaction"`
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
const tombstoneFilename = "tombstones"

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
		return merr
	}
	if err := f.Close(); err != nil {
		return err
	}
	return renameFile(tmp, path)
}

func readTombstoneFile(dir string) (TombstoneReader, error) {
	return newTombStoneReader(dir)
}

func writeTombstoneFile(dir string, tr TombstoneReader) error {
	path := filepath.Join(dir, tombstoneFilename)
	tmp := path + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	stoneOff := make(map[uint32]int64) // The map that holds the ref to offset vals.
	refs := []uint32{}                 // Sorted refs.

	pos := int64(0)
	buf := encbuf{b: make([]byte, 2*binary.MaxVarintLen64)}
	for tr.Next() {
		s := tr.At()

		refs = append(refs, s.ref)
		stoneOff[s.ref] = pos

		// Write the ranges.
		buf.reset()
		buf.putVarint64(int64(len(s.ranges)))
		n, err := f.Write(buf.get())
		if err != nil {
			return err
		}
		pos += int64(n)

		for _, r := range s.ranges {
			buf.reset()
			buf.putVarint64(r.mint)
			buf.putVarint64(r.maxt)
			n, err = f.Write(buf.get())
			if err != nil {
				return err
			}
			pos += int64(n)
		}
	}

	// Write the offset table.
	buf.reset()
	buf.putBE32int(len(refs))
	for _, ref := range refs {
		buf.reset()
		buf.putBE32(ref)
		buf.putBE64int64(stoneOff[ref])
		_, err = f.Write(buf.get())
		if err != nil {
			return err
		}
	}

	// Write the offset to the offset table.
	buf.reset()
	buf.putBE64int64(pos)
	_, err = f.Write(buf.get())
	if err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	return renameFile(tmp, path)
}

type persistedBlock struct {
	dir  string
	meta BlockMeta

	chunkr *chunkReader
	indexr *indexReader
}

func newPersistedBlock(dir string) (*persistedBlock, error) {
	meta, err := readMetaFile(dir)
	if err != nil {
		return nil, err
	}

	cr, err := newChunkReader(chunkDir(dir))
	if err != nil {
		return nil, err
	}
	ir, err := newIndexReader(dir)
	if err != nil {
		return nil, err
	}

	pb := &persistedBlock{
		dir:    dir,
		meta:   *meta,
		chunkr: cr,
		indexr: ir,
	}
	return pb, nil
}

func (pb *persistedBlock) Close() error {
	var merr MultiError

	merr.Add(pb.chunkr.Close())
	merr.Add(pb.indexr.Close())

	return merr.Err()
}

func (pb *persistedBlock) String() string {
	return fmt.Sprintf("(%d, %s)", pb.meta.Sequence, pb.meta.ULID)
}

func (pb *persistedBlock) Querier(mint, maxt int64) Querier {
	return &blockQuerier{
		mint:   mint,
		maxt:   maxt,
		index:  pb.Index(),
		chunks: pb.Chunks(),
	}
}

func (pb *persistedBlock) Dir() string         { return pb.dir }
func (pb *persistedBlock) Index() IndexReader  { return pb.indexr }
func (pb *persistedBlock) Chunks() ChunkReader { return pb.chunkr }
func (pb *persistedBlock) Meta() BlockMeta     { return pb.meta }

func (pb *persistedBlock) Delete(mint, maxt int64, ms ...labels.Matcher) error {
	pr := newPostingsReader(pb.indexr)
	p, absent := pr.Select(ms...)

	ir := pb.indexr

	// Choose only valid postings which have chunks in the time-range.
	vPostings := []uint32{}

Outer:
	for p.Next() {
		lset, chunks, err := ir.Series(p.At())
		if err != nil {
			return err
		}

		for _, abs := range absent {
			if lset.Get(abs) != "" {
				continue Outer
			}
		}

		// XXX(gouthamve): Adjust mint and maxt to match the time-range in the chunks?
		for _, chk := range chunks {
			if (mint <= chk.MinTime && maxt >= chk.MinTime) ||
				(mint > chk.MinTime && mint <= chk.MaxTime) {
				vPostings = append(vPostings, p.At())
				continue Outer
			}
		}
	}

	if p.Err() != nil {
		return p.Err()
	}

	// Merge the current and new tombstones.
	tr := newMapTombstoneReader(ir.tombstones)
	str := newSimpleTombstoneReader(vPostings, []trange{mint, maxt})
	tombreader := newMergedTombstoneReader(tr, str)

	return writeTombstoneFile(pb.dir, tombreader)
}

// stone holds the information on the posting and time-range
// that is deleted.
type stone struct {
	ref    uint32
	ranges []trange
}

// TombstoneReader is the iterator over tombstones.
type TombstoneReader interface {
	Next() bool
	At() stone
	Err() error
}

var emptyTombstoneReader = newMapTombstoneReader(make(map[uint32][]trange))

type tombstoneReader struct {
	stones []byte
	idx    int
	len    int

	b   []byte
	err error
}

func newTombStoneReader(dir string) (*tombstoneReader, error) {
	// TODO(gouthamve): MMAP?
	b, err := ioutil.ReadFile(filepath.Join(dir, tombstoneFilename))
	if err != nil {
		return nil, err
	}

	offsetBytes := b[len(b)-8:]
	d := &decbuf{b: offsetBytes}
	off := d.be64int64()
	if err := d.err(); err != nil {
		return nil, err
	}

	d = &decbuf{b: b[off:]}
	numStones := d.be64int64()
	if err := d.err(); err != nil {
		return nil, err
	}

	return &tombstoneReader{
		stones: b[off+8 : (off+8)+(numStones*12)],
		idx:    -1,
		len:    int(numStones),

		b: b,
	}, nil
}

func (t *tombstoneReader) Next() bool {
	if t.err != nil {
		return false
	}

	t.idx++

	return t.idx < t.len
}

func (t *tombstoneReader) At() stone {
	bytIdx := t.idx * (4 + 8)
	dat := t.stones[bytIdx : bytIdx+12]

	d := &decbuf{b: dat}
	ref := d.be32()
	off := d.be64int64()

	d = &decbuf{b: t.b[off:]}
	numRanges := d.varint64()
	if err := d.err(); err != nil {
		t.err = err
		return stone{ref: ref}
	}

	dranges := make([]trange, 0, numRanges)
	for i := 0; i < int(numRanges); i++ {
		mint := d.varint64()
		maxt := d.varint64()
		if err := d.err(); err != nil {
			t.err = err
			return stone{ref: ref, ranges: dranges}
		}

		dranges = append(dranges, trange{mint, maxt})
	}

	return stone{ref: ref, ranges: dranges}
}

func (t *tombstoneReader) Err() error {
	return t.err
}

type mapTombstoneReader struct {
	refs []uint32
	cur  uint32

	stones map[uint32][]trange
}

func newMapTombstoneReader(ts map[uint32][]trange) *mapTombstoneReader {
	refs := make([]uint32, 0, len(ts))
	for k := range ts {
		refs = append(refs, k)
	}
	sort.Sort(uint32slice(refs))
	return &mapTombstoneReader{stones: ts, refs: refs}
}

func (t *mapTombstoneReader) Next() bool {
	if len(t.refs) > 0 {
		t.cur = t.refs[0]
		return true
	}

	t.cur = 0
	return false
}

func (t *mapTombstoneReader) At() stone {
	return stone{ref: t.cur, ranges: t.stones[t.cur]}
}

func (t *mapTombstoneReader) Err() error {
	return nil
}

type simpleTombstoneReader struct {
	refs []uint32
	cur  uint32

	ranges []trange
}

func newSimpleTombstoneReader(refs []uint32, drange []trange) *simpleTombstoneReader {
	return &simpleTombstoneReader{refs: refs, ranges: drange}
}

func (t *simpleTombstoneReader) Next() bool {
	if len(t.refs) > 0 {
		t.cur = t.refs[0]
		return true
	}

	t.cur = 0
	return false
}

func (t *simpleTombstoneReader) At() stone {
	return stone{ref: t.cur, ranges: t.ranges}
}

func (t *simpleTombstoneReader) Err() error {
	return nil
}

type mergedTombstoneReader struct {
	a, b TombstoneReader
	cur  stone

	initialized bool
	aok, bok    bool
}

func newMergedTombstoneReader(a, b TombstoneReader) *mergedTombstoneReader {
	return &mergedTombstoneReader{a: a, b: b}
}

func (t *mergedTombstoneReader) Next() bool {
	if !t.initialized {
		t.aok = t.a.Next()
		t.bok = t.b.Next()
		t.initialized = true
	}

	if !t.aok && !t.bok {
		return false
	}

	if !t.aok {
		t.cur = t.b.At()
		t.bok = t.b.Next()
		return true
	}
	if !t.bok {
		t.cur = t.a.At()
		t.aok = t.a.Next()
		return true
	}

	acur, bcur := t.a.At(), t.b.At()

	if acur.ref < bcur.ref {
		t.cur = acur
		t.aok = t.a.Next()
	} else if acur.ref > bcur.ref {
		t.cur = bcur
		t.bok = t.b.Next()
	} else {
		t.cur = acur
		// Merge time ranges.
		for _, r := range bcur.ranges {
			acur.ranges = addNewInterval(acur.ranges, r)
		}
		t.aok = t.a.Next()
		t.bok = t.b.Next()
	}
	return true
}

func (t *mergedTombstoneReader) At() stone {
	return t.cur
}

func (t *mergedTombstoneReader) Err() error {
	if t.a.Err() != nil {
		return t.a.Err()
	}
	return t.b.Err()
}

func chunkDir(dir string) string { return filepath.Join(dir, "chunks") }
func walDir(dir string) string   { return filepath.Join(dir, "wal") }

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
