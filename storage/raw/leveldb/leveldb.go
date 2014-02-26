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
	"time"

	"code.google.com/p/goprotobuf/proto"
	"github.com/jmhodges/levigo"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw"
)

// LevelDBPersistence is a disk-backed sorted key-value store. It implements the
// interfaces raw.Database, raw.ForEacher, raw.Pruner, raw.Persistence.
type LevelDBPersistence struct {
	path    string
	name    string
	purpose string

	cache        *levigo.Cache
	filterPolicy *levigo.FilterPolicy
	options      *levigo.Options
	storage      *levigo.DB
	readOptions  *levigo.ReadOptions
	writeOptions *levigo.WriteOptions
}

// levigoIterator wraps the LevelDB resources in a convenient manner for uniform
// resource access and closing through the raw.Iterator protocol.
type levigoIterator struct {
	// iterator is the receiver of most proxied operation calls.
	iterator *levigo.Iterator
	// readOptions is only set if the iterator is a snapshot of an
	// underlying database.  This signals that it needs to be explicitly
	// reaped upon the end of this iterator's life.
	readOptions *levigo.ReadOptions
	// snapshot is only set if the iterator is a snapshot of an underlying
	// database.  This signals that it needs to be explicitly reaped upon
	// the end of this this iterator's life.
	snapshot *levigo.Snapshot
	// storage is only set if the iterator is a snapshot of an underlying
	// database.  This signals that it needs to be explicitly reaped upon
	// the end of this this iterator's life.  The snapshot must be freed in
	// the context of an actual database.
	storage *levigo.DB
	// closed indicates whether the iterator has been closed before.
	closed bool
	// valid indicates whether the iterator may be used.  If a LevelDB
	// iterator ever becomes invalid, it must be disposed of and cannot be
	// reused.
	valid bool
	// creationTime provides the time at which the iterator was made.
	creationTime time.Time
}

func (i levigoIterator) String() string {
	valid := "valid"
	open := "open"
	snapshotted := "snapshotted"

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

func (i *levigoIterator) Close() error {
	if i.closed {
		return nil
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

	return nil
}

func (i *levigoIterator) Seek(m proto.Message) bool {
	buf, _ := buffers.Get()
	defer buffers.Give(buf)

	if err := buf.Marshal(m); err != nil {
		panic(err)
	}

	i.iterator.Seek(buf.Bytes())

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

func (i *levigoIterator) rawKey() (key []byte) {
	return i.iterator.Key()
}

func (i *levigoIterator) Error() (err error) {
	return i.iterator.GetError()
}

func (i *levigoIterator) Key(m proto.Message) error {
	buf, _ := buffers.Get()
	defer buffers.Give(buf)

	buf.SetBuf(i.iterator.Key())

	return buf.Unmarshal(m)
}

func (i *levigoIterator) RawValue() []byte {
	return i.iterator.Value()
}

func (i *levigoIterator) Valid() bool {
	return i.valid
}

// Compression defines the compression mode.
type Compression uint

// Possible compression modes.
const (
	Snappy Compression = iota
	Uncompressed
)

// LevelDBOptions bundles options needed to create a LevelDBPersistence object.
type LevelDBOptions struct {
	Path    string
	Name    string
	Purpose string

	CacheSizeBytes    int
	OpenFileAllowance int

	FlushOnMutate     bool
	UseParanoidChecks bool

	Compression Compression
}

// NewLevelDBPersistence returns an initialized LevelDBPersistence object,
// created with the given options.
func NewLevelDBPersistence(o LevelDBOptions) (*LevelDBPersistence, error) {
	options := levigo.NewOptions()
	options.SetCreateIfMissing(true)
	options.SetParanoidChecks(o.UseParanoidChecks)

	compression := levigo.SnappyCompression
	if o.Compression == Uncompressed {
		compression = levigo.NoCompression
	}
	options.SetCompression(compression)

	cache := levigo.NewLRUCache(o.CacheSizeBytes)
	options.SetCache(cache)

	filterPolicy := levigo.NewBloomFilter(10)
	options.SetFilterPolicy(filterPolicy)

	options.SetMaxOpenFiles(o.OpenFileAllowance)

	storage, err := levigo.Open(o.Path, options)
	if err != nil {
		return nil, err
	}

	readOptions := levigo.NewReadOptions()

	writeOptions := levigo.NewWriteOptions()
	writeOptions.SetSync(o.FlushOnMutate)

	return &LevelDBPersistence{
		path:    o.Path,
		name:    o.Name,
		purpose: o.Purpose,

		cache:        cache,
		filterPolicy: filterPolicy,

		options:      options,
		readOptions:  readOptions,
		writeOptions: writeOptions,

		storage: storage,
	}, nil
}

// Close implements raw.Persistence (and raw.Database).
func (l *LevelDBPersistence) Close() error {
	// These are deferred to take advantage of forced closing in case of
	// stack unwinding due to anomalies.
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
		if l.storage != nil {
			l.storage.Close()
		}
	}()

	return nil
}

// Get implements raw.Persistence.
func (l *LevelDBPersistence) Get(k, v proto.Message) (bool, error) {
	buf, _ := buffers.Get()
	defer buffers.Give(buf)

	if err := buf.Marshal(k); err != nil {
		panic(err)
	}

	raw, err := l.storage.Get(l.readOptions, buf.Bytes())
	if err != nil {
		return false, err
	}
	if raw == nil {
		return false, nil
	}

	if v == nil {
		return true, nil
	}

	buf.SetBuf(raw)

	if err := buf.Unmarshal(v); err != nil {
		return true, err
	}

	return true, nil
}

// Has implements raw.Persistence.
func (l *LevelDBPersistence) Has(k proto.Message) (has bool, err error) {
	return l.Get(k, nil)
}

// Drop implements raw.Persistence.
func (l *LevelDBPersistence) Drop(k proto.Message) error {
	buf, _ := buffers.Get()
	defer buffers.Give(buf)

	if err := buf.Marshal(k); err != nil {
		panic(err)
	}

	return l.storage.Delete(l.writeOptions, buf.Bytes())
}

// Put implements raw.Persistence.
func (l *LevelDBPersistence) Put(k, v proto.Message) error {
	keyBuf, _ := buffers.Get()
	defer buffers.Give(keyBuf)

	if err := keyBuf.Marshal(k); err != nil {
		panic(err)
	}

	valBuf, _ := buffers.Get()
	defer buffers.Give(valBuf)

	if err := valBuf.Marshal(v); err != nil {
		panic(err)
	}

	return l.storage.Put(l.writeOptions, keyBuf.Bytes(), valBuf.Bytes())
}

// PutRaw implements raw.Persistence.
func (l *LevelDBPersistence) PutRaw(key proto.Message, value []byte) error {
	keyBuf, _ := buffers.Get()
	defer buffers.Give(keyBuf)

	if err := keyBuf.Marshal(key); err != nil {
		panic(err)
	}

	return l.storage.Put(l.writeOptions, keyBuf.Bytes(), value)
}

// Commit implements raw.Persistence.
func (l *LevelDBPersistence) Commit(b raw.Batch) (err error) {
	// XXX: This is a wart to clean up later.  Ideally, after doing
	// extensive tests, we could create a Batch struct that journals pending
	// operations which the given Persistence implementation could convert
	// to its specific commit requirements.
	batch, ok := b.(*batch)
	if !ok {
		panic("leveldb.batch expected")
	}

	return l.storage.Write(l.writeOptions, batch.batch)
}

// Prune implements raw.Pruner. It compacts the entire keyspace of the database.
//
// Beware that it would probably be imprudent to run this on a live user-facing
// server due to latency implications.
func (l *LevelDBPersistence) Prune() {

	// Magic values per https://code.google.com/p/leveldb/source/browse/include/leveldb/db.h#131.
	keyspace := levigo.Range{
		Start: nil,
		Limit: nil,
	}

	l.storage.CompactRange(keyspace)
}

// Size returns the approximate size the entire database takes on disk (in
// bytes). It implements the raw.Database interface.
func (l *LevelDBPersistence) Size() (uint64, error) {
	iterator, err := l.NewIterator(false)
	if err != nil {
		return 0, err
	}
	defer iterator.Close()

	if !iterator.SeekToFirst() {
		return 0, fmt.Errorf("could not seek to first key")
	}

	keyspace := levigo.Range{}

	keyspace.Start = iterator.rawKey()

	if !iterator.SeekToLast() {
		return 0, fmt.Errorf("could not seek to last key")
	}

	keyspace.Limit = iterator.rawKey()

	sizes := l.storage.GetApproximateSizes([]levigo.Range{keyspace})
	total := uint64(0)
	for _, size := range sizes {
		total += size
	}

	return total, nil
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
	}, nil
}

// ForEach implements raw.ForEacher.
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

		decodedKey, decodeErr := decoder.DecodeKey(iterator.rawKey())
		if decodeErr != nil {
			continue
		}
		decodedValue, decodeErr := decoder.DecodeValue(iterator.RawValue())
		if decodeErr != nil {
			continue
		}

		switch filter.Filter(decodedKey, decodedValue) {
		case storage.Stop:
			return
		case storage.Skip:
			continue
		case storage.Accept:
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
