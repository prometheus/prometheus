// Code generated from semantic convention specification. DO NOT EDIT.

// Package metrics provides Prometheus instrumentation types for metrics
// defined in this semantic convention registry.
package metrics

import (
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
			Name: "prometheus_target_interval_length_histogram_seconds",
			Help: "Actual intervals between scrapes as a histogram.",
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
	*prometheus.HistogramVec
}

// NewPrometheusTargetIntervalLengthSeconds returns a new PrometheusTargetIntervalLengthSeconds instrument.
func NewPrometheusTargetIntervalLengthSeconds() PrometheusTargetIntervalLengthSeconds {
	labels := []string{}
	return PrometheusTargetIntervalLengthSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_target_interval_length_seconds",
			Help: "Actual intervals between scrapes.",
		}, labels),
	}
}

type PrometheusTargetIntervalLengthSecondsAttr interface {
	Attribute
	implPrometheusTargetIntervalLengthSeconds()
}

func (m PrometheusTargetIntervalLengthSeconds) With(
	extra ...PrometheusTargetIntervalLengthSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
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

// PrometheusTargetScrapeDurationSeconds records the scrape request latency histogram.
type PrometheusTargetScrapeDurationSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusTargetScrapeDurationSeconds returns a new PrometheusTargetScrapeDurationSeconds instrument.
func NewPrometheusTargetScrapeDurationSeconds() PrometheusTargetScrapeDurationSeconds {
	labels := []string{}
	return PrometheusTargetScrapeDurationSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_target_scrape_duration_seconds",
			Help: "Scrape request latency histogram.",
		}, labels),
	}
}

type PrometheusTargetScrapeDurationSecondsAttr interface {
	Attribute
	implPrometheusTargetScrapeDurationSeconds()
}

func (m PrometheusTargetScrapeDurationSeconds) With(
	extra ...PrometheusTargetScrapeDurationSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusTargetScrapePoolExceededLabelLimitsTotal records the total number of times scrape pools hit the label limits.
type PrometheusTargetScrapePoolExceededLabelLimitsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapePoolExceededLabelLimitsTotal returns a new PrometheusTargetScrapePoolExceededLabelLimitsTotal instrument.
func NewPrometheusTargetScrapePoolExceededLabelLimitsTotal() PrometheusTargetScrapePoolExceededLabelLimitsTotal {
	labels := []string{}
	return PrometheusTargetScrapePoolExceededLabelLimitsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_exceeded_label_limits_total",
			Help: "Total number of times scrape pools hit the label limits.",
		}, labels),
	}
}

type PrometheusTargetScrapePoolExceededLabelLimitsTotalAttr interface {
	Attribute
	implPrometheusTargetScrapePoolExceededLabelLimitsTotal()
}

func (m PrometheusTargetScrapePoolExceededLabelLimitsTotal) With(
	extra ...PrometheusTargetScrapePoolExceededLabelLimitsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetScrapePoolExceededTargetLimitTotal records the total number of times scrape pools hit the target limit.
type PrometheusTargetScrapePoolExceededTargetLimitTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapePoolExceededTargetLimitTotal returns a new PrometheusTargetScrapePoolExceededTargetLimitTotal instrument.
func NewPrometheusTargetScrapePoolExceededTargetLimitTotal() PrometheusTargetScrapePoolExceededTargetLimitTotal {
	labels := []string{}
	return PrometheusTargetScrapePoolExceededTargetLimitTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_exceeded_target_limit_total",
			Help: "Total number of times scrape pools hit the target limit.",
		}, labels),
	}
}

type PrometheusTargetScrapePoolExceededTargetLimitTotalAttr interface {
	Attribute
	implPrometheusTargetScrapePoolExceededTargetLimitTotal()
}

func (m PrometheusTargetScrapePoolExceededTargetLimitTotal) With(
	extra ...PrometheusTargetScrapePoolExceededTargetLimitTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetScrapePoolReloadsFailedTotal records the total number of failed scrape pool reloads.
type PrometheusTargetScrapePoolReloadsFailedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapePoolReloadsFailedTotal returns a new PrometheusTargetScrapePoolReloadsFailedTotal instrument.
func NewPrometheusTargetScrapePoolReloadsFailedTotal() PrometheusTargetScrapePoolReloadsFailedTotal {
	labels := []string{}
	return PrometheusTargetScrapePoolReloadsFailedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_reloads_failed_total",
			Help: "Total number of failed scrape pool reloads.",
		}, labels),
	}
}

type PrometheusTargetScrapePoolReloadsFailedTotalAttr interface {
	Attribute
	implPrometheusTargetScrapePoolReloadsFailedTotal()
}

func (m PrometheusTargetScrapePoolReloadsFailedTotal) With(
	extra ...PrometheusTargetScrapePoolReloadsFailedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetScrapePoolReloadsTotal records the total number of scrape pool reloads.
type PrometheusTargetScrapePoolReloadsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapePoolReloadsTotal returns a new PrometheusTargetScrapePoolReloadsTotal instrument.
func NewPrometheusTargetScrapePoolReloadsTotal() PrometheusTargetScrapePoolReloadsTotal {
	labels := []string{}
	return PrometheusTargetScrapePoolReloadsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_reloads_total",
			Help: "Total number of scrape pool reloads.",
		}, labels),
	}
}

type PrometheusTargetScrapePoolReloadsTotalAttr interface {
	Attribute
	implPrometheusTargetScrapePoolReloadsTotal()
}

func (m PrometheusTargetScrapePoolReloadsTotal) With(
	extra ...PrometheusTargetScrapePoolReloadsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
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
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapePoolsFailedTotal returns a new PrometheusTargetScrapePoolsFailedTotal instrument.
func NewPrometheusTargetScrapePoolsFailedTotal() PrometheusTargetScrapePoolsFailedTotal {
	labels := []string{}
	return PrometheusTargetScrapePoolsFailedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pools_failed_total",
			Help: "Total number of scrape pool creations that failed.",
		}, labels),
	}
}

type PrometheusTargetScrapePoolsFailedTotalAttr interface {
	Attribute
	implPrometheusTargetScrapePoolsFailedTotal()
}

func (m PrometheusTargetScrapePoolsFailedTotal) With(
	extra ...PrometheusTargetScrapePoolsFailedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetScrapePoolsTotal records the total number of scrape pool creation attempts.
type PrometheusTargetScrapePoolsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapePoolsTotal returns a new PrometheusTargetScrapePoolsTotal instrument.
func NewPrometheusTargetScrapePoolsTotal() PrometheusTargetScrapePoolsTotal {
	labels := []string{}
	return PrometheusTargetScrapePoolsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pools_total",
			Help: "Total number of scrape pool creation attempts.",
		}, labels),
	}
}

type PrometheusTargetScrapePoolsTotalAttr interface {
	Attribute
	implPrometheusTargetScrapePoolsTotal()
}

func (m PrometheusTargetScrapePoolsTotal) With(
	extra ...PrometheusTargetScrapePoolsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetScrapesCacheFlushForcedTotal records the total number of scrapes that forced a complete label cache flush.
type PrometheusTargetScrapesCacheFlushForcedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapesCacheFlushForcedTotal returns a new PrometheusTargetScrapesCacheFlushForcedTotal instrument.
func NewPrometheusTargetScrapesCacheFlushForcedTotal() PrometheusTargetScrapesCacheFlushForcedTotal {
	labels := []string{}
	return PrometheusTargetScrapesCacheFlushForcedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_cache_flush_forced_total",
			Help: "Total number of scrapes that forced a complete label cache flush.",
		}, labels),
	}
}

type PrometheusTargetScrapesCacheFlushForcedTotalAttr interface {
	Attribute
	implPrometheusTargetScrapesCacheFlushForcedTotal()
}

func (m PrometheusTargetScrapesCacheFlushForcedTotal) With(
	extra ...PrometheusTargetScrapesCacheFlushForcedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetScrapesExceededBodySizeLimitTotal records the total number of scrapes that hit the body size limit.
type PrometheusTargetScrapesExceededBodySizeLimitTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapesExceededBodySizeLimitTotal returns a new PrometheusTargetScrapesExceededBodySizeLimitTotal instrument.
func NewPrometheusTargetScrapesExceededBodySizeLimitTotal() PrometheusTargetScrapesExceededBodySizeLimitTotal {
	labels := []string{}
	return PrometheusTargetScrapesExceededBodySizeLimitTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exceeded_body_size_limit_total",
			Help: "Total number of scrapes that hit the body size limit.",
		}, labels),
	}
}

type PrometheusTargetScrapesExceededBodySizeLimitTotalAttr interface {
	Attribute
	implPrometheusTargetScrapesExceededBodySizeLimitTotal()
}

func (m PrometheusTargetScrapesExceededBodySizeLimitTotal) With(
	extra ...PrometheusTargetScrapesExceededBodySizeLimitTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal records the total number of scrapes that hit the native histogram bucket limit.
type PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal returns a new PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal instrument.
func NewPrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal() PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal {
	labels := []string{}
	return PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exceeded_native_histogram_bucket_limit_total",
			Help: "Total number of scrapes that hit the native histogram bucket limit.",
		}, labels),
	}
}

type PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotalAttr interface {
	Attribute
	implPrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal()
}

func (m PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotal) With(
	extra ...PrometheusTargetScrapesExceededNativeHistogramBucketLimitTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetScrapesExceededSampleLimitTotal records the total number of scrapes that hit the sample limit.
type PrometheusTargetScrapesExceededSampleLimitTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapesExceededSampleLimitTotal returns a new PrometheusTargetScrapesExceededSampleLimitTotal instrument.
func NewPrometheusTargetScrapesExceededSampleLimitTotal() PrometheusTargetScrapesExceededSampleLimitTotal {
	labels := []string{}
	return PrometheusTargetScrapesExceededSampleLimitTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exceeded_sample_limit_total",
			Help: "Total number of scrapes that hit the sample limit.",
		}, labels),
	}
}

type PrometheusTargetScrapesExceededSampleLimitTotalAttr interface {
	Attribute
	implPrometheusTargetScrapesExceededSampleLimitTotal()
}

func (m PrometheusTargetScrapesExceededSampleLimitTotal) With(
	extra ...PrometheusTargetScrapesExceededSampleLimitTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetScrapesExemplarOutOfOrderTotal records the total number of exemplar rejected due to not being out of the expected order.
type PrometheusTargetScrapesExemplarOutOfOrderTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapesExemplarOutOfOrderTotal returns a new PrometheusTargetScrapesExemplarOutOfOrderTotal instrument.
func NewPrometheusTargetScrapesExemplarOutOfOrderTotal() PrometheusTargetScrapesExemplarOutOfOrderTotal {
	labels := []string{}
	return PrometheusTargetScrapesExemplarOutOfOrderTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exemplar_out_of_order_total",
			Help: "Total number of exemplar rejected due to not being out of the expected order.",
		}, labels),
	}
}

type PrometheusTargetScrapesExemplarOutOfOrderTotalAttr interface {
	Attribute
	implPrometheusTargetScrapesExemplarOutOfOrderTotal()
}

func (m PrometheusTargetScrapesExemplarOutOfOrderTotal) With(
	extra ...PrometheusTargetScrapesExemplarOutOfOrderTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetScrapesSampleDuplicateTimestampTotal records the total number of samples rejected due to duplicate timestamps but different values.
type PrometheusTargetScrapesSampleDuplicateTimestampTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapesSampleDuplicateTimestampTotal returns a new PrometheusTargetScrapesSampleDuplicateTimestampTotal instrument.
func NewPrometheusTargetScrapesSampleDuplicateTimestampTotal() PrometheusTargetScrapesSampleDuplicateTimestampTotal {
	labels := []string{}
	return PrometheusTargetScrapesSampleDuplicateTimestampTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_duplicate_timestamp_total",
			Help: "Total number of samples rejected due to duplicate timestamps but different values.",
		}, labels),
	}
}

type PrometheusTargetScrapesSampleDuplicateTimestampTotalAttr interface {
	Attribute
	implPrometheusTargetScrapesSampleDuplicateTimestampTotal()
}

func (m PrometheusTargetScrapesSampleDuplicateTimestampTotal) With(
	extra ...PrometheusTargetScrapesSampleDuplicateTimestampTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetScrapesSampleOutOfBoundsTotal records the total number of samples rejected due to timestamp falling outside of the time bounds.
type PrometheusTargetScrapesSampleOutOfBoundsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapesSampleOutOfBoundsTotal returns a new PrometheusTargetScrapesSampleOutOfBoundsTotal instrument.
func NewPrometheusTargetScrapesSampleOutOfBoundsTotal() PrometheusTargetScrapesSampleOutOfBoundsTotal {
	labels := []string{}
	return PrometheusTargetScrapesSampleOutOfBoundsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_out_of_bounds_total",
			Help: "Total number of samples rejected due to timestamp falling outside of the time bounds.",
		}, labels),
	}
}

type PrometheusTargetScrapesSampleOutOfBoundsTotalAttr interface {
	Attribute
	implPrometheusTargetScrapesSampleOutOfBoundsTotal()
}

func (m PrometheusTargetScrapesSampleOutOfBoundsTotal) With(
	extra ...PrometheusTargetScrapesSampleOutOfBoundsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTargetScrapesSampleOutOfOrderTotal records the total number of samples rejected due to not being out of the expected order.
type PrometheusTargetScrapesSampleOutOfOrderTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTargetScrapesSampleOutOfOrderTotal returns a new PrometheusTargetScrapesSampleOutOfOrderTotal instrument.
func NewPrometheusTargetScrapesSampleOutOfOrderTotal() PrometheusTargetScrapesSampleOutOfOrderTotal {
	labels := []string{}
	return PrometheusTargetScrapesSampleOutOfOrderTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_out_of_order_total",
			Help: "Total number of samples rejected due to not being out of the expected order.",
		}, labels),
	}
}

type PrometheusTargetScrapesSampleOutOfOrderTotalAttr interface {
	Attribute
	implPrometheusTargetScrapesSampleOutOfOrderTotal()
}

func (m PrometheusTargetScrapesSampleOutOfOrderTotal) With(
	extra ...PrometheusTargetScrapesSampleOutOfOrderTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
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
			Name: "prometheus_target_sync_length_histogram_seconds",
			Help: "Actual interval to sync the scrape pool as a histogram.",
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
	*prometheus.HistogramVec
}

// NewPrometheusTargetSyncLengthSeconds returns a new PrometheusTargetSyncLengthSeconds instrument.
func NewPrometheusTargetSyncLengthSeconds() PrometheusTargetSyncLengthSeconds {
	labels := []string{}
	return PrometheusTargetSyncLengthSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_target_sync_length_seconds",
			Help: "Actual interval to sync the scrape pool.",
		}, labels),
	}
}

type PrometheusTargetSyncLengthSecondsAttr interface {
	Attribute
	implPrometheusTargetSyncLengthSeconds()
}

func (m PrometheusTargetSyncLengthSeconds) With(
	extra ...PrometheusTargetSyncLengthSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}
