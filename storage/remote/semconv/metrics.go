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
	prometheus.Gauge
}

// NewPrometheusRemoteReadHandlerQueries returns a new PrometheusRemoteReadHandlerQueries instrument.
func NewPrometheusRemoteReadHandlerQueries() PrometheusRemoteReadHandlerQueries {
	return PrometheusRemoteReadHandlerQueries{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_remote_read_handler_queries",
			Help: "The number of in-flight remote read queries.",
		}),
	}
}

// PrometheusRemoteStorageExemplarsInTotal records the exemplars in to remote storage, compare to determine dropped exemplars.
type PrometheusRemoteStorageExemplarsInTotal struct {
	prometheus.Counter
}

// NewPrometheusRemoteStorageExemplarsInTotal returns a new PrometheusRemoteStorageExemplarsInTotal instrument.
func NewPrometheusRemoteStorageExemplarsInTotal() PrometheusRemoteStorageExemplarsInTotal {
	return PrometheusRemoteStorageExemplarsInTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_remote_storage_exemplars_in_total",
			Help: "Exemplars in to remote storage, compare to determine dropped exemplars.",
		}),
	}
}

// PrometheusRemoteStorageHighestTimestampInSeconds records the highest timestamp that has come into the remote storage via the Appender interface.
type PrometheusRemoteStorageHighestTimestampInSeconds struct {
	prometheus.Gauge
}

// NewPrometheusRemoteStorageHighestTimestampInSeconds returns a new PrometheusRemoteStorageHighestTimestampInSeconds instrument.
func NewPrometheusRemoteStorageHighestTimestampInSeconds() PrometheusRemoteStorageHighestTimestampInSeconds {
	return PrometheusRemoteStorageHighestTimestampInSeconds{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_remote_storage_highest_timestamp_in_seconds",
			Help: "Highest timestamp that has come into the remote storage via the Appender interface.",
		}),
	}
}

// PrometheusRemoteStorageHistogramsInTotal records the histograms in to remote storage, compare to determine dropped histograms.
type PrometheusRemoteStorageHistogramsInTotal struct {
	prometheus.Counter
}

// NewPrometheusRemoteStorageHistogramsInTotal returns a new PrometheusRemoteStorageHistogramsInTotal instrument.
func NewPrometheusRemoteStorageHistogramsInTotal() PrometheusRemoteStorageHistogramsInTotal {
	return PrometheusRemoteStorageHistogramsInTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_remote_storage_histograms_in_total",
			Help: "Histograms in to remote storage, compare to determine dropped histograms.",
		}),
	}
}

// PrometheusRemoteStorageSamplesInTotal records the samples in to remote storage, compare to determine dropped samples.
type PrometheusRemoteStorageSamplesInTotal struct {
	prometheus.Counter
}

// NewPrometheusRemoteStorageSamplesInTotal returns a new PrometheusRemoteStorageSamplesInTotal instrument.
func NewPrometheusRemoteStorageSamplesInTotal() PrometheusRemoteStorageSamplesInTotal {
	return PrometheusRemoteStorageSamplesInTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_remote_storage_samples_in_total",
			Help: "Samples in to remote storage, compare to determine dropped samples.",
		}),
	}
}

// PrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal records the number of times release has been called for strings that are not interned.
type PrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal struct {
	prometheus.Counter
}

// NewPrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal returns a new PrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal instrument.
func NewPrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal() PrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal {
	return PrometheusRemoteStorageStringInternerZeroReferenceReleasesTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_remote_storage_string_interner_zero_reference_releases_total",
			Help: "The number of times release has been called for strings that are not interned.",
		}),
	}
}
