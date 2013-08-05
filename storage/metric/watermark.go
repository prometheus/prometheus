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

package metric

import (
	"container/list"
	"io"
	"sync"
	"time"

	"code.google.com/p/goprotobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"

	dto "github.com/prometheus/prometheus/model/generated"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
)

// unsafe.Sizeof(watermarks{})
const elementSize = 24

type Bytes uint64

// WatermarkCache is a thread-safe LRU cache for fingerprint watermark
// state.
type WatermarkCache struct {
	mu sync.Mutex

	list  *list.List
	table map[clientmodel.Fingerprint]*list.Element

	size Bytes

	allowance Bytes
}

type watermarks struct {
	High time.Time
}

func (w *watermarks) load(d *dto.MetricHighWatermark) {
	w.High = time.Unix(d.GetTimestamp(), 0).UTC()
}

func (w *watermarks) dump(d *dto.MetricHighWatermark) {
	d.Reset()

	d.Timestamp = proto.Int64(w.High.Unix())
}

type entry struct {
	fingerprint *clientmodel.Fingerprint
	watermarks  *watermarks
	accessed    time.Time
}

func NewWatermarkCache(allowance Bytes) *WatermarkCache {
	return &WatermarkCache{
		list:      list.New(),
		table:     map[clientmodel.Fingerprint]*list.Element{},
		allowance: allowance,
	}
}

func (lru *WatermarkCache) Get(f *clientmodel.Fingerprint) (v *watermarks, ok bool) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	element, ok := lru.table[*f]
	if !ok {
		return nil, false
	}

	lru.moveToFront(element)

	return element.Value.(*entry).watermarks, true
}

func (lru *WatermarkCache) Set(f *clientmodel.Fingerprint, w *watermarks) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if element, ok := lru.table[*f]; ok {
		lru.updateInplace(element, w)
	} else {
		lru.addNew(f, w)
	}
}

func (lru *WatermarkCache) SetIfAbsent(f *clientmodel.Fingerprint, w *watermarks) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if element, ok := lru.table[*f]; ok {
		lru.moveToFront(element)
	} else {
		lru.addNew(f, w)
	}
}

func (lru *WatermarkCache) Delete(f *clientmodel.Fingerprint) bool {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	element, ok := lru.table[*f]
	if !ok {
		return false
	}

	lru.list.Remove(element)
	delete(lru.table, *f)
	lru.size -= elementSize

	return true
}

func (lru *WatermarkCache) Clear() {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	lru.list.Init()
	lru.table = map[clientmodel.Fingerprint]*list.Element{}
	lru.size = 0
}

func (lru *WatermarkCache) updateInplace(e *list.Element, w *watermarks) {
	e.Value.(*entry).watermarks = w
	lru.moveToFront(e)
	lru.checkCapacity()
}

func (lru *WatermarkCache) moveToFront(e *list.Element) {
	lru.list.MoveToFront(e)
	e.Value.(*entry).accessed = time.Now()
}

func (lru *WatermarkCache) addNew(f *clientmodel.Fingerprint, w *watermarks) {
	lru.table[*f] = lru.list.PushFront(&entry{
		fingerprint: f,
		watermarks:  w,
		accessed:    time.Now(),
	})
	lru.size += elementSize
	lru.checkCapacity()
}

func (lru *WatermarkCache) checkCapacity() {
	for lru.size > lru.allowance {
		delElem := lru.list.Back()
		delWatermarks := delElem.Value.(*entry)
		lru.list.Remove(delElem)
		delete(lru.table, *delWatermarks.fingerprint)
		lru.size -= elementSize
	}
}

type FingerprintHighWatermarkMapping map[clientmodel.Fingerprint]time.Time

type HighWatermarker interface {
	io.Closer
	raw.ForEacher
	raw.Pruner

	UpdateBatch(FingerprintHighWatermarkMapping) error
	Get(*clientmodel.Fingerprint) (t time.Time, ok bool, err error)
	State() *raw.DatabaseState
	Size() (uint64, bool, error)
}

type LeveldbHighWatermarker struct {
	p *leveldb.LevelDBPersistence
}

func (w *LeveldbHighWatermarker) Get(f *clientmodel.Fingerprint) (t time.Time, ok bool, err error) {
	k := new(dto.Fingerprint)
	dumpFingerprint(k, f)
	v := new(dto.MetricHighWatermark)
	ok, err = w.p.Get(k, v)
	if err != nil {
		return t, ok, err
	}
	if !ok {
		return t, ok, err
	}
	t = time.Unix(v.GetTimestamp(), 0)
	return t, true, nil
}

func (w *LeveldbHighWatermarker) UpdateBatch(m FingerprintHighWatermarkMapping) error {
	batch := leveldb.NewBatch()
	defer batch.Close()

	for fp, t := range m {
		existing, present, err := w.Get(&fp)
		if err != nil {
			return err
		}
		k := new(dto.Fingerprint)
		dumpFingerprint(k, &fp)
		v := new(dto.MetricHighWatermark)
		if !present {
			v.Timestamp = proto.Int64(t.Unix())
			batch.Put(k, v)

			continue
		}

		// BUG(matt): Replace this with watermark management.
		if t.After(existing) {
			v.Timestamp = proto.Int64(t.Unix())
			batch.Put(k, v)
		}
	}

	return w.p.Commit(batch)
}

func (i *LeveldbHighWatermarker) ForEach(d storage.RecordDecoder, f storage.RecordFilter, o storage.RecordOperator) (bool, error) {
	return i.p.ForEach(d, f, o)
}

func (i *LeveldbHighWatermarker) Prune() (bool, error) {
	i.p.Prune()

	return false, nil
}

func (i *LeveldbHighWatermarker) Close() error {
	i.p.Close()

	return nil
}

func (i *LeveldbHighWatermarker) State() *raw.DatabaseState {
	return i.p.State()
}

func (i *LeveldbHighWatermarker) Size() (uint64, bool, error) {
	s, err := i.p.ApproximateSize()
	return s, true, err
}

type LevelDBHighWatermarkerOptions struct {
	leveldb.LevelDBOptions
}

func NewLevelDBHighWatermarker(o *LevelDBHighWatermarkerOptions) (HighWatermarker, error) {
	s, err := leveldb.NewLevelDBPersistence(&o.LevelDBOptions)
	if err != nil {
		return nil, err
	}

	return &LeveldbHighWatermarker{
		p: s,
	}, nil
}
