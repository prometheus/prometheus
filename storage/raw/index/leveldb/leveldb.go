// Copyright 2012 Prometheus Team
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
	"github.com/matttproud/prometheus/coding"
	dto "github.com/matttproud/prometheus/model/generated"
	"github.com/matttproud/prometheus/storage/raw/leveldb"
)

var (
	existenceValue = coding.NewProtocolBufferEncoder(&dto.MembershipIndexValue{})
)

type LevelDBMembershipIndex struct {
	persistence *leveldb.LevelDBPersistence
}

func (l *LevelDBMembershipIndex) Close() error {
	return l.persistence.Close()
}

func (l *LevelDBMembershipIndex) Has(key coding.Encoder) (bool, error) {
	return l.persistence.Has(key)
}

func (l *LevelDBMembershipIndex) Drop(key coding.Encoder) error {
	return l.persistence.Drop(key)
}

func (l *LevelDBMembershipIndex) Put(key coding.Encoder) error {
	return l.persistence.Put(key, existenceValue)
}

func NewLevelDBMembershipIndex(storageRoot string, cacheCapacity, bitsPerBloomFilterEncoded int) (*LevelDBMembershipIndex, error) {
	var leveldbPersistence *leveldb.LevelDBPersistence
	var persistenceError error

	if leveldbPersistence, persistenceError = leveldb.NewLevelDBPersistence(storageRoot, cacheCapacity, bitsPerBloomFilterEncoded); persistenceError == nil {
		leveldbMembershipIndex := &LevelDBMembershipIndex{
			persistence: leveldbPersistence,
		}
		return leveldbMembershipIndex, nil
	}

	return nil, persistenceError
}
