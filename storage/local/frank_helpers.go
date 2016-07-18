// Copyright 2016 The Prometheus Authors
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

package local

import (
	"github.com/prometheus/common/model"
)

func DecodeDoubleDeltaChunk(buf []byte) []model.SamplePair {
	c := newDoubleDeltaEncodedChunk(d1, d0, true, chunkLen)
	c.unmarshalFromBuf(buf)
	it := c.newIterator()
	samples := make([]model.SamplePair, 0, c.len())
	for it.scan() {
		samples = append(samples, it.value())
	}
	return samples
}

func EncodeDoubleDeltaChunk(samples []model.SamplePair) []byte {
	c := newDoubleDeltaEncodedChunk(d1, d0, true, chunkLen)
	for _, s := range samples {
		chunks, err := c.add(s)
		if err != nil {
			panic(err)
		}
		if len(chunks) != 1 {
			panic("too many samples for one chunk")
		}
		c = chunks[0].(*doubleDeltaEncodedChunk)
	}
	buf := make([]byte, chunkLen)
	if err := c.marshalToBuf(buf); err != nil {
		panic(err)
	}
	return buf
}
