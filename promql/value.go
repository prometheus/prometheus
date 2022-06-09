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

package promql

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func (Matrix) Type() parser.ValueType { return parser.ValueTypeMatrix }
func (Vector) Type() parser.ValueType { return parser.ValueTypeVector }
func (Scalar) Type() parser.ValueType { return parser.ValueTypeScalar }
func (String) Type() parser.ValueType { return parser.ValueTypeString }

// String represents a string value.
type String struct {
	T int64
	V string
}

func (s String) String() string {
	return s.V
}

func (s String) MarshalJSON() ([]byte, error) {
	return json.Marshal([...]interface{}{float64(s.T) / 1000, s.V})
}

// Scalar is a data point that's explicitly not associated with a metric.
type Scalar struct {
	T int64
	V float64
}

func (s Scalar) String() string {
	v := strconv.FormatFloat(s.V, 'f', -1, 64)
	return fmt.Sprintf("scalar: %v @[%v]", v, s.T)
}

func (s Scalar) MarshalJSON() ([]byte, error) {
	v := strconv.FormatFloat(s.V, 'f', -1, 64)
	return json.Marshal([...]interface{}{float64(s.T) / 1000, v})
}

// Series is a stream of data points belonging to a metric.
type Series struct {
	Metric labels.Labels
	Points []Point
}

func (s Series) String() string {
	vals := make([]string, len(s.Points))
	for i, v := range s.Points {
		vals[i] = v.String()
	}
	return fmt.Sprintf("%s =>\n%s", s.Metric, strings.Join(vals, "\n"))
}

// MarshalJSON is mirrored in web/api/v1/api.go for efficiency reasons.
// This implementation is still provided for debug purposes and usage
// without jsoniter.
func (s Series) MarshalJSON() ([]byte, error) {
	// Note that this is rather inefficient because it re-creates the whole
	// series, just separated by Histogram Points and Value Points. For API
	// purposes, there is a more efficcient jsoniter implementation in
	// web/api/v1/api.go.
	series := struct {
		M labels.Labels `json:"metric"`
		V []Point       `json:"values,omitempty"`
		H []Point       `json:"histograms,omitempty"`
	}{
		M: s.Metric,
	}
	for _, p := range s.Points {
		if p.H == nil {
			series.V = append(series.V, p)
			continue
		}
		series.H = append(series.H, p)
	}
	return json.Marshal(series)
}

// Point represents a single data point for a given timestamp.
// If H is not nil, then this is a histogram point and only (T, H) is valid.
// If H is nil, then only (T, V) is valid.
type Point struct {
	T int64
	V float64
	H *histogram.FloatHistogram
}

func (p Point) String() string {
	var s string
	if p.H != nil {
		s = p.H.String()
	} else {
		s = strconv.FormatFloat(p.V, 'f', -1, 64)
	}
	return fmt.Sprintf("%s @[%v]", s, p.T)
}

// MarshalJSON implements json.Marshaler.
//
// JSON marshaling is only needed for the HTTP API. Since Point is such a
// frequently marshaled type, it gets an optimized treatment directly in
// web/api/v1/api.go. Therefore, this method is unused within Prometheus. It is
// still provided here as convenience for debugging and for other users of this
// code. Also note that the different marshaling implementations might lead to
// slightly different results in terms of formatting and rounding of the
// timestamp.
func (p Point) MarshalJSON() ([]byte, error) {
	if p.H == nil {
		v := strconv.FormatFloat(p.V, 'f', -1, 64)
		return json.Marshal([...]interface{}{float64(p.T) / 1000, v})
	}
	h := struct {
		Count   string          `json:"count"`
		Sum     string          `json:"sum"`
		Buckets [][]interface{} `json:"buckets,omitempty"`
	}{
		Count: strconv.FormatFloat(p.H.Count, 'f', -1, 64),
		Sum:   strconv.FormatFloat(p.H.Sum, 'f', -1, 64),
	}
	it := p.H.AllBucketIterator()
	for it.Next() {
		bucket := it.At()
		if bucket.Count == 0 {
			continue // No need to expose empty buckets in JSON.
		}
		boundaries := 2 // Exclusive on both sides AKA open interval.
		if bucket.LowerInclusive {
			if bucket.UpperInclusive {
				boundaries = 3 // Inclusive on both sides AKA closed interval.
			} else {
				boundaries = 1 // Inclusive only on lower end AKA right open.
			}
		} else {
			if bucket.UpperInclusive {
				boundaries = 0 // Inclusive only on upper end AKA left open.
			}
		}
		bucketToMarshal := []interface{}{
			boundaries,
			strconv.FormatFloat(bucket.Lower, 'f', -1, 64),
			strconv.FormatFloat(bucket.Upper, 'f', -1, 64),
			strconv.FormatFloat(bucket.Count, 'f', -1, 64),
		}
		h.Buckets = append(h.Buckets, bucketToMarshal)
	}
	return json.Marshal([...]interface{}{float64(p.T) / 1000, h})
}

// Sample is a single sample belonging to a metric.
type Sample struct {
	Point

	Metric labels.Labels
}

func (s Sample) String() string {
	return fmt.Sprintf("%s => %s", s.Metric, s.Point)
}

// MarshalJSON is mirrored in web/api/v1/api.go with jsoniter because Point
// wouldn't be marshaled with jsoniter in all cases otherwise.
func (s Sample) MarshalJSON() ([]byte, error) {
	if s.Point.H == nil {
		v := struct {
			M labels.Labels `json:"metric"`
			V Point         `json:"value"`
		}{
			M: s.Metric,
			V: s.Point,
		}
		return json.Marshal(v)
	}
	h := struct {
		M labels.Labels `json:"metric"`
		H Point         `json:"histogram"`
	}{
		M: s.Metric,
		H: s.Point,
	}
	return json.Marshal(h)
}

// Vector is basically only an alias for model.Samples, but the
// contract is that in a Vector, all Samples have the same timestamp.
type Vector []Sample

func (vec Vector) String() string {
	entries := make([]string, len(vec))
	for i, s := range vec {
		entries[i] = s.String()
	}
	return strings.Join(entries, "\n")
}

// ContainsSameLabelset checks if a vector has samples with the same labelset
// Such a behavior is semantically undefined
// https://github.com/prometheus/prometheus/issues/4562
func (vec Vector) ContainsSameLabelset() bool {
	l := make(map[uint64]struct{}, len(vec))
	for _, s := range vec {
		hash := s.Metric.Hash()
		if _, ok := l[hash]; ok {
			return true
		}
		l[hash] = struct{}{}
	}
	return false
}

// Matrix is a slice of Series that implements sort.Interface and
// has a String method.
type Matrix []Series

func (m Matrix) String() string {
	// TODO(fabxc): sort, or can we rely on order from the querier?
	strs := make([]string, len(m))

	for i, ss := range m {
		strs[i] = ss.String()
	}

	return strings.Join(strs, "\n")
}

// TotalSamples returns the total number of samples in the series within a matrix.
func (m Matrix) TotalSamples() int {
	numSamples := 0
	for _, series := range m {
		numSamples += len(series.Points)
	}
	return numSamples
}

func (m Matrix) Len() int           { return len(m) }
func (m Matrix) Less(i, j int) bool { return labels.Compare(m[i].Metric, m[j].Metric) < 0 }
func (m Matrix) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }

// ContainsSameLabelset checks if a matrix has samples with the same labelset.
// Such a behavior is semantically undefined.
// https://github.com/prometheus/prometheus/issues/4562
func (m Matrix) ContainsSameLabelset() bool {
	l := make(map[uint64]struct{}, len(m))
	for _, ss := range m {
		hash := ss.Metric.Hash()
		if _, ok := l[hash]; ok {
			return true
		}
		l[hash] = struct{}{}
	}
	return false
}

// Result holds the resulting value of an execution or an error
// if any occurred.
type Result struct {
	Err      error
	Value    parser.Value
	Warnings storage.Warnings
}

// Vector returns a Vector if the result value is one. An error is returned if
// the result was an error or the result value is not a Vector.
func (r *Result) Vector() (Vector, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	v, ok := r.Value.(Vector)
	if !ok {
		return nil, errors.New("query result is not a Vector")
	}
	return v, nil
}

// Matrix returns a Matrix. An error is returned if
// the result was an error or the result value is not a Matrix.
func (r *Result) Matrix() (Matrix, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	v, ok := r.Value.(Matrix)
	if !ok {
		return nil, errors.New("query result is not a range Vector")
	}
	return v, nil
}

// Scalar returns a Scalar value. An error is returned if
// the result was an error or the result value is not a Scalar.
func (r *Result) Scalar() (Scalar, error) {
	if r.Err != nil {
		return Scalar{}, r.Err
	}
	v, ok := r.Value.(Scalar)
	if !ok {
		return Scalar{}, errors.New("query result is not a Scalar")
	}
	return v, nil
}

func (r *Result) String() string {
	if r.Err != nil {
		return r.Err.Error()
	}
	if r.Value == nil {
		return ""
	}
	return r.Value.String()
}

// StorageSeries simulates promql.Series as storage.Series.
type StorageSeries struct {
	series Series
}

// NewStorageSeries returns a StorageSeries from a Series.
func NewStorageSeries(series Series) *StorageSeries {
	return &StorageSeries{
		series: series,
	}
}

func (ss *StorageSeries) Labels() labels.Labels {
	return ss.series.Metric
}

// Iterator returns a new iterator of the data of the series.
func (ss *StorageSeries) Iterator() chunkenc.Iterator {
	return newStorageSeriesIterator(ss.series)
}

type storageSeriesIterator struct {
	points []Point
	curr   int
}

func newStorageSeriesIterator(series Series) *storageSeriesIterator {
	return &storageSeriesIterator{
		points: series.Points,
		curr:   -1,
	}
}

func (ssi *storageSeriesIterator) Seek(t int64) chunkenc.ValueType {
	i := ssi.curr
	if i < 0 {
		i = 0
	}
	for ; i < len(ssi.points); i++ {
		p := ssi.points[i]
		if p.T >= t {
			ssi.curr = i
			if p.H != nil {
				return chunkenc.ValFloatHistogram
			}
			return chunkenc.ValFloat
		}
	}
	ssi.curr = len(ssi.points) - 1
	return chunkenc.ValNone
}

func (ssi *storageSeriesIterator) At() (t int64, v float64) {
	p := ssi.points[ssi.curr]
	return p.T, p.V
}

func (ssi *storageSeriesIterator) AtHistogram() (int64, *histogram.Histogram) {
	panic(errors.New("storageSeriesIterator: AtHistogram not supported"))
}

func (ssi *storageSeriesIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) {
	p := ssi.points[ssi.curr]
	return p.T, p.H
}

func (ssi *storageSeriesIterator) AtT() int64 {
	p := ssi.points[ssi.curr]
	return p.T
}

func (ssi *storageSeriesIterator) Next() chunkenc.ValueType {
	ssi.curr++
	if ssi.curr >= len(ssi.points) {
		return chunkenc.ValNone
	}
	p := ssi.points[ssi.curr]
	if p.H != nil {
		return chunkenc.ValFloatHistogram
	}
	return chunkenc.ValFloat
}

func (ssi *storageSeriesIterator) Err() error {
	return nil
}
