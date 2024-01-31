package frames_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/stretchr/testify/suite"
)

// dataTest - test data.
type dataTest struct {
	data []byte
}

func newDataTest(data []byte) *dataTest {
	return &dataTest{
		data: data,
	}
}

// Size returns count of bytes in data
func (dt *dataTest) Size() int64 {
	return int64(len(dt.data))
}

// WriteTo implements io.WriterTo interface
func (dt *dataTest) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(dt.data)
	return int64(n), err
}

func (dt *dataTest) Bytes() []byte {
	return dt.data
}

type SegmentV4Suite struct {
	suite.Suite

	rw *FileBuffer
}

func TestSegmentV4Suite(t *testing.T) {
	suite.Run(t, new(SegmentV4Suite))
}

func (s *SegmentV4Suite) SetupSuite() {
	s.rw = NewFileBuffer()
}

func (s *SegmentV4Suite) TearDownTest() {
	s.rw.Reset()
}

func (s *SegmentV4Suite) TestWriteReadSegmentV4() {
	ctx := context.Background()
	data := newDataTest([]byte(uuid.NewString()))

	ws := frames.NewWriteSegmentV4(1, data)

	_, err := ws.WriteTo(s.rw)
	s.Require().NoError(err)

	rs := frames.NewReadSegmentV4Empty()
	err = rs.Read(ctx, util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Equal(ws.SentAt, rs.SentAt)
	s.Equal(ws.ID, rs.ID)
	s.Equal(ws.Size, rs.Size)
	s.Equal(ws.CRC, rs.CRC)
	s.Equal(data.Bytes(), rs.Body)
}

func (s *SegmentV4Suite) TestWriteReadSegmentV4Nil() {
	ctx := context.Background()
	ws := frames.NewWriteSegmentV4(2, nil)

	_, err := ws.WriteTo(s.rw)
	s.Require().NoError(err)

	rs := frames.NewReadSegmentV4Empty()
	err = rs.Read(ctx, util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Equal(ws.SentAt, rs.SentAt)
	s.Equal(ws.ID, rs.ID)
	s.Equal(ws.Size, rs.Size)
}

func (s *SegmentV4Suite) TestWriteReadResponseV4() {
	ctx := context.Background()

	wf := frames.NewResponseV4(
		time.Now().UnixNano(),
		1,
		200,
		uuid.NewString(),
	)

	_, err := wf.WriteTo(s.rw)
	s.Require().NoError(err)

	rf := frames.NewResponseV4Empty()
	err = rf.Read(ctx, util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Equal(wf.SentAt, rf.SentAt)
	s.Equal(wf.SegmentID, rf.SegmentID)
	s.Equal(wf.Code, rf.Code)
	s.Equal(wf.Text, rf.Text)
	s.Equal(*wf, *rf)
}

func (s *SegmentV4Suite) TestWriteReadResponseV4Nil() {
	ctx := context.Background()

	wf := frames.NewResponseV4(
		time.Now().UnixNano(),
		1,
		200,
		"",
	)

	_, err := wf.WriteTo(s.rw)
	s.Require().NoError(err)

	rf := frames.NewResponseV4Empty()
	err = rf.Read(ctx, util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Equal(wf.SentAt, rf.SentAt)
	s.Equal(wf.SegmentID, rf.SegmentID)
	s.Equal(wf.Code, rf.Code)
	s.Equal(wf.Text, rf.Text)
	s.Equal(*wf, *rf)
}

func (s *SegmentV4Suite) TestWriteReadWriteRefillSegmentV4() {
	ctx := context.Background()
	data := newDataTest([]byte(uuid.NewString()))

	ws := frames.NewWriteRefillSegmentV4(1, data)

	_, err := ws.WriteTo(s.rw)
	s.Require().NoError(err)

	rs := frames.NewReadRefillSegmentV4Empty()
	err = rs.Read(ctx, util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Equal(ws.ID, rs.ID)
	s.Equal(ws.Size, rs.Size)
	s.Equal(ws.CRC, rs.CRC)
	s.Equal(data.Bytes(), rs.Body)
}

func (s *SegmentV4Suite) TestWriteReadRefillSegmentV4Nil() {
	ctx := context.Background()
	ws := frames.NewWriteRefillSegmentV4(2, nil)

	_, err := ws.WriteTo(s.rw)
	s.Require().NoError(err)

	rs := frames.NewReadRefillSegmentV4Empty()
	err = rs.Read(ctx, util.NewOffsetReader(s.rw, 0))
	s.Require().NoError(err)

	s.Equal(ws.ID, rs.ID)
	s.Equal(ws.Size, rs.Size)
}
