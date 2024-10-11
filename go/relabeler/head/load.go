package head

import (
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/client_golang/prometheus"
	"os"
	"regexp"
)

var shardFileNameRegexp = regexp.MustCompile(`shard_\d`)

func Load(generation uint64, dir string, configs []*config.InputRelabelerConfig, numberOfShards uint16, registerer prometheus.Registerer) (*Head, error) {
	d, err := os.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to open dir: %w", err)
	}
	defer func() { _ = d.Close() }()

	fileNames, err := d.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("failed to read file names from dir: %w", err)
	}

	lsses := make([]*cppbridge.LabelSetStorage, numberOfShards)
	wals := make([]*ShardWal, numberOfShards)
	dataStorages := make([]*DataStorage, numberOfShards)

	var shardID uint16
	// empty dir case - new head
	if len(fileNames) == 0 {
		for ; shardID < numberOfShards; shardID++ {
			lss := cppbridge.NewQueryableLssStorage()
			lsses[shardID] = lss
			shardFileName := fmt.Sprintf("shard_%d", shardID)
			shardFile, shardFileErr := os.OpenFile(shardFileName, os.O_CREATE|os.O_RDWR, 0666)
			if shardFileErr != nil {
				return nil, fmt.Errorf("failed to create shard wal file: %w", err)
			}
			wals[shardID] = newShardWal(shardID, uint8(numberOfShards), lss, shardFile)
			dataStorage := cppbridge.NewHeadDataStorage()
			dataStorages[shardID] = &DataStorage{
				dataStorage: dataStorage,
				encoder:     cppbridge.NewHeadEncoderWithDataStorage(dataStorage),
			}
		}

		return New(generation, configs, lsses, wals, dataStorages, numberOfShards, registerer)
	}

	// existing head case
	for _, fileName := range fileNames {
		if !shardFileNameRegexp.MatchString(fileName) {
			continue
		}

		// load shard file
	}

	panic("implement me")
}
