// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fileutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/tsdb/testutil"
)

func TestLocking(t *testing.T) {
	dir := testutil.NewTemporaryDirectory("test_flock", t)
	defer dir.Close()

	fileName := filepath.Join(dir.Path(), "LOCK")

	if _, err := os.Stat(fileName); err == nil {
		t.Fatalf("File %q unexpectedly exists.", fileName)
	}

	lock, existed, err := Flock(fileName)
	if err != nil {
		t.Fatalf("Error locking file %q: %s", fileName, err)
	}
	if existed {
		t.Errorf("File %q reported as existing during locking.", fileName)
	}

	// File must now exist.
	if _, err = os.Stat(fileName); err != nil {
		t.Errorf("Could not stat file %q expected to exist: %s", fileName, err)
	}

	// Try to lock again.
	lockedAgain, existed, err := Flock(fileName)
	if err == nil {
		t.Fatalf("File %q locked twice.", fileName)
	}
	if lockedAgain != nil {
		t.Error("Unsuccessful locking did not return nil.")
	}
	if !existed {
		t.Errorf("Existing file %q not recognized.", fileName)
	}

	if err := lock.Release(); err != nil {
		t.Errorf("Error releasing lock for file %q: %s", fileName, err)
	}

	// File must still exist.
	if _, err = os.Stat(fileName); err != nil {
		t.Errorf("Could not stat file %q expected to exist: %s", fileName, err)
	}

	// Lock existing file.
	lock, existed, err = Flock(fileName)
	if err != nil {
		t.Fatalf("Error locking file %q: %s", fileName, err)
	}
	if !existed {
		t.Errorf("Existing file %q not recognized.", fileName)
	}

	if err := lock.Release(); err != nil {
		t.Errorf("Error releasing lock for file %q: %s", fileName, err)
	}
}
