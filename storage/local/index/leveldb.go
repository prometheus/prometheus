package index

import (
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/cache"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// LevelDB is a LevelDB-backed sorted key-value store.
type LevelDB struct {
	storage   *leveldb.DB
	readOpts  *opt.ReadOptions
	writeOpts *opt.WriteOptions
}

type LevelDBOptions struct {
	Path           string
	CacheSizeBytes int
}

func NewLevelDB(o LevelDBOptions) (*LevelDB, error) {
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
	return &batch{
		batch: &leveldb.Batch{},
	}
}

func (l *LevelDB) Close() error {
	return l.storage.Close()
}

func (l *LevelDB) Get(k encodable, v decodable) (bool, error) {
	raw, err := l.storage.Get(k.encode(), l.readOpts)
	if err == leveldb.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if v == nil {
		return true, nil
	}
	v.decode(raw)
	return true, err
}

func (l *LevelDB) Has(k encodable) (has bool, err error) {
	return l.Get(k, nil)
}

func (l *LevelDB) Delete(k encodable) error {
	return l.storage.Delete(k.encode(), l.writeOpts)
}

func (l *LevelDB) Put(key, value encodable) error {
	return l.storage.Put(key.encode(), value.encode(), l.writeOpts)
}

func (l *LevelDB) Commit(b Batch) error {
	return l.storage.Write(b.(*batch).batch, l.writeOpts)
}
