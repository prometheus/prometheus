package head

import "github.com/prometheus/prometheus/pp/go/cppbridge"

type Configurator struct {
	dataDir        string
	numberOfShards uint16
	lsses          []*cppbridge.LabelSetStorage
	wals           []*ShardWal
	dataStorages   []*DataStorage
}
