package block

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/prometheus/prometheus/pp/go/util"

	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

const (
	// DefaultChunkSegmentSize is the default chunks segment size.
	DefaultChunkSegmentSize = 512 * 1024 * 1024
	// DefaultBlockDuration is the default block duration.
	DefaultBlockDuration         = 2 * time.Hour
	tmpForCreationBlockDirSuffix = ".tmp-for-creation"
	indexFilename                = "index"
	metaFilename                 = "meta.json"
	metaVersion1                 = 1
)

type BlockWriter struct {
	dataDir                  string
	maxBlockChunkSegmentSize int64
	blockDurationMs          int64
	blockWriteDuration       *prometheus.GaugeVec
}

func NewBlockWriter(
	dataDir string,
	maxBlockChunkSegmentSize int64,
	blockDuration time.Duration,
	registerer prometheus.Registerer,
) *BlockWriter {
	factory := util.NewUnconflictRegisterer(registerer)
	return &BlockWriter{
		dataDir:                  dataDir,
		maxBlockChunkSegmentSize: maxBlockChunkSegmentSize,
		blockDurationMs:          blockDuration.Milliseconds(),
		blockWriteDuration: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prompp_block_write_duration",
			Help: "Block write duration in milliseconds.",
		}, []string{"block_id"}),
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

type IndexWriter interface {
	WriteSeriesTo(uint32, []ChunkMetadata, io.Writer) (int64, error)
	WriteRestTo(io.Writer) (int64, error)
}

type Block interface {
	TimeBounds() (minT, maxT int64)
	ChunkIterator(minT, maxT int64) ChunkIterator
	IndexWriter() IndexWriter
}

func (w *BlockWriter) Write(block Block) error {
	minT, maxT := block.TimeBounds()

	ts := (minT / w.blockDurationMs) * w.blockDurationMs
	for ; ts < maxT; ts += w.blockDurationMs {
		endTs := ts + w.blockDurationMs
		if endTs > maxT {
			endTs = maxT
		}
		if err := w.write(block, ts, endTs); err != nil {
			return err
		}
	}

	return nil
}

func (w *BlockWriter) write(block Block, minT, maxT int64) (err error) {
	start := time.Now()
	uid := ulid.MustNew(ulid.Now(), rand.Reader)
	dir := filepath.Join(w.dataDir, uid.String())
	tmp := dir + tmpForCreationBlockDirSuffix
	var closers []io.Closer
	defer func() {
		err = errors.Join(err, closeAll(closers...))
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

	indexFileWriter, err := NewFileWriter(filepath.Join(tmp, indexFilename))
	if err != nil {
		return fmt.Errorf("failed to create index file writer: %w", err)
	}

	closers = append(closers, indexFileWriter)
	indexWriter := block.IndexWriter()

	chunkIterator := block.ChunkIterator(minT, maxT)
	var chunksMetadata []ChunkMetadata
	var chunkMetadata ChunkMetadata
	var previousSeriesID uint32 = math.MaxUint32
	var chunk Chunk

	writeSeries := func() error {
		if len(chunksMetadata) == 0 {
			return nil
		}
		_, err = indexWriter.WriteSeriesTo(previousSeriesID, chunksMetadata, indexFileWriter)
		if err != nil {
			return fmt.Errorf("failed to write series %d: %w", previousSeriesID, err)
		}
		return nil
	}

	blockMeta := &tsdb.BlockMeta{ULID: uid, MinTime: math.MaxInt64, MaxTime: math.MinInt64}

	var hasChunks bool
	for chunkIterator.Next() {
		hasChunks = true
		chunk = chunkIterator.At()
		chunkMetadata, err = chunkw.Write(chunk)
		if err != nil {
			return fmt.Errorf("failed to write chunk: %w", err)
		}

		adjustBlockMetaTimeRange(blockMeta, chunk.MinT(), chunk.MaxT())
		blockMeta.Stats.NumChunks++
		blockMeta.Stats.NumSamples += uint64(chunk.SampleCount())
		seriesID := chunk.SeriesID()

		if previousSeriesID == seriesID {
			chunksMetadata = append(chunksMetadata, chunkMetadata)
		} else {
			if err = writeSeries(); err != nil {
				return err
			}
			blockMeta.Stats.NumSeries++
			chunksMetadata = append(chunksMetadata[:0], chunkMetadata)
			previousSeriesID = seriesID
		}
	}

	if !hasChunks {
		return nil
	}

	if err = writeSeries(); err != nil {
		return err
	}
	indexFileSize, err := indexWriter.WriteRestTo(indexFileWriter)
	if err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}
	// todo: logs & metrics
	_ = indexFileSize

	// write meta
	blockMeta.Version = metaVersion1
	blockMeta.MaxTime += 1
	metaFileSize, err := writeBlockMetaFile(filepath.Join(tmp, metaFilename), blockMeta)
	if err != nil {
		return fmt.Errorf("failed to write block meta file: %w", err)
	}
	// todo: log & metrics
	_ = metaFileSize

	closeErr := err
	for _, closer := range closers {
		closeErr = errors.Join(closeErr, closer.Close())
	}
	closers = closers[:0]

	if closeErr != nil {
		return closeErr
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

	w.blockWriteDuration.With(prometheus.Labels{
		"block_id": blockMeta.ULID.String(),
	}).Set(float64(time.Since(start).Milliseconds()))

	return
}

func chunkDir(dir string) string { return filepath.Join(dir, "chunks") }

func closeAll(closers ...io.Closer) error {
	errs := make([]error, len(closers))
	for i, closer := range closers {
		errs[i] = closer.Close()
	}
	return errors.Join(errs...)
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

func appendChunkMetadata(list [][]ChunkMetadata, chunk Chunk, metadata ChunkMetadata) [][]ChunkMetadata {
	lastSeriesID := len(list) - 1
	chunkSeriesID := chunk.SeriesID()
	for i := chunkSeriesID - 1; i > uint32(lastSeriesID); i-- {
		list = append(list, make([]ChunkMetadata, 0))
	}
	list = append(list, []ChunkMetadata{metadata})
	return list
}
