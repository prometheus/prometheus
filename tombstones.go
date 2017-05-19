package tsdb

import (
	"encoding/binary"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
)

const tombstoneFilename = "tombstones"

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
	if err := tr.Err(); err != nil {
		return err
	}

	// Write the offset table.
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
	ref    uint32
	ranges []trange
}

// TombstoneReader is the iterator over tombstones.
type TombstoneReader interface {
	Next() bool
	Seek(ref uint32) bool
	At() stone
	// A copy of the current instance. Changes to the copy will not affect parent.
	Copy() TombstoneReader
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
	numStones := d.be32int()
	if err := d.err(); err != nil {
		return nil, err
	}

	return &tombstoneReader{
		stones: b[off+4:],
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

func (t *tombstoneReader) Seek(ref uint32) bool {
	bytIdx := t.idx * 12

	t.idx += sort.Search(t.len-t.idx, func(i int) bool {
		return binary.BigEndian.Uint32(t.b[bytIdx+i*12:]) >= ref
	})

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

func (t *tombstoneReader) Copy() TombstoneReader {
	return &tombstoneReader{
		stones: t.stones[:],
		idx:    t.idx,
		len:    t.len,

		b: t.b,
	}
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
	return stone{ref: t.cur, ranges: t.stones[t.cur]}
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
	return stone{ref: t.cur, ranges: t.ranges}
}

func (t *simpleTombstoneReader) Copy() TombstoneReader {
	return &simpleTombstoneReader{refs: t.refs[:], cur: t.cur, ranges: t.ranges}
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

type trange struct {
	mint, maxt int64
}

func (tr trange) inBounds(t int64) bool {
	return t >= tr.mint && t <= tr.maxt
}

func (tr trange) isSubrange(ranges []trange) bool {
	for _, r := range ranges {
		if r.inBounds(tr.mint) && r.inBounds(tr.maxt) {
			return true
		}
	}

	return false
}

// This adds the new time-range to the existing ones.
// The existing ones must be sorted and should not be nil.
func addNewInterval(existing []trange, n trange) []trange {
	for i, r := range existing {
		// TODO(gouthamve): Make this codepath easier to digest.
		if r.inBounds(n.mint) {
			if n.maxt > r.maxt {
				existing[i].maxt = n.maxt
			}

			j := 0
			for _, r2 := range existing[i+1:] {
				if n.maxt < r2.mint {
					break
				}
				j++
			}
			if j != 0 {
				if existing[i+j].maxt > n.maxt {
					existing[i].maxt = existing[i+j].maxt
				}
				existing = append(existing[:i+1], existing[i+j+1:]...)
			}
			return existing
		}

		if r.inBounds(n.maxt) {
			if n.mint < r.maxt {
				existing[i].mint = n.mint
			}
			return existing
		}
		if n.mint < r.mint {
			newRange := make([]trange, i, len(existing[:i])+1)
			copy(newRange, existing[:i])
			newRange = append(newRange, n)
			newRange = append(newRange, existing[i:]...)

			return newRange
		}
	}

	existing = append(existing, n)
	return existing
}
