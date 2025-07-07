package tsdbutil

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// RemoveTmpDirs attempts to remove directories in the specified directory which match the isTmpDir predicate.
func RemoveTmpDirs(l *slog.Logger, dir string, isTmpDir func(fi fs.DirEntry) bool) error {
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, f := range files {
		if isTmpDir(f) {
			if err := os.RemoveAll(filepath.Join(dir, f.Name())); err != nil {
				l.Error("failed to delete tmp dir", "dir", filepath.Join(dir, f.Name()), "err", err)
				continue
			}
			l.Info("Found and deleted tmp dir", "dir", filepath.Join(dir, f.Name()))
		}
	}
	return nil
}
