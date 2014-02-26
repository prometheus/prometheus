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
)

// TODO: Evaluate whether to use coding.Encoder for the key and values instead
//       raw bytes for consistency reasons.

// Iterator provides method to iterate through a leveldb.
type Iterator interface {
	Error() error
	Valid() bool

	SeekToFirst() bool
	SeekToLast() bool
	Seek(proto.Message) bool

	Next() bool
	Previous() bool

	Key(proto.Message) error
	RawValue() []byte

	Close() error

	rawKey() []byte
}
