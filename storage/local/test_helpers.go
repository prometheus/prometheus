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
	t.storage.Close()
	t.directory.Close()
}

// NewTestStorage creates a storage instance backed by files in a temporary
// directory. The returned storage is already in serving state. Upon closing the
// returned test.Closer, the temporary directory is cleaned up.
func NewTestStorage(t testing.TB) (Storage, test.Closer) {
	directory := test.NewTemporaryDirectory("test_storage", t)
	persistence, err := NewDiskPersistence(directory.Path(), 1024)
	if err != nil {
		t.Fatal("Error opening disk persistence: ", err)
	}
	o := &MemorySeriesStorageOptions{
		Persistence:                persistence,
		MemoryEvictionInterval:     time.Minute,
		MemoryRetentionPeriod:      time.Hour,
		PersistencePurgeInterval:   time.Hour,
		PersistenceRetentionPeriod: 24 * 7 * time.Hour,
	}
	storage, err := NewMemorySeriesStorage(o)
	if err != nil {
		directory.Close()
		t.Fatalf("Error creating storage: %s", err)
	}

	storageStarted := make(chan bool)
	go storage.Serve(storageStarted)
	<-storageStarted

	closer := &testStorageCloser{
		storage:   storage,
		directory: directory,
	}

	return storage, closer
}
