package tsdb

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"os"
	"path/filepath"
)

const tombstoneFilename = "tombstones"

const (
	// MagicTombstone is 4 bytes at the head of a tombstone file.
	MagicTombstone = 0x130BA30

	tombstoneFormatV1 = 1
)

func writeTombstoneFile(dir string, tr tombstoneReader) error {
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

	for k, v := range tr {
		refs = append(refs, k)
		stoneOff[k] = pos

		// Write the ranges.
		buf.reset()
		buf.putUvarint(len(v))
		n, err := f.Write(buf.get())
		if err != nil {
			return err
		}
		pos += int64(n)

		buf.reset()
		for _, r := range v {
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
	At(ref uint32) intervals
}

func readTombstones(dir string) (tombstoneReader, error) {
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

	stones := b[off : off+int64(numStones*12)]
	stonesMap := make(map[uint32]intervals)
	for len(stones) >= 12 {
		d := &decbuf{b: stones[:12]}
		ref := d.be32()
		off := d.be64int64()

		d = &decbuf{b: b[off:]}
		numRanges := d.uvarint()
		if err := d.err(); err != nil {
			return nil, err
		}

		dranges := make(intervals, 0, numRanges)
		for i := 0; i < int(numRanges); i++ {
			mint := d.varint64()
			maxt := d.varint64()
			if err := d.err(); err != nil {
				return nil, err
			}

			dranges = append(dranges, interval{mint, maxt})
		}

		// TODO(gouthamve): Verify checksum.
		stones = stones[12:]
		stonesMap[ref] = dranges
	}

	return newTombstoneReader(stonesMap), nil
}

type tombstoneReader map[uint32]intervals

func newTombstoneReader(ts map[uint32]intervals) tombstoneReader {
	return tombstoneReader(ts)
}

func newEmptyTombstoneReader() tombstoneReader {
	return tombstoneReader(make(map[uint32]intervals))
}

func (t tombstoneReader) At(ref uint32) intervals {
	return t[ref]
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
