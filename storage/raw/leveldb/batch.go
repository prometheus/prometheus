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
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/prometheus/prometheus/coding"
)

type Batch struct {
	batch *leveldb.Batch

	drops uint32
	puts  uint32
}

func NewBatch() *Batch {
	return &Batch{
		batch: new(leveldb.Batch),
	}
}

func (b *Batch) Drop(key proto.Message) {
	b.batch.Delete(coding.NewPBEncoder(key).MustEncode())

	b.drops++
}

func (b *Batch) Put(key, value proto.Message) {
	b.batch.Put(coding.NewPBEncoder(key).MustEncode(), coding.NewPBEncoder(value).MustEncode())

	b.puts++

}

func (b *Batch) Close() {
	b.batch.Reset()
}

func (b *Batch) String() string {
	return fmt.Sprintf("LevelDB batch with %d puts and %d drops.", b.puts, b.drops)
}
