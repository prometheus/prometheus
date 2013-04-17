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
	"flag"
	"fmt"
	"github.com/jmhodges/levigo"
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/native"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw"
	"time"
)

var (
	leveldbFlushOnMutate     = flag.Bool("leveldbFlushOnMutate", false, "Whether LevelDB should flush every operation to disk upon mutation before returning (bool).")
	leveldbUseSnappy         = flag.Bool("leveldbUseSnappy", true, "Whether LevelDB attempts to use Snappy for compressing elements (bool).")
	leveldbUseParanoidChecks = flag.Bool("leveldbUseParanoidChecks", true, "Whether LevelDB uses expensive checks (bool).")
)

// LevelDBPersistence is a disk-backed sorted key-value store.
type LevelDBPersistence struct {
	cache        *levigo.Cache
	filterPolicy *levigo.FilterPolicy
	options      *levigo.Options
	storage      *levigo.DB
	readOptions  *levigo.ReadOptions
	writeOptions *levigo.WriteOptions
	comparator   native.Comparator
}

// levigoIterator wraps the LevelDB resources in a convenient manner for uniform
// resource access and closing through the raw.Iterator protocol.
type levigoIterator struct {
	// iterator is the receiver of most proxied operation calls.
	iterator *levigo.Iterator
	// readOptions is only set if the iterator is a snapshot of an underlying
	// database.  This signals that it needs to be explicitly reaped upon the
	// end of this iterator's life.
	readOptions *levigo.ReadOptions
	// snapshot is only set if the iterator is a snapshot of an underlying
	// database.  This signals that it needs to be explicitly reaped upon the
	// end of this this iterator's life.
	snapshot *levigo.Snapshot
	// storage is only set if the iterator is a snapshot of an underlying
	// database.  This signals that it needs to be explicitly reaped upon the
	// end of this this iterator's life.  The snapshot must be freed in the
	// context of an actual database.
	storage *levigo.DB
	// closed indicates whether the iterator has been closed before.
	closed bool
	// valid indicates whether the iterator may be used.  If a LevelDB iterator
	// ever becomes invalid, it must be disposed of and cannot be reused.
	valid bool
	// creationTime provides the time at which the iterator was made.
	creationTime time.Time
}

func (i levigoIterator) String() string {
	var (
		valid       = "valid"
		open        = "open"
		snapshotted = "snapshotted"
	)

	if i.closed {
		open = "closed"
	}
	if !i.valid {
		valid = "invalid"
	}
	if i.snapshot == nil {
		snapshotted = "unsnapshotted"
	}

	return fmt.Sprintf("levigoIterator created at %s that is %s and %s and %s", i.creationTime, open, valid, snapshotted)
}

func (i *levigoIterator) Close() {
	if i.closed {
		return
	}

	if i.iterator != nil {
		i.iterator.Close()
	}
	if i.readOptions != nil {
		i.readOptions.Close()
	}
	if i.snapshot != nil {
		i.storage.ReleaseSnapshot(i.snapshot)
	}

	// Explicitly dereference the pointers to prevent cycles, however unlikely.
	i.iterator = nil
	i.readOptions = nil
	i.snapshot = nil
	i.storage = nil

	i.closed = true
	i.valid = false

	return
}

func (i *levigoIterator) Seek(key []byte) bool {
	i.iterator.Seek(key)

	i.valid = i.iterator.Valid()

	return i.valid
}

func (i *levigoIterator) SeekToFirst() bool {
	i.iterator.SeekToFirst()

	i.valid = i.iterator.Valid()

	return i.valid
}

func (i *levigoIterator) SeekToLast() bool {
	i.iterator.SeekToLast()

	i.valid = i.iterator.Valid()

	return i.valid
}

func (i *levigoIterator) Next() bool {
	i.iterator.Next()

	i.valid = i.iterator.Valid()

	return i.valid
}

func (i *levigoIterator) Previous() bool {
	i.iterator.Prev()

	i.valid = i.iterator.Valid()

	return i.valid
}

func (i levigoIterator) Key() (key []byte) {
	return i.iterator.Key()
}

func (i levigoIterator) Value() (value []byte) {
	return i.iterator.Value()
}

func (i levigoIterator) GetError() (err error) {
	return i.iterator.GetError()
}

// NewLevelDBPersistence furnishes a new LevelDB storage unit.  An optional
// custom comparator may be provided.
func NewLevelDBPersistence(storageRoot string, cacheCapacity, bitsPerBloomFilterEncoded int, comparator native.Comparator) (p *LevelDBPersistence, err error) {
	options := levigo.NewOptions()
	if comparator != nil {
		options.SetComparator(comparator.Comparator())
	}
	options.SetCreateIfMissing(true)
	options.SetParanoidChecks(*leveldbUseParanoidChecks)
	compression := levigo.NoCompression
	if *leveldbUseSnappy {
		compression = levigo.SnappyCompression
	}
	options.SetCompression(compression)

	cache := levigo.NewLRUCache(cacheCapacity)
	options.SetCache(cache)

	filterPolicy := levigo.NewBloomFilter(bitsPerBloomFilterEncoded)
	options.SetFilterPolicy(filterPolicy)

	storage, err := levigo.Open(storageRoot, options)
	if err != nil {
		return
	}

	var (
		readOptions  = levigo.NewReadOptions()
		writeOptions = levigo.NewWriteOptions()
	)

	writeOptions.SetSync(*leveldbFlushOnMutate)
	p = &LevelDBPersistence{
		cache:        cache,
		comparator:   comparator,
		filterPolicy: filterPolicy,
		options:      options,
		readOptions:  readOptions,
		storage:      storage,
		writeOptions: writeOptions,
	}

	return
}

func (l *LevelDBPersistence) Close() {
	// These are deferred to take advantage of forced closing in case of stack
	// unwinding due to anomalies.
	defer func() {
		if l.storage != nil {
			l.storage.Close()
		}
	}()

	defer func() {
		if l.filterPolicy != nil {
			l.filterPolicy.Close()
		}
	}()

	defer func() {
		if l.cache != nil {
			l.cache.Close()
		}
	}()

	defer func() {
		if l.options != nil {
			l.options.Close()
		}
	}()

	defer func() {
		if l.readOptions != nil {
			l.readOptions.Close()
		}
	}()

	defer func() {
		if l.writeOptions != nil {
			l.writeOptions.Close()
		}
	}()

	defer func() {
		if l.comparator != nil {
			l.comparator.Close()
		}
	}()

	return
}

func (l *LevelDBPersistence) Get(value coding.Encoder) (b []byte, err error) {
	key, err := value.Encode()
	if err != nil {
		return
	}

	return l.storage.Get(l.readOptions, key)
}

func (l *LevelDBPersistence) Has(value coding.Encoder) (h bool, err error) {
	raw, err := l.Get(value)
	if err != nil {
		return
	}

	h = raw != nil

	return
}

func (l *LevelDBPersistence) Drop(value coding.Encoder) (err error) {
	key, err := value.Encode()
	if err != nil {
		return
	}

	err = l.storage.Delete(l.writeOptions, key)

	return
}

func (l *LevelDBPersistence) Put(key, value coding.Encoder) (err error) {
	keyEncoded, err := key.Encode()
	if err != nil {
		return
	}

	valueEncoded, err := value.Encode()
	if err != nil {
		return
	}

	err = l.storage.Put(l.writeOptions, keyEncoded, valueEncoded)

	return
}

func (l *LevelDBPersistence) Commit(b raw.Batch) (err error) {
	// XXX: This is a wart to clean up later.  Ideally, after doing extensive
	//      tests, we could create a Batch struct that journals pending
	//      operations which the given Persistence implementation could convert
	//      to its specific commit requirements.
	batch, ok := b.(batch)
	if !ok {
		panic("leveldb.batch expected")
	}

	return l.storage.Write(l.writeOptions, batch.batch)
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
func (l *LevelDBPersistence) NewIterator(snapshotted bool) Iterator {
	var (
		snapshot    *levigo.Snapshot
		readOptions *levigo.ReadOptions
		iterator    *levigo.Iterator
	)

	if snapshotted {
		snapshot = l.storage.NewSnapshot()
		readOptions = levigo.NewReadOptions()
		readOptions.SetSnapshot(snapshot)
		iterator = l.storage.NewIterator(readOptions)
	} else {
		iterator = l.storage.NewIterator(l.readOptions)
	}

	return &levigoIterator{
		creationTime: time.Now(),
		iterator:     iterator,
		readOptions:  readOptions,
		snapshot:     snapshot,
		storage:      l.storage,
	}
}

func (l *LevelDBPersistence) ForEach(decoder storage.RecordDecoder, filter storage.RecordFilter, operator storage.RecordOperator) (scannedEntireCorpus bool, err error) {
	var (
		iterator = l.NewIterator(true)
		valid    bool
	)
	defer iterator.Close()

	for valid = iterator.SeekToFirst(); valid; valid = iterator.Next() {
		err = iterator.GetError()
		if err != nil {
			return
		}

		decodedKey, decodeErr := decoder.DecodeKey(iterator.Key())
		if decodeErr != nil {
			continue
		}
		decodedValue, decodeErr := decoder.DecodeValue(iterator.Value())
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
	scannedEntireCorpus = true
	return
}
