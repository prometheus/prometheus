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
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"

	"github.com/prometheus/prometheus/coding"
)

type Iterator interface {
	Error() error
	Valid() bool

	SeekToFirst() bool
	SeekToLast() bool
	Seek(proto.Message) bool

	Next() bool
	Previous() bool

	Key(proto.Message) error
	Value(proto.Message) error

	Close() error

	getIterator() iterator.Iterator
}

type iter struct {
	iter iterator.Iterator
}

func (i *iter) Error() error {
	return i.iter.Error()
}

func (i *iter) Valid() bool {
	return i.iter.Valid()
}

func (i *iter) SeekToFirst() bool {
	return i.iter.First()
}

func (i *iter) SeekToLast() bool {
	return i.iter.Last()
}

func (i *iter) Seek(k proto.Message) bool {
	return i.iter.Seek(coding.NewPBEncoder(k).MustEncode())
}

func (i *iter) Next() bool {
	return i.iter.Next()
}

func (i *iter) Previous() bool {
	return i.iter.Prev()
}

func (i *iter) Key(k proto.Message) error {
	return proto.Unmarshal(i.iter.Key(), k)
}

func (i *iter) Value(v proto.Message) error {
	return proto.Unmarshal(i.iter.Value(), v)
}

func (*iter) Close() error {
	return nil
}

func (i *iter) getIterator() iterator.Iterator {
	return i.iter
}

type snapIter struct {
	iter

	snap *leveldb.Snapshot
}

func (i *snapIter) Close() error {
	i.snap.Release()

	return nil
}
