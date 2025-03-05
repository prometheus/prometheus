// Copyright 2017 The Prometheus Authors
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
	"context"
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/oklog/ulid"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/util/annotations"
)

type columnarQuerier struct {
	f          *os.File
	closed     bool
	mint, maxt int64
}

func NewColumnarQuerier(f *os.File, mint, maxt int64) (*columnarQuerier, error) {
	return &columnarQuerier{
		f:    f,
		mint: mint,
		maxt: maxt,
	}, nil
}

func (q *columnarQuerier) LabelValues(ctx context.Context, name string, _ *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	panic("implement me")
}

func (q *columnarQuerier) LabelNames(ctx context.Context, _ *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	panic("implement me")
}

func (q *columnarQuerier) Close() error {
	if q.closed {
		return errors.New("columnar querier already closed")
	}

	errs := tsdb_errors.NewMulti(
	// TODO: close parquet file? Or readers?
	)
	q.closed = true
	return errs.Err()
}

func (q *columnarQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) storage.SeriesSet {
	panic("implement me")
}

type columnarSeriesSet struct {
	blockID         ulid.ULID
	p               index.Postings
	index           IndexReader
	chunks          ChunkReader
	tombstones      tombstones.Reader
	mint, maxt      int64
	disableTrimming bool

	curr seriesData

	bufChks []chunks.Meta
	builder labels.ScratchBuilder
	err     error
}

func (b *columnarSeriesSet) At() storage.Series {
	// At can be looped over before iterating, so save the current values locally.
	return &blockSeriesEntry{
		chunks:     b.chunks,
		blockID:    b.blockID,
		seriesData: b.curr,
	}
}

func (b *columnarSeriesSet) Next() bool {
	for b.p.Next() {
		if err := b.index.Series(b.p.At(), &b.builder, &b.bufChks); err != nil {
			// Postings may be stale. Skip if no underlying series exists.
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			b.err = fmt.Errorf("get series %d: %w", b.p.At(), err)
			return false
		}

		if len(b.bufChks) == 0 {
			continue
		}

		intervals, err := b.tombstones.Get(b.p.At())
		if err != nil {
			b.err = fmt.Errorf("get tombstones: %w", err)
			return false
		}

		// NOTE:
		// * block time range is half-open: [meta.MinTime, meta.MaxTime).
		// * chunks are both closed: [chk.MinTime, chk.MaxTime].
		// * requested time ranges are closed: [req.Start, req.End].

		var trimFront, trimBack bool

		// Copy chunks as iterables are reusable.
		// Count those in range to size allocation (roughly - ignoring tombstones).
		nChks := 0
		for _, chk := range b.bufChks {
			if !(chk.MaxTime < b.mint || chk.MinTime > b.maxt) {
				nChks++
			}
		}
		chks := make([]chunks.Meta, 0, nChks)

		// Prefilter chunks and pick those which are not entirely deleted or totally outside of the requested range.
		for _, chk := range b.bufChks {
			if chk.MaxTime < b.mint {
				continue
			}
			if chk.MinTime > b.maxt {
				continue
			}
			if (tombstones.Interval{Mint: chk.MinTime, Maxt: chk.MaxTime}.IsSubrange(intervals)) {
				continue
			}
			chks = append(chks, chk)

			// If still not entirely deleted, check if trim is needed based on requested time range.
			if !b.disableTrimming {
				if chk.MinTime < b.mint {
					trimFront = true
				}
				if chk.MaxTime > b.maxt {
					trimBack = true
				}
			}
		}

		if len(chks) == 0 {
			continue
		}

		if trimFront {
			intervals = intervals.Add(tombstones.Interval{Mint: math.MinInt64, Maxt: b.mint - 1})
		}
		if trimBack {
			intervals = intervals.Add(tombstones.Interval{Mint: b.maxt + 1, Maxt: math.MaxInt64})
		}

		b.curr.labels = b.builder.Labels()
		b.curr.chks = chks
		b.curr.intervals = intervals
		return true
	}
	return false
}

func (b *columnarSeriesSet) Err() error {
	if b.err != nil {
		return b.err
	}
	return b.p.Err()
}

func (b *columnarSeriesSet) Warnings() annotations.Annotations { return nil }
