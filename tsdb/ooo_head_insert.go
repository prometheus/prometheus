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

package tsdb

import (
	"log/slog"
	"math"
	"sort"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// OOOInsertResult reports the outcome of inserting a sample into the out-of-order
// head chunk. The zero value is deliberately invalid so that a missed assignment
// cannot read as a successful insert.
type OOOInsertResult uint8

const (
	OOOInserted OOOInsertResult = iota + 1
	// OOODuplicateExact reports a drop: a sample with the same timestamp and an equal value exists.
	OOODuplicateExact
	// OOODuplicateConflict reports a drop: a sample with the same timestamp but a different value exists.
	OOODuplicateConflict
)

// insertWithResult is OOOChunk.Insert reporting whether a sample dropped for an
// already existing timestamp was an exact duplicate or a value conflict.
func (o *OOOChunk) insertWithResult(st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram) OOOInsertResult {
	// Although out-of-order samples can be out-of-order amongst themselves, we
	// are opinionated and expect them to be usually in-order meaning we could
	// try to append at the end first if the new timestamp is higher than the
	// last known timestamp.
	if len(o.samples) == 0 || t > o.samples[len(o.samples)-1].t {
		o.samples = append(o.samples, sample{st, t, v, h, fh})
		return OOOInserted
	}

	// Find index of sample we should replace.
	i := sort.Search(len(o.samples), func(i int) bool { return o.samples[i].t >= t })

	if i >= len(o.samples) {
		// none found. append it at the end
		o.samples = append(o.samples, sample{st, t, v, h, fh})
		return OOOInserted
	}

	// Overwrites of an existing timestamp are not allowed.
	if o.samples[i].t == t {
		if o.samples[i].valueEqual(v, h, fh) {
			return OOODuplicateExact
		}
		return OOODuplicateConflict
	}

	// Expand length by 1 to make room. use a zero sample, we will overwrite it anyway.
	o.samples = append(o.samples, sample{})
	copy(o.samples[i+1:], o.samples[i:])
	o.samples[i] = sample{st, t, v, h, fh}

	return OOOInserted
}

// valueEqual reports whether (v, h, fh) equals s's value; a type mismatch counts as different.
func (s sample) valueEqual(v float64, h *histogram.Histogram, fh *histogram.FloatHistogram) bool {
	switch {
	case h != nil:
		return s.h != nil && h.Equals(s.h)
	case fh != nil:
		return s.fh != nil && fh.Equals(s.fh)
	default:
		return s.h == nil && s.fh == nil && math.Float64bits(s.f) == math.Float64bits(v)
	}
}

// insertWithResult is memSeries.insert reporting the OOOInsertResult instead of
// collapsing it to a bool.
func (s *memSeries) insertWithResult(st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, o chunkOpts, oooCapMax int64, logger *slog.Logger) (result OOOInsertResult, chunkCreated bool, mmapRefs []chunks.ChunkDiskMapperRef) {
	if s.ooo == nil {
		s.ooo = &memSeriesOOOFields{}
	}
	c := s.ooo.oooHeadChunk
	if c == nil || c.chunk.NumSamples() == int(oooCapMax) {
		// Note: If no new samples come in then we rely on compaction to clean up stale in-memory OOO chunks.
		c, mmapRefs = s.cutNewOOOHeadChunk(t, o, logger)
		chunkCreated = true
	}

	result = c.chunk.insertWithResult(st, t, v, h, fh)
	if result == OOOInserted {
		if chunkCreated || t < c.minTime {
			c.minTime = t
		}
		if chunkCreated || t > c.maxTime {
			c.maxTime = t
		}
	}
	return result, chunkCreated, mmapRefs
}
