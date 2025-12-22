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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestOpenMmapFile(t *testing.T) {
	dir := testutil.NewTemporaryDirectory("test_mmap", t)
	defer dir.Close()

	content := []byte("lorem impsum, this content is mmapped")

	file := filepath.Join(dir.Path(), "mmap_target")
	err := os.WriteFile(file, content, 0666)
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
		err := os.WriteFile(file, content, 0666)
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
	err := os.WriteFile(file, content, 0666)
	require.NoError(t, err, "Failed to write test target file %q.", file)

	mmap, err := OpenMmapFile(file)
	require.NoError(t, err, "Failed to mmap target file %q.", file)

	err = mmap.Close()
	require.NoError(t, err, "Failed to close mmap.")

	err = mmap.Close()
	require.Error(t, err, "Closing mmap multiple times should error.")
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
	err = os.WriteFile(file, content, 0666)
	require.NoError(t, err, "Failed to write test target file %q.", file)

	mmap, err := OpenMmapFile(file)
	require.NoError(t, err, "Failed to mmap target file %q.", file)

	// ensure we can find the file in /proc/self/maps
	mmapped, err := isPathMmapped(file)
	require.NoError(t, err, "Failed to determine if file is mapped %q.", file)
	require.True(t, mmapped, "mmap memory map was unexpectedly missing")

	// leak the mmap. Note the statement here keeps the object alive. After this
	// the mmap is eligible for GC
	_ = mmap

	// run GC to run cleanup. This is undeterministic so let's run it a few times
	for retry := 0; retry < 3; retry++ {
		runtime.GC()
		mmapped, err = isPathMmapped(file)
		require.NoError(t, err, "Failed to determine if file is mapped %q.", file)

		if !mmapped {
			// GC cleaned the mmap
			break
		}

		// GC didn't clean the handle, retry
		time.Sleep(time.Second)
	}

	// ensure the mmap was cleaned up, and we cannot find the mapping in /proc/self/maps
	mmapped, err = isPathMmapped(file)
	require.False(t, mmapped, "mmap memory map was unexpectedly leaked")
}

// Determines if this process has the given file in a file-backed memory map
func isPathMmapped(file string) (bool, error) {
	maps, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		return false, err
	}
	// don't bother parsing maps. The test file path is unique enough
	return strings.Contains(string(maps), file), nil
}
