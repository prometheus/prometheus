package internal_test

// Smoke tests for bindings.

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/common"
	"github.com/stretchr/testify/require"
)

func TestCBindingsInitCleanEncoderSmoke(*testing.T) {
	var encoder = common.NewEncoder(0, 0)
	encoder.Destroy()
}

func TestCBindingsInitCleanDecodeSmoke(t *testing.T) {
	var decoder, err = common.NewDecoder()
	require.NoError(t, err)
	decoder.Destroy()
}
