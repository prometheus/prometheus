// Copyright The Prometheus Authors
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

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestLocking(t *testing.T) {
	dir := testutil.NewTemporaryDirectory("test_flock", t)
	defer dir.Close()

	fileName := filepath.Join(dir.Path(), "LOCK")

	_, err := os.Stat(fileName)
	require.Error(t, err, "File %q unexpectedly exists.", fileName)

	lock, existed, err := Flock(fileName)
	require.NoError(t, err, "Error locking file %q", fileName)
	require.False(t, existed, "File %q reported as existing during locking.", fileName)

	// File must now exist.
	_, err = os.Stat(fileName)
	require.NoError(t, err, "Could not stat file %q expected to exist", fileName)

	// Try to lock again.
	lockedAgain, existed, err := Flock(fileName)
	require.Error(t, err, "File %q locked twice.", fileName)
	require.Nil(t, lockedAgain, "Unsuccessful locking did not return nil.")
	require.True(t, existed, "Existing file %q not recognized.", fileName)

	err = lock.Release()
	require.NoError(t, err, "Error releasing lock for file %q", fileName)

	// File must still exist.
	_, err = os.Stat(fileName)
	require.NoError(t, err, "Could not stat file %q expected to exist", fileName)

	// Lock existing file.
	lock, existed, err = Flock(fileName)
	require.NoError(t, err, "Error locking file %q", fileName)
	require.True(t, existed, "Existing file %q not recognized.", fileName)

	err = lock.Release()
	require.NoError(t, err, "Error releasing lock for file %q", fileName)
}
