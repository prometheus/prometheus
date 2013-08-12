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
	"code.google.com/p/goprotobuf/proto"

	dto "github.com/prometheus/prometheus/model/generated"

	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
)

var existenceValue = new(dto.MembershipIndexValue)

type LevelDBMembershipIndex struct {
	persistence *leveldb.LevelDBPersistence
}

func (l *LevelDBMembershipIndex) Close() {
	l.persistence.Close()
}

func (l *LevelDBMembershipIndex) Has(k proto.Message) (bool, error) {
	return l.persistence.Has(k)
}

func (l *LevelDBMembershipIndex) Drop(k proto.Message) error {
	return l.persistence.Drop(k)
}

func (l *LevelDBMembershipIndex) Put(k proto.Message) error {
	return l.persistence.Put(k, existenceValue)
}

type LevelDBIndexOptions struct {
	leveldb.LevelDBOptions
}

func NewLevelDBMembershipIndex(o LevelDBIndexOptions) (i *LevelDBMembershipIndex, err error) {
	leveldbPersistence, err := leveldb.NewLevelDBPersistence(o.LevelDBOptions)
	if err != nil {
		return nil, err
	}

	return &LevelDBMembershipIndex{
		persistence: leveldbPersistence,
	}, nil
}

func (l *LevelDBMembershipIndex) Commit(batch raw.Batch) error {
	return l.persistence.Commit(batch)
}

// CompactKeyspace compacts the entire database's keyspace.
//
// Beware that it would probably be imprudent to run this on a live user-facing
// server due to latency implications.
func (l *LevelDBMembershipIndex) Prune() {
	l.persistence.Prune()
}

func (l *LevelDBMembershipIndex) Size() (uint64, error) {
	return l.persistence.Size()
}

func (l *LevelDBMembershipIndex) State() *raw.DatabaseState {
	return l.persistence.State()
}
