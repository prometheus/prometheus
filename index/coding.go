package index

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"

	"github.com/boltdb/bolt"
)

var encpool buffers

type buffers struct {
	pool sync.Pool
}

func (b *buffers) get(l int) []byte {
	x := b.pool.Get()
	if x == nil {
		return make([]byte, l)
	}
	buf := x.([]byte)
	if cap(buf) < l {
		return make([]byte, l)
	}
	return buf[:l]
}

func (b *buffers) getZero(l int) []byte {
	buf := b.get(l)
	for i := range buf {
		buf[i] = 0
	}
	return buf
}

func (b *buffers) put(buf []byte) {
	b.pool.Put(buf)
}

func (b *buffers) bucketPut(bkt *bolt.Bucket, k, v []byte) error {
	err := bkt.Put(k, v)
	b.put(k)
	return err
}

func (b *buffers) bucketGet(bkt *bolt.Bucket, k []byte) []byte {
	v := bkt.Get(k)
	b.put(k)
	return v
}

func (b *buffers) uint64be(x uint64) []byte {
	buf := b.get(8)
	binary.BigEndian.PutUint64(buf, x)
	return buf
}

func (b *buffers) uvarint(x uint64) []byte {
	buf := b.get(binary.MaxVarintLen64)
	return buf[:binary.PutUvarint(buf, x)]
}

type txbuffs struct {
	buffers *buffers
	done    [][]byte
}

func (b *txbuffs) get(l int) []byte {
	buf := b.buffers.get(l)
	b.done = append(b.done, buf)
	return buf
}

func (b *txbuffs) getZero(l int) []byte {
	buf := b.buffers.getZero(l)
	b.done = append(b.done, buf)
	return buf
}

func (b *txbuffs) release() {
	for _, buf := range b.done {
		b.buffers.put(buf)
	}
}

func (b *txbuffs) put(buf []byte) {
	b.done = append(b.done, buf)
}

func (b *txbuffs) uint64be(x uint64) []byte {
	buf := b.get(8)
	binary.BigEndian.PutUint64(buf, x)
	return buf
}

func (b *txbuffs) uvarint(x uint64) []byte {
	buf := b.get(binary.MaxVarintLen64)
	return buf[:binary.PutUvarint(buf, x)]
}

// reuse of buffers
var pagePool sync.Pool

// getBuf returns a buffer from the pool. The length of the returned slice is l.
func getPage(l int) []byte {
	x := pagePool.Get()
	if x == nil {
		return make([]byte, l)
	}
	buf := x.([]byte)
	if cap(buf) < l {
		return make([]byte, l)
	}
	return buf[:l]
}

// putBuf returns a buffer to the pool.
func putPage(buf []byte) {
	pagePool.Put(buf)
}

// bufPool is a pool for staging buffers. Using a pool allows concurrency-safe
// reuse of buffers
var bufPool sync.Pool

// getBuf returns a buffer from the pool. The length of the returned slice is l.
func getBuf(l int) []byte {
	x := bufPool.Get()
	if x == nil {
		return make([]byte, l)
	}
	buf := x.([]byte)
	if cap(buf) < l {
		return make([]byte, l)
	}
	return buf[:l]
}

// putBuf returns a buffer to the pool.
func putBuf(buf []byte) {
	bufPool.Put(buf)
}

func encodeUint64(x uint64) []byte {
	buf := getBuf(8)
	binary.BigEndian.PutUint64(buf, x)
	return buf
}

func decodeUint64(buf []byte) uint64 {
	return binary.BigEndian.Uint64(buf)
}

func writeUvarint(w io.ByteWriter, x uint64) (i int, err error) {
	for x >= 0x80 {
		if err = w.WriteByte(byte(x) | 0x80); err != nil {
			return i, err
		}
		x >>= 7
		i++
	}
	if err = w.WriteByte(byte(x)); err != nil {
		return i, err
	}
	return i + 1, err
}

func writeVarint(w io.ByteWriter, x int64) (i int, err error) {
	ux := uint64(x) << 1
	if x < 0 {
		ux = ^ux
	}
	return writeUvarint(w, ux)
}

func readUvarint(r io.ByteReader) (uint64, int, error) {
	var (
		x uint64
		s uint
	)
	for i := 0; ; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return x, i, err
		}
		if b < 0x80 {
			if i > 9 || i == 9 && b > 1 {
				return x, i + 1, errors.New("varint overflows a 64-bit integer")
			}
			return x | uint64(b)<<s, i + 1, nil
		}
		x |= uint64(b&0x7f) << s
		s += 7
	}
}

func readVarint(r io.ByteReader) (int64, int, error) {
	ux, n, err := readUvarint(r)
	x := int64(ux >> 1)
	if ux&1 != 0 {
		x = ^x
	}
	return x, n, err
}
