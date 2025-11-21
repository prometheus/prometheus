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
	"log/slog"
	"os"
	"path/filepath"
)

// RemoveTmpDirs attempts to remove directories in the specified directory which match the isTmpDir predicate.
// Errors encountered during reading the directory that other than non-existence are returned. All other errors
// encountered during removal of tmp directories are logged but do not cause early termination.
func RemoveTmpDirs(l *slog.Logger, dir string, isTmpDir func(fi fs.DirEntry) bool) error {
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, f := range files {
		if isTmpDir(f) {
			if err := os.RemoveAll(filepath.Join(dir, f.Name())); err != nil {
				l.Error("failed to delete tmp dir", "dir", filepath.Join(dir, f.Name()), "err", err)
				continue
			}
			l.Info("Found and deleted tmp dir", "dir", filepath.Join(dir, f.Name()))
		}
	}
	return nil
}
