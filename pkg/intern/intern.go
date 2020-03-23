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

package intern

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pkg/labels"
	"go.uber.org/atomic"
)

// Shared interner
var (
	Global Interner = New(prometheus.DefaultRegisterer)
)

// Iterner is a string interner.
type Interner interface {
	// Metrics returns Metrics for the interner.
	Metrics() *Metrics

	// Intern will intern an input string, returning the interned string as
	// a result.
	Intern(string) string

	// Release removes an interned string from interner.
	Release(string)
}

func New(r prometheus.Registerer) Interner {
	return &pool{
		m:    NewMetrics(r),
		pool: map[string]*entry{},
	}
}

type Metrics struct {
	NoReferenceReleases prometheus.Counter
}

func NewMetrics(r prometheus.Registerer) *Metrics {
	var m Metrics
	m.NoReferenceReleases = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "prometheus",
		Subsystem: "interner",
		Name:      "string_interner_zero_reference_releases_total",
		Help:      "The number of times release has been called for strings that are not interned.",
	})

	if r != nil {
		r.MustRegister(m.NoReferenceReleases)
	}

	return &m
}

type pool struct {
	m *Metrics

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

func (p *pool) Metrics() *Metrics { return p.m }

func (p *pool) Intern(s string) string {
	if s == "" {
		return ""
	}

	p.mtx.RLock()
	interned, ok := p.pool[s]
	p.mtx.RUnlock()
	if ok {
		interned.refs.Inc()
		return interned.s
	}
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

func (p *pool) Release(s string) {
	p.mtx.RLock()
	interned, ok := p.pool[s]
	p.mtx.RUnlock()

	if !ok {
		p.m.NoReferenceReleases.Inc()
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

// InternLabels is a helper function for interning all label
// names and values to a given interner.
func InternLabels(interner Interner, lbls labels.Labels) {
	for i, l := range lbls {
		lbls[i].Name = interner.Intern(l.Name)
		lbls[i].Value = interner.Intern(l.Value)
	}
}

// ReleaseLabels is a helper function for releasing all label
// names and values from a given interner.
func ReleaseLabels(interner Interner, ls labels.Labels) {
	for _, l := range ls {
		interner.Release(l.Name)
		interner.Release(l.Value)
	}
}
