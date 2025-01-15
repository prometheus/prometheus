package remotewriter

import (
	"errors"
	"io"
)

var (
	ErrShardIsCorrupted = errors.New("shard is corrupted")
	ErrEndOfBlock       = errors.New("end of block")
	ErrEmptyReadResult  = errors.New("empty read result")
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
