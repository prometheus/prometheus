package block

import (
	"bufio"
	"fmt"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"os"
	"path/filepath"
)

type FileWriter struct {
	file        *os.File
	writeBuffer *bufio.Writer
}

func NewFileWriter(fileName string) (*FileWriter, error) {
	dir := filepath.Dir(fileName)
	df, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to open parent dir {%s}: %w", dir, err)
	}
	defer func() { _ = df.Close() }()

	if err := os.RemoveAll(fileName); err != nil {
		return nil, fmt.Errorf("failed to cleanup {%s}: %w", fileName, err)
	}

	indexFile, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return nil, fmt.Errorf(" failed to open file {%s}: %w", fileName, err)
	}

	return &FileWriter{
		file:        indexFile,
		writeBuffer: bufio.NewWriterSize(indexFile, 1<<22),
	}, nil
}

func (w *FileWriter) Write(p []byte) (n int, err error) {
	return w.writeBuffer.Write(p)
}

func (w *FileWriter) Close() error {
	if err := w.writeBuffer.Flush(); err != nil {
		return fmt.Errorf("failed to flush write buffer: %w", err)
	}

	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync index file: %w", err)
	}

	return w.file.Close()
}
