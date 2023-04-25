// Copyright 2023 The Prometheus Authors
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

// If we decide to employ this auto generation of markdown documentation for
// amtool and alertmanager, this package could potentially be moved to
// prometheus/common. However, it is crucial to note that this functionality is
// tailored specifically to the way in which the Prometheus documentation is
// rendered, and should be avoided for use by third-party users.

package cache

import (
	"sync"
	"time"

	"go.uber.org/atomic"
)

const (
	shardSize      = 256
	cleanBatchSize = 1000
)

var Default = NewDictCacheWithTTL(60 * 6)

type dictValue struct {
	id    int64
	value string
	ts    int32
}

type DictCache struct {
	dbk     []map[string]*dictValue
	dbv     []map[int64]*dictValue
	kLocks  []*sync.RWMutex
	vLocks  []*sync.RWMutex
	inc     atomic.Int64
	current int32
	ttl     int32
	stopped bool
	closeCh chan interface{}
}

// ttl unit is minute, not second
func NewDictCacheWithTTL(ttl int32) *DictCache {
	dc := DictCache{
		dbk:     make([]map[string]*dictValue, shardSize),
		dbv:     make([]map[int64]*dictValue, shardSize),
		kLocks:  make([]*sync.RWMutex, shardSize),
		vLocks:  make([]*sync.RWMutex, shardSize),
		ttl:     ttl,
		closeCh: make(chan interface{}, 1),
		stopped: true,
	}
	for i := 0; i < shardSize; i++ {
		dc.dbk[i] = make(map[string]*dictValue)
		dc.dbv[i] = make(map[int64]*dictValue)
		dc.kLocks[i] = &sync.RWMutex{}
		dc.vLocks[i] = &sync.RWMutex{}
	}
	return &dc

}

func NewDictCache() *DictCache {
	dc := NewDictCacheWithTTL(0)
	return dc
}

func (d *DictCache) gc() {
	delBuff := make([]string, cleanBatchSize)
	var delIdx, dbIdx int
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	shiftTs := time.Now().Unix()
	for !d.stopped && d.ttl > 0 {
		select {
		case <-d.closeCh:
			return
		case <-ticker.C:
		}
		if now := time.Now().Unix(); now-shiftTs > 60 { //one minute
			d.current++
			shiftTs = now
		}
		if dbIdx >= shardSize {
			dbIdx %= shardSize
		}
		delIdx = 0
		scan := 0
		now := d.current
		minTs := now - d.ttl
		d.vLocks[dbIdx].RLock()
		for _, v := range d.dbv[dbIdx] {
			if v.ts < minTs {
				delBuff[delIdx] = v.value
			}
			scan++
			if scan >= cleanBatchSize {
				break
			}
		}
		d.vLocks[dbIdx].RUnlock()
		d.del(delBuff[:delIdx])
		dbIdx++
	}
}

func (d *DictCache) del(keys []string) {
	for _, key := range keys {
		shard := d.shard(key)
		d.kLocks[shard].Lock()
		v, ok := d.dbk[shard][key]
		if !ok {
			d.kLocks[shard].Unlock()
			continue
		}
		delete(d.dbk[shard], key)
		d.kLocks[shard].Unlock()

		shardV := v.id % shardSize
		d.vLocks[shardV].Lock()
		delete(d.dbv[shardV], v.id)
		d.vLocks[shardV].Unlock()
	}
}

func (d *DictCache) shard(key string) int {
	l := len(key)
	if l == 0 {
		return 0
	}
	s := int(key[0])
	s <<= 8
	s += int(key[l-1])
	s %= shardSize
	return s
}

func (d *DictCache) Get(key string) int64 {
	l := len(key)
	if l == 0 {
		return 0
	}
	s := d.shard(key)

	d.kLocks[s].RLock()
	if v, ok := d.dbk[s][key]; ok {
		if d.current != v.ts {
			v.ts = d.current
		}
		d.kLocks[s].RUnlock()
		return v.id
	}
	d.kLocks[s].RUnlock()

	d.kLocks[s].Lock()
	if v, ok := d.dbk[s][key]; ok {
		d.kLocks[s].Unlock()
		return v.id
	}
	id := d.inc.Add(1)
	v := &dictValue{
		id:    id,
		value: key,
		ts:    d.current,
	}
	d.dbk[s][key] = v
	d.kLocks[s].Unlock()

	shardV := id % shardSize
	d.vLocks[shardV].Lock()
	d.dbv[shardV][id] = v
	d.vLocks[shardV].Unlock()

	return id
}

func (d *DictCache) Value(id int64) (value string, ok bool) {
	if id <= 0 {
		return "", false
	}
	shardV := id % shardSize
	d.vLocks[shardV].RLock()
	defer d.vLocks[shardV].RUnlock()
	if v, ok := d.dbv[shardV][id]; ok {
		if c := d.current; c != v.ts {
			v.ts = c
		}
		return v.value, true
	}
	return "", false
}

func (d *DictCache) RunGC() {
	d.vLocks[0].Lock()
	defer d.vLocks[0].Unlock()
	if !d.stopped {
		return
	}
	d.stopped = false
	go d.gc()
}

func (d *DictCache) Stop() {
	d.vLocks[0].Lock()
	defer d.vLocks[0].Unlock()
	if d.stopped {
		return
	}
	d.closeCh <- ""
	d.stopped = true
}
