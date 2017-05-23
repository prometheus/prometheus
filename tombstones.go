package tsdb

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
)

const tombstoneFilename = "tombstones"

const (
	// MagicTombstone is 4 bytes at the head of a tombstone file.
	MagicTombstone = 0x130BA30

	tombstoneFormatV1 = 1
)

func readTombstoneFile(dir string) (TombstoneReader, error) {
	return newTombStoneReader(dir)
}

func writeTombstoneFile(dir string, tr TombstoneReader) error {
	path := filepath.Join(dir, tombstoneFilename)
	tmp := path + ".tmp"
	hash := crc32.New(crc32.MakeTable(crc32.Castagnoli))

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	stoneOff := make(map[uint32]int64) // The map that holds the ref to offset vals.
	refs := []uint32{}                 // Sorted refs.

	pos := int64(0)
	buf := encbuf{b: make([]byte, 2*binary.MaxVarintLen64)}
	buf.reset()
	// Write the meta.
	buf.putBE32(MagicTombstone)
	buf.putByte(tombstoneFormatV1)
	n, err := f.Write(buf.get())
	if err != nil {
		return err
	}
	pos += int64(n)

	for tr.Next() {
		s := tr.At()

		refs = append(refs, s.ref)
		stoneOff[s.ref] = pos

		// Write the ranges.
		buf.reset()
		buf.putUvarint(len(s.intervals))
		n, err := f.Write(buf.get())
		if err != nil {
			return err
		}
		pos += int64(n)

		buf.reset()
		for _, r := range s.intervals {
			buf.putVarint64(r.mint)
			buf.putVarint64(r.maxt)
		}
		buf.putHash(hash)

		n, err = f.Write(buf.get())
		if err != nil {
			return err
		}
		pos += int64(n)
	}
	if err := tr.Err(); err != nil {
		return err
	}

	// Write the offset table.
	// Pad first.
	if p := 4 - (int(pos) % 4); p != 0 {
		if _, err := f.Write(make([]byte, p)); err != nil {
			return err
		}

		pos += int64(p)
	}

	buf.reset()
	buf.putBE32int(len(refs))
	if _, err := f.Write(buf.get()); err != nil {
		return err
	}

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

// stone holds the information on the posting and time-range
// that is deleted.
type stone struct {
	ref       uint32
	intervals intervals
}

// TombstoneReader is the iterator over tombstones.
type TombstoneReader interface {
	Next() bool
	Seek(ref uint32) bool
	At() stone
	// Copy copies the current reader state. Changes to the copy will not affect parent.
	Copy() TombstoneReader
	Err() error
}

type tombstoneReader struct {
	stones []byte

	cur stone

	b   []byte
	err error
}

func newTombStoneReader(dir string) (*tombstoneReader, error) {
	// TODO(gouthamve): MMAP?
	b, err := ioutil.ReadFile(filepath.Join(dir, tombstoneFilename))
	if err != nil {
		return nil, err
	}

	d := &decbuf{b: b}
	if mg := d.be32(); mg != MagicTombstone {
		return nil, fmt.Errorf("invalid magic number %x", mg)
	}

	offsetBytes := b[len(b)-8:]
	d = &decbuf{b: offsetBytes}
	off := d.be64int64()
	if err := d.err(); err != nil {
		return nil, err
	}

	d = &decbuf{b: b[off:]}
	numStones := d.be32int()
	if err := d.err(); err != nil {
		return nil, err
	}
	off += 4 // For the numStones which has been read.

	return &tombstoneReader{
		stones: b[off : off+int64(numStones*12)],

		b: b,
	}, nil
}

func (t *tombstoneReader) Next() bool {
	if t.err != nil {
		return false
	}

	if len(t.stones) < 12 {
		return false
	}

	d := &decbuf{b: t.stones[:12]}
	ref := d.be32()
	off := d.be64int64()

	d = &decbuf{b: t.b[off:]}
	numRanges := d.uvarint()
	if err := d.err(); err != nil {
		t.err = err
		return false
	}

	dranges := make(intervals, 0, numRanges)
	for i := 0; i < int(numRanges); i++ {
		mint := d.varint64()
		maxt := d.varint64()
		if err := d.err(); err != nil {
			t.err = err
			return false
		}

		dranges = append(dranges, interval{mint, maxt})
	}

	// TODO(gouthamve): Verify checksum.
	t.stones = t.stones[12:]
	t.cur = stone{ref: ref, intervals: dranges}
	return true
}

func (t *tombstoneReader) Seek(ref uint32) bool {
	i := sort.Search(len(t.stones)/12, func(i int) bool {
		x := binary.BigEndian.Uint32(t.stones[i*12:])
		return x >= ref
	})

	if i*12 < len(t.stones) {
		t.stones = t.stones[i*12:]
		return t.Next()
	}

	t.stones = nil
	return false
}

func (t *tombstoneReader) At() stone {
	return t.cur
}

func (t *tombstoneReader) Copy() TombstoneReader {
	return &tombstoneReader{
		stones: t.stones[:],
		cur:    t.cur,

		b: t.b,
	}
}

func (t *tombstoneReader) Err() error {
	return t.err
}

type mapTombstoneReader struct {
	refs []uint32
	cur  uint32

	stones map[uint32]intervals
}

func newMapTombstoneReader(ts map[uint32]intervals) *mapTombstoneReader {
	refs := make([]uint32, 0, len(ts))
	for k := range ts {
		refs = append(refs, k)
	}

	sort.Sort(uint32slice(refs))
	return &mapTombstoneReader{stones: ts, refs: refs}
}

func newEmptyTombstoneReader() *mapTombstoneReader {
	return &mapTombstoneReader{stones: make(map[uint32]intervals)}
}

func (t *mapTombstoneReader) Next() bool {
	if len(t.refs) > 0 {
		t.cur = t.refs[0]
		t.refs = t.refs[1:]
		return true
	}

	t.cur = 0
	return false
}

func (t *mapTombstoneReader) Seek(ref uint32) bool {
	// If the current value satisfies, then return.
	if t.cur >= ref {
		return true
	}

	// Do binary search between current position and end.
	i := sort.Search(len(t.refs), func(i int) bool {
		return t.refs[i] >= ref
	})
	if i < len(t.refs) {
		t.cur = t.refs[i]
		t.refs = t.refs[i+1:]
		return true
	}
	t.refs = nil
	return false
}

func (t *mapTombstoneReader) At() stone {
	return stone{ref: t.cur, intervals: t.stones[t.cur]}
}

func (t *mapTombstoneReader) Copy() TombstoneReader {
	return &mapTombstoneReader{
		refs: t.refs[:],
		cur:  t.cur,

		stones: t.stones,
	}
}

func (t *mapTombstoneReader) Err() error {
	return nil
}

type simpleTombstoneReader struct {
	refs []uint32
	cur  uint32

	intervals intervals
}

func newSimpleTombstoneReader(refs []uint32, dranges intervals) *simpleTombstoneReader {
	return &simpleTombstoneReader{refs: refs, intervals: dranges}
}

func (t *simpleTombstoneReader) Next() bool {
	if len(t.refs) > 0 {
		t.cur = t.refs[0]
		t.refs = t.refs[1:]
		return true
	}

	t.cur = 0
	return false
}

func (t *simpleTombstoneReader) Seek(ref uint32) bool {
	// If the current value satisfies, then return.
	if t.cur >= ref {
		return true
	}

	// Do binary search between current position and end.
	i := sort.Search(len(t.refs), func(i int) bool {
		return t.refs[i] >= ref
	})
	if i < len(t.refs) {
		t.cur = t.refs[i]
		t.refs = t.refs[i+1:]
		return true
	}
	t.refs = nil
	return false
}

func (t *simpleTombstoneReader) At() stone {
	return stone{ref: t.cur, intervals: t.intervals}
}

func (t *simpleTombstoneReader) Copy() TombstoneReader {
	return &simpleTombstoneReader{refs: t.refs[:], cur: t.cur, intervals: t.intervals}
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
		// Merge time ranges.
		for _, r := range bcur.intervals {
			acur.intervals = acur.intervals.add(r)
		}

		t.cur = acur
		t.aok = t.a.Next()
		t.bok = t.b.Next()
	}
	return true
}

func (t *mergedTombstoneReader) Seek(ref uint32) bool {
	if t.cur.ref >= ref {
		return true
	}

	t.aok = t.a.Seek(ref)
	t.bok = t.b.Seek(ref)
	t.initialized = true

	return t.Next()
}
func (t *mergedTombstoneReader) At() stone {
	return t.cur
}

func (t *mergedTombstoneReader) Copy() TombstoneReader {
	return &mergedTombstoneReader{
		a: t.a.Copy(),
		b: t.b.Copy(),

		cur: t.cur,

		initialized: t.initialized,
		aok:         t.aok,
		bok:         t.bok,
	}
}

func (t *mergedTombstoneReader) Err() error {
	if t.a.Err() != nil {
		return t.a.Err()
	}
	return t.b.Err()
}

type interval struct {
	mint, maxt int64
}

func (tr interval) inBounds(t int64) bool {
	return t >= tr.mint && t <= tr.maxt
}

func (tr interval) isSubrange(dranges intervals) bool {
	for _, r := range dranges {
		if r.inBounds(tr.mint) && r.inBounds(tr.maxt) {
			return true
		}
	}

	return false
}

type intervals []interval

// This adds the new time-range to the existing ones.
// The existing ones must be sorted.
func (itvs intervals) add(n interval) intervals {
	for i, r := range itvs {
		// TODO(gouthamve): Make this codepath easier to digest.
		if r.inBounds(n.mint-1) || r.inBounds(n.mint) {
			if n.maxt > r.maxt {
				itvs[i].maxt = n.maxt
			}

			j := 0
			for _, r2 := range itvs[i+1:] {
				if n.maxt < r2.mint {
					break
				}
				j++
			}
			if j != 0 {
				if itvs[i+j].maxt > n.maxt {
					itvs[i].maxt = itvs[i+j].maxt
				}
				itvs = append(itvs[:i+1], itvs[i+j+1:]...)
			}
			return itvs
		}

		if r.inBounds(n.maxt+1) || r.inBounds(n.maxt) {
			if n.mint < r.maxt {
				itvs[i].mint = n.mint
			}
			return itvs
		}

		if n.mint < r.mint {
			newRange := make(intervals, i, len(itvs[:i])+1)
			copy(newRange, itvs[:i])
			newRange = append(newRange, n)
			newRange = append(newRange, itvs[i:]...)

			return newRange
		}
	}

	itvs = append(itvs, n)
	return itvs
}
