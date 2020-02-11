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

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/testutil"
)

// New returns a new storage for testing purposes
// that removes all associated files on closing.
func New(t testutil.T) storage.Storage {
	dir, err := ioutil.TempDir("", "test_storage")
	if err != nil {
		t.Fatalf("Opening test dir failed: %s", err)
	}

	// Tests just load data for a series sequentially. Thus we
	// need a long appendable window.
	opts := tsdb.DefaultOptions()
	opts.MinBlockDuration = int64(24 * time.Hour / time.Millisecond)
	opts.MaxBlockDuration = int64(24 * time.Hour / time.Millisecond)
	db, err := tsdb.Open(dir, nil, nil, opts)
	if err != nil {
		t.Fatalf("Opening test storage failed: %s", err)
	}
	return testStorage{Storage: db, dir: dir}
}

type testStorage struct {
	storage.Storage
	dir string
}

func (s testStorage) Close() error {
	if err := s.Storage.Close(); err != nil {
		return err
	}
	return os.RemoveAll(s.dir)
}
