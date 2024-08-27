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
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"go.uber.org/atomic"
)

var noReferenceReleases = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: namespace,
	Subsystem: subsystem,
	Name:      "string_interner_zero_reference_releases_total",
	Help:      "The number of times release has been called for strings that are not interned.",
})

type pool struct {
	mtx          sync.RWMutex
	pool         map[chunks.HeadSeriesRef]*entry
	shouldIntern bool
}

type entry struct {
	refs atomic.Int64
	lset labels.Labels
}

func newEntry(lset labels.Labels) *entry {
	return &entry{lset: lset}
}

func newPool(shouldIntern bool) *pool {
	return &pool{
		pool:         map[chunks.HeadSeriesRef]*entry{},
		shouldIntern: shouldIntern,
	}
}

func (p *pool) release(ref chunks.HeadSeriesRef) {
	if !p.shouldIntern {
		return
	}
	p.mtx.RLock()
	interned, ok := p.pool[ref]
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
	delete(p.pool, ref)
}

func (p *pool) intern(ref chunks.HeadSeriesRef, lset labels.Labels) labels.Labels {
	if !p.shouldIntern {
		return lset
	}

	p.mtx.RLock()
	interned, ok := p.pool[ref]
	p.mtx.RUnlock()
	if ok {
		interned.refs.Inc()
		return interned.lset
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if interned, ok := p.pool[ref]; ok {
		interned.refs.Inc()
		return interned.lset
	}

	if len(lset) == 0 {
		return nil
	}

	p.pool[ref] = newEntry(lset)
	p.pool[ref].refs.Store(1)
	return p.pool[ref].lset
}
