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

// PrometheusRemoteReadHandlerQueries records the number of in-flight remote read queries.
type PrometheusRemoteReadHandlerQueries struct {
	*prometheus.GaugeVec
}

// NewPrometheusRemoteReadHandlerQueries returns a new PrometheusRemoteReadHandlerQueries instrument.
func NewPrometheusRemoteReadHandlerQueries() PrometheusRemoteReadHandlerQueries {
	labels := []string{}
	return PrometheusRemoteReadHandlerQueries{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_remote_read_handler_queries",
			Help: "The number of in-flight remote read queries.",
		}, labels),
	}
}

type PrometheusRemoteReadHandlerQueriesAttr interface {
	Attribute
	implPrometheusRemoteReadHandlerQueries()
}

func (m PrometheusRemoteReadHandlerQueries) With(
	extra ...PrometheusRemoteReadHandlerQueriesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusRemoteStorageExemplarsInTotal records the exemplars in to remote storage, compare to determine dropped exemplars.
type PrometheusRemoteStorageExemplarsInTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusRemoteStorageExemplarsInTotal returns a new PrometheusRemoteStorageExemplarsInTotal instrument.
func NewPrometheusRemoteStorageExemplarsInTotal() PrometheusRemoteStorageExemplarsInTotal {
	labels := []string{}
	return PrometheusRemoteStorageExemplarsInTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_remote_storage_exemplars_in_total",
			Help: "Exemplars in to remote storage, compare to determine dropped exemplars.",
		}, labels),
	}
}

type PrometheusRemoteStorageExemplarsInTotalAttr interface {
	Attribute
	implPrometheusRemoteStorageExemplarsInTotal()
}

func (m PrometheusRemoteStorageExemplarsInTotal) With(
	extra ...PrometheusRemoteStorageExemplarsInTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusRemoteStorageHighestTimestampInSeconds records the highest timestamp that has come into the remote storage via the Appender interface.
type PrometheusRemoteStorageHighestTimestampInSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusRemoteStorageHighestTimestampInSeconds returns a new PrometheusRemoteStorageHighestTimestampInSeconds instrument.
func NewPrometheusRemoteStorageHighestTimestampInSeconds() PrometheusRemoteStorageHighestTimestampInSeconds {
	labels := []string{}
	return PrometheusRemoteStorageHighestTimestampInSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_remote_storage_highest_timestamp_in_seconds",
			Help: "Highest timestamp that has come into the remote storage via the Appender interface.",
		}, labels),
	}
}

type PrometheusRemoteStorageHighestTimestampInSecondsAttr interface {
	Attribute
	implPrometheusRemoteStorageHighestTimestampInSeconds()
}

func (m PrometheusRemoteStorageHighestTimestampInSeconds) With(
	extra ...PrometheusRemoteStorageHighestTimestampInSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusRemoteStorageHistogramsInTotal records the histograms in to remote storage, compare to determine dropped histograms.
type PrometheusRemoteStorageHistogramsInTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusRemoteStorageHistogramsInTotal returns a new PrometheusRemoteStorageHistogramsInTotal instrument.
func NewPrometheusRemoteStorageHistogramsInTotal() PrometheusRemoteStorageHistogramsInTotal {
	labels := []string{}
	return PrometheusRemoteStorageHistogramsInTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_remote_storage_histograms_in_total",
			Help: "Histograms in to remote storage, compare to determine dropped histograms.",
		}, labels),
	}
}

type PrometheusRemoteStorageHistogramsInTotalAttr interface {
	Attribute
	implPrometheusRemoteStorageHistogramsInTotal()
}

func (m PrometheusRemoteStorageHistogramsInTotal) With(
	extra ...PrometheusRemoteStorageHistogramsInTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusRemoteStorageSamplesInTotal records the samples in to remote storage, compare to determine dropped samples.
type PrometheusRemoteStorageSamplesInTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusRemoteStorageSamplesInTotal returns a new PrometheusRemoteStorageSamplesInTotal instrument.
func NewPrometheusRemoteStorageSamplesInTotal() PrometheusRemoteStorageSamplesInTotal {
	labels := []string{}
	return PrometheusRemoteStorageSamplesInTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_remote_storage_samples_in_total",
			Help: "Samples in to remote storage, compare to determine dropped samples.",
		}, labels),
	}
}

type PrometheusRemoteStorageSamplesInTotalAttr interface {
	Attribute
	implPrometheusRemoteStorageSamplesInTotal()
}

func (m PrometheusRemoteStorageSamplesInTotal) With(
	extra ...PrometheusRemoteStorageSamplesInTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal records the number of times release has been called for strings that are not interned.
type PrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal returns a new PrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal instrument.
func NewPrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal() PrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal {
	labels := []string{}
	return PrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_remote_storage_string_interner_zero_reference_releases_total",
			Help: "The number of times release has been called for strings that are not interned.",
		}, labels),
	}
}

type PrometheusRemoteStorageStringInternerZeroReferenceReleasesTotalAttr interface {
	Attribute
	implPrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal()
}

func (m PrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal) With(
	extra ...PrometheusRemoteStorageStringInternerZeroReferenceReleasesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}
