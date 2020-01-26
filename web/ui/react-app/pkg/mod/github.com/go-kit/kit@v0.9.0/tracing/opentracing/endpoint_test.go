package opentracing_test

import (
	"context"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"

	"github.com/go-kit/kit/endpoint"
	kitot "github.com/go-kit/kit/tracing/opentracing"
)

func TestTraceServer(t *testing.T) {
	tracer := mocktracer.New()

	// Initialize the ctx with a nameless Span.
	contextSpan := tracer.StartSpan("").(*mocktracer.MockSpan)
	ctx := opentracing.ContextWithSpan(context.Background(), contextSpan)

	tracedEndpoint := kitot.TraceServer(tracer, "testOp")(endpoint.Nop)
	if _, err := tracedEndpoint(ctx, struct{}{}); err != nil {
		t.Fatal(err)
	}

	finishedSpans := tracer.FinishedSpans()
	if want, have := 1, len(finishedSpans); want != have {
		t.Fatalf("Want %v span(s), found %v", want, have)
	}

	// Test that the op name is updated
	endpointSpan := finishedSpans[0]
	if want, have := "testOp", endpointSpan.OperationName; want != have {
		t.Fatalf("Want %q, have %q", want, have)
	}
	contextContext := contextSpan.Context().(mocktracer.MockSpanContext)
	endpointContext := endpointSpan.Context().(mocktracer.MockSpanContext)
	// ...and that the ID is unmodified.
	if want, have := contextContext.SpanID, endpointContext.SpanID; want != have {
		t.Errorf("Want SpanID %q, have %q", want, have)
	}
}

func TestTraceServerNoContextSpan(t *testing.T) {
	tracer := mocktracer.New()

	// Empty/background context.
	tracedEndpoint := kitot.TraceServer(tracer, "testOp")(endpoint.Nop)
	if _, err := tracedEndpoint(context.Background(), struct{}{}); err != nil {
		t.Fatal(err)
	}

	// tracedEndpoint created a new Span.
	finishedSpans := tracer.FinishedSpans()
	if want, have := 1, len(finishedSpans); want != have {
		t.Fatalf("Want %v span(s), found %v", want, have)
	}

	endpointSpan := finishedSpans[0]
	if want, have := "testOp", endpointSpan.OperationName; want != have {
		t.Fatalf("Want %q, have %q", want, have)
	}
}

func TestTraceClient(t *testing.T) {
	tracer := mocktracer.New()

	// Initialize the ctx with a parent Span.
	parentSpan := tracer.StartSpan("parent").(*mocktracer.MockSpan)
	defer parentSpan.Finish()
	ctx := opentracing.ContextWithSpan(context.Background(), parentSpan)

	tracedEndpoint := kitot.TraceClient(tracer, "testOp")(endpoint.Nop)
	if _, err := tracedEndpoint(ctx, struct{}{}); err != nil {
		t.Fatal(err)
	}

	// tracedEndpoint created a new Span.
	finishedSpans := tracer.FinishedSpans()
	if want, have := 1, len(finishedSpans); want != have {
		t.Fatalf("Want %v span(s), found %v", want, have)
	}

	endpointSpan := finishedSpans[0]
	if want, have := "testOp", endpointSpan.OperationName; want != have {
		t.Fatalf("Want %q, have %q", want, have)
	}

	parentContext := parentSpan.Context().(mocktracer.MockSpanContext)
	endpointContext := parentSpan.Context().(mocktracer.MockSpanContext)

	// ... and that the parent ID is set appropriately.
	if want, have := parentContext.SpanID, endpointContext.SpanID; want != have {
		t.Errorf("Want ParentID %q, have %q", want, have)
	}
}

func TestTraceClientNoContextSpan(t *testing.T) {
	tracer := mocktracer.New()

	// Empty/background context.
	tracedEndpoint := kitot.TraceClient(tracer, "testOp")(endpoint.Nop)
	if _, err := tracedEndpoint(context.Background(), struct{}{}); err != nil {
		t.Fatal(err)
	}

	// tracedEndpoint created a new Span.
	finishedSpans := tracer.FinishedSpans()
	if want, have := 1, len(finishedSpans); want != have {
		t.Fatalf("Want %v span(s), found %v", want, have)
	}

	endpointSpan := finishedSpans[0]
	if want, have := "testOp", endpointSpan.OperationName; want != have {
		t.Fatalf("Want %q, have %q", want, have)
	}
}
