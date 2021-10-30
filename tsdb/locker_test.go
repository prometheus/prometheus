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

package tsdb

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/go-kit/log"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestLockfile(t *testing.T) {
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
			tmpdir, err := ioutil.TempDir("", "test")
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, os.RemoveAll(tmpdir))
			})

			locker, err := NewLocker(tmpdir, "tsdb", log.NewNopLogger(), nil)
			require.NoError(t, err)

			// Test preconditions (file already exists + lockfile option)
			if c.fileAlreadyExists {
				err = ioutil.WriteFile(locker.path, []byte{}, 0644)
				require.NoError(t, err)
			}

			if !c.lockFileDisabled {
				// Obtain the lock. This should create a lockfile and update the metric.
				require.NoError(t, locker.Lock())
			}
			require.Equal(t, float64(c.expectedValue), prom_testutil.ToFloat64(locker.createdCleanly))

			// Release the lock, this should delete the lockfile
			require.NoError(t, locker.Release())

			// Check that the lockfile is always deleted
			if !c.lockFileDisabled {
				_, err = os.Stat(locker.path)
				require.Error(t, err, "lockfile was not deleted")
			}
		})
	}
}
