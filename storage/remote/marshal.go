// Copyright 2013 The Prometheus Authors
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
	math_bits "math/bits"

	"github.com/prometheus/prometheus/prompb"
)

func marshalTimeseriesSliceToBuffer(timeseries []prompb.TimeSeries, dAtA []byte) (int, error) {
	i := len(dAtA)
	for iNdEx := len(timeseries) - 1; iNdEx >= 0; iNdEx-- {
		size, err := marshalTimeseriesToSizedBuffer(&timeseries[iNdEx], dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarint(dAtA, i, uint64(size))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func marshalTimeseriesToSizedBuffer(m *prompb.TimeSeries, dAtA []byte) (int, error) {
	i := len(dAtA)
	for iNdEx := len(m.Exemplars) - 1; iNdEx >= 0; iNdEx-- {
		size, err := marshalExemplarToSizedBuffer(&m.Exemplars[iNdEx], dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarint(dAtA, i, uint64(size))
		i--
		dAtA[i] = 0x1a
	}
	for iNdEx := len(m.Samples) - 1; iNdEx >= 0; iNdEx-- {
		size, err := marshalSampleToSizedBuffer(&m.Samples[iNdEx], dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarint(dAtA, i, uint64(size))
		i--
		dAtA[i] = 0x12
	}
	for iNdEx := len(m.Labels) - 1; iNdEx >= 0; iNdEx-- {
		size, err := marshalLabelToSizedBuffer(&m.Labels[iNdEx], dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarint(dAtA, i, uint64(size))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func marshalExemplarToSizedBuffer(m *prompb.Exemplar, dAtA []byte) (int, error) {
	i := len(dAtA)
	if m.Timestamp != 0 {
		i = encodeVarint(dAtA, i, uint64(m.Timestamp))
		i--
		dAtA[i] = 0x18
	}
	if m.Value != 0 {
		i -= 8
		encoding_binary.LittleEndian.PutUint64(dAtA[i:], uint64(math.Float64bits(float64(m.Value))))
		i--
		dAtA[i] = 0x11
	}
	for iNdEx := len(m.Labels) - 1; iNdEx >= 0; iNdEx-- {
		size, err := marshalLabelToSizedBuffer(&m.Labels[iNdEx], dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarint(dAtA, i, uint64(size))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func marshalSampleToSizedBuffer(m *prompb.Sample, dAtA []byte) (int, error) {
	i := len(dAtA)
	if m.Timestamp != 0 {
		i = encodeVarint(dAtA, i, uint64(m.Timestamp))
		i--
		dAtA[i] = 0x10
	}
	if m.Value != 0 {
		i -= 8
		encoding_binary.LittleEndian.PutUint64(dAtA[i:], uint64(math.Float64bits(float64(m.Value))))
		i--
		dAtA[i] = 0x9
	}
	return len(dAtA) - i, nil
}

func marshalLabelToSizedBuffer(m *prompb.Label, dAtA []byte) (int, error) {
	i := len(dAtA)
	if len(m.Value) > 0 {
		i -= len(m.Value)
		copy(dAtA[i:], m.Value)
		i = encodeVarint(dAtA, i, uint64(len(m.Value)))
		i--
		dAtA[i] = 0x12
	}
	if len(m.Name) > 0 {
		i -= len(m.Name)
		copy(dAtA[i:], m.Name)
		i = encodeVarint(dAtA, i, uint64(len(m.Name)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func sov(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}

func encodeVarint(dAtA []byte, offset int, v uint64) int {
	offset -= sov(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}

func timeseriesSliceSize(timeseries []prompb.TimeSeries) (n int) {
	for _, e := range timeseries {
		l := timeseriesSize(&e)
		n += 1 + l + sov(uint64(l))
	}
	return n
}

func timeseriesSize(m *prompb.TimeSeries) (n int) {
	if m == nil {
		return 0
	}
	var l int
	if len(m.Labels) > 0 {
		for _, e := range m.Labels {
			l = labelSize(&e)
			n += 1 + l + sov(uint64(l))
		}
	}
	if len(m.Samples) > 0 {
		for _, e := range m.Samples {
			l = sampleSize(&e)
			n += 1 + l + sov(uint64(l))
		}
	}
	if len(m.Exemplars) > 0 {
		for _, e := range m.Exemplars {
			l = exemplarSize(&e)
			n += 1 + l + sov(uint64(l))
		}
	}
	return n
}

func labelSize(m *prompb.Label) (n int) {
	if m == nil {
		return 0
	}
	l := len(m.Name)
	if l > 0 {
		n += 1 + l + sov(uint64(l))
	}
	l = len(m.Value)
	if l > 0 {
		n += 1 + l + sov(uint64(l))
	}
	return n
}

func sampleSize(m *prompb.Sample) (n int) {
	if m == nil {
		return 0
	}
	if m.Value != 0 {
		n += 9
	}
	if m.Timestamp != 0 {
		n += 1 + sov(uint64(m.Timestamp))
	}
	return n
}

func exemplarSize(m *prompb.Exemplar) (n int) {
	if m == nil {
		return 0
	}
	if len(m.Labels) > 0 {
		for _, e := range m.Labels {
			l := labelSize(&e)
			n += 1 + l + sov(uint64(l))
		}
	}
	if m.Value != 0 {
		n += 9
	}
	if m.Timestamp != 0 {
		n += 1 + sov(uint64(m.Timestamp))
	}
	return n
}
