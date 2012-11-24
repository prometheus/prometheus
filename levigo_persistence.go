package main

import (
	"github.com/jmhodges/levigo"
	"io"
)

type LevigoCloser interface {
	Close() error
}

type LevigoPersistence struct {
	cache        *levigo.Cache
	filterPolicy *levigo.FilterPolicy
	options      *levigo.Options
	storage      *levigo.DB
	readOptions  *levigo.ReadOptions
	writeOptions *levigo.WriteOptions
}

func NewLevigoPersistence(storageRoot string, cacheCapacity, bitsPerBloomFilterEncoded int) (*LevigoPersistence, error) {
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

	emission := &LevigoPersistence{
		cache:        cache,
		filterPolicy: filterPolicy,
		options:      options,
		readOptions:  readOptions,
		storage:      storage,
		writeOptions: writeOptions,
	}

	return emission, openErr
}

func (l *LevigoPersistence) Close() error {
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

func (l *LevigoPersistence) Get(value Encoder) ([]byte, error) {
	var key []byte
	var keyError error

	if key, keyError = value.Encode(); keyError == nil {
		return l.storage.Get(l.readOptions, key)
	}

	return nil, keyError
}

func (l *LevigoPersistence) Has(value Encoder) (bool, error) {
	if value, getError := l.Get(value); getError != nil {
		return false, getError
	} else if value == nil {
		return false, nil
	}

	return true, nil
}

func (l *LevigoPersistence) Drop(value Encoder) error {
	var key []byte
	var keyError error

	if key, keyError = value.Encode(); keyError == nil {

		if deleteError := l.storage.Delete(l.writeOptions, key); deleteError != nil {
			return deleteError
		}

		return nil
	}

	return keyError
}

func (l *LevigoPersistence) Put(key Encoder, value Encoder) error {
	var keyEncoded []byte
	var keyError error

	if keyEncoded, keyError = key.Encode(); keyError == nil {
		if valueEncoded, valueError := value.Encode(); valueError == nil {

			if putError := l.storage.Put(l.writeOptions, keyEncoded, valueEncoded); putError != nil {
				return putError
			}
		} else {
			return valueError
		}

		return nil
	}

	return keyError
}

func (l *LevigoPersistence) GetAll() ([]Pair, error) {
	snapshot := l.storage.NewSnapshot()
	defer l.storage.ReleaseSnapshot(snapshot)
	readOptions := levigo.NewReadOptions()
	defer readOptions.Close()

	readOptions.SetSnapshot(snapshot)
	iterator := l.storage.NewIterator(readOptions)
	defer iterator.Close()
	iterator.SeekToFirst()

	result := make([]Pair, 0)

	for iterator := iterator; iterator.Valid(); iterator.Next() {
		result = append(result, Pair{Left: iterator.Key(), Right: iterator.Value()})
	}

	iteratorError := iterator.GetError()

	if iteratorError != nil {
		return nil, iteratorError
	}

	return result, nil
}

type iteratorCloser struct {
	iterator    *levigo.Iterator
	readOptions *levigo.ReadOptions
	snapshot    *levigo.Snapshot
	storage     *levigo.DB
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

func (l *LevigoPersistence) GetIterator() (*levigo.Iterator, io.Closer, error) {
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
