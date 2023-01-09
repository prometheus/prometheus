// Copyright 2017 The Prometheus Authors
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

package tsdb

import (
	"sync"
	"sync/atomic"

	"github.com/prometheus/prometheus/model/labels"
)

// ReferenceCountedStringPool manages a pool of reference counted strings and is safe for concurrent use.
type ReferenceCountedStringPool struct {
	mu   sync.RWMutex
	refs map[string]*stringRef
}

type stringRef struct {
	cnt uint64
	str string
}

// Intern increases the reference count for a pooled instance of s and returns it.
func (p *ReferenceCountedStringPool) Intern(s string) string {
	p.mu.RLock()
	ref, ok := p.refs[s]
	if ok {
		atomic.AddUint64(&ref.cnt, 1)
	}
	p.mu.RUnlock()
	if ok {
		return ref.str
	}
	return p.intern(s)
}

// InternBytes increases the reference count for a pooled instance of b's string representation and returns it.
func (p *ReferenceCountedStringPool) InternBytes(b []byte) string {
	p.mu.RLock()
	ref, ok := p.refs[string(b)] // compiler is smart enough to not allocate on map key lookup
	if ok {
		atomic.AddUint64(&ref.cnt, 1)
	}
	p.mu.RUnlock()
	if ok {
		return ref.str
	}
	return p.intern(string(b))
}

func (p *ReferenceCountedStringPool) intern(s string) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	if ref, ok := p.refs[s]; ok {
		atomic.AddUint64(&ref.cnt, 1)
		return ref.str
	}

	if p.refs == nil {
		p.refs = map[string]*stringRef{}
	}
	p.refs[s] = &stringRef{
		cnt: 1,
		str: s,
	}
	return s
}

// Release decreases the reference count for a pooled instance of s and removes it if the count reaches zero.
func (p *ReferenceCountedStringPool) Release(s string) {
	var (
		cnt uint64
	)

	p.mu.RLock()
	ref, ok := p.refs[s]
	if ok {
		cnt = atomic.AddUint64(&ref.cnt, ^uint64(0))
	}
	p.mu.RUnlock()

	if cnt == 0 && ok {
		p.mu.Lock()
		if atomic.LoadUint64(&ref.cnt) == 0 {
			delete(p.refs, s)
		}
		p.mu.Unlock()
	}
}

type interner struct {
	strings ReferenceCountedStringPool
}

func (in *interner) Intern(lset labels.Labels) error {
	for i, l := range lset {
		lset[i] = labels.Label{
			Name:  in.strings.Intern(l.Name),
			Value: in.strings.Intern(l.Value),
		}
	}
	return nil
}

func (in *interner) Release(lsets ...labels.Labels) {
	for _, lset := range lsets {
		for _, l := range lset {
			in.strings.Release(l.Name)
			in.strings.Release(l.Value)
		}
	}
}
