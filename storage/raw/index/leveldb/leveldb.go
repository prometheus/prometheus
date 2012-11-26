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
	data "github.com/matttproud/prometheus/model/generated"
	"github.com/matttproud/prometheus/storage/raw/leveldb"
)

var (
	existenceValue = coding.NewProtocolBufferEncoder(&data.MembershipIndexValueDDO{})
)

type LevigoMembershipIndex struct {
	persistence *leveldb.LevigoPersistence
}

func (l *LevigoMembershipIndex) Close() error {
	return l.persistence.Close()
}

func (l *LevigoMembershipIndex) Has(key coding.Encoder) (bool, error) {
	return l.persistence.Has(key)
}

func (l *LevigoMembershipIndex) Drop(key coding.Encoder) error {
	return l.persistence.Drop(key)
}

func (l *LevigoMembershipIndex) Put(key coding.Encoder) error {
	return l.persistence.Put(key, existenceValue)
}

func NewLevigoMembershipIndex(storageRoot string, cacheCapacity, bitsPerBloomFilterEncoded int) (*LevigoMembershipIndex, error) {
	var levigoPersistence *leveldb.LevigoPersistence
	var levigoPersistenceError error

	if levigoPersistence, levigoPersistenceError = leveldb.NewLevigoPersistence(storageRoot, cacheCapacity, bitsPerBloomFilterEncoded); levigoPersistenceError == nil {
		levigoMembershipIndex := &LevigoMembershipIndex{
			persistence: levigoPersistence,
		}
		return levigoMembershipIndex, nil
	}

	return nil, levigoPersistenceError
}
