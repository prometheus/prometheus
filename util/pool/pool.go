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

package pool

import (
	"sync"
)

// Pool is a bucketed pool for variably sized byte slices.
type Pool struct {
	buckets []sync.Pool
	sizes   []int
	// pointers holds just pointers to the slice header objects which the main pool holds.
	pointers sync.Pool
	// Simple slice holding blocks bigger than all buckets; we don't want the sync.Pool
	// behaviour of maintaining a pool per CPU core for such big blocks of memory.
	outsizedMtx sync.Mutex
	outsized    [][]byte
}

// New returns a new Pool with size buckets for minSize to maxSize
// increasing by the given factor.
func New(minSize, maxSize int, factor float64) *Pool {
	if minSize < 1 {
		panic("invalid minimum pool size")
	}
	if maxSize < 1 {
		panic("invalid maximum pool size")
	}
	if factor < 1 {
		panic("invalid factor")
	}

	var sizes []int

	for s := minSize; s <= maxSize; s = int(float64(s) * factor) {
		sizes = append(sizes, s)
	}

	p := &Pool{
		buckets: make([]sync.Pool, len(sizes)),
		sizes:   sizes,
	}

	return p
}

// Get returns a new byte slices that fits the given size.
func (p *Pool) Get(sz int) []byte {
	for i, bktSize := range p.sizes {
		if sz > bktSize {
			continue
		}
		b := p.buckets[i].Get()
		if b == nil {
			return make([]byte, 0, bktSize)
		}
		ptr := b.(*[]byte)
		item := *ptr
		*ptr = []byte{} // Zero out before putting back in the pool.
		p.pointers.Put(ptr)
		return item
	}
	// Size is bigger than all buckets; check the outsized pool.
	p.outsizedMtx.Lock()
	defer p.outsizedMtx.Unlock()
	for i, b := range p.outsized {
		if cap(b) >= sz {
			p.outsized = append(p.outsized[:i], p.outsized[i+1:]...) // Delete from slice.
			return b
		}
	}
	return make([]byte, 0, sz)
}

// Put adds a slice to the right bucket in the pool.
func (p *Pool) Put(s []byte) {
	for i, size := range p.sizes {
		if cap(s) > size {
			continue
		}
		// Save the address in a pooled slice-header object, so the Put doesn't allocate memory.
		var ptr *[]byte
		if pooled := p.pointers.Get(); pooled != nil {
			ptr = pooled.(*[]byte)
		} else {
			ptr = new([]byte)
		}
		*ptr = s
		p.buckets[i].Put(ptr)
		return
	}
	// Size is bigger than all buckets; put it in the outsized pool.
	p.outsizedMtx.Lock()
	defer p.outsizedMtx.Unlock()
	p.outsized = append(p.outsized, s)
	// TODO: shrink outsized pool at some point.
}
