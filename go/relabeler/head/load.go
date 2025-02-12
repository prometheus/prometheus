package head

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/util/optional"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	HeadWalEncoderDecoderLogShards uint8 = 0
)

func Create(id string, generation uint64, dir string, configs []*config.InputRelabelerConfig, numberOfShards uint16, maxSegmentSize uint32, lastAppendedSegmentIDSetter LastAppendedSegmentIDSetter, registerer prometheus.Registerer) (_ *Head, err error) {
	lsses := make([]*LSS, numberOfShards)
	wals := make([]*ShardWal, numberOfShards)
	dataStorages := make([]*DataStorage, numberOfShards)

	defer func() {
		if err == nil {
			return
		}
		for _, wal := range wals {
			if wal != nil {
				_ = wal.Close()
			}
		}
	}()

	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		inputLss := cppbridge.NewLssStorage()
		targetLss := cppbridge.NewQueryableLssStorage()
		lsses[shardID] = &LSS{
			input:  inputLss,
			target: targetLss,
		}
		shardFilePath := filepath.Join(dir, fmt.Sprintf("shard_%d.wal", shardID))
		var shardFile *os.File
		shardFile, err = os.Create(shardFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to create shard wal file: %w", err)
		}
		shardWalEncoder := cppbridge.NewHeadWalEncoder(shardID, HeadWalEncoderDecoderLogShards, targetLss)
		shardWal := newShardWal(shardWalEncoder, false, maxSegmentSize, newBufferedShardWalWriter(shardFile))
		if err = shardWal.WriteHeader(); err != nil {
			return nil, fmt.Errorf("failed to write shard wal header: %w", err)
		}
		wals[shardID] = shardWal
		dataStorage := cppbridge.NewHeadDataStorage()
		dataStorages[shardID] = &DataStorage{
			dataStorage: dataStorage,
			encoder:     cppbridge.NewHeadEncoderWithDataStorage(dataStorage),
		}
	}

	return New(id, generation, configs, lsses, wals, dataStorages, numberOfShards, nil, lastAppendedSegmentIDSetter, registerer)
}

func Load(id string, generation uint64, dir string, configs []*config.InputRelabelerConfig, numberOfShards uint16, maxSegmentSize uint32, lastAppendedSegmentIDSetter LastAppendedSegmentIDSetter, registerer prometheus.Registerer) (_ *Head, corrupted bool, numberOfSegments uint32, err error) {
	shardLoadResults := make([]ShardLoadResult, numberOfShards)
	wg := &sync.WaitGroup{}
	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		wg.Add(1)
		shardWalFilePath := filepath.Join(dir, fmt.Sprintf("shard_%d.wal", shardID))
		go func(shardID uint16, shardWalFilePath string) {
			defer wg.Done()
			shardLoadResults[shardID] = NewShardLoader(shardWalFilePath, maxSegmentSize).Load()
		}(shardID, shardWalFilePath)
	}
	wg.Wait()

	lsses := make([]*LSS, numberOfShards)
	wals := make([]*ShardWal, numberOfShards)
	dataStorages := make([]*DataStorage, numberOfShards)
	numberOfSegmentsRead := optional.Optional[uint32]{}

	for shardID, shardLoadResult := range shardLoadResults {
		lsses[shardID] = shardLoadResult.Lss
		wals[shardID] = shardLoadResult.Wal
		dataStorages[shardID] = shardLoadResult.DataStorage
		if shardLoadResult.Corrupted {
			corrupted = true
		}
		if numberOfSegmentsRead.IsNil() {
			numberOfSegmentsRead.Set(shardLoadResult.NumberOfSegments)
		} else if numberOfSegmentsRead.Value() != shardLoadResult.NumberOfSegments {
			corrupted = true
			// calculating maximum number of segments (critical for remote write).
			if numberOfSegmentsRead.Value() < shardLoadResult.NumberOfSegments {
				numberOfSegmentsRead.Set(shardLoadResult.NumberOfSegments)
			}
		}
	}

	var lastAppendedSegmentID *uint32
	if !numberOfSegmentsRead.IsNil() && numberOfSegmentsRead.Value() > 0 {
		value := numberOfSegmentsRead.Value() - 1
		lastAppendedSegmentID = &value
	}

	defer func() {
		if err == nil {
			return
		}
		for _, wal := range wals {
			if wal != nil {
				_ = wal.Close()
			}
		}
	}()

	h, err := New(id, generation, configs, lsses, wals, dataStorages, numberOfShards, lastAppendedSegmentID, lastAppendedSegmentIDSetter, registerer)
	if err != nil {
		return nil, corrupted, numberOfSegmentsRead.Value(), fmt.Errorf("failed to create head: %w", err)
	}

	return h, corrupted, numberOfSegmentsRead.Value(), nil
}

type ShardLoader struct {
	shardFilePath  string
	maxSegmentSize uint32
}

func NewShardLoader(shardFilePath string, maxSegmentSize uint32) *ShardLoader {
	return &ShardLoader{
		shardFilePath:  shardFilePath,
		maxSegmentSize: maxSegmentSize,
	}
}

type ShardLoadResult struct {
	Lss              *LSS
	DataStorage      *DataStorage
	Wal              *ShardWal
	NumberOfSegments uint32
	Corrupted        bool
	Err              error
}

func (l *ShardLoader) Load() (result ShardLoadResult) {
	targetLss := cppbridge.NewQueryableLssStorage()
	dataStorage := NewDataStorage()

	result.Lss = &LSS{
		input:  cppbridge.NewLssStorage(),
		target: targetLss,
	}
	result.DataStorage = dataStorage
	result.Wal = newCorruptedShardWal()
	result.Corrupted = true

	shardWalFile, err := os.OpenFile(l.shardFilePath, os.O_RDWR, 0600)
	if err != nil {
		result.Err = err
		return
	}

	defer func() {
		if result.Corrupted {
			_ = shardWalFile.Close()
		}
	}()

	reader := bufio.NewReaderSize(shardWalFile, 1024*1024*4)
	_, encoderVersion, _, err := ReadHeader(reader)
	if err != nil {
		result.Err = fmt.Errorf("failed to read wal header: %w", err)
		return
	}

	decoder := cppbridge.NewHeadWalDecoder(targetLss, encoderVersion)
	lastReadSegmentID := -1

	for {
		var segment DecodedSegment
		segment, _, err = ReadSegment(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			result.Err = fmt.Errorf("failed to read segment: %w", err)
			return result
		}

		err = decoder.DecodeToDataStorage(segment.data, dataStorage.encoder)
		if err != nil {
			result.Err = fmt.Errorf("failed to decode segment: %w", err)
			return
		}

		lastReadSegmentID++
	}

	result.NumberOfSegments = uint32(lastReadSegmentID + 1)
	result.Wal = newShardWal(decoder.CreateEncoder(), true, l.maxSegmentSize, shardWalFile)
	result.Corrupted = false
	return result
}
