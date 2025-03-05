// Copyright 2022 The Prometheus Authors
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
	"sort"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// OOOChunk maintains samples in time-ascending order.
// Inserts for timestamps already seen, are dropped.
// Samples are stored uncompressed to allow easy sorting.
// Perhaps we can be more efficient later.
type OOOChunk struct {
	samples []sample
}

func NewOOOChunk() *OOOChunk {
	return &OOOChunk{samples: make([]sample, 0, 4)}
}

// Insert inserts the sample such that order is maintained.
// Returns false if insert was not possible due to the same timestamp already existing.
func (o *OOOChunk) Insert(t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram) bool {
	// Although out-of-order samples can be out-of-order amongst themselves, we
	// are opinionated and expect them to be usually in-order meaning we could
	// try to append at the end first if the new timestamp is higher than the
	// last known timestamp.
	if len(o.samples) == 0 || t > o.samples[len(o.samples)-1].t {
		o.samples = append(o.samples, sample{t, v, h, fh})
		return true
	}

	// Find index of sample we should replace.
	i := sort.Search(len(o.samples), func(i int) bool { return o.samples[i].t >= t })

	if i >= len(o.samples) {
		// none found. append it at the end
		o.samples = append(o.samples, sample{t, v, h, fh})
		return true
	}

	// Duplicate sample for timestamp is not allowed.
	if o.samples[i].t == t {
		return false
	}

	// Expand length by 1 to make room. use a zero sample, we will overwrite it anyway.
	o.samples = append(o.samples, sample{})
	copy(o.samples[i+1:], o.samples[i:])
	o.samples[i] = sample{t, v, h, fh}

	return true
}

func (o *OOOChunk) NumSamples() int {
	return len(o.samples)
}

// ToEncodedChunks returns chunks with the samples in the OOOChunk.
//
//nolint:revive // unexported-return.
func (o *OOOChunk) ToEncodedChunks(mint, maxt int64) (chks []memChunk, err error) {
	if len(o.samples) == 0 {
		return nil, nil
	}
	// The most common case is that there will be a single chunk, with the same type of samples in it - this is always true for float samples.
	chks = make([]memChunk, 0, 1)
	var (
		cmint int64
		cmaxt int64
		chunk chunkenc.Chunk
		app   chunkenc.Appender
	)
	prevEncoding := chunkenc.EncNone // Yes we could call the chunk for this, but this is more efficient.
	for _, s := range o.samples {
		if s.t < mint {
			continue
		}
		if s.t > maxt {
			break
		}
		encoding := chunkenc.EncXOR
		if s.h != nil {
			encoding = chunkenc.EncHistogram
		} else if s.fh != nil {
			encoding = chunkenc.EncFloatHistogram
		}

		// prevApp is the appender for the previous sample.
		prevApp := app

		if encoding != prevEncoding { // For the first sample, this will always be true as EncNone != EncXOR | EncHistogram | EncFloatHistogram
			if prevEncoding != chunkenc.EncNone {
				chks = append(chks, memChunk{chunk, cmint, cmaxt, nil})
			}
			cmint = s.t
			switch encoding {
			case chunkenc.EncXOR:
				chunk = chunkenc.NewXORChunk()
			case chunkenc.EncHistogram:
				chunk = chunkenc.NewHistogramChunk()
			case chunkenc.EncFloatHistogram:
				chunk = chunkenc.NewFloatHistogramChunk()
			default:
				chunk = chunkenc.NewXORChunk()
			}
			app, err = chunk.Appender()
			if err != nil {
				return
			}
		}
		switch encoding {
		case chunkenc.EncXOR:
			app.Append(s.t, s.f)
		case chunkenc.EncHistogram:
			// Ignoring ok is ok, since we don't want to compare to the wrong previous appender anyway.
			prevHApp, _ := prevApp.(*chunkenc.HistogramAppender)
			var (
				newChunk chunkenc.Chunk
				recoded  bool
			)
			newChunk, recoded, app, _ = app.AppendHistogram(prevHApp, s.t, s.h, false)
			if newChunk != nil { // A new chunk was allocated.
				if !recoded {
					chks = append(chks, memChunk{chunk, cmint, cmaxt, nil})
					cmint = s.t
				}
				chunk = newChunk
			}
		case chunkenc.EncFloatHistogram:
			// Ignoring ok is ok, since we don't want to compare to the wrong previous appender anyway.
			prevHApp, _ := prevApp.(*chunkenc.FloatHistogramAppender)
			var (
				newChunk chunkenc.Chunk
				recoded  bool
			)
			newChunk, recoded, app, _ = app.AppendFloatHistogram(prevHApp, s.t, s.fh, false)
			if newChunk != nil { // A new chunk was allocated.
				if !recoded {
					chks = append(chks, memChunk{chunk, cmint, cmaxt, nil})
					cmint = s.t
				}
				chunk = newChunk
			}
		}
		cmaxt = s.t
		prevEncoding = encoding
	}
	if prevEncoding != chunkenc.EncNone {
		chks = append(chks, memChunk{chunk, cmint, cmaxt, nil})
	}
	return chks, nil
}
