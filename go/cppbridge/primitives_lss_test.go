package cppbridge_test

import (
	"context"
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/stretchr/testify/suite"
)

type LSSSuite struct {
	suite.Suite
	baseCtx context.Context
}

func TestLSSSuite(t *testing.T) {
	suite.Run(t, new(LSSSuite))
}

func (s *LSSSuite) SetupTest() {
	s.baseCtx = context.Background()
}

func (s *LSSSuite) TestLSS() {
	lss, err := cppbridge.NewLabelSetStorage()
	s.Require().NoError(err)

	s.Equal(uint64(0), lss.AllocatedMemory())
	cp := lss.Pointer()
	s.Require().NotEqual(0, cp)

	err = lss.Reset()
	newcp := lss.Pointer()
	s.Require().NoError(err)
	s.Require().NotEqual(cp, newcp)
}

func (s *LSSSuite) TestOrderedLSS() {
	lss, err := cppbridge.NewOrderedLabelSetStorage()
	s.Require().NoError(err)

	s.Equal(uint64(0), lss.AllocatedMemory())
	cp := lss.Pointer()
	s.Require().NotEqual(0, cp)

	err = lss.Reset()
	newcp := lss.Pointer()
	s.Require().NoError(err)
	s.Require().NotEqual(cp, newcp)
}
