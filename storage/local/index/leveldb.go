package index

import (
	"encoding"

	"github.com/syndtr/goleveldb/leveldb"
	leveldb_cache "github.com/syndtr/goleveldb/leveldb/cache"
	leveldb_filter "github.com/syndtr/goleveldb/leveldb/filter"
	leveldb_iterator "github.com/syndtr/goleveldb/leveldb/iterator"
	leveldb_opt "github.com/syndtr/goleveldb/leveldb/opt"
	leveldb_util "github.com/syndtr/goleveldb/leveldb/util"
)

var (
	keyspace = &leveldb_util.Range{
		Start: nil,
		Limit: nil,
	}

	iteratorOpts = &leveldb_opt.ReadOptions{
		DontFillCache: true,
	}
)

// LevelDB is a LevelDB-backed sorted KeyValueStore.
type LevelDB struct {
	storage   *leveldb.DB
	readOpts  *leveldb_opt.ReadOptions
	writeOpts *leveldb_opt.WriteOptions
}

// LevelDBOptions provides options for a LevelDB.
type LevelDBOptions struct {
	Path           string // Base path to store files.
	CacheSizeBytes int
}

// NewLevelDB returns a newly allocated LevelDB-backed KeyValueStore ready to
// use.
func NewLevelDB(o LevelDBOptions) (KeyValueStore, error) {
	options := &leveldb_opt.Options{
		Compression: leveldb_opt.SnappyCompression,
		BlockCache:  leveldb_cache.NewLRUCache(o.CacheSizeBytes),
		Filter:      leveldb_filter.NewBloomFilter(10),
	}

	storage, err := leveldb.OpenFile(o.Path, options)
	if err != nil {
		return nil, err
	}

	return &LevelDB{
		storage:   storage,
		readOpts:  &leveldb_opt.ReadOptions{},
		writeOpts: &leveldb_opt.WriteOptions{},
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

func (l *LevelDB) ForEach(cb func(kv KeyValueAccessor) error) error {
	it, err := l.NewIterator(true)
	if err != nil {
		return err
	}

	defer it.Close()

	for valid := it.SeekToFirst(); valid; valid = it.Next() {
		if err = it.Error(); err != nil {
			return err
		}

		if err := cb(it); err != nil {
			return err
		}
	}
	return nil
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

// levelDBIterator implements Iterator.
type levelDBIterator struct {
	it leveldb_iterator.Iterator
}

func (i *levelDBIterator) Error() error {
	return i.it.Error()
}

func (i *levelDBIterator) Valid() bool {
	return i.it.Valid()
}

func (i *levelDBIterator) SeekToFirst() bool {
	return i.it.First()
}

func (i *levelDBIterator) SeekToLast() bool {
	return i.it.Last()
}

func (i *levelDBIterator) Seek(k encoding.BinaryMarshaler) bool {
	key, err := k.MarshalBinary()
	if err != nil {
		panic(err)
	}
	return i.it.Seek(key)
}

func (i *levelDBIterator) Next() bool {
	return i.it.Next()
}

func (i *levelDBIterator) Previous() bool {
	return i.it.Prev()
}

func (i *levelDBIterator) Key(key encoding.BinaryUnmarshaler) error {
	return key.UnmarshalBinary(i.it.Key())
}

func (i *levelDBIterator) Value(value encoding.BinaryUnmarshaler) error {
	return value.UnmarshalBinary(i.it.Value())
}

func (*levelDBIterator) Close() error {
	return nil
}

type snapshottedIterator struct {
	levelDBIterator
	snap *leveldb.Snapshot
}

func (i *snapshottedIterator) Close() error {
	i.snap.Release()

	return nil
}

// newIterator creates a new LevelDB iterator which is optionally based on a
// snapshot of the current DB state.
//
// For each of the iterator methods that have a return signature of (ok bool),
// if ok == false, the iterator may not be used any further and must be closed.
// Further work with the database requires the creation of a new iterator.
func (l *LevelDB) NewIterator(snapshotted bool) (Iterator, error) {
	if !snapshotted {
		return &levelDBIterator{
			it: l.storage.NewIterator(keyspace, iteratorOpts),
		}, nil
	}

	snap, err := l.storage.GetSnapshot()
	if err != nil {
		return nil, err
	}

	return &snapshottedIterator{
		levelDBIterator: levelDBIterator{
			it: snap.NewIterator(keyspace, iteratorOpts),
		},
		snap: snap,
	}, nil
}
