package logging

import (
	"io"
	"log/slog"
	"os"
	"testing"

	_ "go.opentelemetry.io/otel/log/noop"
)

// NewTestNoopOtelLogManager creates a new otelLogManager with a noop logger for testing purposes. This modifies
// the process-wide OTEL_LOGS_EXPORTER environment variable
// so side effects are possible on other OpenTelemetry components that may run loggers. It cannot restore the
// configuration to its previous state, since ApplyConfig would clobber it.
func NewTestNoopOtelLogManager() *otelLogManager {
	os.Setenv("OTEL_LOGS_EXPORTER", "noop")
	return &otelLogManager{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func Test_NoopOtelLogManager(t *testing.T) {
	otelLogMgr := NewTestNoopOtelLogManager()
	logger := otelLogMgr.Handler("test_logger")
	if logger == nil {
		t.Fatal("expected non-nil logger from otelLogManager")
	}
}

// TODO add a log manager that can capture logs and verify output
