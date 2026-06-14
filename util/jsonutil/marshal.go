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

package jsonutil

import (
	"math"
	"strconv"

	jsoniter "github.com/json-iterator/go"

	"github.com/prometheus/prometheus/model/histogram"
)

// MarshalTimestamp marshals a point timestamp using the passed jsoniter stream.
func MarshalTimestamp(t int64, stream *jsoniter.Stream) {
	// Write out the timestamp as a float divided by 1000.
	// This is ~3x faster than converting to a float.
	if t < 0 {
		stream.WriteRaw(`-`)
		t = -t
	}
	stream.WriteInt64(t / 1000)
	fraction := t % 1000
	if fraction != 0 {
		stream.WriteRaw(`.`)
		if fraction < 100 {
			stream.WriteRaw(`0`)
		}
		if fraction < 10 {
			stream.WriteRaw(`0`)
		}
		stream.WriteInt64(fraction)
	}
}

// MarshalFloat marshals a float value using the passed jsoniter stream.
func MarshalFloat(f float64, stream *jsoniter.Stream) {
	stream.WriteRaw(`"`)
	// Taken from https://github.com/json-iterator/go/blob/master/stream_float.go#L71 as a workaround
	// to https://github.com/json-iterator/go/issues/365 (jsoniter, to follow json standard, doesn't allow inf/nan).
	buf := stream.Buffer()
	abs := math.Abs(f)
	fmt := byte('f')
	// Note: Must use float32 comparisons for underlying float32 value to get precise cutoffs right.
	if abs != 0 {
		if abs < 1e-6 || abs >= 1e21 {
			fmt = 'e'
		}
	}
	buf = strconv.AppendFloat(buf, f, fmt, -1, 64)
	stream.SetBuffer(buf)
	stream.WriteRaw(`"`)
}

// MarshalHistogram marshals a histogram value using the passed jsoniter stream.
// It writes something like:
//
//	{
//	    "count": "42",
//	    "sum": "34593.34",
//	    "buckets": [
//	      [ 3, "-0.25", "0.25", "3"],
//	      [ 0, "0.25", "0.5", "12"],
//	      [ 0, "0.5", "1", "21"],
//	      [ 0, "2", "4", "6"]
//	    ]
//	}
//
// The 1st element in each bucket array determines if the boundaries are
// inclusive (AKA closed) or exclusive (AKA open):
//
//	0: lower exclusive, upper inclusive
//	1: lower inclusive, upper exclusive
//	2: both exclusive
//	3: both inclusive
//
// The 2nd and 3rd elements are the lower and upper boundary. The 4th element is
// the bucket count.
func MarshalHistogram(h *histogram.FloatHistogram, stream *jsoniter.Stream) {
	stream.WriteObjectStart()
	stream.WriteObjectField(`count`)
	MarshalFloat(h.Count, stream)
	stream.WriteMore()
	stream.WriteObjectField(`sum`)
	MarshalFloat(h.Sum, stream)

	bucketFound := false
	it := h.AllBucketIterator()
	for it.Next() {
		bucket := it.At()
		if bucket.Count == 0 {
			continue // No need to expose empty buckets in JSON.
		}
		stream.WriteMore()
		if !bucketFound {
			stream.WriteObjectField(`buckets`)
			stream.WriteArrayStart()
		}
		bucketFound = true
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
		stream.WriteArrayStart()
		stream.WriteInt(boundaries)
		stream.WriteMore()
		MarshalFloat(bucket.Lower, stream)
		stream.WriteMore()
		MarshalFloat(bucket.Upper, stream)
		stream.WriteMore()
		MarshalFloat(bucket.Count, stream)
		stream.WriteArrayEnd()
	}
	if bucketFound {
		stream.WriteArrayEnd()
	}
	stream.WriteObjectEnd()
}

// MarshalHistogramNative marshals a histogram in a format that mirrors the
// native histogram data model closely. It is intended for callers that already
// understand how buckets are organised given the schema (and, for custom-bucket
// histograms, the bucket boundaries).
//
// The shape depends on the schema. For exponential schemas it looks like:
//
//	{
//	    "count": "42",
//	    "sum":   "34593.34",
//	    "schema": 2,
//	    "zero_threshold": "0.001",
//	    "zero_count":     "12",
//	    "negative_buckets": [ [ -1, "3" ], [ 0, "2" ] ],
//	    "buckets":          [ [ 0, "5" ], [ 1, "8" ] ]
//	}
//
// "zero_threshold" is always emitted for exponential schemas because it is
// part of the schema definition. "zero_count" is only emitted when the zero
// bucket has a non-zero count. "negative_buckets" and "buckets" are each
// optional and only emitted when at least one bucket on that side has a
// non-zero count. The bucket entries are [index, count] pairs where index is
// the schema-defined bucket index.
//
// For custom-bucket histograms (schema -53) it looks like:
//
//	{
//	    "count": "6",
//	    "sum":   "7",
//	    "schema": -53,
//	    "boundaries": [ "0.5", "1", "2", "5" ],
//	    "buckets":    [ [ 1, "2" ], [ 2, "3" ], [ 3, "1" ] ]
//	}
//
// "boundaries" lists the bucket upper bounds. Each bucket entry is
// [index, count] where index is the 0-based index into "boundaries"
// identifying the bucket's upper bound.
//
// Counts are encoded as strings for NaN/Inf safety. Empty buckets are
// skipped.
func MarshalHistogramNative(h *histogram.FloatHistogram, stream *jsoniter.Stream) {
	stream.WriteObjectStart()
	stream.WriteObjectField(`count`)
	MarshalFloat(h.Count, stream)
	stream.WriteMore()
	stream.WriteObjectField(`sum`)
	MarshalFloat(h.Sum, stream)
	stream.WriteMore()
	stream.WriteObjectField(`schema`)
	stream.WriteInt32(h.Schema)

	if histogram.IsCustomBucketsSchema(h.Schema) {
		stream.WriteMore()
		stream.WriteObjectField(`boundaries`)
		stream.WriteArrayStart()
		for i, v := range h.CustomValues {
			if i != 0 {
				stream.WriteMore()
			}
			MarshalFloat(v, stream)
		}
		stream.WriteArrayEnd()

		writeIndexedBuckets(stream, `buckets`, h.PositiveBucketIterator())
		stream.WriteObjectEnd()
		return
	}

	stream.WriteMore()
	stream.WriteObjectField(`zero_threshold`)
	MarshalFloat(h.ZeroThreshold, stream)
	if h.ZeroCount != 0 {
		stream.WriteMore()
		stream.WriteObjectField(`zero_count`)
		MarshalFloat(h.ZeroCount, stream)
	}
	writeIndexedBuckets(stream, `negative_buckets`, h.NegativeBucketIterator())
	writeIndexedBuckets(stream, `buckets`, h.PositiveBucketIterator())
	stream.WriteObjectEnd()
}

// writeIndexedBuckets writes a "<field>: [ [index, count], ... ]" entry from
// the iterator, skipping empty buckets. If every bucket is empty, nothing is
// written (so the field is omitted from the parent object).
func writeIndexedBuckets(stream *jsoniter.Stream, field string, it histogram.BucketIterator[float64]) {
	started := false
	for it.Next() {
		b := it.At()
		if b.Count == 0 {
			continue
		}
		if !started {
			stream.WriteMore()
			stream.WriteObjectField(field)
			stream.WriteArrayStart()
			started = true
		} else {
			stream.WriteMore()
		}
		stream.WriteArrayStart()
		stream.WriteInt32(b.Index)
		stream.WriteMore()
		MarshalFloat(b.Count, stream)
		stream.WriteArrayEnd()
	}
	if started {
		stream.WriteArrayEnd()
	}
}
