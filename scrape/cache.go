// Copyright 2016 The Prometheus Authors
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

package scrape

import (
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/storage"
)

// ScrapeCache tracks mappings of exposed metric strings to label sets and
// storage references. Additionally, it tracks staleness of series between
// scrapes.

type Cacher interface {
	GetMetadata(metric string) (MetricMetadata, bool)
	ListMetadata() []MetricMetadata
	SizeMetadata() (s int)
	LengthMetadata() int
}

// Implements the cache interface
type Cache struct {
	// Current scrape iteration.
	iter uint64

	// How many series and metadata entries there were at the last success.
	successfulCount int

	// Parsed string to an entry with information about the actual label set
	// and its storage reference.
	series map[string]*cacheEntry

	// Cache of dropped metric strings and their iteration. The iteration must
	// be a pointer so we can update it without setting a new entry with an unsafe
	// string in addDropped().
	droppedSeries map[string]*uint64

	// seriesCur and seriesPrev store the labels of series that were seen
	// in the current and previous scrape.
	// We hold two maps and swap them out to save allocations.
	seriesCur  map[uint64]labels.Labels
	seriesPrev map[uint64]labels.Labels

	metadata map[string]*metaEntry
	metaMtx  sync.Mutex
}

type cacheEntry struct {
	ref      storage.SeriesRef
	lastIter uint64
	hash     uint64
	lset     labels.Labels
}

// metaEntry holds meta information about a metric.
type metaEntry struct {
	metadata.Metadata

	// Last scrape iteration the entry was observed at.
	lastIter uint64

	// Last scrape iteration the entry was changed at.
	lastIterChange uint64
}

func (m *metaEntry) size() int {
	// The attribute lastIter although part of the struct it is not metadata.
	return len(m.Help) + len(m.Unit) + len(m.Type)
}

func NewScrapeCache() *Cache {
	return &Cache{
		series:        map[string]*cacheEntry{},
		droppedSeries: map[string]*uint64{},
		seriesCur:     map[uint64]labels.Labels{},
		seriesPrev:    map[uint64]labels.Labels{},
		metadata:      map[string]*metaEntry{},
	}
}

func (c *Cache) iterDone(flushCache bool) {
	c.metaMtx.Lock()
	count := len(c.series) + len(c.droppedSeries) + len(c.metadata)
	c.metaMtx.Unlock()

	if flushCache {
		c.successfulCount = count
	} else if count > c.successfulCount*2+1000 {
		// If a target had varying labels in scrapes that ultimately failed,
		// the caches would grow indefinitely. Force a flush when this happens.
		// We use the heuristic that this is a doubling of the cache size
		// since the last scrape, and allow an additional 1000 in case
		// initial scrapes all fail.
		flushCache = true
		targetScrapeCacheFlushForced.Inc()
	}

	if flushCache {
		// All caches may grow over time through series churn
		// or multiple string representations of the same metric. Clean up entries
		// that haven't appeared in the last scrape.
		for s, e := range c.series {
			if c.iter != e.lastIter {
				delete(c.series, s)
			}
		}
		for s, iter := range c.droppedSeries {
			if c.iter != *iter {
				delete(c.droppedSeries, s)
			}
		}
		c.metaMtx.Lock()
		for m, e := range c.metadata {
			// Keep metadata around for 10 scrapes after its metric disappeared.
			if c.iter-e.lastIter > 10 {
				delete(c.metadata, m)
			}
		}
		c.metaMtx.Unlock()

		c.iter++
	}

	// Swap current and previous series.
	c.seriesPrev, c.seriesCur = c.seriesCur, c.seriesPrev

	// We have to delete every single key in the map.
	for k := range c.seriesCur {
		delete(c.seriesCur, k)
	}
}

func (c *Cache) get(met string) (*cacheEntry, bool) {
	e, ok := c.series[met]
	if !ok {
		return nil, false
	}
	e.lastIter = c.iter
	return e, true
}

func (c *Cache) addRef(met string, ref storage.SeriesRef, lset labels.Labels, hash uint64) {
	if ref == 0 {
		return
	}
	c.series[met] = &cacheEntry{ref: ref, lastIter: c.iter, lset: lset, hash: hash}
}

func (c *Cache) addDropped(met string) {
	iter := c.iter
	c.droppedSeries[met] = &iter
}

func (c *Cache) getDropped(met string) bool {
	iterp, ok := c.droppedSeries[met]
	if ok {
		*iterp = c.iter
	}
	return ok
}

func (c *Cache) trackStaleness(hash uint64, lset labels.Labels) {
	c.seriesCur[hash] = lset
}

func (c *Cache) forEachStale(f func(labels.Labels) bool) {
	for h, lset := range c.seriesPrev {
		if _, ok := c.seriesCur[h]; !ok {
			if !f(lset) {
				break
			}
		}
	}
}

func (c *Cache) setType(metric []byte, t textparse.MetricType) {
	c.metaMtx.Lock()

	e, ok := c.metadata[YoloString(metric)]
	if !ok {
		e = &metaEntry{Metadata: metadata.Metadata{Type: textparse.MetricTypeUnknown}}
		c.metadata[string(metric)] = e
	}
	if e.Type != t {
		e.Type = t
		e.lastIterChange = c.iter
	}
	e.lastIter = c.iter

	c.metaMtx.Unlock()
}

func (c *Cache) setHelp(metric, help []byte) {
	c.metaMtx.Lock()

	e, ok := c.metadata[YoloString(metric)]
	if !ok {
		e = &metaEntry{Metadata: metadata.Metadata{Type: textparse.MetricTypeUnknown}}
		c.metadata[string(metric)] = e
	}
	if e.Help != YoloString(help) {
		e.Help = string(help)
		e.lastIterChange = c.iter
	}
	e.lastIter = c.iter

	c.metaMtx.Unlock()
}

func (c *Cache) setUnit(metric, unit []byte) {
	c.metaMtx.Lock()

	e, ok := c.metadata[YoloString(metric)]
	if !ok {
		e = &metaEntry{Metadata: metadata.Metadata{Type: textparse.MetricTypeUnknown}}
		c.metadata[string(metric)] = e
	}
	if e.Unit != YoloString(unit) {
		e.Unit = string(unit)
		e.lastIterChange = c.iter
	}
	e.lastIter = c.iter

	c.metaMtx.Unlock()
}

func (c *Cache) GetMetadata(metric string) (MetricMetadata, bool) {
	c.metaMtx.Lock()
	defer c.metaMtx.Unlock()

	m, ok := c.metadata[metric]
	if !ok {
		return MetricMetadata{}, false
	}
	return MetricMetadata{
		Metric: metric,
		Type:   m.Type,
		Help:   m.Help,
		Unit:   m.Unit,
	}, true
}

func (c *Cache) ListMetadata() []MetricMetadata {
	c.metaMtx.Lock()
	defer c.metaMtx.Unlock()

	res := make([]MetricMetadata, 0, len(c.metadata))

	for m, e := range c.metadata {
		res = append(res, MetricMetadata{
			Metric: m,
			Type:   e.Type,
			Help:   e.Help,
			Unit:   e.Unit,
		})
	}
	return res
}

// MetadataSize returns the size of the metadata cache.
func (c *Cache) SizeMetadata() (s int) {
	c.metaMtx.Lock()
	defer c.metaMtx.Unlock()
	for _, e := range c.metadata {
		s += e.size()
	}

	return s
}

// MetadataLen returns the number of metadata entries in the cache.
func (c *Cache) LengthMetadata() int {
	c.metaMtx.Lock()
	defer c.metaMtx.Unlock()

	return len(c.metadata)
}
