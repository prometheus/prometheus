package remotewriter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/util/optional"
	"github.com/prometheus/client_golang/prometheus"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

type CorruptMarker interface {
	MarkCorrupted(headID string) error
}

type ShardError struct {
	shardID int
	err     error
}

func (e ShardError) Error() string {
	return e.err.Error()
}

func (e ShardError) Unwrap() error {
	return e.err
}

func (e ShardError) ShardID() int {
	return e.shardID
}

func NewShardError(shardID int, err error) ShardError {
	return ShardError{
		shardID: shardID,
		err:     err,
	}
}

type ShardWalReader interface {
	Read() (segment Segment, err error)
	Close() error
}

type NoOpShardWalReader struct{}

func (NoOpShardWalReader) Read() (segment Segment, err error) { return segment, io.EOF }
func (NoOpShardWalReader) Close() error                       { return nil }

type DecoderStateWriteCloser interface {
	Write([]byte) (int, error)
	Close() error
}

type NoOpDecoderStateWriteCloser struct{}

func (NoOpDecoderStateWriteCloser) Write(i []byte) (int, error) { return len(i), nil }
func (NoOpDecoderStateWriteCloser) Close() error                { return nil }

type shard struct {
	headID                  string
	shardID                 uint16
	corrupted               bool
	lastReadSegmentID       optional.Optional[uint32]
	walReader               ShardWalReader
	decoder                 *Decoder
	decoderStateFile        DecoderStateWriteCloser
	unexpectedEOFCounterVec *prometheus.CounterVec
	segmentSizeHistogramVec *prometheus.HistogramVec
}

func newShard(
	headID string,
	shardID uint16,
	shardFileName, decoderStateFileName string,
	resetDecoderState bool,
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
	unexpectedEOFCounterVec *prometheus.CounterVec,
	segmentSizeHistogramVec *prometheus.HistogramVec,
) (*shard, error) {
	var err error
	var wr *walReader
	var encoderVersion uint8
	wr, encoderVersion, err = newWalReader(shardFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create wal file reader: %w", err)
	}

	var decoder *Decoder

	decoder, err = NewDecoder(
		externalLabels,
		relabelConfigs,
		shardID,
		encoderVersion,
	)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to create decoder: %w", err), wr.Close())
	}

	var decoderStateFile *os.File
	decoderStateFileFlags := os.O_CREATE | os.O_RDWR
	if resetDecoderState {
		decoderStateFileFlags = decoderStateFileFlags | os.O_TRUNC
	}
	decoderStateFile, err = os.OpenFile(decoderStateFileName, decoderStateFileFlags, 0600)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to open cache file: %w", err), wr.Close())
	}

	s := &shard{
		headID:                  headID,
		shardID:                 shardID,
		walReader:               wr,
		decoder:                 decoder,
		decoderStateFile:        decoderStateFile,
		unexpectedEOFCounterVec: unexpectedEOFCounterVec,
		segmentSizeHistogramVec: segmentSizeHistogramVec,
	}

	if !resetDecoderState {
		if err = decoder.LoadFrom(decoderStateFile); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to restore from cache: %w", err), s.Close())
		}
	} else {
		if err = decoderStateFile.Truncate(0); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to truncate decoder state file: %w", err), s.Close())
		}
	}

	return s, nil
}

func newCorruptedShard() *shard {
	return &shard{
		corrupted:        true,
		walReader:        NoOpShardWalReader{},
		decoderStateFile: NoOpDecoderStateWriteCloser{},
	}
}

func (s *shard) SetCorrupted() {
	s.corrupted = true
}

func (s *shard) Read(ctx context.Context, targetSegmentID uint32, minTimestamp int64, isActiveHead bool) (*DecodedSegment, error) {
	if s.corrupted {
		return nil, ErrShardIsCorrupted
	}

	if !s.lastReadSegmentID.IsNil() && s.lastReadSegmentID.Value() >= targetSegmentID {
		return nil, nil
	}

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		segment, err := s.walReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, io.EOF
			}
			if errors.Is(err, io.ErrUnexpectedEOF) && isActiveHead {
				var segmentID uint32 = 0
				if !s.lastReadSegmentID.IsNil() {
					segmentID = s.lastReadSegmentID.Value() + 1
				}
				s.unexpectedEOFCounterVec.WithLabelValues(
					s.headID,
					fmt.Sprintf("%d", s.shardID),
					fmt.Sprintf("%d", segmentID),
				).Inc()
				return nil, io.EOF
			}
			s.corrupted = true
			return nil, errors.Join(err, ErrShardIsCorrupted)
		}

		s.segmentSizeHistogramVec.WithLabelValues(
			s.headID,
			fmt.Sprintf("%d", s.shardID),
			fmt.Sprintf("%d", segment.ID),
		).Observe(float64(len(segment.Data())))

		decodedSegment, err := s.decoder.Decode(segment.Data(), minTimestamp)
		if err != nil {
			s.corrupted = true
			return nil, errors.Join(err, ErrShardIsCorrupted)
		}

		s.lastReadSegmentID.Set(segment.ID)

		if segment.ID == targetSegmentID {
			decodedSegment.ID = segment.ID
			return decodedSegment, nil
		}
	}
}

func (s *shard) Close() error {
	return errors.Join(s.walReader.Close(), s.decoderStateFile.Close())
}

type dataSource struct {
	ID              string
	headRecord      *catalog.Record
	shards          []*shard
	corruptMarker   CorruptMarker
	closed          bool
	completed       bool
	corrupted       bool
	headReleaseFunc func()

	mtx         sync.Mutex
	cacheWriter *cacheWriter

	unexpectedEOFCounterVec *prometheus.CounterVec
	segmentSizeHistogramVec *prometheus.HistogramVec
}

func newDataSource(dataDir string,
	numberOfShards uint16,
	config DestinationConfig,
	discardCache bool,
	corruptMarker CorruptMarker,
	headRecord *catalog.Record,
	unexpectedEOFCounterVec *prometheus.CounterVec,
	segmentSizeHistogramVec *prometheus.HistogramVec,
) (*dataSource, error) {
	var err error
	var convertedRelabelConfigs []*cppbridge.RelabelConfig
	convertedRelabelConfigs, err = convertRelabelConfigs(config.WriteRelabelConfigs...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert relabel configs: %w", err)
	}

	headReleaseFunc := headRecord.Acquire()
	b := &dataSource{
		headRecord:              headRecord,
		corruptMarker:           corruptMarker,
		headReleaseFunc:         headReleaseFunc,
		unexpectedEOFCounterVec: unexpectedEOFCounterVec,
		segmentSizeHistogramVec: segmentSizeHistogramVec,
	}

	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		shardFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.wal", shardID))
		decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("%s_shard_%d.state", config.Name, shardID))
		var s *shard
		s, err = createShard(
			headRecord.ID(),
			shardID,
			shardFileName,
			decoderStateFileName,
			discardCache,
			config.ExternalLabels,
			convertedRelabelConfigs,
			b.unexpectedEOFCounterVec,
			b.segmentSizeHistogramVec,
		)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed to create shard: %w", err), b.Close())
		}
		b.shards = append(b.shards, s)
	}

	return b, nil
}

func createShard(
	headID string,
	shardID uint16,
	shardFileName, decoderStateFileName string,
	resetDecoderState bool,
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
	unexpectedEOFCounterVec *prometheus.CounterVec,
	segmentSizeHistogramVec *prometheus.HistogramVec,
) (*shard, error) {
	s, err := newShard(
		headID,
		shardID,
		shardFileName,
		decoderStateFileName,
		resetDecoderState,
		externalLabels,
		relabelConfigs,
		unexpectedEOFCounterVec,
		segmentSizeHistogramVec,
	)
	if err != nil {
		logger.Errorf("failed to create shard: %v", err)
		return newShard(
			headID,
			shardID,
			shardFileName,
			decoderStateFileName,
			true,
			externalLabels,
			relabelConfigs,
			unexpectedEOFCounterVec,
			segmentSizeHistogramVec,
		)
	}
	return s, nil
}

func convertRelabelConfigs(relabelConfigs ...*relabel.Config) ([]*cppbridge.RelabelConfig, error) {
	convertedConfigs := make([]*cppbridge.RelabelConfig, 0, len(relabelConfigs))
	for _, relabelConfig := range relabelConfigs {
		var sourceLabels []string
		for _, label := range relabelConfig.SourceLabels {
			sourceLabels = append(sourceLabels, string(label))
		}

		convertedConfig := &cppbridge.RelabelConfig{
			SourceLabels: sourceLabels,
			Separator:    relabelConfig.Separator,
			Regex:        relabelConfig.Regex.String(),
			Modulus:      relabelConfig.Modulus,
			TargetLabel:  relabelConfig.TargetLabel,
			Replacement:  relabelConfig.Replacement,
			Action:       cppbridge.ActionNameToValueMap[string(relabelConfig.Action)],
		}

		if err := convertedConfig.Validate(); err != nil {
			return nil, fmt.Errorf("failed to validate config: %w", err)
		}

		convertedConfigs = append(convertedConfigs, convertedConfig)
	}

	return convertedConfigs, nil
}

func (ds *dataSource) Close() error {
	if ds.closed {
		return nil
	}
	ds.closed = true
	var err error
	if ds.cacheWriter != nil {
		err = errors.Join(err, ds.cacheWriter.Close())
	}
	for _, s := range ds.shards {
		err = errors.Join(err, s.Close())
	}
	ds.headReleaseFunc()
	return err
}

func (ds *dataSource) IsCompleted() bool {
	return ds.completed
}

type readShardResult struct {
	segment *DecodedSegment
	err     error
}

func (ds *dataSource) Read(ctx context.Context, segmentID uint32, minTimestamp int64) ([]*DecodedSegment, error) {
	if ds.completed {
		return nil, ErrEndOfBlock
	}

	isActiveHead := ds.headRecord.Status() == catalog.StatusNew || ds.headRecord.Status() == catalog.StatusActive

	wg := &sync.WaitGroup{}
	readShardResults := make([]readShardResult, len(ds.shards))
	for shardID := 0; shardID < len(ds.shards); shardID++ {
		if ds.shards[shardID].corrupted {
			readShardResults[shardID] = readShardResult{segment: nil, err: ErrShardIsCorrupted}
			continue
		}
		wg.Add(1)
		go func(shardID int) {
			defer wg.Done()
			segment, err := ds.shards[shardID].Read(ctx, segmentID, minTimestamp, isActiveHead)
			if err != nil {
				err = NewShardError(shardID, err)
			}
			readShardResults[shardID] = readShardResult{segment: segment, err: err}
		}(shardID)
	}
	wg.Wait()

	segments := make([]*DecodedSegment, 0, len(ds.shards))
	errs := make([]error, 0, len(ds.shards))
	for _, result := range readShardResults {
		if result.segment != nil {
			segments = append(segments, result.segment)
		}
		if result.err != nil {
			errs = append(errs, result.err)
		}
	}

	return segments, ds.handle(errs, isActiveHead)
}

func (ds *dataSource) handle(errs []error, isActiveHead bool) error {
	var numberOfShardIsCorruptedErrors int
	var numberOfEOFs int
	var resultErr error
	for _, err := range errs {
		switch {
		case errors.Is(err, ErrShardIsCorrupted):
			numberOfShardIsCorruptedErrors++
		case errors.Is(err, io.EOF):
			numberOfEOFs++
		}
		resultErr = errors.Join(resultErr, err)
	}

	if numberOfShardIsCorruptedErrors > 0 {
		ds.corrupted = true
		if ds.corruptMarker != nil {
			if err := ds.corruptMarker.MarkCorrupted(ds.ID); err != nil {
				return fmt.Errorf("failed to mark head corrupted: %w", err)
			}
			ds.corruptMarker = nil
		}
	}

	if numberOfShardIsCorruptedErrors == len(ds.shards) {
		ds.completed = true
		return ErrEndOfBlock
	}

	if numberOfEOFs+numberOfShardIsCorruptedErrors == len(ds.shards) {
		if isActiveHead {
			return ErrEmptyReadResult
		}
		ds.completed = true
		return ErrEndOfBlock
	}

	if numberOfEOFs > 0 {
		if isActiveHead {
			return ErrPartialReadResult
		}
		for _, err := range errs {
			var shardErr ShardError
			if errors.Is(err, io.EOF) && errors.As(err, &shardErr) {
				shardID := shardErr.ShardID()
				ds.shards[shardID].SetCorrupted()
			}
		}
		logger.Errorf("head %s contains shards with different number of segments", ds.ID)
		return nil
	}

	return resultErr
}

func (ds *dataSource) LSSes() []*cppbridge.LabelSetStorage {
	lsses := make([]*cppbridge.LabelSetStorage, len(ds.shards))
	for i := 0; i < len(ds.shards); i++ {
		lsses[i] = ds.shards[i].decoder.lss
	}
	return lsses
}

type cacheWriter struct {
	caches []*bytes.Buffer
	files  []io.Writer
	close  chan struct{}
	closed chan struct{}
}

func newCacheWriter(caches []*bytes.Buffer, files []io.Writer) *cacheWriter {
	cw := &cacheWriter{
		caches: caches,
		files:  files,
		close:  make(chan struct{}),
		closed: make(chan struct{}),
	}

	go cw.write()
	return cw
}

func (cw *cacheWriter) write() {
	defer close(cw.closed)
}

func (cw *cacheWriter) Close() error {
	close(cw.close)
	<-cw.closed
	return nil
}

func (cw *cacheWriter) Closed() <-chan struct{} {
	return cw.closed

}

func (ds *dataSource) WriteCaches() {
	if ds.cacheWriter != nil {
		select {
		case <-ds.cacheWriter.Closed():
			ds.cacheWriter = nil
		default:
			return
		}
	}

	caches := make([]*bytes.Buffer, len(ds.shards))
	files := make([]io.Writer, len(ds.shards))
	wg := &sync.WaitGroup{}
	for shardID := range ds.shards {
		wg.Add(1)
		go func(shardID uint16, shrd *shard) {
			defer wg.Done()
			buffer := bytes.NewBuffer(nil)
			if _, err := shrd.decoder.WriteTo(buffer); err != nil {
				logger.Errorf("failed to write cache: %w", err)
				return
			}
			caches[shardID] = buffer
			files[shardID] = shrd.decoderStateFile
		}(uint16(shardID), ds.shards[shardID])

	}
	wg.Wait()
	ds.cacheWriter = newCacheWriter(caches, files)
}
