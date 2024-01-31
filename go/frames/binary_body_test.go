package frames_test

import (
	"context"
	"hash/crc32"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/util"
)

type BinaryBodySuite struct {
	suite.Suite

	rw      *FileBuffer
	version uint8
}

func TestBinaryBodySuite(t *testing.T) {
	suite.Run(t, new(BinaryBodySuite))
}

func (s *BinaryBodySuite) SetupSuite() {
	s.rw = NewFileBuffer()
	s.version = 4
}

func (s *BinaryBodySuite) TearDownTest() {
	s.rw.Reset()
}

func (s *BinaryBodySuite) TestReadFrameSegmentV1() {
	ctx := context.Background()
	var (
		data             = uuid.NewString()
		shardID   uint16 = 1
		segmentID uint32 = 5
	)

	bbv1 := frames.NewBinaryBodyV1([]byte(data))
	wh, err := frames.NewHeaderOld(
		3,
		frames.ContentVersion1,
		frames.SegmentType,
		shardID,
		segmentID,
		uint32(bbv1.Size()),
	)
	s.Require().NoError(err)
	chksum := crc32.NewIEEE()
	_, _ = bbv1.WriteTo(chksum)
	wh.SetChksum(chksum.Sum32())
	wh.SetCreatedAt(time.Now().UnixNano())

	_, err = s.rw.Write(wh.EncodeBinary())
	s.Require().NoError(err)

	_, err = bbv1.WriteTo(s.rw)
	s.Require().NoError(err)

	_, err = s.rw.Seek(0, 0)
	s.Require().NoError(err)

	rbb, err := frames.ReadFrameSegment(ctx, util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Require().Equal(segmentID, rbb.GetSegmentID())
	s.Require().Equal(shardID, rbb.GetShardID())
	s.Require().Equal(data, string(rbb.Bytes()))
}

func (s *BinaryBodySuite) TestReadFrameSegmentV2() {
	ctx := context.Background()
	var (
		data             = uuid.NewString()
		shardID   uint16 = 1
		segmentID uint32 = 5
	)

	bbv2 := frames.NewBinaryBodyV2([]byte(data), shardID, segmentID)
	wf, err := frames.NewWriteFrame(
		4,
		frames.ContentVersion2,
		frames.SegmentType,
		bbv2,
	)
	s.Require().NoError(err)

	_, err = wf.WriteTo(s.rw)
	s.Require().NoError(err)

	rbb, err := frames.ReadFrameSegment(ctx, util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Require().Equal(segmentID, rbb.GetSegmentID())
	s.Require().Equal(shardID, rbb.GetShardID())
	s.Require().Equal(data, string(rbb.Bytes()))
}

func (s *BinaryBodySuite) TestBinaryBodyV2() {
	ctx := context.Background()
	var (
		data             = uuid.NewString()
		shardID   uint16 = 1
		segmentID uint32 = 5
	)
	wbbv2 := frames.NewBinaryBodyV2([]byte(data), shardID, segmentID)
	n, err := wbbv2.WriteTo(s.rw)
	s.Require().NoError(err)

	_, err = s.rw.Seek(0, 0)
	s.Require().NoError(err)

	rbbv2, err := frames.ReadBinaryBodyV2(ctx, s.rw, int(n))
	s.Require().NoError(err)

	s.Require().Equal(shardID, rbbv2.GetShardID())
	s.Require().Equal(segmentID, rbbv2.GetSegmentID())
	s.Require().EqualValues(wbbv2.Size(), rbbv2.Size())
	s.Require().Equal(data, string(rbbv2.Bytes()))
}

func (s *BinaryBodySuite) TestSegmentInfoRead() {
	wsi := frames.NewSegmentInfo(1, 2)
	_, err := wsi.WriteTo(s.rw)
	s.Require().NoError(err)

	ctx := context.Background()
	_, err = s.rw.Seek(0, 0)
	s.Require().NoError(err)
	h, err := frames.NewHeader(s.version, frames.ContentVersion2, frames.SegmentType, 10)
	s.Require().NoError(err)

	rsi := frames.NewSegmentInfoEmpty()
	n, err := rsi.ReadSegmentInfo(ctx, s.rw, h)
	s.Require().NoError(err)
	s.Equal(6, n)
	s.Equal(*wsi, *rsi)
}

func (s *BinaryBodySuite) TestSegmentInfoReadV1() {
	ctx := context.Background()
	h, err := frames.NewHeaderOld(3, frames.ContentVersion1, frames.SegmentType, 1, 2, 10)
	s.Require().NoError(err)

	rsi := frames.NewSegmentInfoEmpty()
	n, err := rsi.ReadSegmentInfo(ctx, s.rw, h)
	s.Require().NoError(err)
	s.Equal(0, n)
	s.Equal(h.GetSegmentID(), rsi.GetSegmentID())
	s.Equal(h.GetShardID(), rsi.GetShardID())
}

func (s *BinaryBodySuite) TestSegmentInfoReadError() {
	wsi := frames.NewSegmentInfo(1, 2)
	_, err := wsi.WriteTo(s.rw)
	s.Require().NoError(err)

	ctx := context.Background()
	_, err = s.rw.Seek(0, 0)
	s.Require().NoError(err)
	h, err := frames.NewHeader(s.version, 4, frames.SegmentType, 10)
	s.Require().NoError(err)

	rsi := frames.NewSegmentInfoEmpty()
	_, err = rsi.ReadSegmentInfo(ctx, s.rw, h)
	s.Require().Error(err)
}

func (s *BinaryBodySuite) TestBinaryWrapper() {
	ctx := context.Background()
	var (
		data             = uuid.NewString()
		shardID   uint16 = 1
		segmentID uint32 = 5
	)

	bbv1 := frames.NewBinaryBodyV1([]byte(data))
	bw := frames.NewBinaryWrapper(shardID, segmentID, bbv1)
	n, err := bw.WriteTo(s.rw)
	s.Require().NoError(err)

	_, err = s.rw.Seek(0, 0)
	s.Require().NoError(err)

	bbv2, err := frames.ReadBinaryBodyV2(ctx, s.rw, int(n))
	s.Require().NoError(err)

	s.Require().Equal(shardID, bbv2.GetShardID())
	s.Require().Equal(segmentID, bbv2.GetSegmentID())
	s.Require().EqualValues(bbv2.Size(), bw.Size())
	s.Require().Equal(data, string(bbv2.Bytes()))
}
