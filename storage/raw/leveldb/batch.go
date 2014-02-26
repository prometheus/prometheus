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
	"fmt"

	"code.google.com/p/goprotobuf/proto"
	"github.com/jmhodges/levigo"
)

type batch struct {
	batch *levigo.WriteBatch
	drops uint32
	puts  uint32
}

// NewBatch returns a fully allocated batch object.
func NewBatch() *batch {
	return &batch{
		batch: levigo.NewWriteBatch(),
	}
}

func (b *batch) Drop(key proto.Message) {
	buf, _ := buffers.Get()
	defer buffers.Give(buf)

	if err := buf.Marshal(key); err != nil {
		panic(err)
	}

	b.batch.Delete(buf.Bytes())

	b.drops++
}

func (b *batch) Put(key, value proto.Message) {
	keyBuf, _ := buffers.Get()
	defer buffers.Give(keyBuf)

	if err := keyBuf.Marshal(key); err != nil {
		panic(err)
	}

	valBuf, _ := buffers.Get()
	defer buffers.Give(valBuf)

	if err := valBuf.Marshal(value); err != nil {
		panic(err)
	}

	b.batch.Put(keyBuf.Bytes(), valBuf.Bytes())

	b.puts++
}

func (b *batch) PutRaw(key proto.Message, value []byte) {
	keyBuf, _ := buffers.Get()
	defer buffers.Give(keyBuf)

	if err := keyBuf.Marshal(key); err != nil {
		panic(err)
	}

	b.batch.Put(keyBuf.Bytes(), value)

	b.puts++
}

func (b *batch) Close() {
	b.batch.Close()
}

func (b *batch) String() string {
	return fmt.Sprintf("LevelDB batch with %d puts and %d drops.", b.puts, b.drops)
}
