// Copyright 2018 The Prometheus Authors

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
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

func TestRemoveTmpDirs(t *testing.T) {
	tests := []struct {
		name         string
		isTmpDir     func(fi fs.DirEntry) bool
		setup        func(t *testing.T, dir string)
		expectedDirs []string // Directories that should remain after cleanup
	}{
		{
			name: "remove directories with tmp prefix",
			isTmpDir: func(fi fs.DirEntry) bool {
				return fi.IsDir() && strings.HasPrefix(fi.Name(), "tmp")
			},
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.Mkdir(filepath.Join(dir, "tmpdir1"), 0o755))
				require.NoError(t, os.Mkdir(filepath.Join(dir, "tmpdir2"), 0o755))
				require.NoError(t, os.Mkdir(filepath.Join(dir, "normaldir"), 0o755))
			},
			expectedDirs: []string{"normaldir"},
		},
		{
			name: "remove directories with specific suffix",
			isTmpDir: func(fi fs.DirEntry) bool {
				return fi.IsDir() && strings.HasSuffix(fi.Name(), ".tmp")
			},
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.Mkdir(filepath.Join(dir, "data.tmp"), 0o755))
				require.NoError(t, os.Mkdir(filepath.Join(dir, "cache.tmp"), 0o755))
				require.NoError(t, os.Mkdir(filepath.Join(dir, "permanent"), 0o755))
			},
			expectedDirs: []string{"permanent"},
		},
		{
			name: "no temporary directories to remove",
			isTmpDir: func(fi fs.DirEntry) bool {
				return fi.IsDir() && strings.HasPrefix(fi.Name(), "tmp")
			},
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.Mkdir(filepath.Join(dir, "normaldir1"), 0o755))
				require.NoError(t, os.Mkdir(filepath.Join(dir, "normaldir2"), 0o755))
			},
			expectedDirs: []string{"normaldir1", "normaldir2"},
		},
		{
			name: "empty directory",
			isTmpDir: func(fi fs.DirEntry) bool {
				return fi.IsDir() && strings.HasPrefix(fi.Name(), "tmp")
			},
			setup:        func(_ *testing.T, _ string) {}, // No setup needed - directory is empty
			expectedDirs: []string{},
		},
		{
			name: "directory with files only (no directories)",
			isTmpDir: func(fi fs.DirEntry) bool {
				return fi.IsDir() && strings.HasPrefix(fi.Name(), "tmp")
			},
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "tmpfile1.txt"), []byte("test"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "tmpfile2.txt"), []byte("test"), 0o644))
			},
			expectedDirs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()

			if tt.setup != nil {
				tt.setup(t, testDir)
			}

			require.NoError(t, RemoveTmpDirs(promslog.NewNopLogger(), testDir, tt.isTmpDir))

			entries, err := os.ReadDir(testDir)
			require.NoError(t, err)

			// Get actual remaining directories
			var actualDirs []string
			for _, entry := range entries {
				if entry.IsDir() {
					actualDirs = append(actualDirs, entry.Name())
				}
			}

			require.ElementsMatch(t, tt.expectedDirs, actualDirs, "Remaining directories don't match expected")
		})
	}
}

func TestRemoveTmpDirs_NonExistentDirectory(t *testing.T) {
	testDir := t.TempDir()
	nonExistent := filepath.Join(testDir, "does_not_exist")

	require.NoError(t, RemoveTmpDirs(promslog.NewNopLogger(), nonExistent, func(_ fs.DirEntry) bool {
		return true
	}))
}
