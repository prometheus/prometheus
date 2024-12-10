// Copyright 2020 The Prometheus Authors
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

package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/prometheus/common/promslog"
)

// JSONFileLogger represents a logger that writes JSON to a file. It implements the promql.QueryLogger interface.
type JSONFileLogger struct {
	logger *slog.Logger
	file   *os.File
}

// NewJSONFileLogger returns a new JSONFileLogger.
func NewJSONFileLogger(s string) (*JSONFileLogger, error) {
	if s == "" {
		return nil, nil
	}

	f, err := os.OpenFile(s, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return nil, fmt.Errorf("can't create json log file: %w", err)
	}

	jsonFmt := &promslog.AllowedFormat{}
	_ = jsonFmt.Set("json")
	return &JSONFileLogger{
		logger: promslog.New(&promslog.Config{Format: jsonFmt, Writer: f}),
		file:   f,
	}, nil
}

// Close closes the underlying file. It implements the promql.QueryLogger interface.
func (l *JSONFileLogger) Close() error {
	return l.file.Close()
}

// With calls the `With()` method on the underlying `log/slog.Logger` with the
// provided msg and args. It implements the promql.QueryLogger interface.
func (l *JSONFileLogger) With(args ...any) {
	l.logger = l.logger.With(args...)
}

// Log calls the `Log()` method on the underlying `log/slog.Logger` with the
// provided msg and args. It implements the promql.QueryLogger interface.
func (l *JSONFileLogger) Log(ctx context.Context, level slog.Level, msg string, args ...any) {
	l.logger.Log(ctx, level, msg, args...)
}
