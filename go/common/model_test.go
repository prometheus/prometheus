package common_test

import (
	"testing"
	"testing/quick"

	"github.com/go-faker/faker/v4"
	"github.com/prometheus/prometheus/pp/go/common"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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
	hlimits := common.DefaultHashdexLimits()
	hx, err := common.NewHashdex(data, hlimits)
	require.NoError(t, err)

	cr := hx.Cluster()
	ra := hx.Replica()
	assert.Equal(t, expectedCluster, cr)
	assert.Equal(t, expectedReplica, ra)

	for i := range data {
		data[i] = 1
	}

	assert.Equal(t, expectedCluster, cr)
	assert.Equal(t, expectedReplica, ra)
}

type HashdexLimitsSuite struct {
	suite.Suite
}

func TestHashdexLimits(t *testing.T) {
	suite.Run(t, new(HashdexLimitsSuite))
}

func (s *HashdexLimitsSuite) TestMarshalBinaryUnmarshalBinary() {
	hlm := common.DefaultHashdexLimits()

	b, err := hlm.MarshalBinary()
	s.NoError(err)

	hlu := common.HashdexLimits{}
	err = hlu.UnmarshalBinary(b)
	s.NoError(err)

	s.Equal(hlm, hlu)
}

func (s *HashdexLimitsSuite) TestMarshalBinaryUnmarshalBinary_Quick() {
	f := func(
		maxLabelNameLength, maxLabelValueLength, maxLabelNamesPerTimeseries uint32,
		maxTimeseriesCount, maxPbSizeInBytes uint64,
	) bool {
		hlm := common.HashdexLimits{
			MaxLabelNameLength:         maxLabelNameLength,
			MaxLabelValueLength:        maxLabelValueLength,
			MaxLabelNamesPerTimeseries: maxLabelNamesPerTimeseries,
			MaxTimeseriesCount:         maxTimeseriesCount,
			MaxPbSizeInBytes:           maxPbSizeInBytes,
		}

		b, err := hlm.MarshalBinary()
		s.NoError(err)

		hlu := common.HashdexLimits{}
		err = hlu.UnmarshalBinary(b)
		s.NoError(err)

		return s.Equal(hlm, hlu)
	}

	err := quick.Check(f, nil)
	s.NoError(err)
}
