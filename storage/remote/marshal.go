// Copyright 2021 The Prometheus Authors
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

package remote

import (
	encoding_binary "encoding/binary"
	"math"

	"github.com/prometheus/prometheus/model/labels"
)

const (
	timeseriesTag        = 0xa
	labelTag             = 0xa
	sampleTag            = 0x12
	sampleTimestampTag   = 0x10
	sampleValueTag       = 0x9
	exemplarTag          = 0x1a
	exemplarTimestampTag = 0x18
	exemplarValueTag     = 0x11
	labelValueTag        = 0x12
	labelNameTag         = 0xa
)

// Turn a set of samples and exemplars into the expected protobuf encoding for RemoteWrite.
// Similar to protoc-generated code, we expect a buffer of exactly the right size, and
// go backwards through the data. This makes it easier to insert the length before each element.
func marshalSampleOrExemplarSliceToBuffer(in []sampleOrExemplar, data []byte) (int, error) {
	i := len(data)
	for index := len(in) - 1; index >= 0; index-- {
		size, err := in[index].marshalToSizedBuffer(data[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarint(data, i, uint64(size))
		i--
		data[i] = timeseriesTag
	}
	return len(data) - i, nil
}

func (m *sampleOrExemplar) marshalToSizedBuffer(data []byte) (int, error) {
	i := len(data)
	if m.isSample {
		size, err := marshalSampleToSizedBuffer(m, data[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarint(data, i, uint64(size))
		i--
		data[i] = sampleTag
	} else { // exemplar
		size, err := marshalExemplarToSizedBuffer(m, data[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarint(data, i, uint64(size))
		i--
		data[i] = exemplarTag
	}
	size, err := marshalLabelsToSizedBuffer(m.seriesLabels, data[:i])
	if err != nil {
		return 0, err
	}
	i -= size
	return len(data) - i, nil
}

func marshalExemplarToSizedBuffer(m *sampleOrExemplar, data []byte) (int, error) {
	i := len(data)
	if m.timestamp != 0 {
		i = encodeVarint(data, i, uint64(m.timestamp))
		i--
		data[i] = exemplarTimestampTag
	}
	if m.value != 0 {
		i -= 8
		encoding_binary.LittleEndian.PutUint64(data[i:], math.Float64bits(m.value))
		i--
		data[i] = exemplarValueTag
	}
	size, err := marshalLabelsToSizedBuffer(m.exemplarLabels, data[:i])
	if err != nil {
		return 0, err
	}
	i -= size
	return len(data) - i, nil
}

func marshalSampleToSizedBuffer(m *sampleOrExemplar, data []byte) (int, error) {
	i := len(data)
	if m.timestamp != 0 {
		i = encodeVarint(data, i, uint64(m.timestamp))
		i--
		data[i] = sampleTimestampTag
	}
	if m.value != 0 {
		i -= 8
		encoding_binary.LittleEndian.PutUint64(data[i:], math.Float64bits(m.value))
		i--
		data[i] = sampleValueTag
	}
	return len(data) - i, nil
}

func marshalLabelsToSizedBuffer(lbls labels.Labels, data []byte) (int, error) {
	i := len(data)
	for index := len(lbls) - 1; index >= 0; index-- {
		size, err := marshalLabelToSizedBuffer(&lbls[index], data[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeSize(data, i, size)
		i--
		data[i] = labelTag
	}
	return len(data) - i, nil
}

func marshalLabelToSizedBuffer(m *labels.Label, data []byte) (int, error) {
	i := len(data)
	if len(m.Value) > 0 {
		i -= len(m.Value)
		copy(data[i:], m.Value)
		i = encodeSize(data, i, len(m.Value))
		i--
		data[i] = labelValueTag
	}
	if len(m.Name) > 0 {
		i -= len(m.Name)
		copy(data[i:], m.Name)
		i = encodeSize(data, i, len(m.Name))
		i--
		data[i] = labelNameTag
	}
	return len(data) - i, nil
}

func sizeVarint(x uint64) (n int) {
	// Most common case first
	if x < 1<<7 {
		return 1
	}
	if x >= 1<<56 {
		return 9
	}
	if x >= 1<<28 {
		x >>= 28
		n = 4
	}
	if x >= 1<<14 {
		x >>= 14
		n += 2
	}
	if x >= 1<<7 {
		n++
	}
	return n + 1
}

func encodeVarint(data []byte, offset int, v uint64) int {
	offset -= sizeVarint(v)
	base := offset
	for v >= 1<<7 {
		data[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	data[offset] = uint8(v)
	return base
}

// Special code for the common case that a size is less than 128
func encodeSize(data []byte, offset, v int) int {
	if v < 1<<7 {
		offset--
		data[offset] = uint8(v)
		return offset
	}
	return encodeVarint(data, offset, uint64(v))
}

func sampleOrExemplarSliceSize(data []sampleOrExemplar) (n int) {
	for _, e := range data {
		l := e.protoSize()             // for the sample or exemplar struct
		l += 1 + sizeVarint(uint64(l)) // for the timeSeries struct that wraps it
		n += 1 + l + sizeVarint(uint64(l))
	}
	return n
}

func (m *sampleOrExemplar) protoSize() (n int) {
	n += labelsSize(&m.seriesLabels)
	if !m.isSample {
		n += labelsSize(&m.exemplarLabels)
	}
	if m.value != 0 {
		n += 9
	}
	if m.timestamp != 0 {
		n += 1 + sizeVarint(uint64(m.timestamp))
	}
	return n
}

func labelsSize(lbls *labels.Labels) (n int) {
	for _, e := range *lbls {
		l := labelSize(&e)
		n += 1 + l + sizeVarint(uint64(l))
	}
	return n
}

func labelSize(m *labels.Label) (n int) {
	l := len(m.Name)
	if l > 0 {
		n += 1 + l + sizeVarint(uint64(l))
	}
	l = len(m.Value)
	if l > 0 {
		n += 1 + l + sizeVarint(uint64(l))
	}
	return n
}
