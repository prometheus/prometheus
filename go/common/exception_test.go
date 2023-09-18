package common_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/common"
)

func TestException(t *testing.T) {
	ctx := context.Background()
	enc := common.NewEncoder(0, 1)
	t.Cleanup(enc.Destroy)

	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
					},
				},
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

	hlimits := common.DefaultHashdexLimits()
	h, err := common.NewHashdex(b, hlimits)
	require.NoError(t, err)

	_, _, _, err = enc.Encode(ctx, h)
	assert.Error(t, err)
	assert.True(t,
		common.IsExceptionCodeFromErrorAnyOf(err, 0x546e143d302c4860),
		"Exception code is %x: %+v",
		common.GetExceptionCodeFromError(err),
		err,
	)
	fmt.Printf("%+v\n", err)
}
