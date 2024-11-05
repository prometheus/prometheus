package cppbridge_test

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHeadWalEncoder_Finalize(t *testing.T) {
	lss := cppbridge.NewQueryableLssStorage()
	encoder := cppbridge.NewHeadWalEncoder(0, 1, lss)
	segmentData, err := encoder.Finalize()
	require.NoError(t, err)
	_ = segmentData
}
