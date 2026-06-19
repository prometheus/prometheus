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

//go:build windows

package chunks

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// TestRepairLastChunkFile_RetriesSharingViolation deterministically reproduces
// the Windows flake in https://github.com/prometheus/prometheus/issues/16176.
// A background goroutine holds the file open without FILE_SHARE_DELETE,
// mimicking an antivirus or indexing service. repairLastChunkFile must retry
// until the handle is released and then succeed.
func TestRepairLastChunkFile_RetriesSharingViolation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "000001")
	require.NoError(t, os.WriteFile(path, nil, 0o666))

	pathp, err := syscall.UTF16PtrFromString(path)
	require.NoError(t, err)
	handle, err := syscall.CreateFile(
		pathp,
		syscall.GENERIC_READ,
		// FILE_SHARE_DELETE is intentionally omitted: this is what causes
		// os.Remove to return ERROR_SHARING_VIOLATION while the handle is open.
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	require.NoError(t, err)

	var closeOnce sync.Once
	closeHandle := func() { closeOnce.Do(func() { _ = syscall.CloseHandle(handle) }) }
	t.Cleanup(closeHandle)

	// Sanity check: while the handle is held a plain os.Remove must fail with
	// ERROR_SHARING_VIOLATION. If this ever changes the test would pass trivially.
	require.ErrorIs(t, os.Remove(path), windows.ERROR_SHARING_VIOLATION)

	// Release the blocking handle after 100ms, well within the 2s retry deadline.
	released := make(chan struct{})
	go func() {
		defer close(released)
		time.Sleep(100 * time.Millisecond)
		closeHandle()
	}()

	files, err := repairLastChunkFile(map[int]string{1: path}, false)
	require.NoError(t, err)
	require.Empty(t, files, "corrupted last chunk file should be removed from the map")

	<-released
	_, statErr := os.Stat(path)
	require.ErrorIs(t, statErr, os.ErrNotExist, "corrupted last chunk file should be deleted from disk")
}
