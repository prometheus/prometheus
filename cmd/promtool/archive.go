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
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
)

const FILE_PERM = 0644

type ArchiveWriterConfig struct {
	ArchiveName string
}

type ArchiveWriter interface {
	Write(fileName string, buf *bytes.Buffer) error
	Close() error
}

func NewArchiveWriter(cfg ArchiveWriterConfig) (ArchiveWriter, error) {
	f, err := os.Create(cfg.ArchiveName)
	if err != nil {
		return nil, fmt.Errorf("error of creating archive %s: %s", cfg.ArchiveName, err)
	}
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)
	return &TarGzFileWriter{
		TarWriter: tw,
		GzWriter:  gzw,
		File:      f,
	}, nil
}

type TarGzFileWriter struct {
	TarWriter *tar.Writer
	GzWriter  *gzip.Writer
	File      *os.File
}

func (w *TarGzFileWriter) Close() error {
	if err := w.TarWriter.Close(); err != nil {
		return err
	}
	if err := w.GzWriter.Close(); err != nil {
		return err
	}
	if err := w.File.Close(); err != nil {
		return err
	}
	return nil
}

func (w *TarGzFileWriter) Write(fileName string, buf *bytes.Buffer) error {
	header := &tar.Header{
		Name: fileName,
		Mode: FILE_PERM,
		Size: int64(buf.Len()),
	}
	if err := w.TarWriter.WriteHeader(header); err != nil {
		return err
	}
	if _, err := w.TarWriter.Write(buf.Bytes()); err != nil {
		return err
	}
	return nil
}
