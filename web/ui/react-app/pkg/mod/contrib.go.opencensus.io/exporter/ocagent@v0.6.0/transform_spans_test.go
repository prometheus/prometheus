// Copyright 2018, OpenCensus Authors
//
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

package ocagent_test

import (
	"reflect"
	"testing"
	"time"

	"contrib.go.opencensus.io/exporter/ocagent"
	"go.opencensus.io/trace"
	"go.opencensus.io/trace/tracestate"

	tracepb "github.com/census-instrumentation/opencensus-proto/gen-go/trace/v1"
	"github.com/golang/protobuf/ptypes/timestamp"
)

func TestOCSpanToProtoSpan_endToEnd(t *testing.T) {
	// The goal of this test is to ensure that each
	// spanData is transformed and exported correctly!

	agent := runMockAgent(t)
	defer agent.stop()

	serviceName := "spanTranslation"
	exp, err := ocagent.NewExporter(ocagent.WithInsecure(),
		ocagent.WithAddress(agent.address),
		ocagent.WithReconnectionPeriod(50*time.Millisecond),
		ocagent.WithServiceName(serviceName))
	if err != nil {
		t.Fatalf("Failed to create a new agent exporter: %v", err)
	}
	defer exp.Stop()

	// Give the background agent connection sometime to setup.
	<-time.After(20 * time.Millisecond)

	startTime := time.Now()
	endTime := startTime.Add(10 * time.Second)
	ocTracestate, err := tracestate.New(new(tracestate.Tracestate), tracestate.Entry{Key: "foo", Value: "bar"},
		tracestate.Entry{Key: "a", Value: "b"})
	if err != nil || ocTracestate == nil {
		t.Fatalf("Failed to create ocTracestate: %v", err)
	}

	ocSpanData := &trace.SpanData{
		SpanContext: trace.SpanContext{
			TraceID:    trace.TraceID{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F},
			SpanID:     trace.SpanID{0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9, 0xF8},
			Tracestate: ocTracestate,
		},
		SpanKind:     trace.SpanKindServer,
		ParentSpanID: trace.SpanID{0xEF, 0xEE, 0xED, 0xEC, 0xEB, 0xEA, 0xE9, 0xE8},
		Name:         "End-To-End Here",
		StartTime:    startTime,
		EndTime:      endTime,
		Annotations: []trace.Annotation{
			{
				Time:    startTime,
				Message: "start",
				Attributes: map[string]interface{}{
					"timeout_ns": int64(12e9),
					"agent":      "ocagent",
					"cache_hit":  true,
					"ping_count": int(25), // Should be transformed into int64
				},
			},
			{
				Time:    endTime,
				Message: "end",
				Attributes: map[string]interface{}{
					"timeout_ns": int64(12e9),
					"agent":      "ocagent",
					"cache_hit":  false,
					"ping_count": int(25), // Should be transformed into int64
				},
			},
		},
		MessageEvents: []trace.MessageEvent{
			{Time: startTime, EventType: trace.MessageEventTypeSent, UncompressedByteSize: 1024, CompressedByteSize: 512},
			{Time: endTime, EventType: trace.MessageEventTypeRecv, UncompressedByteSize: 1024, CompressedByteSize: 1000},
		},
		Links: []trace.Link{
			{
				TraceID: trace.TraceID{0xC0, 0xC1, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9, 0xCA, 0xCB, 0xCC, 0xCD, 0xCE, 0xCF},
				SpanID:  trace.SpanID{0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7},
				Type:    trace.LinkTypeParent,
			},
			{
				TraceID: trace.TraceID{0xE0, 0xE1, 0xE2, 0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xEB, 0xEC, 0xED, 0xEE, 0xEF},
				SpanID:  trace.SpanID{0xD0, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7},
				Type:    trace.LinkTypeChild,
			},
		},
		Status: trace.Status{
			Code:    trace.StatusCodeInternal,
			Message: "This is not a drill!",
		},
		HasRemoteParent: true,
		Attributes: map[string]interface{}{
			"timeout_ns": int64(12e9),
			"agent":      "ocagent",
			"cache_hit":  true,
			"ping_count": int(25), // Should be transformed into int64
		},
	}

	// TODO: file a bug with opencensus-go because trace.Annotation.Attributes' type is map[string]interface{}
	// yet Attribute value and key cannot be easily introspected, so we can't easily test the values.

	exp.ExportSpan(ocSpanData)
	exp.Flush()
	// Also try to export a nil span and it should never make it
	exp.ExportSpan(nil)
	exp.Flush()
	// Give flush sometime to upload its data
	<-time.After(40 * time.Millisecond)
	exp.Stop()
	agent.stop()

	spans := agent.getSpans()
	if len(spans) == 0 || spans[0] == nil {
		t.Fatal("Expected the exported span")
	}

	wantProtoSpan := &tracepb.Span{
		TraceId:      []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F},
		SpanId:       []byte{0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9, 0xF8},
		ParentSpanId: []byte{0xEF, 0xEE, 0xED, 0xEC, 0xEB, 0xEA, 0xE9, 0xE8},
		Name:         &tracepb.TruncatableString{Value: "End-To-End Here"},
		Kind:         tracepb.Span_SERVER,
		StartTime:    timeToTimestamp(startTime),
		EndTime:      timeToTimestamp(endTime),
		Status: &tracepb.Status{
			Code:    13,
			Message: "This is not a drill!",
		},
		TimeEvents: &tracepb.Span_TimeEvents{
			TimeEvent: []*tracepb.Span_TimeEvent{
				// annotation
				{
					Time: timeToTimestamp(startTime),
					Value: &tracepb.Span_TimeEvent_Annotation_{
						Annotation: &tracepb.Span_TimeEvent_Annotation{
							Description: &tracepb.TruncatableString{Value: "start"},
							Attributes: &tracepb.Span_Attributes{
								AttributeMap: map[string]*tracepb.AttributeValue{
									"cache_hit":  {Value: &tracepb.AttributeValue_BoolValue{BoolValue: true}},
									"timeout_ns": {Value: &tracepb.AttributeValue_IntValue{IntValue: 12e9}},
									"ping_count": {Value: &tracepb.AttributeValue_IntValue{IntValue: 25}},
									"agent": {Value: &tracepb.AttributeValue_StringValue{
										StringValue: &tracepb.TruncatableString{Value: "ocagent"},
									}},
								},
							},
						},
					},
				},
				{
					Time: timeToTimestamp(endTime),
					Value: &tracepb.Span_TimeEvent_Annotation_{
						Annotation: &tracepb.Span_TimeEvent_Annotation{
							Description: &tracepb.TruncatableString{Value: "end"},
							Attributes: &tracepb.Span_Attributes{
								AttributeMap: map[string]*tracepb.AttributeValue{
									"cache_hit":  {Value: &tracepb.AttributeValue_BoolValue{BoolValue: false}},
									"timeout_ns": {Value: &tracepb.AttributeValue_IntValue{IntValue: 12e9}},
									"ping_count": {Value: &tracepb.AttributeValue_IntValue{IntValue: 25}},
									"agent": {Value: &tracepb.AttributeValue_StringValue{
										StringValue: &tracepb.TruncatableString{Value: "ocagent"},
									}},
								},
							},
						},
					},
				},

				// message event
				{
					Time: timeToTimestamp(startTime),
					Value: &tracepb.Span_TimeEvent_MessageEvent_{
						MessageEvent: &tracepb.Span_TimeEvent_MessageEvent{
							Type:             tracepb.Span_TimeEvent_MessageEvent_SENT,
							UncompressedSize: 1024,
							CompressedSize:   512,
						},
					},
				},
				{
					Time: timeToTimestamp(endTime),
					Value: &tracepb.Span_TimeEvent_MessageEvent_{
						MessageEvent: &tracepb.Span_TimeEvent_MessageEvent{
							Type:             tracepb.Span_TimeEvent_MessageEvent_RECEIVED,
							UncompressedSize: 1024,
							CompressedSize:   1000,
						},
					},
				},
			},
		},
		Links: &tracepb.Span_Links{
			Link: []*tracepb.Span_Link{
				{
					TraceId: []byte{0xC0, 0xC1, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9, 0xCA, 0xCB, 0xCC, 0xCD, 0xCE, 0xCF},
					SpanId:  []byte{0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7},
					Type:    tracepb.Span_Link_PARENT_LINKED_SPAN,
				},
				{
					TraceId: []byte{0xE0, 0xE1, 0xE2, 0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xEB, 0xEC, 0xED, 0xEE, 0xEF},
					SpanId:  []byte{0xD0, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7},
					Type:    tracepb.Span_Link_CHILD_LINKED_SPAN,
				},
			},
		},
		Tracestate: &tracepb.Span_Tracestate{
			Entries: []*tracepb.Span_Tracestate_Entry{
				{Key: "foo", Value: "bar"},
				{Key: "a", Value: "b"},
			},
		},
		Attributes: &tracepb.Span_Attributes{
			AttributeMap: map[string]*tracepb.AttributeValue{
				"cache_hit":  {Value: &tracepb.AttributeValue_BoolValue{BoolValue: true}},
				"timeout_ns": {Value: &tracepb.AttributeValue_IntValue{IntValue: 12e9}},
				"ping_count": {Value: &tracepb.AttributeValue_IntValue{IntValue: 25}},
				"agent": {Value: &tracepb.AttributeValue_StringValue{
					StringValue: &tracepb.TruncatableString{Value: "ocagent"},
				}},
			},
		},
	}

	if g, w := spans[0], wantProtoSpan; !reflect.DeepEqual(g, w) {
		for i := 0; i < len(w.Links.Link); i++ {
			t.Logf("#%d:: \nGot:  %#v\nWant: %#v\n\n", i, g.Links.Link[i], w.Links.Link[i])
		}
		t.Fatalf("End-to-end transformed span\n\tGot  %+v\n\tWant %+v", g, w)
	}
}

func timeToTimestamp(t time.Time) *timestamp.Timestamp {
	nanoTime := t.UnixNano()
	return &timestamp.Timestamp{
		Seconds: nanoTime / 1e9,
		Nanos:   int32(nanoTime % 1e9),
	}
}
