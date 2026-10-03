// Copyright The Prometheus Authors
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

package writev2

import (
	"slices"
)

func (m Sample) T() int64   { return m.Timestamp }
func (m Sample) V() float64 { return m.Value }

// OptimizedMarshal marshals m into dst when its capacity suffices. Otherwise
// the new buffer's capacity is the allocator's block for the message, so later
// messages within that block can reuse it.
func (m *Request) OptimizedMarshal(dst []byte) ([]byte, error) {
	siz := m.Size()
	if cap(dst) < siz {
		dst = growBuffer(siz)
	}
	n, err := m.OptimizedMarshalToSizedBuffer(dst[:siz])
	if err != nil {
		return nil, err
	}
	return dst[:n], nil
}

// Go allocates objects above 32 KiB in whole 8 KiB pages.
const (
	largeObjectSize = 32 << 10
	heapPageSize    = 8 << 10
)

// growBuffer returns an empty buffer whose capacity is the allocator's block
// for n bytes, so it never retains more than an exact allocation.
func growBuffer(n int) []byte {
	if n > largeObjectSize {
		// Unlike slices.Grow, make lets the runtime skip clearing pages that the
		// OS returns zeroed.
		return make([]byte, 0, (n+heapPageSize-1)&^(heapPageSize-1))
	}
	// Growing from nil skips append's growth factor and copying stale contents.
	return slices.Grow([]byte(nil), n)
}

// OptimizedMarshalToSizedBuffer is mostly a copy of the generated MarshalToSizedBuffer,
// but calls OptimizedMarshalToSizedBuffer on the timeseries.
func (m *Request) OptimizedMarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.XXX_unrecognized != nil {
		i -= len(m.XXX_unrecognized)
		copy(dAtA[i:], m.XXX_unrecognized)
	}
	if len(m.Timeseries) > 0 {
		for _, v := range slices.Backward(m.Timeseries) {
			{
				size, err := v.OptimizedMarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintTypes(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x2a
		}
	}
	if len(m.Symbols) > 0 {
		for _, v := range slices.Backward(m.Symbols) {
			i -= len(v)
			copy(dAtA[i:], v)
			i = encodeVarintTypes(dAtA, i, uint64(len(v)))
			i--
			dAtA[i] = 0x22
		}
	}
	return len(dAtA) - i, nil
}

// OptimizedMarshalToSizedBuffer is mostly a copy of the generated MarshalToSizedBuffer,
// but marshals m.LabelsRefs in place without extra allocations.
func (m *TimeSeries) OptimizedMarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.XXX_unrecognized != nil {
		i -= len(m.XXX_unrecognized)
		copy(dAtA[i:], m.XXX_unrecognized)
	}
	{
		size, err := m.Metadata.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintTypes(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0x2a
	if len(m.Histograms) > 0 {
		for _, v := range slices.Backward(m.Histograms) {
			{
				size, err := v.MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintTypes(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x1a
		}
	}
	if len(m.Exemplars) > 0 {
		for _, v := range slices.Backward(m.Exemplars) {
			{
				size, err := v.MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintTypes(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x22
		}
	}
	if len(m.Samples) > 0 {
		for _, v := range slices.Backward(m.Samples) {
			{
				size, err := v.MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintTypes(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x12
		}
	}

	if len(m.LabelsRefs) > 0 {
		// This is the trick: encode the varints in reverse order to make it easier
		// to do it in place. Then reverse the whole thing.
		var j10 int
		start := i
		for _, num := range m.LabelsRefs {
			for num >= 1<<7 {
				dAtA[i-1] = uint8(uint64(num)&0x7f | 0x80)
				num >>= 7
				i--
				j10++
			}
			dAtA[i-1] = uint8(num)
			i--
			j10++
		}
		slices.Reverse(dAtA[i:start])
		// --- end of trick

		i = encodeVarintTypes(dAtA, i, uint64(j10))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}
