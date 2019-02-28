// Copyright 2018 The Prometheus Authors
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

package index

import (
	"encoding/binary"
	"hash"
	"hash/crc32"
	"unsafe"

	"github.com/pkg/errors"
)

// enbuf is a helper type to populate a byte slice with various types.
type encbuf struct {
	b []byte
	c [binary.MaxVarintLen64]byte
}

func (e *encbuf) reset()      { e.b = e.b[:0] }
func (e *encbuf) get() []byte { return e.b }
func (e *encbuf) len() int    { return len(e.b) }

func (e *encbuf) putString(s string) { e.b = append(e.b, s...) }
func (e *encbuf) putByte(c byte)     { e.b = append(e.b, c) }

func (e *encbuf) putBE32int(x int)      { e.putBE32(uint32(x)) }
func (e *encbuf) putUvarint32(x uint32) { e.putUvarint64(uint64(x)) }
func (e *encbuf) putUvarint(x int)      { e.putUvarint64(uint64(x)) }

func (e *encbuf) putBE32(x uint32) {
	binary.BigEndian.PutUint32(e.c[:], x)
	e.b = append(e.b, e.c[:4]...)
}

func (e *encbuf) putBE64(x uint64) {
	binary.BigEndian.PutUint64(e.c[:], x)
	e.b = append(e.b, e.c[:8]...)
}

func (e *encbuf) putUvarint64(x uint64) {
	n := binary.PutUvarint(e.c[:], x)
	e.b = append(e.b, e.c[:n]...)
}

func (e *encbuf) putVarint64(x int64) {
	n := binary.PutVarint(e.c[:], x)
	e.b = append(e.b, e.c[:n]...)
}

// putVarintStr writes a string to the buffer prefixed by its varint length (in bytes!).
func (e *encbuf) putUvarintStr(s string) {
	b := *(*[]byte)(unsafe.Pointer(&s))
	e.putUvarint(len(b))
	e.putString(s)
}

// putHash appends a hash over the buffers current contents to the buffer.
func (e *encbuf) putHash(h hash.Hash) {
	h.Reset()
	_, err := h.Write(e.b)
	if err != nil {
		panic(err) // The CRC32 implementation does not error
	}
	e.b = h.Sum(e.b)
}

// decbuf provides safe methods to extract data from a byte slice. It does all
// necessary bounds checking and advancing of the byte slice.
// Several datums can be extracted without checking for errors. However, before using
// any datum, the err() method must be checked.
type decbuf struct {
	b []byte
	e error
}

// newDecbufAt returns a new decoding buffer. It expects the first 4 bytes
// after offset to hold the big endian encoded content length, followed by the contents and the expected
// checksum.
func newDecbufAt(bs ByteSlice, off int) decbuf {
	if bs.Len() < off+4 {
		return decbuf{e: errInvalidSize}
	}
	b := bs.Range(off, off+4)
	l := int(binary.BigEndian.Uint32(b))

	if bs.Len() < off+4+l+4 {
		return decbuf{e: errInvalidSize}
	}

	// Load bytes holding the contents plus a CRC32 checksum.
	b = bs.Range(off+4, off+4+l+4)
	dec := decbuf{b: b[:len(b)-4]}

	if exp := binary.BigEndian.Uint32(b[len(b)-4:]); dec.crc32() != exp {
		return decbuf{e: errInvalidChecksum}
	}
	return dec
}

// decbufUvarintAt returns a new decoding buffer. It expects the first bytes
// after offset to hold the uvarint-encoded buffers length, followed by the contents and the expected
// checksum.
func newDecbufUvarintAt(bs ByteSlice, off int) decbuf {
	// We never have to access this method at the far end of the byte slice. Thus just checking
	// against the MaxVarintLen32 is sufficient.
	if bs.Len() < off+binary.MaxVarintLen32 {
		return decbuf{e: errInvalidSize}
	}
	b := bs.Range(off, off+binary.MaxVarintLen32)

	l, n := binary.Uvarint(b)
	if n <= 0 || n > binary.MaxVarintLen32 {
		return decbuf{e: errors.Errorf("invalid uvarint %d", n)}
	}

	if bs.Len() < off+n+int(l)+4 {
		return decbuf{e: errInvalidSize}
	}

	// Load bytes holding the contents plus a CRC32 checksum.
	b = bs.Range(off+n, off+n+int(l)+4)
	dec := decbuf{b: b[:len(b)-4]}

	if dec.crc32() != binary.BigEndian.Uint32(b[len(b)-4:]) {
		return decbuf{e: errInvalidChecksum}
	}
	return dec
}

func (d *decbuf) uvarint() int { return int(d.uvarint64()) }
func (d *decbuf) be32int() int { return int(d.be32()) }

// crc32 returns a CRC32 checksum over the remaining bytes.
func (d *decbuf) crc32() uint32 {
	return crc32.Checksum(d.b, castagnoliTable)
}

func (d *decbuf) uvarintStr() string {
	l := d.uvarint64()
	if d.e != nil {
		return ""
	}
	if len(d.b) < int(l) {
		d.e = errInvalidSize
		return ""
	}
	s := string(d.b[:l])
	d.b = d.b[l:]
	return s
}

func (d *decbuf) varint64() int64 {
	if d.e != nil {
		return 0
	}
	x, n := binary.Varint(d.b)
	if n < 1 {
		d.e = errInvalidSize
		return 0
	}
	d.b = d.b[n:]
	return x
}

func (d *decbuf) uvarint64() uint64 {
	if d.e != nil {
		return 0
	}
	x, n := binary.Uvarint(d.b)
	if n < 1 {
		d.e = errInvalidSize
		return 0
	}
	d.b = d.b[n:]
	return x
}

func (d *decbuf) be64() uint64 {
	if d.e != nil {
		return 0
	}
	if len(d.b) < 8 {
		d.e = errInvalidSize
		return 0
	}
	x := binary.BigEndian.Uint64(d.b)
	d.b = d.b[8:]
	return x
}

func (d *decbuf) be32() uint32 {
	if d.e != nil {
		return 0
	}
	if len(d.b) < 4 {
		d.e = errInvalidSize
		return 0
	}
	x := binary.BigEndian.Uint32(d.b)
	d.b = d.b[4:]
	return x
}

func (d *decbuf) err() error  { return d.e }
func (d *decbuf) len() int    { return len(d.b) }
func (d *decbuf) get() []byte { return d.b }
