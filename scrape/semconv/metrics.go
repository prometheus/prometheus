// Code generated from semantic convention specification. DO NOT EDIT.

// Package metrics provides Prometheus instrumentation types for metrics
// defined in this semantic convention registry.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Attribute is an interface for metric label attributes.
type Attribute interface {
	ID() string
	Value() string
}
type IntervalAttr string

func (a IntervalAttr) ID() string {
	return "interval"
}

func (a IntervalAttr) Value() string {
	return string(a)
}

type ScrapeJobAttr string

func (a ScrapeJobAttr) ID() string {
	return "scrape_job"
}

func (a ScrapeJobAttr) Value() string {
	return string(a)
}

// PrometheusTargetIntervalLengthHistogramSeconds records the actual intervals between scrapes as a histogram.
type PrometheusTargetIntervalLengthHistogramSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusTargetIntervalLengthHistogramSeconds returns a new PrometheusTargetIntervalLengthHistogramSeconds instrument.
func NewPrometheusTargetIntervalLengthHistogramSeconds() PrometheusTargetIntervalLengthHistogramSeconds {
	labels := []string{
		"interval",
	}
	return PrometheusTargetIntervalLengthHistogramSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:                            "prometheus_target_interval_length_histogram_seconds",
			Help:                            "Actual intervals between scrapes as a histogram.",
			Buckets:                         []float64{0.01, 0.1, 1, 10},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		}, labels),
	}
}

type PrometheusTargetIntervalLengthHistogramSecondsAttr interface {
	Attribute
	implPrometheusTargetIntervalLengthHistogramSeconds()
}

func (a IntervalAttr) implPrometheusTargetIntervalLengthHistogramSeconds() {}

func (m PrometheusTargetIntervalLengthHistogramSeconds) With(
	extra ...PrometheusTargetIntervalLengthHistogramSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{
		"interval": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusTargetIntervalLengthSeconds records the actual intervals between scrapes.
type PrometheusTargetIntervalLengthSeconds struct {
	*prometheus.SummaryVec
}

// NewPrometheusTargetIntervalLengthSeconds returns a new PrometheusTargetIntervalLengthSeconds instrument.
func NewPrometheusTargetIntervalLengthSeconds() PrometheusTargetIntervalLengthSeconds {
	labels := []string{
		"interval",
	}
	return PrometheusTargetIntervalLengthSeconds{
		SummaryVec: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Name: "prometheus_target_interval_length_seconds",
			Help: "Actual intervals between scrapes.",
			Objectives: map[float64]float64{
				0.01: 0.001,
				0.05: 0.005,
				0.5:  0.05,
				0.9:  0.01,
				0.99: 0.001,
			},
		}, labels),
	}
}

type PrometheusTargetIntervalLengthSecondsAttr interface {
	Attribute
	implPrometheusTargetIntervalLengthSeconds()
}

func (a IntervalAttr) implPrometheusTargetIntervalLengthSeconds() {}

func (m PrometheusTargetIntervalLengthSeconds) With(
	extra ...PrometheusTargetIntervalLengthSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{
		"interval": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.SummaryVec.With(labels)
}

// PrometheusTargetMetadataCacheBytes records the number of bytes that are currently used for storing metric metadata in the cache.
type PrometheusTargetMetadataCacheBytes struct {
	*prometheus.GaugeVec
}

// NewPrometheusTargetMetadataCacheBytes returns a new PrometheusTargetMetadataCacheBytes instrument.
func NewPrometheusTargetMetadataCacheBytes() PrometheusTargetMetadataCacheBytes {
	labels := []string{
		"scrape_job",
	}
	return PrometheusTargetMetadataCacheBytes{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_target_metadata_cache_bytes",
			Help: "The number of bytes that are currently used for storing metric metadata in the cache.",
		}, labels),
	}
}

type PrometheusTargetMetadataCacheBytesAttr interface {
	Attribute
	implPrometheusTargetMetadataCacheBytes()
}

func (a ScrapeJobAttr) implPrometheusTargetMetadataCacheBytes() {}

func (m PrometheusTargetMetadataCacheBytes) With(
	extra ...PrometheusTargetMetadataCacheBytesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"scrape_job": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTargetMetadataCacheEntries records the total number of metric metadata entries in the cache.
type PrometheusTargetMetadataCacheEntries struct {
	*prometheus.GaugeVec
}

// NewPrometheusTargetMetadataCacheEntries returns a new PrometheusTargetMetadataCacheEntries instrument.
func NewPrometheusTargetMetadataCacheEntries() PrometheusTargetMetadataCacheEntries {
	labels := []string{
		"scrape_job",
	}
	return PrometheusTargetMetadataCacheEntries{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_target_metadata_cache_entries",
			Help: "Total number of metric metadata entries in the cache.",
		}, labels),
	}
}

type PrometheusTargetMetadataCacheEntriesAttr interface {
	Attribute
	implPrometheusTargetMetadataCacheEntries()
}

func (a ScrapeJobAttr) implPrometheusTargetMetadataCacheEntries() {}

func (m PrometheusTargetMetadataCacheEntries) With(
	extra ...PrometheusTargetMetadataCacheEntriesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"scrape_job": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTargetReloadLengthSeconds records the actual interval to reload the scrape pool with a given configuration.
type PrometheusTargetReloadLengthSeconds struct {
	*prometheus.SummaryVec
}

// NewPrometheusTargetReloadLengthSeconds returns a new PrometheusTargetReloadLengthSeconds instrument.
func NewPrometheusTargetReloadLengthSeconds() PrometheusTargetReloadLengthSeconds {
	labels := []string{
		"interval",
	}
	return PrometheusTargetReloadLengthSeconds{
		SummaryVec: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Name: "prometheus_target_reload_length_seconds",
			Help: "Actual interval to reload the scrape pool with a given configuration.",
			Objectives: map[float64]float64{
				0.01: 0.001,
				0.05: 0.005,
				0.5:  0.05,
				0.9:  0.01,
				0.99: 0.001,
			},
		}, labels),
	}
}

type PrometheusTargetReloadLengthSecondsAttr interface {
	Attribute
	implPrometheusTargetReloadLengthSeconds()
}

func (a IntervalAttr) implPrometheusTargetReloadLengthSeconds() {}

func (m PrometheusTargetReloadLengthSeconds) With(
	extra ...PrometheusTargetReloadLengthSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{
		"interval": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.SummaryVec.With(labels)
}

// PrometheusTargetScrapeDurationSeconds records the scrape request latency histogram.
type PrometheusTargetScrapeDurationSeconds struct {
	prometheus.Histogram
}

// NewPrometheusTargetScrapeDurationSeconds returns a new PrometheusTargetScrapeDurationSeconds instrument.
func NewPrometheusTargetScrapeDurationSeconds() PrometheusTargetScrapeDurationSeconds {
	return PrometheusTargetScrapeDurationSeconds{
		Histogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:                            "prometheus_target_scrape_duration_seconds",
			Help:                            "Scrape request latency histogram.",
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		}),
	}
}

// PrometheusTargetScrapePoolExceededLabelLimitsTotal records the total number of times scrape pools hit the label limits.
type PrometheusTargetScrapePoolExceededLabelLimitsTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapePoolExceededLabelLimitsTotal returns a new PrometheusTargetScrapePoolExceededLabelLimitsTotal instrument.
func NewPrometheusTargetScrapePoolExceededLabelLimitsTotal() PrometheusTargetScrapePoolExceededLabelLimitsTotal {
	return PrometheusTargetScrapePoolExceededLabelLimitsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_exceeded_label_limits_total",
			Help: "Total number of times scrape pools hit the label limits.",
		}),
	}
}

// PrometheusTargetScrapePoolExceededTargetLimitTotal records the total number of times scrape pools hit the target limit.
type PrometheusTargetScrapePoolExceededTargetLimitTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapePoolExceededTargetLimitTotal returns a new PrometheusTargetScrapePoolExceededTargetLimitTotal instrument.
func NewPrometheusTargetScrapePoolExceededTargetLimitTotal() PrometheusTargetScrapePoolExceededTargetLimitTotal {
	return PrometheusTargetScrapePoolExceededTargetLimitTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_exceeded_target_limit_total",
			Help: "Total number of times scrape pools hit the target limit.",
		}),
	}
}

// PrometheusTargetScrapePoolReloadsFailedTotal records the total number of failed scrape pool reloads.
type PrometheusTargetScrapePoolReloadsFailedTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapePoolReloadsFailedTotal returns a new PrometheusTargetScrapePoolReloadsFailedTotal instrument.
func NewPrometheusTargetScrapePoolReloadsFailedTotal() PrometheusTargetScrapePoolReloadsFailedTotal {
	return PrometheusTargetScrapePoolReloadsFailedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_reloads_failed_total",
			Help: "Total number of failed scrape pool reloads.",
		}),
	}
}

// PrometheusTargetScrapePoolReloadsTotal records the total number of scrape pool reloads.
type PrometheusTargetScrapePoolReloadsTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapePoolReloadsTotal returns a new PrometheusTargetScrapePoolReloadsTotal instrument.
func NewPrometheusTargetScrapePoolReloadsTotal() PrometheusTargetScrapePoolReloadsTotal {
	return PrometheusTargetScrapePoolReloadsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_reloads_total",
			Help: "Total number of scrape pool reloads.",
		}),
	}
}

// PrometheusTargetScrapePoolSymboltableItems records the current number of symbols in the scrape pool symbol table.
type PrometheusTargetScrapePoolSymboltableItems struct {
	*prometheus.GaugeVec
}

// NewPrometheusTargetScrapePoolSymboltableItems returns a new PrometheusTargetScrapePoolSymboltableItems instrument.
func NewPrometheusTargetScrapePoolSymboltableItems() PrometheusTargetScrapePoolSymboltableItems {
	labels := []string{
		"scrape_job",
	}
	return PrometheusTargetScrapePoolSymboltableItems{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_target_scrape_pool_symboltable_items",
			Help: "Current number of symbols in the scrape pool symbol table.",
		}, labels),
	}
}

type PrometheusTargetScrapePoolSymboltableItemsAttr interface {
	Attribute
	implPrometheusTargetScrapePoolSymboltableItems()
}

func (a ScrapeJobAttr) implPrometheusTargetScrapePoolSymboltableItems() {}

func (m PrometheusTargetScrapePoolSymboltableItems) With(
	extra ...PrometheusTargetScrapePoolSymboltableItemsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"scrape_job": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTargetScrapePoolSyncTotal records the total number of syncs that were executed on a scrape pool.
type PrometheusTargetScrapePoolSyncTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapePoolSyncTotal returns a new PrometheusTargetScrapePoolSyncTotal instrument.
func NewPrometheusTargetScrapePoolSyncTotal() PrometheusTargetScrapePoolSyncTotal {
	labels := []string{
		"scrape_job",
	}
	return PrometheusTargetScrapePoolSyncTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_sync_total",
			Help: "Total number of syncs that were executed on a scrape pool.",
		}, labels),
	}
}

type PrometheusTargetScrapePoolSyncTotalAttr interface {
	Attribute
	implPrometheusTargetScrapePoolSyncTotal()
}

func (a ScrapeJobAttr) implPrometheusTargetScrapePoolSyncTotal() {}

func (m PrometheusTargetScrapePoolSyncTotal) With(
	extra ...PrometheusTargetScrapePoolSyncTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"scrape_job": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetScrapePoolTargetLimit records the maximum number of targets allowed in this scrape pool.
type PrometheusTargetScrapePoolTargetLimit struct {
	*prometheus.GaugeVec
}

// NewPrometheusTargetScrapePoolTargetLimit returns a new PrometheusTargetScrapePoolTargetLimit instrument.
func NewPrometheusTargetScrapePoolTargetLimit() PrometheusTargetScrapePoolTargetLimit {
	labels := []string{
		"scrape_job",
	}
	return PrometheusTargetScrapePoolTargetLimit{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_target_scrape_pool_target_limit",
			Help: "Maximum number of targets allowed in this scrape pool.",
		}, labels),
	}
}

type PrometheusTargetScrapePoolTargetLimitAttr interface {
	Attribute
	implPrometheusTargetScrapePoolTargetLimit()
}

func (a ScrapeJobAttr) implPrometheusTargetScrapePoolTargetLimit() {}

func (m PrometheusTargetScrapePoolTargetLimit) With(
	extra ...PrometheusTargetScrapePoolTargetLimitAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"scrape_job": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTargetScrapePoolTargets records the current number of targets in this scrape pool.
type PrometheusTargetScrapePoolTargets struct {
	*prometheus.GaugeVec
}

// NewPrometheusTargetScrapePoolTargets returns a new PrometheusTargetScrapePoolTargets instrument.
func NewPrometheusTargetScrapePoolTargets() PrometheusTargetScrapePoolTargets {
	labels := []string{
		"scrape_job",
	}
	return PrometheusTargetScrapePoolTargets{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_target_scrape_pool_targets",
			Help: "Current number of targets in this scrape pool.",
		}, labels),
	}
}

type PrometheusTargetScrapePoolTargetsAttr interface {
	Attribute
	implPrometheusTargetScrapePoolTargets()
}

func (a ScrapeJobAttr) implPrometheusTargetScrapePoolTargets() {}

func (m PrometheusTargetScrapePoolTargets) With(
	extra ...PrometheusTargetScrapePoolTargetsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"scrape_job": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTargetScrapePoolsFailedTotal records the total number of scrape pool creations that failed.
type PrometheusTargetScrapePoolsFailedTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapePoolsFailedTotal returns a new PrometheusTargetScrapePoolsFailedTotal instrument.
func NewPrometheusTargetScrapePoolsFailedTotal() PrometheusTargetScrapePoolsFailedTotal {
	return PrometheusTargetScrapePoolsFailedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pools_failed_total",
			Help: "Total number of scrape pool creations that failed.",
		}),
	}
}

// PrometheusTargetScrapePoolsTotal records the total number of scrape pool creation attempts.
type PrometheusTargetScrapePoolsTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapePoolsTotal returns a new PrometheusTargetScrapePoolsTotal instrument.
func NewPrometheusTargetScrapePoolsTotal() PrometheusTargetScrapePoolsTotal {
	return PrometheusTargetScrapePoolsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pools_total",
			Help: "Total number of scrape pool creation attempts.",
		}),
	}
}

// PrometheusTargetScrapesCacheFlushForcedTotal records the total number of scrapes that forced a complete label cache flush.
type PrometheusTargetScrapesCacheFlushForcedTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapesCacheFlushForcedTotal returns a new PrometheusTargetScrapesCacheFlushForcedTotal instrument.
func NewPrometheusTargetScrapesCacheFlushForcedTotal() PrometheusTargetScrapesCacheFlushForcedTotal {
	return PrometheusTargetScrapesCacheFlushForcedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_cache_flush_forced_total",
			Help: "Total number of scrapes that forced a complete label cache flush.",
		}),
	}
}

// PrometheusTargetScrapesExceededBodySizeLimitTotal records the total number of scrapes that hit the body size limit.
type PrometheusTargetScrapesExceededBodySizeLimitTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapesExceededBodySizeLimitTotal returns a new PrometheusTargetScrapesExceededBodySizeLimitTotal instrument.
func NewPrometheusTargetScrapesExceededBodySizeLimitTotal() PrometheusTargetScrapesExceededBodySizeLimitTotal {
	return PrometheusTargetScrapesExceededBodySizeLimitTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exceeded_body_size_limit_total",
			Help: "Total number of scrapes that hit the body size limit.",
		}),
	}
}

// PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal records the total number of scrapes that hit the native histogram bucket limit.
type PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal returns a new PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal instrument.
func NewPrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal() PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal {
	return PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exceeded_native_histogram_bucket_limit_total",
			Help: "Total number of scrapes that hit the native histogram bucket limit.",
		}),
	}
}

// PrometheusTargetScrapesExceededSampleLimitTotal records the total number of scrapes that hit the sample limit.
type PrometheusTargetScrapesExceededSampleLimitTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapesExceededSampleLimitTotal returns a new PrometheusTargetScrapesExceededSampleLimitTotal instrument.
func NewPrometheusTargetScrapesExceededSampleLimitTotal() PrometheusTargetScrapesExceededSampleLimitTotal {
	return PrometheusTargetScrapesExceededSampleLimitTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exceeded_sample_limit_total",
			Help: "Total number of scrapes that hit the sample limit.",
		}),
	}
}

// PrometheusTargetScrapesExemplarOutOfOrderTotal records the total number of exemplar rejected due to not being out of the expected order.
type PrometheusTargetScrapesExemplarOutOfOrderTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapesExemplarOutOfOrderTotal returns a new PrometheusTargetScrapesExemplarOutOfOrderTotal instrument.
func NewPrometheusTargetScrapesExemplarOutOfOrderTotal() PrometheusTargetScrapesExemplarOutOfOrderTotal {
	return PrometheusTargetScrapesExemplarOutOfOrderTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exemplar_out_of_order_total",
			Help: "Total number of exemplar rejected due to not being out of the expected order.",
		}),
	}
}

// PrometheusTargetScrapesSampleDuplicateTimestampTotal records the total number of samples rejected due to duplicate timestamps but different values.
type PrometheusTargetScrapesSampleDuplicateTimestampTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapesSampleDuplicateTimestampTotal returns a new PrometheusTargetScrapesSampleDuplicateTimestampTotal instrument.
func NewPrometheusTargetScrapesSampleDuplicateTimestampTotal() PrometheusTargetScrapesSampleDuplicateTimestampTotal {
	return PrometheusTargetScrapesSampleDuplicateTimestampTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_duplicate_timestamp_total",
			Help: "Total number of samples rejected due to duplicate timestamps but different values.",
		}),
	}
}

// PrometheusTargetScrapesSampleOutOfBoundsTotal records the total number of samples rejected due to timestamp falling outside of the time bounds.
type PrometheusTargetScrapesSampleOutOfBoundsTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapesSampleOutOfBoundsTotal returns a new PrometheusTargetScrapesSampleOutOfBoundsTotal instrument.
func NewPrometheusTargetScrapesSampleOutOfBoundsTotal() PrometheusTargetScrapesSampleOutOfBoundsTotal {
	return PrometheusTargetScrapesSampleOutOfBoundsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_out_of_bounds_total",
			Help: "Total number of samples rejected due to timestamp falling outside of the time bounds.",
		}),
	}
}

// PrometheusTargetScrapesSampleOutOfOrderTotal records the total number of samples rejected due to not being out of the expected order.
type PrometheusTargetScrapesSampleOutOfOrderTotal struct {
	prometheus.Counter
}

// NewPrometheusTargetScrapesSampleOutOfOrderTotal returns a new PrometheusTargetScrapesSampleOutOfOrderTotal instrument.
func NewPrometheusTargetScrapesSampleOutOfOrderTotal() PrometheusTargetScrapesSampleOutOfOrderTotal {
	return PrometheusTargetScrapesSampleOutOfOrderTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_out_of_order_total",
			Help: "Total number of samples rejected due to not being out of the expected order.",
		}),
	}
}

// PrometheusTargetSyncFailedTotal records the total number of target sync failures.
type PrometheusTargetSyncFailedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetSyncFailedTotal returns a new PrometheusTargetSyncFailedTotal instrument.
func NewPrometheusTargetSyncFailedTotal() PrometheusTargetSyncFailedTotal {
	labels := []string{
		"scrape_job",
	}
	return PrometheusTargetSyncFailedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_sync_failed_total",
			Help: "Total number of target sync failures.",
		}, labels),
	}
}

type PrometheusTargetSyncFailedTotalAttr interface {
	Attribute
	implPrometheusTargetSyncFailedTotal()
}

func (a ScrapeJobAttr) implPrometheusTargetSyncFailedTotal() {}

func (m PrometheusTargetSyncFailedTotal) With(
	extra ...PrometheusTargetSyncFailedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"scrape_job": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetSyncLengthHistogramSeconds records the actual interval to sync the scrape pool as a histogram.
type PrometheusTargetSyncLengthHistogramSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusTargetSyncLengthHistogramSeconds returns a new PrometheusTargetSyncLengthHistogramSeconds instrument.
func NewPrometheusTargetSyncLengthHistogramSeconds() PrometheusTargetSyncLengthHistogramSeconds {
	labels := []string{
		"scrape_job",
	}
	return PrometheusTargetSyncLengthHistogramSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:                            "prometheus_target_sync_length_histogram_seconds",
			Help:                            "Actual interval to sync the scrape pool as a histogram.",
			Buckets:                         []float64{0.01, 0.1, 1, 10},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		}, labels),
	}
}

type PrometheusTargetSyncLengthHistogramSecondsAttr interface {
	Attribute
	implPrometheusTargetSyncLengthHistogramSeconds()
}

func (a ScrapeJobAttr) implPrometheusTargetSyncLengthHistogramSeconds() {}

func (m PrometheusTargetSyncLengthHistogramSeconds) With(
	extra ...PrometheusTargetSyncLengthHistogramSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{
		"scrape_job": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusTargetSyncLengthSeconds records the actual interval to sync the scrape pool.
type PrometheusTargetSyncLengthSeconds struct {
	*prometheus.SummaryVec
}

// NewPrometheusTargetSyncLengthSeconds returns a new PrometheusTargetSyncLengthSeconds instrument.
func NewPrometheusTargetSyncLengthSeconds() PrometheusTargetSyncLengthSeconds {
	labels := []string{
		"scrape_job",
	}
	return PrometheusTargetSyncLengthSeconds{
		SummaryVec: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Name: "prometheus_target_sync_length_seconds",
			Help: "Actual interval to sync the scrape pool.",
			Objectives: map[float64]float64{
				0.01: 0.001,
				0.05: 0.005,
				0.5:  0.05,
				0.9:  0.01,
				0.99: 0.001,
			},
		}, labels),
	}
}

type PrometheusTargetSyncLengthSecondsAttr interface {
	Attribute
	implPrometheusTargetSyncLengthSeconds()
}

func (a ScrapeJobAttr) implPrometheusTargetSyncLengthSeconds() {}

func (m PrometheusTargetSyncLengthSeconds) With(
	extra ...PrometheusTargetSyncLengthSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{
		"scrape_job": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.SummaryVec.With(labels)
}
