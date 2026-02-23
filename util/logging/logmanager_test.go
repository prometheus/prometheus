package logging

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/config"
	"github.com/stretchr/testify/require"
)

// TestLogManagerCreation excersises NewTestLogManager for the noop log manager
// intended for test use.
func TestTestLogManagerCreation(t *testing.T) {
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

func TestLogManagerQueryFileLogging(t *testing.T) {

	cfg := ConfigForTest(nil)
	td := t.TempDir()
	qlf := path.Join(td, "test_query.log")
	cfg.GlobalConfig.QueryLogFile = qlf

	const testLogMsg = "test query log message"

	manager := NewManager(slog.New(slog.DiscardHandler))
	go manager.Run()
	manager.ApplyConfig(&cfg)

	ql := manager.NewQueryLogger()
	require.True(t, ql.Enabled(context.Background(), slog.LevelInfo))

	logctx := context.Background()
	pc, _, _, _ := runtime.Caller(0)
	ql.Handle(logctx, slog.NewRecord(time.Now(), slog.LevelInfo, testLogMsg, pc))

	manager.Stop()

	// Check that the log file was created and contains the expected log message.
	data, err := os.ReadFile(qlf)
	if err != nil {
		t.Fatalf("unexpected error reading query log file: %v", err)
	}
	logContent := string(data)
	expectedSubstring := testLogMsg
	if !strings.Contains(logContent, expectedSubstring) {
		t.Fatalf("expected log content to contain %q, got: %s", expectedSubstring, logContent)
	}

	// Check that the log content is valid JSON with the expected fields.
	var logEntry map[string]interface{}
	if err := json.Unmarshal(data, &logEntry); err != nil {
		t.Fatalf("expected log content to be valid JSON, got error: %v", err)
	}
	require.Equal(t, logEntry["msg"], testLogMsg)
	require.Equal(t, "INFO", logEntry["level"])
	if _, ok := logEntry["time"]; !ok {
		t.Fatal("expected log entry to have a 'time' field")
	}
}

func TestLogManagerScrapeFileLogging(t *testing.T) {

	cfg := ConfigForTest(nil)
	td := t.TempDir()
	slf := path.Join(td, "test_scrape.log")
	cfg.GlobalConfig.ScrapeFailureLogFile = slf

	const testLogMsg = "test scrape log message"

	manager := NewManager(slog.New(slog.DiscardHandler))
	go manager.Run()
	manager.ApplyConfig(&cfg)

	sl := manager.NewScrapeFailureLogger("test_job", slf)
	require.True(t, sl.Enabled(context.Background(), slog.LevelInfo))

	logctx := context.Background()
	pc, _, _, _ := runtime.Caller(0)
	sl.Handle(logctx, slog.NewRecord(time.Now(), slog.LevelInfo, testLogMsg, pc))

	manager.Stop()

	// Check that the log file was created and contains the expected log message.
	data, err := os.ReadFile(slf)
	if err != nil {
		t.Fatalf("unexpected error reading scrape failure log file: %v", err)
	}
	logContent := string(data)
	expectedSubstring := testLogMsg
	if !strings.Contains(logContent, expectedSubstring) {
		t.Fatalf("expected log content to contain %q, got: %s", expectedSubstring, logContent)
	}
}

func TestLogFileRotation(t *testing.T) {

	cfg := ConfigForTest(nil)
	td := t.TempDir()
	qlf := path.Join(td, "test_query.log")
	cfg.GlobalConfig.QueryLogFile = qlf

	const testLogMsg1 = "test query log message before rotation"
	const testLogMsg2 = "test query log message after reload, before rotation"
	const testLogMsg3 = "test query log message after rotation before new logger"
	const testLogMsg4 = "test query log message after new logger"

	manager := NewManager(slog.New(slog.DiscardHandler))
	go manager.Run()
	manager.ApplyConfig(&cfg)

	ql := manager.NewQueryLogger()
	require.True(t, ql.Enabled(context.Background(), slog.LevelInfo))

	logctx := context.Background()

	pc, _, _, _ := runtime.Caller(0)
	ql.Handle(logctx, slog.NewRecord(time.Now(), slog.LevelInfo, testLogMsg1, pc))

	// A reload and new handler creation without a log file change should not cause any issues.
	// The new handler will seek to the end of the file.
	ql.Close()
	manager.ApplyConfig(&cfg)
	ql = manager.NewQueryLogger()
	pc, _, _, _ = runtime.Caller(0)
	ql.Handle(logctx, slog.NewRecord(time.Now(), slog.LevelInfo, testLogMsg2, pc))

	// Rotate the log file by applying a new config with a different log file path.
	qlf2 := path.Join(td, "test_query_rotated.log")
	cfg.GlobalConfig.QueryLogFile = qlf2
	manager.ApplyConfig(&cfg)

	// The existing logger still points to the old file, which is not closed.
	pc, _, _, _ = runtime.Caller(0)
	ql.Handle(logctx, slog.NewRecord(time.Now(), slog.LevelInfo, testLogMsg3, pc))

	// Re-create the logger to pick up the new file path. The old logger is closed before
	// the new one is opened. On *nix it'd actually be ok to open the new logger before
	// closing the old one, but that'll make Windows sad.
	ql.Close()
	ql = manager.NewQueryLogger()

	// This message will go to the new log file.
	ql.Handle(logctx, slog.NewRecord(time.Now(), slog.LevelInfo, testLogMsg4, pc))

	manager.Stop()

	// Check that the first log file contains all messages before rotation
	data1, err := os.ReadFile(qlf)
	require.NoError(t, err, "unexpected error reading first query log file")
	logContent1 := string(data1)
	require.Contains(t, logContent1, testLogMsg1, "expected first log file "+qlf+" to contain first log message")
	require.Contains(t, logContent1, testLogMsg2, "expected first log file "+qlf+" to contain second log message")
	require.Contains(t, logContent1, testLogMsg3, "expected first log file "+qlf+" to contain third log message")
	require.NotContains(t, logContent1, testLogMsg4, "expected first log file "+qlf+" to not contain fourth log message after rotation")

	// Check that the second log file contains only the fourth message, after rotation
	data2, err := os.ReadFile(qlf2)
	require.NoError(t, err, "unexpected error reading second query log file")
	logContent2 := string(data2)
	require.Contains(t, logContent2, testLogMsg4, "expected second log file "+qlf2+" to contain fourth log message after rotation")
	require.NotContains(t, logContent2, testLogMsg1, "expected second log file "+qlf2+" to not contain first log message")
	require.NotContains(t, logContent2, testLogMsg2, "expected second log file "+qlf2+" to not contain second log message")
	require.NotContains(t, logContent2, testLogMsg3, "expected second log file "+qlf2+" to not contain third log message")
}

// Verify that the query log can be rotated in-place by renaming it aside. This will only work on
// non-windows platforms, as Windows does not permit an open file to be renamed.
func TestLogFileRotationInplace(t *testing.T) {
	// This test is platform-specific and should be skipped on Windows.
	if runtime.GOOS == "windows" {
		t.Skip("skipping in-place log rotation test on Windows")
	}

	cfg := ConfigForTest(nil)
	td := t.TempDir()
	qlf := path.Join(td, "test_query.log")
	cfg.GlobalConfig.QueryLogFile = qlf

	const testLogMsg1 = "test query log message before rotation"
	const testLogMsg2 = "test query log message after rotation before new logger"
	const testLogMsg3 = "test query log message after new logger"

	manager := NewManager(slog.New(slog.DiscardHandler))
	go manager.Run()
	manager.ApplyConfig(&cfg)

	ql := manager.NewQueryLogger()
	require.True(t, ql.Enabled(context.Background(), slog.LevelInfo))

	logctx := context.Background()
	pc, _, _, _ := runtime.Caller(0)
	ql.Handle(logctx, slog.NewRecord(time.Now(), slog.LevelInfo, testLogMsg1, pc))

	// Rotate the log file by renaming it aside.
	qlfRotated := path.Join(td, "test_query_rotated.log")
	require.NoError(t, os.Rename(qlf, qlfRotated), "unexpected error renaming log file for rotation")
	require.NoFileExistsf(t, qlf, "expected log file %s to not exist after rotation", qlf)

	// Trigger the file to be reopened
	manager.ApplyConfig(&cfg)

	// The existing logger still points to the old file, which is not closed; this will append to
	// the renamed file.
	pc, _, _, _ = runtime.Caller(0)
	ql.Handle(logctx, slog.NewRecord(time.Now(), slog.LevelInfo, testLogMsg2, pc))

	// Re-create the logger to pick up the new file path.
	ql.Close()
	ql = manager.NewQueryLogger()

	// The log file must now have been re-created
	require.FileExistsf(t, qlf, "expected log file %s to exist after rotation", qlf)

	// And log messages must go to it, not the rotated file.
	ql.Handle(logctx, slog.NewRecord(time.Now(), slog.LevelInfo, testLogMsg3, pc))

	manager.Stop()

	// Check that the first log file contains the first and second messages.
	data1, err := os.ReadFile(qlfRotated)
	require.NoError(t, err, "unexpected error reading first query log file")
	logContent1 := string(data1)
	require.Contains(t, logContent1, testLogMsg1, "expected first log file to contain first log message")
	require.Contains(t, logContent1, testLogMsg2, "expected first log file to contain second log message")
	require.NotContains(t, logContent1, testLogMsg3, "expected first log file to not contain third log message")

	// Check that the second log file contains the third message and not the first two.
	data2, err := os.ReadFile(qlf)
	require.NoError(t, err, "unexpected error reading second query log file")
	logContent2 := string(data2)
	require.Contains(t, logContent2, testLogMsg3, "expected second log file to contain third log message")
	require.NotContains(t, logContent2, testLogMsg1, "expected second log file to not contain first log message")
	require.NotContains(t, logContent2, testLogMsg2, "expected second log file to not contain second log message")
}
