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
	"io"
	"log"
	"os"
	"slices"

	"github.com/parquet-go/parquet-go"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
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

// TODO: make this a columnarQuerier field
var selectColumns = []string{"instance", "job"}

func buildSchemaForLabels(labels []string, chunks bool) *parquet.Schema {
	node := parquet.Group{}
	node["x_id"] = parquet.Leaf(parquet.Int64Type)
	for _, label := range labels {
		node["l_"+label] = parquet.String()
	}
	if chunks {
		node["x_chunk"] = parquet.Leaf(parquet.ByteArrayType)
	}
	return parquet.NewSchema("metric_family", node)
}
func matches(row parquet.Row, ms []*labels.Matcher, schema *parquet.Schema) bool {
	// TODO: very inefficient
	for _, val := range row {
		colName := schema.Columns()[val.Column()][0]
		if colName == "x_chunk" {
			continue
		}
		lname := colName[2:]
		// TODO: if we have dict encoding, is there a way to make this comparison quicker?

		lvalue := string(val.ByteArray())
		for _, m := range ms {
			if m.Name == lname && m.Value != lvalue {
				return false
			}
		}
	}
	return true
}

func (q *columnarQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) storage.SeriesSet {
	columns := selectColumns
	for _, m := range ms {
		if m.Type != labels.MatchEqual {
			panic("only MatchEqual is supported")
		}
		if !slices.Contains(columns, m.Name) {
			columns = append(columns, m.Name)
		}
	}
	// TODO: i believe that including the chunks in the schema makes it so we load them into memory, but we don't want them all.
	// should we do a first pass to get the rowids, and then a second pass to get the chunks?
	// For now let's just make it work.
	schema := buildSchemaForLabels(columns, true)
	reader := parquet.NewGenericReader[any](q.f, schema)
	defer reader.Close()

	var result []parquet.Row
	buf := make([]parquet.Row, 10)

	for {
		readRows, err := reader.ReadRows(buf)
		if err != nil && err != io.EOF {
			log.Fatal(err)
		}
		if readRows == 0 {
			break
		}

		for _, row := range buf[:readRows] {
			if matches(row, ms, reader.Schema()) {
				result = append(result, row)
			}
		}

		if err == io.EOF {
			break
		}
	}

	return nil
}

type columnarSeriesSet struct {
	rows   []parquet.Row
	schema *parquet.Schema

	mint, maxt int64

	curr seriesData

	builder labels.ScratchBuilder
	err     error
}

func (b *columnarSeriesSet) At() storage.Series {
	// TODO
	return nil
}

func (b *columnarSeriesSet) Next() bool {
	// for b.p.Next() {
	// 	if err := b.index.Series(b.p.At(), &b.builder, &b.bufChks); err != nil {
	// 		// Postings may be stale. Skip if no underlying series exists.
	// 		if errors.Is(err, storage.ErrNotFound) {
	// 			continue
	// 		}
	// 		b.err = fmt.Errorf("get series %d: %w", b.p.At(), err)
	// 		return false
	// 	}

	// 	if len(b.bufChks) == 0 {
	// 		continue
	// 	}

	// 	intervals, err := b.tombstones.Get(b.p.At())
	// 	if err != nil {
	// 		b.err = fmt.Errorf("get tombstones: %w", err)
	// 		return false
	// 	}

	// 	// NOTE:
	// 	// * block time range is half-open: [meta.MinTime, meta.MaxTime).
	// 	// * chunks are both closed: [chk.MinTime, chk.MaxTime].
	// 	// * requested time ranges are closed: [req.Start, req.End].

	// 	var trimFront, trimBack bool

	// 	// Copy chunks as iterables are reusable.
	// 	// Count those in range to size allocation (roughly - ignoring tombstones).
	// 	nChks := 0
	// 	for _, chk := range b.bufChks {
	// 		if !(chk.MaxTime < b.mint || chk.MinTime > b.maxt) {
	// 			nChks++
	// 		}
	// 	}
	// 	chks := make([]chunks.Meta, 0, nChks)

	// 	// Prefilter chunks and pick those which are not entirely deleted or totally outside of the requested range.
	// 	for _, chk := range b.bufChks {
	// 		if chk.MaxTime < b.mint {
	// 			continue
	// 		}
	// 		if chk.MinTime > b.maxt {
	// 			continue
	// 		}
	// 		if (tombstones.Interval{Mint: chk.MinTime, Maxt: chk.MaxTime}.IsSubrange(intervals)) {
	// 			continue
	// 		}
	// 		chks = append(chks, chk)

	// 		// If still not entirely deleted, check if trim is needed based on requested time range.
	// 		if !b.disableTrimming {
	// 			if chk.MinTime < b.mint {
	// 				trimFront = true
	// 			}
	// 			if chk.MaxTime > b.maxt {
	// 				trimBack = true
	// 			}
	// 		}
	// 	}

	// 	if len(chks) == 0 {
	// 		continue
	// 	}

	// 	if trimFront {
	// 		intervals = intervals.Add(tombstones.Interval{Mint: math.MinInt64, Maxt: b.mint - 1})
	// 	}
	// 	if trimBack {
	// 		intervals = intervals.Add(tombstones.Interval{Mint: b.maxt + 1, Maxt: math.MaxInt64})
	// 	}

	// 	b.curr.labels = b.builder.Labels()
	// 	b.curr.chks = chks
	// 	b.curr.intervals = intervals
	// 	return true
	// }
	return false
}

func (b *columnarSeriesSet) Err() error {
	if b.err != nil {
		return b.err
	}
	return nil // TODO?
}

func (b *columnarSeriesSet) Warnings() annotations.Annotations { return nil }
