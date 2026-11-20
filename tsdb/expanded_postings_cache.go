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

package tsdb

import (
	"container/list"
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/oklog/ulid/v2"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/index"
)

type expandedPostingsCacheKey struct {
	blockID  ulid.ULID
	matchers string
}

type expandedPostingsCacheEntry struct {
	key  expandedPostingsCacheKey
	refs []storage.SeriesRef
	size int64
}

// expandedPostingsCache is a byte-bounded LRU for immutable block postings.
// Cached reference slices are immutable after insertion and can therefore be
// safely shared by independent list-postings iterators.
type expandedPostingsCache struct {
	mtx      sync.Mutex
	maxBytes int64
	bytes    int64
	lru      *list.List
	entries  map[expandedPostingsCacheKey]*list.Element
}

func newExpandedPostingsCache(maxBytes int64) *expandedPostingsCache {
	if maxBytes <= 0 {
		return nil
	}
	return &expandedPostingsCache{
		maxBytes: maxBytes,
		lru:      list.New(),
		entries:  make(map[expandedPostingsCacheKey]*list.Element),
	}
}

func (c *expandedPostingsCache) postingsForMatchers(
	ctx context.Context,
	blockID ulid.ULID,
	ix IndexReader,
	ms ...*labels.Matcher,
) (index.Postings, error) {
	key := expandedPostingsCacheKey{
		blockID:  blockID,
		matchers: expandedPostingsMatchersKey(ms),
	}
	if refs, ok := c.get(key); ok {
		return index.NewListPostings(refs), nil
	}

	p, err := PostingsForMatchers(ctx, ix, ms...)
	if err != nil {
		return nil, err
	}
	refs, err := index.ExpandPostings(p)
	if err != nil {
		return nil, err
	}
	c.add(key, refs)
	return index.NewListPostings(refs), nil
}

func expandedPostingsMatchersKey(ms []*labels.Matcher) string {
	matchers := make([]string, len(ms))
	for i, m := range ms {
		matchers[i] = m.String()
	}
	sort.Strings(matchers)

	var b strings.Builder
	for _, m := range matchers {
		b.WriteString(strconv.Itoa(len(m)))
		b.WriteByte(':')
		b.WriteString(m)
		b.WriteByte(';')
	}
	return b.String()
}

func (c *expandedPostingsCache) get(key expandedPostingsCacheKey) ([]storage.SeriesRef, bool) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	elem, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.lru.MoveToFront(elem)
	return elem.Value.(*expandedPostingsCacheEntry).refs, true
}

func expandedPostingsCacheEntrySize(key expandedPostingsCacheKey, refs []storage.SeriesRef) int64 {
	// Account for the entry, its LRU node, the map's key copy, the matcher
	// bytes, and the full backing array retained by the cached slice.
	return int64(unsafe.Sizeof(expandedPostingsCacheEntry{})) +
		int64(unsafe.Sizeof(list.Element{})) +
		int64(unsafe.Sizeof(key)) +
		int64(len(key.matchers)) +
		int64(cap(refs))*int64(unsafe.Sizeof(storage.SeriesRef(0)))
}

func (c *expandedPostingsCache) add(key expandedPostingsCacheKey, refs []storage.SeriesRef) {
	size := expandedPostingsCacheEntrySize(key, refs)
	if size > c.maxBytes {
		return
	}

	c.mtx.Lock()
	defer c.mtx.Unlock()

	if elem, ok := c.entries[key]; ok {
		entry := elem.Value.(*expandedPostingsCacheEntry)
		c.bytes -= entry.size
		entry.refs = refs
		entry.size = size
		c.bytes += size
		c.lru.MoveToFront(elem)
	} else {
		entry := &expandedPostingsCacheEntry{key: key, refs: refs, size: size}
		c.entries[key] = c.lru.PushFront(entry)
		c.bytes += size
	}

	for c.bytes > c.maxBytes {
		c.removeElement(c.lru.Back())
	}
}

func (c *expandedPostingsCache) removeBlocks(keep map[ulid.ULID]struct{}) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	for elem := c.lru.Back(); elem != nil; {
		prev := elem.Prev()
		entry := elem.Value.(*expandedPostingsCacheEntry)
		if _, ok := keep[entry.key.blockID]; !ok {
			c.removeElement(elem)
		}
		elem = prev
	}
}

func (c *expandedPostingsCache) removeElement(elem *list.Element) {
	if elem == nil {
		return
	}
	entry := elem.Value.(*expandedPostingsCacheEntry)
	delete(c.entries, entry.key)
	c.bytes -= entry.size
	c.lru.Remove(elem)
}
