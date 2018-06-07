package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
)

type TarGzFileWriterConfig struct {
	FileName string
}

type TarGzFileWriter struct {
	TarWriter *tar.Writer
	GzWriter  *gzip.Writer
	File      *os.File
}

func NewTarGzFileFileWriter(cfg TarGzFileWriterConfig) *TarGzFileWriter {
	f, err := os.Create(cfg.FileName)
	if err != nil {
		panic(err)
	}
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)
	return &TarGzFileWriter{
		TarWriter: tw,
		GzWriter:  gzw,
		File:      f,
	}
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

func (w *TarGzFileWriter) AddFile(fileName string, buf bytes.Buffer) error {
	header := &tar.Header{
		Name: fileName,
		Mode: 0644,
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
