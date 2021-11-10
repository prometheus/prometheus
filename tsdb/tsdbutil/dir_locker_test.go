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
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestLockfile(t *testing.T) {
	TestDirLockerUsage(t, func(t *testing.T, data string, createLock bool) (*DirLocker, testutil.Closer) {
		locker, err := NewDirLocker(data, "tsdbutil", log.NewNopLogger(), nil)
		require.NoError(t, err)

		if createLock {
			require.NoError(t, locker.Lock())
		}

		return locker, testutil.NewCallbackCloser(func() {
			require.NoError(t, locker.Release())
		})
	})
}
