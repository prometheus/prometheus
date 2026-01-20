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
type CompressionAttr string

func (a CompressionAttr) ID() string {
	return "compression"
}

func (a CompressionAttr) Value() string {
	return string(a)
}

type TypeAttr string

func (a TypeAttr) ID() string {
	return "type"
}

func (a TypeAttr) Value() string {
	return string(a)
}

// PrometheusTSDBBlocksLoaded records the number of currently loaded data blocks.
func PrometheusTSDBBlocksLoadedOpts() prometheus.GaugeOpts {
	return prometheus.GaugeOpts{
		Name: "prometheus_tsdb_blocks_loaded",
		Help: "Number of currently loaded data blocks.",
	}
}

// PrometheusTSDBCheckpointCreationsFailedTotal records the total number of checkpoint creations that failed.
type PrometheusTSDBCheckpointCreationsFailedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBCheckpointCreationsFailedTotal returns a new PrometheusTSDBCheckpointCreationsFailedTotal instrument.
func NewPrometheusTSDBCheckpointCreationsFailedTotal() PrometheusTSDBCheckpointCreationsFailedTotal {
	return PrometheusTSDBCheckpointCreationsFailedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_checkpoint_creations_failed_total",
			Help: "Total number of checkpoint creations that failed.",
		}),
	}
}

// PrometheusTSDBCheckpointCreationsTotal records the total number of checkpoint creations attempted.
type PrometheusTSDBCheckpointCreationsTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBCheckpointCreationsTotal returns a new PrometheusTSDBCheckpointCreationsTotal instrument.
func NewPrometheusTSDBCheckpointCreationsTotal() PrometheusTSDBCheckpointCreationsTotal {
	return PrometheusTSDBCheckpointCreationsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_checkpoint_creations_total",
			Help: "Total number of checkpoint creations attempted.",
		}),
	}
}

// PrometheusTSDBCheckpointDeletionsFailedTotal records the total number of checkpoint deletions that failed.
type PrometheusTSDBCheckpointDeletionsFailedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBCheckpointDeletionsFailedTotal returns a new PrometheusTSDBCheckpointDeletionsFailedTotal instrument.
func NewPrometheusTSDBCheckpointDeletionsFailedTotal() PrometheusTSDBCheckpointDeletionsFailedTotal {
	return PrometheusTSDBCheckpointDeletionsFailedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_checkpoint_deletions_failed_total",
			Help: "Total number of checkpoint deletions that failed.",
		}),
	}
}

// PrometheusTSDBCheckpointDeletionsTotal records the total number of checkpoint deletions attempted.
type PrometheusTSDBCheckpointDeletionsTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBCheckpointDeletionsTotal returns a new PrometheusTSDBCheckpointDeletionsTotal instrument.
func NewPrometheusTSDBCheckpointDeletionsTotal() PrometheusTSDBCheckpointDeletionsTotal {
	return PrometheusTSDBCheckpointDeletionsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_checkpoint_deletions_total",
			Help: "Total number of checkpoint deletions attempted.",
		}),
	}
}

// PrometheusTSDBCleanStart records the set to 1 if the TSDB was clean at startup, 0 otherwise.
type PrometheusTSDBCleanStart struct {
	prometheus.Gauge
}

// NewPrometheusTSDBCleanStart returns a new PrometheusTSDBCleanStart instrument.
func NewPrometheusTSDBCleanStart() PrometheusTSDBCleanStart {
	return PrometheusTSDBCleanStart{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_clean_start",
			Help: "Set to 1 if the TSDB was clean at startup, 0 otherwise.",
		}),
	}
}

// PrometheusTSDBCompactionChunkRangeSeconds records the final time range of chunks on their first compaction.
type PrometheusTSDBCompactionChunkRangeSeconds struct {
	prometheus.Histogram
}

// NewPrometheusTSDBCompactionChunkRangeSeconds returns a new PrometheusTSDBCompactionChunkRangeSeconds instrument.
func NewPrometheusTSDBCompactionChunkRangeSeconds() PrometheusTSDBCompactionChunkRangeSeconds {
	return PrometheusTSDBCompactionChunkRangeSeconds{
		Histogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "prometheus_tsdb_compaction_chunk_range_seconds",
			Help:    "Final time range of chunks on their first compaction.",
			Buckets: prometheus.ExponentialBuckets(100, 4, 10),
		}),
	}
}

// PrometheusTSDBCompactionChunkSamples records the final number of samples on their first compaction.
type PrometheusTSDBCompactionChunkSamples struct {
	prometheus.Histogram
}

// NewPrometheusTSDBCompactionChunkSamples returns a new PrometheusTSDBCompactionChunkSamples instrument.
func NewPrometheusTSDBCompactionChunkSamples() PrometheusTSDBCompactionChunkSamples {
	return PrometheusTSDBCompactionChunkSamples{
		Histogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "prometheus_tsdb_compaction_chunk_samples",
			Help:    "Final number of samples on their first compaction.",
			Buckets: prometheus.ExponentialBuckets(4, 1.5, 12),
		}),
	}
}

// PrometheusTSDBCompactionChunkSizeBytes records the final size of chunks on their first compaction.
type PrometheusTSDBCompactionChunkSizeBytes struct {
	prometheus.Histogram
}

// NewPrometheusTSDBCompactionChunkSizeBytes returns a new PrometheusTSDBCompactionChunkSizeBytes instrument.
func NewPrometheusTSDBCompactionChunkSizeBytes() PrometheusTSDBCompactionChunkSizeBytes {
	return PrometheusTSDBCompactionChunkSizeBytes{
		Histogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "prometheus_tsdb_compaction_chunk_size_bytes",
			Help:    "Final size of chunks on their first compaction.",
			Buckets: prometheus.ExponentialBuckets(32, 1.5, 12),
		}),
	}
}

// PrometheusTSDBCompactionDurationSeconds records the duration of compaction runs.
type PrometheusTSDBCompactionDurationSeconds struct {
	prometheus.Histogram
}

// NewPrometheusTSDBCompactionDurationSeconds returns a new PrometheusTSDBCompactionDurationSeconds instrument.
func NewPrometheusTSDBCompactionDurationSeconds() PrometheusTSDBCompactionDurationSeconds {
	return PrometheusTSDBCompactionDurationSeconds{
		Histogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:                            "prometheus_tsdb_compaction_duration_seconds",
			Help:                            "Duration of compaction runs.",
			Buckets:                         prometheus.ExponentialBuckets(1, 2, 14),
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		}),
	}
}

// PrometheusTSDBCompactionPopulatingBlock records the set to 1 when a block is being written to the disk.
type PrometheusTSDBCompactionPopulatingBlock struct {
	prometheus.Gauge
}

// NewPrometheusTSDBCompactionPopulatingBlock returns a new PrometheusTSDBCompactionPopulatingBlock instrument.
func NewPrometheusTSDBCompactionPopulatingBlock() PrometheusTSDBCompactionPopulatingBlock {
	return PrometheusTSDBCompactionPopulatingBlock{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_compaction_populating_block",
			Help: "Set to 1 when a block is being written to the disk.",
		}),
	}
}

// PrometheusTSDBCompactionsFailedTotal records the total number of compactions that failed.
type PrometheusTSDBCompactionsFailedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBCompactionsFailedTotal returns a new PrometheusTSDBCompactionsFailedTotal instrument.
func NewPrometheusTSDBCompactionsFailedTotal() PrometheusTSDBCompactionsFailedTotal {
	return PrometheusTSDBCompactionsFailedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_compactions_failed_total",
			Help: "Total number of compactions that failed.",
		}),
	}
}

// PrometheusTSDBCompactionsSkippedTotal records the total number of skipped compactions due to overlap.
type PrometheusTSDBCompactionsSkippedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBCompactionsSkippedTotal returns a new PrometheusTSDBCompactionsSkippedTotal instrument.
func NewPrometheusTSDBCompactionsSkippedTotal() PrometheusTSDBCompactionsSkippedTotal {
	return PrometheusTSDBCompactionsSkippedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_compactions_skipped_total",
			Help: "Total number of skipped compactions due to overlap.",
		}),
	}
}

// PrometheusTSDBCompactionsTotal records the total number of compactions that were executed.
type PrometheusTSDBCompactionsTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBCompactionsTotal returns a new PrometheusTSDBCompactionsTotal instrument.
func NewPrometheusTSDBCompactionsTotal() PrometheusTSDBCompactionsTotal {
	return PrometheusTSDBCompactionsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_compactions_total",
			Help: "Total number of compactions that were executed.",
		}),
	}
}

// PrometheusTSDBCompactionsTriggeredTotal records the total number of triggered compactions.
type PrometheusTSDBCompactionsTriggeredTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBCompactionsTriggeredTotal returns a new PrometheusTSDBCompactionsTriggeredTotal instrument.
func NewPrometheusTSDBCompactionsTriggeredTotal() PrometheusTSDBCompactionsTriggeredTotal {
	return PrometheusTSDBCompactionsTriggeredTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_compactions_triggered_total",
			Help: "Total number of triggered compactions.",
		}),
	}
}

// PrometheusTSDBDataReplayDurationSeconds records the time taken to replay the data on disk.
type PrometheusTSDBDataReplayDurationSeconds struct {
	prometheus.Gauge
}

// NewPrometheusTSDBDataReplayDurationSeconds returns a new PrometheusTSDBDataReplayDurationSeconds instrument.
func NewPrometheusTSDBDataReplayDurationSeconds() PrometheusTSDBDataReplayDurationSeconds {
	return PrometheusTSDBDataReplayDurationSeconds{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_data_replay_duration_seconds",
			Help: "Time taken to replay the data on disk.",
		}),
	}
}

// PrometheusTSDBExemplarExemplarsAppendedTotal records the total number of appended exemplars.
type PrometheusTSDBExemplarExemplarsAppendedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBExemplarExemplarsAppendedTotal returns a new PrometheusTSDBExemplarExemplarsAppendedTotal instrument.
func NewPrometheusTSDBExemplarExemplarsAppendedTotal() PrometheusTSDBExemplarExemplarsAppendedTotal {
	return PrometheusTSDBExemplarExemplarsAppendedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_exemplar_exemplars_appended_total",
			Help: "Total number of appended exemplars.",
		}),
	}
}

// PrometheusTSDBExemplarExemplarsInStorage records the number of exemplars currently in circular storage.
type PrometheusTSDBExemplarExemplarsInStorage struct {
	prometheus.Gauge
}

// NewPrometheusTSDBExemplarExemplarsInStorage returns a new PrometheusTSDBExemplarExemplarsInStorage instrument.
func NewPrometheusTSDBExemplarExemplarsInStorage() PrometheusTSDBExemplarExemplarsInStorage {
	return PrometheusTSDBExemplarExemplarsInStorage{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_exemplars_in_storage",
			Help: "Number of exemplars currently in circular storage.",
		}),
	}
}

// PrometheusTSDBExemplarLastExemplarsTimestampSeconds records the timestamp of the oldest exemplar stored in circular storage.
type PrometheusTSDBExemplarLastExemplarsTimestampSeconds struct {
	prometheus.Gauge
}

// NewPrometheusTSDBExemplarLastExemplarsTimestampSeconds returns a new PrometheusTSDBExemplarLastExemplarsTimestampSeconds instrument.
func NewPrometheusTSDBExemplarLastExemplarsTimestampSeconds() PrometheusTSDBExemplarLastExemplarsTimestampSeconds {
	return PrometheusTSDBExemplarLastExemplarsTimestampSeconds{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_last_exemplars_timestamp_seconds",
			Help: "The timestamp of the oldest exemplar stored in circular storage.",
		}),
	}
}

// PrometheusTSDBExemplarMaxExemplars records the total number of exemplars the exemplar storage can store.
type PrometheusTSDBExemplarMaxExemplars struct {
	prometheus.Gauge
}

// NewPrometheusTSDBExemplarMaxExemplars returns a new PrometheusTSDBExemplarMaxExemplars instrument.
func NewPrometheusTSDBExemplarMaxExemplars() PrometheusTSDBExemplarMaxExemplars {
	return PrometheusTSDBExemplarMaxExemplars{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_max_exemplars",
			Help: "Total number of exemplars the exemplar storage can store.",
		}),
	}
}

// PrometheusTSDBExemplarOutOfOrderExemplarsTotal records the total number of out-of-order exemplar ingestion failed attempts.
type PrometheusTSDBExemplarOutOfOrderExemplarsTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBExemplarOutOfOrderExemplarsTotal returns a new PrometheusTSDBExemplarOutOfOrderExemplarsTotal instrument.
func NewPrometheusTSDBExemplarOutOfOrderExemplarsTotal() PrometheusTSDBExemplarOutOfOrderExemplarsTotal {
	return PrometheusTSDBExemplarOutOfOrderExemplarsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_exemplar_out_of_order_exemplars_total",
			Help: "Total number of out-of-order exemplar ingestion failed attempts.",
		}),
	}
}

// PrometheusTSDBExemplarSeriesWithExemplarsInStorage records the number of series with exemplars currently in circular storage.
type PrometheusTSDBExemplarSeriesWithExemplarsInStorage struct {
	prometheus.Gauge
}

// NewPrometheusTSDBExemplarSeriesWithExemplarsInStorage returns a new PrometheusTSDBExemplarSeriesWithExemplarsInStorage instrument.
func NewPrometheusTSDBExemplarSeriesWithExemplarsInStorage() PrometheusTSDBExemplarSeriesWithExemplarsInStorage {
	return PrometheusTSDBExemplarSeriesWithExemplarsInStorage{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_series_with_exemplars_in_storage",
			Help: "Number of series with exemplars currently in circular storage.",
		}),
	}
}

// PrometheusTSDBHeadActiveAppenders records the number of currently active appender transactions.
type PrometheusTSDBHeadActiveAppenders struct {
	prometheus.Gauge
}

// NewPrometheusTSDBHeadActiveAppenders returns a new PrometheusTSDBHeadActiveAppenders instrument.
func NewPrometheusTSDBHeadActiveAppenders() PrometheusTSDBHeadActiveAppenders {
	return PrometheusTSDBHeadActiveAppenders{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_active_appenders",
			Help: "Number of currently active appender transactions.",
		}),
	}
}

// PrometheusTSDBHeadChunks records the total number of chunks in the head block.
type PrometheusTSDBHeadChunks struct {
	prometheus.Gauge
}

// NewPrometheusTSDBHeadChunks returns a new PrometheusTSDBHeadChunks instrument.
func NewPrometheusTSDBHeadChunks() PrometheusTSDBHeadChunks {
	return PrometheusTSDBHeadChunks{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_chunks",
			Help: "Total number of chunks in the head block.",
		}),
	}
}

// PrometheusTSDBHeadChunksCreatedTotal records the total number of chunks created in the head block.
type PrometheusTSDBHeadChunksCreatedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBHeadChunksCreatedTotal returns a new PrometheusTSDBHeadChunksCreatedTotal instrument.
func NewPrometheusTSDBHeadChunksCreatedTotal() PrometheusTSDBHeadChunksCreatedTotal {
	return PrometheusTSDBHeadChunksCreatedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_chunks_created_total",
			Help: "Total number of chunks created in the head block.",
		}),
	}
}

// PrometheusTSDBHeadChunksRemovedTotal records the total number of chunks removed from the head block.
type PrometheusTSDBHeadChunksRemovedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBHeadChunksRemovedTotal returns a new PrometheusTSDBHeadChunksRemovedTotal instrument.
func NewPrometheusTSDBHeadChunksRemovedTotal() PrometheusTSDBHeadChunksRemovedTotal {
	return PrometheusTSDBHeadChunksRemovedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_chunks_removed_total",
			Help: "Total number of chunks removed from the head block.",
		}),
	}
}

// PrometheusTSDBHeadChunksStorageSizeBytes records the size of the chunks_head directory.
func PrometheusTSDBHeadChunksStorageSizeBytesOpts() prometheus.GaugeOpts {
	return prometheus.GaugeOpts{
		Name: "prometheus_tsdb_head_chunks_storage_size_bytes",
		Help: "Size of the chunks_head directory.",
	}
}

// PrometheusTSDBHeadGCDurationSeconds records the runtime of garbage collection in the head block.
type PrometheusTSDBHeadGCDurationSeconds struct {
	prometheus.Summary
}

// NewPrometheusTSDBHeadGCDurationSeconds returns a new PrometheusTSDBHeadGCDurationSeconds instrument.
func NewPrometheusTSDBHeadGCDurationSeconds() PrometheusTSDBHeadGCDurationSeconds {
	return PrometheusTSDBHeadGCDurationSeconds{
		Summary: prometheus.NewSummary(prometheus.SummaryOpts{
			Name:       "prometheus_tsdb_head_gc_duration_seconds",
			Help:       "Runtime of garbage collection in the head block.",
			Objectives: map[float64]float64{},
		}),
	}
}

// PrometheusTSDBHeadMaxTime records the maximum timestamp of the head block.
func PrometheusTSDBHeadMaxTimeOpts() prometheus.GaugeOpts {
	return prometheus.GaugeOpts{
		Name: "prometheus_tsdb_head_max_time",
		Help: "Maximum timestamp of the head block.",
	}
}

// PrometheusTSDBHeadMaxTimeSeconds records the maximum timestamp of the head block in seconds.
type PrometheusTSDBHeadMaxTimeSeconds struct {
	prometheus.Gauge
}

// NewPrometheusTSDBHeadMaxTimeSeconds returns a new PrometheusTSDBHeadMaxTimeSeconds instrument.
func NewPrometheusTSDBHeadMaxTimeSeconds() PrometheusTSDBHeadMaxTimeSeconds {
	return PrometheusTSDBHeadMaxTimeSeconds{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_max_time_seconds",
			Help: "Maximum timestamp of the head block in seconds.",
		}),
	}
}

// PrometheusTSDBHeadMinTime records the minimum timestamp of the head block.
func PrometheusTSDBHeadMinTimeOpts() prometheus.GaugeOpts {
	return prometheus.GaugeOpts{
		Name: "prometheus_tsdb_head_min_time",
		Help: "Minimum timestamp of the head block.",
	}
}

// PrometheusTSDBHeadMinTimeSeconds records the minimum timestamp of the head block in seconds.
type PrometheusTSDBHeadMinTimeSeconds struct {
	prometheus.Gauge
}

// NewPrometheusTSDBHeadMinTimeSeconds returns a new PrometheusTSDBHeadMinTimeSeconds instrument.
func NewPrometheusTSDBHeadMinTimeSeconds() PrometheusTSDBHeadMinTimeSeconds {
	return PrometheusTSDBHeadMinTimeSeconds{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_min_time_seconds",
			Help: "Minimum timestamp of the head block in seconds.",
		}),
	}
}

// PrometheusTSDBHeadOutOfOrderSamplesAppendedTotal records the total number of appended out-of-order samples.
type PrometheusTSDBHeadOutOfOrderSamplesAppendedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBHeadOutOfOrderSamplesAppendedTotal returns a new PrometheusTSDBHeadOutOfOrderSamplesAppendedTotal instrument.
func NewPrometheusTSDBHeadOutOfOrderSamplesAppendedTotal() PrometheusTSDBHeadOutOfOrderSamplesAppendedTotal {
	labels := []string{
		"type",
	}
	return PrometheusTSDBHeadOutOfOrderSamplesAppendedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_out_of_order_samples_appended_total",
			Help: "Total number of appended out-of-order samples.",
		}, labels),
	}
}

type PrometheusTSDBHeadOutOfOrderSamplesAppendedTotalAttr interface {
	Attribute
	implPrometheusTSDBHeadOutOfOrderSamplesAppendedTotal()
}

func (a TypeAttr) implPrometheusTSDBHeadOutOfOrderSamplesAppendedTotal() {}

func (m PrometheusTSDBHeadOutOfOrderSamplesAppendedTotal) With(
	extra ...PrometheusTSDBHeadOutOfOrderSamplesAppendedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"type": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBHeadSamplesAppendedTotal records the total number of appended samples.
type PrometheusTSDBHeadSamplesAppendedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBHeadSamplesAppendedTotal returns a new PrometheusTSDBHeadSamplesAppendedTotal instrument.
func NewPrometheusTSDBHeadSamplesAppendedTotal() PrometheusTSDBHeadSamplesAppendedTotal {
	labels := []string{
		"type",
	}
	return PrometheusTSDBHeadSamplesAppendedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_samples_appended_total",
			Help: "Total number of appended samples.",
		}, labels),
	}
}

type PrometheusTSDBHeadSamplesAppendedTotalAttr interface {
	Attribute
	implPrometheusTSDBHeadSamplesAppendedTotal()
}

func (a TypeAttr) implPrometheusTSDBHeadSamplesAppendedTotal() {}

func (m PrometheusTSDBHeadSamplesAppendedTotal) With(
	extra ...PrometheusTSDBHeadSamplesAppendedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"type": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBHeadSeries records the total number of series in the head block.
func PrometheusTSDBHeadSeriesOpts() prometheus.GaugeOpts {
	return prometheus.GaugeOpts{
		Name: "prometheus_tsdb_head_series",
		Help: "Total number of series in the head block.",
	}
}

// PrometheusTSDBHeadSeriesCreatedTotal records the total number of series created in the head block.
type PrometheusTSDBHeadSeriesCreatedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBHeadSeriesCreatedTotal returns a new PrometheusTSDBHeadSeriesCreatedTotal instrument.
func NewPrometheusTSDBHeadSeriesCreatedTotal() PrometheusTSDBHeadSeriesCreatedTotal {
	return PrometheusTSDBHeadSeriesCreatedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_series_created_total",
			Help: "Total number of series created in the head block.",
		}),
	}
}

// PrometheusTSDBHeadSeriesNotFoundTotal records the total number of requests for series that were not found.
type PrometheusTSDBHeadSeriesNotFoundTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBHeadSeriesNotFoundTotal returns a new PrometheusTSDBHeadSeriesNotFoundTotal instrument.
func NewPrometheusTSDBHeadSeriesNotFoundTotal() PrometheusTSDBHeadSeriesNotFoundTotal {
	return PrometheusTSDBHeadSeriesNotFoundTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_series_not_found_total",
			Help: "Total number of requests for series that were not found.",
		}),
	}
}

// PrometheusTSDBHeadSeriesRemovedTotal records the total number of series removed from the head block.
type PrometheusTSDBHeadSeriesRemovedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBHeadSeriesRemovedTotal returns a new PrometheusTSDBHeadSeriesRemovedTotal instrument.
func NewPrometheusTSDBHeadSeriesRemovedTotal() PrometheusTSDBHeadSeriesRemovedTotal {
	return PrometheusTSDBHeadSeriesRemovedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_series_removed_total",
			Help: "Total number of series removed from the head block.",
		}),
	}
}

// PrometheusTSDBHeadStaleSeries records the number of stale series in the head block.
func PrometheusTSDBHeadStaleSeriesOpts() prometheus.GaugeOpts {
	return prometheus.GaugeOpts{
		Name: "prometheus_tsdb_head_stale_series",
		Help: "Number of stale series in the head block.",
	}
}

// PrometheusTSDBHeadTruncationsFailedTotal records the total number of head truncations that failed.
type PrometheusTSDBHeadTruncationsFailedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBHeadTruncationsFailedTotal returns a new PrometheusTSDBHeadTruncationsFailedTotal instrument.
func NewPrometheusTSDBHeadTruncationsFailedTotal() PrometheusTSDBHeadTruncationsFailedTotal {
	return PrometheusTSDBHeadTruncationsFailedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_truncations_failed_total",
			Help: "Total number of head truncations that failed.",
		}),
	}
}

// PrometheusTSDBHeadTruncationsTotal records the total number of head truncations attempted.
type PrometheusTSDBHeadTruncationsTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBHeadTruncationsTotal returns a new PrometheusTSDBHeadTruncationsTotal instrument.
func NewPrometheusTSDBHeadTruncationsTotal() PrometheusTSDBHeadTruncationsTotal {
	return PrometheusTSDBHeadTruncationsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_truncations_total",
			Help: "Total number of head truncations attempted.",
		}),
	}
}

// PrometheusTSDBIsolationHighWatermark records the isolation high watermark.
func PrometheusTSDBIsolationHighWatermarkOpts() prometheus.GaugeOpts {
	return prometheus.GaugeOpts{
		Name: "prometheus_tsdb_isolation_high_watermark",
		Help: "The isolation high watermark.",
	}
}

// PrometheusTSDBIsolationLowWatermark records the isolation low watermark.
func PrometheusTSDBIsolationLowWatermarkOpts() prometheus.GaugeOpts {
	return prometheus.GaugeOpts{
		Name: "prometheus_tsdb_isolation_low_watermark",
		Help: "The isolation low watermark.",
	}
}

// PrometheusTSDBLowestTimestamp records the lowest timestamp value stored in the database.
func PrometheusTSDBLowestTimestampOpts() prometheus.GaugeOpts {
	return prometheus.GaugeOpts{
		Name: "prometheus_tsdb_lowest_timestamp",
		Help: "Lowest timestamp value stored in the database.",
	}
}

// PrometheusTSDBLowestTimestampSeconds records the lowest timestamp value stored in the database in seconds.
type PrometheusTSDBLowestTimestampSeconds struct {
	prometheus.Gauge
}

// NewPrometheusTSDBLowestTimestampSeconds returns a new PrometheusTSDBLowestTimestampSeconds instrument.
func NewPrometheusTSDBLowestTimestampSeconds() PrometheusTSDBLowestTimestampSeconds {
	return PrometheusTSDBLowestTimestampSeconds{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_lowest_timestamp_seconds",
			Help: "Lowest timestamp value stored in the database in seconds.",
		}),
	}
}

// PrometheusTSDBMmapChunkCorruptionsTotal records the total number of memory-mapped chunk corruptions.
type PrometheusTSDBMmapChunkCorruptionsTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBMmapChunkCorruptionsTotal returns a new PrometheusTSDBMmapChunkCorruptionsTotal instrument.
func NewPrometheusTSDBMmapChunkCorruptionsTotal() PrometheusTSDBMmapChunkCorruptionsTotal {
	return PrometheusTSDBMmapChunkCorruptionsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_mmap_chunk_corruptions_total",
			Help: "Total number of memory-mapped chunk corruptions.",
		}),
	}
}

// PrometheusTSDBMmapChunksTotal records the total number of memory-mapped chunks.
type PrometheusTSDBMmapChunksTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBMmapChunksTotal returns a new PrometheusTSDBMmapChunksTotal instrument.
func NewPrometheusTSDBMmapChunksTotal() PrometheusTSDBMmapChunksTotal {
	return PrometheusTSDBMmapChunksTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_mmap_chunks_total",
			Help: "Total number of memory-mapped chunks.",
		}),
	}
}

// PrometheusTSDBOutOfBoundSamplesTotal records the total number of out-of-bound samples ingestion failed attempts.
type PrometheusTSDBOutOfBoundSamplesTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBOutOfBoundSamplesTotal returns a new PrometheusTSDBOutOfBoundSamplesTotal instrument.
func NewPrometheusTSDBOutOfBoundSamplesTotal() PrometheusTSDBOutOfBoundSamplesTotal {
	labels := []string{
		"type",
	}
	return PrometheusTSDBOutOfBoundSamplesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_bound_samples_total",
			Help: "Total number of out-of-bound samples ingestion failed attempts.",
		}, labels),
	}
}

type PrometheusTSDBOutOfBoundSamplesTotalAttr interface {
	Attribute
	implPrometheusTSDBOutOfBoundSamplesTotal()
}

func (a TypeAttr) implPrometheusTSDBOutOfBoundSamplesTotal() {}

func (m PrometheusTSDBOutOfBoundSamplesTotal) With(
	extra ...PrometheusTSDBOutOfBoundSamplesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"type": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBOutOfOrderSamplesTotal records the total number of out-of-order samples ingestion failed attempts.
type PrometheusTSDBOutOfOrderSamplesTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBOutOfOrderSamplesTotal returns a new PrometheusTSDBOutOfOrderSamplesTotal instrument.
func NewPrometheusTSDBOutOfOrderSamplesTotal() PrometheusTSDBOutOfOrderSamplesTotal {
	labels := []string{
		"type",
	}
	return PrometheusTSDBOutOfOrderSamplesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_samples_total",
			Help: "Total number of out-of-order samples ingestion failed attempts.",
		}, labels),
	}
}

type PrometheusTSDBOutOfOrderSamplesTotalAttr interface {
	Attribute
	implPrometheusTSDBOutOfOrderSamplesTotal()
}

func (a TypeAttr) implPrometheusTSDBOutOfOrderSamplesTotal() {}

func (m PrometheusTSDBOutOfOrderSamplesTotal) With(
	extra ...PrometheusTSDBOutOfOrderSamplesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"type": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBOutOfOrderWBLCompletedPagesTotal records the total number of completed WBL pages for out-of-order samples.
type PrometheusTSDBOutOfOrderWBLCompletedPagesTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBOutOfOrderWBLCompletedPagesTotal returns a new PrometheusTSDBOutOfOrderWBLCompletedPagesTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLCompletedPagesTotal() PrometheusTSDBOutOfOrderWBLCompletedPagesTotal {
	return PrometheusTSDBOutOfOrderWBLCompletedPagesTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_completed_pages_total",
			Help: "Total number of completed WBL pages for out-of-order samples.",
		}),
	}
}

// PrometheusTSDBOutOfOrderWBLFsyncDurationSeconds records the duration of WBL fsync for out-of-order samples.
type PrometheusTSDBOutOfOrderWBLFsyncDurationSeconds struct {
	prometheus.Summary
}

// NewPrometheusTSDBOutOfOrderWBLFsyncDurationSeconds returns a new PrometheusTSDBOutOfOrderWBLFsyncDurationSeconds instrument.
func NewPrometheusTSDBOutOfOrderWBLFsyncDurationSeconds() PrometheusTSDBOutOfOrderWBLFsyncDurationSeconds {
	return PrometheusTSDBOutOfOrderWBLFsyncDurationSeconds{
		Summary: prometheus.NewSummary(prometheus.SummaryOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_fsync_duration_seconds",
			Help: "Duration of WBL fsync for out-of-order samples.",
			Objectives: map[float64]float64{
				0.5:  0.05,
				0.9:  0.01,
				0.99: 0.001,
			},
		}),
	}
}

// PrometheusTSDBOutOfOrderWBLPageFlushesTotal records the total number of WBL page flushes for out-of-order samples.
type PrometheusTSDBOutOfOrderWBLPageFlushesTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBOutOfOrderWBLPageFlushesTotal returns a new PrometheusTSDBOutOfOrderWBLPageFlushesTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLPageFlushesTotal() PrometheusTSDBOutOfOrderWBLPageFlushesTotal {
	return PrometheusTSDBOutOfOrderWBLPageFlushesTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_page_flushes_total",
			Help: "Total number of WBL page flushes for out-of-order samples.",
		}),
	}
}

// PrometheusTSDBOutOfOrderWBLRecordBytesSavedTotal records the total bytes saved by WBL record compression for out-of-order samples.
type PrometheusTSDBOutOfOrderWBLRecordBytesSavedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBOutOfOrderWBLRecordBytesSavedTotal returns a new PrometheusTSDBOutOfOrderWBLRecordBytesSavedTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLRecordBytesSavedTotal() PrometheusTSDBOutOfOrderWBLRecordBytesSavedTotal {
	labels := []string{
		"compression",
	}
	return PrometheusTSDBOutOfOrderWBLRecordBytesSavedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_record_bytes_saved_total",
			Help: "Total bytes saved by WBL record compression for out-of-order samples.",
		}, labels),
	}
}

type PrometheusTSDBOutOfOrderWBLRecordBytesSavedTotalAttr interface {
	Attribute
	implPrometheusTSDBOutOfOrderWBLRecordBytesSavedTotal()
}

func (a CompressionAttr) implPrometheusTSDBOutOfOrderWBLRecordBytesSavedTotal() {}

func (m PrometheusTSDBOutOfOrderWBLRecordBytesSavedTotal) With(
	extra ...PrometheusTSDBOutOfOrderWBLRecordBytesSavedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"compression": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBOutOfOrderWBLRecordPartWritesTotal records the total number of WBL record part writes for out-of-order samples.
type PrometheusTSDBOutOfOrderWBLRecordPartWritesTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBOutOfOrderWBLRecordPartWritesTotal returns a new PrometheusTSDBOutOfOrderWBLRecordPartWritesTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLRecordPartWritesTotal() PrometheusTSDBOutOfOrderWBLRecordPartWritesTotal {
	return PrometheusTSDBOutOfOrderWBLRecordPartWritesTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_record_part_writes_total",
			Help: "Total number of WBL record part writes for out-of-order samples.",
		}),
	}
}

// PrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal records the total bytes written to WBL record parts for out-of-order samples.
type PrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal returns a new PrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal() PrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal {
	return PrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_record_parts_bytes_written_total",
			Help: "Total bytes written to WBL record parts for out-of-order samples.",
		}),
	}
}

// PrometheusTSDBOutOfOrderWBLSegmentCurrent records the current out-of-order WBL segment.
type PrometheusTSDBOutOfOrderWBLSegmentCurrent struct {
	prometheus.Gauge
}

// NewPrometheusTSDBOutOfOrderWBLSegmentCurrent returns a new PrometheusTSDBOutOfOrderWBLSegmentCurrent instrument.
func NewPrometheusTSDBOutOfOrderWBLSegmentCurrent() PrometheusTSDBOutOfOrderWBLSegmentCurrent {
	return PrometheusTSDBOutOfOrderWBLSegmentCurrent{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_segment_current",
			Help: "Current out-of-order WBL segment.",
		}),
	}
}

// PrometheusTSDBOutOfOrderWBLStorageSizeBytes records the size of the out-of-order WBL storage.
func PrometheusTSDBOutOfOrderWBLStorageSizeBytesOpts() prometheus.GaugeOpts {
	return prometheus.GaugeOpts{
		Name: "prometheus_tsdb_out_of_order_wbl_storage_size_bytes",
		Help: "Size of the out-of-order WBL storage.",
	}
}

// PrometheusTSDBOutOfOrderWBLTruncationsFailedTotal records the total number of out-of-order WBL truncations that failed.
type PrometheusTSDBOutOfOrderWBLTruncationsFailedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBOutOfOrderWBLTruncationsFailedTotal returns a new PrometheusTSDBOutOfOrderWBLTruncationsFailedTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLTruncationsFailedTotal() PrometheusTSDBOutOfOrderWBLTruncationsFailedTotal {
	return PrometheusTSDBOutOfOrderWBLTruncationsFailedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_truncations_failed_total",
			Help: "Total number of out-of-order WBL truncations that failed.",
		}),
	}
}

// PrometheusTSDBOutOfOrderWBLTruncationsTotal records the total number of out-of-order WBL truncations.
type PrometheusTSDBOutOfOrderWBLTruncationsTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBOutOfOrderWBLTruncationsTotal returns a new PrometheusTSDBOutOfOrderWBLTruncationsTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLTruncationsTotal() PrometheusTSDBOutOfOrderWBLTruncationsTotal {
	return PrometheusTSDBOutOfOrderWBLTruncationsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_truncations_total",
			Help: "Total number of out-of-order WBL truncations.",
		}),
	}
}

// PrometheusTSDBOutOfOrderWBLWritesFailedTotal records the total number of out-of-order WBL writes that failed.
type PrometheusTSDBOutOfOrderWBLWritesFailedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBOutOfOrderWBLWritesFailedTotal returns a new PrometheusTSDBOutOfOrderWBLWritesFailedTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLWritesFailedTotal() PrometheusTSDBOutOfOrderWBLWritesFailedTotal {
	return PrometheusTSDBOutOfOrderWBLWritesFailedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_writes_failed_total",
			Help: "Total number of out-of-order WBL writes that failed.",
		}),
	}
}

// PrometheusTSDBReloadsFailuresTotal records the number of times the database reloads failed.
type PrometheusTSDBReloadsFailuresTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBReloadsFailuresTotal returns a new PrometheusTSDBReloadsFailuresTotal instrument.
func NewPrometheusTSDBReloadsFailuresTotal() PrometheusTSDBReloadsFailuresTotal {
	return PrometheusTSDBReloadsFailuresTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_reloads_failures_total",
			Help: "Number of times the database reloads failed.",
		}),
	}
}

// PrometheusTSDBReloadsTotal records the number of times the database reloads.
type PrometheusTSDBReloadsTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBReloadsTotal returns a new PrometheusTSDBReloadsTotal instrument.
func NewPrometheusTSDBReloadsTotal() PrometheusTSDBReloadsTotal {
	return PrometheusTSDBReloadsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_reloads_total",
			Help: "Number of times the database reloads.",
		}),
	}
}

// PrometheusTSDBRetentionLimitBytes records the maximum number of bytes to be retained in the TSDB.
type PrometheusTSDBRetentionLimitBytes struct {
	prometheus.Gauge
}

// NewPrometheusTSDBRetentionLimitBytes returns a new PrometheusTSDBRetentionLimitBytes instrument.
func NewPrometheusTSDBRetentionLimitBytes() PrometheusTSDBRetentionLimitBytes {
	return PrometheusTSDBRetentionLimitBytes{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_retention_limit_bytes",
			Help: "Maximum number of bytes to be retained in the TSDB.",
		}),
	}
}

// PrometheusTSDBRetentionLimitSeconds records the maximum age in seconds for samples to be retained in the TSDB.
type PrometheusTSDBRetentionLimitSeconds struct {
	prometheus.Gauge
}

// NewPrometheusTSDBRetentionLimitSeconds returns a new PrometheusTSDBRetentionLimitSeconds instrument.
func NewPrometheusTSDBRetentionLimitSeconds() PrometheusTSDBRetentionLimitSeconds {
	return PrometheusTSDBRetentionLimitSeconds{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_retention_limit_seconds",
			Help: "Maximum age in seconds for samples to be retained in the TSDB.",
		}),
	}
}

// PrometheusTSDBSampleOOODelta records the delta in seconds between the time when an out-of-order sample was ingested and the latest sample in the chunk.
type PrometheusTSDBSampleOOODelta struct {
	prometheus.Histogram
}

// NewPrometheusTSDBSampleOOODelta returns a new PrometheusTSDBSampleOOODelta instrument.
func NewPrometheusTSDBSampleOOODelta() PrometheusTSDBSampleOOODelta {
	return PrometheusTSDBSampleOOODelta{
		Histogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:                            "prometheus_tsdb_sample_ooo_delta",
			Help:                            "Delta in seconds between the time when an out-of-order sample was ingested and the latest sample in the chunk.",
			Buckets:                         []float64{600, 1800, 3600, 7200, 10800, 21600, 43200},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		}),
	}
}

// PrometheusTSDBSizeRetentionsTotal records the number of times that blocks were deleted because the maximum number of bytes was exceeded.
type PrometheusTSDBSizeRetentionsTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBSizeRetentionsTotal returns a new PrometheusTSDBSizeRetentionsTotal instrument.
func NewPrometheusTSDBSizeRetentionsTotal() PrometheusTSDBSizeRetentionsTotal {
	return PrometheusTSDBSizeRetentionsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_size_retentions_total",
			Help: "Number of times that blocks were deleted because the maximum number of bytes was exceeded.",
		}),
	}
}

// PrometheusTSDBSnapshotReplayErrorTotal records the total number of snapshot replay errors.
type PrometheusTSDBSnapshotReplayErrorTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBSnapshotReplayErrorTotal returns a new PrometheusTSDBSnapshotReplayErrorTotal instrument.
func NewPrometheusTSDBSnapshotReplayErrorTotal() PrometheusTSDBSnapshotReplayErrorTotal {
	return PrometheusTSDBSnapshotReplayErrorTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_snapshot_replay_error_total",
			Help: "Total number of snapshot replay errors.",
		}),
	}
}

// PrometheusTSDBStorageBlocksBytes records the number of bytes that are currently used for local storage by all blocks.
type PrometheusTSDBStorageBlocksBytes struct {
	prometheus.Gauge
}

// NewPrometheusTSDBStorageBlocksBytes returns a new PrometheusTSDBStorageBlocksBytes instrument.
func NewPrometheusTSDBStorageBlocksBytes() PrometheusTSDBStorageBlocksBytes {
	return PrometheusTSDBStorageBlocksBytes{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_storage_blocks_bytes",
			Help: "The number of bytes that are currently used for local storage by all blocks.",
		}),
	}
}

// PrometheusTSDBSymbolTableSizeBytes records the size of the symbol table in bytes.
func PrometheusTSDBSymbolTableSizeBytesOpts() prometheus.GaugeOpts {
	return prometheus.GaugeOpts{
		Name: "prometheus_tsdb_symbol_table_size_bytes",
		Help: "Size of the symbol table in bytes.",
	}
}

// PrometheusTSDBTimeRetentionsTotal records the number of times that blocks were deleted because the maximum time limit was exceeded.
type PrometheusTSDBTimeRetentionsTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBTimeRetentionsTotal returns a new PrometheusTSDBTimeRetentionsTotal instrument.
func NewPrometheusTSDBTimeRetentionsTotal() PrometheusTSDBTimeRetentionsTotal {
	return PrometheusTSDBTimeRetentionsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_time_retentions_total",
			Help: "Number of times that blocks were deleted because the maximum time limit was exceeded.",
		}),
	}
}

// PrometheusTSDBTombstoneCleanupSeconds records the time taken to clean up tombstones.
type PrometheusTSDBTombstoneCleanupSeconds struct {
	prometheus.Histogram
}

// NewPrometheusTSDBTombstoneCleanupSeconds returns a new PrometheusTSDBTombstoneCleanupSeconds instrument.
func NewPrometheusTSDBTombstoneCleanupSeconds() PrometheusTSDBTombstoneCleanupSeconds {
	return PrometheusTSDBTombstoneCleanupSeconds{
		Histogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:                            "prometheus_tsdb_tombstone_cleanup_seconds",
			Help:                            "Time taken to clean up tombstones.",
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		}),
	}
}

// PrometheusTSDBTooOldSamplesTotal records the total number of samples that were too old to be ingested.
type PrometheusTSDBTooOldSamplesTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBTooOldSamplesTotal returns a new PrometheusTSDBTooOldSamplesTotal instrument.
func NewPrometheusTSDBTooOldSamplesTotal() PrometheusTSDBTooOldSamplesTotal {
	labels := []string{
		"type",
	}
	return PrometheusTSDBTooOldSamplesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_too_old_samples_total",
			Help: "Total number of samples that were too old to be ingested.",
		}, labels),
	}
}

type PrometheusTSDBTooOldSamplesTotalAttr interface {
	Attribute
	implPrometheusTSDBTooOldSamplesTotal()
}

func (a TypeAttr) implPrometheusTSDBTooOldSamplesTotal() {}

func (m PrometheusTSDBTooOldSamplesTotal) With(
	extra ...PrometheusTSDBTooOldSamplesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"type": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBVerticalCompactionsTotal records the total number of compactions done on overlapping blocks.
type PrometheusTSDBVerticalCompactionsTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBVerticalCompactionsTotal returns a new PrometheusTSDBVerticalCompactionsTotal instrument.
func NewPrometheusTSDBVerticalCompactionsTotal() PrometheusTSDBVerticalCompactionsTotal {
	return PrometheusTSDBVerticalCompactionsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_vertical_compactions_total",
			Help: "Total number of compactions done on overlapping blocks.",
		}),
	}
}

// PrometheusTSDBWALCompletedPagesTotal records the total number of completed WAL pages.
type PrometheusTSDBWALCompletedPagesTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBWALCompletedPagesTotal returns a new PrometheusTSDBWALCompletedPagesTotal instrument.
func NewPrometheusTSDBWALCompletedPagesTotal() PrometheusTSDBWALCompletedPagesTotal {
	return PrometheusTSDBWALCompletedPagesTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_completed_pages_total",
			Help: "Total number of completed WAL pages.",
		}),
	}
}

// PrometheusTSDBWALCorruptionsTotal records the total number of WAL corruptions.
type PrometheusTSDBWALCorruptionsTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBWALCorruptionsTotal returns a new PrometheusTSDBWALCorruptionsTotal instrument.
func NewPrometheusTSDBWALCorruptionsTotal() PrometheusTSDBWALCorruptionsTotal {
	return PrometheusTSDBWALCorruptionsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_corruptions_total",
			Help: "Total number of WAL corruptions.",
		}),
	}
}

// PrometheusTSDBWALFsyncDurationSeconds records the duration of WAL fsync.
type PrometheusTSDBWALFsyncDurationSeconds struct {
	prometheus.Summary
}

// NewPrometheusTSDBWALFsyncDurationSeconds returns a new PrometheusTSDBWALFsyncDurationSeconds instrument.
func NewPrometheusTSDBWALFsyncDurationSeconds() PrometheusTSDBWALFsyncDurationSeconds {
	return PrometheusTSDBWALFsyncDurationSeconds{
		Summary: prometheus.NewSummary(prometheus.SummaryOpts{
			Name: "prometheus_tsdb_wal_fsync_duration_seconds",
			Help: "Duration of WAL fsync.",
			Objectives: map[float64]float64{
				0.5:  0.05,
				0.9:  0.01,
				0.99: 0.001,
			},
		}),
	}
}

// PrometheusTSDBWALPageFlushesTotal records the total number of WAL page flushes.
type PrometheusTSDBWALPageFlushesTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBWALPageFlushesTotal returns a new PrometheusTSDBWALPageFlushesTotal instrument.
func NewPrometheusTSDBWALPageFlushesTotal() PrometheusTSDBWALPageFlushesTotal {
	return PrometheusTSDBWALPageFlushesTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_page_flushes_total",
			Help: "Total number of WAL page flushes.",
		}),
	}
}

// PrometheusTSDBWALRecordBytesSavedTotal records the total bytes saved by WAL record compression.
type PrometheusTSDBWALRecordBytesSavedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBWALRecordBytesSavedTotal returns a new PrometheusTSDBWALRecordBytesSavedTotal instrument.
func NewPrometheusTSDBWALRecordBytesSavedTotal() PrometheusTSDBWALRecordBytesSavedTotal {
	labels := []string{
		"compression",
	}
	return PrometheusTSDBWALRecordBytesSavedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_record_bytes_saved_total",
			Help: "Total bytes saved by WAL record compression.",
		}, labels),
	}
}

type PrometheusTSDBWALRecordBytesSavedTotalAttr interface {
	Attribute
	implPrometheusTSDBWALRecordBytesSavedTotal()
}

func (a CompressionAttr) implPrometheusTSDBWALRecordBytesSavedTotal() {}

func (m PrometheusTSDBWALRecordBytesSavedTotal) With(
	extra ...PrometheusTSDBWALRecordBytesSavedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"compression": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBWALRecordPartWritesTotal records the total number of WAL record part writes.
type PrometheusTSDBWALRecordPartWritesTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBWALRecordPartWritesTotal returns a new PrometheusTSDBWALRecordPartWritesTotal instrument.
func NewPrometheusTSDBWALRecordPartWritesTotal() PrometheusTSDBWALRecordPartWritesTotal {
	return PrometheusTSDBWALRecordPartWritesTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_record_part_writes_total",
			Help: "Total number of WAL record part writes.",
		}),
	}
}

// PrometheusTSDBWALRecordPartsBytesWrittenTotal records the total bytes written to WAL record parts.
type PrometheusTSDBWALRecordPartsBytesWrittenTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBWALRecordPartsBytesWrittenTotal returns a new PrometheusTSDBWALRecordPartsBytesWrittenTotal instrument.
func NewPrometheusTSDBWALRecordPartsBytesWrittenTotal() PrometheusTSDBWALRecordPartsBytesWrittenTotal {
	return PrometheusTSDBWALRecordPartsBytesWrittenTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_record_parts_bytes_written_total",
			Help: "Total bytes written to WAL record parts.",
		}),
	}
}

// PrometheusTSDBWALReplayUnknownRefsTotal records the total number of unknown series references encountered during WAL replay.
type PrometheusTSDBWALReplayUnknownRefsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBWALReplayUnknownRefsTotal returns a new PrometheusTSDBWALReplayUnknownRefsTotal instrument.
func NewPrometheusTSDBWALReplayUnknownRefsTotal() PrometheusTSDBWALReplayUnknownRefsTotal {
	labels := []string{
		"type",
	}
	return PrometheusTSDBWALReplayUnknownRefsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_replay_unknown_refs_total",
			Help: "Total number of unknown series references encountered during WAL replay.",
		}, labels),
	}
}

type PrometheusTSDBWALReplayUnknownRefsTotalAttr interface {
	Attribute
	implPrometheusTSDBWALReplayUnknownRefsTotal()
}

func (a TypeAttr) implPrometheusTSDBWALReplayUnknownRefsTotal() {}

func (m PrometheusTSDBWALReplayUnknownRefsTotal) With(
	extra ...PrometheusTSDBWALReplayUnknownRefsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"type": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBWALSegmentCurrent records the current WAL segment.
type PrometheusTSDBWALSegmentCurrent struct {
	prometheus.Gauge
}

// NewPrometheusTSDBWALSegmentCurrent returns a new PrometheusTSDBWALSegmentCurrent instrument.
func NewPrometheusTSDBWALSegmentCurrent() PrometheusTSDBWALSegmentCurrent {
	return PrometheusTSDBWALSegmentCurrent{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_wal_segment_current",
			Help: "Current WAL segment.",
		}),
	}
}

// PrometheusTSDBWALStorageSizeBytes records the size of the WAL storage.
func PrometheusTSDBWALStorageSizeBytesOpts() prometheus.GaugeOpts {
	return prometheus.GaugeOpts{
		Name: "prometheus_tsdb_wal_storage_size_bytes",
		Help: "Size of the WAL storage.",
	}
}

// PrometheusTSDBWALTruncateDurationSeconds records the duration of WAL truncation.
type PrometheusTSDBWALTruncateDurationSeconds struct {
	prometheus.Summary
}

// NewPrometheusTSDBWALTruncateDurationSeconds returns a new PrometheusTSDBWALTruncateDurationSeconds instrument.
func NewPrometheusTSDBWALTruncateDurationSeconds() PrometheusTSDBWALTruncateDurationSeconds {
	return PrometheusTSDBWALTruncateDurationSeconds{
		Summary: prometheus.NewSummary(prometheus.SummaryOpts{
			Name:       "prometheus_tsdb_wal_truncate_duration_seconds",
			Help:       "Duration of WAL truncation.",
			Objectives: map[float64]float64{},
		}),
	}
}

// PrometheusTSDBWALTruncationsFailedTotal records the total number of WAL truncations that failed.
type PrometheusTSDBWALTruncationsFailedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBWALTruncationsFailedTotal returns a new PrometheusTSDBWALTruncationsFailedTotal instrument.
func NewPrometheusTSDBWALTruncationsFailedTotal() PrometheusTSDBWALTruncationsFailedTotal {
	return PrometheusTSDBWALTruncationsFailedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_truncations_failed_total",
			Help: "Total number of WAL truncations that failed.",
		}),
	}
}

// PrometheusTSDBWALTruncationsTotal records the total number of WAL truncations.
type PrometheusTSDBWALTruncationsTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBWALTruncationsTotal returns a new PrometheusTSDBWALTruncationsTotal instrument.
func NewPrometheusTSDBWALTruncationsTotal() PrometheusTSDBWALTruncationsTotal {
	return PrometheusTSDBWALTruncationsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_truncations_total",
			Help: "Total number of WAL truncations.",
		}),
	}
}

// PrometheusTSDBWALWritesFailedTotal records the total number of WAL writes that failed.
type PrometheusTSDBWALWritesFailedTotal struct {
	prometheus.Counter
}

// NewPrometheusTSDBWALWritesFailedTotal returns a new PrometheusTSDBWALWritesFailedTotal instrument.
func NewPrometheusTSDBWALWritesFailedTotal() PrometheusTSDBWALWritesFailedTotal {
	return PrometheusTSDBWALWritesFailedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_writes_failed_total",
			Help: "Total number of WAL writes that failed.",
		}),
	}
}

// PrometheusTSDBWBLReplayUnknownRefsTotal records the total number of unknown series references encountered during WBL replay.
type PrometheusTSDBWBLReplayUnknownRefsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBWBLReplayUnknownRefsTotal returns a new PrometheusTSDBWBLReplayUnknownRefsTotal instrument.
func NewPrometheusTSDBWBLReplayUnknownRefsTotal() PrometheusTSDBWBLReplayUnknownRefsTotal {
	labels := []string{
		"type",
	}
	return PrometheusTSDBWBLReplayUnknownRefsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wbl_replay_unknown_refs_total",
			Help: "Total number of unknown series references encountered during WBL replay.",
		}, labels),
	}
}

type PrometheusTSDBWBLReplayUnknownRefsTotalAttr interface {
	Attribute
	implPrometheusTSDBWBLReplayUnknownRefsTotal()
}

func (a TypeAttr) implPrometheusTSDBWBLReplayUnknownRefsTotal() {}

func (m PrometheusTSDBWBLReplayUnknownRefsTotal) With(
	extra ...PrometheusTSDBWBLReplayUnknownRefsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"type": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}
