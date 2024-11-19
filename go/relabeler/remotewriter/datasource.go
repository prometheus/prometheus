package remotewriter

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
)

type dataSourceShard struct {
	walReader *walReader
	decoder   any
}

func (s *dataSourceShard) Next() (Segment, error) {
	return s.walReader.Next()
}

func (s *dataSourceShard) Close() error {
	return s.walReader.Close()
}

type dataSource struct {
	ID          string
	ConfigCRC32 uint32
	shards      []*dataSourceShard
	cursor      *Cursor
}

func newDataSource(dataDir string, numberOfShards uint16, destinationName string, configCRC32 uint32) (*dataSource, error) {
	cursor, err := NewCursor(filepath.Join(dataDir, fmt.Sprintf("%s.cursor", destinationName)))
	if err != nil {
		return nil, fmt.Errorf("failed to create cursor: %w", err)
	}

	cursorData, err := cursor.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read cursor")
	}

	var configChanged bool
	if cursorData.ConfigCRC32 != configCRC32 {
		configChanged = true
	}

	ds := &dataSource{
		ConfigCRC32: configCRC32,
		cursor:      cursor,
	}

	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		shardFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.wal", shardID))
		wr, err := newWalReader(shardFileName)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed to create wal file reader: %w", err), ds.Close())
		}
		ds.shards = append(ds.shards, &dataSourceShard{
			walReader: wr,
			decoder:   nil,
		})

		_ = configChanged
	}

	return ds, nil
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
	segment Segment
	err     error
}

func (ds *dataSource) Next() ([]Segment, error) {
	wg := &sync.WaitGroup{}
	nextSegmentResults := make([]nextSegmentResult, len(ds.shards))
	for i := 0; i < len(ds.shards); i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			segment, err := ds.shards[index].Next()
			nextSegmentResults[index] = nextSegmentResult{segment: segment, err: err}
		}(i)
	}
	wg.Wait()

	segments := make([]Segment, 0, len(ds.shards))
	var err error
	for _, result := range nextSegmentResults {
		segments = append(segments, result.segment)
		err = errors.Join(err, result.err)
	}

	return segments, err
}

func (ds *dataSource) Ack(segmentID uint32) error {
	return ds.cursor.Write(CursorData{
		SegmentID:   segmentID,
		ConfigCRC32: ds.ConfigCRC32,
	})
}
