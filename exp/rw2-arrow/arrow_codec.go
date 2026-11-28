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

package rw2arrow

import (
	"bytes"
	"fmt"
	"io"
	"sort"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/prometheus/prometheus/model/labels"
)

var dictStringType = &arrow.DictionaryType{
	IndexType: arrow.PrimitiveTypes.Int32,
	ValueType: arrow.BinaryTypes.String,
}

// collectSortedLabelNames returns the sorted union of all label names in series.
func collectSortedLabelNames(series []SeriesData) []string {
	seen := make(map[string]struct{}, 16)
	for _, s := range series {
		s.Labels.Range(func(l labels.Label) {
			seen[l.Name] = struct{}{}
		})
	}
	names := make([]string, 0, len(seen))
	for k := range seen {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// NewColumnarSchema builds an Arrow schema with one dictionary-encoded string
// column per label key, followed by timestamp_ms (int64) and value (float64).
func NewColumnarSchema(labelNames []string) *arrow.Schema {
	fields := make([]arrow.Field, 0, len(labelNames)+2)
	for _, name := range labelNames {
		fields = append(fields, arrow.Field{
			Name:     name,
			Type:     dictStringType,
			Nullable: true,
		})
	}
	fields = append(fields,
		arrow.Field{Name: "timestamp_ms", Type: arrow.PrimitiveTypes.Int64, Nullable: false},
		arrow.Field{Name: "value", Type: arrow.PrimitiveTypes.Float64, Nullable: false},
	)
	return arrow.NewSchema(fields, nil)
}

func appendColumnarRows(rb *array.RecordBuilder, labelNames []string, labelIdx map[string]int, series []SeriesData) {
	numCols := len(labelNames)
	dictBuilders := make([]*array.BinaryDictionaryBuilder, numCols)
	for i := range numCols {
		dictBuilders[i] = rb.Field(i).(*array.BinaryDictionaryBuilder)
	}
	tsBuilder := rb.Field(numCols).(*array.Int64Builder)
	valBuilder := rb.Field(numCols + 1).(*array.Float64Builder)

	// Reusable scratch slice to track which label columns were populated for a series.
	present := make([]string, numCols)

	for _, s := range series {
		clear(present)
		s.Labels.Range(func(l labels.Label) {
			if idx, ok := labelIdx[l.Name]; ok {
				present[idx] = l.Value
			}
		})
		for _, sp := range s.Samples {
			for i, val := range present {
				if val == "" {
					dictBuilders[i].AppendNull()
				} else {
					_ = dictBuilders[i].AppendString(val)
				}
			}
			tsBuilder.Append(sp.Timestamp)
			valBuilder.Append(sp.Value)
		}
	}
}

// EncodeArrowColumnarStateless encodes series into a stateless Arrow IPC stream
// using a fresh RecordBuilder per call.
func EncodeArrowColumnarStateless(series []SeriesData, buf *bytes.Buffer, opts ...ipc.Option) error {
	labelNames := collectSortedLabelNames(series)
	labelIdx := make(map[string]int, len(labelNames))
	for i, name := range labelNames {
		labelIdx[name] = i
	}

	schema := NewColumnarSchema(labelNames)
	rb := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer rb.Release()

	appendColumnarRows(rb, labelNames, labelIdx, series)
	rec := rb.NewRecordBatch()
	defer rec.Release()

	buf.Reset()
	wOpts := append([]ipc.Option{ipc.WithSchema(schema)}, opts...)
	w := ipc.NewWriter(buf, wOpts...)
	if err := w.Write(rec); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

// PooledArrowColumnarEncoder reuses a RecordBuilder and label index map across
// stateless IPC encodes for a fixed label schema.
type PooledArrowColumnarEncoder struct {
	labelNames []string
	labelIdx   map[string]int
	schema     *arrow.Schema
	rb         *array.RecordBuilder
}

// NewPooledArrowColumnarEncoder creates a reusable columnar Arrow encoder for
// a known set of label names.
func NewPooledArrowColumnarEncoder(labelNames []string) *PooledArrowColumnarEncoder {
	sorted := make([]string, len(labelNames))
	copy(sorted, labelNames)
	sort.Strings(sorted)

	labelIdx := make(map[string]int, len(sorted))
	for i, name := range sorted {
		labelIdx[name] = i
	}
	schema := NewColumnarSchema(sorted)
	return &PooledArrowColumnarEncoder{
		labelNames: sorted,
		labelIdx:   labelIdx,
		schema:     schema,
		rb:         array.NewRecordBuilder(memory.DefaultAllocator, schema),
	}
}

// Encode writes a stateless Arrow IPC stream into buf while reusing the
// internal RecordBuilder.
func (p *PooledArrowColumnarEncoder) Encode(series []SeriesData, buf *bytes.Buffer, opts ...ipc.Option) error {
	appendColumnarRows(p.rb, p.labelNames, p.labelIdx, series)
	rec := p.rb.NewRecordBatch()
	defer rec.Release()

	buf.Reset()
	wOpts := append([]ipc.Option{ipc.WithSchema(p.schema)}, opts...)
	w := ipc.NewWriter(buf, wOpts...)
	if err := w.Write(rec); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

// Release frees the underlying Arrow RecordBuilder memory.
func (p *PooledArrowColumnarEncoder) Release() {
	p.rb.Release()
}

// DecodeArrowColumnar decodes a stateless Arrow IPC stream (columnar schema)
// and materializes labels.Labels and sample values for every row.
func DecodeArrowColumnar(data []byte) (int, error) {
	r, err := ipc.NewReader(bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	defer r.Release()

	var sb labels.ScratchBuilder
	totalSamples := 0

	for r.Next() {
		rec := r.RecordBatch()
		schema := rec.Schema()
		numCols := int(rec.NumCols())
		tsColIdx := numCols - 2
		valColIdx := numCols - 1

		labelCols := make([]*array.Dictionary, tsColIdx)
		labelDicts := make([]*array.String, tsColIdx)
		labelNames := make([]string, tsColIdx)
		for c := range tsColIdx {
			labelCols[c] = rec.Column(c).(*array.Dictionary)
			labelDicts[c] = labelCols[c].Dictionary().(*array.String)
			labelNames[c] = schema.Field(c).Name
		}

		tsCol := rec.Column(tsColIdx).(*array.Int64)
		valCol := rec.Column(valColIdx).(*array.Float64)

		numRows := int(rec.NumRows())
		for row := range numRows {
			sb.Reset()
			for c := range tsColIdx {
				if !labelCols[c].IsNull(row) {
					idx := labelCols[c].GetValueIndex(row)
					sb.Add(labelNames[c], labelDicts[c].Value(idx))
				}
			}
			_ = sb.Labels()
			_ = tsCol.Value(row)
			_ = valCol.Value(row)
		}
		totalSamples += numRows
	}
	if err := r.Err(); err != nil && err != io.EOF {
		return totalSamples, err
	}
	return totalSamples, nil
}

// NewListSchema builds a fixed Arrow schema that supports arbitrary,
// heterogeneous label sets per series via List<Struct<name, value>>.
func NewListSchema() *arrow.Schema {
	pairType := arrow.StructOf(
		arrow.Field{Name: "name", Type: dictStringType, Nullable: false},
		arrow.Field{Name: "value", Type: dictStringType, Nullable: false},
	)
	return arrow.NewSchema([]arrow.Field{
		{Name: "labels", Type: arrow.ListOf(pairType), Nullable: false},
		{Name: "timestamp_ms", Type: arrow.PrimitiveTypes.Int64, Nullable: false},
		{Name: "value", Type: arrow.PrimitiveTypes.Float64, Nullable: false},
	}, nil)
}

var defaultListSchema = NewListSchema()

// EncodeArrowListStateless encodes series into a stateless Arrow IPC stream
// using a fixed List<Struct<name, value>> schema for arbitrary label sets.
func EncodeArrowListStateless(series []SeriesData, buf *bytes.Buffer, opts ...ipc.Option) error {
	rb := array.NewRecordBuilder(memory.DefaultAllocator, defaultListSchema)
	defer rb.Release()

	listB := rb.Field(0).(*array.ListBuilder)
	structB := listB.ValueBuilder().(*array.StructBuilder)
	nameB := structB.FieldBuilder(0).(*array.BinaryDictionaryBuilder)
	valLabelB := structB.FieldBuilder(1).(*array.BinaryDictionaryBuilder)
	tsB := rb.Field(1).(*array.Int64Builder)
	valB := rb.Field(2).(*array.Float64Builder)

	for _, s := range series {
		for _, sp := range s.Samples {
			listB.Append(true)
			s.Labels.Range(func(l labels.Label) {
				structB.Append(true)
				_ = nameB.AppendString(l.Name)
				_ = valLabelB.AppendString(l.Value)
			})
			tsB.Append(sp.Timestamp)
			valB.Append(sp.Value)
		}
	}

	rec := rb.NewRecordBatch()
	defer rec.Release()

	buf.Reset()
	wOpts := append([]ipc.Option{ipc.WithSchema(defaultListSchema)}, opts...)
	w := ipc.NewWriter(buf, wOpts...)
	if err := w.Write(rec); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

// DecodeArrowList decodes a stateless Arrow IPC stream encoded with ListSchema
// and materializes labels.Labels and sample values for every row.
func DecodeArrowList(data []byte) (int, error) {
	r, err := ipc.NewReader(bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	defer r.Release()

	var sb labels.ScratchBuilder
	totalSamples := 0

	for r.Next() {
		rec := r.RecordBatch()
		listCol := rec.Column(0).(*array.List)
		structCol := listCol.ListValues().(*array.Struct)
		nameCol := structCol.Field(0).(*array.Dictionary)
		nameDict := nameCol.Dictionary().(*array.String)
		valLabelCol := structCol.Field(1).(*array.Dictionary)
		valLabelDict := valLabelCol.Dictionary().(*array.String)
		offsets := listCol.Offsets()

		tsCol := rec.Column(1).(*array.Int64)
		valCol := rec.Column(2).(*array.Float64)

		numRows := int(rec.NumRows())
		for row := range numRows {
			sb.Reset()
			start, end := offsets[row], offsets[row+1]
			for idx := start; idx < end; idx++ {
				n := nameDict.Value(nameCol.GetValueIndex(int(idx)))
				v := valLabelDict.Value(valLabelCol.GetValueIndex(int(idx)))
				sb.Add(n, v)
			}
			_ = sb.Labels()
			_ = tsCol.Value(row)
			_ = valCol.Value(row)
		}
		totalSamples += numRows
	}
	if err := r.Err(); err != nil && err != io.EOF {
		return totalSamples, err
	}
	return totalSamples, nil
}

// StatefulArrowStream benchmarks a long-lived Arrow IPC stream where the schema
// is transmitted once and subsequent batches only emit RecordBatch frames.
type StatefulArrowStream struct {
	enc    *PooledArrowColumnarEncoder
	buf    bytes.Buffer
	writer *ipc.Writer
}

// NewStatefulArrowStream initializes a stateful Arrow IPC writer for labelNames.
func NewStatefulArrowStream(labelNames []string, opts ...ipc.Option) (*StatefulArrowStream, error) {
	enc := NewPooledArrowColumnarEncoder(labelNames)
	s := &StatefulArrowStream{enc: enc}
	wOpts := append([]ipc.Option{ipc.WithSchema(enc.schema)}, opts...)
	s.writer = ipc.NewWriter(&s.buf, wOpts...)
	return s, nil
}

// WriteBatch encodes series as a RecordBatch on the stateful stream and returns
// the newly written wire bytes for this batch.
func (s *StatefulArrowStream) WriteBatch(series []SeriesData) ([]byte, error) {
	s.buf.Reset()
	appendColumnarRows(s.enc.rb, s.enc.labelNames, s.enc.labelIdx, series)
	rec := s.enc.rb.NewRecordBatch()
	defer rec.Release()

	if err := s.writer.Write(rec); err != nil {
		return nil, fmt.Errorf("stateful arrow write: %w", err)
	}
	return s.buf.Bytes(), nil
}

// Close releases the underlying writer and pooled encoder.
func (s *StatefulArrowStream) Close() error {
	err := s.writer.Close()
	s.enc.Release()
	return err
}
