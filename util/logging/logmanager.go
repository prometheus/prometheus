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
	"io"
	"log/slog"

	"github.com/prometheus/prometheus/config"

	// The stdout log provider can be enabled via otel configuration env-vars, but we need
	// to ensure it gets compiled and linked.
	_ "go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
)

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
	logger       *slog.Logger
	done         chan struct{}
	shutdownFunc func() error

	config config.LoggingConfig

	// For historical reasons, the query and scrape log file paths are
	// configured in the global config. This manager takes over the management
	// of those options, but for BC reasons, they are not moved.
	queryLogFile         string
	scrapeFailureLogFile string

	// The logger instances issued to the query and scrape loggers are retained
	// so that their destinations can be updated when the configuration changes.
	queryLogger         multiLogger
	scrapeFailureLogger multiLogger

	otelLogMgr otelLogManager
}

// Create a new Manager. The passed Logger is for the manager's own logs,
// and has nothing to do with the output OTLP log stream. Generally
// only one manager should exist at a time.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		logger:     logger,
		done:       make(chan struct{}),
		otelLogMgr: NewOTELLogManager(logger),
	}
}

// Run starts the logging manager. Closes the done channel on return.
func (m *Manager) Run() {
	<-m.done
}

// ApplyConfig takes care of refreshing the logging configuration by shutting down
// the current OpenTelemetry logging provider (if any is registered) and installing
// a new one.
//
// The query and scrape providers must apply their own configurations AFTER this
// function returns, to ensure that they obtain new loggers from the provider. This way
// we don't need the indirection overhead of a logger wrapper for every logged event.
func (m *Manager) ApplyConfig(cfg *config.Config) error {
	m.queryLogFile = cfg.GlobalConfig.QueryLogFile
	m.scrapeFailureLogFile = cfg.GlobalConfig.ScrapeFailureLogFile

	// The OTLP log manager caches its loggers and internally updates them to
	// point to a new log provider when a reload is required.
	m.otelLogMgr.ApplyConfig(cfg.LoggingConfig.OTELLoggingConfig)

	queryLoggers := []CloseableLogger{
		m.otelLogMgr.Handler("query"),
	}
	scrapeFailureLoggers := []CloseableLogger{
		m.otelLogMgr.Handler("scrape_failure"),
	}

	// The file-backed loggers are re-created every config apply, even if file
	// paths did not change, because Prometheus treats a SIGHUP as a signal to
	// truncate log files.
	if m.queryLogFile != "" {
		l, err := NewJSONFileLogger(m.queryLogFile)
		if err != nil {
			return err
		}
		queryLoggers = append(queryLoggers, l)
	}

	if m.scrapeFailureLogFile != "" {
		l, err := NewJSONFileLogger(m.scrapeFailureLogFile)
		if err != nil {
			return err
		}
		scrapeFailureLoggers = append(scrapeFailureLoggers, l)
	}

	// Replace the destination loggers in the multi-logger adapter instances
	// used by the query and scrape loggers.
	m.queryLogger.SetLoggers(queryLoggers...)
	m.scrapeFailureLogger.SetLoggers(scrapeFailureLoggers...)

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

func (m *Manager) QueryLogger() CloseableLogger {
	return m.queryLogger
}

// Interface to replace promql.QueryLogger and the scrape logger interface
// TODO
type CloseableLogger interface {
	slog.Handler
	io.Closer
}

// Implements CloseableLogger
type closeableLogger struct {
	slog.Handler
	closeFunc func() error
}

func (c closeableLogger) Close() error {
	if c.closeFunc != nil {
		return c.closeFunc()
	}
	return nil
}
