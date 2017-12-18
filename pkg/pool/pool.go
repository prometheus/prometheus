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
	"errors"
	"reflect"
	"sync"
)

type bucket struct {
	slices []sync.Pool
	// new is the function used to initialise an empty slice when none exist yet.
	new func(int) interface{}
	// Holds all available sizes for this bucket.
	sizes []int
}

// Pool is a bucketed pool for variably sized slices of any type.
type Pool struct {
	buckets map[interface{}]bucket
}

// New initializes a new pool.
func New() *Pool {
	p := &Pool{
		buckets: make(map[interface{}]bucket),
	}
	return p
}

// Add adds a new bucket with a given id to the Pool with size for minSize to maxSize increasing by the given factor.
func (p *Pool) Add(id interface{}, minSize, maxSize int, factor float64, newFunc func(int) interface{}) {
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
	p.buckets[id] = bucket{
		slices: make([]sync.Pool, len(sizes)),
		new:    newFunc,
		sizes:  sizes,
	}
}

// Get looks up a bucket by a given id and returns an initilized slice that fits the given size.
// The caller needs to use type assertion to use the returned slice.
func (p *Pool) Get(id interface{}, giveMe int) (interface{}, error) {
	if bucket, ok := p.buckets[id]; ok {
		for i, size := range bucket.sizes {
			if giveMe > size {
				continue
			}
			b := bucket.slices[i].Get()
			if b == nil {
				return bucket.new(size), nil
			}
			return b, nil
		}
		return bucket.new(giveMe), nil
	}
	return nil, errors.New("bucket id not found")
}

// Put adds a slice with a given id to the right bucket in the pool.
func (p *Pool) Put(id interface{}, s interface{}) error {
	if bucket, ok := p.buckets[id]; ok {
		slice := reflect.ValueOf(s)
		if slice.Kind() == reflect.Slice {
			for i, size := range bucket.sizes {
				if slice.Cap() > size {
					continue
				}
				bucket.slices[i].Put(slice.Slice(0, 0).Interface())
				return nil
			}
		}
	}
	return errors.New("bucket id not found")
}
