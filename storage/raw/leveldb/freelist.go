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
	"github.com/prometheus/prometheus/utility"

	"code.google.com/p/goprotobuf/proto"
)

var buffers = newBufferList(50)

type bufferList struct {
	l utility.FreeList
}

func (l *bufferList) Get() (*proto.Buffer, bool) {
	if v, ok := l.l.Get(); ok {
		return v.(*proto.Buffer), ok
	}

	return proto.NewBuffer(make([]byte, 0, 4096)), false
}

func (l *bufferList) Give(v *proto.Buffer) bool {
	v.Reset()

	return l.l.Give(v)
}

func newBufferList(cap int) *bufferList {
	return &bufferList{
		l: utility.NewFreeList(cap),
	}
}
