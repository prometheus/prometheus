// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"unsafe"

	jsoniter "github.com/json-iterator/go"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/jsonutil"
)

func init() {
	jsoniter.RegisterTypeEncoderFunc("promql.Series", marshalSeriesJSON, marshalSeriesJSONIsEmpty)
	jsoniter.RegisterTypeEncoderFunc("promql.Sample", marshalSampleJSON, marshalSampleJSONIsEmpty)
	jsoniter.RegisterTypeEncoderFunc("promql.Point", marshalPointJSON, marshalPointJSONIsEmpty)
	jsoniter.RegisterTypeEncoderFunc("exemplar.Exemplar", marshalExemplarJSON, marshalExemplarJSONEmpty)
}

// JSONCodec is a Codec that encodes API responses as JSON.
type JSONCodec struct{}

func (j JSONCodec) ContentType() string {
	return "application/json"
}

func (j JSONCodec) CanEncode(_ *Response) bool {
	return true
}

func (j JSONCodec) Encode(resp *Response) ([]byte, error) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	return json.Marshal(resp)
}

// marshalSeriesJSON writes something like the following:
//
//	{
//	   "metric" : {
//	      "__name__" : "up",
//	      "job" : "prometheus",
//	      "instance" : "localhost:9090"
//	   },
//	   "values": [
//	      [ 1435781451.781, "1" ],
//	      < more values>
//	   ],
//	   "histograms": [
//	      [ 1435781451.781, { < histogram, see below > } ],
//	      < more histograms >
//	   ],
//	},
func marshalSeriesJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	s := *((*promql.Series)(ptr))
	stream.WriteObjectStart()
	stream.WriteObjectField(`metric`)
	m, err := s.Metric.MarshalJSON()
	if err != nil {
		stream.Error = err
		return
	}
	stream.SetBuffer(append(stream.Buffer(), m...))

	// We make two passes through the series here: In the first marshaling
	// all value points, in the second marshaling all histogram
	// points. That's probably cheaper than just one pass in which we copy
	// out histogram Points into a newly allocated slice for separate
	// marshaling. (Could be benchmarked, though.)
	var foundValue, foundHistogram bool
	for _, p := range s.Points {
		if p.H == nil {
			stream.WriteMore()
			if !foundValue {
				stream.WriteObjectField(`values`)
				stream.WriteArrayStart()
			}
			foundValue = true
			marshalPointJSON(unsafe.Pointer(&p), stream)
		} else {
			foundHistogram = true
		}
	}
	if foundValue {
		stream.WriteArrayEnd()
	}
	if foundHistogram {
		firstHistogram := true
		for _, p := range s.Points {
			if p.H != nil {
				stream.WriteMore()
				if firstHistogram {
					stream.WriteObjectField(`histograms`)
					stream.WriteArrayStart()
				}
				firstHistogram = false
				marshalPointJSON(unsafe.Pointer(&p), stream)
			}
		}
		stream.WriteArrayEnd()
	}
	stream.WriteObjectEnd()
}

func marshalSeriesJSONIsEmpty(ptr unsafe.Pointer) bool {
	return false
}

// marshalSampleJSON writes something like the following for normal value samples:
//
//	{
//	   "metric" : {
//	      "__name__" : "up",
//	      "job" : "prometheus",
//	      "instance" : "localhost:9090"
//	   },
//	   "value": [ 1435781451.781, "1" ]
//	},
//
// For histogram samples, it writes something like this:
//
//	{
//	   "metric" : {
//	      "__name__" : "up",
//	      "job" : "prometheus",
//	      "instance" : "localhost:9090"
//	   },
//	   "histogram": [ 1435781451.781, { < histogram, see below > } ]
//	},
func marshalSampleJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	s := *((*promql.Sample)(ptr))
	stream.WriteObjectStart()
	stream.WriteObjectField(`metric`)
	m, err := s.Metric.MarshalJSON()
	if err != nil {
		stream.Error = err
		return
	}
	stream.SetBuffer(append(stream.Buffer(), m...))
	stream.WriteMore()
	if s.Point.H == nil {
		stream.WriteObjectField(`value`)
	} else {
		stream.WriteObjectField(`histogram`)
	}
	marshalPointJSON(unsafe.Pointer(&s.Point), stream)
	stream.WriteObjectEnd()
}

func marshalSampleJSONIsEmpty(ptr unsafe.Pointer) bool {
	return false
}

// marshalPointJSON writes `[ts, "val"]`.
func marshalPointJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	p := *((*promql.Point)(ptr))
	stream.WriteArrayStart()
	jsonutil.MarshalTimestamp(p.T, stream)
	stream.WriteMore()
	if p.H == nil {
		jsonutil.MarshalValue(p.V, stream)
	} else {
		marshalHistogram(p.H, stream)
	}
	stream.WriteArrayEnd()
}

func marshalPointJSONIsEmpty(ptr unsafe.Pointer) bool {
	return false
}

// marshalHistogramJSON writes something like:
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
func marshalHistogram(h *histogram.FloatHistogram, stream *jsoniter.Stream) {
	stream.WriteObjectStart()
	stream.WriteObjectField(`count`)
	jsonutil.MarshalValue(h.Count, stream)
	stream.WriteMore()
	stream.WriteObjectField(`sum`)
	jsonutil.MarshalValue(h.Sum, stream)

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
		jsonutil.MarshalValue(bucket.Lower, stream)
		stream.WriteMore()
		jsonutil.MarshalValue(bucket.Upper, stream)
		stream.WriteMore()
		jsonutil.MarshalValue(bucket.Count, stream)
		stream.WriteArrayEnd()
	}
	if bucketFound {
		stream.WriteArrayEnd()
	}
	stream.WriteObjectEnd()
}

// marshalExemplarJSON writes.
//
//	{
//	   labels: <labels>,
//	   value: "<string>",
//	   timestamp: <float>
//	}
func marshalExemplarJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	p := *((*exemplar.Exemplar)(ptr))
	stream.WriteObjectStart()

	// "labels" key.
	stream.WriteObjectField(`labels`)
	lbls, err := p.Labels.MarshalJSON()
	if err != nil {
		stream.Error = err
		return
	}
	stream.SetBuffer(append(stream.Buffer(), lbls...))

	// "value" key.
	stream.WriteMore()
	stream.WriteObjectField(`value`)
	jsonutil.MarshalValue(p.Value, stream)

	// "timestamp" key.
	stream.WriteMore()
	stream.WriteObjectField(`timestamp`)
	jsonutil.MarshalTimestamp(p.Ts, stream)

	stream.WriteObjectEnd()
}

func marshalExemplarJSONEmpty(ptr unsafe.Pointer) bool {
	return false
}
