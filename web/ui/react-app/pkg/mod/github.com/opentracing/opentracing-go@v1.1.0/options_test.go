package opentracing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChildOfAndFollowsFrom(t *testing.T) {
	tests := []struct {
		newOpt  func(SpanContext) SpanReference
		refType SpanReferenceType
		name    string
	}{
		{ChildOf, ChildOfRef, "ChildOf"},
		{FollowsFrom, FollowsFromRef, "FollowsFrom"},
	}

	for _, test := range tests {
		opts := new(StartSpanOptions)

		test.newOpt(nil).Apply(opts)
		require.Nil(t, opts.References, "%s(nil) must not append a reference", test.name)

		ctx := new(noopSpanContext)
		test.newOpt(ctx).Apply(opts)
		require.Equal(t, []SpanReference{
			SpanReference{ReferencedContext: ctx, Type: test.refType},
		}, opts.References, "%s(ctx) must append a reference", test.name)
	}
}
