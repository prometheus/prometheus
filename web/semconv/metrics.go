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
type CodeAttr string

func (a CodeAttr) ID() string {
	return "code"
}

func (a CodeAttr) Value() string {
	return string(a)
}

type HandlerAttr string

func (a HandlerAttr) ID() string {
	return "handler"
}

func (a HandlerAttr) Value() string {
	return string(a)
}

// PrometheusAPINotificationActiveSubscribers records the current number of active notification subscribers.
type PrometheusAPINotificationActiveSubscribers struct {
	*prometheus.GaugeVec
}

// NewPrometheusAPINotificationActiveSubscribers returns a new PrometheusAPINotificationActiveSubscribers instrument.
func NewPrometheusAPINotificationActiveSubscribers() PrometheusAPINotificationActiveSubscribers {
	labels := []string{}
	return PrometheusAPINotificationActiveSubscribers{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_api_notification_active_subscribers",
			Help: "The current number of active notification subscribers.",
		}, labels),
	}
}

type PrometheusAPINotificationActiveSubscribersAttr interface {
	Attribute
	implPrometheusAPINotificationActiveSubscribers()
}

func (m PrometheusAPINotificationActiveSubscribers) With(
	extra ...PrometheusAPINotificationActiveSubscribersAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusAPINotificationUpdatesDroppedTotal records the total number of API notification updates that were dropped.
type PrometheusAPINotificationUpdatesDroppedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusAPINotificationUpdatesDroppedTotal returns a new PrometheusAPINotificationUpdatesDroppedTotal instrument.
func NewPrometheusAPINotificationUpdatesDroppedTotal() PrometheusAPINotificationUpdatesDroppedTotal {
	labels := []string{}
	return PrometheusAPINotificationUpdatesDroppedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_api_notification_updates_dropped_total",
			Help: "Total number of API notification updates that were dropped.",
		}, labels),
	}
}

type PrometheusAPINotificationUpdatesDroppedTotalAttr interface {
	Attribute
	implPrometheusAPINotificationUpdatesDroppedTotal()
}

func (m PrometheusAPINotificationUpdatesDroppedTotal) With(
	extra ...PrometheusAPINotificationUpdatesDroppedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusAPINotificationUpdatesSentTotal records the total number of API notification updates sent.
type PrometheusAPINotificationUpdatesSentTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusAPINotificationUpdatesSentTotal returns a new PrometheusAPINotificationUpdatesSentTotal instrument.
func NewPrometheusAPINotificationUpdatesSentTotal() PrometheusAPINotificationUpdatesSentTotal {
	labels := []string{}
	return PrometheusAPINotificationUpdatesSentTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_api_notification_updates_sent_total",
			Help: "Total number of API notification updates sent.",
		}, labels),
	}
}

type PrometheusAPINotificationUpdatesSentTotalAttr interface {
	Attribute
	implPrometheusAPINotificationUpdatesSentTotal()
}

func (m PrometheusAPINotificationUpdatesSentTotal) With(
	extra ...PrometheusAPINotificationUpdatesSentTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusHTTPRequestDurationSeconds records the histogram of latencies for HTTP requests.
type PrometheusHTTPRequestDurationSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusHTTPRequestDurationSeconds returns a new PrometheusHTTPRequestDurationSeconds instrument.
func NewPrometheusHTTPRequestDurationSeconds() PrometheusHTTPRequestDurationSeconds {
	labels := []string{
		"handler",
	}
	return PrometheusHTTPRequestDurationSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_http_request_duration_seconds",
			Help: "Histogram of latencies for HTTP requests.",
		}, labels),
	}
}

type PrometheusHTTPRequestDurationSecondsAttr interface {
	Attribute
	implPrometheusHTTPRequestDurationSeconds()
}

func (a HandlerAttr) implPrometheusHTTPRequestDurationSeconds() {}

func (m PrometheusHTTPRequestDurationSeconds) With(
	extra ...PrometheusHTTPRequestDurationSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{
		"handler": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusHTTPRequestsTotal records the counter of HTTP requests.
type PrometheusHTTPRequestsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusHTTPRequestsTotal returns a new PrometheusHTTPRequestsTotal instrument.
func NewPrometheusHTTPRequestsTotal() PrometheusHTTPRequestsTotal {
	labels := []string{
		"handler",
		"code",
	}
	return PrometheusHTTPRequestsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_http_requests_total",
			Help: "Counter of HTTP requests.",
		}, labels),
	}
}

type PrometheusHTTPRequestsTotalAttr interface {
	Attribute
	implPrometheusHTTPRequestsTotal()
}

func (a HandlerAttr) implPrometheusHTTPRequestsTotal() {}
func (a CodeAttr) implPrometheusHTTPRequestsTotal()    {}

func (m PrometheusHTTPRequestsTotal) With(
	extra ...PrometheusHTTPRequestsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"handler": "",
		"code":    "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusHTTPResponseSizeBytes records the histogram of response size for HTTP requests.
type PrometheusHTTPResponseSizeBytes struct {
	*prometheus.HistogramVec
}

// NewPrometheusHTTPResponseSizeBytes returns a new PrometheusHTTPResponseSizeBytes instrument.
func NewPrometheusHTTPResponseSizeBytes() PrometheusHTTPResponseSizeBytes {
	labels := []string{
		"handler",
	}
	return PrometheusHTTPResponseSizeBytes{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_http_response_size_bytes",
			Help: "Histogram of response size for HTTP requests.",
		}, labels),
	}
}

type PrometheusHTTPResponseSizeBytesAttr interface {
	Attribute
	implPrometheusHTTPResponseSizeBytes()
}

func (a HandlerAttr) implPrometheusHTTPResponseSizeBytes() {}

func (m PrometheusHTTPResponseSizeBytes) With(
	extra ...PrometheusHTTPResponseSizeBytesAttr,
) prometheus.Observer {
	labels := prometheus.Labels{
		"handler": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusReady records the whether Prometheus startup was fully completed and the server is ready for normal operation.
type PrometheusReady struct {
	*prometheus.GaugeVec
}

// NewPrometheusReady returns a new PrometheusReady instrument.
func NewPrometheusReady() PrometheusReady {
	labels := []string{}
	return PrometheusReady{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_ready",
			Help: "Whether Prometheus startup was fully completed and the server is ready for normal operation.",
		}, labels),
	}
}

type PrometheusReadyAttr interface {
	Attribute
	implPrometheusReady()
}

func (m PrometheusReady) With(
	extra ...PrometheusReadyAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusWebFederationErrorsTotal records the total number of errors that occurred while sending federation responses.
type PrometheusWebFederationErrorsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusWebFederationErrorsTotal returns a new PrometheusWebFederationErrorsTotal instrument.
func NewPrometheusWebFederationErrorsTotal() PrometheusWebFederationErrorsTotal {
	labels := []string{}
	return PrometheusWebFederationErrorsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_web_federation_errors_total",
			Help: "Total number of errors that occurred while sending federation responses.",
		}, labels),
	}
}

type PrometheusWebFederationErrorsTotalAttr interface {
	Attribute
	implPrometheusWebFederationErrorsTotal()
}

func (m PrometheusWebFederationErrorsTotal) With(
	extra ...PrometheusWebFederationErrorsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusWebFederationWarningsTotal records the total number of warnings that occurred while sending federation responses.
type PrometheusWebFederationWarningsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusWebFederationWarningsTotal returns a new PrometheusWebFederationWarningsTotal instrument.
func NewPrometheusWebFederationWarningsTotal() PrometheusWebFederationWarningsTotal {
	labels := []string{}
	return PrometheusWebFederationWarningsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_web_federation_warnings_total",
			Help: "Total number of warnings that occurred while sending federation responses.",
		}, labels),
	}
}

type PrometheusWebFederationWarningsTotalAttr interface {
	Attribute
	implPrometheusWebFederationWarningsTotal()
}

func (m PrometheusWebFederationWarningsTotal) With(
	extra ...PrometheusWebFederationWarningsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}
