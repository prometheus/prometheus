// Copyright 2013 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package leveldb

import (
	"github.com/prometheus/prometheus/utility"
	"time"
)

const (
	statsKey    = "leveldb.stats"
	sstablesKey = "leveldb.sstables"
)

// DatabaseState models a bundle of metadata about a LevelDB database used in
// template format string interpolation.
type DatabaseState struct {
	LastRefreshed   time.Time
	Type            string
	Name            string
	Path            string
	LowLevelStatus  string
	SSTablesStatus  string
	ApproximateSize utility.ByteSize
	Error           error
}

func (l *LevelDBPersistence) State() DatabaseState {
	databaseState := DatabaseState{
		LastRefreshed:  time.Now(),
		Path:           l.path,
		LowLevelStatus: l.storage.PropertyValue(statsKey),
		SSTablesStatus: l.storage.PropertyValue(sstablesKey),
	}

	if size, err := l.ApproximateSize(); err != nil {
		databaseState.Error = err
	} else {
		databaseState.ApproximateSize = utility.ByteSize(size)
	}

	return databaseState
}
