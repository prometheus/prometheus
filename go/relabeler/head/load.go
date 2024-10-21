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
	"sort"
	"strconv"
	"strings"
)

var shardFileNameRegexp = regexp.MustCompile(`shard_\d`)

func Load(id string, generation uint64, dir string, configs []*config.InputRelabelerConfig, numberOfShards uint16, registerer prometheus.Registerer) (_ *Head, err error) {
	d, err := os.OpenFile(dir, os.O_RDONLY, 0777)
	if err != nil {
		return nil, fmt.Errorf("failed to open dir: %w", err)
	}
	defer func() { _ = d.Close() }()
	fmt.Println("tmddir:", d.Name())

	fileNames, err := d.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("failed to read file names from dir: %w", err)
	}

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

	var shardID uint16
	// empty dir case - new head
	if len(fileNames) == 0 {
		for ; shardID < numberOfShards; shardID++ {
			lss := cppbridge.NewQueryableLssStorage()
			lsses[shardID] = lss
			shardFilePath := filepath.Join(dir, fmt.Sprintf("shard_%d", shardID))
			var shardFile *os.File
			shardFile, err = os.OpenFile(shardFilePath, os.O_CREATE|os.O_RDWR, 0666)
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

	// existing head case
	shardFileNames, err := findShardFiles(fileNames)
	if err != nil {
		return nil, fmt.Errorf("failed to find shard files: %w", err)
	}

	for _, shardFileName := range shardFileNames {
		lss := cppbridge.NewQueryableLssStorage()
		lsses[shardID] = lss
		dataStorage := cppbridge.NewHeadDataStorage()
		dataStorages[shardID] = &DataStorage{
			dataStorage: dataStorage,
			encoder:     cppbridge.NewHeadEncoderWithDataStorage(dataStorage),
		}

		shardFilePath := filepath.Join(dir, shardFileName)
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

		wals[shardID] = newShardWal(encoder, offset != 0, shardFile)
		shardID++
	}

	return New(id, generation, configs, lsses, wals, dataStorages, numberOfShards, registerer)
}

func findShardFiles(fileNames []string) ([]string, error) {
	type shard struct {
		id       uint16
		fileName string
	}

	var shards []shard
	for _, fileName := range fileNames {
		if !shardFileNameRegexp.MatchString(fileName) {
			continue
		}

		shardID, err := strconv.ParseInt(strings.TrimLeft(fileName, "shard_"), 10, 64)
		if err != nil {
			continue
		}

		shards = append(shards, shard{
			id:       uint16(shardID),
			fileName: fileName,
		})
	}

	sort.Slice(shards, func(i, j int) bool {
		return shards[i].id < shards[j].id
	})

	result := make([]string, 0, len(shards))
	for _, s := range shards {
		result = append(result, s.fileName)
	}

	return result, nil
}

func replayWal(file *os.File, lss *cppbridge.LabelSetStorage, dataStorage *DataStorage) (offset int, encoder *cppbridge.HeadWalEncoder, err error) {
	_, encoderVersion, _, err := ReadHeader(file)
	if err != nil {
		return offset, nil, fmt.Errorf("failed to read header: %w", err)
	}

	decoder := cppbridge.NewHeadWalDecoder(lss, encoderVersion)
	var innerSeries *cppbridge.InnerSeries
	var segment []byte
	for {
		_, _, segment, _, err = ReadSegment(file)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return offset, decoder.CreateEncoder(), nil
			}
			return offset, nil, fmt.Errorf("failed to read segment: %w", err)
		}

		innerSeries, err = decoder.Decode(segment)
		if err != nil {
			return offset, nil, fmt.Errorf("failed to decode segment: %w", err)
		}

		dataStorage.AppendInnerSeriesSlice([]*cppbridge.InnerSeries{innerSeries})
	}

}
