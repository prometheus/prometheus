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

package textparse

import (
	"fmt"
	"io"

	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
)

// ProtobufSeriesParser is a Protobuf parser of Timeseries set encoded as
// [prompb.QueryResult] that implements the [Parser] interface. It currently
// only supports parsing Sample values, not Histograms.
type ProtobufSeriesParser struct {
	b  []byte
	ts []*prompb.TimeSeries

	// state is marked by the entry we are processing. EntryInvalid implies
	// that we have to decode the next MetricDescriptor.
	state          Entry
	currTsIndex    int
	currTs         *prompb.TimeSeries
	currEntryIndex int
}

func NewProtobufSeriesParser(b []byte) Parser {
	return &ProtobufSeriesParser{b: b, state: EntryInvalid}
}

// Series implements [Parser.Series].
func (p *ProtobufSeriesParser) Series() ([]byte, *int64, float64) {
	switch p.state {
	case EntrySeries:
		v := p.currTs.Samples[p.currEntryIndex]
		return nil, &v.Timestamp, v.Value
	}
	panic("invalid state")
}

// Histogram implements [Parser.Histogram] (currently not implemented).
func (*ProtobufSeriesParser) Histogram() ([]byte, *int64, *histogram.Histogram, *histogram.FloatHistogram) {
	panic("not implemented")
}

// Help implements [Parser.Help] (currently not implemented).
func (*ProtobufSeriesParser) Help() ([]byte, []byte) {
	panic("not implemented")
}

// Type implements [Parser.Type] (currently not implemented).
func (*ProtobufSeriesParser) Type() ([]byte, model.MetricType) {
	panic("not implemented")
}

// Unit implements [Parser.Unit] (currently not implemented).
func (*ProtobufSeriesParser) Unit() ([]byte, []byte) {
	panic("not implemented")
}

// Comment implements [Parser.Comment] (currently not implemented).
func (*ProtobufSeriesParser) Comment() []byte {
	panic("not implemented")
}

// Labels implements [Parser.Labels].
func (p *ProtobufSeriesParser) Labels(l *labels.Labels) {
	builder := labels.NewScratchBuilder(len(p.currTs.Labels))
	*l = p.currTs.ToLabels(&builder, nil)
}

// Exemplar implements [Parser.Exemplar] (currently not implemented).
func (*ProtobufSeriesParser) Exemplar(_ *exemplar.Exemplar) bool {
	panic("not implemented")
}

// StartTimestamp implements [Parser.StartTimestamp].
func (*ProtobufSeriesParser) StartTimestamp() int64 {
	return 0
}

// Next implements [Parser.Next].
func (p *ProtobufSeriesParser) Next() (Entry, error) {
	switch p.state {
	case EntryInvalid:
		if p.ts == nil {
			var result prompb.QueryResult
			err := proto.Unmarshal(p.b, &result)
			if err != nil {
				return p.state, err
			}
			p.ts = result.Timeseries
		} else {
			p.currTsIndex++
			p.currEntryIndex = 0
		}
		if len(p.ts) <= p.currTsIndex {
			return p.state, io.EOF
		}
		p.currTs = p.ts[p.currTsIndex]
		p.state = EntrySeries
		p.currEntryIndex = -1
		return p.Next()
	case EntrySeries:
		p.currEntryIndex++
		if len(p.currTs.Samples) <= p.currEntryIndex {
			p.state = EntryInvalid
			p.currEntryIndex = -1
			return p.Next()
		}
	default:
		return EntryInvalid, fmt.Errorf("invalid protobuf parsing state: %d", p.state)
	}
	return p.state, nil
}
