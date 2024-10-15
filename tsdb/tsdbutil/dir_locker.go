// Copyright 2021 The Prometheus Authors
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

package tsdbutil

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/prometheus/client_golang/prometheus"

	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

const (
	lockfileDisabled       = -1
	lockfileReplaced       = 0
	lockfileCreatedCleanly = 1
)

type DirLocker struct {
	logger *slog.Logger

	createdCleanly prometheus.Gauge

	releaser fileutil.Releaser
	path     string
}

// NewDirLocker creates a DirLocker that can obtain an exclusive lock on dir.
func NewDirLocker(dir, subsystem string, l *slog.Logger, r prometheus.Registerer) (*DirLocker, error) {
	lock := &DirLocker{
		logger: l,
		createdCleanly: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: fmt.Sprintf("prometheus_%s_clean_start", subsystem),
			Help: "-1: lockfile is disabled. 0: a lockfile from a previous execution was replaced. 1: lockfile creation was clean",
		}),
	}

	if r != nil {
		r.MustRegister(lock.createdCleanly)
	}

	lock.createdCleanly.Set(lockfileDisabled)

	absdir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	lock.path = filepath.Join(absdir, "lock")

	return lock, nil
}

// Lock obtains the lock on the locker directory.
func (l *DirLocker) Lock() error {
	if l.releaser != nil {
		return errors.New("DB lock already obtained")
	}

	if _, err := os.Stat(l.path); err == nil {
		l.logger.Warn("A lockfile from a previous execution already existed. It was replaced", "file", l.path)

		l.createdCleanly.Set(lockfileReplaced)
	} else {
		l.createdCleanly.Set(lockfileCreatedCleanly)
	}

	lockf, _, err := fileutil.Flock(l.path)
	if err != nil {
		return fmt.Errorf("lock DB directory: %w", err)
	}
	l.releaser = lockf
	return nil
}

// Release releases the lock. No-op if the lock is not held.
func (l *DirLocker) Release() error {
	if l.releaser == nil {
		return nil
	}

	errs := tsdb_errors.NewMulti()
	errs.Add(l.releaser.Release())
	errs.Add(os.Remove(l.path))

	l.releaser = nil
	return errs.Err()
}
