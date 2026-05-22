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

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/jsonutil"
)

// nativeHistogramVector wraps a promql.Vector so that histogram samples are
// marshaled using the native histogram JSON shape (see
// jsonutil.MarshalHistogramNative). It is used to opt in to the alternative
// format on a per-request basis without affecting the default encoders.
type nativeHistogramVector promql.Vector

// nativeHistogramMatrix wraps a promql.Matrix for the same reason as
// nativeHistogramVector.
type nativeHistogramMatrix promql.Matrix

// Type satisfies the parser.Value interface so the wrapper can stand in for
// the underlying value type in QueryData.Result.
func (nativeHistogramVector) Type() parser.ValueType { return parser.ValueTypeVector }

// Type satisfies the parser.Value interface so the wrapper can stand in for
// the underlying value type in QueryData.Result.
func (nativeHistogramMatrix) Type() parser.ValueType { return parser.ValueTypeMatrix }

// String delegates to the wrapped promql.Vector so debug output matches.
func (v nativeHistogramVector) String() string { return promql.Vector(v).String() }

// String delegates to the wrapped promql.Matrix so debug output matches.
func (m nativeHistogramMatrix) String() string { return promql.Matrix(m).String() }

// histogramFormatNative is the histogram_format query parameter value that
// selects the native histogram JSON shape.
const histogramFormatNative = "native"

// applyHistogramFormat returns v wrapped so it is encoded according to the
// requested histogram_format value, or v unchanged when the format is empty
// or not recognised. Non-histogram-bearing values are returned unchanged.
func applyHistogramFormat(v parser.Value, format string) parser.Value {
	if format != histogramFormatNative {
		return v
	}
	switch x := v.(type) {
	case promql.Vector:
		return nativeHistogramVector(x)
	case promql.Matrix:
		return nativeHistogramMatrix(x)
	default:
		return v
	}
}

func init() {
	jsoniter.RegisterTypeEncoderFunc("v1.nativeHistogramVector", unsafeMarshalNativeHistogramVectorJSON, neverEmpty)
	jsoniter.RegisterTypeEncoderFunc("v1.nativeHistogramMatrix", unsafeMarshalNativeHistogramMatrixJSON, neverEmpty)
}

func unsafeMarshalNativeHistogramVectorJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	v := *((*nativeHistogramVector)(ptr))
	stream.WriteArrayStart()
	for i, s := range v {
		marshalNativeHistogramSampleJSON(s, stream)
		if i != len(v)-1 {
			stream.WriteMore()
		}
	}
	stream.WriteArrayEnd()
}

func unsafeMarshalNativeHistogramMatrixJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	m := *((*nativeHistogramMatrix)(ptr))
	stream.WriteArrayStart()
	for i, s := range m {
		marshalNativeHistogramSeriesJSON(s, stream)
		if i != len(m)-1 {
			stream.WriteMore()
		}
	}
	stream.WriteArrayEnd()
}

func marshalNativeHistogramSampleJSON(s promql.Sample, stream *jsoniter.Stream) {
	stream.WriteObjectStart()
	stream.WriteObjectField(`metric`)
	marshalLabelsJSON(s.Metric, stream)
	stream.WriteMore()
	if s.H == nil {
		stream.WriteObjectField(`value`)
		stream.WriteArrayStart()
		jsonutil.MarshalTimestamp(s.T, stream)
		stream.WriteMore()
		jsonutil.MarshalFloat(s.F, stream)
		stream.WriteArrayEnd()
	} else {
		stream.WriteObjectField(`histogram`)
		stream.WriteArrayStart()
		jsonutil.MarshalTimestamp(s.T, stream)
		stream.WriteMore()
		jsonutil.MarshalHistogramNative(s.H, stream)
		stream.WriteArrayEnd()
	}
	stream.WriteObjectEnd()
}

func marshalNativeHistogramSeriesJSON(s promql.Series, stream *jsoniter.Stream) {
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
		stream.WriteArrayStart()
		jsonutil.MarshalTimestamp(p.T, stream)
		stream.WriteMore()
		jsonutil.MarshalHistogramNative(p.H, stream)
		stream.WriteArrayEnd()
	}
	if len(s.Histograms) > 0 {
		stream.WriteArrayEnd()
	}
	stream.WriteObjectEnd()
}
