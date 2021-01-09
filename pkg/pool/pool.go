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
	"fmt"
	"reflect"
	"sort"
	"sync"
)

// Pool is a bucketed pool for variably sized byte slices.
type Pool struct {
	buckets []sync.Pool
	sizes   []int
	// make is the function used to create an empty slice when none exist yet.
	make func(int) interface{}
}

// New returns a new Pool with size buckets for minSize to maxSize
// increasing by the given factor.
func New(minSize, maxSize int, factor float64, makeFunc func(int) interface{}) *Pool {
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
		make:    makeFunc,
	}

	return p
}

// Get returns a new byte slices that fits the given size.
func (p *Pool) Get(sz int) interface{} {
	if idx, ok := p.index(sz); ok {
		b := p.buckets[idx].Get()
		if b == nil {
			b = p.make(p.sizes[idx])
		}
		return b
	}
	return p.make(sz)
}

// Put adds a slice to the right bucket in the pool.
func (p *Pool) Put(s interface{}) {
	slice := reflect.ValueOf(s)

	if slice.Kind() != reflect.Slice {
		panic(fmt.Sprintf("%+v is not a slice", slice))
	}
	if idx, ok := p.index(slice.Cap()); ok {
		p.buckets[idx].Put(slice.Slice(0, 0).Interface())
	}
}

func (p *Pool) index(size int) (int, bool) {
	index := sort.SearchInts(p.sizes, size)
	return index, index != len(p.sizes)
}
