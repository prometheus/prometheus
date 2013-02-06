// Copyright 2012 Prometheus Team
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
	"github.com/jmhodges/levigo"
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw"
	"io"
)

var (
	leveldbFlushOnMutate     = flag.Bool("leveldbFlushOnMutate", true, "Whether LevelDB should flush every operation to disk upon mutation before returning (bool).")
	leveldbUseSnappy         = flag.Bool("leveldbUseSnappy", true, "Whether LevelDB attempts to use Snappy for compressing elements (bool).")
	leveldbUseParanoidChecks = flag.Bool("leveldbUseParanoidChecks", true, "Whether LevelDB uses expensive checks (bool).")
)

type LevelDBPersistence struct {
	cache        *levigo.Cache
	filterPolicy *levigo.FilterPolicy
	options      *levigo.Options
	storage      *levigo.DB
	readOptions  *levigo.ReadOptions
	writeOptions *levigo.WriteOptions
}

type iteratorCloser struct {
	iterator    *levigo.Iterator
	readOptions *levigo.ReadOptions
	snapshot    *levigo.Snapshot
	storage     *levigo.DB
}

func NewLevelDBPersistence(storageRoot string, cacheCapacity, bitsPerBloomFilterEncoded int) (p *LevelDBPersistence, err error) {
	options := levigo.NewOptions()
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

	readOptions := levigo.NewReadOptions()
	writeOptions := levigo.NewWriteOptions()
	writeOptions.SetSync(*leveldbFlushOnMutate)
	p = &LevelDBPersistence{
		cache:        cache,
		filterPolicy: filterPolicy,
		options:      options,
		readOptions:  readOptions,
		storage:      storage,
		writeOptions: writeOptions,
	}

	return
}

func (l *LevelDBPersistence) Close() (err error) {
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

func (l *LevelDBPersistence) GetAll() (pairs []raw.Pair, err error) {
	snapshot := l.storage.NewSnapshot()
	defer l.storage.ReleaseSnapshot(snapshot)
	readOptions := levigo.NewReadOptions()
	defer readOptions.Close()

	readOptions.SetSnapshot(snapshot)
	iterator := l.storage.NewIterator(readOptions)
	defer iterator.Close()
	iterator.SeekToFirst()

	for iterator := iterator; iterator.Valid(); iterator.Next() {
		pairs = append(pairs, raw.Pair{Left: iterator.Key(), Right: iterator.Value()})

		err = iterator.GetError()
		if err != nil {
			return
		}
	}

	return
}

func (i *iteratorCloser) Close() (err error) {
	defer func() {
		if i.storage != nil {
			if i.snapshot != nil {
				i.storage.ReleaseSnapshot(i.snapshot)
			}
		}
	}()

	defer func() {
		if i.iterator != nil {
			i.iterator.Close()
		}
	}()

	defer func() {
		if i.readOptions != nil {
			i.readOptions.Close()
		}
	}()

	return
}

func (l *LevelDBPersistence) GetIterator() (i *levigo.Iterator, c io.Closer, err error) {
	snapshot := l.storage.NewSnapshot()
	readOptions := levigo.NewReadOptions()
	readOptions.SetSnapshot(snapshot)
	i = l.storage.NewIterator(readOptions)

	c = &iteratorCloser{
		iterator:    i,
		readOptions: readOptions,
		snapshot:    snapshot,
		storage:     l.storage,
	}

	return
}

func (l *LevelDBPersistence) ForEach(decoder storage.RecordDecoder, filter storage.RecordFilter, operator storage.RecordOperator) (scannedEntireCorpus bool, err error) {
	iterator, closer, err := l.GetIterator()
	if err != nil {
		return
	}
	defer closer.Close()

	for iterator.SeekToFirst(); iterator.Valid(); iterator.Next() {
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
