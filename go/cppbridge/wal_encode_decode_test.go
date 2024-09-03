package cppbridge_test

import (
	"context"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/frames/framestest"
)

type EncoderDecoderSuite struct {
	suite.Suite

	baseCtx context.Context
}

func TestEncoderDecoderSuite(t *testing.T) {
	suite.Run(t, new(EncoderDecoderSuite))
}

func (s *EncoderDecoderSuite) SetupTest() {
	s.baseCtx = context.Background()
}

func (*EncoderDecoderSuite) makeData(count int, sid int64) *prompb.WriteRequest {
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{},
	}

	var (
		startTime int64 = 1654608400000
		step      int64 = 60000
	)

	startTime += step * (sid * 3)

	for i := 0; i < count; i++ {
		wr.Timeseries = append(
			wr.Timeseries,
			prompb.TimeSeries{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test" + strconv.Itoa(i),
					},
					{
						Name:  "instance",
						Value: "blablabla" + strconv.Itoa(i),
					},
					{
						Name:  "job",
						Value: "tester" + strconv.Itoa(i),
					},
					{
						Name:  "low",
						Value: "banan" + strconv.Itoa(i),
					},
					{
						Name:  "zero",
						Value: "non_zero" + strconv.Itoa(i),
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: startTime,
						Value:     4444,
					},
					{
						Timestamp: startTime + step,
						Value:     4447,
					},
					{
						Timestamp: startTime + step*2,
						Value:     4448,
					},
				},
			},
		)
	}

	return wr
}

func (*EncoderDecoderSuite) transferringData(income frames.WritePayload) []byte {
	buf, _ := framestest.ReadPayload(income)
	return buf
}

func (s *EncoderDecoderSuite) TestEncodeDecode() {
	hlimits := cppbridge.DefaultWALHashdexLimits()
	dec := cppbridge.NewWALDecoder(cppbridge.EncodersVersion())
	enc := cppbridge.NewWALEncoder(0, 0)
	count := 10
	k := count + 1
	for i := 0; i < count; i++ {
		s.T().Log("generate protobuf")
		k--
		seriesCount := k * (i + 1)
		expectedWr := s.makeData(seriesCount, int64(i))
		data, err := expectedWr.Marshal()
		s.Require().NoError(err)

		s.T().Log("sharding protobuf")
		h, err := cppbridge.NewWALProtobufHashdex(data, hlimits)
		s.Require().NoError(err)

		s.T().Log("encoding protobuf")
		createdAt := time.Now()
		_, gos, err := enc.Encode(s.baseCtx, h)
		s.Require().NoError(err)

		s.EqualValues(seriesCount, gos.Series())
		s.EqualValues(seriesCount*3, gos.Samples())

		s.T().Log("transferring segment")
		segByte := s.transferringData(gos)

		s.T().Log("decoding protobuf")
		protocont, err := dec.Decode(s.baseCtx, segByte)
		s.Require().NoError(err)

		s.T().Log("compare income and outcome protobuf")
		actualWr := &prompb.WriteRequest{}
		s.Require().NoError(protocont.UnmarshalTo(actualWr))
		s.Equal(expectedWr.String(), actualWr.String())

		s.InDelta(createdAt.UnixNano(), protocont.CreatedAt(), float64(time.Second))

		s.Equal(gos.Samples(), protocont.Samples())
		s.Equal(gos.Series(), protocont.Series())
	}
}

func (s *EncoderDecoderSuite) TestEncodeDecodeOpenHead() {
	createdAt := time.Now()
	hlimits := cppbridge.DefaultWALHashdexLimits()
	dec := cppbridge.NewWALDecoder(cppbridge.EncodersVersion())
	enc := cppbridge.NewWALEncoder(0, 0)
	for i := 1; i < 21; i++ {
		s.T().Log("generate protobuf")
		expectedWr := s.makeData(10, int64(i))
		data, err := expectedWr.Marshal()
		s.Require().NoError(err)

		s.T().Log("sharding protobuf")
		h, err := cppbridge.NewWALProtobufHashdex(data, hlimits)
		s.Require().NoError(err)

		s.T().Log("encoding protobuf")
		gosF, err := enc.Add(s.baseCtx, h)
		s.Require().NoError(err)

		s.EqualValues(10, gosF.Series())
		s.EqualValues(30*i, gosF.Samples())
	}

	_, gos, err := enc.Finalize(s.baseCtx)
	s.Require().NoError(err)

	s.T().Log("transferring segment")
	segByte := s.transferringData(gos)

	s.T().Log("decoding protobuf")
	protob, err := dec.Decode(s.baseCtx, segByte)
	s.Require().NoError(err)

	s.Equal(gos.Samples(), protob.Samples())
	s.Equal(gos.Series(), protob.Series())

	s.T().Log("compare income and outcome protobuf")
	actualWr := &prompb.WriteRequest{}
	s.Require().NoError(protob.UnmarshalTo(actualWr))

	s.InDelta(createdAt.UnixNano(), protob.CreatedAt(), float64(time.Second))
}

func (s *EncoderDecoderSuite) TestRestoreFromStream() {
	hlimits := cppbridge.DefaultWALHashdexLimits()
	enc := cppbridge.NewWALEncoder(0, 0)
	encodersVersion := cppbridge.EncodersVersion()
	buf := make([]byte, 0)
	count := 10
	offsets := make([]uint64, count)

	s.T().Log("generate wal with segments")
	for i := 0; i < count; i++ {
		expectedWr := s.makeData(10, int64(i))
		data, err := expectedWr.Marshal()
		s.Require().NoError(err)
		h, err := cppbridge.NewWALProtobufHashdex(data, hlimits)
		s.Require().NoError(err)
		_, gos, err := enc.Encode(s.baseCtx, h)
		s.Require().NoError(err)
		segByte := s.transferringData(gos)
		buf = append(buf, segByte...)
		offsets[i] = uint64(len(buf))
	}

	for i := range offsets {
		s.T().Logf("restore decoder for segment id: %d\n", i)
		dec := cppbridge.NewWALDecoder(encodersVersion)
		offset, retoreSID, err := dec.RestoreFromStream(s.baseCtx, buf, uint32(i))
		s.Require().NoError(err)

		s.Equal(offsets[i], offset)
		s.Equal(uint32(i), retoreSID)
	}

	s.T().Logf("restore decoder for over segment id: %d\n", count-1)
	dec := cppbridge.NewWALDecoder(encodersVersion)
	// cannot restore to more id
	offset, retoreSID, err := dec.RestoreFromStream(s.baseCtx, buf, uint32(count-1))
	s.Require().NoError(err)

	s.Equal(uint64(len(buf)), offset)
	s.Equal(uint32(count-1), retoreSID)
}

// this test run for local  benchmark test
func (s *EncoderDecoderSuite) EncodeDecodeBench(i int64) {
	expectedWr := s.makeData(100, i)
	data, err := expectedWr.Marshal()
	s.Require().NoError(err)
	hlimits := cppbridge.DefaultWALHashdexLimits()
	dec := cppbridge.NewWALDecoder(cppbridge.EncodersVersion())
	enc := cppbridge.NewWALEncoder(0, 0)
	h, err := cppbridge.NewWALProtobufHashdex(data, hlimits)
	s.Require().NoError(err)

	_, gos, err := enc.Encode(s.baseCtx, h)
	s.Require().NoError(err)

	segByte := s.transferringData(gos)

	protob, err := dec.Decode(s.baseCtx, segByte)
	s.Require().NoError(err)
	_ = protob
}

// this test run for local  benchmark test
func (s *EncoderDecoderSuite) TestEncodeDecodeBenchmark() {
	for i := 0; i < 10; i++ {
		s.EncodeDecodeBench(int64(i))
	}
}

func (s *EncoderDecoderSuite) TestEncodeDecodeToHashdex() {
	hlimits := cppbridge.DefaultWALHashdexLimits()
	dec := cppbridge.NewWALDecoder(cppbridge.EncodersVersion())
	enc := cppbridge.NewWALEncoder(0, 0)

	enc2 := cppbridge.NewWALEncoder(0, 0)
	dec2 := cppbridge.NewWALDecoder(cppbridge.EncodersVersion())

	count := 11
	for i := 1; i < count; i++ {
		seriesCount := rand.Intn(5000-100) + 100
		s.T().Log("generate protobuf")
		var expectedWr *prompb.WriteRequest
		if i%2 == 0 {
			expectedWr = s.makeData(seriesCount, int64(i-1))
		} else {
			expectedWr = s.makeData(seriesCount, int64(i))
		}
		data, err := expectedWr.Marshal()
		s.Require().NoError(err)

		s.T().Log("sharding protobuf")
		h, err := cppbridge.NewWALProtobufHashdex(data, hlimits)
		s.Require().NoError(err)

		s.T().Log("encoding protobuf")
		_, gos, err := enc.Encode(s.baseCtx, h)
		s.Require().NoError(err)

		s.EqualValues(seriesCount, gos.Series())
		s.EqualValues(seriesCount*3, gos.Samples())

		s.T().Log("transferring segment")
		segByte := s.transferringData(gos)

		s.T().Log("decoding to hashdex")
		hContent, err := dec.DecodeToHashdex(s.baseCtx, segByte)
		s.Require().NoError(err)

		s.EqualValues(seriesCount, hContent.Series())
		s.EqualValues(seriesCount*3, hContent.Samples())

		s.T().Log("encoding hashdex")
		_, gos2, err := enc2.Encode(s.baseCtx, hContent.ShardedData())
		s.Require().NoError(err)

		s.EqualValues(seriesCount, gos2.Series())
		s.EqualValues(seriesCount*3, gos2.Samples())

		s.T().Log("transferring segment")
		segByte = s.transferringData(gos2)

		s.T().Log("decoding to protobuf")
		protocont, err := dec2.Decode(s.baseCtx, segByte)
		s.Require().NoError(err)

		s.T().Log("compare income and outcome protobuf")
		actualWr := &prompb.WriteRequest{}
		s.Require().NoError(protocont.UnmarshalTo(actualWr))
		s.Equal(expectedWr.String(), actualWr.String())

		s.EqualValues(seriesCount, protocont.Series())
		s.EqualValues(seriesCount*3, protocont.Samples())
	}
}

func (s *EncoderDecoderSuite) TestEncodeDecodeToHashdexWithMetricInjection() {
	hlimits := cppbridge.DefaultWALHashdexLimits()
	dec := cppbridge.NewWALDecoder(cppbridge.EncodersVersion())
	enc := cppbridge.NewWALEncoder(0, 0)

	enc2 := cppbridge.NewWALEncoder(0, 0)
	dec2 := cppbridge.NewWALDecoder(cppbridge.EncodersVersion())

	count := 3
	for i := 1; i < count; i++ {
		now := time.Now()
		meta := cppbridge.MetaInjection{
			Now:       now.UnixNano(),
			SentAt:    now.UnixNano(),
			AgentUUID: "UUID",
			Hostname:  "SOMEHOSTNAME",
		}

		seriesCount := rand.Intn(100-50) + 50
		s.T().Log("generate protobuf")
		var expectedWr *prompb.WriteRequest
		if i%2 == 0 {
			expectedWr = s.makeData(seriesCount, int64(i-1))
		} else {
			expectedWr = s.makeData(seriesCount, int64(i))
		}
		data, err := expectedWr.Marshal()
		s.Require().NoError(err)

		s.T().Log("sharding protobuf")
		h, err := cppbridge.NewWALProtobufHashdex(data, hlimits)
		s.Require().NoError(err)

		s.T().Log("encoding protobuf")
		_, gos, err := enc.Encode(s.baseCtx, h)
		s.Require().NoError(err)

		s.EqualValues(seriesCount, gos.Series())
		s.EqualValues(seriesCount*3, gos.Samples())

		s.T().Log("transferring segment")
		segByte := s.transferringData(gos)

		s.T().Log("decoding to hashdex")
		hContent, err := dec.DecodeToHashdexWithMetricInjection(s.baseCtx, segByte, &meta)
		s.Require().NoError(err)

		s.EqualValues(seriesCount+3, hContent.Series())
		s.EqualValues((seriesCount*3)+3, hContent.Samples())

		s.T().Log("encoding hashdex")
		_, gos2, err := enc2.Encode(s.baseCtx, hContent.ShardedData())
		s.Require().NoError(err)

		s.EqualValues(seriesCount+3, gos2.Series())
		s.EqualValues((seriesCount*3)+3, gos2.Samples())

		s.T().Log("transferring segment")
		segByte = s.transferringData(gos2)

		s.T().Log("decoding to protobuf")
		protocont, err := dec2.Decode(s.baseCtx, segByte)
		s.Require().NoError(err)

		s.T().Log("compare income and outcome protobuf")
		actualWr := &prompb.WriteRequest{}
		s.Require().NoError(protocont.UnmarshalTo(actualWr))
		s.metricInjection(expectedWr, &meta, now)
		slices.SortFunc(expectedWr.Timeseries, func(a, b prompb.TimeSeries) int { return strings.Compare(a.String(), b.String()) })
		slices.SortFunc(actualWr.Timeseries, func(a, b prompb.TimeSeries) int { return strings.Compare(a.String(), b.String()) })
		s.Equal(expectedWr.String(), actualWr.String())

		s.EqualValues(seriesCount+3, protocont.Series())
		s.EqualValues((seriesCount*3)+3, protocont.Samples())
	}
}

func (s *EncoderDecoderSuite) TestEncodeDecodeToHashdexClusterReplica() {
	hlimits := cppbridge.DefaultWALHashdexLimits()
	dec := cppbridge.NewWALDecoder(cppbridge.EncodersVersion())
	enc := cppbridge.NewWALEncoder(0, 0)

	enc2 := cppbridge.NewWALEncoder(0, 0)
	dec2 := cppbridge.NewWALDecoder(cppbridge.EncodersVersion())

	s.T().Log("generate protobuf")
	expectedReplica := "replica_name"
	expectedCluster := "cluster_name"
	expectedWr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "test",
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
						Name:  "job",
						Value: "tester",
					},
				},
				Samples: []prompb.Sample{
					{
						Timestamp: time.Now().UnixMilli(),
						Value:     4444,
					},
				},
			},
		},
	}

	data, err := expectedWr.Marshal()
	s.Require().NoError(err)

	s.T().Log("sharding protobuf")
	h, err := cppbridge.NewWALProtobufHashdex(data, hlimits)
	s.Require().NoError(err)

	s.T().Log("encoding protobuf")
	_, gos, err := enc.Encode(s.baseCtx, h)
	s.Require().NoError(err)

	s.EqualValues(1, gos.Series())
	s.EqualValues(1, gos.Samples())

	s.T().Log("transferring segment")
	segByte := s.transferringData(gos)

	s.T().Log("decoding to hashdex")
	hContent, err := dec.DecodeToHashdex(s.baseCtx, segByte)
	s.Require().NoError(err)

	s.EqualValues(1, hContent.Series())
	s.EqualValues(1, hContent.Samples())
	s.Equal(expectedReplica, hContent.ShardedData().Replica())
	s.Equal(expectedCluster, hContent.ShardedData().Cluster())

	s.T().Log("encoding hashdex")
	_, gos2, err := enc2.Encode(s.baseCtx, hContent.ShardedData())
	s.Require().NoError(err)

	s.EqualValues(1, gos2.Series())
	s.EqualValues(1, gos2.Samples())

	s.T().Log("transferring segment")
	segByte = s.transferringData(gos2)

	s.T().Log("decoding to protobuf")
	protocont, err := dec2.Decode(s.baseCtx, segByte)
	s.Require().NoError(err)

	s.T().Log("compare income and outcome protobuf")
	actualWr := &prompb.WriteRequest{}
	s.Require().NoError(protocont.UnmarshalTo(actualWr))
	s.Equal(expectedWr.String(), actualWr.String())

	s.EqualValues(1, protocont.Series())
	s.EqualValues(1, protocont.Samples())
}

func (*EncoderDecoderSuite) metricInjection(wr *prompb.WriteRequest, meta *cppbridge.MetaInjection, ts time.Time) {
	tsNowClient := float64(meta.SentAt) / float64(time.Second)

	wr.Timeseries = append(wr.Timeseries,
		prompb.TimeSeries{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "okagent__timestamp"},
				{Name: "agent_uuid", Value: meta.AgentUUID},
				{Name: "conf", Value: "/usr/local/okagent/etc/config.yaml"},
				{Name: "instance", Value: meta.Hostname},
				{Name: "job", Value: "heartbeat"},
				{Name: "okmeter_plugin", Value: "heartbeat"},
				{Name: "okmeter_plugin_instance", Value: "/usr/local/okagent/etc/config.yaml"},
			},
			Samples: []prompb.Sample{
				{Value: tsNowClient, Timestamp: ts.UnixMilli()},
			},
		},
		prompb.TimeSeries{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "okagent__heartbeat"},
				{Name: "agent_uuid", Value: meta.AgentUUID},
				{Name: "instance", Value: meta.Hostname},
				{Name: "job", Value: "collector"},
			},
			Samples: []prompb.Sample{
				{Value: 1, Timestamp: ts.UnixMilli()},
			},
		},
		prompb.TimeSeries{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "time__offset__collector"},
				{Name: "agent_uuid", Value: meta.AgentUUID},
				{Name: "instance", Value: meta.Hostname},
				{Name: "job", Value: "collector"},
			},
			Samples: []prompb.Sample{
				{Value: float64(ts.UnixMilli()*int64(time.Millisecond)-meta.SentAt) / float64(time.Second), Timestamp: ts.UnixMilli()},
			},
		},
	)
}
