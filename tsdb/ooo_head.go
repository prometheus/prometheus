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
	"fmt"
	"sort"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tombstones"
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

// ToChunk returns chunks with the samples in the OOOChunk.
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
		created := false
		if encoding != prevEncoding { // For the first sample, this will always be true as EncNone != EncXOR | EncHistogram | EncFloatHistogram
			if prevEncoding != chunkenc.EncNone {
				chks = append(chks, memChunk{chunk, cmint, cmaxt})
			}
			cmint = s.t
			switch encoding {
			case chunkenc.EncXOR:
				chunk = chunkenc.NewXORChunk()
			case chunkenc.EncHistogram:
				chunk = chunkenc.NewHistogramChunk()
			case chunkenc.EncFloatHistogram:
				chunk = chunkenc.NewFloatHistogramChunk()
			}
			app, err = chunk.Appender()
			if err != nil {
				return
			}
			created = true
		}
		switch encoding {
		case chunkenc.EncXOR:
			// shortcut for XOR
			app.Append(s.t, s.f)
		case chunkenc.EncHistogram:
			var okToAppend, counterReset, gauge bool
			if created {
				okToAppend = true
				switch s.h.CounterResetHint {
				case histogram.GaugeType:
					gauge = true
				case histogram.CounterReset:
					counterReset = true
				}
			} else { // Not a new chunk, must check for appendable.
				happ := app.(*chunkenc.HistogramAppender)
				var (
					pForwardInserts, nForwardInserts   []chunkenc.Insert
					pBackwardInserts, nBackwardInserts []chunkenc.Insert
					pMergedSpans, nMergedSpans         []histogram.Span
				)
				switch s.h.CounterResetHint {
				case histogram.GaugeType:
					pForwardInserts, nForwardInserts,
						pBackwardInserts, nBackwardInserts,
						pMergedSpans, nMergedSpans,
						okToAppend = happ.AppendableGauge(s.h)
				case histogram.CounterReset:
					counterReset = true
				default:
					pForwardInserts, nForwardInserts, okToAppend, counterReset = happ.Appendable(s.h)
				}

				if len(pBackwardInserts)+len(nBackwardInserts) > 0 {
					s.h.PositiveSpans = pMergedSpans
					s.h.NegativeSpans = nMergedSpans
					happ.RecodeHistogram(s.h, pBackwardInserts, nBackwardInserts)
				}
				// We have 3 cases here
				// - !okToAppend or counterReset -> We need to cut a new chunk.
				// - okToAppend but we have inserts → Existing chunk needs
				//   recoding before we can append our histogram.
				// - okToAppend and no inserts → Chunk is ready to support our histogram.
				switch {
				case !okToAppend || counterReset:
					chks = append(chks, memChunk{chunk, cmint, cmaxt})
					chunk = chunkenc.NewHistogramChunk()
					cmint = s.t
					created = true
				case len(pForwardInserts) > 0 || len(nForwardInserts) > 0:
					// New buckets have appeared. We need to recode all
					// prior histogram samples within the chunk before we
					// can process this one.
					chunk, app = happ.Recode(
						pForwardInserts, nForwardInserts,
						s.h.PositiveSpans, s.h.NegativeSpans,
					)
				}
			}

			if created {
				hc := chunk.(*chunkenc.HistogramChunk)
				header := chunkenc.UnknownCounterReset
				switch {
				case gauge:
					header = chunkenc.GaugeType
				case counterReset:
					header = chunkenc.CounterReset
				case okToAppend:
					header = chunkenc.NotCounterReset
				}
				hc.SetCounterResetHeader(header)
			}
			app.AppendHistogram(s.t, s.h)
		case chunkenc.EncFloatHistogram:
			var okToAppend, counterReset, gauge bool
			if created {
				okToAppend = true
				switch s.fh.CounterResetHint {
				case histogram.GaugeType:
					gauge = true
				case histogram.CounterReset:
					counterReset = true
				}
			} else { // Not a new chunk, must check for appendable.
				happ := app.(*chunkenc.FloatHistogramAppender)
				var (
					pForwardInserts, nForwardInserts   []chunkenc.Insert
					pBackwardInserts, nBackwardInserts []chunkenc.Insert
					pMergedSpans, nMergedSpans         []histogram.Span
				)
				switch s.fh.CounterResetHint {
				case histogram.GaugeType:
					pForwardInserts, nForwardInserts,
						pBackwardInserts, nBackwardInserts,
						pMergedSpans, nMergedSpans,
						okToAppend = happ.AppendableGauge(s.fh)
				case histogram.CounterReset:
					counterReset = true
				default:
					pForwardInserts, nForwardInserts, okToAppend, counterReset = happ.Appendable(s.fh)
				}

				if len(pBackwardInserts)+len(nBackwardInserts) > 0 {
					s.fh.PositiveSpans = pMergedSpans
					s.fh.NegativeSpans = nMergedSpans
					happ.RecodeHistogramm(s.fh, pBackwardInserts, nBackwardInserts)
				}
				// We have 3 cases here
				// - !okToAppend or counterReset -> We need to cut a new chunk.
				// - okToAppend but we have inserts → Existing chunk needs
				//   recoding before we can append our histogram.
				// - okToAppend and no inserts → Chunk is ready to support our histogram.
				switch {
				case !okToAppend || counterReset:
					chks = append(chks, memChunk{chunk, cmint, cmaxt})
					chunk = chunkenc.NewFloatHistogramChunk()
					cmint = s.t
					created = true
				case len(pForwardInserts) > 0 || len(nForwardInserts) > 0:
					// New buckets have appeared. We need to recode all
					// prior histogram samples within the chunk before we
					// can process this one.
					chunk, app = happ.Recode(
						pForwardInserts, nForwardInserts,
						s.fh.PositiveSpans, s.fh.NegativeSpans,
					)
				}
			}

			if created {
				hc := chunk.(*chunkenc.FloatHistogramChunk)
				header := chunkenc.UnknownCounterReset
				switch {
				case gauge:
					header = chunkenc.GaugeType
				case counterReset:
					header = chunkenc.CounterReset
				case okToAppend:
					header = chunkenc.NotCounterReset
				}
				hc.SetCounterResetHeader(header)
			}
			app.AppendFloatHistogram(s.t, s.fh)
		}
		cmaxt = s.t
		prevEncoding = encoding
	}
	if prevEncoding != chunkenc.EncNone {
		chks = append(chks, memChunk{chunk, cmint, cmaxt})
	}
	return chks, nil
}

var _ BlockReader = &OOORangeHead{}

// OOORangeHead allows querying Head out of order samples via BlockReader
// interface implementation.
type OOORangeHead struct {
	head *Head
	// mint and maxt are tracked because when a query is handled we only want
	// the timerange of the query and having preexisting pointers to the first
	// and last timestamp help with that.
	mint, maxt int64
}

func NewOOORangeHead(head *Head, mint, maxt int64) *OOORangeHead {
	return &OOORangeHead{
		head: head,
		mint: mint,
		maxt: maxt,
	}
}

func (oh *OOORangeHead) Index() (IndexReader, error) {
	return NewOOOHeadIndexReader(oh.head, oh.mint, oh.maxt), nil
}

func (oh *OOORangeHead) Chunks() (ChunkReader, error) {
	return NewOOOHeadChunkReader(oh.head, oh.mint, oh.maxt), nil
}

func (oh *OOORangeHead) Tombstones() (tombstones.Reader, error) {
	// As stated in the design doc https://docs.google.com/document/d/1Kppm7qL9C-BJB1j6yb6-9ObG3AbdZnFUBYPNNWwDBYM/edit?usp=sharing
	// Tombstones are not supported for out of order metrics.
	return tombstones.NewMemTombstones(), nil
}

func (oh *OOORangeHead) Meta() BlockMeta {
	var id [16]byte
	copy(id[:], "____ooo_head____")
	return BlockMeta{
		MinTime: oh.mint,
		MaxTime: oh.maxt,
		ULID:    id,
		Stats: BlockStats{
			NumSeries: oh.head.NumSeries(),
		},
	}
}

// Size returns the size taken by the Head block.
func (oh *OOORangeHead) Size() int64 {
	return oh.head.Size()
}

// String returns an human readable representation of the out of order range
// head. It's important to keep this function in order to avoid the struct dump
// when the head is stringified in errors or logs.
func (oh *OOORangeHead) String() string {
	return fmt.Sprintf("ooo range head (mint: %d, maxt: %d)", oh.MinTime(), oh.MaxTime())
}

func (oh *OOORangeHead) MinTime() int64 {
	return oh.mint
}

func (oh *OOORangeHead) MaxTime() int64 {
	return oh.maxt
}
