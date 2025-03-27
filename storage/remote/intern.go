// Copyright 2019 The Prometheus Authors
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

type pool struct {
	mtx  sync.RWMutex
	pool map[string]*entry
}

type entry struct {
	refs atomic.Int64

	s string
}

func newEntry(s string) *entry {
	return &entry{s: s}
}

func newPool() *pool {
	return &pool{
		pool: map[string]*entry{},
	}
}

func (p *pool) intern(s string) string {
	if s == "" {
		return ""
	}

	p.mtx.RLock()
	interned, ok := p.pool[s]
	if ok {
		// Increase the reference count while we're still holding the read lock,
		// This will prevent the release() from deleting the entry while we're increasing its ref count.
		interned.refs.Inc()
		p.mtx.RUnlock()
		return interned.s
	}
	p.mtx.RUnlock()

	p.mtx.Lock()
	defer p.mtx.Unlock()
	if interned, ok := p.pool[s]; ok {
		interned.refs.Inc()
		return interned.s
	}

	p.pool[s] = newEntry(s)
	p.pool[s].refs.Store(1)
	return s
}

func (p *pool) release(s string) {
	p.mtx.RLock()
	interned, ok := p.pool[s]
	p.mtx.RUnlock()

	if !ok {
		noReferenceReleases.Inc()
		return
	}

	refs := interned.refs.Dec()
	if refs > 0 {
		return
	}

	p.mtx.Lock()
	defer p.mtx.Unlock()
	if interned.refs.Load() != 0 {
		return
	}
	delete(p.pool, s)
}
