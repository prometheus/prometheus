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
type PrometheusTSDBBlocksLoaded struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBBlocksLoaded returns a new PrometheusTSDBBlocksLoaded instrument.
func NewPrometheusTSDBBlocksLoaded() PrometheusTSDBBlocksLoaded {
	labels := []string{}
	return PrometheusTSDBBlocksLoaded{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_blocks_loaded",
			Help: "Number of currently loaded data blocks.",
		}, labels),
	}
}

type PrometheusTSDBBlocksLoadedAttr interface {
	Attribute
	implPrometheusTSDBBlocksLoaded()
}

func (m PrometheusTSDBBlocksLoaded) With(
	extra ...PrometheusTSDBBlocksLoadedAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBCheckpointCreationsFailedTotal records the total number of checkpoint creations that failed.
type PrometheusTSDBCheckpointCreationsFailedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBCheckpointCreationsFailedTotal returns a new PrometheusTSDBCheckpointCreationsFailedTotal instrument.
func NewPrometheusTSDBCheckpointCreationsFailedTotal() PrometheusTSDBCheckpointCreationsFailedTotal {
	labels := []string{}
	return PrometheusTSDBCheckpointCreationsFailedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_checkpoint_creations_failed_total",
			Help: "Total number of checkpoint creations that failed.",
		}, labels),
	}
}

type PrometheusTSDBCheckpointCreationsFailedTotalAttr interface {
	Attribute
	implPrometheusTSDBCheckpointCreationsFailedTotal()
}

func (m PrometheusTSDBCheckpointCreationsFailedTotal) With(
	extra ...PrometheusTSDBCheckpointCreationsFailedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBCheckpointCreationsTotal records the total number of checkpoint creations attempted.
type PrometheusTSDBCheckpointCreationsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBCheckpointCreationsTotal returns a new PrometheusTSDBCheckpointCreationsTotal instrument.
func NewPrometheusTSDBCheckpointCreationsTotal() PrometheusTSDBCheckpointCreationsTotal {
	labels := []string{}
	return PrometheusTSDBCheckpointCreationsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_checkpoint_creations_total",
			Help: "Total number of checkpoint creations attempted.",
		}, labels),
	}
}

type PrometheusTSDBCheckpointCreationsTotalAttr interface {
	Attribute
	implPrometheusTSDBCheckpointCreationsTotal()
}

func (m PrometheusTSDBCheckpointCreationsTotal) With(
	extra ...PrometheusTSDBCheckpointCreationsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBCheckpointDeletionsFailedTotal records the total number of checkpoint deletions that failed.
type PrometheusTSDBCheckpointDeletionsFailedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBCheckpointDeletionsFailedTotal returns a new PrometheusTSDBCheckpointDeletionsFailedTotal instrument.
func NewPrometheusTSDBCheckpointDeletionsFailedTotal() PrometheusTSDBCheckpointDeletionsFailedTotal {
	labels := []string{}
	return PrometheusTSDBCheckpointDeletionsFailedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_checkpoint_deletions_failed_total",
			Help: "Total number of checkpoint deletions that failed.",
		}, labels),
	}
}

type PrometheusTSDBCheckpointDeletionsFailedTotalAttr interface {
	Attribute
	implPrometheusTSDBCheckpointDeletionsFailedTotal()
}

func (m PrometheusTSDBCheckpointDeletionsFailedTotal) With(
	extra ...PrometheusTSDBCheckpointDeletionsFailedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBCheckpointDeletionsTotal records the total number of checkpoint deletions attempted.
type PrometheusTSDBCheckpointDeletionsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBCheckpointDeletionsTotal returns a new PrometheusTSDBCheckpointDeletionsTotal instrument.
func NewPrometheusTSDBCheckpointDeletionsTotal() PrometheusTSDBCheckpointDeletionsTotal {
	labels := []string{}
	return PrometheusTSDBCheckpointDeletionsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_checkpoint_deletions_total",
			Help: "Total number of checkpoint deletions attempted.",
		}, labels),
	}
}

type PrometheusTSDBCheckpointDeletionsTotalAttr interface {
	Attribute
	implPrometheusTSDBCheckpointDeletionsTotal()
}

func (m PrometheusTSDBCheckpointDeletionsTotal) With(
	extra ...PrometheusTSDBCheckpointDeletionsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBCleanStart records the set to 1 if the TSDB was clean at startup, 0 otherwise.
type PrometheusTSDBCleanStart struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBCleanStart returns a new PrometheusTSDBCleanStart instrument.
func NewPrometheusTSDBCleanStart() PrometheusTSDBCleanStart {
	labels := []string{}
	return PrometheusTSDBCleanStart{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_clean_start",
			Help: "Set to 1 if the TSDB was clean at startup, 0 otherwise.",
		}, labels),
	}
}

type PrometheusTSDBCleanStartAttr interface {
	Attribute
	implPrometheusTSDBCleanStart()
}

func (m PrometheusTSDBCleanStart) With(
	extra ...PrometheusTSDBCleanStartAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBCompactionChunkRangeSeconds records the final time range of chunks on their first compaction.
type PrometheusTSDBCompactionChunkRangeSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusTSDBCompactionChunkRangeSeconds returns a new PrometheusTSDBCompactionChunkRangeSeconds instrument.
func NewPrometheusTSDBCompactionChunkRangeSeconds() PrometheusTSDBCompactionChunkRangeSeconds {
	labels := []string{}
	return PrometheusTSDBCompactionChunkRangeSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_tsdb_compaction_chunk_range_seconds",
			Help: "Final time range of chunks on their first compaction.",
		}, labels),
	}
}

type PrometheusTSDBCompactionChunkRangeSecondsAttr interface {
	Attribute
	implPrometheusTSDBCompactionChunkRangeSeconds()
}

func (m PrometheusTSDBCompactionChunkRangeSeconds) With(
	extra ...PrometheusTSDBCompactionChunkRangeSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusTSDBCompactionChunkSamples records the final number of samples on their first compaction.
type PrometheusTSDBCompactionChunkSamples struct {
	*prometheus.HistogramVec
}

// NewPrometheusTSDBCompactionChunkSamples returns a new PrometheusTSDBCompactionChunkSamples instrument.
func NewPrometheusTSDBCompactionChunkSamples() PrometheusTSDBCompactionChunkSamples {
	labels := []string{}
	return PrometheusTSDBCompactionChunkSamples{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_tsdb_compaction_chunk_samples",
			Help: "Final number of samples on their first compaction.",
		}, labels),
	}
}

type PrometheusTSDBCompactionChunkSamplesAttr interface {
	Attribute
	implPrometheusTSDBCompactionChunkSamples()
}

func (m PrometheusTSDBCompactionChunkSamples) With(
	extra ...PrometheusTSDBCompactionChunkSamplesAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusTSDBCompactionChunkSizeBytes records the final size of chunks on their first compaction.
type PrometheusTSDBCompactionChunkSizeBytes struct {
	*prometheus.HistogramVec
}

// NewPrometheusTSDBCompactionChunkSizeBytes returns a new PrometheusTSDBCompactionChunkSizeBytes instrument.
func NewPrometheusTSDBCompactionChunkSizeBytes() PrometheusTSDBCompactionChunkSizeBytes {
	labels := []string{}
	return PrometheusTSDBCompactionChunkSizeBytes{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_tsdb_compaction_chunk_size_bytes",
			Help: "Final size of chunks on their first compaction.",
		}, labels),
	}
}

type PrometheusTSDBCompactionChunkSizeBytesAttr interface {
	Attribute
	implPrometheusTSDBCompactionChunkSizeBytes()
}

func (m PrometheusTSDBCompactionChunkSizeBytes) With(
	extra ...PrometheusTSDBCompactionChunkSizeBytesAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusTSDBCompactionDurationSeconds records the duration of compaction runs.
type PrometheusTSDBCompactionDurationSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusTSDBCompactionDurationSeconds returns a new PrometheusTSDBCompactionDurationSeconds instrument.
func NewPrometheusTSDBCompactionDurationSeconds() PrometheusTSDBCompactionDurationSeconds {
	labels := []string{}
	return PrometheusTSDBCompactionDurationSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_tsdb_compaction_duration_seconds",
			Help: "Duration of compaction runs.",
		}, labels),
	}
}

type PrometheusTSDBCompactionDurationSecondsAttr interface {
	Attribute
	implPrometheusTSDBCompactionDurationSeconds()
}

func (m PrometheusTSDBCompactionDurationSeconds) With(
	extra ...PrometheusTSDBCompactionDurationSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusTSDBCompactionPopulatingBlock records the set to 1 when a block is being written to the disk.
type PrometheusTSDBCompactionPopulatingBlock struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBCompactionPopulatingBlock returns a new PrometheusTSDBCompactionPopulatingBlock instrument.
func NewPrometheusTSDBCompactionPopulatingBlock() PrometheusTSDBCompactionPopulatingBlock {
	labels := []string{}
	return PrometheusTSDBCompactionPopulatingBlock{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_compaction_populating_block",
			Help: "Set to 1 when a block is being written to the disk.",
		}, labels),
	}
}

type PrometheusTSDBCompactionPopulatingBlockAttr interface {
	Attribute
	implPrometheusTSDBCompactionPopulatingBlock()
}

func (m PrometheusTSDBCompactionPopulatingBlock) With(
	extra ...PrometheusTSDBCompactionPopulatingBlockAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBCompactionsFailedTotal records the total number of compactions that failed.
type PrometheusTSDBCompactionsFailedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBCompactionsFailedTotal returns a new PrometheusTSDBCompactionsFailedTotal instrument.
func NewPrometheusTSDBCompactionsFailedTotal() PrometheusTSDBCompactionsFailedTotal {
	labels := []string{}
	return PrometheusTSDBCompactionsFailedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_compactions_failed_total",
			Help: "Total number of compactions that failed.",
		}, labels),
	}
}

type PrometheusTSDBCompactionsFailedTotalAttr interface {
	Attribute
	implPrometheusTSDBCompactionsFailedTotal()
}

func (m PrometheusTSDBCompactionsFailedTotal) With(
	extra ...PrometheusTSDBCompactionsFailedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBCompactionsSkippedTotal records the total number of skipped compactions due to overlap.
type PrometheusTSDBCompactionsSkippedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBCompactionsSkippedTotal returns a new PrometheusTSDBCompactionsSkippedTotal instrument.
func NewPrometheusTSDBCompactionsSkippedTotal() PrometheusTSDBCompactionsSkippedTotal {
	labels := []string{}
	return PrometheusTSDBCompactionsSkippedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_compactions_skipped_total",
			Help: "Total number of skipped compactions due to overlap.",
		}, labels),
	}
}

type PrometheusTSDBCompactionsSkippedTotalAttr interface {
	Attribute
	implPrometheusTSDBCompactionsSkippedTotal()
}

func (m PrometheusTSDBCompactionsSkippedTotal) With(
	extra ...PrometheusTSDBCompactionsSkippedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBCompactionsTotal records the total number of compactions that were executed.
type PrometheusTSDBCompactionsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBCompactionsTotal returns a new PrometheusTSDBCompactionsTotal instrument.
func NewPrometheusTSDBCompactionsTotal() PrometheusTSDBCompactionsTotal {
	labels := []string{}
	return PrometheusTSDBCompactionsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_compactions_total",
			Help: "Total number of compactions that were executed.",
		}, labels),
	}
}

type PrometheusTSDBCompactionsTotalAttr interface {
	Attribute
	implPrometheusTSDBCompactionsTotal()
}

func (m PrometheusTSDBCompactionsTotal) With(
	extra ...PrometheusTSDBCompactionsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBCompactionsTriggeredTotal records the total number of triggered compactions.
type PrometheusTSDBCompactionsTriggeredTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBCompactionsTriggeredTotal returns a new PrometheusTSDBCompactionsTriggeredTotal instrument.
func NewPrometheusTSDBCompactionsTriggeredTotal() PrometheusTSDBCompactionsTriggeredTotal {
	labels := []string{}
	return PrometheusTSDBCompactionsTriggeredTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_compactions_triggered_total",
			Help: "Total number of triggered compactions.",
		}, labels),
	}
}

type PrometheusTSDBCompactionsTriggeredTotalAttr interface {
	Attribute
	implPrometheusTSDBCompactionsTriggeredTotal()
}

func (m PrometheusTSDBCompactionsTriggeredTotal) With(
	extra ...PrometheusTSDBCompactionsTriggeredTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBDataReplayDurationSeconds records the time taken to replay the data on disk.
type PrometheusTSDBDataReplayDurationSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBDataReplayDurationSeconds returns a new PrometheusTSDBDataReplayDurationSeconds instrument.
func NewPrometheusTSDBDataReplayDurationSeconds() PrometheusTSDBDataReplayDurationSeconds {
	labels := []string{}
	return PrometheusTSDBDataReplayDurationSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_data_replay_duration_seconds",
			Help: "Time taken to replay the data on disk.",
		}, labels),
	}
}

type PrometheusTSDBDataReplayDurationSecondsAttr interface {
	Attribute
	implPrometheusTSDBDataReplayDurationSeconds()
}

func (m PrometheusTSDBDataReplayDurationSeconds) With(
	extra ...PrometheusTSDBDataReplayDurationSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBExemplarExemplarsAppendedTotal records the total number of appended exemplars.
type PrometheusTSDBExemplarExemplarsAppendedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBExemplarExemplarsAppendedTotal returns a new PrometheusTSDBExemplarExemplarsAppendedTotal instrument.
func NewPrometheusTSDBExemplarExemplarsAppendedTotal() PrometheusTSDBExemplarExemplarsAppendedTotal {
	labels := []string{}
	return PrometheusTSDBExemplarExemplarsAppendedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_exemplar_exemplars_appended_total",
			Help: "Total number of appended exemplars.",
		}, labels),
	}
}

type PrometheusTSDBExemplarExemplarsAppendedTotalAttr interface {
	Attribute
	implPrometheusTSDBExemplarExemplarsAppendedTotal()
}

func (m PrometheusTSDBExemplarExemplarsAppendedTotal) With(
	extra ...PrometheusTSDBExemplarExemplarsAppendedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBExemplarExemplarsInStorage records the number of exemplars currently in circular storage.
type PrometheusTSDBExemplarExemplarsInStorage struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBExemplarExemplarsInStorage returns a new PrometheusTSDBExemplarExemplarsInStorage instrument.
func NewPrometheusTSDBExemplarExemplarsInStorage() PrometheusTSDBExemplarExemplarsInStorage {
	labels := []string{}
	return PrometheusTSDBExemplarExemplarsInStorage{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_exemplars_in_storage",
			Help: "Number of exemplars currently in circular storage.",
		}, labels),
	}
}

type PrometheusTSDBExemplarExemplarsInStorageAttr interface {
	Attribute
	implPrometheusTSDBExemplarExemplarsInStorage()
}

func (m PrometheusTSDBExemplarExemplarsInStorage) With(
	extra ...PrometheusTSDBExemplarExemplarsInStorageAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBExemplarLastExemplarsTimestampSeconds records the timestamp of the oldest exemplar stored in circular storage.
type PrometheusTSDBExemplarLastExemplarsTimestampSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBExemplarLastExemplarsTimestampSeconds returns a new PrometheusTSDBExemplarLastExemplarsTimestampSeconds instrument.
func NewPrometheusTSDBExemplarLastExemplarsTimestampSeconds() PrometheusTSDBExemplarLastExemplarsTimestampSeconds {
	labels := []string{}
	return PrometheusTSDBExemplarLastExemplarsTimestampSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_last_exemplars_timestamp_seconds",
			Help: "The timestamp of the oldest exemplar stored in circular storage.",
		}, labels),
	}
}

type PrometheusTSDBExemplarLastExemplarsTimestampSecondsAttr interface {
	Attribute
	implPrometheusTSDBExemplarLastExemplarsTimestampSeconds()
}

func (m PrometheusTSDBExemplarLastExemplarsTimestampSeconds) With(
	extra ...PrometheusTSDBExemplarLastExemplarsTimestampSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBExemplarMaxExemplars records the total number of exemplars the exemplar storage can store.
type PrometheusTSDBExemplarMaxExemplars struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBExemplarMaxExemplars returns a new PrometheusTSDBExemplarMaxExemplars instrument.
func NewPrometheusTSDBExemplarMaxExemplars() PrometheusTSDBExemplarMaxExemplars {
	labels := []string{}
	return PrometheusTSDBExemplarMaxExemplars{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_max_exemplars",
			Help: "Total number of exemplars the exemplar storage can store.",
		}, labels),
	}
}

type PrometheusTSDBExemplarMaxExemplarsAttr interface {
	Attribute
	implPrometheusTSDBExemplarMaxExemplars()
}

func (m PrometheusTSDBExemplarMaxExemplars) With(
	extra ...PrometheusTSDBExemplarMaxExemplarsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBExemplarOutOfOrderExemplarsTotal records the total number of out-of-order exemplar ingestion failed attempts.
type PrometheusTSDBExemplarOutOfOrderExemplarsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBExemplarOutOfOrderExemplarsTotal returns a new PrometheusTSDBExemplarOutOfOrderExemplarsTotal instrument.
func NewPrometheusTSDBExemplarOutOfOrderExemplarsTotal() PrometheusTSDBExemplarOutOfOrderExemplarsTotal {
	labels := []string{}
	return PrometheusTSDBExemplarOutOfOrderExemplarsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_exemplar_out_of_order_exemplars_total",
			Help: "Total number of out-of-order exemplar ingestion failed attempts.",
		}, labels),
	}
}

type PrometheusTSDBExemplarOutOfOrderExemplarsTotalAttr interface {
	Attribute
	implPrometheusTSDBExemplarOutOfOrderExemplarsTotal()
}

func (m PrometheusTSDBExemplarOutOfOrderExemplarsTotal) With(
	extra ...PrometheusTSDBExemplarOutOfOrderExemplarsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBExemplarSeriesWithExemplarsInStorage records the number of series with exemplars currently in circular storage.
type PrometheusTSDBExemplarSeriesWithExemplarsInStorage struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBExemplarSeriesWithExemplarsInStorage returns a new PrometheusTSDBExemplarSeriesWithExemplarsInStorage instrument.
func NewPrometheusTSDBExemplarSeriesWithExemplarsInStorage() PrometheusTSDBExemplarSeriesWithExemplarsInStorage {
	labels := []string{}
	return PrometheusTSDBExemplarSeriesWithExemplarsInStorage{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_exemplar_series_with_exemplars_in_storage",
			Help: "Number of series with exemplars currently in circular storage.",
		}, labels),
	}
}

type PrometheusTSDBExemplarSeriesWithExemplarsInStorageAttr interface {
	Attribute
	implPrometheusTSDBExemplarSeriesWithExemplarsInStorage()
}

func (m PrometheusTSDBExemplarSeriesWithExemplarsInStorage) With(
	extra ...PrometheusTSDBExemplarSeriesWithExemplarsInStorageAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBHeadActiveAppenders records the number of currently active appender transactions.
type PrometheusTSDBHeadActiveAppenders struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBHeadActiveAppenders returns a new PrometheusTSDBHeadActiveAppenders instrument.
func NewPrometheusTSDBHeadActiveAppenders() PrometheusTSDBHeadActiveAppenders {
	labels := []string{}
	return PrometheusTSDBHeadActiveAppenders{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_active_appenders",
			Help: "Number of currently active appender transactions.",
		}, labels),
	}
}

type PrometheusTSDBHeadActiveAppendersAttr interface {
	Attribute
	implPrometheusTSDBHeadActiveAppenders()
}

func (m PrometheusTSDBHeadActiveAppenders) With(
	extra ...PrometheusTSDBHeadActiveAppendersAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBHeadChunks records the total number of chunks in the head block.
type PrometheusTSDBHeadChunks struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBHeadChunks returns a new PrometheusTSDBHeadChunks instrument.
func NewPrometheusTSDBHeadChunks() PrometheusTSDBHeadChunks {
	labels := []string{}
	return PrometheusTSDBHeadChunks{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_chunks",
			Help: "Total number of chunks in the head block.",
		}, labels),
	}
}

type PrometheusTSDBHeadChunksAttr interface {
	Attribute
	implPrometheusTSDBHeadChunks()
}

func (m PrometheusTSDBHeadChunks) With(
	extra ...PrometheusTSDBHeadChunksAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBHeadChunksCreatedTotal records the total number of chunks created in the head block.
type PrometheusTSDBHeadChunksCreatedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBHeadChunksCreatedTotal returns a new PrometheusTSDBHeadChunksCreatedTotal instrument.
func NewPrometheusTSDBHeadChunksCreatedTotal() PrometheusTSDBHeadChunksCreatedTotal {
	labels := []string{}
	return PrometheusTSDBHeadChunksCreatedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_chunks_created_total",
			Help: "Total number of chunks created in the head block.",
		}, labels),
	}
}

type PrometheusTSDBHeadChunksCreatedTotalAttr interface {
	Attribute
	implPrometheusTSDBHeadChunksCreatedTotal()
}

func (m PrometheusTSDBHeadChunksCreatedTotal) With(
	extra ...PrometheusTSDBHeadChunksCreatedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBHeadChunksRemovedTotal records the total number of chunks removed from the head block.
type PrometheusTSDBHeadChunksRemovedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBHeadChunksRemovedTotal returns a new PrometheusTSDBHeadChunksRemovedTotal instrument.
func NewPrometheusTSDBHeadChunksRemovedTotal() PrometheusTSDBHeadChunksRemovedTotal {
	labels := []string{}
	return PrometheusTSDBHeadChunksRemovedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_chunks_removed_total",
			Help: "Total number of chunks removed from the head block.",
		}, labels),
	}
}

type PrometheusTSDBHeadChunksRemovedTotalAttr interface {
	Attribute
	implPrometheusTSDBHeadChunksRemovedTotal()
}

func (m PrometheusTSDBHeadChunksRemovedTotal) With(
	extra ...PrometheusTSDBHeadChunksRemovedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBHeadChunksStorageSizeBytes records the size of the chunks_head directory.
type PrometheusTSDBHeadChunksStorageSizeBytes struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBHeadChunksStorageSizeBytes returns a new PrometheusTSDBHeadChunksStorageSizeBytes instrument.
func NewPrometheusTSDBHeadChunksStorageSizeBytes() PrometheusTSDBHeadChunksStorageSizeBytes {
	labels := []string{}
	return PrometheusTSDBHeadChunksStorageSizeBytes{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_chunks_storage_size_bytes",
			Help: "Size of the chunks_head directory.",
		}, labels),
	}
}

type PrometheusTSDBHeadChunksStorageSizeBytesAttr interface {
	Attribute
	implPrometheusTSDBHeadChunksStorageSizeBytes()
}

func (m PrometheusTSDBHeadChunksStorageSizeBytes) With(
	extra ...PrometheusTSDBHeadChunksStorageSizeBytesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBHeadGCDurationSeconds records the runtime of garbage collection in the head block.
type PrometheusTSDBHeadGCDurationSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusTSDBHeadGCDurationSeconds returns a new PrometheusTSDBHeadGCDurationSeconds instrument.
func NewPrometheusTSDBHeadGCDurationSeconds() PrometheusTSDBHeadGCDurationSeconds {
	labels := []string{}
	return PrometheusTSDBHeadGCDurationSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_tsdb_head_gc_duration_seconds",
			Help: "Runtime of garbage collection in the head block.",
		}, labels),
	}
}

type PrometheusTSDBHeadGCDurationSecondsAttr interface {
	Attribute
	implPrometheusTSDBHeadGCDurationSeconds()
}

func (m PrometheusTSDBHeadGCDurationSeconds) With(
	extra ...PrometheusTSDBHeadGCDurationSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusTSDBHeadMaxTime records the maximum timestamp of the head block.
type PrometheusTSDBHeadMaxTime struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBHeadMaxTime returns a new PrometheusTSDBHeadMaxTime instrument.
func NewPrometheusTSDBHeadMaxTime() PrometheusTSDBHeadMaxTime {
	labels := []string{}
	return PrometheusTSDBHeadMaxTime{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_max_time",
			Help: "Maximum timestamp of the head block.",
		}, labels),
	}
}

type PrometheusTSDBHeadMaxTimeAttr interface {
	Attribute
	implPrometheusTSDBHeadMaxTime()
}

func (m PrometheusTSDBHeadMaxTime) With(
	extra ...PrometheusTSDBHeadMaxTimeAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBHeadMaxTimeSeconds records the maximum timestamp of the head block in seconds.
type PrometheusTSDBHeadMaxTimeSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBHeadMaxTimeSeconds returns a new PrometheusTSDBHeadMaxTimeSeconds instrument.
func NewPrometheusTSDBHeadMaxTimeSeconds() PrometheusTSDBHeadMaxTimeSeconds {
	labels := []string{}
	return PrometheusTSDBHeadMaxTimeSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_max_time_seconds",
			Help: "Maximum timestamp of the head block in seconds.",
		}, labels),
	}
}

type PrometheusTSDBHeadMaxTimeSecondsAttr interface {
	Attribute
	implPrometheusTSDBHeadMaxTimeSeconds()
}

func (m PrometheusTSDBHeadMaxTimeSeconds) With(
	extra ...PrometheusTSDBHeadMaxTimeSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBHeadMinTime records the minimum timestamp of the head block.
type PrometheusTSDBHeadMinTime struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBHeadMinTime returns a new PrometheusTSDBHeadMinTime instrument.
func NewPrometheusTSDBHeadMinTime() PrometheusTSDBHeadMinTime {
	labels := []string{}
	return PrometheusTSDBHeadMinTime{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_min_time",
			Help: "Minimum timestamp of the head block.",
		}, labels),
	}
}

type PrometheusTSDBHeadMinTimeAttr interface {
	Attribute
	implPrometheusTSDBHeadMinTime()
}

func (m PrometheusTSDBHeadMinTime) With(
	extra ...PrometheusTSDBHeadMinTimeAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBHeadMinTimeSeconds records the minimum timestamp of the head block in seconds.
type PrometheusTSDBHeadMinTimeSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBHeadMinTimeSeconds returns a new PrometheusTSDBHeadMinTimeSeconds instrument.
func NewPrometheusTSDBHeadMinTimeSeconds() PrometheusTSDBHeadMinTimeSeconds {
	labels := []string{}
	return PrometheusTSDBHeadMinTimeSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_min_time_seconds",
			Help: "Minimum timestamp of the head block in seconds.",
		}, labels),
	}
}

type PrometheusTSDBHeadMinTimeSecondsAttr interface {
	Attribute
	implPrometheusTSDBHeadMinTimeSeconds()
}

func (m PrometheusTSDBHeadMinTimeSeconds) With(
	extra ...PrometheusTSDBHeadMinTimeSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
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
type PrometheusTSDBHeadSeries struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBHeadSeries returns a new PrometheusTSDBHeadSeries instrument.
func NewPrometheusTSDBHeadSeries() PrometheusTSDBHeadSeries {
	labels := []string{}
	return PrometheusTSDBHeadSeries{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_series",
			Help: "Total number of series in the head block.",
		}, labels),
	}
}

type PrometheusTSDBHeadSeriesAttr interface {
	Attribute
	implPrometheusTSDBHeadSeries()
}

func (m PrometheusTSDBHeadSeries) With(
	extra ...PrometheusTSDBHeadSeriesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBHeadSeriesCreatedTotal records the total number of series created in the head block.
type PrometheusTSDBHeadSeriesCreatedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBHeadSeriesCreatedTotal returns a new PrometheusTSDBHeadSeriesCreatedTotal instrument.
func NewPrometheusTSDBHeadSeriesCreatedTotal() PrometheusTSDBHeadSeriesCreatedTotal {
	labels := []string{}
	return PrometheusTSDBHeadSeriesCreatedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_series_created_total",
			Help: "Total number of series created in the head block.",
		}, labels),
	}
}

type PrometheusTSDBHeadSeriesCreatedTotalAttr interface {
	Attribute
	implPrometheusTSDBHeadSeriesCreatedTotal()
}

func (m PrometheusTSDBHeadSeriesCreatedTotal) With(
	extra ...PrometheusTSDBHeadSeriesCreatedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBHeadSeriesNotFoundTotal records the total number of requests for series that were not found.
type PrometheusTSDBHeadSeriesNotFoundTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBHeadSeriesNotFoundTotal returns a new PrometheusTSDBHeadSeriesNotFoundTotal instrument.
func NewPrometheusTSDBHeadSeriesNotFoundTotal() PrometheusTSDBHeadSeriesNotFoundTotal {
	labels := []string{}
	return PrometheusTSDBHeadSeriesNotFoundTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_series_not_found_total",
			Help: "Total number of requests for series that were not found.",
		}, labels),
	}
}

type PrometheusTSDBHeadSeriesNotFoundTotalAttr interface {
	Attribute
	implPrometheusTSDBHeadSeriesNotFoundTotal()
}

func (m PrometheusTSDBHeadSeriesNotFoundTotal) With(
	extra ...PrometheusTSDBHeadSeriesNotFoundTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBHeadSeriesRemovedTotal records the total number of series removed from the head block.
type PrometheusTSDBHeadSeriesRemovedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBHeadSeriesRemovedTotal returns a new PrometheusTSDBHeadSeriesRemovedTotal instrument.
func NewPrometheusTSDBHeadSeriesRemovedTotal() PrometheusTSDBHeadSeriesRemovedTotal {
	labels := []string{}
	return PrometheusTSDBHeadSeriesRemovedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_series_removed_total",
			Help: "Total number of series removed from the head block.",
		}, labels),
	}
}

type PrometheusTSDBHeadSeriesRemovedTotalAttr interface {
	Attribute
	implPrometheusTSDBHeadSeriesRemovedTotal()
}

func (m PrometheusTSDBHeadSeriesRemovedTotal) With(
	extra ...PrometheusTSDBHeadSeriesRemovedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBHeadStaleSeries records the number of stale series in the head block.
type PrometheusTSDBHeadStaleSeries struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBHeadStaleSeries returns a new PrometheusTSDBHeadStaleSeries instrument.
func NewPrometheusTSDBHeadStaleSeries() PrometheusTSDBHeadStaleSeries {
	labels := []string{}
	return PrometheusTSDBHeadStaleSeries{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_stale_series",
			Help: "Number of stale series in the head block.",
		}, labels),
	}
}

type PrometheusTSDBHeadStaleSeriesAttr interface {
	Attribute
	implPrometheusTSDBHeadStaleSeries()
}

func (m PrometheusTSDBHeadStaleSeries) With(
	extra ...PrometheusTSDBHeadStaleSeriesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBHeadTruncationsFailedTotal records the total number of head truncations that failed.
type PrometheusTSDBHeadTruncationsFailedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBHeadTruncationsFailedTotal returns a new PrometheusTSDBHeadTruncationsFailedTotal instrument.
func NewPrometheusTSDBHeadTruncationsFailedTotal() PrometheusTSDBHeadTruncationsFailedTotal {
	labels := []string{}
	return PrometheusTSDBHeadTruncationsFailedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_truncations_failed_total",
			Help: "Total number of head truncations that failed.",
		}, labels),
	}
}

type PrometheusTSDBHeadTruncationsFailedTotalAttr interface {
	Attribute
	implPrometheusTSDBHeadTruncationsFailedTotal()
}

func (m PrometheusTSDBHeadTruncationsFailedTotal) With(
	extra ...PrometheusTSDBHeadTruncationsFailedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBHeadTruncationsTotal records the total number of head truncations attempted.
type PrometheusTSDBHeadTruncationsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBHeadTruncationsTotal returns a new PrometheusTSDBHeadTruncationsTotal instrument.
func NewPrometheusTSDBHeadTruncationsTotal() PrometheusTSDBHeadTruncationsTotal {
	labels := []string{}
	return PrometheusTSDBHeadTruncationsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_head_truncations_total",
			Help: "Total number of head truncations attempted.",
		}, labels),
	}
}

type PrometheusTSDBHeadTruncationsTotalAttr interface {
	Attribute
	implPrometheusTSDBHeadTruncationsTotal()
}

func (m PrometheusTSDBHeadTruncationsTotal) With(
	extra ...PrometheusTSDBHeadTruncationsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBIsolationHighWatermark records the isolation high watermark.
type PrometheusTSDBIsolationHighWatermark struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBIsolationHighWatermark returns a new PrometheusTSDBIsolationHighWatermark instrument.
func NewPrometheusTSDBIsolationHighWatermark() PrometheusTSDBIsolationHighWatermark {
	labels := []string{}
	return PrometheusTSDBIsolationHighWatermark{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_isolation_high_watermark",
			Help: "The isolation high watermark.",
		}, labels),
	}
}

type PrometheusTSDBIsolationHighWatermarkAttr interface {
	Attribute
	implPrometheusTSDBIsolationHighWatermark()
}

func (m PrometheusTSDBIsolationHighWatermark) With(
	extra ...PrometheusTSDBIsolationHighWatermarkAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBIsolationLowWatermark records the isolation low watermark.
type PrometheusTSDBIsolationLowWatermark struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBIsolationLowWatermark returns a new PrometheusTSDBIsolationLowWatermark instrument.
func NewPrometheusTSDBIsolationLowWatermark() PrometheusTSDBIsolationLowWatermark {
	labels := []string{}
	return PrometheusTSDBIsolationLowWatermark{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_isolation_low_watermark",
			Help: "The isolation low watermark.",
		}, labels),
	}
}

type PrometheusTSDBIsolationLowWatermarkAttr interface {
	Attribute
	implPrometheusTSDBIsolationLowWatermark()
}

func (m PrometheusTSDBIsolationLowWatermark) With(
	extra ...PrometheusTSDBIsolationLowWatermarkAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBLowestTimestamp records the lowest timestamp value stored in the database.
type PrometheusTSDBLowestTimestamp struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBLowestTimestamp returns a new PrometheusTSDBLowestTimestamp instrument.
func NewPrometheusTSDBLowestTimestamp() PrometheusTSDBLowestTimestamp {
	labels := []string{}
	return PrometheusTSDBLowestTimestamp{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_lowest_timestamp",
			Help: "Lowest timestamp value stored in the database.",
		}, labels),
	}
}

type PrometheusTSDBLowestTimestampAttr interface {
	Attribute
	implPrometheusTSDBLowestTimestamp()
}

func (m PrometheusTSDBLowestTimestamp) With(
	extra ...PrometheusTSDBLowestTimestampAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBLowestTimestampSeconds records the lowest timestamp value stored in the database in seconds.
type PrometheusTSDBLowestTimestampSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBLowestTimestampSeconds returns a new PrometheusTSDBLowestTimestampSeconds instrument.
func NewPrometheusTSDBLowestTimestampSeconds() PrometheusTSDBLowestTimestampSeconds {
	labels := []string{}
	return PrometheusTSDBLowestTimestampSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_lowest_timestamp_seconds",
			Help: "Lowest timestamp value stored in the database in seconds.",
		}, labels),
	}
}

type PrometheusTSDBLowestTimestampSecondsAttr interface {
	Attribute
	implPrometheusTSDBLowestTimestampSeconds()
}

func (m PrometheusTSDBLowestTimestampSeconds) With(
	extra ...PrometheusTSDBLowestTimestampSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBMmapChunkCorruptionsTotal records the total number of memory-mapped chunk corruptions.
type PrometheusTSDBMmapChunkCorruptionsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBMmapChunkCorruptionsTotal returns a new PrometheusTSDBMmapChunkCorruptionsTotal instrument.
func NewPrometheusTSDBMmapChunkCorruptionsTotal() PrometheusTSDBMmapChunkCorruptionsTotal {
	labels := []string{}
	return PrometheusTSDBMmapChunkCorruptionsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_mmap_chunk_corruptions_total",
			Help: "Total number of memory-mapped chunk corruptions.",
		}, labels),
	}
}

type PrometheusTSDBMmapChunkCorruptionsTotalAttr interface {
	Attribute
	implPrometheusTSDBMmapChunkCorruptionsTotal()
}

func (m PrometheusTSDBMmapChunkCorruptionsTotal) With(
	extra ...PrometheusTSDBMmapChunkCorruptionsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBMmapChunksTotal records the total number of memory-mapped chunks.
type PrometheusTSDBMmapChunksTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBMmapChunksTotal returns a new PrometheusTSDBMmapChunksTotal instrument.
func NewPrometheusTSDBMmapChunksTotal() PrometheusTSDBMmapChunksTotal {
	labels := []string{}
	return PrometheusTSDBMmapChunksTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_mmap_chunks_total",
			Help: "Total number of memory-mapped chunks.",
		}, labels),
	}
}

type PrometheusTSDBMmapChunksTotalAttr interface {
	Attribute
	implPrometheusTSDBMmapChunksTotal()
}

func (m PrometheusTSDBMmapChunksTotal) With(
	extra ...PrometheusTSDBMmapChunksTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
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
	*prometheus.CounterVec
}

// NewPrometheusTSDBOutOfOrderWBLCompletedPagesTotal returns a new PrometheusTSDBOutOfOrderWBLCompletedPagesTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLCompletedPagesTotal() PrometheusTSDBOutOfOrderWBLCompletedPagesTotal {
	labels := []string{}
	return PrometheusTSDBOutOfOrderWBLCompletedPagesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_completed_pages_total",
			Help: "Total number of completed WBL pages for out-of-order samples.",
		}, labels),
	}
}

type PrometheusTSDBOutOfOrderWBLCompletedPagesTotalAttr interface {
	Attribute
	implPrometheusTSDBOutOfOrderWBLCompletedPagesTotal()
}

func (m PrometheusTSDBOutOfOrderWBLCompletedPagesTotal) With(
	extra ...PrometheusTSDBOutOfOrderWBLCompletedPagesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBOutOfOrderWBLFsyncDurationSeconds records the duration of WBL fsync for out-of-order samples.
type PrometheusTSDBOutOfOrderWBLFsyncDurationSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusTSDBOutOfOrderWBLFsyncDurationSeconds returns a new PrometheusTSDBOutOfOrderWBLFsyncDurationSeconds instrument.
func NewPrometheusTSDBOutOfOrderWBLFsyncDurationSeconds() PrometheusTSDBOutOfOrderWBLFsyncDurationSeconds {
	labels := []string{}
	return PrometheusTSDBOutOfOrderWBLFsyncDurationSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_fsync_duration_seconds",
			Help: "Duration of WBL fsync for out-of-order samples.",
		}, labels),
	}
}

type PrometheusTSDBOutOfOrderWBLFsyncDurationSecondsAttr interface {
	Attribute
	implPrometheusTSDBOutOfOrderWBLFsyncDurationSeconds()
}

func (m PrometheusTSDBOutOfOrderWBLFsyncDurationSeconds) With(
	extra ...PrometheusTSDBOutOfOrderWBLFsyncDurationSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusTSDBOutOfOrderWBLPageFlushesTotal records the total number of WBL page flushes for out-of-order samples.
type PrometheusTSDBOutOfOrderWBLPageFlushesTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBOutOfOrderWBLPageFlushesTotal returns a new PrometheusTSDBOutOfOrderWBLPageFlushesTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLPageFlushesTotal() PrometheusTSDBOutOfOrderWBLPageFlushesTotal {
	labels := []string{}
	return PrometheusTSDBOutOfOrderWBLPageFlushesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_page_flushes_total",
			Help: "Total number of WBL page flushes for out-of-order samples.",
		}, labels),
	}
}

type PrometheusTSDBOutOfOrderWBLPageFlushesTotalAttr interface {
	Attribute
	implPrometheusTSDBOutOfOrderWBLPageFlushesTotal()
}

func (m PrometheusTSDBOutOfOrderWBLPageFlushesTotal) With(
	extra ...PrometheusTSDBOutOfOrderWBLPageFlushesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBOutOfOrderWBLRecordPartWritesTotal records the total number of WBL record part writes for out-of-order samples.
type PrometheusTSDBOutOfOrderWBLRecordPartWritesTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBOutOfOrderWBLRecordPartWritesTotal returns a new PrometheusTSDBOutOfOrderWBLRecordPartWritesTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLRecordPartWritesTotal() PrometheusTSDBOutOfOrderWBLRecordPartWritesTotal {
	labels := []string{}
	return PrometheusTSDBOutOfOrderWBLRecordPartWritesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_record_part_writes_total",
			Help: "Total number of WBL record part writes for out-of-order samples.",
		}, labels),
	}
}

type PrometheusTSDBOutOfOrderWBLRecordPartWritesTotalAttr interface {
	Attribute
	implPrometheusTSDBOutOfOrderWBLRecordPartWritesTotal()
}

func (m PrometheusTSDBOutOfOrderWBLRecordPartWritesTotal) With(
	extra ...PrometheusTSDBOutOfOrderWBLRecordPartWritesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal records the total bytes written to WBL record parts for out-of-order samples.
type PrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal returns a new PrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal() PrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal {
	labels := []string{}
	return PrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_record_parts_bytes_written_total",
			Help: "Total bytes written to WBL record parts for out-of-order samples.",
		}, labels),
	}
}

type PrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotalAttr interface {
	Attribute
	implPrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal()
}

func (m PrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotal) With(
	extra ...PrometheusTSDBOutOfOrderWBLRecordPartsBytesWrittenTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBOutOfOrderWBLSegmentCurrent records the current out-of-order WBL segment.
type PrometheusTSDBOutOfOrderWBLSegmentCurrent struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBOutOfOrderWBLSegmentCurrent returns a new PrometheusTSDBOutOfOrderWBLSegmentCurrent instrument.
func NewPrometheusTSDBOutOfOrderWBLSegmentCurrent() PrometheusTSDBOutOfOrderWBLSegmentCurrent {
	labels := []string{}
	return PrometheusTSDBOutOfOrderWBLSegmentCurrent{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_segment_current",
			Help: "Current out-of-order WBL segment.",
		}, labels),
	}
}

type PrometheusTSDBOutOfOrderWBLSegmentCurrentAttr interface {
	Attribute
	implPrometheusTSDBOutOfOrderWBLSegmentCurrent()
}

func (m PrometheusTSDBOutOfOrderWBLSegmentCurrent) With(
	extra ...PrometheusTSDBOutOfOrderWBLSegmentCurrentAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBOutOfOrderWBLStorageSizeBytes records the size of the out-of-order WBL storage.
type PrometheusTSDBOutOfOrderWBLStorageSizeBytes struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBOutOfOrderWBLStorageSizeBytes returns a new PrometheusTSDBOutOfOrderWBLStorageSizeBytes instrument.
func NewPrometheusTSDBOutOfOrderWBLStorageSizeBytes() PrometheusTSDBOutOfOrderWBLStorageSizeBytes {
	labels := []string{}
	return PrometheusTSDBOutOfOrderWBLStorageSizeBytes{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_storage_size_bytes",
			Help: "Size of the out-of-order WBL storage.",
		}, labels),
	}
}

type PrometheusTSDBOutOfOrderWBLStorageSizeBytesAttr interface {
	Attribute
	implPrometheusTSDBOutOfOrderWBLStorageSizeBytes()
}

func (m PrometheusTSDBOutOfOrderWBLStorageSizeBytes) With(
	extra ...PrometheusTSDBOutOfOrderWBLStorageSizeBytesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBOutOfOrderWBLTruncationsFailedTotal records the total number of out-of-order WBL truncations that failed.
type PrometheusTSDBOutOfOrderWBLTruncationsFailedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBOutOfOrderWBLTruncationsFailedTotal returns a new PrometheusTSDBOutOfOrderWBLTruncationsFailedTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLTruncationsFailedTotal() PrometheusTSDBOutOfOrderWBLTruncationsFailedTotal {
	labels := []string{}
	return PrometheusTSDBOutOfOrderWBLTruncationsFailedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_truncations_failed_total",
			Help: "Total number of out-of-order WBL truncations that failed.",
		}, labels),
	}
}

type PrometheusTSDBOutOfOrderWBLTruncationsFailedTotalAttr interface {
	Attribute
	implPrometheusTSDBOutOfOrderWBLTruncationsFailedTotal()
}

func (m PrometheusTSDBOutOfOrderWBLTruncationsFailedTotal) With(
	extra ...PrometheusTSDBOutOfOrderWBLTruncationsFailedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBOutOfOrderWBLTruncationsTotal records the total number of out-of-order WBL truncations.
type PrometheusTSDBOutOfOrderWBLTruncationsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBOutOfOrderWBLTruncationsTotal returns a new PrometheusTSDBOutOfOrderWBLTruncationsTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLTruncationsTotal() PrometheusTSDBOutOfOrderWBLTruncationsTotal {
	labels := []string{}
	return PrometheusTSDBOutOfOrderWBLTruncationsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_truncations_total",
			Help: "Total number of out-of-order WBL truncations.",
		}, labels),
	}
}

type PrometheusTSDBOutOfOrderWBLTruncationsTotalAttr interface {
	Attribute
	implPrometheusTSDBOutOfOrderWBLTruncationsTotal()
}

func (m PrometheusTSDBOutOfOrderWBLTruncationsTotal) With(
	extra ...PrometheusTSDBOutOfOrderWBLTruncationsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBOutOfOrderWBLWritesFailedTotal records the total number of out-of-order WBL writes that failed.
type PrometheusTSDBOutOfOrderWBLWritesFailedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBOutOfOrderWBLWritesFailedTotal returns a new PrometheusTSDBOutOfOrderWBLWritesFailedTotal instrument.
func NewPrometheusTSDBOutOfOrderWBLWritesFailedTotal() PrometheusTSDBOutOfOrderWBLWritesFailedTotal {
	labels := []string{}
	return PrometheusTSDBOutOfOrderWBLWritesFailedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_out_of_order_wbl_writes_failed_total",
			Help: "Total number of out-of-order WBL writes that failed.",
		}, labels),
	}
}

type PrometheusTSDBOutOfOrderWBLWritesFailedTotalAttr interface {
	Attribute
	implPrometheusTSDBOutOfOrderWBLWritesFailedTotal()
}

func (m PrometheusTSDBOutOfOrderWBLWritesFailedTotal) With(
	extra ...PrometheusTSDBOutOfOrderWBLWritesFailedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBReloadsFailuresTotal records the number of times the database reloads failed.
type PrometheusTSDBReloadsFailuresTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBReloadsFailuresTotal returns a new PrometheusTSDBReloadsFailuresTotal instrument.
func NewPrometheusTSDBReloadsFailuresTotal() PrometheusTSDBReloadsFailuresTotal {
	labels := []string{}
	return PrometheusTSDBReloadsFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_reloads_failures_total",
			Help: "Number of times the database reloads failed.",
		}, labels),
	}
}

type PrometheusTSDBReloadsFailuresTotalAttr interface {
	Attribute
	implPrometheusTSDBReloadsFailuresTotal()
}

func (m PrometheusTSDBReloadsFailuresTotal) With(
	extra ...PrometheusTSDBReloadsFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBReloadsTotal records the number of times the database reloads.
type PrometheusTSDBReloadsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBReloadsTotal returns a new PrometheusTSDBReloadsTotal instrument.
func NewPrometheusTSDBReloadsTotal() PrometheusTSDBReloadsTotal {
	labels := []string{}
	return PrometheusTSDBReloadsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_reloads_total",
			Help: "Number of times the database reloads.",
		}, labels),
	}
}

type PrometheusTSDBReloadsTotalAttr interface {
	Attribute
	implPrometheusTSDBReloadsTotal()
}

func (m PrometheusTSDBReloadsTotal) With(
	extra ...PrometheusTSDBReloadsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBRetentionLimitBytes records the maximum number of bytes to be retained in the TSDB.
type PrometheusTSDBRetentionLimitBytes struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBRetentionLimitBytes returns a new PrometheusTSDBRetentionLimitBytes instrument.
func NewPrometheusTSDBRetentionLimitBytes() PrometheusTSDBRetentionLimitBytes {
	labels := []string{}
	return PrometheusTSDBRetentionLimitBytes{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_retention_limit_bytes",
			Help: "Maximum number of bytes to be retained in the TSDB.",
		}, labels),
	}
}

type PrometheusTSDBRetentionLimitBytesAttr interface {
	Attribute
	implPrometheusTSDBRetentionLimitBytes()
}

func (m PrometheusTSDBRetentionLimitBytes) With(
	extra ...PrometheusTSDBRetentionLimitBytesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBRetentionLimitSeconds records the maximum age in seconds for samples to be retained in the TSDB.
type PrometheusTSDBRetentionLimitSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBRetentionLimitSeconds returns a new PrometheusTSDBRetentionLimitSeconds instrument.
func NewPrometheusTSDBRetentionLimitSeconds() PrometheusTSDBRetentionLimitSeconds {
	labels := []string{}
	return PrometheusTSDBRetentionLimitSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_retention_limit_seconds",
			Help: "Maximum age in seconds for samples to be retained in the TSDB.",
		}, labels),
	}
}

type PrometheusTSDBRetentionLimitSecondsAttr interface {
	Attribute
	implPrometheusTSDBRetentionLimitSeconds()
}

func (m PrometheusTSDBRetentionLimitSeconds) With(
	extra ...PrometheusTSDBRetentionLimitSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBSampleOOODelta records the delta in seconds between the time when an out-of-order sample was ingested and the latest sample in the chunk.
type PrometheusTSDBSampleOOODelta struct {
	*prometheus.HistogramVec
}

// NewPrometheusTSDBSampleOOODelta returns a new PrometheusTSDBSampleOOODelta instrument.
func NewPrometheusTSDBSampleOOODelta() PrometheusTSDBSampleOOODelta {
	labels := []string{}
	return PrometheusTSDBSampleOOODelta{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_tsdb_sample_ooo_delta",
			Help: "Delta in seconds between the time when an out-of-order sample was ingested and the latest sample in the chunk.",
		}, labels),
	}
}

type PrometheusTSDBSampleOOODeltaAttr interface {
	Attribute
	implPrometheusTSDBSampleOOODelta()
}

func (m PrometheusTSDBSampleOOODelta) With(
	extra ...PrometheusTSDBSampleOOODeltaAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusTSDBSizeRetentionsTotal records the number of times that blocks were deleted because the maximum number of bytes was exceeded.
type PrometheusTSDBSizeRetentionsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBSizeRetentionsTotal returns a new PrometheusTSDBSizeRetentionsTotal instrument.
func NewPrometheusTSDBSizeRetentionsTotal() PrometheusTSDBSizeRetentionsTotal {
	labels := []string{}
	return PrometheusTSDBSizeRetentionsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_size_retentions_total",
			Help: "Number of times that blocks were deleted because the maximum number of bytes was exceeded.",
		}, labels),
	}
}

type PrometheusTSDBSizeRetentionsTotalAttr interface {
	Attribute
	implPrometheusTSDBSizeRetentionsTotal()
}

func (m PrometheusTSDBSizeRetentionsTotal) With(
	extra ...PrometheusTSDBSizeRetentionsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBSnapshotReplayErrorTotal records the total number of snapshot replay errors.
type PrometheusTSDBSnapshotReplayErrorTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBSnapshotReplayErrorTotal returns a new PrometheusTSDBSnapshotReplayErrorTotal instrument.
func NewPrometheusTSDBSnapshotReplayErrorTotal() PrometheusTSDBSnapshotReplayErrorTotal {
	labels := []string{}
	return PrometheusTSDBSnapshotReplayErrorTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_snapshot_replay_error_total",
			Help: "Total number of snapshot replay errors.",
		}, labels),
	}
}

type PrometheusTSDBSnapshotReplayErrorTotalAttr interface {
	Attribute
	implPrometheusTSDBSnapshotReplayErrorTotal()
}

func (m PrometheusTSDBSnapshotReplayErrorTotal) With(
	extra ...PrometheusTSDBSnapshotReplayErrorTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBStorageBlocksBytes records the number of bytes that are currently used for local storage by all blocks.
type PrometheusTSDBStorageBlocksBytes struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBStorageBlocksBytes returns a new PrometheusTSDBStorageBlocksBytes instrument.
func NewPrometheusTSDBStorageBlocksBytes() PrometheusTSDBStorageBlocksBytes {
	labels := []string{}
	return PrometheusTSDBStorageBlocksBytes{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_storage_blocks_bytes",
			Help: "The number of bytes that are currently used for local storage by all blocks.",
		}, labels),
	}
}

type PrometheusTSDBStorageBlocksBytesAttr interface {
	Attribute
	implPrometheusTSDBStorageBlocksBytes()
}

func (m PrometheusTSDBStorageBlocksBytes) With(
	extra ...PrometheusTSDBStorageBlocksBytesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBSymbolTableSizeBytes records the size of the symbol table in bytes.
type PrometheusTSDBSymbolTableSizeBytes struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBSymbolTableSizeBytes returns a new PrometheusTSDBSymbolTableSizeBytes instrument.
func NewPrometheusTSDBSymbolTableSizeBytes() PrometheusTSDBSymbolTableSizeBytes {
	labels := []string{}
	return PrometheusTSDBSymbolTableSizeBytes{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_symbol_table_size_bytes",
			Help: "Size of the symbol table in bytes.",
		}, labels),
	}
}

type PrometheusTSDBSymbolTableSizeBytesAttr interface {
	Attribute
	implPrometheusTSDBSymbolTableSizeBytes()
}

func (m PrometheusTSDBSymbolTableSizeBytes) With(
	extra ...PrometheusTSDBSymbolTableSizeBytesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBTimeRetentionsTotal records the number of times that blocks were deleted because the maximum time limit was exceeded.
type PrometheusTSDBTimeRetentionsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBTimeRetentionsTotal returns a new PrometheusTSDBTimeRetentionsTotal instrument.
func NewPrometheusTSDBTimeRetentionsTotal() PrometheusTSDBTimeRetentionsTotal {
	labels := []string{}
	return PrometheusTSDBTimeRetentionsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_time_retentions_total",
			Help: "Number of times that blocks were deleted because the maximum time limit was exceeded.",
		}, labels),
	}
}

type PrometheusTSDBTimeRetentionsTotalAttr interface {
	Attribute
	implPrometheusTSDBTimeRetentionsTotal()
}

func (m PrometheusTSDBTimeRetentionsTotal) With(
	extra ...PrometheusTSDBTimeRetentionsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBTombstoneCleanupSeconds records the time taken to clean up tombstones.
type PrometheusTSDBTombstoneCleanupSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusTSDBTombstoneCleanupSeconds returns a new PrometheusTSDBTombstoneCleanupSeconds instrument.
func NewPrometheusTSDBTombstoneCleanupSeconds() PrometheusTSDBTombstoneCleanupSeconds {
	labels := []string{}
	return PrometheusTSDBTombstoneCleanupSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_tsdb_tombstone_cleanup_seconds",
			Help: "Time taken to clean up tombstones.",
		}, labels),
	}
}

type PrometheusTSDBTombstoneCleanupSecondsAttr interface {
	Attribute
	implPrometheusTSDBTombstoneCleanupSeconds()
}

func (m PrometheusTSDBTombstoneCleanupSeconds) With(
	extra ...PrometheusTSDBTombstoneCleanupSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
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
	*prometheus.CounterVec
}

// NewPrometheusTSDBVerticalCompactionsTotal returns a new PrometheusTSDBVerticalCompactionsTotal instrument.
func NewPrometheusTSDBVerticalCompactionsTotal() PrometheusTSDBVerticalCompactionsTotal {
	labels := []string{}
	return PrometheusTSDBVerticalCompactionsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_vertical_compactions_total",
			Help: "Total number of compactions done on overlapping blocks.",
		}, labels),
	}
}

type PrometheusTSDBVerticalCompactionsTotalAttr interface {
	Attribute
	implPrometheusTSDBVerticalCompactionsTotal()
}

func (m PrometheusTSDBVerticalCompactionsTotal) With(
	extra ...PrometheusTSDBVerticalCompactionsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBWALCompletedPagesTotal records the total number of completed WAL pages.
type PrometheusTSDBWALCompletedPagesTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBWALCompletedPagesTotal returns a new PrometheusTSDBWALCompletedPagesTotal instrument.
func NewPrometheusTSDBWALCompletedPagesTotal() PrometheusTSDBWALCompletedPagesTotal {
	labels := []string{}
	return PrometheusTSDBWALCompletedPagesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_completed_pages_total",
			Help: "Total number of completed WAL pages.",
		}, labels),
	}
}

type PrometheusTSDBWALCompletedPagesTotalAttr interface {
	Attribute
	implPrometheusTSDBWALCompletedPagesTotal()
}

func (m PrometheusTSDBWALCompletedPagesTotal) With(
	extra ...PrometheusTSDBWALCompletedPagesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBWALCorruptionsTotal records the total number of WAL corruptions.
type PrometheusTSDBWALCorruptionsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBWALCorruptionsTotal returns a new PrometheusTSDBWALCorruptionsTotal instrument.
func NewPrometheusTSDBWALCorruptionsTotal() PrometheusTSDBWALCorruptionsTotal {
	labels := []string{}
	return PrometheusTSDBWALCorruptionsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_corruptions_total",
			Help: "Total number of WAL corruptions.",
		}, labels),
	}
}

type PrometheusTSDBWALCorruptionsTotalAttr interface {
	Attribute
	implPrometheusTSDBWALCorruptionsTotal()
}

func (m PrometheusTSDBWALCorruptionsTotal) With(
	extra ...PrometheusTSDBWALCorruptionsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBWALFsyncDurationSeconds records the duration of WAL fsync.
type PrometheusTSDBWALFsyncDurationSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusTSDBWALFsyncDurationSeconds returns a new PrometheusTSDBWALFsyncDurationSeconds instrument.
func NewPrometheusTSDBWALFsyncDurationSeconds() PrometheusTSDBWALFsyncDurationSeconds {
	labels := []string{}
	return PrometheusTSDBWALFsyncDurationSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_tsdb_wal_fsync_duration_seconds",
			Help: "Duration of WAL fsync.",
		}, labels),
	}
}

type PrometheusTSDBWALFsyncDurationSecondsAttr interface {
	Attribute
	implPrometheusTSDBWALFsyncDurationSeconds()
}

func (m PrometheusTSDBWALFsyncDurationSeconds) With(
	extra ...PrometheusTSDBWALFsyncDurationSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusTSDBWALPageFlushesTotal records the total number of WAL page flushes.
type PrometheusTSDBWALPageFlushesTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBWALPageFlushesTotal returns a new PrometheusTSDBWALPageFlushesTotal instrument.
func NewPrometheusTSDBWALPageFlushesTotal() PrometheusTSDBWALPageFlushesTotal {
	labels := []string{}
	return PrometheusTSDBWALPageFlushesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_page_flushes_total",
			Help: "Total number of WAL page flushes.",
		}, labels),
	}
}

type PrometheusTSDBWALPageFlushesTotalAttr interface {
	Attribute
	implPrometheusTSDBWALPageFlushesTotal()
}

func (m PrometheusTSDBWALPageFlushesTotal) With(
	extra ...PrometheusTSDBWALPageFlushesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
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
	*prometheus.CounterVec
}

// NewPrometheusTSDBWALRecordPartWritesTotal returns a new PrometheusTSDBWALRecordPartWritesTotal instrument.
func NewPrometheusTSDBWALRecordPartWritesTotal() PrometheusTSDBWALRecordPartWritesTotal {
	labels := []string{}
	return PrometheusTSDBWALRecordPartWritesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_record_part_writes_total",
			Help: "Total number of WAL record part writes.",
		}, labels),
	}
}

type PrometheusTSDBWALRecordPartWritesTotalAttr interface {
	Attribute
	implPrometheusTSDBWALRecordPartWritesTotal()
}

func (m PrometheusTSDBWALRecordPartWritesTotal) With(
	extra ...PrometheusTSDBWALRecordPartWritesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBWALRecordPartsBytesWrittenTotal records the total bytes written to WAL record parts.
type PrometheusTSDBWALRecordPartsBytesWrittenTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBWALRecordPartsBytesWrittenTotal returns a new PrometheusTSDBWALRecordPartsBytesWrittenTotal instrument.
func NewPrometheusTSDBWALRecordPartsBytesWrittenTotal() PrometheusTSDBWALRecordPartsBytesWrittenTotal {
	labels := []string{}
	return PrometheusTSDBWALRecordPartsBytesWrittenTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_record_parts_bytes_written_total",
			Help: "Total bytes written to WAL record parts.",
		}, labels),
	}
}

type PrometheusTSDBWALRecordPartsBytesWrittenTotalAttr interface {
	Attribute
	implPrometheusTSDBWALRecordPartsBytesWrittenTotal()
}

func (m PrometheusTSDBWALRecordPartsBytesWrittenTotal) With(
	extra ...PrometheusTSDBWALRecordPartsBytesWrittenTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBWALSegmentCurrent records the current WAL segment.
type PrometheusTSDBWALSegmentCurrent struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBWALSegmentCurrent returns a new PrometheusTSDBWALSegmentCurrent instrument.
func NewPrometheusTSDBWALSegmentCurrent() PrometheusTSDBWALSegmentCurrent {
	labels := []string{}
	return PrometheusTSDBWALSegmentCurrent{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_wal_segment_current",
			Help: "Current WAL segment.",
		}, labels),
	}
}

type PrometheusTSDBWALSegmentCurrentAttr interface {
	Attribute
	implPrometheusTSDBWALSegmentCurrent()
}

func (m PrometheusTSDBWALSegmentCurrent) With(
	extra ...PrometheusTSDBWALSegmentCurrentAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBWALStorageSizeBytes records the size of the WAL storage.
type PrometheusTSDBWALStorageSizeBytes struct {
	*prometheus.GaugeVec
}

// NewPrometheusTSDBWALStorageSizeBytes returns a new PrometheusTSDBWALStorageSizeBytes instrument.
func NewPrometheusTSDBWALStorageSizeBytes() PrometheusTSDBWALStorageSizeBytes {
	labels := []string{}
	return PrometheusTSDBWALStorageSizeBytes{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_wal_storage_size_bytes",
			Help: "Size of the WAL storage.",
		}, labels),
	}
}

type PrometheusTSDBWALStorageSizeBytesAttr interface {
	Attribute
	implPrometheusTSDBWALStorageSizeBytes()
}

func (m PrometheusTSDBWALStorageSizeBytes) With(
	extra ...PrometheusTSDBWALStorageSizeBytesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTSDBWALTruncateDurationSeconds records the duration of WAL truncation.
type PrometheusTSDBWALTruncateDurationSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusTSDBWALTruncateDurationSeconds returns a new PrometheusTSDBWALTruncateDurationSeconds instrument.
func NewPrometheusTSDBWALTruncateDurationSeconds() PrometheusTSDBWALTruncateDurationSeconds {
	labels := []string{}
	return PrometheusTSDBWALTruncateDurationSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_tsdb_wal_truncate_duration_seconds",
			Help: "Duration of WAL truncation.",
		}, labels),
	}
}

type PrometheusTSDBWALTruncateDurationSecondsAttr interface {
	Attribute
	implPrometheusTSDBWALTruncateDurationSeconds()
}

func (m PrometheusTSDBWALTruncateDurationSeconds) With(
	extra ...PrometheusTSDBWALTruncateDurationSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusTSDBWALTruncationsFailedTotal records the total number of WAL truncations that failed.
type PrometheusTSDBWALTruncationsFailedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBWALTruncationsFailedTotal returns a new PrometheusTSDBWALTruncationsFailedTotal instrument.
func NewPrometheusTSDBWALTruncationsFailedTotal() PrometheusTSDBWALTruncationsFailedTotal {
	labels := []string{}
	return PrometheusTSDBWALTruncationsFailedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_truncations_failed_total",
			Help: "Total number of WAL truncations that failed.",
		}, labels),
	}
}

type PrometheusTSDBWALTruncationsFailedTotalAttr interface {
	Attribute
	implPrometheusTSDBWALTruncationsFailedTotal()
}

func (m PrometheusTSDBWALTruncationsFailedTotal) With(
	extra ...PrometheusTSDBWALTruncationsFailedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBWALTruncationsTotal records the total number of WAL truncations.
type PrometheusTSDBWALTruncationsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBWALTruncationsTotal returns a new PrometheusTSDBWALTruncationsTotal instrument.
func NewPrometheusTSDBWALTruncationsTotal() PrometheusTSDBWALTruncationsTotal {
	labels := []string{}
	return PrometheusTSDBWALTruncationsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_truncations_total",
			Help: "Total number of WAL truncations.",
		}, labels),
	}
}

type PrometheusTSDBWALTruncationsTotalAttr interface {
	Attribute
	implPrometheusTSDBWALTruncationsTotal()
}

func (m PrometheusTSDBWALTruncationsTotal) With(
	extra ...PrometheusTSDBWALTruncationsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTSDBWALWritesFailedTotal records the total number of WAL writes that failed.
type PrometheusTSDBWALWritesFailedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBWALWritesFailedTotal returns a new PrometheusTSDBWALWritesFailedTotal instrument.
func NewPrometheusTSDBWALWritesFailedTotal() PrometheusTSDBWALWritesFailedTotal {
	labels := []string{}
	return PrometheusTSDBWALWritesFailedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_writes_failed_total",
			Help: "Total number of WAL writes that failed.",
		}, labels),
	}
}

type PrometheusTSDBWALWritesFailedTotalAttr interface {
	Attribute
	implPrometheusTSDBWALWritesFailedTotal()
}

func (m PrometheusTSDBWALWritesFailedTotal) With(
	extra ...PrometheusTSDBWALWritesFailedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}
