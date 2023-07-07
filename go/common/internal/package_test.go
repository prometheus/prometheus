package internal_test

// Smoke tests for bindings.

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/common"
)

func TestCBindingsInitCleanEncoderSmoke(t *testing.T) {
	var encoder = common.NewEncoder(0, 0)
	encoder.Destroy()
}

func TestCBindingsInitCleanDecodeSmoke(t *testing.T) {
	var decoder = common.NewDecoder()
	decoder.Destroy()
}
