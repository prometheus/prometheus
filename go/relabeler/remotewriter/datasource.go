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
	shardID     int
	processable bool
	err         error
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

func NewShardError(shardID int, processable bool, err error) ShardError {
	return ShardError{
		shardID:     shardID,
		processable: processable,
		err:         err,
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
	headID             string
	shardID            uint16
	corrupted          bool
	lastReadSegmentID  optional.Optional[uint32]
	walReader          ShardWalReader
	decoder            *Decoder
	decoderStateFile   DecoderStateWriteCloser
	unexpectedEOFCount prometheus.Counter
	segmentSize        prometheus.Histogram
}

func newShard(
	headID string,
	shardID uint16,
	shardFileName, decoderStateFileName string,
	resetDecoderState bool,
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
	unexpectedEOFCount prometheus.Counter,
	segmentSize prometheus.Histogram,
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
		headID:             headID,
		shardID:            shardID,
		walReader:          wr,
		decoder:            decoder,
		decoderStateFile:   decoderStateFile,
		unexpectedEOFCount: unexpectedEOFCount,
		segmentSize:        segmentSize,
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

func (s *shard) Read(ctx context.Context, targetSegmentID uint32, minTimestamp int64) (*DecodedSegment, error) {
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
			s.corrupted = true
			logger.Errorf("remotewritedebug shard %s/%d is corrupted by read: %v", s.headID, s.shardID, err)
			return nil, errors.Join(err, ErrShardIsCorrupted)
		}

		s.segmentSize.Observe(float64(len(segment.Data())))

		decodedSegment, err := s.decoder.Decode(segment.Data(), minTimestamp)
		if err != nil {
			s.corrupted = true
			logger.Errorf("remotewritedebug shard %s/%d is corrupted by decode: %v", s.headID, s.shardID, err)
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

type SegmentReadyChecker interface {
	SegmentIsReady(segmentID uint32) (ready bool, outOfRange bool)
}

type segmentReadyChecker struct {
	headRecord *catalog.Record
}

func newSegmentReadyChecker(headRecord *catalog.Record) *segmentReadyChecker {
	return &segmentReadyChecker{headRecord: headRecord}
}

func (src *segmentReadyChecker) SegmentIsReady(segmentID uint32) (ready bool, outOfRange bool) {
	ready = src.headRecord.LastAppendedSegmentID() != nil && *src.headRecord.LastAppendedSegmentID() >= segmentID
	outOfRange = (src.headRecord.Status() != catalog.StatusNew && src.headRecord.Status() != catalog.StatusActive) && !ready
	return ready, outOfRange
}

type shardCache struct {
	shardID uint16
	cache   *bytes.Buffer
	written bool
	writer  io.Writer
}

type dataSource struct {
	ID                  string
	shards              []*shard
	segmentReadyChecker SegmentReadyChecker
	corruptMarker       CorruptMarker
	closed              bool
	completed           bool
	corrupted           bool
	headReleaseFunc     func()

	lssSlice []*cppbridge.LabelSetStorage

	cacheMtx             sync.Mutex
	caches               []*shardCache
	cacheWriteSignal     chan struct{}
	cacheWriteLoopClosed chan struct{}

	unexpectedEOFCount prometheus.Counter
	segmentSize        prometheus.Histogram
}

func newDataSource(dataDir string,
	numberOfShards uint16,
	config DestinationConfig,
	discardCache bool,
	segmentReadyChecker SegmentReadyChecker,
	corruptMarker CorruptMarker,
	headRecord *catalog.Record,
	unexpectedEOFCount prometheus.Counter,
	segmentSize prometheus.Histogram,
) (*dataSource, error) {
	var err error
	var convertedRelabelConfigs []*cppbridge.RelabelConfig
	convertedRelabelConfigs, err = convertRelabelConfigs(config.WriteRelabelConfigs...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert relabel configs: %w", err)
	}

	b := &dataSource{
		corruptMarker:        corruptMarker,
		segmentReadyChecker:  segmentReadyChecker,
		headReleaseFunc:      headRecord.Acquire(),
		unexpectedEOFCount:   unexpectedEOFCount,
		segmentSize:          segmentSize,
		cacheWriteSignal:     make(chan struct{}),
		cacheWriteLoopClosed: make(chan struct{}),
	}

	go b.cacheWriteLoop()

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
			b.unexpectedEOFCount,
			b.segmentSize,
		)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed to create shard: %w", err), b.Close())
		}
		b.shards = append(b.shards, s)
		b.lssSlice = append(b.lssSlice, s.decoder.lss)
		b.caches = append(b.caches, &shardCache{
			shardID: shardID,
			cache:   bytes.NewBuffer(nil),
			written: true,
			writer:  s.decoderStateFile,
		})
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
	unexpectedEOFCount prometheus.Counter,
	segmentSize prometheus.Histogram,
) (*shard, error) {
	s, err := newShard(
		headID,
		shardID,
		shardFileName,
		decoderStateFileName,
		resetDecoderState,
		externalLabels,
		relabelConfigs,
		unexpectedEOFCount,
		segmentSize,
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
			unexpectedEOFCount,
			segmentSize,
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
	// stop cache writing first
	close(ds.cacheWriteSignal)
	<-ds.cacheWriteLoopClosed

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

	segmentIsReady, segmentIsOutOfRange := ds.segmentReadyChecker.SegmentIsReady(segmentID)
	if !segmentIsReady {
		if segmentIsOutOfRange {
			return nil, ErrEndOfBlock
		}

		return nil, ErrEmptyReadResult
	}

	wg := &sync.WaitGroup{}
	readShardResults := make([]readShardResult, len(ds.shards))
	for shardID := 0; shardID < len(ds.shards); shardID++ {
		if ds.shards[shardID].corrupted {
			readShardResults[shardID] = readShardResult{segment: nil, err: NewShardError(shardID, false, ErrShardIsCorrupted)}
			continue
		}
		wg.Add(1)
		go func(shardID int) {
			defer wg.Done()
			segment, err := ds.shards[shardID].Read(ctx, segmentID, minTimestamp)
			if err != nil {
				err = NewShardError(shardID, true, err)
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

	return segments, ds.handleReadErrors(errs)
}

func (ds *dataSource) handleReadErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	if len(errs) == len(ds.shards) {
		ds.corrupted = true
		if ds.corruptMarker != nil {
			if err := ds.corruptMarker.MarkCorrupted(ds.ID); err != nil {
				return fmt.Errorf("failed to mark head corrupted: %w", err)
			}
			ds.corruptMarker = nil
		}

		return ErrEndOfBlock
	}

	ds.corrupted = true
	if ds.corruptMarker != nil {
		if err := ds.corruptMarker.MarkCorrupted(ds.ID); err != nil {
			return fmt.Errorf("failed to mark head corrupted: %w", err)
		}
		ds.corruptMarker = nil
	}

	for _, err := range errs {
		var shardErr ShardError
		if errors.As(err, &shardErr) {
			if shardErr.processable {
				logger.Errorf("shard %s/%d is corrupted", ds.ID, shardErr.ShardID())
			}
		}
	}

	return nil
}

func (ds *dataSource) LSSes() []*cppbridge.LabelSetStorage {
	return ds.lssSlice
}

func (ds *dataSource) WriteCaches() {
	ds.cacheMtx.Lock()
	for shardID, sc := range ds.caches {
		if !sc.written {
			continue
		}
		sc.cache.Reset()
		if _, err := ds.shards[shardID].decoder.WriteTo(sc.cache); err != nil {
			logger.Errorf("failed to get output decoder cache: %v", err)
			continue
		}
		sc.written = false
	}
	ds.cacheMtx.Unlock()

	select {
	case ds.cacheWriteSignal <- struct{}{}:
	default:
	}
}

func (ds *dataSource) cacheWriteLoop() {
	defer close(ds.cacheWriteLoopClosed)
	var closed bool
	var writeRequested bool
	var writeResultc chan struct{}

	for {
		if writeRequested && !closed && writeResultc == nil {
			writeResultc = make(chan struct{})
			go func() {
				defer close(writeResultc)
				ds.writeCaches()
			}()
			writeRequested = false
		}

		if closed && writeResultc == nil {
			return
		}

		select {
		case _, ok := <-ds.cacheWriteSignal:
			if !ok {
				return
			}
			writeRequested = true
		case <-writeResultc:
			writeResultc = nil
		}
	}
}

func (ds *dataSource) writeCaches() {
	ds.cacheMtx.Lock()
	caches := make([]*shardCache, 0, len(ds.caches))
	for _, sc := range ds.caches {
		if sc.written {
			continue
		}
		sc := sc
		caches = append(caches, sc)
	}
	ds.cacheMtx.Unlock()

	writtenCacheShardIDs := make([]uint16, 0, len(caches))
	for _, sc := range caches {
		if _, err := sc.cache.WriteTo(sc.writer); err != nil {
			logger.Errorf("failed to write cache: %v", err)
			continue
		}
		writtenCacheShardIDs = append(writtenCacheShardIDs, sc.shardID)
	}

	if len(writtenCacheShardIDs) > 0 {
		ds.cacheMtx.Lock()
		for _, shardID := range writtenCacheShardIDs {
			ds.caches[shardID].written = true
		}
		ds.cacheMtx.Unlock()
	}
}
