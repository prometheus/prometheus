package opentracing

import (
	"reflect"
	"testing"
)

func TestIsGlobalTracerRegisteredDefaultIsFalse(t *testing.T) {
	if IsGlobalTracerRegistered() {
		t.Errorf("Should return false when no global tracer is registered.")
	}
}

func TestAfterSettingGlobalTracerIsGlobalTracerRegisteredReturnsTrue(t *testing.T) {
	SetGlobalTracer(NoopTracer{})

	if !IsGlobalTracerRegistered() {
		t.Errorf("Should return true after a tracer has been registered.")
	}
}

func TestDefaultTracerIsNoopTracer(t *testing.T) {
	if reflect.TypeOf(GlobalTracer()) != reflect.TypeOf(NoopTracer{}) {
		t.Errorf("Should return false when no global tracer is registered.")
	}
}
