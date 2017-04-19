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

package testutil

import (
	"io/ioutil"
	"os"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/tsdb"
)

// NewStorage returns a new storage for testing purposes
// that removes all associated files on closing.
func NewStorage(t T) storage.Storage {
	dir, err := ioutil.TempDir("", "test_storage")
	if err != nil {
		t.Fatalf("Opening test dir failed: %s", err)
	}

	log.With("dir", dir).Debugln("opening test storage")

	db, err := tsdb.Open(dir, nil, &tsdb.Options{
		MinBlockDuration: 2 * time.Hour,
		MaxBlockDuration: 24 * time.Hour,
		AppendableBlocks: 10,
	})
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
	log.With("dir", s.dir).Debugln("closing test storage")

	if err := s.Storage.Close(); err != nil {
		return err
	}
	return os.RemoveAll(s.dir)
}
