package block

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-multierror"
	"github.com/oklog/ulid"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"io"
	"math"
	"os"
	"path/filepath"
)

const (
	// DefaultChunkSegmentSize is the default chunks segment size.
	DefaultChunkSegmentSize      = 512 * 1024 * 1024
	tmpForCreationBlockDirSuffix = ".tmp-for-creation"
	indexFilename                = "index"
	metaFilename                 = "meta.json"
	metaVersion1                 = 1
)

type BlockWriter struct {
	dataDir                  string
	maxBlockChunkSegmentSize int64
}

func NewBlockWriter(dataDir string, maxBlockChunkSegmentSize int64) *BlockWriter {
	return &BlockWriter{
		dataDir:                  dataDir,
		maxBlockChunkSegmentSize: maxBlockChunkSegmentSize,
	}
}

type Chunk interface {
	MinT() int64
	MaxT() int64
	SeriesID() uint32
	Encoding() chunkenc.Encoding
	SampleCount() uint8
	Bytes() []byte
}

type ChunkIterator interface {
	Next() bool
	At() Chunk
}

type Block interface {
	ChunkIterator() ChunkIterator
	IndexWriterTo(metadataList [][]ChunkMetadata) io.WriterTo
}

func (w *BlockWriter) Write(block Block) (err error) {
	uid := ulid.MustNew(ulid.Now(), rand.Reader)
	dir := filepath.Join(w.dataDir, uid.String())
	tmp := dir + tmpForCreationBlockDirSuffix
	var closers []io.Closer
	defer func() {
		err = multierror.Append(err, closeAll(closers...)).ErrorOrNil()
		if cleanUpErr := os.RemoveAll(tmp); err != nil {
			// todo: log error
			_ = cleanUpErr
		}
	}()
	if err = os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("failed to cleanup tmp directory {%s}: %w", tmp, err)
	}

	if err = os.MkdirAll(tmp, 0o777); err != nil {
		return fmt.Errorf("failed to create tmp directory {%s}: %w", tmp, err)
	}

	chunkw, err := NewChunkWriter(chunkDir(tmp), w.maxBlockChunkSegmentSize)
	if err != nil {
		return fmt.Errorf("failed to create chunk writer: %w", err)
	}
	closers = append(closers, chunkw)

	chunkIterator := block.ChunkIterator()
	var chunkMetadataListBySeries [][]ChunkMetadata
	var chunkMetadata ChunkMetadata
	var previousSeriesID *uint32
	var chunk Chunk

	blockMeta := &tsdb.BlockMeta{ULID: uid, MinTime: math.MaxInt64, MaxTime: math.MinInt64}

	for chunkIterator.Next() {
		chunk = chunkIterator.At()
		chunkMetadata, err = chunkw.Write(chunk)
		if err != nil {
			return fmt.Errorf("failed to write chunk: %w", err)
		}

		adjustBlockMetaTimeRange(blockMeta, chunk.MinT(), chunk.MaxT())
		blockMeta.Stats.NumChunks++
		blockMeta.Stats.NumSamples += uint64(chunk.SampleCount())

		if previousSeriesID == nil {
			chunkMetadataListBySeries = append(chunkMetadataListBySeries, []ChunkMetadata{chunkMetadata})
			blockMeta.Stats.NumSeries++
		} else {
			if *previousSeriesID == chunk.SeriesID() {
				chunkMetadataList := chunkMetadataListBySeries[len(chunkMetadataListBySeries)-1]
				chunkMetadataList = append(chunkMetadataList, chunkMetadata)
				chunkMetadataListBySeries[len(chunkMetadataListBySeries)-1] = chunkMetadataList
			} else {
				chunkMetadataListBySeries = append(chunkMetadataListBySeries, []ChunkMetadata{chunkMetadata})
				blockMeta.Stats.NumSeries++
			}
		}

		seriesID := chunk.SeriesID()
		previousSeriesID = &seriesID
	}

	// write index
	indexFileWriter, err := NewFileWriter(filepath.Join(tmp, indexFilename))
	if err != nil {
		return fmt.Errorf("failed to create index file writer: %w", err)
	}

	closers = append(closers, indexFileWriter)
	indexWriterTo := block.IndexWriterTo(chunkMetadataListBySeries)
	indexFileSize, err := indexWriterTo.WriteTo(indexFileWriter)
	if err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}
	// todo: logs & metrics
	_ = indexFileSize

	// write meta
	blockMeta.Version = metaVersion1
	metaFileSize, err := writeBlockMetaFile(filepath.Join(tmp, metaFilename), blockMeta)
	if err != nil {
		return fmt.Errorf("failed to write block meta file: %w", err)
	}
	// todo: log & metrics
	_ = metaFileSize

	closeErr := multierror.Append(err)
	for _, closer := range closers {
		closeErr = multierror.Append(closeErr, closer.Close())
	}
	closers = closers[:0]

	if closeErr.ErrorOrNil() != nil {
		return closeErr.ErrorOrNil()
	}

	var df *os.File
	df, err = fileutil.OpenDir(tmp)
	if err != nil {
		return fmt.Errorf("failed to open temporary block dir: %w", err)
	}
	defer func() {
		if df != nil {
			_ = df.Close()
		}
	}()

	if err = df.Sync(); err != nil {
		return fmt.Errorf("failed to sync temporary block dir: %w", err)
	}

	if err = df.Close(); err != nil {
		return fmt.Errorf("failed to close temporary block dir: %w", err)
	}
	df = nil

	if err = fileutil.Replace(tmp, dir); err != nil {
		return fmt.Errorf("failed to move temporary block dir {%s} to {%s}: %w", tmp, dir, err)
	}

	fmt.Println(*blockMeta)

	return
}

func chunkDir(dir string) string { return filepath.Join(dir, "chunks") }

func closeAll(closers ...io.Closer) error {
	var err *multierror.Error
	for _, closer := range closers {
		err = multierror.Append(err, closer.Close())
	}
	return err.ErrorOrNil()
}

func adjustBlockMetaTimeRange(blockMeta *tsdb.BlockMeta, mint, maxt int64) {
	if mint < blockMeta.MinTime {
		blockMeta.MinTime = mint
	}

	if maxt > blockMeta.MaxTime {
		blockMeta.MaxTime = maxt
	}
}

func writeBlockMetaFile(fileName string, blockMeta *tsdb.BlockMeta) (int64, error) {
	tmp := fileName + ".tmp"
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			// todo: log error
		}
	}()

	metaFile, err := os.Create(tmp)
	if err != nil {
		return 0, fmt.Errorf("failed to create block meta file: %w", err)
	}
	defer func() {
		if metaFile != nil {
			if err = metaFile.Close(); err != nil {
				// todo: log error
			}
		}
	}()

	jsonBlockMeta, err := json.MarshalIndent(blockMeta, "", "\t")
	if err != nil {
		return 0, fmt.Errorf("failed to marshal meta json: %w", err)
	}

	n, err := metaFile.Write(jsonBlockMeta)
	if err != nil {
		return 0, fmt.Errorf("failed to write meta json: %w", err)
	}

	if err = metaFile.Sync(); err != nil {
		return 0, fmt.Errorf("failed to sync meta file: %w", err)
	}

	if err = metaFile.Close(); err != nil {
		return 0, fmt.Errorf("faield to close meta file: %w", err)
	}
	metaFile = nil

	return int64(n), fileutil.Replace(tmp, fileName)
}
