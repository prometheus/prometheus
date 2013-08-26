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
	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/utility"
)

const (
	statsKey    = "leveldb.stats"
	sstablesKey = "leveldb.sstables"
)

func (l *LevelDBPersistence) State() *raw.DatabaseState {
	databaseState := &raw.DatabaseState{
		Location:     l.path,
		Name:         l.name,
		Purpose:      l.purpose,
		Supplemental: map[string]string{},
	}

	if size, err := l.Size(); err != nil {
		databaseState.Supplemental["Errors"] = err.Error()
	} else {
		databaseState.Size = utility.ByteSize(size)
	}

	databaseState.Supplemental["Low Level"], _ = l.storage.GetProperty(statsKey)
	databaseState.Supplemental["SSTable"], _ = l.storage.GetProperty(sstablesKey)

	return databaseState
}
