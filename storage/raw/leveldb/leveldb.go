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
	"github.com/jmhodges/levigo"
	"github.com/matttproud/prometheus/coding"
	"github.com/matttproud/prometheus/storage/raw"
	"io"
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

func NewLevelDBPersistence(storageRoot string, cacheCapacity, bitsPerBloomFilterEncoded int) (*LevelDBPersistence, error) {
	options := levigo.NewOptions()
	options.SetCreateIfMissing(true)
	options.SetParanoidChecks(true)

	cache := levigo.NewLRUCache(cacheCapacity)
	options.SetCache(cache)

	filterPolicy := levigo.NewBloomFilter(bitsPerBloomFilterEncoded)
	options.SetFilterPolicy(filterPolicy)

	storage, openErr := levigo.Open(storageRoot, options)

	readOptions := levigo.NewReadOptions()
	writeOptions := levigo.NewWriteOptions()
	writeOptions.SetSync(true)

	emission := &LevelDBPersistence{
		cache:        cache,
		filterPolicy: filterPolicy,
		options:      options,
		readOptions:  readOptions,
		storage:      storage,
		writeOptions: writeOptions,
	}

	return emission, openErr
}

func (l *LevelDBPersistence) Close() error {
	if l.storage != nil {
		l.storage.Close()
	}

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

	return nil
}

func (l *LevelDBPersistence) Get(value coding.Encoder) ([]byte, error) {
	if key, keyError := value.Encode(); keyError == nil {
		return l.storage.Get(l.readOptions, key)
	} else {
		return nil, keyError
	}

	panic("unreachable")
}

func (l *LevelDBPersistence) Has(value coding.Encoder) (bool, error) {
	if value, getError := l.Get(value); getError != nil {
		return false, getError
	} else if value == nil {
		return false, nil
	}

	return true, nil
}

func (l *LevelDBPersistence) Drop(value coding.Encoder) error {
	if key, keyError := value.Encode(); keyError == nil {

		if deleteError := l.storage.Delete(l.writeOptions, key); deleteError != nil {
			return deleteError
		}
	} else {
		return keyError
	}

	return nil
}

func (l *LevelDBPersistence) Put(key, value coding.Encoder) error {
	if keyEncoded, keyError := key.Encode(); keyError == nil {
		if valueEncoded, valueError := value.Encode(); valueError == nil {

			if putError := l.storage.Put(l.writeOptions, keyEncoded, valueEncoded); putError != nil {
				return putError
			}
		} else {
			return valueError
		}
	} else {
		return keyError
	}

	return nil
}

func (l *LevelDBPersistence) GetAll() ([]raw.Pair, error) {
	snapshot := l.storage.NewSnapshot()
	defer l.storage.ReleaseSnapshot(snapshot)
	readOptions := levigo.NewReadOptions()
	defer readOptions.Close()

	readOptions.SetSnapshot(snapshot)
	iterator := l.storage.NewIterator(readOptions)
	defer iterator.Close()
	iterator.SeekToFirst()

	result := make([]raw.Pair, 0)

	for iterator := iterator; iterator.Valid(); iterator.Next() {
		result = append(result, raw.Pair{Left: iterator.Key(), Right: iterator.Value()})
	}

	iteratorError := iterator.GetError()

	if iteratorError != nil {
		return nil, iteratorError
	}

	return result, nil
}

func (i *iteratorCloser) Close() error {
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

	return nil
}

func (l *LevelDBPersistence) GetIterator() (*levigo.Iterator, io.Closer, error) {
	snapshot := l.storage.NewSnapshot()
	readOptions := levigo.NewReadOptions()
	readOptions.SetSnapshot(snapshot)
	iterator := l.storage.NewIterator(readOptions)

	closer := &iteratorCloser{
		iterator:    iterator,
		readOptions: readOptions,
		snapshot:    snapshot,
		storage:     l.storage,
	}

	return iterator, closer, nil
}
