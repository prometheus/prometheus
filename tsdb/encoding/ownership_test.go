// Copyright The Prometheus Authors
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

package encoding

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func TestByteViewDerivedViewRetainsOwner(t *testing.T) {
	cleaned := exerciseDerivedView(t)
	requireOwnerCleaned(t, cleaned)
}

func exerciseDerivedView(t *testing.T) *atomic.Bool {
	t.Helper()
	owner := new([64]byte)
	cleaned := addOwnerCleanup(owner)
	view := NewByteViewWithOwner([]byte("mapped"), owner).Slice(1, 4)

	runtime.GC()
	runtime.KeepAlive(view)
	require.False(t, cleaned.Load())
	require.Equal(t, "app", view.String())
	return cleaned
}

func TestByteViewWithBytesRetainsOwnerDuringCallback(t *testing.T) {
	cleaned := exerciseWithBytes(t)
	requireOwnerCleaned(t, cleaned)
}

func exerciseWithBytes(t *testing.T) *atomic.Bool {
	t.Helper()
	owner := new([64]byte)
	cleaned := addOwnerCleanup(owner)
	view := NewByteViewWithOwner([]byte("mapped"), owner)

	require.NoError(t, view.WithBytes(func(b []byte) error {
		runtime.GC()
		require.False(t, cleaned.Load())
		require.Equal(t, []byte("mapped"), b)
		return nil
	}))
	return cleaned
}

func addOwnerCleanup(owner *[64]byte) *atomic.Bool {
	cleaned := &atomic.Bool{}
	runtime.AddCleanup(owner, func(cleaned *atomic.Bool) {
		cleaned.Store(true)
	}, cleaned)
	return cleaned
}

func requireOwnerCleaned(t *testing.T, cleaned *atomic.Bool) {
	t.Helper()
	require.Eventually(t, func() bool {
		runtime.GC()
		return cleaned.Load()
	}, 2*time.Second, 10*time.Millisecond)
}
