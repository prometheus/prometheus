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

package scrape

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type scrapeMetrics struct {
	reg prometheus.Registerer
	// Used by Manager.
	targetMetadataCache     *MetadataMetricsCollector
	targetScrapePools       prometheus.Counter
	targetScrapePoolsFailed prometheus.Counter

	// Used by scrapePool.
	targetReloadIntervalLength          *prometheus.SummaryVec
	targetScrapePoolReloads             prometheus.Counter
	targetScrapePoolReloadsFailed       prometheus.Counter
	targetScrapePoolSyncsCounter        *prometheus.CounterVec
	targetScrapePoolExceededTargetLimit prometheus.Counter
	targetScrapePoolTargetLimit         *prometheus.GaugeVec
	targetScrapePoolTargetsAdded        *prometheus.GaugeVec
	targetScrapePoolSymbolTableItems    *prometheus.GaugeVec
	targetSyncIntervalLength            *prometheus.SummaryVec
	targetSyncIntervalLengthHistogram   *prometheus.HistogramVec
	targetSyncFailed                    *prometheus.CounterVec

	// Used by targetScraper.
	targetScrapeExceededBodySizeLimit prometheus.Counter

	// Used by scrapeCache.
	targetScrapeCacheFlushForced prometheus.Counter

	// Used by scrapeLoop.
	targetIntervalLength                   *prometheus.SummaryVec
	targetIntervalLengthHistogram          *prometheus.HistogramVec
	targetScrapeSampleLimit                prometheus.Counter
	targetScrapeSampleDuplicate            prometheus.Counter
	targetScrapeSampleOutOfOrder           prometheus.Counter
	targetScrapeSampleOutOfBounds          prometheus.Counter
	targetScrapeExemplarOutOfOrder         prometheus.Counter
	targetScrapePoolExceededLabelLimits    prometheus.Counter
	targetScrapeNativeHistogramBucketLimit prometheus.Counter
	targetScrapeDuration                   prometheus.Histogram
}

func newScrapeMetrics(reg prometheus.Registerer) (*scrapeMetrics, error) {
	sm := &scrapeMetrics{reg: reg}

	// Manager metrics.
	sm.targetMetadataCache = &MetadataMetricsCollector{
		CacheEntries: prometheus.NewDesc(
			"prometheus_target_metadata_cache_entries",
			"Total number of metric metadata entries in the cache",
			[]string{"scrape_job"},
			nil,
		),
		CacheBytes: prometheus.NewDesc(
			"prometheus_target_metadata_cache_bytes",
			"The number of bytes that are currently used for storing metric metadata in the cache",
			[]string{"scrape_job"},
			nil,
		),
		// TargetsGatherer should be set later, because it's a circular dependency.
		// newScrapeMetrics() is called by NewManager(), while also TargetsGatherer is the new Manager.
	}

	sm.targetScrapePools = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pools_total",
			Help: "Total number of scrape pool creation attempts.",
		},
	)
	sm.targetScrapePoolsFailed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pools_failed_total",
			Help: "Total number of scrape pool creations that failed.",
		},
	)

	// Used by scrapePool.
	sm.targetReloadIntervalLength = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_target_reload_length_seconds",
			Help:       "Actual interval to reload the scrape pool with a given configuration.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{"interval"},
	)
	sm.targetScrapePoolReloads = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_reloads_total",
			Help: "Total number of scrape pool reloads.",
		},
	)
	sm.targetScrapePoolReloadsFailed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_reloads_failed_total",
			Help: "Total number of failed scrape pool reloads.",
		},
	)
	sm.targetScrapePoolExceededTargetLimit = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_exceeded_target_limit_total",
			Help: "Total number of times scrape pools hit the target limit, during sync or config reload.",
		},
	)
	sm.targetScrapePoolTargetLimit = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "prometheus_target_scrape_pool_target_limit",
			Help: "Maximum number of targets allowed in this scrape pool.",
		},
		[]string{"scrape_job"},
	)
	sm.targetScrapePoolTargetsAdded = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "prometheus_target_scrape_pool_targets",
			Help: "Current number of targets in this scrape pool.",
		},
		[]string{"scrape_job"},
	)
	sm.targetScrapePoolSymbolTableItems = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "prometheus_target_scrape_pool_symboltable_items",
			Help: "Current number of symbols in table for this scrape pool.",
		},
		[]string{"scrape_job"},
	)
	sm.targetScrapePoolSyncsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_sync_total",
			Help: "Total number of syncs that were executed on a scrape pool.",
		},
		[]string{"scrape_job"},
	)
	sm.targetSyncIntervalLength = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_target_sync_length_seconds",
			Help:       "Actual interval to sync the scrape pool.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{"scrape_job"},
	)
	sm.targetSyncIntervalLengthHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:                            "prometheus_target_sync_length_histogram_seconds",
			Help:                            "Actual interval to sync the scrape pool.",
			Buckets:                         []float64{.01, .1, 1, 10},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
		[]string{"scrape_job"},
	)
	sm.targetSyncFailed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_target_sync_failed_total",
			Help: "Total number of target sync failures.",
		},
		[]string{"scrape_job"},
	)

	// Used by targetScraper.
	sm.targetScrapeExceededBodySizeLimit = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exceeded_body_size_limit_total",
			Help: "Total number of scrapes that hit the body size limit",
		},
	)

	// Used by scrapeCache.
	sm.targetScrapeCacheFlushForced = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_cache_flush_forced_total",
			Help: "How many times a scrape cache was flushed due to getting big while scrapes are failing.",
		},
	)

	// Used by scrapeLoop.
	sm.targetIntervalLength = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_target_interval_length_seconds",
			Help:       "Actual intervals between scrapes.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{"interval"},
	)
	sm.targetIntervalLengthHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:                            "prometheus_target_interval_length_histogram_seconds",
			Help:                            "Actual intervals between scrapes.",
			Buckets:                         []float64{.01, .1, 1, 10},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
		[]string{"interval"},
	)
	sm.targetScrapeSampleLimit = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exceeded_sample_limit_total",
			Help: "Total number of scrapes that hit the sample limit and were rejected.",
		},
	)
	sm.targetScrapeSampleDuplicate = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_duplicate_timestamp_total",
			Help: "Total number of samples rejected due to duplicate timestamps but different values.",
		},
	)
	sm.targetScrapeSampleOutOfOrder = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_out_of_order_total",
			Help: "Total number of samples rejected due to not being out of the expected order.",
		},
	)
	sm.targetScrapeSampleOutOfBounds = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_out_of_bounds_total",
			Help: "Total number of samples rejected due to timestamp falling outside of the time bounds.",
		},
	)
	sm.targetScrapePoolExceededLabelLimits = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_exceeded_label_limits_total",
			Help: "Total number of times scrape pools hit the label limits, during sync or config reload.",
		},
	)
	sm.targetScrapeNativeHistogramBucketLimit = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exceeded_native_histogram_bucket_limit_total",
			Help: "Total number of scrapes that hit the native histogram bucket limit and were rejected.",
		},
	)
	sm.targetScrapeExemplarOutOfOrder = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exemplar_out_of_order_total",
			Help: "Total number of exemplar rejected due to not being out of the expected order.",
		},
	)
	sm.targetScrapeDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:                            "prometheus_target_scrape_duration_seconds",
			Help:                            "Total duration of the scrape from start to commit completion in seconds.",
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
	)

	for _, collector := range []prometheus.Collector{
		// Used by Manager.
		sm.targetMetadataCache,
		sm.targetScrapePools,
		sm.targetScrapePoolsFailed,
		// Used by scrapePool.
		sm.targetReloadIntervalLength,
		sm.targetScrapePoolReloads,
		sm.targetScrapePoolReloadsFailed,
		sm.targetSyncIntervalLength,
		sm.targetSyncIntervalLengthHistogram,
		sm.targetScrapePoolSyncsCounter,
		sm.targetScrapePoolExceededTargetLimit,
		sm.targetScrapePoolTargetLimit,
		sm.targetScrapePoolTargetsAdded,
		sm.targetScrapePoolSymbolTableItems,
		sm.targetSyncFailed,
		// Used by targetScraper.
		sm.targetScrapeExceededBodySizeLimit,
		// Used by scrapeCache.
		sm.targetScrapeCacheFlushForced,
		// Used by scrapeLoop.
		sm.targetIntervalLength,
		sm.targetIntervalLengthHistogram,
		sm.targetScrapeSampleLimit,
		sm.targetScrapeSampleDuplicate,
		sm.targetScrapeSampleOutOfOrder,
		sm.targetScrapeSampleOutOfBounds,
		sm.targetScrapeExemplarOutOfOrder,
		sm.targetScrapePoolExceededLabelLimits,
		sm.targetScrapeNativeHistogramBucketLimit,
		sm.targetScrapeDuration,
	} {
		err := reg.Register(collector)
		if err != nil {
			return nil, fmt.Errorf("failed to register scrape metrics: %w", err)
		}
	}
	return sm, nil
}

func (sm *scrapeMetrics) setTargetMetadataCacheGatherer(gatherer TargetsGatherer) {
	sm.targetMetadataCache.TargetsGatherer = gatherer
}

// Unregister unregisters all metrics.
func (sm *scrapeMetrics) Unregister() {
	sm.reg.Unregister(sm.targetMetadataCache)
	sm.reg.Unregister(sm.targetScrapePools)
	sm.reg.Unregister(sm.targetScrapePoolsFailed)
	sm.reg.Unregister(sm.targetReloadIntervalLength)
	sm.reg.Unregister(sm.targetScrapePoolReloads)
	sm.reg.Unregister(sm.targetScrapePoolReloadsFailed)
	sm.reg.Unregister(sm.targetSyncIntervalLength)
	sm.reg.Unregister(sm.targetSyncIntervalLengthHistogram)
	sm.reg.Unregister(sm.targetScrapePoolSyncsCounter)
	sm.reg.Unregister(sm.targetScrapePoolExceededTargetLimit)
	sm.reg.Unregister(sm.targetScrapePoolTargetLimit)
	sm.reg.Unregister(sm.targetScrapePoolTargetsAdded)
	sm.reg.Unregister(sm.targetScrapePoolSymbolTableItems)
	sm.reg.Unregister(sm.targetSyncFailed)
	sm.reg.Unregister(sm.targetScrapeExceededBodySizeLimit)
	sm.reg.Unregister(sm.targetScrapeCacheFlushForced)
	sm.reg.Unregister(sm.targetIntervalLength)
	sm.reg.Unregister(sm.targetIntervalLengthHistogram)
	sm.reg.Unregister(sm.targetScrapeSampleLimit)
	sm.reg.Unregister(sm.targetScrapeSampleDuplicate)
	sm.reg.Unregister(sm.targetScrapeSampleOutOfOrder)
	sm.reg.Unregister(sm.targetScrapeSampleOutOfBounds)
	sm.reg.Unregister(sm.targetScrapeExemplarOutOfOrder)
	sm.reg.Unregister(sm.targetScrapePoolExceededLabelLimits)
	sm.reg.Unregister(sm.targetScrapeNativeHistogramBucketLimit)
	sm.reg.Unregister(sm.targetScrapeDuration)
}

type TargetsGatherer interface {
	TargetsActive() map[string][]*Target
}

// MetadataMetricsCollector is a Custom Collector for the metadata cache metrics.
type MetadataMetricsCollector struct {
	CacheEntries    *prometheus.Desc
	CacheBytes      *prometheus.Desc
	TargetsGatherer TargetsGatherer
}

// Describe sends the metrics descriptions to the channel.
func (mc *MetadataMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- mc.CacheEntries
	ch <- mc.CacheBytes
}

// Collect creates and sends the metrics for the metadata cache.
func (mc *MetadataMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	if mc.TargetsGatherer == nil {
		return
	}

	for tset, targets := range mc.TargetsGatherer.TargetsActive() {
		var size, length int
		for _, t := range targets {
			size += t.SizeMetadata()
			length += t.LengthMetadata()
		}

		ch <- prometheus.MustNewConstMetric(
			mc.CacheEntries,
			prometheus.GaugeValue,
			float64(length),
			tset,
		)

		ch <- prometheus.MustNewConstMetric(
			mc.CacheBytes,
			prometheus.GaugeValue,
			float64(size),
			tset,
		)
	}
}
