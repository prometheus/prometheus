// Copyright 2015 The Prometheus Authors
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
	"compress/gzip"
	"fmt"
	"os"
)

const filePerm = 0o666

type tarGzFileWriter struct {
	tarWriter *tar.Writer
	gzWriter  *gzip.Writer
	file      *os.File
}

func newTarGzFileWriter(archiveName string) (*tarGzFileWriter, error) {
	file, err := os.Create(archiveName)
	if err != nil {
		return nil, fmt.Errorf("error creating archive %q: %w", archiveName, err)
	}
	gzw := gzip.NewWriter(file)
	tw := tar.NewWriter(gzw)
	return &tarGzFileWriter{
		tarWriter: tw,
		gzWriter:  gzw,
		file:      file,
	}, nil
}

func (w *tarGzFileWriter) close() error {
	if err := w.tarWriter.Close(); err != nil {
		return err
	}
	if err := w.gzWriter.Close(); err != nil {
		return err
	}
	return w.file.Close()
}

func (w *tarGzFileWriter) write(filename string, b []byte) error {
	header := &tar.Header{
		Name: filename,
		Mode: filePerm,
		Size: int64(len(b)),
	}
	if err := w.tarWriter.WriteHeader(header); err != nil {
		return err
	}
	if _, err := w.tarWriter.Write(b); err != nil {
		return err
	}
	return nil
}
