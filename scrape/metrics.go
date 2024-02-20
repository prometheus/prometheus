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
	"errors"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

type scrapeMetrics struct {
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
	targetSyncIntervalLength            *prometheus.SummaryVec
	targetSyncFailed                    *prometheus.CounterVec

	// Used by targetScraper.
	targetScrapeExceededBodySizeLimit prometheus.Counter

	// Used by scrapeCache.
	targetScrapeCacheFlushForced prometheus.Counter

	// Used by scrapeLoop.
	targetIntervalLength                   *prometheus.SummaryVec
	targetScrapeSampleLimit                prometheus.Counter
	targetScrapeSampleDuplicate            prometheus.Counter
	targetScrapeSampleOutOfOrder           prometheus.Counter
	targetScrapeSampleOutOfBounds          prometheus.Counter
	targetScrapeExemplarOutOfOrder         prometheus.Counter
	targetScrapePoolExceededLabelLimits    prometheus.Counter
	targetScrapeNativeHistogramBucketLimit prometheus.Counter
}

func newScrapeMetrics(reg prometheus.Registerer) (*scrapeMetrics, error) {
	sm := &scrapeMetrics{}

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

	var err error
	sm.targetScrapePools, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pools_total",
			Help: "Total number of scrape pool creation attempts.",
		},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapePoolsFailed, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pools_failed_total",
			Help: "Total number of scrape pool creations that failed.",
		},
	))
	if err != nil {
		return nil, err
	}

	// Used by scrapePool.
	sm.targetReloadIntervalLength, err = registerCollector(reg, prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_target_reload_length_seconds",
			Help:       "Actual interval to reload the scrape pool with a given configuration.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{"interval"},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapePoolReloads, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_reloads_total",
			Help: "Total number of scrape pool reloads.",
		},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapePoolReloadsFailed, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_reloads_failed_total",
			Help: "Total number of failed scrape pool reloads.",
		},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapePoolExceededTargetLimit, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_exceeded_target_limit_total",
			Help: "Total number of times scrape pools hit the target limit, during sync or config reload.",
		},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapePoolTargetLimit, err = registerCollector(reg, prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "prometheus_target_scrape_pool_target_limit",
			Help: "Maximum number of targets allowed in this scrape pool.",
		},
		[]string{"scrape_job"},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapePoolTargetsAdded, err = registerCollector(reg, prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "prometheus_target_scrape_pool_targets",
			Help: "Current number of targets in this scrape pool.",
		},
		[]string{"scrape_job"},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapePoolSyncsCounter, err = registerCollector(reg, prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_sync_total",
			Help: "Total number of syncs that were executed on a scrape pool.",
		},
		[]string{"scrape_job"},
	))
	if err != nil {
		return nil, err
	}

	sm.targetSyncIntervalLength, err = registerCollector(reg, prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_target_sync_length_seconds",
			Help:       "Actual interval to sync the scrape pool.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{"scrape_job"},
	))
	if err != nil {
		return nil, err
	}

	sm.targetSyncFailed, err = registerCollector(reg, prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_target_sync_failed_total",
			Help: "Total number of target sync failures.",
		},
		[]string{"scrape_job"},
	))
	if err != nil {
		return nil, err
	}

	// Used by targetScraper.
	sm.targetScrapeExceededBodySizeLimit, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exceeded_body_size_limit_total",
			Help: "Total number of scrapes that hit the body size limit",
		},
	))
	if err != nil {
		return nil, err
	}

	// Used by scrapeCache.
	sm.targetScrapeCacheFlushForced, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_cache_flush_forced_total",
			Help: "How many times a scrape cache was flushed due to getting big while scrapes are failing.",
		},
	))
	if err != nil {
		return nil, err
	}

	// Used by scrapeLoop.
	sm.targetIntervalLength, err = registerCollector(reg, prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_target_interval_length_seconds",
			Help:       "Actual intervals between scrapes.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{"interval"},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapeSampleLimit, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exceeded_sample_limit_total",
			Help: "Total number of scrapes that hit the sample limit and were rejected.",
		},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapeSampleDuplicate, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_duplicate_timestamp_total",
			Help: "Total number of samples rejected due to duplicate timestamps but different values.",
		},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapeSampleOutOfOrder, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_out_of_order_total",
			Help: "Total number of samples rejected due to not being out of the expected order.",
		},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapeSampleOutOfBounds, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_out_of_bounds_total",
			Help: "Total number of samples rejected due to timestamp falling outside of the time bounds.",
		},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapePoolExceededLabelLimits, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_exceeded_label_limits_total",
			Help: "Total number of times scrape pools hit the label limits, during sync or config reload.",
		},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapeNativeHistogramBucketLimit, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exceeded_native_histogram_bucket_limit_total",
			Help: "Total number of scrapes that hit the native histogram bucket limit and were rejected.",
		},
	))
	if err != nil {
		return nil, err
	}

	sm.targetScrapeExemplarOutOfOrder, err = registerCollector(reg, prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exemplar_out_of_order_total",
			Help: "Total number of exemplar rejected due to not being out of the expected order.",
		},
	))
	if err != nil {
		return nil, err
	}

	return sm, nil
}

func registerCollector[C prometheus.Collector](reg prometheus.Registerer, collector C) (C, error) {
	err := reg.Register(collector)
	if err != nil {
		are := &prometheus.AlreadyRegisteredError{}
		if errors.As(err, are) {
			return are.ExistingCollector.(C), nil
		}
		return collector, fmt.Errorf("failed to register scrape metrics: %w", err)
	}
	return collector, nil
}

func (sm *scrapeMetrics) setTargetMetadataCacheGatherer(gatherer TargetsGatherer) {
	sm.targetMetadataCache.TargetsGatherer = gatherer
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
