package common_test

import (
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/prometheus/prometheus/pp/go/common"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashdex(t *testing.T) {
	expectedCluster := faker.Username()
	expectedReplica := faker.Username()

	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: faker.Username(),
					},
					{
						Name:  "__replica__",
						Value: expectedReplica,
					},
					{
						Name:  "cluster",
						Value: expectedCluster,
					},
					{
						Name:  "instance",
						Value: faker.Username(),
					},
					{
						Name:  "job",
						Value: faker.Username(),
					},
					{
						Name:  "low",
						Value: faker.URL(),
					},
					{
						Name:  "zero",
						Value: faker.Email(),
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: faker.UnixTime(),
						Value:     faker.Latitude(),
					},
				},
			},
		},
	}

	data, err := wr.Marshal()
	require.NoError(t, err)
	hx, err := common.NewHashdex(data)
	require.NoError(t, err)

	cr := hx.Cluster()
	ra := hx.Replica()
	assert.Equal(t, expectedCluster, cr)
	assert.Equal(t, expectedReplica, ra)

	hx.Destroy()
	for i := range data {
		data[i] = 1
	}

	assert.Equal(t, expectedCluster, cr)
	assert.Equal(t, expectedReplica, ra)
}
