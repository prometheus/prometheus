package frames_test

import (
	"context"
	"hash/crc32"
	"testing"
	"testing/quick"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/stretchr/testify/suite"
)

type TitleSuite struct {
	suite.Suite

	rw      *FileBuffer
	version uint8
}

func TestTitleSuite(t *testing.T) {
	suite.Run(t, new(TitleSuite))
}

func (s *TitleSuite) SetupSuite() {
	s.rw = NewFileBuffer()
	s.version = 3
}

func (s *TitleSuite) TearDownTest() {
	s.rw.Reset()
}

func (s *TitleSuite) TestTitleV1() {
	wm := frames.NewTitleV1(1, uuid.New())
	b, err := wm.MarshalBinary()
	s.Require().NoError(err)

	rm := frames.NewTitleV1Empty()
	err = rm.UnmarshalBinary(b)
	s.Require().NoError(err)

	s.Require().Equal(*wm, *rm)
}

func (s *TitleSuite) TestTitleV1Quick() {
	f := func(snp uint8, blockID uuid.UUID) bool {
		wm := frames.NewTitleV1(snp, blockID)
		b, err := wm.MarshalBinary()
		s.Require().NoError(err)

		rm := frames.NewTitleV1Empty()
		err = rm.UnmarshalBinary(b)
		s.Require().NoError(err)

		return s.Equal(*wm, *rm)
	}

	err := quick.Check(f, nil)
	s.NoError(err)
}

func (s *TitleSuite) TestTitleV1FrameAt() {
	ctx := context.Background()
	var (
		snp     uint8     = 2
		blockID uuid.UUID = uuid.New()
	)
	wm, err := frames.NewTitleFrameV1(snp, blockID)
	s.Require().NoError(err)
	b := wm.EncodeBinary()

	_, err = s.rw.Write(b)
	s.Require().NoError(err)

	var off int64
	h, err := frames.ReadHeader(ctx, util.NewOffsetReader(s.rw, off))
	s.Require().NoError(err)
	off += int64(h.SizeOf())
	s.Require().Equal(frames.TitleType, h.GetType())

	rm, err := frames.ReadAtTitle(ctx, s.rw, off, int(h.GetSize()), h.GetContentVersion())
	s.Require().NoError(err)

	s.Require().Equal(snp, rm.GetShardsNumberPower())
	s.Require().Equal(blockID, rm.GetBlockID())
}

func (s *TitleSuite) TestTitleV1Frame() {
	ctx := context.Background()
	var (
		snp     uint8     = 2
		blockID uuid.UUID = uuid.New()
	)
	wm, err := frames.NewTitleFrameV1(snp, blockID)
	s.Require().NoError(err)
	b := wm.EncodeBinary()

	_, err = s.rw.Write(b)
	s.Require().NoError(err)

	_, err = s.rw.Seek(0, 0)
	s.Require().NoError(err)

	h, err := frames.ReadHeader(ctx, s.rw)
	s.Require().NoError(err)
	s.Require().Equal(frames.TitleType, h.GetType())

	rm, err := frames.ReadTitle(ctx, s.rw, int(h.GetSize()), h.GetContentVersion())
	s.Require().NoError(err)

	s.Require().Equal(snp, rm.GetShardsNumberPower())
	s.Require().Equal(blockID, rm.GetBlockID())
}

func (s *TitleSuite) TestTitleV2() {
	wm := frames.NewTitleV2(1, 1, uuid.New())
	b, err := wm.MarshalBinary()
	s.Require().NoError(err)

	rm := frames.NewTitleV2Empty()
	err = rm.UnmarshalBinary(b)
	s.Require().NoError(err)

	s.Require().Equal(*wm, *rm)
}

func (s *TitleSuite) TestTitleV2Quick() {
	f := func(snp, encodersVersion uint8, blockID uuid.UUID) bool {
		wm := frames.NewTitleV2(snp, encodersVersion, blockID)
		b, err := wm.MarshalBinary()
		s.Require().NoError(err)

		rm := frames.NewTitleV2Empty()
		err = rm.UnmarshalBinary(b)
		s.Require().NoError(err)

		return s.Equal(*wm, *rm)
	}

	err := quick.Check(f, nil)
	s.NoError(err)
}

func (s *TitleSuite) TestTitleV2FrameAt() {
	ctx := context.Background()
	var (
		snp             uint8     = 2
		encodersVersion uint8     = 1
		blockID         uuid.UUID = uuid.New()
	)
	wm, err := frames.NewTitleFrameV2(snp, encodersVersion, blockID)
	s.Require().NoError(err)
	b := wm.EncodeBinary()

	_, err = s.rw.Write(b)
	s.Require().NoError(err)

	var off int64
	h, err := frames.ReadHeader(ctx, util.NewOffsetReader(s.rw, off))
	s.Require().NoError(err)
	off += int64(h.SizeOf())
	s.Require().Equal(frames.TitleType, h.GetType())

	rm, err := frames.ReadAtTitle(ctx, s.rw, off, int(h.GetSize()), h.GetContentVersion())
	s.Require().NoError(err)

	s.Require().Equal(snp, rm.GetShardsNumberPower())
	s.Require().Equal(blockID, rm.GetBlockID())
}

func (s *TitleSuite) TestTitleV2Frame() {
	ctx := context.Background()
	var (
		snp             uint8     = 2
		encodersVersion uint8     = 1
		blockID         uuid.UUID = uuid.New()
	)
	wm, err := frames.NewTitleFrameV2(snp, encodersVersion, blockID)
	s.Require().NoError(err)
	b := wm.EncodeBinary()

	_, err = s.rw.Write(b)
	s.Require().NoError(err)

	_, err = s.rw.Seek(0, 0)
	s.Require().NoError(err)

	h, err := frames.ReadHeader(ctx, s.rw)
	s.Require().NoError(err)
	s.Require().Equal(frames.TitleType, h.GetType())

	rm, err := frames.ReadTitle(ctx, s.rw, int(h.GetSize()), h.GetContentVersion())
	s.Require().NoError(err)

	s.Require().Equal(snp, rm.GetShardsNumberPower())
	s.Require().Equal(encodersVersion, rm.GetEncodersVersion())
	s.Require().Equal(blockID, rm.GetBlockID())
}

func (s *TitleSuite) TestTitleOldFrameAt() {
	ctx := context.Background()
	var (
		snp     uint8     = 2
		blockID uuid.UUID = uuid.New()
	)

	body, err := frames.NewTitleV1(snp, blockID).MarshalBinary()
	s.Require().NoError(err)
	wh, err := frames.NewHeaderOld(3, frames.ContentVersion1, frames.TitleType, 0, 1, uint32(len(body)))
	s.Require().NoError(err)
	wh.SetChksum(crc32.ChecksumIEEE(body))
	wh.SetCreatedAt(time.Now().UnixNano())

	wm := &frames.ReadFrame{Header: wh, Body: body}
	b := wm.EncodeBinary()

	_, err = s.rw.Write(b)
	s.Require().NoError(err)

	var off int64
	h, err := frames.ReadHeader(ctx, util.NewOffsetReader(s.rw, off))
	s.Require().NoError(err)
	off += int64(h.SizeOf())
	s.Require().Equal(frames.TitleType, h.GetType())

	rm, err := frames.ReadAtTitle(ctx, s.rw, off, int(h.GetSize()), h.GetContentVersion())
	s.Require().NoError(err)

	s.Require().Equal(snp, rm.GetShardsNumberPower())
	s.Require().Equal(blockID, rm.GetBlockID())
}

func (s *TitleSuite) TestTitleOldFrame() {
	ctx := context.Background()
	var (
		snp     uint8     = 2
		blockID uuid.UUID = uuid.New()
	)

	body, err := frames.NewTitleV1(snp, blockID).MarshalBinary()
	s.Require().NoError(err)
	wh, err := frames.NewHeaderOld(3, frames.ContentVersion1, frames.TitleType, 0, 1, uint32(len(body)))
	s.Require().NoError(err)
	wh.SetChksum(crc32.ChecksumIEEE(body))
	wh.SetCreatedAt(time.Now().UnixNano())

	wm := &frames.ReadFrame{Header: wh, Body: body}
	b := wm.EncodeBinary()

	_, err = s.rw.Write(b)
	s.Require().NoError(err)

	_, err = s.rw.Seek(0, 0)
	s.Require().NoError(err)

	h, err := frames.ReadHeader(ctx, s.rw)
	s.Require().NoError(err)
	s.Require().Equal(frames.TitleType, h.GetType())

	rm, err := frames.ReadTitle(ctx, s.rw, int(h.GetSize()), h.GetContentVersion())
	s.Require().NoError(err)

	s.Require().Equal(snp, rm.GetShardsNumberPower())
	s.Require().Equal(blockID, rm.GetBlockID())
}
