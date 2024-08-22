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

type QueryableLSSSuite struct {
	suite.Suite
	baseCtx     context.Context
	lss         *cppbridge.LabelSetStorage
	labelSets   []model.LabelSet
	labelSetIDs []uint32
}

func TestQueryableLSSSuite(t *testing.T) {
	suite.Run(t, new(QueryableLSSSuite))
}

func (s *QueryableLSSSuite) SetupTest() {
	s.baseCtx = context.Background()
	s.lss = cppbridge.NewQueryableLssStorage()

	s.labelSets = []model.LabelSet{
		model.NewLabelSetBuilder().Set("lol", "kek").Build(),
		model.NewLabelSetBuilder().Set("lol", "kek").Set("che", "bureck").Build(),
		model.NewLabelSetBuilder().Set("foo", "bar").Build(),
		model.NewLabelSetBuilder().Set("foo", "baz").Build(),
	}

	s.labelSetIDs = make([]uint32, 0, len(s.labelSets))
	for _, labelSet := range s.labelSets {
		s.labelSetIDs = append(s.labelSetIDs, s.lss.FindOrEmplace(labelSet))
	}
}

func (s *QueryableLSSSuite) TestQuery() {
	// match
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "lol", Value: "kek", MatcherType: model.MatcherTypeExactMatch},
		}
		queryResult := s.lss.Query(labelMatchers)
		s.Require().Equal(cppbridge.LSSQueryStatusMatch, queryResult.Status())
		s.Require().Len(queryResult.Matches(), 2)
		s.Require().Equal(s.labelSetIDs[0], queryResult.Matches()[0])
		s.Require().Equal(s.labelSetIDs[1], queryResult.Matches()[1])
	}

	// no positive matchers
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "kek", Value: "lol", MatcherType: model.MatcherTypeExactNotMatch},
		}
		queryResult := s.lss.Query(labelMatchers)
		s.Require().Equal(cppbridge.LSSQueryStatusNoPositiveMatchers, queryResult.Status())
	}

	// no match
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "kek", Value: "lol", MatcherType: model.MatcherTypeExactMatch},
		}
		queryResult := s.lss.Query(labelMatchers)
		s.Require().Equal(cppbridge.LSSQueryStatusNoMatch, queryResult.Status())
	}

	// invalid regexp
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "kek", Value: ".[", MatcherType: model.MatcherTypeRegexpMatch},
		}
		queryResult := s.lss.Query(labelMatchers)
		s.Require().Equal(cppbridge.LSSQueryStatusRegexpError, queryResult.Status())
	}
}

func (s *QueryableLSSSuite) TestGetLabelSets() {
	fetchedLabelSets := s.lss.GetLabelSets(s.labelSetIDs)

	for index, labelSet := range s.labelSets {
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

type queryLabelNameCase struct {
	matchers        []model.LabelMatcher
	expected_status uint32
	expected_names  []string
}

var queryLabelNamesCases = []queryLabelNameCase{
	{
		matchers:        []model.LabelMatcher{},
		expected_status: cppbridge.LSSQueryStatusMatch,
		expected_names:  []string{"che", "foo", "lol"},
	},
	{
		matchers:        []model.LabelMatcher{{Name: "lol", Value: ".+", MatcherType: model.MatcherTypeRegexpMatch}},
		expected_status: cppbridge.LSSQueryStatusMatch,
		expected_names:  []string{"che", "lol"},
	},
}

func (s *QueryableLSSSuite) TestQueryLabelNames() {
	for _, test_case := range queryLabelNamesCases {
		s.testQueryLabelNamesImpl(test_case)
	}
}

func (s *QueryableLSSSuite) testQueryLabelNamesImpl(test_case queryLabelNameCase) {
	// Arrange

	// Act
	result := s.lss.QueryLabelNames(test_case.matchers)

	// Assert
	s.Equal(test_case.expected_status, result.Status())
	s.Equal(test_case.expected_names, result.Names())
}

type queryLabelValuesCase struct {
	label_name      string
	matchers        []model.LabelMatcher
	expected_status uint32
	expected_values []string
}

var queryLabelValuesCases = []queryLabelValuesCase{
	{
		label_name:      "foo",
		matchers:        []model.LabelMatcher{},
		expected_status: cppbridge.LSSQueryStatusMatch,
		expected_values: []string{"bar", "baz"},
	},
	{
		label_name:      "foo",
		matchers:        []model.LabelMatcher{{Name: "foo", Value: ".+", MatcherType: model.MatcherTypeRegexpMatch}},
		expected_status: cppbridge.LSSQueryStatusMatch,
		expected_values: []string{"bar", "baz"},
	},
}

func (s *QueryableLSSSuite) TestQueryLabelValues() {
	for _, test_case := range queryLabelValuesCases {
		s.testQueryLabelValuesImpl(test_case)
	}
}

func (s *QueryableLSSSuite) testQueryLabelValuesImpl(test_case queryLabelValuesCase) {
	// Arrange

	// Act
	result := s.lss.QueryLabelValues(test_case.label_name, test_case.matchers)

	// Assert
	s.Equal(test_case.expected_status, result.Status())
	s.Equal(test_case.expected_values, result.Values())
}
