package testutil

import (
	"io/ioutil"
	"os"

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
	db, err := tsdb.Open(dir)
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
