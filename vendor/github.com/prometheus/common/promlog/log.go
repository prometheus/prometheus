// Copyright 2017 The Prometheus Authors
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

// Package promlog defines standardised ways to initialize Go kit loggers
// across Prometheus components.
// It should typically only ever be imported by main packages.
package promlog

import (
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
)

var (
	// This timestamp format differs from RFC3339Nano by using .000 instead
	// of .999999999 which changes the timestamp from 9 variable to 3 fixed
	// decimals (.130 instead of .130987456).
	timestampFormat = log.TimestampFormat(
		func() time.Time { return time.Now().UTC() },
		"2006-01-02T15:04:05.000Z07:00",
	)
)

// AllowedLevel is a settable identifier for the minimum level a log entry
// must be have.
type AllowedLevel struct {
	s string
	o level.Option
}

func (l *AllowedLevel) String() string {
	return l.s
}

func (l *AllowedLevel) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Create a clean global config as the previous one was already populated
	// by the default due to the YAML parser behavior for empty blocks.
	var s string
	type plain string
	if err := unmarshal((*plain)(&s)); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	lo := &AllowedLevel{}
	if err := lo.Set(s); err != nil {
		return err
	}
	*l = *lo
	return nil
}

// Set updates the value of the allowed level.
func (l *AllowedLevel) Set(s string) error {
	switch s {
	case "debug":
		l.o = level.AllowDebug()
	case "info":
		l.o = level.AllowInfo()
	case "warn":
		l.o = level.AllowWarn()
	case "error":
		l.o = level.AllowError()
	default:
		return errors.Errorf("unrecognized log level %q", s)
	}
	l.s = s
	return nil
}

// AllowedFormat is a settable identifier for the output format that the logger can have.
type AllowedFormat struct {
	s string
}

func (f *AllowedFormat) String() string {
	return f.s
}

// Set updates the value of the allowed format.
func (f *AllowedFormat) Set(s string) error {
	switch s {
	case "logfmt", "json":
		f.s = s
	default:
		return errors.Errorf("unrecognized log format %q", s)
	}
	return nil
}

// Config is a struct containing configurable settings for the logger
type Config struct {
	Level  *AllowedLevel
	Format *AllowedFormat
}

// New returns a new leveled oklog logger. Each logged line will be annotated
// with a timestamp. The output always goes to stderr.
func New(config *Config) *logger {
	var l log.Logger
	if config.Format != nil && config.Format.s == "json" {
		l = log.NewJSONLogger(log.NewSyncWriter(os.Stderr))
	} else {
		l = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	}
	l = log.With(l, "ts", timestampFormat, "caller", log.DefaultCaller)

	lo := &logger{
		base:    l,
		leveled: l,
	}
	if config.Level != nil {
		lo.SetLevel(config.Level)
	}
	return lo
}

type logger struct {
	base         log.Logger
	leveled      log.Logger
	currentLevel *AllowedLevel
}

// l implements logger.Log
func (l *logger) Log(keyvals ...interface{}) error {
	return l.leveled.Log(keyvals...)
}

// SetLevel allows changing the level
func (l *logger) SetLevel(lvl *AllowedLevel) {
	if lvl != nil {
		if l.currentLevel != nil && l.currentLevel.s != lvl.s {
			l.base.Log("msg", "Log level changed", "prev", l.currentLevel, "current", lvl)
		}
		l.currentLevel = lvl
	}
	l.leveled = level.NewFilter(l.base, lvl.o)
}
