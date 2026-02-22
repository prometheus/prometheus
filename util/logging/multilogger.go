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

package logging

import (
	"context"
	"errors"
	"log/slog"
)

// Implements promql.QueryLogger; should probably switch to using
// slog.NewMultiHandler once we can rely no go 1.26, though there's some
// question about how to handle the Close() method currently used in
// the existing interface.
type multiLogger struct {
	loggers []CloseableLogger
}

func NewMultiLogger(loggers ...CloseableLogger) multiLogger {
	return multiLogger{loggers: loggers}
}

func (q multiLogger) Enabled(ctx context.Context, level slog.Level) bool {
	for _, logger := range q.loggers {
		if logger.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (q multiLogger) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, logger := range q.loggers {
		if err := logger.Handle(ctx, r); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (q multiLogger) WithAttrs(attrs []slog.Attr) slog.Handler {
	var loggers []CloseableLogger
	for _, logger := range q.loggers {
		loggers = append(loggers, logger.WithAttrs(attrs).(CloseableLogger))
	}
	return multiLogger{loggers: loggers}
}

func (q multiLogger) WithGroup(group string) slog.Handler {
	var loggers []CloseableLogger
	for _, logger := range q.loggers {
		loggers = append(loggers, logger.WithGroup(group).(CloseableLogger))
	}
	return multiLogger{loggers: loggers}
}

func (q multiLogger) Close() error {
	var errs []error
	for _, logger := range q.loggers {
		if err := logger.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
