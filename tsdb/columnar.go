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
	"io"
	"os"
	"path/filepath"

	"strings"

	"github.com/parquet-go/parquet-go"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/columnar"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/util/annotations"
)

type columnarQuerier struct {
	dir        string
	closed     bool
	mint, maxt int64

	includeLabels []string

	ix columnar.Index
}

func NewColumnarQuerier(dir string, mint, maxt int64, includeLabels []string) (*columnarQuerier, error) {
	ix, err := columnar.ReadIndex(dir)
	if err != nil {
		return nil, err
	}
	return &columnarQuerier{
		dir:           dir,
		mint:          mint,
		maxt:          maxt,
		includeLabels: includeLabels,
		ix:            ix,
	}, nil
}

func (q *columnarQuerier) LabelValues(ctx context.Context, name string, _ *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	if name == "instance" {
		return []string{"instance1", "instance2"}, nil, nil
	} else {
		return []string{"job1", "job2"}, nil, nil
	}
}

func (q *columnarQuerier) LabelNames(ctx context.Context, _ *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return []string{"instance", "job"}, nil, nil
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

func buildSchemaForLabels(lbls []string, chunks bool) *parquet.Schema {
	// TODO: use common util
	node := parquet.Group{
		"x_series_id": parquet.Encoded(parquet.Int(64), &parquet.RLEDictionary),
	}
	for _, label := range lbls {
		// The metric name is not stored in the parquet file, so we don't need to include it in the schema.
		if label == labels.MetricName {
			continue
		}
		node["l_"+label] = parquet.String()
	}
	if chunks {
		node["x_chunk"] = parquet.Leaf(parquet.ByteArrayType)
		node["x_chunk_max_time"] = parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked)
		node["x_chunk_min_time"] = parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked)
	}
	return parquet.NewSchema("metric_family", node)
}

func matches(row parquet.Row, ms []*labels.Matcher, schema *parquet.Schema) bool {
	// TODO: very inefficient
	for _, val := range row {
		colName := schema.Columns()[val.Column()][0]
		if !strings.HasPrefix(colName, "l_") {
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
	metricFamily, err := q.getMetricFamily(ms)
	if err != nil {
		return storage.ErrSeriesSet(fmt.Errorf("we only accept matchers that have EQ matcher on __name__: %w", err))
	}

	f, err := os.Open(filepath.Join(q.dir, "data", fmt.Sprintf("%s.parquet", metricFamily)))

	if err != nil {
		panic(err)
	}
	defer f.Close()

	// Get the size of the parquet file.
	fstat, err := f.Stat()
	if err != nil {
		panic(err)
	}

	// These will be the columns to filter on.
	// matchedColumns := []string{}
	// for _, m := range ms {
	// 	if m.Type != labels.MatchEqual {
	// 		panic("only MatchEqual is supported")
	// 	}
	// 	if !slices.Contains(matchedColumns, m.Name) && slices.Contains(q.ix.Metrics[metricFamily].LabelNames, m.Name) {
	// 		matchedColumns = append(matchedColumns, m.Name)
	// 	}
	// }


	// TODO: i believe that including the chunks in the schema makes it so we load them into memory, but we don't want them all.
	// should we do a first pass to get the rowids, and then a second pass to get the chunks?
	// For now let's just make it work.
	//schema := buildSchemaForLabels(q.ix.Metrics[metricFamily].LabelNames, true)
	// reader := parquet.NewGenericReader[any](f, schema)
	// defer reader.Close()

	pFile, err := parquet.OpenFile(f, fstat.Size())
	if err != nil {
		panic(err)
	}

	root := pFile.Root()

	seriesIds, _ := loadSeriesIds(root)


	cols := root.Columns()
	var col *parquet.Column
	for _, c := range cols {
		if c.Name() == "x_chunk" {
			col = c
			break
		}
	}
	if col == nil {
		panic("x_chunk not found")
	}

	pages := col.Pages()
	rawPageData := [][]byte{}
	numberOfChunks := 0
	for {
		page, err := pages.ReadPage()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			panic(err)
		}

		valReader := page.Values()
		fmt.Printf("Page has %d values, size %d\n", page.NumValues(), page.Size())
		chunks := make([]byte, page.Size())
		binaryReader, ok := valReader.(parquet.ByteArrayReader)
		if !ok {
			panic("not a ByteArrayReader")
		}
		n, err := binaryReader.ReadByteArrays(chunks)

		if err == nil || errors.Is(err, io.EOF) {
			numberOfChunks += n
			rawPageData = append(rawPageData, chunks)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			panic(err)
		}
		fmt.Printf("Read %d chunks\n", n)
	}

	metas := make([]chunks.Meta, 0, numberOfChunks)
	for _, raw := range rawPageData {
		pos := 0
		for pos < len(raw)-4 {
			fmt.Printf("Reading chunk at %d\n", pos)
			chkLen := int(raw[pos]) | int(raw[pos+1])<<8 | int(raw[pos+2])<<16 | int(raw[pos+3])<<24
			chk, erro := chunkenc.FromData(chunkenc.EncXOR, raw[pos+4:pos+4+chkLen])
			if erro != nil {
				panic(erro)
			}
			meta := chunks.Meta{
				Chunk: chk,
			}
			metas = append(metas, meta)
			pos += 4 + chkLen
		}
	}
	fmt.Printf("Read %d and have %d chunks from %d pages\n", numberOfChunks, len(metas), len(rawPageData))

	// columnIndex := make(map[string]int, len(schema.Columns()))
	// for i, col := range schema.Columns() {
	// 	columnIndex[col[0]] = i
	// }


	// var rows []parquet.Row
	// buf := make([]parquet.Row, 10)

	// for {
	// 	readRows, err := reader.ReadRows(buf)
	// 	if err != nil && err != io.EOF {
	// 		return storage.ErrSeriesSet(err)
	// 	}
	// 	if readRows == 0 {
	// 		break
	// 	}

	// 	for _, row := range buf[:readRows] {
	// 		if matches(row, ms, reader.Schema()) && chunkRangeOverlaps(
	// 			row[columnIndex["x_chunk_min_time"]].Int64(),
	// 			row[columnIndex["x_chunk_max_time"]].Int64(),
	// 			q.mint,
	// 			q.maxt,
	// 		) {
	// 			rows = append(rows, row)
	// 		}
	// 	}

	// 	if err == io.EOF {
	// 		break
	// 	}
	// }

	return &columnarSeriesSet{
		metas:    metas,
		schema:  nil,
		mint:    q.mint,
		maxt:    q.maxt,
		builder: labels.NewScratchBuilder(1),
		curr:   rowsSeries{
			//metas: metas,
            labels: labels.FromStrings(labels.MetricName, metricFamily),
		},
		seriesIds: seriesIds,
	}
}

func loadSeriesIds(root *parquet.Column) ([]int64, error) {
	cols := root.Columns()
	var col *parquet.Column
	for _, c := range cols {
		if c.Name() == "x_series_id" {
			col = c
			break
		}
	}
	if col == nil {
		panic("x_series_id not found")
	}
	if !col.Leaf() {
		panic("x_series_id is not a leaf")
	}
	pages := col.Pages()
	seriesIds := []int64{}
	for {
		page, err := pages.ReadPage()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			panic(err)
		}

		valReader := page.Values()
		fmt.Printf("Page has %d values, size %d\n", page.NumValues(), page.Size())

		// Read parquet.Value:
		nextSeriesIds := make([]parquet.Value, page.NumValues())
		_, err = valReader.ReadValues(nextSeriesIds)
		for _, v := range nextSeriesIds {
			seriesIds = append(seriesIds, v.Int64())
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			panic(err)
		}
	}
	return seriesIds, nil
}


func chunkRangeOverlaps(chunkmint, chunkmaxt, mint, maxt int64) bool {
	return !(chunkmaxt < mint || chunkmint > maxt)
}

func (*columnarQuerier) getMetricFamily(ms []*labels.Matcher) (string, error) {
	var metricFamily string
	for _, m := range ms {
		if m.Type == labels.MatchEqual && m.Name == labels.MetricName {
			metricFamily = m.Value
			break
		}
	}
	if metricFamily == "" {
		return "", errors.New("no metric name provided")
	}
	return metricFamily, nil
}

type columnarSeriesSet struct {
	metas      []chunks.Meta
	//rows        []parquet.Row
	schema      *parquet.Schema
	//columnIndex map[string]int

	mint, maxt int64

	curr   rowsSeries
	//rowIdx int

	builder labels.ScratchBuilder
	err     error
	seriesIds []int64
	seriesPos int
}

func (b *columnarSeriesSet) At() storage.Series {
	return &b.curr
}

type rowsSeries struct {
	labels      labels.Labels
	//columnIndex map[string]int
	//rows        []parquet.Row
	metas       []chunks.Meta
}

type consecutiveChunkIterators struct {
	chunkenc.Iterator
	left []chunkenc.Iterator
}

func newConsecutiveChunkIterators(its []chunkenc.Iterator) *consecutiveChunkIterators {
	return &consecutiveChunkIterators{
		Iterator: its[0],
		left:     its[1:],
	}
}

func (c *consecutiveChunkIterators) Next() chunkenc.ValueType {
	rv := c.Iterator.Next()
	if rv != chunkenc.ValNone {
		return rv
	}

	if len(c.left) == 0 {
		return rv
	}

	c.Iterator = c.left[0]
	c.left = c.left[1:]
	return c.Next()
}

// Iterator implements storage.Series.
func (r *rowsSeries) Iterator(it chunkenc.Iterator) chunkenc.Iterator {

	//pool := chunkenc.NewPool()
	// TODO: no closers
	its := make([]chunkenc.Iterator, 0, len(r.metas))
	// metas := make([]chunks.Meta, len(r.rows))
	for _, meta := range r.metas {

		//chnk, err := pool.Get(chunkenc.EncXOR, row[r.columnIndex["x_chunk"]].ByteArray())
		// if err != nil {
		// 	panic(err)
		// }
		its = append(its, meta.Chunk.Iterator(nil))
		// meta := chunks.Meta{
		// 	Ref:     chunks.ChunkRef(i),
		// 	MinTime: 0, // TODO
		// 	MaxTime: 0, // TODO
		// 	Chunk:   chnk,
		// }

	}

	return newConsecutiveChunkIterators(its)
}

// Labels implements storage.Series.
func (r *rowsSeries) Labels() labels.Labels {
	return r.labels
}

func (b *columnarSeriesSet) Next() bool {
	if b.seriesPos >= len(b.seriesIds) {
		return false
	}
	b.curr.metas = []chunks.Meta{}
	nextSeriesId := b.seriesIds[b.seriesPos]
	for b.seriesPos < len(b.seriesIds) && nextSeriesId == b.seriesIds[b.seriesPos] {
		b.curr.metas = append(b.curr.metas, b.metas[b.seriesPos])
		b.seriesPos++
	}

	// if b.columnIndex == nil {
	// 	b.columnIndex = make(map[string]int, len(b.schema.Columns()))
	// 	for i, col := range b.schema.Columns() {
	// 		b.columnIndex[col[0]] = i
	// 	}
	// }
	// if b.rowIdx == len(b.rows) {
	// 	return false
	// }
	// seriesID := b.rows[b.rowIdx][b.columnIndex["x_series_id"]].Int64()
	// from := b.rowIdx
	// to := from
	// for to < len(b.rows) && b.rows[to][b.columnIndex["x_series_id"]].Int64() == seriesID {
	// 	to++
	// }
	// b.builder.Reset()
	// for l, i := range b.columnIndex {
	// 	if strings.HasPrefix(l, "l_") {
	// 		b.builder.Add(l[2:], string(b.rows[from][i].ByteArray()))
	// 	}
	// }
	// b.curr = rowsSeries{
	// 	labels:      b.builder.Labels(),
	// 	rows:        b.rows[from:to],
	// 	columnIndex: b.columnIndex,
	// }
	// b.rowIdx = to

	return true

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
}

func (b *columnarSeriesSet) Err() error {
	if b.err != nil {
		return b.err
	}
	return nil // TODO?
}

func (b *columnarSeriesSet) Warnings() annotations.Annotations { return nil }
