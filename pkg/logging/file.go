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
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
)

var (
	timestampFormat = log.TimestampFormat(
		func() time.Time { return time.Now().UTC() },
		"2006-01-02T15:04:05.000Z07:00",
	)
)

// JSONFileLogger represents a logger that writes JSON to a file.
type JSONFileLogger struct {
	logger log.Logger
	file   *os.File
}

// NewJSONFileLogger returns a new JSONFileLogger.
func NewJSONFileLogger(s string) (*JSONFileLogger, error) {
	if s == "" {
		return nil, nil
	}

	f, err := os.OpenFile(s, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return nil, errors.Wrap(err, "can't create json logger")
	}

	return &JSONFileLogger{
		logger: log.With(log.NewJSONLogger(f), "ts", timestampFormat),
		file:   f,
	}, nil
}

// Close closes the underlying file.
func (l *JSONFileLogger) Close() error {
	return l.file.Close()
}

// Log calls the Log function of the underlying logger.
func (l *JSONFileLogger) Log(i ...interface{}) error {
	return l.logger.Log(i...)
}
