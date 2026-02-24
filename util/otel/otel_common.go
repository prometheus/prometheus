/*
 * OpenTelemetry integration and helpers that are not specific to tracing or otel log exporters.
 *
 * Providers a mechanism to register for otel internal errors, maintains a metric for otel internal
 * error counts, and installs the global otel logger.
 */
package promotel_common

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"

	"github.com/prometheus/common/version"
)

const DefaultOTELServiceName = "prometheus"

type metrics struct {
	otelInternalErrors prometheus.Counter
}

type OtelErrorCallback func(error)

var (
	promOtelMetrics    *metrics
	otelErrorCallbacks []OtelErrorCallback
	isConfigured       bool
)

func newMetrics(registerer prometheus.Registerer) (*metrics, error) {
	m := &metrics{
		otelInternalErrors: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "otel_internal_errors_total",
				Help: "Total number of internal errors encountered by Prometheus's OpenTelemetry instrumentation",
			},
		),
	}

	collector := []prometheus.Collector{
		m.otelInternalErrors,
	}

	for _, collector := range collector {
		err := registerer.Register(collector)
		if err != nil {
			return nil, fmt.Errorf("failed to register discovery manager metrics: %w", err)
		}
	}
	return m, nil
}

func (m *metrics) unregister(registerer prometheus.Registerer) {
	registerer.Unregister(m.otelInternalErrors)
}

// Initialize global OpenTelemetry settings that are common to all OTLP exporters, such as the propagator and error handler. This should be called once at startup.
func GlobalOTELSetup(logger *slog.Logger, registerer prometheus.Registerer) {
	if promOtelMetrics != nil {
		logger.Warn("BUG: GlobalOTELSetup called multiple times; this should only be called once at startup")
	}
	// Keep a counter of internal errors in the OpenTelemetry instrumentation.
	var err error
	promOtelMetrics, err = newMetrics(registerer)
	if err != nil {
		logger.Error("Failed to initialize OpenTelemetry metrics", "err", err.Error())
	}
	// Deliver internal logs from OpenTelemetry instrumentation to our slog logger.
	otel.SetLogger(logr.FromSlogHandler(logger.Handler()))
	// Propagate otel internal errors to any registered listeners, for test use etc
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		promOtelMetrics.otelInternalErrors.Inc()
		for _, callback := range otelErrorCallbacks {
			callback(err)
		}
	}))
	// Use the W3C Trace Context propagator, which is the default for otel, and is compatible with most other systems. This is used by both the tracing and logging components, so it's set globally here.
	otel.SetTextMapPropagator(propagation.TraceContext{})
	isConfigured = true
}

func IsConfigured() bool {
	return isConfigured
}

// RegisterOtelErrorCallback allows components to register callbacks that will be called when the global OpenTelemetry error handler is invoked. This is useful for testing, allowing tests to be notified of errors directly, as the opentelemetry-go stack lacks good built-in error handling and introspection.
func RegisterOtelErrorCallback(callback OtelErrorCallback) {
	otelErrorCallbacks = append(otelErrorCallbacks, callback)
}

func ClearOtelErrorCallbacks() {
	otelErrorCallbacks = nil
}

// For testing use where re-configuration is desired, provide for tearing down and resetting the global otel
// setup. Note that the first-time behaviour of SetErrorHandler re-sending errors to the first-registered
// handler cannot be repeated.
func ResetGlobalOTELSetup(registerer prometheus.Registerer) {
	otel.SetLogger(logr.Discard())
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {}))
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	ClearOtelErrorCallbacks()
	promOtelMetrics.unregister(registerer)
	promOtelMetrics = nil
	isConfigured = false
}

// Create a new OpenTelmetry resource definition, merging defaults, vars discovered from the
// OTEL_RESOURCE_ATTRIBUTES environment variable, and any additional attributes provided in
// the config. The resource will include the service name and version by default, and any
// additional attributes provided in the config will be merged in.
func NewOTELResource(ctx context.Context, attrs map[string]string) (*resource.Resource, error) {
	res, err := resource.New(
		ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(DefaultOTELServiceName),
			semconv.ServiceVersionKey.String(version.Version),
		),
		resource.WithProcessPID(),
		resource.WithProcessRuntimeDescription(),
		resource.WithTelemetrySDK(),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, fmt.Errorf("while creating otel resource attributes: %w", err)
	}
	// Add resource attributes from the config, which may include things like service.instance.id that are useful for distinguishing between different instances in the same environment.
	for k, v := range attrs {
		res, err = resource.Merge(res, resource.NewWithAttributes(semconv.SchemaURL,
			attribute.KeyValue{
				Key:   attribute.Key(k),
				Value: attribute.StringValue(v),
			},
		))
		if err != nil {
			return nil, fmt.Errorf("while adding otel resource attribute %s=%s: %w", k, v, err)
		}
	}
	return res, nil
}
