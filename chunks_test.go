package tsdb

import (
	"errors"

	"github.com/prometheus/tsdb/chunks"
)

type mockChunkReader map[uint64]chunks.Chunk

func (cr mockChunkReader) Chunk(ref uint64) (chunks.Chunk, error) {
	chk, ok := cr[ref]
	if ok {
		return chk, nil
	}

	return nil, errors.New("Chunk with ref not found")
}

func (cr mockChunkReader) Close() error {
	return nil
}
