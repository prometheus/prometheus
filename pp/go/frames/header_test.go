package frames_test

import (
	"context"
	"testing"

	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/stretchr/testify/suite"
)

type HeaderSuite struct {
	suite.Suite

	rw      *FileBuffer
	version uint8
}

func TestHeaderSuite(t *testing.T) {
	suite.Run(t, new(HeaderSuite))
}

func (s *HeaderSuite) SetupSuite() {
	s.rw = NewFileBuffer()
	s.version = 3
}

func (s *HeaderSuite) TearDownTest() {
	s.rw.Reset()
}

func (s *HeaderSuite) TestHeaderV3EncodeDecodeBinaryAt() {
	wh := frames.NewHeaderV3(0, 1, 10)
	wh.SetCreatedAt(100)
	wh.SetChksum(1000)
	b := wh.EncodeBinary()
	_, err := s.rw.Write(b)
	s.Require().NoError(err)

	rh := frames.NewHeaderV3Empty()
	err = rh.DecodeBinary(util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Equal(wh.String(), rh.String())
	s.Equal(wh.GetChksum(), rh.GetChksum())
	s.Equal(wh.GetCreatedAt(), rh.GetCreatedAt())
}

func (s *HeaderSuite) TestHeaderV4EncodeDecodeBinaryAt() {
	wh := frames.NewHeaderV4(1, 10)
	wh.SetCreatedAt(100)
	wh.SetChksum(1000)
	b := wh.EncodeBinary()
	_, err := s.rw.Write(b)
	s.Require().NoError(err)

	rh := frames.NewHeaderV4Empty()
	err = rh.DecodeBinary(util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Equal(wh.String(), rh.String())
	s.Equal(wh.GetChksum(), rh.GetChksum())
	s.Equal(wh.GetCreatedAt(), rh.GetCreatedAt())
}

func (s *HeaderSuite) TestMainHeaderV3EncodeDecodeBinaryAt() {
	wh, err := frames.NewHeaderOld(3, 2, 2, 0, 1, 10)
	s.Require().NoError(err)
	wh.SetCreatedAt(100)
	wh.SetChksum(1000)
	b := wh.EncodeBinary()
	_, err = s.rw.Write(b)
	s.Require().NoError(err)

	rh := frames.NewHeaderEmpty()
	err = rh.DecodeBinary(util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Equal(wh.String(), rh.String())
}

func (s *HeaderSuite) TestMainHeaderV4EncodeDecodeBinaryAt() {
	wh, err := frames.NewHeaderOld(4, 2, 2, 0, 1, 10)
	s.Require().NoError(err)
	wh.SetCreatedAt(100)
	wh.SetChksum(1000)
	b := wh.EncodeBinary()
	_, err = s.rw.Write(b)
	s.Require().NoError(err)

	rh := frames.NewHeaderEmpty()
	err = rh.DecodeBinary(util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Equal(wh.String(), rh.String())
}

func (s *HeaderSuite) TestHeaderEncodeDecodeBinaryAt() {
	wh, err := frames.NewHeader(4, 2, 2, 10)
	s.Require().NoError(err)
	wh.SetCreatedAt(100)
	wh.SetChksum(1000)
	b := wh.EncodeBinary()
	_, err = s.rw.Write(b)
	s.Require().NoError(err)

	rh := frames.NewHeaderEmpty()
	err = rh.DecodeBinary(util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Equal(wh.String(), rh.String())
}

func (s *HeaderSuite) TestHeaderEncodeDecodeBinary() {
	wh, err := frames.NewHeader(4, 2, 2, 10)
	wh.SetCreatedAt(100)
	wh.SetChksum(1000)
	s.Require().NoError(err)
	b := wh.EncodeBinary()

	_, err = s.rw.Write(b)
	s.Require().NoError(err)

	_, err = s.rw.Seek(0, 0)
	s.Require().NoError(err)

	rh := frames.NewHeaderEmpty()
	err = rh.DecodeBinary(s.rw)
	s.Require().NoError(err)

	s.Equal(wh.String(), rh.String())
}

func (s *HeaderSuite) TestHeaderEncodeWithErrorVersion() {
	_, err := frames.NewHeader(3, 2, 2, 10)
	s.Require().Error(err)
}

func (s *HeaderSuite) TestHeaderEncodeWithErrorTypeFrame() {
	_, err := frames.NewHeader(4, 2, 20, 10)
	s.Require().Error(err)
}

func (s *HeaderSuite) TestHeaderEncodeWithErrorMaxSize() {
	_, err := frames.NewHeader(4, 2, 2, 201<<20)
	s.Require().Error(err)
}

func (s *HeaderSuite) TestHeaderEncodeWithErrorMinSize() {
	_, err := frames.NewHeader(4, 2, 2, 0)
	s.Require().Error(err)
}

func (s *HeaderSuite) TestHeaderDecodeWithError() {
	wh, err := frames.NewHeader(4, 2, 2, 10)
	s.Require().NoError(err)
	b := wh.EncodeBinary()
	b[2] = 20

	_, err = s.rw.Write(b)
	s.Require().NoError(err)

	_, err = s.rw.Seek(0, 0)
	s.Require().NoError(err)

	rh := frames.NewHeaderEmpty()
	err = rh.DecodeBinary(s.rw)
	s.Require().Error(err)
}

func (s *HeaderSuite) TestHeaderDecodeWithErrorMagicByte() {
	wh, err := frames.NewHeader(4, 2, 2, 10)
	s.Require().NoError(err)
	b := wh.EncodeBinary()
	b[0] = 1

	_, err = s.rw.Write(b)
	s.Require().NoError(err)

	_, err = s.rw.Seek(0, 0)
	s.Require().NoError(err)

	rh := frames.NewHeaderEmpty()
	err = rh.DecodeBinary(s.rw)
	s.Require().Error(err)
}

func (s *HeaderSuite) TestReadHeader() {
	wh, err := frames.NewHeader(4, 2, 2, 10)
	wh.SetCreatedAt(100)
	wh.SetChksum(1000)
	s.Require().NoError(err)
	b := wh.EncodeBinary()

	_, err = s.rw.Write(b)
	s.Require().NoError(err)

	_, err = s.rw.Seek(0, 0)
	s.Require().NoError(err)

	ctx := context.Background()
	rh, err := frames.ReadHeader(ctx, util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Equal(wh.String(), rh.String())
}
