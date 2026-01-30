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

package teststorage

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb"
)

type Option func(opt *tsdb.Options)

// New returns a new TestStorage for testing purposes
// that removes all associated files on closing.
//
// Caller does not need to close the TestStorage after use, it's deferred via t.Cleanup.
func New(t testing.TB, o ...Option) *TestStorage {
	s, err := NewWithError(o...)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = s.Close() // Ignore errors, as it could be a double close.
	})
	return s
}

// NewWithError returns a new TestStorage for user facing tests, which reports
// errors directly.
//
// It's a caller responsibility to close the TestStorage after use.
func NewWithError(o ...Option) (*TestStorage, error) {
	// Tests just load data for a series sequentially. Thus we
	// need a long appendable window.
	opts := tsdb.DefaultOptions()
	opts.MinBlockDuration = int64(24 * time.Hour / time.Millisecond)
	opts.MaxBlockDuration = int64(24 * time.Hour / time.Millisecond)
	opts.RetentionDuration = 0
	opts.OutOfOrderTimeWindow = 0

	// Enable exemplars storage by default.
	opts.EnableExemplarStorage = true
	opts.MaxExemplars = 1e5

	for _, opt := range o {
		opt(opts)
	}

	dir, err := os.MkdirTemp("", "test_storage")
	if err != nil {
		return nil, fmt.Errorf("opening test directory: %w", err)
	}

	db, err := tsdb.Open(dir, nil, nil, opts, tsdb.NewDBStats())
	if err != nil {
		return nil, fmt.Errorf("opening test storage: %w", err)
	}
	return &TestStorage{DB: db, dir: dir}, nil
}

type TestStorage struct {
	*tsdb.DB
	dir string
}

func (s TestStorage) Close() error {
	if err := s.DB.Close(); err != nil {
		return err
	}
	return os.RemoveAll(s.dir)
}
