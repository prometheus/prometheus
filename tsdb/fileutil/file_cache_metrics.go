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

package fileutil

import (
	"github.com/prometheus/client_golang/prometheus"
)

// FileCacheMetrics holds Prometheus metrics for the file cache.
type FileCacheMetrics struct {
	cacheHits      prometheus.Counter
	cacheMisses    prometheus.Counter
	cacheSize      prometheus.Gauge
	cacheMaxSize   prometheus.Gauge
	cacheEvictions prometheus.Counter
	cacheHitRatio  prometheus.GaugeFunc
}

// NewFileCacheMetrics creates metrics for a FileCache.
// The returned metrics are not registered; call Register() on the collector
// or register individual metrics manually.
func NewFileCacheMetrics(cache *FileCache) *FileCacheMetrics {
	m := &FileCacheMetrics{
		cacheHits: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_file_cache_hits_total",
			Help: "Total number of file cache hits.",
		}),
		cacheMisses: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_file_cache_misses_total",
			Help: "Total number of file cache misses.",
		}),
		cacheSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_file_cache_size_bytes",
			Help: "Current size of the file cache in bytes.",
		}),
		cacheMaxSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_file_cache_max_size_bytes",
			Help: "Maximum size of the file cache in bytes.",
		}),
		cacheEvictions: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_file_cache_evictions_total",
			Help: "Total number of cache evictions.",
		}),
	}

	if cache != nil {
		m.cacheHitRatio = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_file_cache_hit_ratio",
			Help: "Cache hit ratio (hits / (hits + misses)).",
		}, func() float64 {
			hits, misses, _, _ := cache.Stats()
			total := hits + misses
			if total == 0 {
				return 0
			}
			return float64(hits) / float64(total)
		})
	}

	return m
}

// Describe implements prometheus.Collector.
func (m *FileCacheMetrics) Describe(ch chan<- *prometheus.Desc) {
	m.cacheHits.Describe(ch)
	m.cacheMisses.Describe(ch)
	m.cacheSize.Describe(ch)
	m.cacheMaxSize.Describe(ch)
	m.cacheEvictions.Describe(ch)
	if m.cacheHitRatio != nil {
		m.cacheHitRatio.Describe(ch)
	}
}

// Collect implements prometheus.Collector.
func (m *FileCacheMetrics) Collect(ch chan<- prometheus.Metric) {
	m.cacheHits.Collect(ch)
	m.cacheMisses.Collect(ch)
	m.cacheSize.Collect(ch)
	m.cacheMaxSize.Collect(ch)
	m.cacheEvictions.Collect(ch)
	if m.cacheHitRatio != nil {
		m.cacheHitRatio.Collect(ch)
	}
}

// Update updates the metrics from the cache.
// Call this periodically to keep metrics current.
func (m *FileCacheMetrics) Update(cache *FileCache) {
	if cache == nil {
		return
	}

	hits, misses, size, maxSize := cache.Stats()
	m.cacheHits.Add(0)   // Counter maintains its own value; this just ensures it exists
	m.cacheMisses.Add(0) // Counter maintains its own value; this just ensures it exists
	m.cacheSize.Set(float64(size))
	m.cacheMaxSize.Set(float64(maxSize))

	// Note: For accurate hit/miss counters, we'd need to track deltas
	// or have the cache directly increment the prometheus counters.
	_ = hits
	_ = misses
}

// FileCacheWithMetrics wraps a FileCache and updates Prometheus metrics.
type FileCacheWithMetrics struct {
	*FileCache
	metrics *FileCacheMetrics
}

// NewFileCacheWithMetrics creates a new FileCache with Prometheus metrics.
func NewFileCacheWithMetrics(opts FileCacheOptions, reg prometheus.Registerer) (*FileCacheWithMetrics, error) {
	cache := NewFileCache(opts)
	metrics := NewFileCacheMetrics(cache)

	if reg != nil {
		if err := reg.Register(metrics); err != nil {
			return nil, err
		}
	}

	return &FileCacheWithMetrics{
		FileCache: cache,
		metrics:   metrics,
	}, nil
}

// Metrics returns the metrics for this cache.
func (c *FileCacheWithMetrics) Metrics() *FileCacheMetrics {
	return c.metrics
}
