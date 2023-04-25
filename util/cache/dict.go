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

	"go.uber.org/atomic"
)

const (
	shardSize = 256
)

type dictValue struct {
	id    int64
	value string
}

type DictCache struct {
	dbk    []map[string]*dictValue
	dbv    []map[int64]*dictValue
	kLocks []*sync.RWMutex
	vLocks []*sync.RWMutex
	inc    atomic.Int64
}

func NewDictCache() *DictCache {
	dc := DictCache{
		dbk:    make([]map[string]*dictValue, shardSize),
		dbv:    make([]map[int64]*dictValue, shardSize),
		kLocks: make([]*sync.RWMutex, shardSize),
		vLocks: make([]*sync.RWMutex, shardSize),
	}
	for i := 0; i < shardSize; i++ {
		dc.dbk[i] = make(map[string]*dictValue)
		dc.dbv[i] = make(map[int64]*dictValue)
		dc.kLocks[i] = &sync.RWMutex{}
		dc.vLocks[i] = &sync.RWMutex{}
	}
	return &dc
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
		d.kLocks[s].RUnlock()
		return v.id
	}
	d.kLocks[s].RUnlock()

	id := d.inc.Add(1)
	d.kLocks[s].Lock()
	v := &dictValue{
		id:    id,
		value: key,
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
		return v.value, true
	}
	return "", false
}
