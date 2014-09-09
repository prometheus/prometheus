package index

import (
	"encoding"

	"github.com/syndtr/goleveldb/leveldb"
)

type batch struct {
	batch *leveldb.Batch
}

func (b *batch) Put(key, value encoding.BinaryMarshaler) error {
	k, err := key.MarshalBinary()
	if err != nil {
		return err
	}
	v, err := value.MarshalBinary()
	if err != nil {
		return err
	}
	b.batch.Put(k, v)
	return nil
}

func (b *batch) Delete(key encoding.BinaryMarshaler) error {
	k, err := key.MarshalBinary()
	if err != nil {
		return err
	}
	b.batch.Delete(k)
	return nil
}

func (b *batch) Reset() {
	b.batch.Reset()
}
