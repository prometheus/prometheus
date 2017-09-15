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

import "sync"

// BytesPool is a bucketed pool for variably sized byte slices.
type BytesPool struct {
	buckets []sync.Pool
	sizes   []int
}

// NewBytesPool returns a new BytesPool with size buckets for minSize to maxSize
// increasing by the given factor.
func NewBytesPool(minSize, maxSize int, factor float64) *BytesPool {
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

	p := &BytesPool{
		buckets: make([]sync.Pool, len(sizes)),
		sizes:   sizes,
	}

	return p
}

// Get returns a new byte slices that fits the given size.
func (p *BytesPool) Get(sz int) []byte {
	for i, bktSize := range p.sizes {
		if sz > bktSize {
			continue
		}
		b, ok := p.buckets[i].Get().([]byte)
		if !ok {
			b = make([]byte, 0, bktSize)
		}
		return b
	}
	return make([]byte, 0, sz)
}

// Put returns a byte slice to the right bucket in the pool.
func (p *BytesPool) Put(b []byte) {
	for i, bktSize := range p.sizes {
		if cap(b) > bktSize {
			continue
		}
		p.buckets[i].Put(b[:0])
		return
	}
}
