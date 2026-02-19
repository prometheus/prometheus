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
// This is based on tracing/tracing.go.
package prometheus_otel_log

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/tracing"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"

	// The stdout log provider can be enabled via otel configuration env-vars, but we need
	// to ensure it gets compiled and linked.
	_ "go.opentelemetry.io/otel/exporters/stdout/stdoutlog"

	otel_log_api "go.opentelemetry.io/otel/log"
	otel_log_sdk "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"google.golang.org/grpc/credentials"
)

//
// TODO make this a scrape/query log manager that can manage the otlp log provider and
// the query and scrape file logs, so it can be more easily extended with other providers
// in future.
//

// Manager (re)creates and shuts down OpenTelemetry log providers
// based on the provided configuration.
//
// Usage:
//
//	m := NewManager(logger)
//	go m.Run()
//	...
//	if err := m.ApplyConfig(cfg); err != nil {
//	  // handle error
//	}
//	...
//	m.Stop()
type Manager struct {
	// The OTLP query log exporter writes its own logs to the slog stream.
	logger         *slog.Logger
	done           chan struct{}
	config         config.LoggingConfig
	shutdownFunc   func() error
	loggerProvider *otel_log_api.LoggerProvider

	// TODO add query and scrape log files here
}

// Create a new Manager. The passed Logger is for the manager's own logs,
// and has nothing to do with the output OTLP log stream. Generally
// only one manager should exist at a time.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		logger: logger,
		done:   make(chan struct{}),
	}
}

// Run starts the OpenTelemetry logging manager. Closes the done channel on return.
func (m *Manager) Run() {
	// FIXME: need to set an otel error handler, but it's currently only done within the tracing
	// stack. Factor it out to a common handler.
	<-m.done
}

// ApplyConfig takes care of refreshing the logging configuration by shutting down
// the current opentelemetry logging provider (if any is registered) and installing a new one.
//
// TODO unify with the tracing manager's ApplyConfig as much as feasible
func (m *Manager) ApplyConfig(cfg *config.Config) error {
	// Update only if a config change is detected. If TLS configuration is
	// set, we still have to restart the manager to make sure that new TLS
	// certificates are picked up.
	var blankTLSConfig config_util.TLSConfig
	if reflect.DeepEqual(m.config, cfg.LoggingConfig) && m.config.OTELLoggingConfig.TLSConfig == blankTLSConfig {
		return nil
	}

	if m.shutdownFunc != nil {
		if err := m.shutdownFunc(); err != nil {
			return fmt.Errorf("failed to shut down the otel logging provider: %w", err)
		}
	}

	// If no endpoint is set for OpenTelemetry logging, it can still be enabled as the user
	// may configure the OpenTelemetry SDK's exporter(s) via environment variables
	// OTEL_LOGS_EXPORTER=otel OTEL_EXPORTER_OTLP_LOGS_ENDPOINT, etc.
	//
	// TODO add similar support for the tracing provider.
	//
	if !cfg.LoggingConfig.OTELLoggingConfig.Enabled {
		m.config = cfg.LoggingConfig
		m.shutdownFunc = nil
		m.loggerProvider = nil
		// TODO actually unregister it and detach the loggers
		m.logger.Info("OpenTelemetry logging provider uninstalled.")
		return nil
	}

	lp, shutdownFunc, err := buildLoggingProvider(context.Background(), cfg.LoggingConfig)
	if err != nil {
		return fmt.Errorf("failed to install a new opentelemetry logging provider: %w", err)
	}

	// TODO detect if the provider actually created successfully with at least one
	// valid exporter

	m.shutdownFunc = shutdownFunc
	m.config = cfg.LoggingConfig
	m.loggerProvider = &lp

	// FIXME: actually apply the new log provider. The otel logger has no global logger provider
	// in the otel package like there is for tracing, and we also need to make sure the query
	// and scrape loggers are updated to use the new provider.
	//
	// Scrape failure loggers are set up in scrape/manager.go ApplyConfig
	// and qyery loggers are updated by a reloader defined directly on the
	// cmd/prometheus/main.go at the moment, but should be moved.

	m.logger.Info("Successfully installed a new OpenTelemetry logging provider.")
	return nil
}

// Stop gracefully shuts down the logging provider and stops the logging manager.
func (m *Manager) Stop() {
	defer close(m.done)

	if m.shutdownFunc == nil {
		return
	}

	if err := m.shutdownFunc(); err != nil {
		m.logger.Error("failed to shut down the opentelemetry logging provider", "err", err)
	}

	m.logger.Info("OpenTelemetry logging manager stopped")
}

// buildLoggingProvider return a new opentelemetry logging provider ready for installation, together
// with a shutdown function.
func buildLoggingProvider(ctx context.Context, loggingCfg config.LoggingConfig) (otel_log_api.LoggerProvider, func() error, error) {

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
	if loggingCfg.OTELLoggingConfig.Endpoint != "" {
		exp, err := getExporter(ctx, loggingCfg)
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
func getExporter(ctx context.Context, loggingCfg config.LoggingConfig) (otel_log_sdk.Exporter, error) {
	var exporter otel_log_sdk.Exporter
	var err error

	switch loggingCfg.OTELLoggingConfig.ClientType {
	case config.OTELClientGRPC:
		opts := []otlploggrpc.Option{otlploggrpc.WithEndpoint(loggingCfg.OTELLoggingConfig.Endpoint)}
		if loggingCfg.OTELLoggingConfig.Insecure != nil && *loggingCfg.OTELLoggingConfig.Insecure {
			opts = append(opts, otlploggrpc.WithInsecure())
		} else {
			// Use of TLS Credentials forces the use of TLS. Therefore it can
			// only be set when `insecure` is set to false.
			tlsConf, err := config_util.NewTLSConfig(&loggingCfg.OTELLoggingConfig.TLSConfig)
			if err != nil {
				return nil, err
			}
			opts = append(opts, otlploggrpc.WithTLSCredentials(credentials.NewTLS(tlsConf)))
		}
		if loggingCfg.OTELLoggingConfig.Compression != nil && *loggingCfg.OTELLoggingConfig.Compression != "" {
			opts = append(opts, otlploggrpc.WithCompressor(*loggingCfg.OTELLoggingConfig.Compression))
		}
		if len(loggingCfg.OTELLoggingConfig.Headers) != 0 {
			opts = append(opts, otlploggrpc.WithHeaders(loggingCfg.OTELLoggingConfig.Headers))
		}
		if loggingCfg.OTELLoggingConfig.Timeout != nil {
			opts = append(opts, otlploggrpc.WithTimeout(time.Duration(*loggingCfg.OTELLoggingConfig.Timeout)))
		}

		exporter, err = otlploggrpc.New(ctx, opts...)
		if err != nil {
			return nil, err
		}
	case config.OTELClientHTTP:
		opts := []otlploghttp.Option{otlploghttp.WithEndpoint(loggingCfg.OTELLoggingConfig.Endpoint)}
		if loggingCfg.OTELLoggingConfig.Insecure != nil && *loggingCfg.OTELLoggingConfig.Insecure {
			opts = append(opts, otlploghttp.WithInsecure())
		} else {
			tlsConf, err := config_util.NewTLSConfig(&loggingCfg.OTELLoggingConfig.TLSConfig)
			if err != nil {
				return nil, err
			}
			opts = append(opts, otlploghttp.WithTLSClientConfig(tlsConf))
		}
		if loggingCfg.OTELLoggingConfig.Compression != nil && *loggingCfg.OTELLoggingConfig.Compression == config.GzipCompression {
			opts = append(opts, otlploghttp.WithCompression(otlploghttp.GzipCompression))
		}
		if len(loggingCfg.OTELLoggingConfig.Headers) != 0 {
			opts = append(opts, otlploghttp.WithHeaders(loggingCfg.OTELLoggingConfig.Headers))
		}
		if loggingCfg.OTELLoggingConfig.Timeout != nil {
			opts = append(opts, otlploghttp.WithTimeout(time.Duration(*loggingCfg.OTELLoggingConfig.Timeout)))
		}

		exporter, err = otlploghttp.New(ctx, opts...)
		if err != nil {
			return nil, err
		}

	}

	return exporter, nil
}

// OTLPQueryLogger implements the promql/engine.go QueryLogger interface
// using OpenTelemetry's log API.
type OTLPQueryLogger struct {
	logger otel_log_api.Logger
}

func (m *Manager) NewOTLPQueryLogger() *OTLPQueryLogger {
	// If additional resource attributes are needed, they can be added here,
	// but we're going to rely on the environment-variable based
	// configuration conventions for OpenTelemetry SDKs. It'd be possible to
	// extend this with resource detectors etc, but this should be done
	// consistently with the OpenTelemetry tracing setup in tracing/tracing.go.
	if m.loggerProvider == nil {
		panic("OTLP log manager not initialized yet")
	}
	return &OTLPQueryLogger{
		logger: (*m.loggerProvider).Logger("promql-query-logger"),
	}
}

// Log implements the QueryLogger interface.
func (q *OTLPQueryLogger) Log(ctx context.Context, msg string, fields ...interface{}) {
	record := otel_log_api.Record{}
	record.SetSeverity(otel_log_api.SeverityInfo)
	record.SetBody(otel_log_api.StringValue(msg))

	// Convert fields to log record attributes
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			key := fields[i].(string)
			value := fields[i+1]
			record.AddAttributes(otel_log_api.KeyValue{
				Key:   key,
				Value: adaptFieldValue(value),
			})
		}
	}

	q.logger.Emit(ctx, record)
}

func adaptFieldValue(v interface{}) otel_log_api.Value {
	switch val := v.(type) {
	case string:
		return otel_log_api.StringValue(val)
	case int:
		return otel_log_api.Int64Value(int64(val))
	case int64:
		return otel_log_api.Int64Value(val)
	case float64:
		return otel_log_api.Float64Value(val)
	case bool:
		return otel_log_api.BoolValue(val)
	case error:
		return otel_log_api.StringValue(val.Error())
	default:
		return otel_log_api.StringValue(fmt.Sprintf("%v", val))
	}
}
