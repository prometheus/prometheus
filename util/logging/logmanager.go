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

const (
	queryLoggerName         = "query"
	scrapeFailureLoggerName = "scrape_failure"
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
//
// Do NOT pass concrete types of logging.Manager around; instead, pass interfaces exposing
// appropriate subsets of its functionality.
type Manager struct {
	// The OTLP query log exporter writes its own logs to the slog stream.
	logger       *slog.Logger
	done         chan struct{}
	shutdownFunc func() error

	// Copy of the last-applied configuration for the logging provider.
	config config.LoggingConfig

	// For historical reasons, the query and scrape log file paths are
	// configured in the global config. This manager takes over the management
	// of those options, but for BC reasons, they are not moved.
	queryLogFile         string
	scrapeFailureLogFile string

	otelLogMgr otelLogManager

	isRunning bool
}

// Create a new Manager. The passed Logger is for the manager's own logs,
// and has nothing to do with the output OTLP log stream. Generally
// only one manager should exist at a time.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		logger:     logger,
		done:       make(chan struct{}),
		otelLogMgr: NewOTELLogManager(logger),
		isRunning:  false,
	}
}

// Run starts the logging manager. Closes the done channel on return.
func (m *Manager) Run() {
	m.isRunning = true
	m.logger.Debug("Logging manager started")
	<-m.done
}

func (m *Manager) IsRunning() bool {
	return m.isRunning
}

// ApplyConfig takes care of refreshing the logging configuration by shutting down
// the current OpenTelemetry logging provider (if any is registered) and installing
// a new one.
//
// The consuming query engine and scrape manager will refresh their loggers in their
// own config update requests, which must occur after this has run.
func (m *Manager) ApplyConfig(cfg *config.Config) error {
	m.logger.Debug("Logging manager configuration reloading")

	m.queryLogFile = cfg.GlobalConfig.QueryLogFile
	m.scrapeFailureLogFile = cfg.GlobalConfig.ScrapeFailureLogFile

	m.otelLogMgr.ApplyConfig(cfg.LoggingConfig.OTELLoggingConfig)

	m.logger.Debug("Logging manager configuration reloaded")

	m.config = cfg.LoggingConfig

	return nil
}

// Stop gracefully shuts down the logging provider and stops the logging manager.
func (m *Manager) Stop() {

	m.logger.Debug("Logging manager shutting down...")

	defer close(m.done)

	if m.shutdownFunc != nil {
		if err := m.shutdownFunc(); err != nil {
			m.logger.Error("failed to shut down the opentelemetry logging provider", "err", err)
		}
	}

	m.isRunning = false
	m.logger.Info("OpenTelemetry logging manager stopped")
}

// Request log flushes for any active OpenTelemetry log destinations. Non-blocking.
func (m *Manager) Flush() {
	m.logger.Debug("Flushing OpenTelemetry logs...")
	m.otelLogMgr.Flush()
}

// Test whether a given log destination is included in the log output configuration
// used by the OTLP logger.
func (m *Manager) isIncluded(include string) bool {
	for _, includeStr := range m.config.Include {
		if includeStr == include {
			return true
		}
	}
	return false
}

// Simplified type for the manager to return new query logger instances, reopening
// the query log file and creating a new OTLP logger with updated configuration
// as needed.
type QueryLoggerFactory interface {
	NewQueryLogger() CloseableLogger
}

// Implements QueryLoggerFactory
func (m *Manager) NewQueryLogger() CloseableLogger {
	queryLoggers := []CloseableLogger{}

	if m.isIncluded(queryLoggerName) {
		otlpLogger := m.otelLogMgr.Handler(queryLoggerName)
		if otlpLogger != nil {
			queryLoggers = append(queryLoggers, otlpLogger)
		}
	}

	if m.queryLogFile != "" {
		l, err := NewJSONFileLogger(m.queryLogFile)
		if err != nil {
			m.logger.Error("failed to create query file logger at %s", "path", m.queryLogFile, "err", err)
		} else {
			queryLoggers = append(queryLoggers, l)
		}
	}

	return NewMultiLogger(queryLoggers...)
}

// Simplified type for the manager to return new scrape failure logger instances, reopening
// the scrape failure log file and creating a new OTLP logger with updated configuration
// as needed.
type ScrapeFailureLoggerFactory interface {
	NewScrapeFailureLogger(jobName string, logfile string) CloseableLogger
}

// Implements ScrapeFailureLoggerFactory
// TODO inject scrape config name, job name, instance name (?) etc
func (m *Manager) NewScrapeFailureLogger(jobName string, logfile string) CloseableLogger {
	queryLoggers := []CloseableLogger{}

	if jobName == "" {
		m.logger.Warn("scrape failure logger requested with empty-string as job name, using 'unknown_job'", "logfile", logfile)
		jobName = "unknown_job"
	}

	if m.isIncluded(scrapeFailureLoggerName) {
		// TODO inject the job name as an attribute
		otlpLogger := m.otelLogMgr.Handler(scrapeFailureLoggerName)
		if otlpLogger != nil {
			queryLoggers = append(queryLoggers, otlpLogger)
		}
	}

	// TODO: if a scrape job doesn't set a specific log file, do we fall back to the default global
	// scrape failure log?
	if logfile == "" {
		// the scrape log manager is supposed to ensure that the scrape log is never empty
		m.logger.Warn("scrape logger requested with empty-string as filename", "job", jobName)
	}
	if logfile != "" {
		l, err := NewJSONFileLogger(logfile)
		if err != nil {
			m.logger.Error("failed to create scrape failure file logger at %s", "path", logfile, "err", err)
		} else {
			queryLoggers = append(queryLoggers, l)
		}
	}

	return NewMultiLogger(queryLoggers...)
}

// Extend slog.Logger with the ability to close the logger. Necessary to support
// file-based loggers with updateable paths and log rotation.
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

// Test helpers for other packages to consume.

// Use ConfigForTest to create a config for the logging.Manager tests.
func ConfigForTest(loggingConfig *config.LoggingConfig) config.Config {
	config := config.Config{}
	if loggingConfig != nil {
		config.LoggingConfig = *loggingConfig
	}
	return config
}

// NewTestLogManager creates a pre-configured Manager for testing purposes. The optional
// config can be used to set specific logging options, but if nil is passed, a default config will be used.
// The initial config is immediately applied and the manager is started, so the returned manager is ready to use.
// A noop slog is used for the manager's own logs.
func NewTestLogManager(config *config.Config) *Manager {
	if config == nil {
		defaultConfig := ConfigForTest(nil)
		config = &defaultConfig
	}

	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := NewManager(discard)
	go manager.Run()
	manager.ApplyConfig(config)
	return manager
}
