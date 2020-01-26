package opentracing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestContextWithSpan(t *testing.T) {
	span := &noopSpan{}
	ctx := ContextWithSpan(context.Background(), span)
	span2 := SpanFromContext(ctx)
	if span != span2 {
		t.Errorf("Not the same span returned from context, expected=%+v, actual=%+v", span, span2)
	}

	ctx = context.Background()
	span2 = SpanFromContext(ctx)
	if span2 != nil {
		t.Errorf("Expected nil span, found %+v", span2)
	}

	ctx = ContextWithSpan(ctx, span)
	span2 = SpanFromContext(ctx)
	if span != span2 {
		t.Errorf("Not the same span returned from context, expected=%+v, actual=%+v", span, span2)
	}
}

func TestStartSpanFromContext(t *testing.T) {
	testTracer := testTracer{}

	// Test the case where there *is* a Span in the Context.
	{
		parentSpan := &testSpan{}
		parentCtx := ContextWithSpan(context.Background(), parentSpan)
		childSpan, childCtx := StartSpanFromContextWithTracer(parentCtx, testTracer, "child")
		if !childSpan.Context().(testSpanContext).HasParent {
			t.Errorf("Failed to find parent: %v", childSpan)
		}
		if !childSpan.(testSpan).Equal(SpanFromContext(childCtx)) {
			t.Errorf("Unable to find child span in context: %v", childCtx)
		}
	}

	// Test the case where there *is not* a Span in the Context.
	{
		emptyCtx := context.Background()
		childSpan, childCtx := StartSpanFromContextWithTracer(emptyCtx, testTracer, "child")
		if childSpan.Context().(testSpanContext).HasParent {
			t.Errorf("Should not have found parent: %v", childSpan)
		}
		if !childSpan.(testSpan).Equal(SpanFromContext(childCtx)) {
			t.Errorf("Unable to find child span in context: %v", childCtx)
		}
	}
}

func TestStartSpanFromContextOptions(t *testing.T) {
	testTracer := testTracer{}

	// Test options are passed to tracer

	startTime := time.Now().Add(-10 * time.Second) // ten seconds ago
	span, ctx := StartSpanFromContextWithTracer(
		context.Background(), testTracer, "parent", StartTime(startTime), Tag{"component", "test"})

	assert.Equal(t, "test", span.(testSpan).Tags["component"])
	assert.Equal(t, startTime, span.(testSpan).StartTime)

	// Test it also works for a child span

	childStartTime := startTime.Add(3 * time.Second)
	childSpan, _ := StartSpanFromContextWithTracer(
		ctx, testTracer, "child", StartTime(childStartTime))

	assert.Equal(t, childSpan.(testSpan).Tags["component"], nil)
	assert.Equal(t, childSpan.(testSpan).StartTime, childStartTime)
}
