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
//
// This package provides an adapter to use OpenTelemetry's log API as a promql.QueryLogger,
// allowing query logs to be exported via OTLP to any compatible backend. It hooks into
// Prometheus's SIGHUP handler for runtime configuration reloading.
//
// Only the query logger currently uses OpenTelemetry native logging, the rest
// of Prometheus continues to use log/slog. An adapter could be added to bridge
// slog logs into OpenTelemetry as well, but as most environments already consume
// logs from stdout there's little benefit to the added in-process overhead of doing so, and it
// makes things like ensuring we flush logs just before shutdown more complicated.
//
// This is partly based on tracing/tracing.go.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"time"
	"weak"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/tracing"
	otelslog "go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otel_log_api "go.opentelemetry.io/otel/log"
	otel_log_sdk "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"google.golang.org/grpc/credentials"
)

type otelLogManager struct {
	logger         *slog.Logger
	shutdownFunc   func() error
	config         config.OTELLoggingConfig
	loggerProvider *otel_log_api.LoggerProvider
	handlers       map[string]weak.Pointer[closeableLogger]
}

func NewOTELLogManager(logger *slog.Logger) otelLogManager {
	return otelLogManager{
		logger: logger,
	}
}

func (m *otelLogManager) Start() {
	// FIXME: need to set an otel error handler, but it's currently only done within the tracing
	// stack. Factor it out to a common handler.
}

func (m *otelLogManager) Stop() error {
	if m.shutdownFunc != nil {
		return m.shutdownFunc()
	}
	return nil
}

// Obtain a slog.Handler for an OpenTelemetry logger with the given name. This
// handler will be updated to point to the current logger provider whenever ApplyConfig is called,
// or a no-op logger if no provider is currently configured.
func (m *otelLogManager) Handler(name string) CloseableLogger {
	if p, ok := m.handlers[name]; ok {
		h := p.Value()
		if h != nil {
			// A handler already exists for this name and the weak pointer is still valid, so we can return it.
			return h
		}
		// The weak pointer has been collected, so we need to create a new handler and update the pointer.
		delete(m.handlers, name)
	}

	h := closeableLogger{
		Handler: m.newHandler(name),
		closeFunc: func() error {
			// No resources to clean up on the handler itself; we rely on the
			// provider's shutdown to clean up resources. TODO it might be worth
			// adding a Flush() in future. There's no need to unregister these
			// objets on close as weak references point to them, and the GC
			// will take care of it.
			return nil
		},
	}
	m.handlers[name] = weak.Make[closeableLogger](&h)
	return h
}

// Obtain a new slog.Handler that writes to an OpenTelemetry logger
// with the given name, or if no otel provider is created, a noop
// logger. The handler is not cached, and not updated on config reload,
// so Handler() should be used to get a logger for external use.
func (m *otelLogManager) newHandler(name string) slog.Handler {
	if m.loggerProvider != nil {
		return otelslog.NewHandler(
			name,
			otelslog.WithLoggerProvider(*m.loggerProvider),
		)
	}
	return noopHandler
}

// Enabled returns true if and only if the manager has an active logger provider.
func (m *otelLogManager) Enabled() bool {
	return m.loggerProvider != nil
}

// ApplyConfig applies the provided OpenTelemetry logging configuration.
// If the configuration is unchanged it may still restart the provider to ensure
// that TLS certificates are reloaded.
//
// The provider keeps track of handlers it has issued and replaces the embedded
// logger within each one, so changes take effect immediately.
//
// TODO unify with the tracing manager's ApplyConfig as much as feasible
func (m *otelLogManager) ApplyConfig(cfg config.OTELLoggingConfig) error {
	// Update only if a config change is detected. If TLS configuration is
	// set, we still have to restart the manager to make sure that new TLS
	// certificates are picked up.
	var blankTLSConfig config_util.TLSConfig
	if reflect.DeepEqual(m.config, cfg) && m.config.TLSConfig == blankTLSConfig {
		return nil
	}

	// The old provider will be shut down after the new one is installed to minimise
	// disruption
	shutdownOldProvider := m.shutdownFunc

	// If no endpoint is set for OpenTelemetry logging, it can still be enabled as the user
	// may configure the OpenTelemetry SDK's exporter(s) via environment variables
	// OTEL_LOGS_EXPORTER=otel OTEL_EXPORTER_OTLP_LOGS_ENDPOINT, etc.
	//
	// TODO add similar support for the tracing provider.
	//
	if !cfg.Enabled {
		m.config = cfg
		m.shutdownFunc = nil
		m.loggerProvider = nil
		// TODO actually unregister it and detach the loggers
		m.logger.Info("OpenTelemetry logging provider uninstalled.")
		return nil
	}

	lp, shutdownFunc, err := buildOtelLoggingProvider(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("failed to install a new opentelemetry logging provider: %w", err)
	}

	// TODO detect if the provider actually created successfully with at least one
	// valid exporter

	m.shutdownFunc = shutdownFunc
	m.config = cfg
	m.loggerProvider = &lp

	// For all existing handlers, update them to use the new provider. This ensures that
	// all handlers issued by this manager will point to the current provider before the old
	// one is shut down. This indirection allows us to avoid the need to call every consumer
	// to reload their logger(s).
	for name, p := range m.handlers {
		h := p.Value()
		if h == nil {
			// The weak pointer has been collected, so we can skip updating this handler.
			delete(m.handlers, name)
			continue
		}
		h.Handler = m.newHandler(name)
	}

	m.logger.Info("Successfully installed a new OpenTelemetry logging provider.")

	// The old provider is intentionally shut down only after the new one is installed.
	if shutdownOldProvider != nil {
		if err := shutdownOldProvider(); err != nil {
			m.logger.Warn("failed to shut down the old otel logging provider", "err", err)
		}
	}
	return nil
}

// buildLoggingProvider return a new opentelemetry logging provider ready for installation, together
// with a shutdown function.
func buildOtelLoggingProvider(ctx context.Context, otelLoggingCfg config.OTELLoggingConfig) (otel_log_api.LoggerProvider, func() error, error) {

	// Create a resource describing the service and the runtime.
	// TODO unify this with the tracing provider
	// TODO verify that environment variable service name overrides the one hardcoded
	// here
	res, err := resource.New(
		ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(tracing.OTELServiceName),
			semconv.ServiceVersionKey.String(version.Version),
		),
		resource.WithProcessRuntimeDescription(),
		resource.WithProcessPID(),
		resource.WithTelemetrySDK(),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, nil, err
	}

	providerOpts := []otel_log_sdk.LoggerProviderOption{
		otel_log_sdk.WithResource(res),
	}

	// If an exporter is explicitly configured, add it to the
	// provider options. Otherwise the SDK's environment-based
	// auto-configuration should pick one. The main downside
	// of env-var based auto-configuration is that it cannot
	// easily be overridden at runtime.
	if otelLoggingCfg.Endpoint != "" {
		exp, err := buildExporter(ctx, otelLoggingCfg)
		if err != nil {
			return nil, nil, err
		}
		// TODO add support for non-batch processors if needed, and make
		// the batch processor options configurable
		providerOpts = append(providerOpts,
			otel_log_sdk.WithProcessor(
				otel_log_sdk.NewBatchProcessor(exp),
			),
		)
	}

	lp := otel_log_sdk.NewLoggerProvider(providerOpts...)

	return lp, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// It is important that the shutdown func use the provider instance from the
		// closure, not the one from the manager struct, as the manager may wish to
		// create a new provider and update it before shutting down the old one.
		err := lp.Shutdown(ctx)
		if err != nil {
			return err
		}

		return nil
	}, nil
}

// getExporter returns an appropriate OTLP exporter (either gRPC or HTTP), based
// on the provided logging exporter configuration.
//
// TODO unify this with the tracing getClient() as much as reasonable.
func buildExporter(ctx context.Context, otelLoggingCfg config.OTELLoggingConfig) (otel_log_sdk.Exporter, error) {
	var exporter otel_log_sdk.Exporter
	var err error

	switch otelLoggingCfg.ClientType {
	case config.OTELClientGRPC:
		opts := []otlploggrpc.Option{otlploggrpc.WithEndpoint(otelLoggingCfg.Endpoint)}
		if otelLoggingCfg.Insecure != nil && *otelLoggingCfg.Insecure {
			opts = append(opts, otlploggrpc.WithInsecure())
		} else {
			// Use of TLS Credentials forces the use of TLS. Therefore it can
			// only be set when `insecure` is set to false.
			tlsConf, err := config_util.NewTLSConfig(&otelLoggingCfg.TLSConfig)
			if err != nil {
				return nil, err
			}
			opts = append(opts, otlploggrpc.WithTLSCredentials(credentials.NewTLS(tlsConf)))
		}
		if otelLoggingCfg.Compression != nil && *otelLoggingCfg.Compression != "" {
			opts = append(opts, otlploggrpc.WithCompressor(*otelLoggingCfg.Compression))
		}
		if len(otelLoggingCfg.Headers) != 0 {
			opts = append(opts, otlploggrpc.WithHeaders(otelLoggingCfg.Headers))
		}
		if otelLoggingCfg.Timeout != nil {
			opts = append(opts, otlploggrpc.WithTimeout(time.Duration(*otelLoggingCfg.Timeout)))
		}

		exporter, err = otlploggrpc.New(ctx, opts...)
		if err != nil {
			return nil, err
		}
	case config.OTELClientHTTP:
		opts := []otlploghttp.Option{otlploghttp.WithEndpoint(otelLoggingCfg.Endpoint)}
		if otelLoggingCfg.Insecure != nil && *otelLoggingCfg.Insecure {
			opts = append(opts, otlploghttp.WithInsecure())
		} else {
			tlsConf, err := config_util.NewTLSConfig(&otelLoggingCfg.TLSConfig)
			if err != nil {
				return nil, err
			}
			opts = append(opts, otlploghttp.WithTLSClientConfig(tlsConf))
		}
		if otelLoggingCfg.Compression != nil && *otelLoggingCfg.Compression == config.GzipCompression {
			opts = append(opts, otlploghttp.WithCompression(otlploghttp.GzipCompression))
		}
		if len(otelLoggingCfg.Headers) != 0 {
			opts = append(opts, otlploghttp.WithHeaders(otelLoggingCfg.Headers))
		}
		if otelLoggingCfg.Timeout != nil {
			opts = append(opts, otlploghttp.WithTimeout(time.Duration(*otelLoggingCfg.Timeout)))
		}

		exporter, err = otlploghttp.New(ctx, opts...)
		if err != nil {
			return nil, err
		}

	}

	return exporter, nil
}
