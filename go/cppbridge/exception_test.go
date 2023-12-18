package cppbridge_test

import (
	"testing"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

func TestHashdex_error(t *testing.T) {
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{},
				Samples: []prompb.Sample{
					{
						Timestamp: -1654608420000,
						Value:     4444,
					},
				},
			},
		},
	}
	b, err := wr.Marshal()
	require.NoError(t, err)

	hlimits := cppbridge.DefaultHashdexLimits()
	_, err = cppbridge.NewWALHashdex(b, hlimits)
	assert.Error(t, err)
	var coreErr *cppbridge.Exception
	assert.ErrorAs(t, err, &coreErr)
	assert.EqualValues(t, 0x68997b7d2e49de1e, coreErr.Code())
}
