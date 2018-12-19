// Copyright 2015 The Prometheus Authors
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

package api

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	otext "github.com/opentracing/opentracing-go/ext"
	"github.com/prometheus/prometheus/tracer/model"
)

type endpoint struct {
	ServiceName string `json:"serviceName"`
}

func fromIDStr(id string) (uint64, error) {
	bytes, err := hex.DecodeString(id)
	if err != nil {
		return 0, err
	}
	if len(bytes) != 8 {
		return 0, fmt.Errorf("Invalid id")
	}
	return binary.BigEndian.Uint64(bytes), nil
}

func idStr(id *uint64) string {
	if id == nil {
		return ""
	}

	var idBytes [8]byte
	binary.BigEndian.PutUint64(idBytes[:], *id)
	return hex.EncodeToString(idBytes[:])
}

func getKind(span *model.Span) string {
	kind := ""

	switch span.Tag(string(otext.SpanKind)).String_ {
	case string(otext.SpanKindRPCClientEnum):
		kind = "CLIENT"
	case string(otext.SpanKindRPCServerEnum):
		kind = "SERVER"
	}
	return kind
}

func getTags(span *model.Span) map[string]string {
	tags := make(map[string]string)

	for _, tag := range span.Tags {
		tags[tag.Key] = tag.Value().(string)
	}

	return tags
}

func spanToWire(span *model.Span) interface{} {
	kind := getKind(span)
	tags := getTags(span)
	return struct {
		TraceID        string            `json:"traceId"`
		Name           string            `json:"name"`
		ParentID       string            `json:"parentId,omitempty"`
		ID             string            `json:"id"`
		Kind           string            `json:"kind,omitempty"`
		Timestamp      int64             `json:"timestamp,omitempty"`
		Duration       int64             `json:"duration,omitempty"`
		LocalEndpoint  endpoint          `json:"localEndpoint,omitEmpty"`
		RemoteEndpoint endpoint          `json:"remoteEndpoint,omitEmpty"`
		Tags           map[string]string `json:"tags,omitempty"`
	}{
		TraceID:        idStr(&span.TraceId),
		Name:           span.OperationName,
		ParentID:       idStr(&span.ParentSpanId),
		ID:             idStr(&span.SpanId),
		Kind:           kind,
		Timestamp:      span.Start.UnixNano() / int64(time.Microsecond),
		Duration:       int64(span.End.Sub(span.Start) / time.Microsecond),
		LocalEndpoint:  endpoint{strings.ToLower(serviceName)},
		RemoteEndpoint: endpoint{strings.ToLower(serviceName)},
		Tags:           tags,
	}
}

func SpansToWire(spans []model.Span) []interface{} {
	result := make([]interface{}, 0, len(spans))
	for _, span := range spans {
		result = append(result, spanToWire(&span))
	}
	return result
}

func TracesToWire(traces []model.Trace) [][]interface{} {
	result := make([][]interface{}, 0, len(traces))
	for _, trace := range traces {
		result = append(result, SpansToWire(trace.Spans))
	}
	return result
}
