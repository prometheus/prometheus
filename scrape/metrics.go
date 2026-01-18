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

	"github.com/prometheus/client_golang/prometheus"

	semconv "github.com/prometheus/prometheus/scrape/semconv"
)

type scrapeMetrics struct {
	reg prometheus.Registerer
	// Used by Manager.
	targetMetadataCache     *MetadataMetricsCollector
	targetScrapePools       semconv.PrometheusTargetScrapePoolsTotal
	targetScrapePoolsFailed semconv.PrometheusTargetScrapePoolsFailedTotal

	// Used by scrapePool.
	targetReloadIntervalLength          semconv.PrometheusTargetReloadLengthSeconds
	targetScrapePoolReloads             semconv.PrometheusTargetScrapePoolReloadsTotal
	targetScrapePoolReloadsFailed       semconv.PrometheusTargetScrapePoolReloadsFailedTotal
	targetScrapePoolSyncsCounter        semconv.PrometheusTargetScrapePoolSyncTotal
	targetScrapePoolExceededTargetLimit semconv.PrometheusTargetScrapePoolExceededTargetLimitTotal
	targetScrapePoolTargetLimit         semconv.PrometheusTargetScrapePoolTargetLimit
	targetScrapePoolTargetsAdded        semconv.PrometheusTargetScrapePoolTargets
	targetScrapePoolSymbolTableItems    semconv.PrometheusTargetScrapePoolSymboltableItems
	targetSyncIntervalLength            semconv.PrometheusTargetSyncLengthSeconds
	targetSyncIntervalLengthHistogram   semconv.PrometheusTargetSyncLengthHistogramSeconds
	targetSyncFailed                    semconv.PrometheusTargetSyncFailedTotal

	// Used by targetScraper.
	targetScrapeExceededBodySizeLimit semconv.PrometheusTargetScrapesExceededBodySizeLimitTotal

	// Used by scrapeCache.
	targetScrapeCacheFlushForced semconv.PrometheusTargetScrapesCacheFlushForcedTotal

	// Used by scrapeLoop.
	targetIntervalLength                   semconv.PrometheusTargetIntervalLengthSeconds
	targetIntervalLengthHistogram          semconv.PrometheusTargetIntervalLengthHistogramSeconds
	targetScrapeSampleLimit                semconv.PrometheusTargetScrapesExceededSampleLimitTotal
	targetScrapeSampleDuplicate            semconv.PrometheusTargetScrapesSampleDuplicateTimestampTotal
	targetScrapeSampleOutOfOrder           semconv.PrometheusTargetScrapesSampleOutOfOrderTotal
	targetScrapeSampleOutOfBounds          semconv.PrometheusTargetScrapesSampleOutOfBoundsTotal
	targetScrapeExemplarOutOfOrder         semconv.PrometheusTargetScrapesExemplarOutOfOrderTotal
	targetScrapePoolExceededLabelLimits    semconv.PrometheusTargetScrapePoolExceededLabelLimitsTotal
	targetScrapeNativeHistogramBucketLimit semconv.PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal
	targetScrapeDuration                   semconv.PrometheusTargetScrapeDurationSeconds
}

func newScrapeMetrics(reg prometheus.Registerer) (*scrapeMetrics, error) {
	sm := &scrapeMetrics{
		reg: reg,
		// Manager metrics - MetadataMetricsCollector uses custom collector pattern (cannot migrate)
		targetMetadataCache: &MetadataMetricsCollector{
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
		},
		targetScrapePools:       semconv.NewPrometheusTargetScrapePoolsTotal(),
		targetScrapePoolsFailed: semconv.NewPrometheusTargetScrapePoolsFailedTotal(),

		// Used by scrapePool.
		targetReloadIntervalLength:          semconv.NewPrometheusTargetReloadLengthSeconds(),
		targetScrapePoolReloads:             semconv.NewPrometheusTargetScrapePoolReloadsTotal(),
		targetScrapePoolReloadsFailed:       semconv.NewPrometheusTargetScrapePoolReloadsFailedTotal(),
		targetScrapePoolExceededTargetLimit: semconv.NewPrometheusTargetScrapePoolExceededTargetLimitTotal(),
		targetScrapePoolTargetLimit:         semconv.NewPrometheusTargetScrapePoolTargetLimit(),
		targetScrapePoolTargetsAdded:        semconv.NewPrometheusTargetScrapePoolTargets(),
		targetScrapePoolSymbolTableItems:    semconv.NewPrometheusTargetScrapePoolSymboltableItems(),
		targetScrapePoolSyncsCounter:        semconv.NewPrometheusTargetScrapePoolSyncTotal(),
		targetSyncIntervalLength:            semconv.NewPrometheusTargetSyncLengthSeconds(),
		targetSyncIntervalLengthHistogram:   semconv.NewPrometheusTargetSyncLengthHistogramSeconds(),
		targetSyncFailed:                    semconv.NewPrometheusTargetSyncFailedTotal(),

		// Used by targetScraper.
		targetScrapeExceededBodySizeLimit: semconv.NewPrometheusTargetScrapesExceededBodySizeLimitTotal(),

		// Used by scrapeCache.
		targetScrapeCacheFlushForced: semconv.NewPrometheusTargetScrapesCacheFlushForcedTotal(),

		// Used by scrapeLoop.
		targetIntervalLength:                   semconv.NewPrometheusTargetIntervalLengthSeconds(),
		targetIntervalLengthHistogram:          semconv.NewPrometheusTargetIntervalLengthHistogramSeconds(),
		targetScrapeSampleLimit:                semconv.NewPrometheusTargetScrapesExceededSampleLimitTotal(),
		targetScrapeSampleDuplicate:            semconv.NewPrometheusTargetScrapesSampleDuplicateTimestampTotal(),
		targetScrapeSampleOutOfOrder:           semconv.NewPrometheusTargetScrapesSampleOutOfOrderTotal(),
		targetScrapeSampleOutOfBounds:          semconv.NewPrometheusTargetScrapesSampleOutOfBoundsTotal(),
		targetScrapeExemplarOutOfOrder:         semconv.NewPrometheusTargetScrapesExemplarOutOfOrderTotal(),
		targetScrapePoolExceededLabelLimits:    semconv.NewPrometheusTargetScrapePoolExceededLabelLimitsTotal(),
		targetScrapeNativeHistogramBucketLimit: semconv.NewPrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal(),
		targetScrapeDuration:                   semconv.NewPrometheusTargetScrapeDurationSeconds(),
	}

	for _, collector := range []prometheus.Collector{
		// Used by Manager.
		sm.targetMetadataCache,
		sm.targetScrapePools.Counter,
		sm.targetScrapePoolsFailed.Counter,
		// Used by scrapePool.
		sm.targetReloadIntervalLength.SummaryVec,
		sm.targetScrapePoolReloads.Counter,
		sm.targetScrapePoolReloadsFailed.Counter,
		sm.targetSyncIntervalLength.SummaryVec,
		sm.targetSyncIntervalLengthHistogram.HistogramVec,
		sm.targetScrapePoolSyncsCounter.CounterVec,
		sm.targetScrapePoolExceededTargetLimit.Counter,
		sm.targetScrapePoolTargetLimit.GaugeVec,
		sm.targetScrapePoolTargetsAdded.GaugeVec,
		sm.targetScrapePoolSymbolTableItems.GaugeVec,
		sm.targetSyncFailed.CounterVec,
		// Used by targetScraper.
		sm.targetScrapeExceededBodySizeLimit.Counter,
		// Used by scrapeCache.
		sm.targetScrapeCacheFlushForced.Counter,
		// Used by scrapeLoop.
		sm.targetIntervalLength.SummaryVec,
		sm.targetIntervalLengthHistogram.HistogramVec,
		sm.targetScrapeSampleLimit.Counter,
		sm.targetScrapeSampleDuplicate.Counter,
		sm.targetScrapeSampleOutOfOrder.Counter,
		sm.targetScrapeSampleOutOfBounds.Counter,
		sm.targetScrapeExemplarOutOfOrder.Counter,
		sm.targetScrapePoolExceededLabelLimits.Counter,
		sm.targetScrapeNativeHistogramBucketLimit.Counter,
		sm.targetScrapeDuration.Histogram,
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
	sm.reg.Unregister(sm.targetScrapePools.Counter)
	sm.reg.Unregister(sm.targetScrapePoolsFailed.Counter)
	sm.reg.Unregister(sm.targetReloadIntervalLength.SummaryVec)
	sm.reg.Unregister(sm.targetScrapePoolReloads.Counter)
	sm.reg.Unregister(sm.targetScrapePoolReloadsFailed.Counter)
	sm.reg.Unregister(sm.targetSyncIntervalLength.SummaryVec)
	sm.reg.Unregister(sm.targetSyncIntervalLengthHistogram.HistogramVec)
	sm.reg.Unregister(sm.targetScrapePoolSyncsCounter.CounterVec)
	sm.reg.Unregister(sm.targetScrapePoolExceededTargetLimit.Counter)
	sm.reg.Unregister(sm.targetScrapePoolTargetLimit.GaugeVec)
	sm.reg.Unregister(sm.targetScrapePoolTargetsAdded.GaugeVec)
	sm.reg.Unregister(sm.targetScrapePoolSymbolTableItems.GaugeVec)
	sm.reg.Unregister(sm.targetSyncFailed.CounterVec)
	sm.reg.Unregister(sm.targetScrapeExceededBodySizeLimit.Counter)
	sm.reg.Unregister(sm.targetScrapeCacheFlushForced.Counter)
	sm.reg.Unregister(sm.targetIntervalLength.SummaryVec)
	sm.reg.Unregister(sm.targetIntervalLengthHistogram.HistogramVec)
	sm.reg.Unregister(sm.targetScrapeSampleLimit.Counter)
	sm.reg.Unregister(sm.targetScrapeSampleDuplicate.Counter)
	sm.reg.Unregister(sm.targetScrapeSampleOutOfOrder.Counter)
	sm.reg.Unregister(sm.targetScrapeSampleOutOfBounds.Counter)
	sm.reg.Unregister(sm.targetScrapeExemplarOutOfOrder.Counter)
	sm.reg.Unregister(sm.targetScrapePoolExceededLabelLimits.Counter)
	sm.reg.Unregister(sm.targetScrapeNativeHistogramBucketLimit.Counter)
	sm.reg.Unregister(sm.targetScrapeDuration.Histogram)
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
