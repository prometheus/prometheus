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
	"bytes"
	"fmt"

	"github.com/prometheus/prometheus/utility"
)

const (
	statsKey    = "leveldb.stats"
	sstablesKey = "leveldb.sstables"
)

// DatabaseState models a bundle of metadata about a LevelDB database used in
// template format string interpolation.
type DatabaseState struct {
	Name            string
	Purpose         string
	Path            string
	LowLevelStatus  string
	SSTablesStatus  string
	ApproximateSize utility.ByteSize
	Error           error
}

func (s DatabaseState) String() string {
	b := new(bytes.Buffer)

	fmt.Fprintln(b, "Name:", s.Name)
	fmt.Fprintln(b, "Path:", s.Path)
	fmt.Fprintln(b, "Purpose:", s.Purpose)
	fmt.Fprintln(b, "Low Level Diagnostics:", s.LowLevelStatus)
	fmt.Fprintln(b, "SSTable Statistics:", s.SSTablesStatus)
	fmt.Fprintln(b, "Approximate Size:", s.ApproximateSize)
	fmt.Fprintln(b, "Error:", s.Error)

	return b.String()
}

func (l *LevelDBPersistence) LowLevelState() DatabaseState {
	databaseState := DatabaseState{
		Path:           l.path,
		Name:           l.name,
		Purpose:        l.purpose,
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

func (l *LevelDBPersistence) State() string {
	return l.LowLevelState().String()
}
