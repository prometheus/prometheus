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

	"github.com/prometheus/prometheus/util/testutil"
)

func TestLocking(t *testing.T) {
	dir := testutil.NewTemporaryDirectory("test_flock", t)
	defer dir.Close()

	fileName := filepath.Join(dir.Path(), "LOCK")
	_, err := os.Stat(fileName)
	testutil.Assert(t, err != nil, "File %q unexpectedly exists.", fileName)

	lock, existed, err := Flock(fileName)
	testutil.Ok(t, err)
	testutil.Assert(t, !existed, "File %q reported as existing during locking.", fileName)

	// File must now exist.
	_, err = os.Stat(fileName)
	testutil.Ok(t, err)

	// Try to lock again.
	lockedAgain, existed, err := Flock(fileName)
	testutil.Assert(t, err != nil, "File %q locked twice.", fileName)
	testutil.Assert(t, lockedAgain == nil, "Unsuccessful locking did not return nil.")
	testutil.Assert(t, existed, "Existing file %q not recognized.", fileName)
	testutil.Ok(t, lock.Release())

	// File must still exist.
	_, err = os.Stat(fileName)
	testutil.Ok(t, err)

	// Lock existing file.
	lock, existed, err = Flock(fileName)
	testutil.Ok(t, err)
	testutil.Assert(t, existed, "Existing file %q not recognized.", fileName)
	testutil.Ok(t, lock.Release())
}
