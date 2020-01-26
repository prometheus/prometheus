package harness

import (
	"testing"

	"github.com/opentracing/opentracing-go"
)

func TestAPI(t *testing.T) {
	RunAPIChecks(t, func() (tracer opentracing.Tracer, closer func()) {
		return opentracing.NoopTracer{}, nil
	}, // NoopTracer doesn't do much
		CheckBaggageValues(false),
		CheckInject(false),
		CheckExtract(false),
	)
}
