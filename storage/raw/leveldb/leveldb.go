// Copyright 2013 Prometheus Team
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

package leveldb

import (
	"fmt"

	"code.google.com/p/goprotobuf/proto"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/cache"
	"github.com/syndtr/goleveldb/leveldb/comparer"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw"
)

var (
	// Magic values per https://code.google.com/p/leveldb/source/browse/include/leveldb/db.h#131.
	keyspace = leveldb.Range{
		Start: nil,
		Limit: nil,
	}

	iteratorOpts = &opt.ReadOptions{
		Flag: opt.RFDontFillCache | opt.RFDontCopyBuffer,
	}
)

// LevelDBPersistence is a disk-backed sorted key-value store.
type LevelDBPersistence struct {
	path    string
	name    string
	purpose string

	storage *leveldb.DB

	readOpts  *opt.ReadOptions
	writeOpts *opt.WriteOptions
}

type LevelDBOptions struct {
	Path    string
	Name    string
	Purpose string

	CacheSizeBytes    int
	OpenFileAllowance int
}

func NewLevelDBPersistence(o LevelDBOptions) (*LevelDBPersistence, error) {
	options := &opt.Options{
		Comparer: new(comparer.BytesComparer),

		Flag: opt.OFCreateIfMissing,

		CompressionType: opt.SnappyCompression,

		MaxOpenFiles: o.OpenFileAllowance,

		BlockCache: cache.NewLRUCache(o.CacheSizeBytes),

		Filter: filter.NewBloomFilter(10),
	}

	storage, err := leveldb.OpenFile(o.Path, options)
	if err != nil {
		return nil, err
	}

	return &LevelDBPersistence{
		path:    o.Path,
		name:    o.Name,
		purpose: o.Purpose,

		storage: storage,

		readOpts: &opt.ReadOptions{
			Flag: opt.RFDontCopyBuffer,
		},
		writeOpts: &opt.WriteOptions{},
	}, nil
}

func (l *LevelDBPersistence) Close() error {
	return l.storage.Close()
}

func (l *LevelDBPersistence) Get(k, v proto.Message) (bool, error) {
	raw, err := l.storage.Get(coding.NewPBEncoder(k).MustEncode(), l.readOpts)
	if err != nil {
		return false, err
	}
	if raw == nil {
		return false, nil
	}

	if v == nil {
		return true, nil
	}

	err = proto.Unmarshal(raw, v)
	if err != nil {
		return true, err
	}

	return true, nil
}

func (l *LevelDBPersistence) Has(k proto.Message) (has bool, err error) {
	return l.Get(k, nil)
}

func (l *LevelDBPersistence) Drop(k proto.Message) error {
	return l.storage.Delete(coding.NewPBEncoder(k).MustEncode(), l.writeOpts)
}

func (l *LevelDBPersistence) Put(key, value proto.Message) error {
	return l.storage.Put(coding.NewPBEncoder(key).MustEncode(), coding.NewPBEncoder(value).MustEncode(), l.writeOpts)
}

func (l *LevelDBPersistence) Commit(b raw.Batch) (err error) {
	return l.storage.Write(b.(*Batch).batch, l.writeOpts)
}

// CompactKeyspace compacts the entire database's keyspace.
//
// Beware that it would probably be imprudent to run this on a live user-facing
// server due to latency implications.
func (l *LevelDBPersistence) Prune() {
	l.storage.CompactRange(keyspace)
}

func (l *LevelDBPersistence) Size() (uint64, error) {
	iterator, err := l.NewIterator(false)
	if err != nil {
		return 0, err
	}
	defer iterator.Close()

	if !iterator.SeekToFirst() {
		return 0, fmt.Errorf("could not seek to first key")
	}

	keyspace := leveldb.Range{}

	keyspace.Start = iterator.getIterator().Key()

	if !iterator.SeekToLast() {
		return 0, fmt.Errorf("could not seek to last key")
	}

	keyspace.Limit = iterator.getIterator().Key()

	sizes, err := l.storage.GetApproximateSizes([]leveldb.Range{keyspace})
	if err != nil {
		return 0, err
	}

	return sizes.Sum(), nil
}

// NewIterator creates a new levigoIterator, which follows the Iterator
// interface.
//
// Important notes:
//
// For each of the iterator methods that have a return signature of (ok bool),
// if ok == false, the iterator may not be used any further and must be closed.
// Further work with the database requires the creation of a new iterator.  This
// is due to LevelDB and Levigo design.  Please refer to Jeff and Sanjay's notes
// in the LevelDB documentation for this behavior's rationale.
//
// The returned iterator must explicitly be closed; otherwise non-managed memory
// will be leaked.
//
// The iterator is optionally snapshotable.
func (l *LevelDBPersistence) NewIterator(snapshotted bool) (Iterator, error) {
	if !snapshotted {
		return &iter{
			iter: l.storage.NewIterator(iteratorOpts),
		}, nil
	}

	snap, err := l.storage.GetSnapshot()
	if err != nil {
		return nil, err
	}

	return &snapIter{
		iter: iter{
			iter: snap.NewIterator(iteratorOpts),
		},
		snap: snap,
	}, nil
}

func (l *LevelDBPersistence) ForEach(decoder storage.RecordDecoder, filter storage.RecordFilter, operator storage.RecordOperator) (scannedEntireCorpus bool, err error) {
	iterator, err := l.NewIterator(true)
	if err != nil {
		return false, err
	}

	defer iterator.Close()

	for valid := iterator.SeekToFirst(); valid; valid = iterator.Next() {
		if err = iterator.Error(); err != nil {
			return false, err
		}

		decodedKey, decodeErr := decoder.DecodeKey(iterator.getIterator().Key())
		if decodeErr != nil {
			continue
		}
		decodedValue, decodeErr := decoder.DecodeValue(iterator.getIterator().Value())
		if decodeErr != nil {
			continue
		}

		switch filter.Filter(decodedKey, decodedValue) {
		case storage.STOP:
			return
		case storage.SKIP:
			continue
		case storage.ACCEPT:
			opErr := operator.Operate(decodedKey, decodedValue)
			if opErr != nil {
				if opErr.Continuable {
					continue
				}
				break
			}
		}
	}
	return true, nil
}
