package storage_ng

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

func NewTestStorage(t testing.TB) (Storage, test.Closer) {
	directory := test.NewTemporaryDirectory("test_storage", t)
	persistence, err := NewDiskPersistence(directory.Path(), 1024)
	if err != nil {
		t.Fatal("Error opening disk persistence: ", err)
	}
	o := &MemorySeriesStorageOptions{
		Persistence:            persistence,
		MemoryEvictionInterval: time.Minute,
		MemoryRetentionPeriod:  time.Hour,
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
