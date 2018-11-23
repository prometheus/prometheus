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

// Package modtimevfs implements a virtual file system that returns a fixed
// modification time for all files and directories.
package modtimevfs

import (
	"net/http"
	"os"
	"time"
)

type timefs struct {
	fs http.FileSystem
	t  time.Time
}

// New returns a file system that returns constant modification time for all files.
func New(fs http.FileSystem, t time.Time) http.FileSystem {
	return &timefs{fs: fs, t: t}
}

type file struct {
	http.File
	os.FileInfo
	t time.Time
}

func (t *timefs) Open(name string) (http.File, error) {
	f, err := t.fs.Open(name)
	if err != nil {
		return f, err
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	fstat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	return &file{f, fstat, t.t}, nil
}

// Stat implements the http.File interface.
func (f *file) Stat() (os.FileInfo, error) {
	return f, nil
}

// ModTime implements the os.FileInfo interface.
func (f *file) ModTime() time.Time {
	return f.t
}
