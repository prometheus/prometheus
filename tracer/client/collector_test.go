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
package tracer

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/prometheus/prometheus/tracer/model"
)

func TestParseTagsQuery(t *testing.T) {
	testData := []struct {
		query string
		res   map[string]string
		err   error
	}{
		{
			`"x":"test*"`,
			map[string]string{"x": "test*"},
			nil,
		},
		{
			`"x":"test*" , "y":"abc+"`,
			map[string]string{"x": "test*", "y": "abc+"},
			nil,
		},
		{
			`x:"test*"`,
			map[string]string{},
			errors.New("invalid input"),
		},
		{
			`{x="test.*",y=".*test"}`,
			map[string]string{},
			errors.New("invalid input"),
		},
		{
			`x="test.*" . y=".*test"`,
			map[string]string{},
			errors.New("invalid input"),
		},
	}

	for _, data := range testData {
		r, err := ParseTagsQuery(data.query)
		if (data.err == nil && err != nil) || (data.err != nil && err == nil) {
			t.Errorf("Expected Error %v, got %v", data.err, err)
		}

		if data.err == nil {
			for k, v := range r {
				if val, ok := data.res[k]; !ok || val != v.String() {
					t.Errorf("Expected (<TEST_KEY>,%v), got (%v,%v)", val, k, v.String())
				}
			}
		}
	}
}

func TestGetFreeSlotInBuffer(t *testing.T) {
	collector := newCollector(5)

	// test adding in non-full buffer
	for i := 1; i < 3; i++ {
		span := model.Span{
			SpanContext: model.SpanContext{
				TraceId: uint64(i),
			},
			OperationName: fmt.Sprintf("span %d", i),
		}
		if err := collector.Collect(span); err != nil {
			t.Fatal(err)
		}
	}
	if slotID := collector.getFreeSlotInBuffer(0); slotID != 2 {
		t.Fatalf("Expected Free SlotID 2, got %d", slotID)
	}

	// test wrap around in buffer
	for i := 3; i < 12; i++ {
		span := model.Span{
			SpanContext: model.SpanContext{
				TraceId: uint64(i),
			},
			OperationName: fmt.Sprintf("span %d", i),
		}
		if err := collector.Collect(span); err != nil {
			t.Fatal(err)
		}
	}
	if slotID := collector.getFreeSlotInBuffer(0); slotID != 2 {
		t.Fatalf("Expected Free SlotID 2, got %d", slotID)
	}

}

func TestCollectorSpans(t *testing.T) {
	collector := newCollector(5)

	// TraceID should never be 0
	if err := collector.Collect(model.Span{}); err == nil {
		t.Fatal(err)
	}

	want := []model.Trace{}
	for i := 1; i < 6; i++ {
		span := model.Span{
			SpanContext: model.SpanContext{
				TraceId: uint64(i),
			},
			OperationName: fmt.Sprintf("span %d", i),
		}
		want = append(want, model.Trace{TraceId: uint64(i), Spans: []model.Span{span}})
		if err := collector.Collect(span); err != nil {
			t.Fatal(err)
		}
	}

	if have := collector.Gather(nil); !reflect.DeepEqual(want, have) {
		t.Fatalf("%s", Diff(want, have))
	}

	want = []model.Trace{}
	for i := 6; i < 11; i++ {
		span := model.Span{
			SpanContext: model.SpanContext{
				TraceId: uint64(i),
			},
			OperationName: fmt.Sprintf("span %d", i),
		}
		want = append(want, model.Trace{TraceId: uint64(i), Spans: []model.Span{span}})
		if err := collector.Collect(span); err != nil {
			t.Fatal(err)
		}
	}

	if have := collector.Gather(nil); !reflect.DeepEqual(want, have) {
		t.Fatalf("%s", Diff(want, have))
	}
}

func TestCollectorFuzzSpans(t *testing.T) {
	capacity := 7

	iterate := func(iteration int) {
		collector := newCollector(capacity)
		toInsert := rand.Intn(capacity + 1)
		want := []model.Trace{}
		for i := 1; i <= toInsert; i++ {
			span := model.Span{
				SpanContext: model.SpanContext{
					TraceId: uint64(i),
				},
				OperationName: fmt.Sprintf("span %d %d", iteration, i),
			}
			want = append(want, model.Trace{TraceId: uint64(i), Spans: []model.Span{span}})
			if err := collector.Collect(span); err != nil {
				t.Fatal(err)
			}
		}

		if have := collector.Gather(nil); !reflect.DeepEqual(want, have) {
			t.Fatalf("%s", Diff(want, have))
		}
	}

	for i := 0; i < 100; i++ {
		iterate(i)
	}
}

func TestLongerTraceCollection(t *testing.T) {
	collector := newCollector(5)

	shortTraces := []model.Trace{}
	for i := 1; i < 6; i++ {
		span := model.Span{
			SpanContext: model.SpanContext{
				TraceId: uint64(i),
			},
			OperationName: fmt.Sprintf("span %d", i),
		}
		shortTraces = append(shortTraces, model.Trace{TraceId: uint64(i), Spans: []model.Span{span}})
		if err := collector.Collect(span); err != nil {
			t.Fatal(err)
		}
	}

	var defaultTime time.Time
	longTraces := []model.Trace{}

	for i := 7; i < 11; i++ {
		span := model.Span{
			SpanContext: model.SpanContext{
				TraceId: uint64(i),
			},
			// creating spans of duration 10 minutes
			End:           defaultTime.Add(10 * time.Minute),
			OperationName: fmt.Sprintf("span %d", i),
		}
		longTraces = append(longTraces, model.Trace{TraceId: uint64(i), Spans: []model.Span{span}})
		if err := collector.Collect(span); err != nil {
			t.Fatal(err)
		}
	}

	want := append(longTraces, shortTraces...)

	if have := collector.Gather(nil); !reflect.DeepEqual(want, have) {
		t.Fatalf("%s", Diff(want, have))
	}
}
func TestCollectorTracePromotion(t *testing.T) {
	collector := newCollector(5)

	want := []model.Trace{}
	for i := 1; i < 6; i++ {
		span := model.Span{
			SpanContext: model.SpanContext{
				TraceId: uint64(i),
			},
			OperationName: fmt.Sprintf("span %d", i),
		}
		want = append(want, model.Trace{TraceId: uint64(i), Spans: []model.Span{span}})
		if err := collector.Collect(span); err != nil {
			t.Fatal(err)
		}
	}

	var defaultTime time.Time
	for i := 1; i < 6; i++ {
		span := model.Span{
			SpanContext: model.SpanContext{
				TraceId: uint64(i),
			},
			// creating spans of duration 10 minutes
			End:           defaultTime.Add(10 * time.Minute),
			OperationName: fmt.Sprintf("span-2 %d", i),
		}

		want[i-1].Spans = append(want[i-1].Spans, span)
		if err := collector.Collect(span); err != nil {
			t.Fatal(err)
		}
	}

	if have := collector.Gather(nil); !reflect.DeepEqual(want, have) {
		t.Fatalf("%s", Diff(want, have))
	}
}

// Diff diffs two arbitrary data structures, giving human-readable output.
func Diff(want, have interface{}) string {
	config := spew.NewDefaultConfig()
	config.ContinueOnMethod = true
	config.SortKeys = true
	config.SpewKeys = true
	text, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(config.Sdump(want)),
		B:        difflib.SplitLines(config.Sdump(have)),
		FromFile: "want",
		ToFile:   "have",
		Context:  3,
	})
	return "\n" + text
}
