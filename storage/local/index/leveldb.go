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

// LevelDBOptions provides options for a LevelDB.
type LevelDBOptions struct {
	Path           string // Base path to store files.
	CacheSizeBytes int
}

// NewLevelDB returns a newly allocated LevelDB-backed KeyValueStore ready to
// use.
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

// NewBatch implements KeyValueStore.
func (l *LevelDB) NewBatch() Batch {
	return &LevelDBBatch{
		batch: &leveldb.Batch{},
	}
}

// Close implements KeyValueStore.
func (l *LevelDB) Close() error {
	return l.storage.Close()
}

// Get implements KeyValueStore.
func (l *LevelDB) Get(key encoding.BinaryMarshaler, value encoding.BinaryUnmarshaler) (bool, error) {
	k, err := key.MarshalBinary()
	if err != nil {
		return false, err
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

// Has implements KeyValueStore.
func (l *LevelDB) Has(key encoding.BinaryMarshaler) (has bool, err error) {
	return l.Get(key, nil)
}

// Delete implements KeyValueStore.
func (l *LevelDB) Delete(key encoding.BinaryMarshaler) error {
	k, err := key.MarshalBinary()
	if err != nil {
		return err
	}
	return l.storage.Delete(k, l.writeOpts)
}

// Put implements KeyValueStore.
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

// Commit implements KeyValueStore.
func (l *LevelDB) Commit(b Batch) error {
	return l.storage.Write(b.(*LevelDBBatch).batch, l.writeOpts)
}

// LevelDBBatch is a Batch implementation for LevelDB.
type LevelDBBatch struct {
	batch *leveldb.Batch
}

// Put implements Batch.
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

// Delete implements Batch.
func (b *LevelDBBatch) Delete(key encoding.BinaryMarshaler) error {
	k, err := key.MarshalBinary()
	if err != nil {
		return err
	}
	b.batch.Delete(k)
	return nil
}

// Reset implements Batch.
func (b *LevelDBBatch) Reset() {
	b.batch.Reset()
}
