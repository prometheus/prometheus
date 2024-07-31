package relabeler

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prometheus/prometheus/pp/go/util"
)

const (
	// refillFileExtension - file for refill extension.
	refillFileExtension = ".refill"
	// refillIntermediateFileExtension - file for temporary refill extension.
	refillIntermediateFileExtension = ".tmprefill"
)

// FileStorageConfig - config for FileStorage.
type FileStorageConfig struct {
	Dir      string
	FileName string
}

// FileStorage - wrapper for worker with file.
type FileStorage struct {
	fileDescriptor *os.File
	dir            string
	fileName       string
}

// NewFileStorage - init new FileStorage.
func NewFileStorage(cfg FileStorageConfig) (*FileStorage, error) {
	return &FileStorage{
		dir:      cfg.Dir,
		fileName: cfg.FileName + refillFileExtension,
	}, nil
}

// OpenFile - open or create new file.
func (fs *FileStorage) OpenFile() error {
	//revive:disable-next-line:add-constant file permissions simple readable as octa-number
	if err := os.MkdirAll(fs.dir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(fs.fileName), err)
	}

	f, err := os.OpenFile(
		fs.GetPath(),
		os.O_CREATE|os.O_RDWR,
		//revive:disable-next-line:add-constant file permissions simple readable as octa-number
		0o600,
	)
	if err != nil {
		return fmt.Errorf("open file %s: %w", fs.fileName, err)
	}

	fs.fileDescriptor = f

	return nil
}

// Writer - wrapper for write with offset.
func (fs *FileStorage) Writer(ctx context.Context, off int64) io.Writer {
	return util.FnWriter(func(p []byte) (int, error) {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if deadline, ok := ctx.Deadline(); ok {
			_ = fs.fileDescriptor.SetWriteDeadline(deadline)
		} else {
			_ = fs.fileDescriptor.SetWriteDeadline(time.Time{})
		}
		n, err := fs.fileDescriptor.WriteAt(p, off)
		off += int64(n)
		if err != nil {
			return n, err
		}
		return n, fs.fileDescriptor.Sync()
	})
}

// Seek - implements Seek of reader.
func (fs *FileStorage) Seek(offset int64, whence int) (int64, error) {
	return fs.fileDescriptor.Seek(offset, whence)
}

// Read - implements io.Read.
func (fs *FileStorage) Read(data []byte) (int, error) {
	return fs.fileDescriptor.Read(data)
}

// ReadAt - implements io.ReadAt.
func (fs *FileStorage) ReadAt(b []byte, off int64) (int, error) {
	return fs.fileDescriptor.ReadAt(b, off)
}

// Close - implements os.Close
func (fs *FileStorage) Close() error {
	if fs.fileDescriptor == nil {
		return nil
	}

	err := fs.fileDescriptor.Close()
	if err != nil {
		return err
	}

	fs.fileDescriptor = nil

	return nil
}

// FileExist - check file exist.
func (fs *FileStorage) FileExist() (bool, error) {
	_, err := os.Stat(fs.GetPath())
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}

// GetPath - return path to current file.
func (fs *FileStorage) GetPath() string {
	return filepath.Join(fs.dir, fs.fileName)
}

// Rename file
func (fs *FileStorage) Rename(name string) error {
	return fs.renameFile(name + refillFileExtension)
}

// IntermediateRename renames file with temporary extension
func (fs *FileStorage) IntermediateRename(name string) error {
	return fs.renameFile(name + refillIntermediateFileExtension)
}

// GetIntermediateName returns true with name if file has temporary extension
func (fs *FileStorage) GetIntermediateName() (string, bool) {
	if strings.HasSuffix(fs.fileName, refillIntermediateFileExtension) {
		return strings.TrimSuffix(fs.fileName, refillIntermediateFileExtension), true
	}
	return "", false
}

func (fs *FileStorage) renameFile(name string) error {
	if ok, err := fs.FileExist(); !ok {
		return err
	}

	if err := os.Rename(fs.GetPath(), filepath.Join(fs.dir, name)); err != nil {
		return err
	}

	fs.fileName = name

	return nil
}

// DeleteCurrentFile - delete current file.
func (fs *FileStorage) DeleteCurrentFile() error {
	if err := fs.Close(); err != nil {
		return err
	}

	return os.Remove(fs.GetPath())
}

// Truncate - changes the size of the file.
func (fs *FileStorage) Truncate(size int64) error {
	if err := fs.fileDescriptor.Truncate(size); err != nil {
		return err
	}

	if _, err := fs.fileDescriptor.Seek(size, 0); err != nil {
		return err
	}

	return nil
}

// Size - length in bytes for regular files; system-dependent for others.
func (fs *FileStorage) Size() (int64, error) {
	fstat, err := fs.fileDescriptor.Stat()
	if err != nil {
		return 0, err
	}

	return fstat.Size(), nil
}
