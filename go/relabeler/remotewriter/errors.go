package remotewriter

import (
	"errors"
	"io"
)

var (
	ErrShardIsCorrupted  = errors.New("shard is corrupted")
	ErrNoData            = errors.New("no data")
	ErrCursorIsCorrupted = errors.New("cursor is corrupted")
	ErrEndOfBlock        = errors.New("end of block")
	ErrPartialReadResult = errors.New("partial read result")
	ErrEmptyReadResult   = errors.New("empty read result")
	ErrBlockIsCorrupted  = errors.New("block is corrupted")
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
