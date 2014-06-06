package storage_ng

import (
	"github.com/prometheus/prometheus/utility"
)

var chunkBufs = newChunkBufList(10000, 10000)

type chunkBufList struct {
	l utility.FreeList
}

func newChunkBuf() []byte {
	return make([]byte, 0, 1024) // TODO: This value somehow needs to be set in coordination with the one passed into the disk persistence.
}

func newChunkBufList(length, capacity int) *chunkBufList {
	l := &chunkBufList{
		l: utility.NewFreeList(capacity),
	}
	for i := 0; i < length; i++ {
		l.l.Give(newChunkBuf())
	}
	return l
}

func (l *chunkBufList) Get() []byte {
	numChunkGets.Inc()
	if v, ok := l.l.Get(); ok {
		return v.([]byte)
	}

	return newChunkBuf()
}

func (l *chunkBufList) Give(v []byte) bool {
	numChunkGives.Inc()
	v = v[:0]
	return l.l.Give(v)
}

func (l *chunkBufList) Close() {
	l.l.Close()
}
