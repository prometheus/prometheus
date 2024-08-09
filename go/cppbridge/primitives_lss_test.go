package cppbridge_test

import (
	"context"
	"testing"

	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/model/labels"

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
	lss := cppbridge.NewLssStorage()

	s.Equal(uint64(0), lss.AllocatedMemory())
	cp := lss.Pointer()
	s.Require().NotEqual(0, cp)

	lss.Reset()
	newcp := lss.Pointer()
	s.Require().NotEqual(cp, newcp)
}

func (s *LSSSuite) TestOrderedLSS() {
	lss := cppbridge.NewOrderedLssStorage()

	s.Equal(uint64(0), lss.AllocatedMemory())
	cp := lss.Pointer()
	s.Require().NotEqual(0, cp)

	lss.Reset()
	newcp := lss.Pointer()
	s.Require().NotEqual(cp, newcp)
}

func (s *LSSSuite) TestQueryableLSS() {
	lss := cppbridge.NewQueryableLssStorage()

	s.NotEqual(uint64(0), lss.AllocatedMemory())
	cp := lss.Pointer()
	s.Require().NotEqual(0, cp)

	lss.Reset()
	newcp := lss.Pointer()
	s.Require().NotEqual(cp, newcp)
}

func (s *LSSSuite) TestQueryableLSSQuery() {
	lss := cppbridge.NewQueryableLssStorage()

	labelSets := []model.LabelSet{
		model.NewLabelSetBuilder().Set("lol", "kek").Build(),
		model.NewLabelSetBuilder().Set("lol", "kek").Set("che", "bureck").Build(),
		model.NewLabelSetBuilder().Set("foo", "bar").Build(),
		model.NewLabelSetBuilder().Set("foo", "baz").Build(),
	}
	labelSetIDs := make([]uint32, 0, len(labelSets))

	for _, labelSet := range labelSets {
		labelSetIDs = append(labelSetIDs, lss.FindOrEmplace(labelSet))
	}

	// match
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "lol", Value: "kek", MatcherType: model.MatcherTypeExactMatch},
		}
		queryResult := lss.Query(labelMatchers)
		s.Require().Equal(cppbridge.LSSQueryStatusMatch, queryResult.Status())
		s.Require().Len(queryResult.Matches(), 2)
		s.Require().Equal(labelSetIDs[0], queryResult.Matches()[0])
		s.Require().Equal(labelSetIDs[1], queryResult.Matches()[1])
	}

	// no positive matchers
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "kek", Value: "lol", MatcherType: model.MatcherTypeExactNotMatch},
		}
		queryResult := lss.Query(labelMatchers)
		s.Require().Equal(cppbridge.LSSQueryStatusNoPositiveMatchers, queryResult.Status())
	}

	// no match
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "kek", Value: "lol", MatcherType: model.MatcherTypeExactMatch},
		}
		queryResult := lss.Query(labelMatchers)
		s.Require().Equal(cppbridge.LSSQueryStatusNoMatch, queryResult.Status())
	}

	// invalid regexp
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "kek", Value: ".[", MatcherType: model.MatcherTypeRegexpMatch},
		}
		queryResult := lss.Query(labelMatchers)
		s.Require().Equal(cppbridge.LSSQueryStatusRegexpError, queryResult.Status())
	}
}

func (s *LSSSuite) TestQueryableLSSGetLabelSets() {
	lss := cppbridge.NewQueryableLssStorage()

	labelSets := []model.LabelSet{
		model.NewLabelSetBuilder().Set("lol", "kek").Build(),
		model.NewLabelSetBuilder().Set("lol", "kek").Set("che", "bureck").Build(),
		model.NewLabelSetBuilder().Set("foo", "bar").Build(),
		model.NewLabelSetBuilder().Set("foo", "baz").Build(),
	}
	labelSetIDs := make([]uint32, 0, len(labelSets))

	for _, labelSet := range labelSets {
		labelSetIDs = append(labelSetIDs, lss.FindOrEmplace(labelSet))
	}

	fetchedLabelSets := lss.GetLabelSets(labelSetIDs)

	for index, labelSet := range labelSets {
		s.Require().True(isLabelSetEqualsToLabels(labelSet, fetchedLabelSets.LabelsSets()[index]))
	}
}

func isLabelSetEqualsToLabels(labelSet model.LabelSet, labels labels.Labels) bool {
	labelSetString := labelSet.String()
	labelsString := ""
	for _, label := range labels {
		labelsString += label.Name + ":" + label.Value + ";"
	}
	return labelSetString == labelsString
}
