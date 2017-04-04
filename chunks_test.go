package tsdb

import "github.com/prometheus/tsdb/chunks"

type mockChunkReader struct {
	chunk func(ref uint64) (chunks.Chunk, error)
	close func() error
}

func (cr *mockChunkReader) Chunk(ref uint64) (chunks.Chunk, error) {
	return cr.chunk(ref)
}

func (cr *mockChunkReader) Close() error {
	return cr.close()
}
