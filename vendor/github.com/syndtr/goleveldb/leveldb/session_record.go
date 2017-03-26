// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"bufio"
	"encoding/binary"
	"io"
	"strings"

	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

type byteReader interface {
	io.Reader
	io.ByteReader
}

// These numbers are written to disk and should not be changed.
const (
	recComparer    = 1
	recJournalNum  = 2
	recNextFileNum = 3
	recSeqNum      = 4
	recCompPtr     = 5
	recDelTable    = 6
	recAddTable    = 7
	// 8 was used for large value refs
	recPrevJournalNum = 9
)

type cpRecord struct {
	level int
	ikey  internalKey
}

type atRecord struct {
	level int
	num   int64
	size  int64
	imin  internalKey
	imax  internalKey
}

type dtRecord struct {
	level int
	num   int64
}

type sessionRecord struct {
	hasRec         int
	comparer       string
	journalNum     int64
	prevJournalNum int64
	nextFileNum    int64
	seqNum         uint64
	compPtrs       []cpRecord
	addedTables    []atRecord
	deletedTables  []dtRecord

	scratch [binary.MaxVarintLen64]byte
	err     error
}

func (p *sessionRecord) has(rec int) bool {
	return p.hasRec&(1<<uint(rec)) != 0
}

func (p *sessionRecord) setComparer(name string) {
	p.hasRec |= 1 << recComparer
	p.comparer = name
}

func (p *sessionRecord) setJournalNum(num int64) {
	p.hasRec |= 1 << recJournalNum
	p.journalNum = num
}

func (p *sessionRecord) setPrevJournalNum(num int64) {
	p.hasRec |= 1 << recPrevJournalNum
	p.prevJournalNum = num
}

func (p *sessionRecord) setNextFileNum(num int64) {
	p.hasRec |= 1 << recNextFileNum
	p.nextFileNum = num
}

func (p *sessionRecord) setSeqNum(num uint64) {
	p.hasRec |= 1 << recSeqNum
	p.seqNum = num
}

func (p *sessionRecord) addCompPtr(level int, ikey internalKey) {
	p.hasRec |= 1 << recCompPtr
	p.compPtrs = append(p.compPtrs, cpRecord{level, ikey})
}

func (p *sessionRecord) resetCompPtrs() {
	p.hasRec &= ^(1 << recCompPtr)
	p.compPtrs = p.compPtrs[:0]
}

func (p *sessionRecord) addTable(level int, num, size int64, imin, imax internalKey) {
	p.hasRec |= 1 << recAddTable
	p.addedTables = append(p.addedTables, atRecord{level, num, size, imin, imax})
}

func (p *sessionRecord) addTableFile(level int, t *tFile) {
	p.addTable(level, t.fd.Num, t.size, t.imin, t.imax)
}

func (p *sessionRecord) resetAddedTables() {
	p.hasRec &= ^(1 << recAddTable)
	p.addedTables = p.addedTables[:0]
}

func (p *sessionRecord) delTable(level int, num int64) {
	p.hasRec |= 1 << recDelTable
	p.deletedTables = append(p.deletedTables, dtRecord{level, num})
}

func (p *sessionRecord) resetDeletedTables() {
	p.hasRec &= ^(1 << recDelTable)
	p.deletedTables = p.deletedTables[:0]
}

func (p *sessionRecord) putUvarint(w io.Writer, x uint64) {
	if p.err != nil {
		return
	}
	n := binary.PutUvarint(p.scratch[:], x)
	_, p.err = w.Write(p.scratch[:n])
}

func (p *sessionRecord) putVarint(w io.Writer, x int64) {
	if x < 0 {
		panic("invalid negative value")
	}
	p.putUvarint(w, uint64(x))
}

func (p *sessionRecord) putBytes(w io.Writer, x []byte) {
	if p.err != nil {
		return
	}
	p.putUvarint(w, uint64(len(x)))
	if p.err != nil {
		return
	}
	_, p.err = w.Write(x)
}

func (p *sessionRecord) encode(w io.Writer) error {
	p.err = nil
	if p.has(recComparer) {
		p.putUvarint(w, recComparer)
		p.putBytes(w, []byte(p.comparer))
	}
	if p.has(recJournalNum) {
		p.putUvarint(w, recJournalNum)
		p.putVarint(w, p.journalNum)
	}
	if p.has(recNextFileNum) {
		p.putUvarint(w, recNextFileNum)
		p.putVarint(w, p.nextFileNum)
	}
	if p.has(recSeqNum) {
		p.putUvarint(w, recSeqNum)
		p.putUvarint(w, p.seqNum)
	}
	for _, r := range p.compPtrs {
		p.putUvarint(w, recCompPtr)
		p.putUvarint(w, uint64(r.level))
		p.putBytes(w, r.ikey)
	}
	for _, r := range p.deletedTables {
		p.putUvarint(w, recDelTable)
		p.putUvarint(w, uint64(r.level))
		p.putVarint(w, r.num)
	}
	for _, r := range p.addedTables {
		p.putUvarint(w, recAddTable)
		p.putUvarint(w, uint64(r.level))
		p.putVarint(w, r.num)
		p.putVarint(w, r.size)
		p.putBytes(w, r.imin)
		p.putBytes(w, r.imax)
	}
	return p.err
}

func (p *sessionRecord) readUvarintMayEOF(field string, r io.ByteReader, mayEOF bool) uint64 {
	if p.err != nil {
		return 0
	}
	x, err := binary.ReadUvarint(r)
	if err != nil {
		if err == io.ErrUnexpectedEOF || (mayEOF == false && err == io.EOF) {
			p.err = errors.NewErrCorrupted(storage.FileDesc{}, &ErrManifestCorrupted{field, "short read"})
		} else if strings.HasPrefix(err.Error(), "binary:") {
			p.err = errors.NewErrCorrupted(storage.FileDesc{}, &ErrManifestCorrupted{field, err.Error()})
		} else {
			p.err = err
		}
		return 0
	}
	return x
}

func (p *sessionRecord) readUvarint(field string, r io.ByteReader) uint64 {
	return p.readUvarintMayEOF(field, r, false)
}

func (p *sessionRecord) readVarint(field string, r io.ByteReader) int64 {
	x := int64(p.readUvarintMayEOF(field, r, false))
	if x < 0 {
		p.err = errors.NewErrCorrupted(storage.FileDesc{}, &ErrManifestCorrupted{field, "invalid negative value"})
	}
	return x
}

func (p *sessionRecord) readBytes(field string, r byteReader) []byte {
	if p.err != nil {
		return nil
	}
	n := p.readUvarint(field, r)
	if p.err != nil {
		return nil
	}
	x := make([]byte, n)
	_, p.err = io.ReadFull(r, x)
	if p.err != nil {
		if p.err == io.ErrUnexpectedEOF {
			p.err = errors.NewErrCorrupted(storage.FileDesc{}, &ErrManifestCorrupted{field, "short read"})
		}
		return nil
	}
	return x
}

func (p *sessionRecord) readLevel(field string, r io.ByteReader) int {
	if p.err != nil {
		return 0
	}
	x := p.readUvarint(field, r)
	if p.err != nil {
		return 0
	}
	return int(x)
}

func (p *sessionRecord) decode(r io.Reader) error {
	br, ok := r.(byteReader)
	if !ok {
		br = bufio.NewReader(r)
	}
	p.err = nil
	for p.err == nil {
		rec := p.readUvarintMayEOF("field-header", br, true)
		if p.err != nil {
			if p.err == io.EOF {
				return nil
			}
			return p.err
		}
		switch rec {
		case recComparer:
			x := p.readBytes("comparer", br)
			if p.err == nil {
				p.setComparer(string(x))
			}
		case recJournalNum:
			x := p.readVarint("journal-num", br)
			if p.err == nil {
				p.setJournalNum(x)
			}
		case recPrevJournalNum:
			x := p.readVarint("prev-journal-num", br)
			if p.err == nil {
				p.setPrevJournalNum(x)
			}
		case recNextFileNum:
			x := p.readVarint("next-file-num", br)
			if p.err == nil {
				p.setNextFileNum(x)
			}
		case recSeqNum:
			x := p.readUvarint("seq-num", br)
			if p.err == nil {
				p.setSeqNum(x)
			}
		case recCompPtr:
			level := p.readLevel("comp-ptr.level", br)
			ikey := p.readBytes("comp-ptr.ikey", br)
			if p.err == nil {
				p.addCompPtr(level, internalKey(ikey))
			}
		case recAddTable:
			level := p.readLevel("add-table.level", br)
			num := p.readVarint("add-table.num", br)
			size := p.readVarint("add-table.size", br)
			imin := p.readBytes("add-table.imin", br)
			imax := p.readBytes("add-table.imax", br)
			if p.err == nil {
				p.addTable(level, num, size, imin, imax)
			}
		case recDelTable:
			level := p.readLevel("del-table.level", br)
			num := p.readVarint("del-table.num", br)
			if p.err == nil {
				p.delTable(level, num)
			}
		}
	}

	return p.err
}
