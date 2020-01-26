// Copyright 2016 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Tests that require access to unexported names of the logging package.

package logging

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/golang/protobuf/proto"
	durpb "github.com/golang/protobuf/ptypes/duration"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/api/logging/v2"
	"google.golang.org/api/support/bundler"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	logtypepb "google.golang.org/genproto/googleapis/logging/type"
)

func TestLoggerCreation(t *testing.T) {
	const logID = "testing"
	c := &Client{parent: "projects/PROJECT_ID"}
	customResource := &mrpb.MonitoredResource{
		Type: "global",
		Labels: map[string]string{
			"project_id": "ANOTHER_PROJECT",
		},
	}
	defaultBundler := &bundler.Bundler{
		DelayThreshold:       DefaultDelayThreshold,
		BundleCountThreshold: DefaultEntryCountThreshold,
		BundleByteThreshold:  DefaultEntryByteThreshold,
		BundleByteLimit:      0,
		BufferedByteLimit:    DefaultBufferedByteLimit,
	}
	for _, test := range []struct {
		options         []LoggerOption
		wantLogger      *Logger
		defaultResource bool
		wantBundler     *bundler.Bundler
	}{
		{
			options:         nil,
			wantLogger:      &Logger{},
			defaultResource: true,
			wantBundler:     defaultBundler,
		},
		{
			options: []LoggerOption{
				CommonResource(nil),
				CommonLabels(map[string]string{"a": "1"}),
			},
			wantLogger: &Logger{
				commonResource: nil,
				commonLabels:   map[string]string{"a": "1"},
			},
			wantBundler: defaultBundler,
		},
		{
			options:     []LoggerOption{CommonResource(customResource)},
			wantLogger:  &Logger{commonResource: customResource},
			wantBundler: defaultBundler,
		},
		{
			options: []LoggerOption{
				DelayThreshold(time.Minute),
				EntryCountThreshold(99),
				EntryByteThreshold(17),
				EntryByteLimit(18),
				BufferedByteLimit(19),
			},
			wantLogger:      &Logger{},
			defaultResource: true,
			wantBundler: &bundler.Bundler{
				DelayThreshold:       time.Minute,
				BundleCountThreshold: 99,
				BundleByteThreshold:  17,
				BundleByteLimit:      18,
				BufferedByteLimit:    19,
			},
		},
	} {
		gotLogger := c.Logger(logID, test.options...)
		if got, want := gotLogger.commonResource, test.wantLogger.commonResource; !test.defaultResource && !proto.Equal(got, want) {
			t.Errorf("%v: resource: got %v, want %v", test.options, got, want)
		}
		if got, want := gotLogger.commonLabels, test.wantLogger.commonLabels; !testutil.Equal(got, want) {
			t.Errorf("%v: commonLabels: got %v, want %v", test.options, got, want)
		}
		if got, want := gotLogger.bundler.DelayThreshold, test.wantBundler.DelayThreshold; got != want {
			t.Errorf("%v: DelayThreshold: got %v, want %v", test.options, got, want)
		}
		if got, want := gotLogger.bundler.BundleCountThreshold, test.wantBundler.BundleCountThreshold; got != want {
			t.Errorf("%v: BundleCountThreshold: got %v, want %v", test.options, got, want)
		}
		if got, want := gotLogger.bundler.BundleByteThreshold, test.wantBundler.BundleByteThreshold; got != want {
			t.Errorf("%v: BundleByteThreshold: got %v, want %v", test.options, got, want)
		}
		if got, want := gotLogger.bundler.BundleByteLimit, test.wantBundler.BundleByteLimit; got != want {
			t.Errorf("%v: BundleByteLimit: got %v, want %v", test.options, got, want)
		}
		if got, want := gotLogger.bundler.BufferedByteLimit, test.wantBundler.BufferedByteLimit; got != want {
			t.Errorf("%v: BufferedByteLimit: got %v, want %v", test.options, got, want)
		}
	}
}

func TestToProtoStruct(t *testing.T) {
	v := struct {
		Foo string                 `json:"foo"`
		Bar int                    `json:"bar,omitempty"`
		Baz []float64              `json:"baz"`
		Moo map[string]interface{} `json:"moo"`
	}{
		Foo: "foovalue",
		Baz: []float64{1.1},
		Moo: map[string]interface{}{
			"a": 1,
			"b": "two",
			"c": true,
		},
	}

	got, err := toProtoStruct(v)
	if err != nil {
		t.Fatal(err)
	}
	want := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"foo": {Kind: &structpb.Value_StringValue{StringValue: v.Foo}},
			"baz": {Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{
				{Kind: &structpb.Value_NumberValue{NumberValue: 1.1}},
			}}}},
			"moo": {Kind: &structpb.Value_StructValue{
				StructValue: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"a": {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
						"b": {Kind: &structpb.Value_StringValue{StringValue: "two"}},
						"c": {Kind: &structpb.Value_BoolValue{BoolValue: true}},
					},
				},
			}},
		},
	}
	if !proto.Equal(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}

	// Non-structs should fail to convert.
	for v := range []interface{}{3, "foo", []int{1, 2, 3}} {
		_, err := toProtoStruct(v)
		if err == nil {
			t.Errorf("%v: got nil, want error", v)
		}
	}

	// Test fast path.
	got, err = toProtoStruct(want)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Error("got and want should be identical, but are not")
	}
}

func TestToLogEntryPayload(t *testing.T) {
	var logger Logger
	for _, test := range []struct {
		in         interface{}
		wantText   string
		wantStruct *structpb.Struct
	}{
		{
			in:       "string",
			wantText: "string",
		},
		{
			in: map[string]interface{}{"a": 1, "b": true},
			wantStruct: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"a": {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
					"b": {Kind: &structpb.Value_BoolValue{BoolValue: true}},
				},
			},
		},
		{
			in: json.RawMessage([]byte(`{"a": 1, "b": true}`)),
			wantStruct: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"a": {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
					"b": {Kind: &structpb.Value_BoolValue{BoolValue: true}},
				},
			},
		},
	} {
		e, err := logger.toLogEntry(Entry{Payload: test.in})
		if err != nil {
			t.Fatalf("%+v: %v", test.in, err)
		}
		if test.wantStruct != nil {
			got := e.GetJsonPayload()
			if !proto.Equal(got, test.wantStruct) {
				t.Errorf("%+v: got %s, want %s", test.in, got, test.wantStruct)
			}
		} else {
			got := e.GetTextPayload()
			if got != test.wantText {
				t.Errorf("%+v: got %s, want %s", test.in, got, test.wantText)
			}
		}
	}
}

func TestToLogEntryTrace(t *testing.T) {
	logger := &Logger{client: &Client{parent: "projects/P"}}
	// Verify that we get the trace from the HTTP request if it isn't
	// provided by the caller.
	u := &url.URL{Scheme: "http"}
	for _, test := range []struct {
		in   Entry
		want logging.LogEntry
	}{
		{Entry{}, logging.LogEntry{}},
		{Entry{Trace: "t1"}, logging.LogEntry{Trace: "t1"}},
		{
			Entry{
				HTTPRequest: &HTTPRequest{
					Request: &http.Request{URL: u, Header: http.Header{"foo": {"bar"}}},
				},
			},
			logging.LogEntry{},
		},
		{
			Entry{
				HTTPRequest: &HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"X-Cloud-Trace-Context": {"t2"}},
					},
				},
			},
			logging.LogEntry{Trace: "projects/P/traces/t2"},
		},
		{
			Entry{
				HTTPRequest: &HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"X-Cloud-Trace-Context": {"t3"}},
					},
				},
				Trace: "t4",
			},
			logging.LogEntry{Trace: "t4"},
		},
		{Entry{Trace: "t1", SpanID: "007"}, logging.LogEntry{Trace: "t1", SpanId: "007"}},
	} {
		e, err := logger.toLogEntry(test.in)
		if err != nil {
			t.Fatalf("%+v: %v", test.in, err)
		}
		if got := e.Trace; got != test.want.Trace {
			t.Errorf("%+v: got %q, want %q", test.in, got, test.want.Trace)
		}
		if got := e.SpanId; got != test.want.SpanId {
			t.Errorf("%+v: got %q, want %q", test.in, got, test.want.SpanId)
		}
	}
}

func TestFromHTTPRequest(t *testing.T) {
	// The test URL has invalid UTF-8 runes.
	const testURL = "http://example.com/path?q=1&name=\xfe\xff"
	u, err := url.Parse(testURL)
	if err != nil {
		t.Fatal(err)
	}
	req := &HTTPRequest{
		Request: &http.Request{
			Method: "GET",
			URL:    u,
			Header: map[string][]string{
				"User-Agent": {"user-agent"},
				"Referer":    {"referer"},
			},
		},
		RequestSize:                    100,
		Status:                         200,
		ResponseSize:                   25,
		Latency:                        100 * time.Second,
		LocalIP:                        "127.0.0.1",
		RemoteIP:                       "10.0.1.1",
		CacheHit:                       true,
		CacheValidatedWithOriginServer: true,
	}
	got := fromHTTPRequest(req)
	want := &logtypepb.HttpRequest{
		RequestMethod: "GET",

		// RequestUrl should have its invalid utf-8 runes replaced by the Unicode replacement character U+FFFD.
		// See Issue https://github.com/googleapis/google-cloud-go/issues/1383
		RequestUrl: "http://example.com/path?q=1&name=" + string('\ufffd') + string('\ufffd'),

		RequestSize:                    100,
		Status:                         200,
		ResponseSize:                   25,
		Latency:                        &durpb.Duration{Seconds: 100},
		UserAgent:                      "user-agent",
		ServerIp:                       "127.0.0.1",
		RemoteIp:                       "10.0.1.1",
		Referer:                        "referer",
		CacheHit:                       true,
		CacheValidatedWithOriginServer: true,
	}
	if !proto.Equal(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}

	// And finally checks directly that the error that was
	// in https://github.com/googleapis/google-cloud-go/issues/1383
	// doesn't not regress.
	if _, err := proto.Marshal(got); err != nil {
		t.Fatalf("Unexpected proto.Marshal error: %v", err)
	}
}

func TestMonitoredResource(t *testing.T) {
	for _, test := range []struct {
		parent string
		want   *mrpb.MonitoredResource
	}{
		{
			"projects/P",
			&mrpb.MonitoredResource{
				Type:   "project",
				Labels: map[string]string{"project_id": "P"},
			},
		},

		{
			"folders/F",
			&mrpb.MonitoredResource{
				Type:   "folder",
				Labels: map[string]string{"folder_id": "F"},
			},
		},
		{
			"billingAccounts/B",
			&mrpb.MonitoredResource{
				Type:   "billing_account",
				Labels: map[string]string{"account_id": "B"},
			},
		},
		{
			"organizations/123",
			&mrpb.MonitoredResource{
				Type:   "organization",
				Labels: map[string]string{"organization_id": "123"},
			},
		},
		{
			"unknown/X",
			&mrpb.MonitoredResource{
				Type:   "global",
				Labels: map[string]string{"project_id": "X"},
			},
		},
		{
			"whatever",
			&mrpb.MonitoredResource{
				Type:   "global",
				Labels: map[string]string{"project_id": "whatever"},
			},
		},
	} {
		got := monitoredResource(test.parent)
		if !testutil.Equal(got, test.want) {
			t.Errorf("%q: got %+v, want %+v", test.parent, got, test.want)
		}
	}
}

// Used by the tests in logging_test.
func SetNow(f func() time.Time) {
	now = f
}
