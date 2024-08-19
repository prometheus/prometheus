package relabeler

import (
	"crypto/rand"
	"fmt"
	"github.com/oklog/ulid"
	"io"
	"os"
	"path/filepath"
)

const (
	// DefaultChunkSegmentSize is the default chunks segment size.
	DefaultChunkSegmentSize      = 512 * 1024 * 1024
	tmpForCreationBlockDirSuffix = ".tmp-for-creation"
)

type PrometheusBlockWriter struct {
	dataDir                  string
	maxBlockChunkSegmentSize int64
}

func NewBlockWriter(dataDir string, maxBlockChunkSegmentSize int64) *PrometheusBlockWriter {
	return &PrometheusBlockWriter{
		dataDir:                  dataDir,
		maxBlockChunkSegmentSize: maxBlockChunkSegmentSize,
	}
}

type PrometheusChunk interface {
	MinT() int64
	MaxT() int64
	SeriesID() uint32
	Data() []byte
}

type PrometheusChunkIterator interface {
	Next() bool
	At() PrometheusChunk
}

type PrometheusBlock interface {
	ChunkIterator() PrometheusChunkIterator
}

func (w *PrometheusBlockWriter) Write(block PrometheusBlock) (err error) {
	uid := ulid.MustNew(ulid.Now(), rand.Reader)
	dir := filepath.Join(w.dataDir, uid.String())
	tmp := dir + tmpForCreationBlockDirSuffix
	var closers []io.Closer

	if err = os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("failed to cleanup tmp directory {%s}: %w", tmp, err)
	}

	if err = os.MkdirAll(tmp, 0o777); err != nil {
		return fmt.Errorf("failed to create tmp directory {%s}: %w", tmp, err)
	}

	chunkw, err := NewPrometheusChunkWriter(chunkDir(tmp), w.maxBlockChunkSegmentSize)
	if err != nil {
		return fmt.Errorf("failed to create prometheus chunk writer: %w", err)
	}
	closers = append(closers, chunkw)

	chunkIterator := block.ChunkIterator()
	var chunkMetadataListBySeries [][]PrometheusChunkMetadata
	var chunkMetadata PrometheusChunkMetadata
	var previousSeriesID *uint32
	var chunk PrometheusChunk

	for chunkIterator.Next() {
		chunk = chunkIterator.At()
		chunkMetadata, err = chunkw.Write(chunk)
		if err != nil {
			return fmt.Errorf("failed to write chunk: %w", err)
		}

		if previousSeriesID == nil {
			chunkMetadataListBySeries = append(chunkMetadataListBySeries, []PrometheusChunkMetadata{chunkMetadata})
		} else {
			if *previousSeriesID == chunk.SeriesID() {
				chunkMetadataList := chunkMetadataListBySeries[len(chunkMetadataListBySeries)-1]
				chunkMetadataList = append(chunkMetadataList, chunkMetadata)
				chunkMetadataListBySeries[len(chunkMetadataListBySeries)-1] = chunkMetadataList
			} else {
				chunkMetadataListBySeries = append(chunkMetadataListBySeries, []PrometheusChunkMetadata{chunkMetadata})
			}
		}

		seriesID := chunk.SeriesID()
		previousSeriesID = &seriesID
	}

	return nil
}

func chunkDir(dir string) string { return filepath.Join(dir, "chunks") }
