// Copyright 2018 The Prometheus Authors
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

package remote

import (
	"fmt"
	"io"
	"net/http"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/prompb"
)

const (
	protoContentType          = "application/x-protobuf"
	protoDelimitedContentType = "application/x-protobuf-delimited"

	snappyContentEnc       = "snappy"
	snappyFramedContentEnc = "snappy-framed"
)

// Encoder types encodes QueryResult into an underlying wire protocol.
type Encoder interface {
	ContentHeaders() map[string]string
	Encode(ts []*prompb.TimeSeries, queryNum int) error
	Close() error
}

// bufferedEncoder accumulates each QueryResult on each Encode invoke in memory.
// Close invoke makes sure all is flushed to client.
// sampleLimit relates to max number of samples in all encoded QueryResults together.
type bufferedEncoder struct {
	w           io.Writer
	sampleLimit int

	resp       *prompb.ReadResponse
	numSamples int
}

func (e *bufferedEncoder) ContentHeaders() map[string]string {
	return map[string]string{
		"Content-Type":     protoContentType,
		"Content-Encoding": snappyContentEnc,
	}
}

func (e *bufferedEncoder) Encode(ts []*prompb.TimeSeries, queryNum int) error {
	if e.sampleLimit > 0 {
		for _, series := range ts {
			e.numSamples += len(series.Samples)
		}
		if e.numSamples > e.sampleLimit {
			return HTTPError{
				msg:    fmt.Sprintf("exceeded sample limit (%d)", e.sampleLimit),
				status: http.StatusBadRequest,
			}
		}
	}

	e.resp.Results[queryNum].Timeseries = append(e.resp.Results[queryNum].Timeseries, ts...)
	return nil
}

func (e *bufferedEncoder) Close() error {
	data, err := proto.Marshal(e.resp)
	if err != nil {
		return err
	}

	_, err = e.w.Write(snappy.Encode(nil, data))
	return err
}

// streamEncoder writes and flushes each QueryResult on each Encode invoke allowing streaming.
// sampleLimit relates to max number of samples per each encoded QueryResult frame.
type streamEncoder struct {
	w           *snappy.Writer
	sampleLimit int
}

func (e *streamEncoder) ContentHeaders() map[string]string {
	return map[string]string{
		"Content-Type":     protoDelimitedContentType,
		"Content-Encoding": snappyFramedContentEnc,
	}
}

func (e *streamEncoder) Encode(ts []*prompb.TimeSeries, queryNum int) error {
	var numSamples int
	if e.sampleLimit > 0 {
		for _, series := range ts {
			numSamples += len(series.Samples)
		}
		if numSamples > e.sampleLimit {
			return HTTPError{
				msg:    fmt.Sprintf("exceeded sample limit (%d)", e.sampleLimit),
				status: http.StatusBadRequest,
			}
		}
	}

	frame := &prompb.QueryResult{
		Timeseries: ts,
		QueryNum:   int64(queryNum),
	}

	_, err := pbutil.WriteDelimited(e.w, frame)
	if err != nil {
		return err
	}
	return e.w.Flush()
}

func (e *streamEncoder) Close() error { return e.w.Close() }

// NewEncoder returns a new encoder based on content type negotiation.
// Returned Encoder is also responsible for checking sample limit if non-zero.
func NewEncoder(w io.Writer, req *prompb.ReadRequest, sampleLimit int) (Encoder, error) {
	switch req.ResponseType {

	case prompb.ReadRequest_RAW:
		resp := &prompb.ReadResponse{Results: make([]*prompb.QueryResult, len(req.Queries))}
		for i := range resp.Results {
			resp.Results[i] = &prompb.QueryResult{}
		}
		return &bufferedEncoder{w: w, resp: resp, sampleLimit: sampleLimit}, nil

	case prompb.ReadRequest_RAW_STREAMED:
		return &streamEncoder{w: snappy.NewBufferedWriter(w), sampleLimit: sampleLimit}, nil

	default:
		return nil, errors.New("unknown read request response format")
	}
}
