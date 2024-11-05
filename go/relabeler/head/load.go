package head

import (
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/client_golang/prometheus"
	"io"
	"os"
	"path/filepath"
)

const (
	HeadWalEncoderDecoderLogShards uint8 = 0
)

func Create(id string, generation uint64, dir string, configs []*config.InputRelabelerConfig, numberOfShards uint16, registerer prometheus.Registerer) (_ *Head, err error) {
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
		wals[shardID] = newShardWal(shardWalEncoder, false, shardFile)
		dataStorage := cppbridge.NewHeadDataStorage()
		dataStorages[shardID] = &DataStorage{
			dataStorage: dataStorage,
			encoder:     cppbridge.NewHeadEncoderWithDataStorage(dataStorage),
		}
	}

	return New(id, generation, configs, lsses, wals, dataStorages, numberOfShards, registerer)
}

func Load(id string, generation uint64, dir string, configs []*config.InputRelabelerConfig, numberOfShards uint16, registerer prometheus.Registerer) (_ *Head, err error) {
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
		dataStorage := cppbridge.NewHeadDataStorage()
		dataStorages[shardID] = &DataStorage{
			dataStorage: dataStorage,
			encoder:     cppbridge.NewHeadEncoderWithDataStorage(dataStorage),
		}
		shardFilePath := filepath.Join(dir, fmt.Sprintf("shard_%d.wal", shardID))
		var shardFile *os.File
		shardFile, err = os.OpenFile(shardFilePath, os.O_CREATE|os.O_RDWR, 0666)
		if err != nil {
			return nil, fmt.Errorf("failed to create shard wal file: %w", err)
		}

		var offset int
		var encoder *cppbridge.HeadWalEncoder
		offset, encoder, err = replayWal(shardFile, targetLss, dataStorages[shardID])
		if err != nil {
			return nil, fmt.Errorf("failed to replay wal: %w", err)
		}
		wals[shardID] = newShardWal(encoder, offset != 0, shardFile)
	}

	return New(id, generation, configs, lsses, wals, dataStorages, numberOfShards, registerer)
}

func replayWal(file *os.File, lss *cppbridge.LabelSetStorage, dataStorage *DataStorage) (offset int, encoder *cppbridge.HeadWalEncoder, err error) {
	logger.Debugf("replaying wal file", file.Name())
	_, encoderVersion, _, err := ReadHeader(file)
	if err != nil {
		return offset, nil, fmt.Errorf("failed to read header: %w", err)
	}

	decoder := cppbridge.NewHeadWalDecoder(lss, encoderVersion)
	innerSeries := cppbridge.NewInnerSeries()

	for {
		var bytesRead int
		var segment DecodedSegment
		segment, bytesRead, err = ReadSegment(file)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return offset, decoder.CreateEncoder(), nil
			}
			return offset, nil, fmt.Errorf("failed to read segment: %w", err)
		}
		offset += bytesRead

		err = decoder.Decode(segment.Data(), innerSeries)
		if err != nil {
			return offset, nil, fmt.Errorf("failed to decode segment: %w", err)
		}

		dataStorage.AppendInnerSeriesSlice([]*cppbridge.InnerSeries{innerSeries})
	}

}
