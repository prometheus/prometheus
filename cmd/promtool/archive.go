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

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
)

const FILE_PERM = 0644

type archiverConfig struct {
	archiveName string
}

type archiver interface {
	write(fileName string, buf *bytes.Buffer) error
	close() error
	archive() *os.File
}

func newArchiver(cfg archiverConfig) (archiver, error) {
	f, err := os.Create(cfg.archiveName)
	if err != nil {
		return nil, fmt.Errorf("error of creating archive %s: %s", cfg.archiveName, err)
	}
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)
	return &tarGzFileWriter{
		tarWriter: tw,
		gzWriter:  gzw,
		file:      f,
	}, nil
}

type tarGzFileWriter struct {
	tarWriter *tar.Writer
	gzWriter  *gzip.Writer
	file      *os.File
}

func (w *tarGzFileWriter) close() error {
	if err := w.tarWriter.Close(); err != nil {
		return err
	}
	if err := w.gzWriter.Close(); err != nil {
		return err
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	return nil
}

func (w *tarGzFileWriter) write(fileName string, buf *bytes.Buffer) error {
	header := &tar.Header{
		Name: fileName,
		Mode: FILE_PERM,
		Size: int64(buf.Len()),
	}
	if err := w.tarWriter.WriteHeader(header); err != nil {
		return err
	}
	if _, err := w.tarWriter.Write(buf.Bytes()); err != nil {
		return err
	}
	return nil
}

func (w *tarGzFileWriter) archive() *os.File {
	return w.file
}
