package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type CorruptMarker interface {
	MarkCorrupted(headID uuid.UUID) error
}

type shard struct {
	corrupted                   bool
	lastReadSegmentID           *uint32
	walReader                   *walReader
	decoder                     *Decoder
	lastWrittenStateBySegmentID *uint32
	decoderStateFile            *os.File
}

func newShard(
	shardID uint16,
	shardFileName, decoderStateFileName string,
	resetDecoderState bool,
	lastWrittenStateBySegmentID *uint32,
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
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
		walReader:        wr,
		decoder:          decoder,
		decoderStateFile: decoderStateFile,
	}

	if !resetDecoderState {
		if err = decoder.LoadFrom(decoderStateFile); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to restore from cache: %w", err), s.Close())
		}
		if lastWrittenStateBySegmentID != nil {
			s.lastWrittenStateBySegmentID = &(*lastWrittenStateBySegmentID)
		}
	}

	return s, nil
}

func (s *shard) Read(ctx context.Context, targetSegmentID uint32, minTimestamp int64, isActiveHead bool) (*DecodedSegment, error) {
	if s.corrupted {
		return nil, ErrShardIsCorrupted
	}

	if s.lastReadSegmentID != nil && *s.lastReadSegmentID >= targetSegmentID {
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
				return nil, io.EOF
			}
			s.corrupted = true
			return nil, errors.Join(err, ErrShardIsCorrupted)
		}

		decodedSegment, err := s.decoder.Decode(segment.Data(), minTimestamp)
		if err != nil {
			s.corrupted = true
			return nil, errors.Join(err, ErrShardIsCorrupted)
		}

		s.lastReadSegmentID = &segment.ID

		if s.lastWrittenStateBySegmentID == nil || *s.lastWrittenStateBySegmentID < segment.ID {
			if _, err = s.decoder.WriteTo(s.decoderStateFile); err != nil {
				// todo: i che delat' to s oshibkoi???
				logger.Errorf("failed to write decoder state: %v", err)
			} else {
				s.lastWrittenStateBySegmentID = &segment.ID
			}
		}

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
	ID              uuid.UUID
	headRecord      *catalog.Record
	shards          []*shard
	corruptMarker   CorruptMarker
	closed          bool
	completed       bool
	corrupted       bool
	headReleaseFunc func()
}

func newDataSource(dataDir string, numberOfShards uint16, config DestinationConfig, lastAcknowledgedSegmentID *uint32, discardCache bool, corruptMarker CorruptMarker, headRecord *catalog.Record) (*dataSource, error) {
	var err error
	var convertedRelabelConfigs []*cppbridge.RelabelConfig
	convertedRelabelConfigs, err = convertRelabelConfigs(config.WriteRelabelConfigs...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert relabel configs: %w", err)
	}

	headReleaseFunc := headRecord.Acquire()
	b := &dataSource{
		headRecord:      headRecord,
		corruptMarker:   corruptMarker,
		headReleaseFunc: headReleaseFunc,
	}

	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		shardFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.wal", shardID))
		decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("%s_shard_%d.state", config.Name, shardID))
		var s *shard
		s, err = newShard(shardID, shardFileName, decoderStateFileName, discardCache, lastAcknowledgedSegmentID, config.ExternalLabels, convertedRelabelConfigs)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed to create shard: %w", err), b.Close())
		}
		b.shards = append(b.shards, s)
	}

	return b, nil
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

	isActiveHead := ds.headRecord.Status() == catalog.StatusNew

	wg := &sync.WaitGroup{}
	readShardResults := make([]readShardResult, len(ds.shards))
	for i := 0; i < len(ds.shards); i++ {
		if ds.shards[i].corrupted {
			readShardResults[i] = readShardResult{segment: nil, err: ErrShardIsCorrupted}
			continue
		}
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			segment, err := ds.shards[index].Read(ctx, segmentID, minTimestamp, isActiveHead)
			readShardResults[index] = readShardResult{segment: segment, err: err}
		}(i)
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
		// shards contains different number of segments
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
