package index

import (
	"github.com/syndtr/goleveldb/leveldb"
)

type batch struct {
	batch *leveldb.Batch
}

func (b *batch) Put(key, value encodable) {
	b.batch.Put(key.encode(), value.encode())
}

func (b *batch) Delete(k encodable) {
	b.batch.Delete(k.encode())
}

func (b *batch) Reset() {
	b.batch.Reset()
}
