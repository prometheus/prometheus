package logging

import (
	"context"
	"log/slog"
	"testing"

	"github.com/prometheus/prometheus/config"
)

// TestLogManagerCreation verifies that a LogManager can be created with a custom config.
func TestLogManagerCreation(t *testing.T) {
	manager := NewTestLogManager(nil)
	if manager == nil {
		t.Fatal("expected non-nil LogManager")
	}

	config := ConfigForTest(&config.LoggingConfig{})
	if err := manager.ApplyConfig(&config); err != nil {
		t.Fatalf("unexpected error applying config: %v", err)
	}

	ql := manager.NewQueryLogger()
	if ql.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("expected fake logger to be disabled by default")
	}

	sl := manager.NewScrapeFailureLogger("test_job", "")
	if sl.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("expected fake scrape failure logger to be disabled by default")
	}
}
