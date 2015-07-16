// Copyright 2014 The Prometheus Authors
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
	"encoding"

	"github.com/syndtr/goleveldb/leveldb"
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
		BlockCacheCapacity: o.CacheSizeBytes,
		Filter:             leveldb_filter.NewBloomFilter(10),
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
func (l *LevelDB) Delete(key encoding.BinaryMarshaler) (bool, error) {
	k, err := key.MarshalBinary()
	if err != nil {
		return false, err
	}
	// Note that Delete returns nil if k does not exist. So we have to test
	// for existence with Has first.
	if has, err := l.storage.Has(k, l.readOpts); !has || err != nil {
		return false, err
	}
	if err = l.storage.Delete(k, l.writeOpts); err != nil {
		return false, err
	}
	return true, nil
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

// ForEach implements KeyValueStore.
func (l *LevelDB) ForEach(cb func(kv KeyValueAccessor) error) error {
	snap, err := l.storage.GetSnapshot()
	if err != nil {
		return err
	}
	defer snap.Release()

	iter := snap.NewIterator(keyspace, iteratorOpts)

	kv := &levelDBKeyValueAccessor{it: iter}

	for valid := iter.First(); valid; valid = iter.Next() {
		if err = iter.Error(); err != nil {
			return err
		}

		if err := cb(kv); err != nil {
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

// levelDBKeyValueAccessor implements KeyValueAccessor.
type levelDBKeyValueAccessor struct {
	it leveldb_iterator.Iterator
}

func (i *levelDBKeyValueAccessor) Key(key encoding.BinaryUnmarshaler) error {
	return key.UnmarshalBinary(i.it.Key())
}

func (i *levelDBKeyValueAccessor) Value(value encoding.BinaryUnmarshaler) error {
	return value.UnmarshalBinary(i.it.Value())
}
