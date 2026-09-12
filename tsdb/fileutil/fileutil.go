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

// Package fileutil provides utility methods used when dealing with the filesystem in tsdb.
// It is largely copied from github.com/coreos/etcd/pkg/fileutil to avoid the
// dependency chain it brings with it.
// Please check github.com/coreos/etcd for licensing information.
package fileutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CopyDirs copies all directories, subdirectories and files recursively including the empty folders.
// Source and destination must be full paths.
func CopyDirs(src, dest string) error {
	if err := os.MkdirAll(dest, 0o777); err != nil {
		return err
	}
	files, err := readDirs(src)
	if err != nil {
		return err
	}

	for _, f := range files {
		dp := filepath.Join(dest, f)
		sp := filepath.Join(src, f)

		stat, err := os.Stat(sp)
		if err != nil {
			return err
		}

		// Empty directories are also created.
		if stat.IsDir() {
			if err := os.MkdirAll(dp, 0o777); err != nil {
				return err
			}
			continue
		}

		if err := copyFile(sp, dp); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	err = os.WriteFile(dest, data, 0o666)
	if err != nil {
		return err
	}
	return nil
}

// readDirs reads the source directory recursively and
// returns relative paths to all files and empty directories.
func readDirs(src string) ([]string, error) {
	var files []string

	err := filepath.Walk(src, func(path string, _ os.FileInfo, _ error) error {
		relativePath := strings.TrimPrefix(path, src)
		if relativePath != "" {
			files = append(files, relativePath)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// Rename safely renames a file.
func Rename(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}

	// Directory was renamed; sync parent dir to persist rename.
	pdir, err := OpenDir(filepath.Dir(to))
	if err != nil {
		return err
	}

	if err = pdir.Sync(); err != nil {
		pdir.Close()
		return err
	}
	return pdir.Close()
}

// Replace moves a file or directory to a new location and deletes any previous data.
// It is not atomic.
func Replace(from, to string) error {
	// Remove destination only if it is a dir otherwise leave it to os.Rename
	// as it replaces the destination file and is atomic.
	{
		f, err := os.Stat(to)
		if !os.IsNotExist(err) {
			if err == nil && f.IsDir() {
				if err := os.RemoveAll(to); err != nil {
					return err
				}
			}
		}
	}

	if err := Rename(from, to); err != nil {
		// On Windows, renaming a directory can fail when the data directory
		// lives on a bind-mounted volume (for example a Docker volume), in
		// which case MoveFileEx returns "The system cannot find the path
		// specified.". Fall back to copying the directory and removing the
		// source. See https://github.com/prometheus/prometheus/issues/18308.
		if runtime.GOOS != "windows" {
			return err
		}
		if fi, statErr := os.Stat(from); statErr != nil || !fi.IsDir() {
			return err
		}
		return replaceDirByCopy(from, to)
	}
	return nil
}

// replaceDirByCopy moves the directory at from to to by copying it and then
// removing the source. It is used as a fallback when Rename fails.
func replaceDirByCopy(from, to string) error {
	if err := os.RemoveAll(to); err != nil {
		return err
	}
	if err := CopyDirs(from, to); err != nil {
		return err
	}
	if err := syncTree(to); err != nil {
		return err
	}
	return os.RemoveAll(from)
}

// syncTree fsyncs every file and directory under dir as well as dir's parent,
// so the copied data and the new directory entry are persisted to disk.
func syncTree(dir string) error {
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			d, err := OpenDir(path)
			if err != nil {
				return err
			}
			if err := d.Sync(); err != nil {
				d.Close()
				return err
			}
			return d.Close()
		}
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return err
		}
		if err := Fdatasync(f); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}); err != nil {
		return err
	}

	pdir, err := OpenDir(filepath.Dir(dir))
	if err != nil {
		return err
	}
	if err := pdir.Sync(); err != nil {
		pdir.Close()
		return err
	}
	return pdir.Close()
}
