package common_test

import (
	"context"
	"testing"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/common"
)

type EncoderSuite struct {
	suite.Suite

	enc         *common.Encoder
	ctx         context.Context
	shardID     uint16
	encodeCount int
	bufSeg      []common.Segment
	bufRed      []common.Redundant
	expSnapshot []byte
	expSegment  [][]byte
}

func TestEncoderSuite(t *testing.T) {
	suite.Run(t, new(EncoderSuite))
}

func (es *EncoderSuite) SetupTest() {
	es.shardID = 0
	es.ctx = context.Background()
	es.enc = common.NewEncoder(es.shardID, 1)
	es.encodeCount = 100
	es.bufSeg = make([]common.Segment, 0, es.encodeCount)
	es.bufRed = make([]common.Redundant, 0, es.encodeCount)
	es.expSnapshot = make([]byte, 0, es.encodeCount)
	es.expSegment = make([][]byte, 0, es.encodeCount)
}

func (es *EncoderSuite) TearDownTest() {
	es.enc.Destroy()
}

func (es *EncoderSuite) makeData() []byte {
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
					},
					{
						Name:  "job",
						Value: "tester",
					},
					{
						Name:  "instance",
						Value: "blablabla",
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: 1654608420000,
						Value:     4444,
					},
				},
			},
		},
	}

	b, err := wr.Marshal()
	es.NoError(err)
	return b
}

func (es *EncoderSuite) TestEncode() {
	es.T().Log("encode data and accumulate segment and redundant")
	for i := 0; i < es.encodeCount; i++ {
		data := es.makeData()
		h := common.NewHashdex(data)

		segKey, gos, gor, err := es.enc.Encode(es.ctx, h)
		es.NoError(err)
		h.Destroy()

		es.Equal(segKey.Segment, es.enc.LastEncodedSegment())
		es.bufSeg = append(es.bufSeg, gos)
		es.bufRed = append(es.bufRed, gor)
		es.expSnapshot = append(es.expSnapshot, data[0])
		es.expSegment = append(es.expSegment, data)
	}

	es.T().Log("get snapshot")
	snapshot, err := es.enc.Snapshot(es.ctx, es.bufRed)
	es.NoError(err)
	es.T().Log("destroy snapshot")
	snapshot.Destroy()

	es.T().Log("destroy segments and redundants")
	for i, gos := range es.bufSeg {
		gos.Destroy()
		es.bufRed[i].Destroy()
	}
}

func (es *EncoderSuite) TestEncodeError() {
	ctx, cancel := context.WithCancel(es.ctx)
	cancel()

	h := common.NewHashdex(es.makeData())
	defer h.Destroy()

	_, _, _, err := es.enc.Encode(ctx, h)
	es.Error(err)
}

func (es *EncoderSuite) TestSnapshotError() {
	ctx, cancel := context.WithCancel(es.ctx)
	cancel()
	_, err := es.enc.Snapshot(ctx, es.bufRed)
	es.Error(err)
}

func BenchmarkEncoder(b *testing.B) {
	ctx := context.Background()
	enc := common.NewEncoder(0, 1)

	defer enc.Destroy()

	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
					},
					{
						Name:  "job",
						Value: "tester",
					},
					{
						Name:  "instance",
						Value: "blablabla",
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: 1654608420000,
						Value:     4444,
					},
				},
			},
		},
	}

	data, err := wr.Marshal()
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		h := common.NewHashdex(data)
		_, gos, gor, _ := enc.Encode(ctx, h)
		h.Destroy()
		gos.Destroy()
		gor.Destroy()
	}
}
