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

//go:build builtinassets
// +build builtinassets

package ui

import (
	"compress/gzip"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"time"
)

const (
	zipSuffix = ".gz"
)

//go:embed static templates
var embedFS embed.FS

var Assets = http.FS(FileSystem{embedFS})

type FileSystem struct {
	embed embed.FS
}

func (compressed FileSystem) Open(path string) (fs.File, error) {
	// if we have the file in our embed FS, just return that (could be a
	// dir)
	var f fs.File
	if f, err := compressed.embed.Open(path); err == nil {
		return f, nil
	}
	// try opening path with .gz suffix
	f, err := compressed.embed.Open(path + zipSuffix)
	if err != nil {
		return f, err
	}
	// read the (decompressed) content into a buffer
	gr, err := gzip.NewReader(f)
	defer gr.Close()
	if err != nil {
		return f, err
	}
	c, err := io.ReadAll(gr)
	if err != nil {
		return f, err
	}
	// and wrap everything in ui.File
	return &File{file: f, content: c}, nil
}

type File struct {
	// the underlying file
	file fs.File
	// the decrompressed content, needed to return an accurate size
	content []byte
	// offset for calls to Read()
	offset int
}

func (f File) Stat() (fs.FileInfo, error) {
	stat, err := f.file.Stat()
	if err != nil {
		return stat, err
	}
	return FileInfo{stat, int64(len(f.content))}, nil
}

func (f *File) Read(buf []byte) (n int, err error) {
	if len(buf) > len(f.content)-f.offset {
		buf = buf[0:len(f.content[f.offset:])]
	}
	n = copy(buf, f.content[f.offset:])
	if n == len(f.content)-f.offset {
		return n, io.EOF
	}
	f.offset += n
	return
}

func (f File) Close() error {
	return f.file.Close()
}

type FileInfo struct {
	fi         fs.FileInfo
	actualSize int64
}

func (fi FileInfo) Name() string {
	name := fi.fi.Name()
	return name[:len(name)-len(zipSuffix)]
}

func (fi FileInfo) Size() int64 { return fi.actualSize }

func (fi FileInfo) Mode() fs.FileMode { return fi.fi.Mode() }

func (fi FileInfo) ModTime() time.Time { return fi.fi.ModTime() }

func (fi FileInfo) IsDir() bool { return fi.fi.IsDir() }

func (fi FileInfo) Sys() interface{} { return nil }
