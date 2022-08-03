// Copyright 2021 The Prometheus Authors
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

package tracing

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/version"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/credentials"

	"github.com/prometheus/prometheus/config"
)

const serviceName = "prometheus"

// Manager is capable of building, (re)installing and shutting down
// the tracer provider.
type Manager struct {
	logger       log.Logger
	done         chan struct{}
	config       config.TracingConfig
	shutdownFunc func() error
}

// NewManager creates a new tracing manager.
func NewManager(logger log.Logger) *Manager {
	return &Manager{
		logger: logger,
		done:   make(chan struct{}),
	}
}

// Run starts the tracing manager. It registers the global text map propagator and error handler.
// It is blocking.
func (m *Manager) Run() {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetErrorHandler(otelErrHandler(func(err error) {
		level.Error(m.logger).Log("msg", "OpenTelemetry handler returned an error", "err", err)
	}))
	<-m.done
}

// ApplyConfig takes care of refreshing the tracing configuration by shutting down
// the current tracer provider (if any is registered) and installing a new one.
func (m *Manager) ApplyConfig(cfg *config.Config) error {
	// Update only if a config change is detected. If TLS configuration is
	// set, we have to restart the manager to make sure that new TLS
	// certificates are picked up.
	var blankTLSConfig config_util.TLSConfig
	if reflect.DeepEqual(m.config, cfg.TracingConfig) && m.config.TLSConfig == blankTLSConfig {
		return nil
	}

	if m.shutdownFunc != nil {
		if err := m.shutdownFunc(); err != nil {
			return fmt.Errorf("failed to shut down the tracer provider: %w", err)
		}
	}

	// If no endpoint is set, assume tracing should be disabled.
	if cfg.TracingConfig.Endpoint == "" {
		m.config = cfg.TracingConfig
		m.shutdownFunc = nil
		otel.SetTracerProvider(trace.NewNoopTracerProvider())
		level.Info(m.logger).Log("msg", "Tracing provider uninstalled.")
		return nil
	}

	tp, shutdownFunc, err := buildTracerProvider(context.Background(), cfg.TracingConfig)
	if err != nil {
		return fmt.Errorf("failed to install a new tracer provider: %w", err)
	}

	m.shutdownFunc = shutdownFunc
	m.config = cfg.TracingConfig
	otel.SetTracerProvider(tp)

	level.Info(m.logger).Log("msg", "Successfully installed a new tracer provider.")
	return nil
}

// Stop gracefully shuts down the tracer provider and stops the tracing manager.
func (m *Manager) Stop() {
	defer close(m.done)

	if m.shutdownFunc == nil {
		return
	}

	if err := m.shutdownFunc(); err != nil {
		level.Error(m.logger).Log("msg", "failed to shut down the tracer provider", "err", err)
	}

	level.Info(m.logger).Log("msg", "Tracing manager stopped")
}

type otelErrHandler func(err error)

func (o otelErrHandler) Handle(err error) {
	o(err)
}

// buildTracerProvider return a new tracer provider ready for installation, together
// with a shutdown function.
func buildTracerProvider(ctx context.Context, tracingCfg config.TracingConfig) (trace.TracerProvider, func() error, error) {
	client, err := getClient(tracingCfg)
	if err != nil {
		return nil, nil, err
	}

	exp, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, nil, err
	}

	// Create a resource describing the service and the runtime.
	res, err := resource.New(
		ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(version.Version),
		),
		resource.WithProcessRuntimeDescription(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, nil, err
	}

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithSampler(tracesdk.ParentBased(
			tracesdk.TraceIDRatioBased(tracingCfg.SamplingFraction),
		)),
		tracesdk.WithResource(res),
	)

	return tp, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := tp.Shutdown(ctx)
		if err != nil {
			return err
		}

		return nil
	}, nil
}

// getClient returns an appropriate OTLP client (either gRPC or HTTP), based
// on the provided tracing configuration.
func getClient(tracingCfg config.TracingConfig) (otlptrace.Client, error) {
	var client otlptrace.Client
	switch tracingCfg.ClientType {
	case config.TracingClientGRPC:
		opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(tracingCfg.Endpoint)}
		if tracingCfg.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		} else {
			// Use of TLS Credentials forces the use of TLS. Therefore it can
			// only be set when `insecure` is set to false.
			tlsConf, err := config_util.NewTLSConfig(&tracingCfg.TLSConfig)
			if err != nil {
				return nil, err
			}
			opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsConf)))
		}
		if tracingCfg.Compression != "" {
			opts = append(opts, otlptracegrpc.WithCompressor(tracingCfg.Compression))
		}
		if len(tracingCfg.Headers) != 0 {
			opts = append(opts, otlptracegrpc.WithHeaders(tracingCfg.Headers))
		}
		if tracingCfg.Timeout != 0 {
			opts = append(opts, otlptracegrpc.WithTimeout(time.Duration(tracingCfg.Timeout)))
		}

		client = otlptracegrpc.NewClient(opts...)
	case config.TracingClientHTTP:
		opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(tracingCfg.Endpoint)}
		if tracingCfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		// Currently, OTEL supports only gzip compression.
		if tracingCfg.Compression == config.GzipCompression {
			opts = append(opts, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
		}
		if len(tracingCfg.Headers) != 0 {
			opts = append(opts, otlptracehttp.WithHeaders(tracingCfg.Headers))
		}
		if tracingCfg.Timeout != 0 {
			opts = append(opts, otlptracehttp.WithTimeout(time.Duration(tracingCfg.Timeout)))
		}

		tlsConf, err := config_util.NewTLSConfig(&tracingCfg.TLSConfig)
		if err != nil {
			return nil, err
		}
		opts = append(opts, otlptracehttp.WithTLSClientConfig(tlsConf))

		client = otlptracehttp.NewClient(opts...)
	}

	return client, nil
}
