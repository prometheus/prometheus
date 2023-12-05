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

	"github.com/oklog/ulid"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
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
func (o *OOOChunk) Insert(t int64, v float64) bool {
	// Although out-of-order samples can be out-of-order amongst themselves, we
	// are opinionated and expect them to be usually in-order meaning we could
	// try to append at the end first if the new timestamp is higher than the
	// last known timestamp.
	if len(o.samples) == 0 || t > o.samples[len(o.samples)-1].t {
		o.samples = append(o.samples, sample{t, v, nil, nil})
		return true
	}

	// Find index of sample we should replace.
	i := sort.Search(len(o.samples), func(i int) bool { return o.samples[i].t >= t })

	if i >= len(o.samples) {
		// none found. append it at the end
		o.samples = append(o.samples, sample{t, v, nil, nil})
		return true
	}

	// Duplicate sample for timestamp is not allowed.
	if o.samples[i].t == t {
		return false
	}

	// Expand length by 1 to make room. use a zero sample, we will overwrite it anyway.
	o.samples = append(o.samples, sample{})
	copy(o.samples[i+1:], o.samples[i:])
	o.samples[i] = sample{t, v, nil, nil}

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

var _ BlockReader = &OOORangeHead{}

// OOORangeHead allows querying Head out of order samples via BlockReader
// interface implementation.
type OOORangeHead struct {
	head *Head
	// mint and maxt are tracked because when a query is handled we only want
	// the timerange of the query and having preexisting pointers to the first
	// and last timestamp help with that.
	mint, maxt int64

	isoState *oooIsolationState
}

func NewOOORangeHead(head *Head, mint, maxt int64, minRef chunks.ChunkDiskMapperRef) *OOORangeHead {
	isoState := head.oooIso.TrackReadAfter(minRef)

	return &OOORangeHead{
		head:     head,
		mint:     mint,
		maxt:     maxt,
		isoState: isoState,
	}
}

func (oh *OOORangeHead) Index() (IndexReader, error) {
	return NewOOOHeadIndexReader(oh.head, oh.mint, oh.maxt, oh.isoState.minRef), nil
}

func (oh *OOORangeHead) Chunks() (ChunkReader, error) {
	return NewOOOHeadChunkReader(oh.head, oh.mint, oh.maxt, oh.isoState), nil
}

func (oh *OOORangeHead) Tombstones() (tombstones.Reader, error) {
	// As stated in the design doc https://docs.google.com/document/d/1Kppm7qL9C-BJB1j6yb6-9ObG3AbdZnFUBYPNNWwDBYM/edit?usp=sharing
	// Tombstones are not supported for out of order metrics.
	return tombstones.NewMemTombstones(), nil
}

var oooRangeHeadULID = ulid.MustParse("0000000000XXXX000RANGEHEAD")

func (oh *OOORangeHead) Meta() BlockMeta {
	return BlockMeta{
		MinTime: oh.mint,
		MaxTime: oh.maxt,
		ULID:    oooRangeHeadULID,
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
