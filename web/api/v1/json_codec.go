// Copyright The Prometheus Authors
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
	jsoniter.RegisterTypeEncoderFunc("promql.Vector", unsafeMarshalVectorJSON, neverEmpty)
	jsoniter.RegisterTypeEncoderFunc("promql.Matrix", unsafeMarshalMatrixJSON, neverEmpty)
	jsoniter.RegisterTypeEncoderFunc("promql.Series", unsafeMarshalSeriesJSON, neverEmpty)
	jsoniter.RegisterTypeEncoderFunc("promql.Sample", unsafeMarshalSampleJSON, neverEmpty)
	jsoniter.RegisterTypeEncoderFunc("promql.FPoint", unsafeMarshalFPointJSON, neverEmpty)
	jsoniter.RegisterTypeEncoderFunc("promql.HPoint", unsafeMarshalHPointJSON, neverEmpty)
	jsoniter.RegisterTypeEncoderFunc("exemplar.Exemplar", marshalExemplarJSON, neverEmpty)
	jsoniter.RegisterTypeEncoderFunc("labels.Labels", unsafeMarshalLabelsJSON, labelsIsEmpty)
}

// JSONCodec is a Codec that encodes API responses as JSON.
type JSONCodec struct{}

func (JSONCodec) ContentType() MIMEType {
	return MIMEType{Type: "application", SubType: "json"}
}

func (JSONCodec) CanEncode(*Response) bool {
	return true
}

func (JSONCodec) Encode(resp *Response) ([]byte, error) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	return json.Marshal(resp)
}

// unsafeMarshalSeriesJSON writes something like the following:
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
func unsafeMarshalSeriesJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	s := *((*promql.Series)(ptr))
	marshalSeriesJSON(s, stream)
}

func marshalSeriesJSON(s promql.Series, stream *jsoniter.Stream) {
	marshalSeriesJSONWithContext(s, nil, stream)
}

// marshalSeriesJSONWithContext is like marshalSeriesJSON but additionally emits
// a "context" field describing which context table entry applies to which
// samples. runs is a change-point list indexed into s.Floats (or s.Histograms
// for histogram-only series); a nil/empty runs omits the field.
func marshalSeriesJSONWithContext(s promql.Series, runs []contextRun, stream *jsoniter.Stream) {
	stream.WriteObjectStart()
	stream.WriteObjectField(`metric`)
	marshalLabelsJSON(s.Metric, stream)

	for i, p := range s.Floats {
		stream.WriteMore()
		if i == 0 {
			stream.WriteObjectField(`values`)
			stream.WriteArrayStart()
		}
		marshalFPointJSON(p, stream)
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
		marshalHPointJSON(p, stream)
	}
	if len(s.Histograms) > 0 {
		stream.WriteArrayEnd()
	}
	marshalSeriesContextJSON(runs, stream)
	stream.WriteObjectEnd()
}

// marshalSeriesContextJSON writes the "context" field for a matrix series. A
// single run starting at index 0 collapses to a bare id string; otherwise it is
// a change-point array of {"i":<index>,"id":<id|null>} entries. Nothing is
// written when there is no resolvable context.
func marshalSeriesContextJSON(runs []contextRun, stream *jsoniter.Stream) {
	if len(runs) == 0 {
		return
	}
	stream.WriteMore()
	stream.WriteObjectField(`context`)
	if len(runs) == 1 && runs[0].StartIndex == 0 && runs[0].ID != "" {
		stream.WriteString(runs[0].ID)
		return
	}
	stream.WriteArrayStart()
	for i, run := range runs {
		if i != 0 {
			stream.WriteMore()
		}
		stream.WriteObjectStart()
		stream.WriteObjectField(`i`)
		stream.WriteInt(run.StartIndex)
		stream.WriteMore()
		stream.WriteObjectField(`id`)
		if run.ID == "" {
			stream.WriteNil()
		} else {
			stream.WriteString(run.ID)
		}
		stream.WriteObjectEnd()
	}
	stream.WriteArrayEnd()
}

// In the Prometheus API we render an empty object as `[]` or similar.
func neverEmpty(unsafe.Pointer) bool {
	return false
}

// unsafeMarshalSampleJSON writes something like the following for normal value samples:
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
func unsafeMarshalSampleJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	s := *((*promql.Sample)(ptr))
	marshalSampleJSON(s, stream)
}

func marshalSampleJSON(s promql.Sample, stream *jsoniter.Stream) {
	marshalSampleJSONWithContext(s, "", stream)
}

// marshalSampleJSONWithContext is like marshalSampleJSON but additionally emits
// a "context" field with the given context table id. An empty contextID omits
// the field.
func marshalSampleJSONWithContext(s promql.Sample, contextID string, stream *jsoniter.Stream) {
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
	if contextID != "" {
		stream.WriteMore()
		stream.WriteObjectField(`context`)
		stream.WriteString(contextID)
	}
	stream.WriteObjectEnd()
}

// unsafeMarshalFPointJSON writes `[ts, "1.234"]`.
func unsafeMarshalFPointJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	p := *((*promql.FPoint)(ptr))
	marshalFPointJSON(p, stream)
}

func marshalFPointJSON(p promql.FPoint, stream *jsoniter.Stream) {
	stream.WriteArrayStart()
	jsonutil.MarshalTimestamp(p.T, stream)
	stream.WriteMore()
	jsonutil.MarshalFloat(p.F, stream)
	stream.WriteArrayEnd()
}

// unsafeMarshalHPointJSON writes `[ts, { < histogram, see jsonutil.MarshalHistogram > } ]`.
func unsafeMarshalHPointJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	p := *((*promql.HPoint)(ptr))
	marshalHPointJSON(p, stream)
}

func marshalHPointJSON(p promql.HPoint, stream *jsoniter.Stream) {
	stream.WriteArrayStart()
	jsonutil.MarshalTimestamp(p.T, stream)
	stream.WriteMore()
	jsonutil.MarshalHistogram(p.H, stream)
	stream.WriteArrayEnd()
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

// Marshal a Vector as `[sample,sample,...]` - empty Vector is `[]`.
func unsafeMarshalVectorJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	v := *((*promql.Vector)(ptr))
	stream.WriteArrayStart()
	for i, s := range v {
		marshalSampleJSON(s, stream)
		if i != len(v)-1 {
			stream.WriteMore()
		}
	}
	stream.WriteArrayEnd()
}

// Marshal a Matrix as `[series,series,...]` - empty Matrix is `[]`.
func unsafeMarshalMatrixJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	m := *((*promql.Matrix)(ptr))
	stream.WriteArrayStart()
	for i, s := range m {
		marshalSeriesJSON(s, stream)
		if i != len(m)-1 {
			stream.WriteMore()
		}
	}
	stream.WriteArrayEnd()
}
