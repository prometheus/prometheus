package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/scaleway/scaleway-sdk-go/logger"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type dataSourceShard struct {
	corrupted bool
	walReader *walReader
	decoder   *Decoder
}

func (s *dataSourceShard) Next(ctx context.Context) (*cppbridge.DecodedRefSamples, error) {
	if s.corrupted {
		return nil, ErrCorrupted
	}

	segment, err := s.walReader.Next()
	if err != nil {
		if errors.Is(err, ErrNoData) || errors.Is(err, io.EOF) {
			return nil, err
		}

		s.corrupted = true
		return nil, ErrCorrupted
	}

	samples, err := s.decoder.Decode(ctx, segment.Data())
	if err != nil {
		s.corrupted = true
		return nil, errors.Join(err, ErrCorrupted)
	}

	if err = s.decoder.WriteCache(); err != nil {
		logger.Errorf("failed to write cache: %w", err)
	}

	return samples, nil
}

func (s *dataSourceShard) Restore() error {
	return nil
}

func (s *dataSourceShard) Close() error {
	return s.walReader.Close()
}

type dataSource struct {
	ID              string
	shards          []*dataSourceShard
	writeCompletion func() error
	closed          bool
	completed       bool
	corrupted       bool
	restored        bool
}

func newDataSource(dataDir string, numberOfShards uint16, config DestinationConfig, discardCache bool) (*dataSource, error) {
	ds := &dataSource{
		writeCompletion: func() error {
			return os.WriteFile(filepath.Join(dataDir, fmt.Sprintf("%s.completed", config.Name)), nil, 0600)
		},
	}

	var err error
	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		var wr *walReader
		var encoderVersion uint8
		shardFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.wal", shardID))
		wr, encoderVersion, err = newWalReader(shardFileName)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed to create wal file reader: %w", err), ds.Close())
		}
		cacheFileName := filepath.Join(dataDir, fmt.Sprintf("%s_shard_%d.cache", config.Name, shardID))
		var decoder *Decoder
		var convertedRelabelConfigs []*cppbridge.RelabelConfig
		convertedRelabelConfigs, err = convertRelabelConfigs(config.WriteRelabelConfigs...)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed to convert relabel configs: %w", err), ds.Close())
		}

		decoder, err = NewDecoder(
			nil,
			convertedRelabelConfigs,
			cacheFileName,
			discardCache,
			shardID,
			encoderVersion,
		)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed to create decoder: %w", err), ds.Close())
		}

		shard := &dataSourceShard{
			walReader: wr,
			decoder:   decoder,
		}

		ds.shards = append(ds.shards, shard)
	}

	return ds, nil
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
	for _, shard := range ds.shards {
		err = errors.Join(err, shard.Close())
	}
	return err
}

func (ds *dataSource) IsCompleted() bool {
	return ds.completed
}

func (ds *dataSource) WriteCompletion() error {
	if ds.completed && ds.writeCompletion != nil {
		return ds.writeCompletion()
	}

	return nil
}

type dataSourceNextResult struct {
	data []byte
	err  error
}

type nextSegmentResult struct {
	segmentID uint32
	segment   *cppbridge.DecodedRefSamples
	err       error
}

func (ds *dataSource) Next(ctx context.Context) ([]*cppbridge.DecodedRefSamples, uint32, error) {
	wg := &sync.WaitGroup{}
	nextSegmentResults := make([]nextSegmentResult, len(ds.shards))
	for i := 0; i < len(ds.shards); i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			segment, err := ds.shards[index].Next(ctx)
			nextSegmentResults[index] = nextSegmentResult{segment: segment, err: err}
		}(i)
	}
	wg.Wait()

	segments := make([]*cppbridge.DecodedRefSamples, 0, len(ds.shards))
	var err error
	for _, result := range nextSegmentResults {
		segments = append(segments, result.segment)
		err = errors.Join(err, result.err)
	}

	var segmentID uint32
	if len(ds.shards) > 0 {
		segmentID = nextSegmentResults[0].segmentID
	}
	return segments, segmentID, err
}

func (ds *dataSource) LSSes() []*cppbridge.LabelSetStorage {
	lsses := make([]*cppbridge.LabelSetStorage, len(ds.shards))
	for i := 0; i < len(ds.shards); i++ {
		lsses[i] = ds.shards[i].decoder.lss
	}
	return lsses
}
