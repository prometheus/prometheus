// Copyright 2021 The Prometheus Authors
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

package tsdbutil

import (
	"fmt"
	"os"
	"testing"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/testutil"
)

// TestDirLockerUsage performs a set of tests which guarantee correct usage of
// DirLocker. open should use data as the storage directory, and createLock
// to determine if a lock file should be used.
func TestDirLockerUsage(t *testing.T, open func(t *testing.T, data string, createLock bool) (*DirLocker, testutil.Closer)) {
	t.Helper()

	cases := []struct {
		fileAlreadyExists bool
		lockFileDisabled  bool
		expectedValue     int
	}{
		{
			fileAlreadyExists: false,
			lockFileDisabled:  false,
			expectedValue:     lockfileCreatedCleanly,
		},
		{
			fileAlreadyExists: true,
			lockFileDisabled:  false,
			expectedValue:     lockfileReplaced,
		},
		{
			fileAlreadyExists: true,
			lockFileDisabled:  true,
			expectedValue:     lockfileDisabled,
		},
		{
			fileAlreadyExists: false,
			lockFileDisabled:  true,
			expectedValue:     lockfileDisabled,
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%+v", c), func(t *testing.T) {
			tmpdir := t.TempDir()

			// Test preconditions (file already exists + lockfile option)
			if c.fileAlreadyExists {
				tmpLocker, err := NewDirLocker(tmpdir, "tsdb", promslog.NewNopLogger(), nil)
				require.NoError(t, err)
				err = os.WriteFile(tmpLocker.path, []byte{}, 0o644)
				require.NoError(t, err)
			}

			locker, closer := open(t, tmpdir, !c.lockFileDisabled)
			require.Equal(t, float64(c.expectedValue), prom_testutil.ToFloat64(locker.createdCleanly))

			// Close the client. This should delete the lockfile.
			closer.Close()

			// Check that the lockfile is always deleted
			if !c.lockFileDisabled {
				_, err := os.Stat(locker.path)
				require.True(t, os.IsNotExist(err), "lockfile was not deleted")
			}
		})
	}
}
