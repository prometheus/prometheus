// Copyright 2014 The Prometheus Authors
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

// NOTE ON FILENAME: Do not rename this file helpers_test.go (which might appear
// an obvious choice). We need NewTestStorage in tests outside of the local
// package, too. On the other hand, moving NewTestStorage in its own package
// would cause circular dependencies in the tests in packages local.

package local

import (
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage/local/chunk"
	"github.com/prometheus/prometheus/util/testutil"
)

type testStorageCloser struct {
	storage   Storage
	directory testutil.Closer
}

func (t *testStorageCloser) Close() {
	if err := t.storage.Stop(); err != nil {
		panic(err)
	}
	t.directory.Close()
}

// NewTestStorage creates a storage instance backed by files in a temporary
// directory. The returned storage is already in serving state. Upon closing the
// returned test.Closer, the temporary directory is cleaned up.
func NewTestStorage(t testutil.T, encoding chunk.Encoding) (*MemorySeriesStorage, testutil.Closer) {
	chunk.DefaultEncoding = encoding
	directory := testutil.NewTemporaryDirectory("test_storage", t)
	o := &MemorySeriesStorageOptions{
		TargetHeapSize:             1000000000,
		PersistenceRetentionPeriod: 24 * time.Hour * 365 * 100, // Enough to never trigger purging.
		PersistenceStoragePath:     directory.Path(),
		HeadChunkTimeout:           5 * time.Minute,
		CheckpointInterval:         time.Hour,
		SyncStrategy:               Adaptive,
	}
	storage := NewMemorySeriesStorage(o)
	storage.archiveHighWatermark = model.Latest
	if err := storage.Start(); err != nil {
		directory.Close()
		t.Fatalf("Error creating storage: %s", err)
	}

	closer := &testStorageCloser{
		storage:   storage,
		directory: directory,
	}

	return storage, closer
}

func makeFingerprintSeriesPair(s *MemorySeriesStorage, fp model.Fingerprint) fingerprintSeriesPair {
	return fingerprintSeriesPair{fp, s.seriesForRange(fp, model.Earliest, model.Latest)}
}
