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
	"github.com/gogo/protobuf/proto"

	"github.com/prometheus/prometheus/model/labels"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

// EncodeRW2 encodes series into a Prometheus Remote Write 2.0 protobuf payload
// using SymbolTable and OptimizedMarshal.
func EncodeRW2(series []SeriesData, dst []byte) ([]byte, error) {
	st := writev2.NewSymbolTable()
	tsList := make([]writev2.TimeSeries, len(series))
	var labelBuf []uint32

	for i, s := range series {
		labelBuf = st.SymbolizeLabels(s.Labels, labelBuf)
		refs := make([]uint32, len(labelBuf))
		copy(refs, labelBuf)

		samples := make([]writev2.Sample, len(s.Samples))
		for j, sp := range s.Samples {
			samples[j] = writev2.Sample{
				Timestamp: sp.Timestamp,
				Value:     sp.Value,
			}
		}

		tsList[i] = writev2.TimeSeries{
			LabelsRefs: refs,
			Samples:    samples,
		}
	}

	req := &writev2.Request{
		Symbols:    st.Symbols(),
		Timeseries: tsList,
	}
	return req.OptimizedMarshal(dst)
}

// DecodeRW2 unmarshals a Remote Write 2.0 protobuf payload and materializes
// labels and samples for each series, returning the total sample count.
func DecodeRW2(data []byte) (int, error) {
	var req writev2.Request
	if err := proto.Unmarshal(data, &req); err != nil {
		return 0, err
	}

	var sb labels.ScratchBuilder
	totalSamples := 0
	for _, ts := range req.Timeseries {
		if _, err := ts.ToLabels(&sb, req.Symbols); err != nil {
			return 0, err
		}
		totalSamples += len(ts.Samples)
	}
	return totalSamples, nil
}
