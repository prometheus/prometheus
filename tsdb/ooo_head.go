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

func (o *OOOChunk) ToXOR() (*chunkenc.XORChunk, error) {
	x := chunkenc.NewXORChunk()
	app, err := x.Appender()
	if err != nil {
		return nil, err
	}
	for _, s := range o.samples {
		app.Append(s.t, s.f)
	}
	return x, nil
}

func (o *OOOChunk) ToXORBetweenTimestamps(mint, maxt int64) (*chunkenc.XORChunk, error) {
	x := chunkenc.NewXORChunk()
	app, err := x.Appender()
	if err != nil {
		return nil, err
	}
	for _, s := range o.samples {
		if s.t < mint {
			continue
		}
		if s.t > maxt {
			break
		}
		app.Append(s.t, s.f)
	}
	return x, nil
}

func (o *OOOChunk) ToHistogram() ([]*chunkenc.HistogramChunk, []int64, []int64, error) {
	ch, err := chunkenc.NewEmptyChunk(chunkenc.EncHistogram)
	if err != nil {
		return nil, nil, nil, err
	}
	chunkCreated := true
	chunks := []*chunkenc.HistogramChunk{}
	minTimes := []int64{}
	minT := int64(0)
	maxTimes := []int64{}
	maxT := int64(0)
	appender, err := ch.Appender()
	if err != nil {
		return nil, nil, nil, err
	}
	app := appender.(*chunkenc.HistogramAppender)
	var (
		pForwardInserts, nForwardInserts   []chunkenc.Insert
		pBackwardInserts, nBackwardInserts []chunkenc.Insert
		pMergedSpans, nMergedSpans         []histogram.Span
		okToAppend, counterReset, gauge    bool
	)
	for _, s := range o.samples {
		switch {
		case s.h == nil:
			return nil, nil, nil, fmt.Errorf("mixing histograms and non-histograms in OOO is not allowed")
		case s.h.CounterResetHint == histogram.GaugeType:
			pForwardInserts, nForwardInserts,
				pBackwardInserts, nBackwardInserts,
				pMergedSpans, nMergedSpans,
				okToAppend = app.AppendableGauge(s.h)
			app.AppendHistogram(s.t, s.h)
		case s.h.CounterResetHint == histogram.CounterReset:
			counterReset = true
		default:
			pForwardInserts, nForwardInserts, okToAppend, counterReset = app.Appendable(s.h)
		}

		if !chunkCreated {
			if len(pBackwardInserts)+len(nBackwardInserts) > 0 {
				s.h.PositiveSpans = pMergedSpans
				s.h.NegativeSpans = nMergedSpans
				app.RecodeHistogram(s.h, pBackwardInserts, nBackwardInserts)
			}
			// We have 3 cases here
			// - !okToAppend or counterReset -> We need to cut a new chunk.
			// - okToAppend but we have inserts → Existing chunk needs
			//   recoding before we can append our histogram.
			// - okToAppend and no inserts → Chunk is ready to support our histogram.
			switch {
			case !okToAppend || counterReset:
				chunks = append(chunks, ch.(*chunkenc.HistogramChunk))
				minTimes = append(minTimes, minT)
				maxTimes = append(maxTimes, maxT)
				ch = chunkenc.NewHistogramChunk()
				minT = s.t
				maxT = s.t
				chunkCreated = true
			case len(pForwardInserts) > 0 || len(nForwardInserts) > 0:
				// New buckets have appeared. We need to recode all
				// prior histogram samples within the chunk before we
				// can process this one.
				var newApp chunkenc.Appender
				ch, newApp = app.Recode(
					pForwardInserts, nForwardInserts,
					s.h.PositiveSpans, s.h.NegativeSpans,
				)
				app = newApp.(*chunkenc.HistogramAppender)
			}
		}

		if chunkCreated {
			minT = s.t
			maxT = s.t
			hc := ch.(*chunkenc.HistogramChunk)
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
	}
	chunks = append(chunks, ch.(*chunkenc.HistogramChunk))
	minTimes = append(minTimes, minT)
	maxTimes = append(maxTimes, maxT)
	return chunks, minTimes, maxTimes, nil
}

func (o *OOOChunk) ToHistogramBetweenTimestamps(mint, maxt int64) ([]*chunkenc.HistogramChunk, error) {
	ch := chunkenc.NewHistogramChunk()
	app, err := ch.Appender()
	if err != nil {
		return nil, err
	}
	for _, s := range o.samples {
		if s.t < mint {
			continue
		}
		if s.t > maxt {
			break
		}
		app.AppendHistogram(s.t, s.h)
	}
	return nil, nil // TODO Fix
}

func (o *OOOChunk) ToFloatHistogram() ([]*chunkenc.FloatHistogramChunk, error) {
	ch := chunkenc.NewFloatHistogramChunk()
	app, err := ch.Appender()
	if err != nil {
		return nil, err
	}
	for _, s := range o.samples {
		app.AppendFloatHistogram(s.t, s.fh)
	}
	return nil, nil // TODO Ffix
}

func (o *OOOChunk) ToFloatHistogramBetweenTimestamps(mint, maxt int64) ([]*chunkenc.FloatHistogramChunk, error) {
	ch := chunkenc.NewFloatHistogramChunk()
	app, err := ch.Appender()
	if err != nil {
		return nil, err
	}
	for _, s := range o.samples {
		if s.t < mint {
			continue
		}
		if s.t > maxt {
			break
		}
		app.AppendFloatHistogram(s.t, s.fh)
	}
	return nil, nil // TODO Fix
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
