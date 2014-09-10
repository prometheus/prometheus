package index

import (
	"encoding"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/cache"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// LevelDB is a LevelDB-backed sorted KeyValueStore.
type LevelDB struct {
	storage   *leveldb.DB
	readOpts  *opt.ReadOptions
	writeOpts *opt.WriteOptions
}

type LevelDBOptions struct {
	Path           string
	CacheSizeBytes int
}

func NewLevelDB(o LevelDBOptions) (KeyValueStore, error) {
	options := &opt.Options{
		Compression: opt.SnappyCompression,
		BlockCache:  cache.NewLRUCache(o.CacheSizeBytes),
		Filter:      filter.NewBloomFilter(10),
	}

	storage, err := leveldb.OpenFile(o.Path, options)
	if err != nil {
		return nil, err
	}

	return &LevelDB{
		storage:   storage,
		readOpts:  &opt.ReadOptions{},
		writeOpts: &opt.WriteOptions{},
	}, nil
}

func (l *LevelDB) NewBatch() Batch {
	return &LevelDBBatch{
		batch: &leveldb.Batch{},
	}
}

func (l *LevelDB) Close() error {
	return l.storage.Close()
}

func (l *LevelDB) Get(key encoding.BinaryMarshaler, value encoding.BinaryUnmarshaler) (bool, error) {
	k, err := key.MarshalBinary()
	if err != nil {
		return false, nil
	}
	raw, err := l.storage.Get(k, l.readOpts)
	if err == leveldb.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if value == nil {
		return true, nil
	}
	return true, value.UnmarshalBinary(raw)
}

func (l *LevelDB) Has(key encoding.BinaryMarshaler) (has bool, err error) {
	return l.Get(key, nil)
}

func (l *LevelDB) Delete(key encoding.BinaryMarshaler) error {
	k, err := key.MarshalBinary()
	if err != nil {
		return err
	}
	return l.storage.Delete(k, l.writeOpts)
}

func (l *LevelDB) Put(key, value encoding.BinaryMarshaler) error {
	k, err := key.MarshalBinary()
	if err != nil {
		return err
	}
	v, err := value.MarshalBinary()
	if err != nil {
		return err
	}
	return l.storage.Put(k, v, l.writeOpts)
}

func (l *LevelDB) Commit(b Batch) error {
	return l.storage.Write(b.(*LevelDBBatch).batch, l.writeOpts)
}

// LevelDBBatch is a Batch implementation for LevelDB.
type LevelDBBatch struct {
	batch *leveldb.Batch
}

func (b *LevelDBBatch) Put(key, value encoding.BinaryMarshaler) error {
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

func (b *LevelDBBatch) Delete(key encoding.BinaryMarshaler) error {
	k, err := key.MarshalBinary()
	if err != nil {
		return err
	}
	b.batch.Delete(k)
	return nil
}

func (b *LevelDBBatch) Reset() {
	b.batch.Reset()
}
