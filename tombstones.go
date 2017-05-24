package tsdb

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
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
	defer f.Close()

	buf := encbuf{b: make([]byte, 3*binary.MaxVarintLen64)}
	buf.reset()
	// Write the meta.
	buf.putBE32(MagicTombstone)
	buf.putByte(tombstoneFormatV1)
	_, err = f.Write(buf.get())
	if err != nil {
		return err
	}

	mw := io.MultiWriter(f, hash)

	for k, v := range tr {
		for _, itv := range v {
			buf.reset()
			buf.putUvarint32(k)
			buf.putVarint64(itv.mint)
			buf.putVarint64(itv.maxt)

			_, err = mw.Write(buf.get())
			if err != nil {
				return err
			}
		}
	}

	_, err = f.Write(hash.Sum(nil))
	if err != nil {
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

	d := &decbuf{b: b[:len(b)-4]} // 4 for the checksum.
	if mg := d.be32(); mg != MagicTombstone {
		return nil, fmt.Errorf("invalid magic number %x", mg)
	}
	if flag := d.byte(); flag != tombstoneFormatV1 {
		return nil, fmt.Errorf("invalid tombstone format %x", flag)
	}

	// Verify checksum
	hash := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	if _, err := hash.Write(d.get()); err != nil {
		return nil, errors.Wrap(err, "write to hash")
	}
	if binary.BigEndian.Uint32(b[len(b)-4:]) != hash.Sum32() {
		return nil, errors.New("checksum did not match")
	}

	stonesMap := make(map[uint32]intervals)
	for d.len() > 0 {
		k := d.uvarint32()
		mint := d.varint64()
		maxt := d.varint64()
		if d.err() != nil {
			return nil, d.err()
		}

		stonesMap[k] = stonesMap[k].add(interval{mint, maxt})
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
