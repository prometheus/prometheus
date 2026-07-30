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
)

func init() {
	jsoniter.RegisterTypeEncoderFunc("v1.VectorWithContext", marshalVectorWithContextJSON, neverEmpty)
	jsoniter.RegisterTypeEncoderFunc("v1.MatrixWithContext", marshalMatrixWithContextJSON, neverEmpty)
}

// marshalVectorWithContextJSON marshals a VectorWithContext as a sample array
// where each sample optionally carries a "context" id.
func marshalVectorWithContextJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	w := *((*VectorWithContext)(ptr))
	stream.WriteArrayStart()
	for i, s := range w.v {
		ref := ""
		if i < len(w.refs) {
			ref = w.refs[i]
		}
		marshalSampleJSONWithContext(s, ref, stream)
		if i != len(w.v)-1 {
			stream.WriteMore()
		}
	}
	stream.WriteArrayEnd()
}

// marshalMatrixWithContextJSON marshals a MatrixWithContext as a series array
// where each series optionally carries a "context" change-point list.
func marshalMatrixWithContextJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	w := *((*MatrixWithContext)(ptr))
	stream.WriteArrayStart()
	for i, s := range w.m {
		var runs []contextRun
		if i < len(w.runs) {
			runs = w.runs[i]
		}
		marshalSeriesJSONWithContext(s, runs, stream)
		if i != len(w.m)-1 {
			stream.WriteMore()
		}
	}
	stream.WriteArrayEnd()
}
