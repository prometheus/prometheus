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
	"io/ioutil"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/testutil"
)

// New returns a new TestStorage for testing purposes
// that removes all associated files on closing.
func New(t testutil.T) *TestStorage {
	dir, err := ioutil.TempDir("", "test_storage")
	require.NoError(t, err, "unexpected error while opening test directory")

	// Tests just load data for a series sequentially. Thus we
	// need a long appendable window.
	opts := tsdb.DefaultOptions()
	opts.MinBlockDuration = int64(24 * time.Hour / time.Millisecond)
	opts.MaxBlockDuration = int64(24 * time.Hour / time.Millisecond)
	db, err := tsdb.Open(dir, nil, nil, opts, tsdb.NewDBStats())
	require.NoError(t, err, "unexpected error while opening test storage")
	reg := prometheus.NewRegistry()
	eMetrics := tsdb.NewExemplarMetrics(reg)

	es, err := tsdb.NewCircularExemplarStorage(10, eMetrics)
	require.NoError(t, err, "unexpected error while opening test exemplar storage")
	return &TestStorage{DB: db, exemplarStorage: es, dir: dir}
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

func (s TestStorage) AppendExemplar(ref uint64, l labels.Labels, e exemplar.Exemplar) (uint64, error) {
	return ref, s.exemplarStorage.AddExemplar(l, e)
}
