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

package histogram

type SparseHistogram struct {
	Count, ZeroCount                 uint64
	Sum, ZeroThreshold               float64
	Schema                           int32
	PositiveSpans, NegativeSpans     []Span
	PositiveBuckets, NegativeBuckets []int64
}

type Span struct {
	Offset int32
	Length uint32
}

func (s SparseHistogram) Copy() SparseHistogram {
	c := s

	if s.PositiveSpans != nil {
		c.PositiveSpans = make([]Span, len(s.PositiveSpans))
		copy(c.PositiveSpans, s.PositiveSpans)
	}
	if s.NegativeSpans != nil {
		c.NegativeSpans = make([]Span, len(s.NegativeSpans))
		copy(c.NegativeSpans, s.NegativeSpans)
	}
	if s.PositiveBuckets != nil {
		c.PositiveBuckets = make([]int64, len(s.PositiveBuckets))
		copy(c.PositiveBuckets, s.PositiveBuckets)
	}
	if s.NegativeBuckets != nil {
		c.NegativeBuckets = make([]int64, len(s.NegativeBuckets))
		copy(c.NegativeBuckets, s.NegativeBuckets)
	}

	return c
}
