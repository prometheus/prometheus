package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"
)

type HeapProfileWriter struct {
	dir string
}

func NewHeapProfileWriter(dir string) *HeapProfileWriter {
	return &HeapProfileWriter{dir: dir}
}

func (w *HeapProfileWriter) WriteHeapProfile() error {
	dirStat, err := os.Stat(w.dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to stat dir: %w", err)
		}
		if err = os.MkdirAll(w.dir, 0777); err != nil {
			return fmt.Errorf("failed to create dir: %w", err)
		}
		dirStat, err = os.Stat(w.dir)
		if err != nil {
			return fmt.Errorf("failed to stat dir: %w", err)
		}
	}

	if !dirStat.IsDir() {
		return fmt.Errorf("%s is not dir", w.dir)
	}

	filePath := filepath.Join(w.dir, fmt.Sprintf("heap_profile_%d", time.Now().UnixMilli()))
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create heap profile file: %w", err)
	}
	defer func() { _ = file.Close() }()

	return pprof.WriteHeapProfile(file)
}

type NoOpHeapProfileWriter struct{}

func (NoOpHeapProfileWriter) WriteHeapProfile() error { return nil }
