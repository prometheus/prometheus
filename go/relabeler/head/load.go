package head

import (
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/client_golang/prometheus"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var shardFileNameRegexp = regexp.MustCompile(`shard_\d`)

func Create(id string, generation uint64, dir string, configs []*config.InputRelabelerConfig, numberOfShards uint16, registerer prometheus.Registerer) (_ *Head, err error) {
	d, err := os.OpenFile(dir, os.O_RDONLY, 0777)
	if err != nil {
		return nil, fmt.Errorf("failed to open dir: %w", err)
	}
	defer func() { _ = d.Close() }()

	lsses := make([]*cppbridge.LabelSetStorage, numberOfShards)
	wals := make([]*ShardWal, numberOfShards)
	dataStorages := make([]*DataStorage, numberOfShards)

	defer func() {
		if err != nil {
			for _, wal := range wals {
				if wal != nil {
					_ = wal.Close()
				}
			}
		}
	}()

	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		lss := cppbridge.NewQueryableLssStorage()
		lsses[shardID] = lss
		shardFilePath := filepath.Join(dir, fmt.Sprintf("shard_%d.wal", shardID))
		var shardFile *os.File
		shardFile, err = os.Create(shardFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to create shard wal file: %w", err)
		}
		shardWalEncoder := cppbridge.NewHeadWalEncoder(shardID, uint8(numberOfShards), lss)
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
	d, err := os.OpenFile(dir, os.O_RDONLY, 0777)
	if err != nil {
		return nil, fmt.Errorf("failed to open dir: %w", err)
	}
	defer func() { _ = d.Close() }()
	fmt.Println("tmddir:", d.Name())

	lsses := make([]*cppbridge.LabelSetStorage, numberOfShards)
	wals := make([]*ShardWal, numberOfShards)
	dataStorages := make([]*DataStorage, numberOfShards)

	defer func() {
		if err != nil {
			for _, wal := range wals {
				if wal != nil {
					_ = wal.Close()
				}
			}
		}
	}()

	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		lss := cppbridge.NewQueryableLssStorage()
		lsses[shardID] = lss
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
		offset, encoder, err = replayWal(shardFile, lss, dataStorages[shardID])
		if err != nil {
			return nil, fmt.Errorf("failed to replay wal: %w", err)
		}
		fmt.Println("creating shard", shardID, "offset", offset)
		wals[shardID] = newShardWal(encoder, offset != 0, shardFile)
	}

	return New(id, generation, configs, lsses, wals, dataStorages, numberOfShards, registerer)
}

func replayWal(file *os.File, lss *cppbridge.LabelSetStorage, dataStorage *DataStorage) (offset int, encoder *cppbridge.HeadWalEncoder, err error) {
	fmt.Println("replaying wal file", file.Name())
	_, encoderVersion, _, err := ReadHeader(file)
	if err != nil {
		return offset, nil, fmt.Errorf("failed to read header: %w", err)
	}

	decoder := cppbridge.NewHeadWalDecoder(lss, encoderVersion)
	innerSeries := cppbridge.NewInnerSeries()
	segmentID := -1
	hexFile, err := os.Create("failed_shard_0.wal")
	if err != nil {
		return offset, nil, err
	}
	defer hexFile.Close()

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
		segmentID++
		_, err = hexFile.WriteString(fmt.Sprintf("%d\n", segment.sampleCount))
		if err != nil {
			return offset, nil, err
		}
		_, err = hexFile.WriteString(fmt.Sprintf("%x\n\n", segment.Data()))
		if err != nil {
			return offset, nil, err
		}

		fmt.Println("decoding segment id:", segmentID)
		err = decoder.Decode(segment.Data(), innerSeries)
		if err != nil {
			return offset, nil, fmt.Errorf("failed to decode segment: %w", err)
		}

		dataStorage.AppendInnerSeriesSlice([]*cppbridge.InnerSeries{innerSeries})
	}

}
