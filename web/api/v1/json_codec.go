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
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/jsonutil"
)

func init() {
	jsoniter.RegisterTypeEncoderFunc("promql.Series", marshalSeriesJSON, marshalSeriesJSONIsEmpty)
	jsoniter.RegisterTypeEncoderFunc("promql.Sample", marshalSampleJSON, marshalSampleJSONIsEmpty)
	jsoniter.RegisterTypeEncoderFunc("promql.FPoint", marshalFPointJSON, marshalPointJSONIsEmpty)
	jsoniter.RegisterTypeEncoderFunc("promql.HPoint", marshalHPointJSON, marshalPointJSONIsEmpty)
	jsoniter.RegisterTypeEncoderFunc("exemplar.Exemplar", marshalExemplarJSON, marshalExemplarJSONEmpty)
	jsoniter.RegisterTypeEncoderFunc("labels.Labels", unsafeMarshalLabelsJSON, labelsIsEmpty)
}

// JSONCodec is a Codec that encodes API responses as JSON.
type JSONCodec struct{}

func (j JSONCodec) ContentType() MIMEType {
	return MIMEType{Type: "application", SubType: "json"}
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
//	      [ 1435781451.781, { < histogram, see jsonutil.MarshalHistogram > } ],
//	      < more histograms >
//	   ],
//	},
func marshalSeriesJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	s := *((*promql.Series)(ptr))
	stream.WriteObjectStart()
	stream.WriteObjectField(`metric`)
	marshalLabelsJSON(s.Metric, stream)

	for i, p := range s.Floats {
		stream.WriteMore()
		if i == 0 {
			stream.WriteObjectField(`values`)
			stream.WriteArrayStart()
		}
		marshalFPointJSON(unsafe.Pointer(&p), stream)
	}
	if len(s.Floats) > 0 {
		stream.WriteArrayEnd()
	}
	for i, p := range s.Histograms {
		stream.WriteMore()
		if i == 0 {
			stream.WriteObjectField(`histograms`)
			stream.WriteArrayStart()
		}
		marshalHPointJSON(unsafe.Pointer(&p), stream)
	}
	if len(s.Histograms) > 0 {
		stream.WriteArrayEnd()
	}
	stream.WriteObjectEnd()
}

func marshalSeriesJSONIsEmpty(unsafe.Pointer) bool {
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
//	   "value": [ 1435781451.781, "1.234" ]
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
//	   "histogram": [ 1435781451.781, { < histogram, see jsonutil.MarshalHistogram > } ]
//	},
func marshalSampleJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	s := *((*promql.Sample)(ptr))
	stream.WriteObjectStart()
	stream.WriteObjectField(`metric`)
	marshalLabelsJSON(s.Metric, stream)
	stream.WriteMore()
	if s.H == nil {
		stream.WriteObjectField(`value`)
	} else {
		stream.WriteObjectField(`histogram`)
	}
	stream.WriteArrayStart()
	jsonutil.MarshalTimestamp(s.T, stream)
	stream.WriteMore()
	if s.H == nil {
		jsonutil.MarshalFloat(s.F, stream)
	} else {
		jsonutil.MarshalHistogram(s.H, stream)
	}
	stream.WriteArrayEnd()
	stream.WriteObjectEnd()
}

func marshalSampleJSONIsEmpty(unsafe.Pointer) bool {
	return false
}

// marshalFPointJSON writes `[ts, "1.234"]`.
func marshalFPointJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	p := *((*promql.FPoint)(ptr))
	stream.WriteArrayStart()
	jsonutil.MarshalTimestamp(p.T, stream)
	stream.WriteMore()
	jsonutil.MarshalFloat(p.F, stream)
	stream.WriteArrayEnd()
}

// marshalHPointJSON writes `[ts, { < histogram, see jsonutil.MarshalHistogram > } ]`.
func marshalHPointJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	p := *((*promql.HPoint)(ptr))
	stream.WriteArrayStart()
	jsonutil.MarshalTimestamp(p.T, stream)
	stream.WriteMore()
	jsonutil.MarshalHistogram(p.H, stream)
	stream.WriteArrayEnd()
}

func marshalPointJSONIsEmpty(unsafe.Pointer) bool {
	return false
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
	marshalLabelsJSON(p.Labels, stream)

	// "value" key.
	stream.WriteMore()
	stream.WriteObjectField(`value`)
	jsonutil.MarshalFloat(p.Value, stream)

	// "timestamp" key.
	stream.WriteMore()
	stream.WriteObjectField(`timestamp`)
	jsonutil.MarshalTimestamp(p.Ts, stream)

	stream.WriteObjectEnd()
}

func marshalExemplarJSONEmpty(unsafe.Pointer) bool {
	return false
}

func unsafeMarshalLabelsJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	labelsPtr := (*labels.Labels)(ptr)
	marshalLabelsJSON(*labelsPtr, stream)
}

func marshalLabelsJSON(lbls labels.Labels, stream *jsoniter.Stream) {
	stream.WriteObjectStart()
	i := 0
	lbls.Range(func(v labels.Label) {
		if i != 0 {
			stream.WriteMore()
		}
		i++
		stream.WriteString(v.Name)
		stream.WriteRaw(`:`)
		stream.WriteString(v.Value)
	})
	stream.WriteObjectEnd()
}

func labelsIsEmpty(ptr unsafe.Pointer) bool {
	labelsPtr := (*labels.Labels)(ptr)
	return labelsPtr.IsEmpty()
}
