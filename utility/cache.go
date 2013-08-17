// Copyright 2013 Prometheus Team
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

package utility

import (
	"container/list"
	"sync"
)

type Cache interface {
	Put(k, v interface{}) (replaced bool, err error)
	PutIfAbsent(k, v interface{}) (put bool, err error)
	Get(k interface{}) (v interface{}, ok bool, err error)
	Has(k interface{}) (ok bool, err error)
	Delete(k interface{}) (deleted bool, err error)
	Clear() (cleared bool, err error)
}

type LRUCache struct {
	list  *list.List
	table map[interface{}]*list.Element

	limit uint
	size  uint
}

func NewLRUCache(limit uint) *LRUCache {
	return &LRUCache{
		list:  list.New(),
		table: map[interface{}]*list.Element{},

		limit: limit,
	}
}

func (c *LRUCache) Has(k interface{}) (has bool, err error) {
	_, ok := c.table[k]
	return ok, nil
}

func (c *LRUCache) Get(k interface{}) (v interface{}, ok bool, err error) {
	element, ok := c.table[k]
	if !ok {
		return nil, false, nil
	}

	c.moveToFront(element)

	return element.Value, true, nil
}

func (c *LRUCache) Put(k, v interface{}) (replaced bool, err error) {
	element, ok := c.table[k]
	if ok {
		c.updateInplace(element, v)
		return true, nil
	}

	c.addNew(k, v)
	return false, nil
}

func (c *LRUCache) PutIfAbsent(k, v interface{}) (put bool, err error) {
	if _, ok := c.table[k]; ok {
		return false, nil
	}

	c.addNew(k, v)
	return true, nil
}

func (c *LRUCache) Delete(k interface{}) (deleted bool, err error) {
	element, ok := c.table[k]
	if !ok {
		return false, nil
	}

	c.list.Remove(element)
	delete(c.table, k)

	return true, nil
}

func (c *LRUCache) Clear() (cleared bool, err error) {
	c.list.Init()
	c.table = map[interface{}]*list.Element{}
	c.size = 0

	return true, nil
}

func (c *LRUCache) updateInplace(e *list.Element, v interface{}) {
	e.Value = v
	c.moveToFront(e)
	c.checkCapacity()
}

func (c *LRUCache) moveToFront(e *list.Element) {
	c.list.MoveToFront(e)
}

func (c *LRUCache) addNew(k, v interface{}) {
	c.table[k] = c.list.PushFront(v)
	c.checkCapacity()
}

func (c *LRUCache) checkCapacity() {
	for c.size > c.limit {
		delElem := c.list.Back()
		v := delElem.Value
		c.list.Remove(delElem)
		delete(c.table, v)
	}
}

type SynchronizedCache struct {
	mu sync.Mutex
	c  Cache
}

func (c *SynchronizedCache) Put(k, v interface{}) (replaced bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.c.Put(k, v)
}
func (c *SynchronizedCache) PutIfAbsent(k, v interface{}) (put bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.PutIfAbsent(k, v)
}

func (c *SynchronizedCache) Get(k interface{}) (v interface{}, ok bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.c.Get(k)
}
func (c *SynchronizedCache) Has(k interface{}) (ok bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.c.Has(k)
}

func (c *SynchronizedCache) Delete(k interface{}) (deleted bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.c.Delete(k)
}

func (c *SynchronizedCache) Clear() (cleared bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.c.Clear()
}

func NewSynchronizedCache(c Cache) *SynchronizedCache {
	return &SynchronizedCache{
		c: c,
	}
}
