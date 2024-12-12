// Copyright 2024 The Prometheus Authors
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

package encoding

import (
	"encoding/binary"
)

type Key [DataKeySize]byte

func (k *Key) Reset() {
	clear(k[:])
}

func (k *Key) Tenant() uint64 {
	return binary.BigEndian.Uint64(k[TenantOffset:])
}

func (k *Key) Shard() uint64 {
	return binary.BigEndian.Uint64(k[ShardOffset:])
}

func (k *Key) Field() uint64 {
	return binary.BigEndian.Uint64(k[FieldOffset:])
}

func (k *Key) Container() uint64 {
	return binary.BigEndian.Uint64(k[ContainerOffset:])
}

func (k *Key) Bytes() []byte {
	return k[:]
}

func (k *Key) SetContainer(co uint64) {
	binary.BigEndian.PutUint64(k[ContainerOffset:], co)
}

func (k *Key) SetData(tenant, shard, field, container uint64) {
	b := k[:][:0]
	AppendDataKey(tenant, shard, field, container, b)
}

func (k *Key) SetExistence(tenant, shard, field, container uint64) {
	b := k[:][:0]
	AppendExistenceKey(tenant, shard, field, container, b)
}
