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

//go:build !js && !plan9

package fileutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestOpenMmapFile(t *testing.T) {
	dir := testutil.NewTemporaryDirectory("test_mmap", t)
	defer dir.Close()

	content := []byte("lorem impsum, this content is mmapped")

	file := filepath.Join(dir.Path(), "mmap_target")
	err := os.WriteFile(file, content, 0o666)
	require.NoError(t, err, "Failed to write test target file %q.", file)

	mmap, err := OpenMmapFile(file)
	require.NoError(t, err, "Failed to mmap target file %q.", file)

	defer mmap.Close()
	require.Equal(t, content, mmap.Bytes(), "Mmap does not match the data in the file")
}

func TestOpenMmapFileWithSize(t *testing.T) {
	dir := testutil.NewTemporaryDirectory("test_mmap", t)
	defer dir.Close()

	content := []byte("lorem impsum, this content is mmapped")
	sizes := []int{len(content), 12}

	for idx, size := range sizes {
		file := filepath.Join(dir.Path(), fmt.Sprintf("mmap_target_%d", idx))
		err := os.WriteFile(file, content, 0o666)
		require.NoError(t, err, "Failed to write test target file %q.", file)

		mmap, err := OpenMmapFileWithSize(file, size)
		require.NoError(t, err, "Failed to mmap target file %q.", file)

		defer mmap.Close()
		require.Equal(t, content[:size], mmap.Bytes(), "Mmap does not match the data in the file")
	}
}

func TestClose(t *testing.T) {
	dir := testutil.NewTemporaryDirectory("test_mmap", t)
	defer dir.Close()

	content := []byte("lorem impsum, this content is mmapped")

	file := filepath.Join(dir.Path(), "mmap_target")
	err := os.WriteFile(file, content, 0o666)
	require.NoError(t, err, "Failed to write test target file %q.", file)

	mmap, err := OpenMmapFile(file)
	require.NoError(t, err, "Failed to mmap target file %q.", file)

	err = mmap.Close()
	require.NoError(t, err, "Failed to close mmap.")

	err = mmap.Close()
	require.Error(t, err, "Closing mmap multiple times should error.")
}

func TestMmapRefCloseRetriesAfterFailure(t *testing.T) {
	wantErr := errors.New("unmap failed")
	m := &mmapRef{b: []byte("mapped")}
	calls := 0

	err := m.closeWith(func([]byte) error {
		calls++
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)
	require.NotNil(t, m.b)

	err = m.closeWith(func([]byte) error {
		calls++
		return nil
	})
	require.NoError(t, err)
	require.Nil(t, m.b)
	require.Equal(t, 2, calls)
}

func TestMmapViewRetainsMapping(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("inspecting process memory maps is only implemented on Linux")
	}
	if _, err := os.ReadFile("/proc/self/maps"); err != nil {
		t.Skip("procfs is not mounted")
	}

	dir := testutil.NewTemporaryDirectory("test_mmap_view", t)
	defer dir.Close()
	path := filepath.Join(dir.Path(), "mmap_view_target")
	content := []byte("the view owns this mapping")
	require.NoError(t, os.WriteFile(path, content, 0o666))

	assertMmapViewRetainsMapping(t, path, content)
	requireEventuallyUnmapped(t, path)
}

func assertMmapViewRetainsMapping(t *testing.T, path string, content []byte) {
	t.Helper()
	view := openMmapView(t, path)
	for range 3 {
		runtime.GC()
	}
	mapped, err := isPathMmapped(path)
	require.NoError(t, err)
	require.True(t, mapped)
	require.Equal(t, content, view.Copy())
	runtime.KeepAlive(view)
}

func openMmapView(t *testing.T, path string) encoding.ByteView {
	t.Helper()
	f, err := OpenMmapFile(path)
	require.NoError(t, err)
	return f.BytesView()
}

func TestGCCleanup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("inspecting process memorymaps not implemented on this platform")
	}

	_, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		t.Skip("procfs is not mounted, cannot validate mmappings")
	}

	dir := testutil.NewTemporaryDirectory("test_mmap", t)
	defer dir.Close()

	content := []byte("lorem impsum, this content is mmapped")

	file := filepath.Join(dir.Path(), "mmap_leak_target")
	err = os.WriteFile(file, content, 0o666)
	require.NoError(t, err, "Failed to write test target file %q.", file)

	openAndVerifyMmap(t, file)
	requireEventuallyUnmapped(t, file)
}

func openAndVerifyMmap(t *testing.T, path string) {
	t.Helper()
	mmapFile, err := OpenMmapFile(path)
	require.NoError(t, err, "Failed to mmap target file %q.", path)

	mapped, err := isPathMmapped(path)
	require.NoError(t, err, "Failed to determine if file is mapped %q.", path)
	require.True(t, mapped, "mmap memory map was unexpectedly missing")
	runtime.KeepAlive(mmapFile)
}

func requireEventuallyUnmapped(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		runtime.GC()
		mapped, err := isPathMmapped(path)
		require.NoError(t, err, "Failed to determine if file is mapped %q.", path)
		if !mapped {
			return
		}
		if time.Now().After(deadline) {
			require.FailNow(t, "mmap memory map was unexpectedly leaked", "path: %q", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Determines if this process has the given file in a file-backed memory map.
func isPathMmapped(file string) (bool, error) {
	maps, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		return false, err
	}
	// don't bother parsing maps. The test file path is unique enough
	return strings.Contains(string(maps), file), nil
}
