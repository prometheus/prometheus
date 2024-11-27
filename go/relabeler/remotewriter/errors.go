package remotewriter

import (
	"errors"
	"io"
)

var (
	ErrCorrupted    = errors.New("corrupted")
	ErrNoData       = errors.New("no data")
	ErrDecodeFailed = errors.New("decode failed")
)

// CloseAll closes all given closers.
func CloseAll(closers ...io.Closer) error {
	var err error
	for _, closer := range closers {
		if closer != nil {
			err = errors.Join(err, closer.Close())
		}
	}
	return err
}
