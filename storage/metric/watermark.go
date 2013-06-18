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
	"sync"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	dto "github.com/prometheus/prometheus/model/generated"
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
	w.High = time.Unix(d.GetTimestamp(), 0).Time.Unix()
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

// // ToMetricHighWatermarkDTO builds a MetricHighWatermark DTO out of a given
// // Watermark.
// func (w watermark) ToMetricHighWatermarkDTO() *dto.MetricHighWatermark {
// 	return &dto.MetricHighWatermark{
// 		Timestamp: proto.Int64(w.Time.Unix()),
// 	}
// }

// // NewWatermarkFromHighWatermarkDTO builds Watermark from the provided
// // dto.MetricHighWatermark object.
// func NewWatermarkFromHighWatermarkDTO(d *dto.MetricHighWatermark) Watermark {
// 	return Watermark{
// 		time.Unix(*d.Timestamp, 0).UTC(),
// 	}
// }

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
