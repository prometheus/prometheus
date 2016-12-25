package testutil

import (
	"io/ioutil"
	"os"

	"github.com/fabxc/tsdb"
	"github.com/prometheus/prometheus/storage"
)

// NewTestStorage returns a new storage for testing purposes
// that removes all associated files on closing.
func NewTestStorage(t T) storage.Storage {
	dir, err := ioutil.TempDir("", "test_storage")
	if err != nil {
		t.Fatalf("Opening test dir failed: %s", errs)
	}
	db, err := tsdb.Open(dir)
	if err != nil {
		t.Fatalf("Opening test storage failed: %s", err)
	}
	return testStorage{DB: db, dir: dir}
}

type testStorage struct {
	*tsdb.DB
	dir string
}

func (s testStorage) Close() error {
	if err := s.db.Close(); err != nil {
		return err
	}
	return os.RemoveAll(s.dir)
}
