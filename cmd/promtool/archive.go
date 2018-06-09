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
