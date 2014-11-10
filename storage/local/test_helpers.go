// Copyright 2014 Prometheus Team
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

package local

import (
	"testing"
	"time"

	"github.com/prometheus/prometheus/utility/test"
)

type testStorageCloser struct {
	storage   Storage
	directory test.Closer
}

func (t *testStorageCloser) Close() {
	t.storage.Stop()
	t.directory.Close()
}

// NewTestStorage creates a storage instance backed by files in a temporary
// directory. The returned storage is already in serving state. Upon closing the
// returned test.Closer, the temporary directory is cleaned up.
func NewTestStorage(t testing.TB) (Storage, test.Closer) {
	directory := test.NewTemporaryDirectory("test_storage", t)
	o := &MemorySeriesStorageOptions{
		MemoryEvictionInterval:     time.Minute,
		MemoryRetentionPeriod:      time.Hour,
		PersistenceRetentionPeriod: 24 * 7 * time.Hour,
		PersistenceStoragePath:     directory.Path(),
		CheckpointInterval:         time.Hour,
	}
	storage, err := NewMemorySeriesStorage(o)
	if err != nil {
		directory.Close()
		t.Fatalf("Error creating storage: %s", err)
	}

	storage.Start()

	closer := &testStorageCloser{
		storage:   storage,
		directory: directory,
	}

	return storage, closer
}
