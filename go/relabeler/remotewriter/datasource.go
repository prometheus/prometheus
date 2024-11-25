package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/model/relabel"
	"path/filepath"
	"sync"
)

type dataSourceShard struct {
	walReader *walReader
	decoder   *Decoder
}

func (s *dataSourceShard) Next(ctx context.Context) (*cppbridge.DecodedRefSamples, error) {
	segment, err := s.walReader.Next()
	if err != nil {
		return nil, fmt.Errorf("failed to read next segment: %w", err)
	}

	samples, err := s.decoder.Decode(ctx, segment.Data())
	if err != nil {
		return nil, fmt.Errorf("failed to decode segment: %w", err)
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
	ID          string
	ConfigCRC32 uint32
	shards      []*dataSourceShard
	cursor      *Cursor
	restored    bool
}

func newDataSource(dataDir string, numberOfShards uint16, config DestinationConfig) (*dataSource, error) {
	cursor, err := NewCursor(filepath.Join(dataDir, fmt.Sprintf("%s.cursor", config.Name)))
	if err != nil {
		return nil, fmt.Errorf("failed to create cursor: %w", err)
	}

	cursorData, err := cursor.Read()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to read cursor"), cursor.Close())
	}

	configCrc32, err := config.CRC32()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to calculate config crc32: %w", err), cursor.Close())
	}

	var configChanged bool
	if cursorData.ConfigCRC32 != configCrc32 {
		configChanged = true
	}

	ds := &dataSource{
		ConfigCRC32: configCrc32,
		cursor:      cursor,
	}

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
			return nil, fmt.Errorf("failed to convert relabel configs: %w", err)
		}

		decoder, err = NewDecoder(
			nil,
			convertedRelabelConfigs,
			cacheFileName,
			configChanged,
			shardID,
			encoderVersion,
		)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed to create decoder: %w", err), ds.Close())
		}

		ds.shards = append(ds.shards, &dataSourceShard{
			walReader: wr,
			decoder:   decoder,
		})
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
	var err error
	for _, shard := range ds.shards {
		err = errors.Join(err, shard.Close())
	}
	return err
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
	//if !ds.restored {
	//	wg := &sync.WaitGroup{}
	//	errs := make([]error, len(ds.shards))
	//	for _, shard := range ds.shards {
	//		wg.Add(1)
	//		go func(shard *dataSourceShard) {
	//			defer wg.Done()
	//
	//		}(shard)
	//	}
	//	wg.Wait()
	//	ds.restored = true
	//}

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

func (ds *dataSource) Ack(segmentID uint32) error {
	return ds.cursor.Write(CursorData{
		SegmentID:   segmentID,
		ConfigCRC32: ds.ConfigCRC32,
	})
}
