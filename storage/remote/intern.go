// Copyright The Prometheus Authors
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
//
// Inspired / copied / modified from https://gitlab.com/cznic/strutil/blob/master/strutil.go,
// which is MIT licensed, so:
//
// Copyright (c) 2014 The strutil Authors. All rights reserved.

package remote

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/atomic"
)

var noReferenceReleases = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: namespace,
	Subsystem: subsystem,
	Name:      "string_interner_zero_reference_releases_total",
	Help:      "The number of times release has been called for strings that are not interned.",
})

// poolShard is a single shard of the intern pool. Each shard has its own
// lock so that concurrent intern/release calls on different strings do not
// contend on a single mutex.
type poolShard struct {
	mtx  sync.RWMutex
	pool map[string]*entry
}

// pool is a sharded string interner. Strings are distributed across shards
// by hash, so that calls on different strings do not serialize on a single lock.
type pool struct {
	shards [numInternShards]*poolShard
}

const numInternShards = 32

type entry struct {
	refs atomic.Int64

	s string
}

func newEntry(s string) *entry {
	return &entry{s: s}
}

func newPool() *pool {
	p := &pool{}
	for i := range numInternShards {
		p.shards[i] = &poolShard{pool: map[string]*entry{}}
	}
	return p
}

// fnv32a computes the FNV-1a 32-bit hash of s. Inlined to avoid the
// allocation overhead of hash/fnv.New32a on every call.
func fnv32a(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// shard returns the shard that handles the given string.
func (p *pool) shard(s string) *poolShard {
	return p.shards[fnv32a(s)&(numInternShards-1)]
}

func (p *pool) intern(s string) string {
	if s == "" {
		return ""
	}

	shard := p.shard(s)

	shard.mtx.RLock()
	interned, ok := shard.pool[s]
	if ok {
		// Increase the reference count while we're still holding the read lock,
		// This will prevent the release() from deleting the entry while we're increasing its ref count.
		interned.refs.Inc()
		shard.mtx.RUnlock()
		return interned.s
	}
	shard.mtx.RUnlock()

	shard.mtx.Lock()
	defer shard.mtx.Unlock()
	if interned, ok := shard.pool[s]; ok {
		interned.refs.Inc()
		return interned.s
	}

	shard.pool[s] = newEntry(s)
	shard.pool[s].refs.Store(1)
	return s
}

func (p *pool) release(s string) {
	shard := p.shard(s)

	shard.mtx.RLock()
	interned, ok := shard.pool[s]
	shard.mtx.RUnlock()

	if !ok {
		noReferenceReleases.Inc()
		return
	}

	refs := interned.refs.Dec()
	if refs > 0 {
		return
	}

	shard.mtx.Lock()
	defer shard.mtx.Unlock()
	if interned.refs.Load() != 0 {
		return
	}
	delete(shard.pool, s)
}
