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

package teststorage

import (
	"fmt"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/testutil"
)

// New returns a new TestStorage for testing purposes
// that removes all associated files on closing.
func New(t testutil.T, outOfOrderTimeWindow ...int64) *TestStorage {
	stor, err := NewWithError(outOfOrderTimeWindow...)
	require.NoError(t, err)
	return stor
}

// NewWithError returns a new TestStorage for user facing tests, which reports
// errors directly.
func NewWithError(outOfOrderTimeWindow ...int64) (*TestStorage, error) {
	dir, err := os.MkdirTemp("", "test_storage")
	if err != nil {
		return nil, fmt.Errorf("opening test directory: %w", err)
	}

	// Tests just load data for a series sequentially. Thus we
	// need a long appendable window.
	opts := tsdb.DefaultOptions()
	opts.MinBlockDuration = int64(24 * time.Hour / time.Millisecond)
	opts.MaxBlockDuration = int64(24 * time.Hour / time.Millisecond)
	opts.RetentionDuration = 0

	// Set OutOfOrderTimeWindow if provided, otherwise use default (0)
	if len(outOfOrderTimeWindow) > 0 {
		opts.OutOfOrderTimeWindow = outOfOrderTimeWindow[0]
	} else {
		opts.OutOfOrderTimeWindow = 0 // Default value is zero
	}

	db, err := tsdb.Open(dir, nil, nil, opts, tsdb.NewDBStats())
	if err != nil {
		return nil, fmt.Errorf("opening test storage: %w", err)
	}
	reg := prometheus.NewRegistry()
	eMetrics := tsdb.NewExemplarMetrics(reg)

	es, err := tsdb.NewCircularExemplarStorage(10, eMetrics)
	if err != nil {
		return nil, fmt.Errorf("opening test exemplar storage: %w", err)
	}
	return &TestStorage{DB: db, exemplarStorage: es, dir: dir}, nil
}

type TestStorage struct {
	*tsdb.DB
	exemplarStorage tsdb.ExemplarStorage
	dir             string
}

func (s TestStorage) Close() error {
	if err := s.DB.Close(); err != nil {
		return err
	}
	return os.RemoveAll(s.dir)
}

func (s TestStorage) ExemplarAppender() storage.ExemplarAppender {
	return s
}

func (s TestStorage) ExemplarQueryable() storage.ExemplarQueryable {
	return s.exemplarStorage
}

func (s TestStorage) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	return ref, s.exemplarStorage.AddExemplar(l, e)
}
