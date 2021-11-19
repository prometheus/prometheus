package labels

import "github.com/prometheus/prometheus/model/labels"

const MetricName = labels.MetricName
const MatchEqual = labels.MatchEqual
const MatchNotEqual = labels.MatchNotEqual
const MatchRegexp = labels.MatchRegexp
const MatchNotRegexp = labels.MatchNotRegexp

var Compare = labels.Compare
var Equal = labels.Equal
var NewBuilder = labels.NewBuilder
var NewFastRegexMatcher = labels.NewFastRegexMatcher
var FromMap = labels.FromMap
var FromStrings = labels.FromStrings
var New = labels.New
var ReadLabels = labels.ReadLabels
var MustNewMatcher = labels.MustNewMatcher
var NewMatcher = labels.NewMatcher

type Builder = labels.Builder
type FastRegexMatcher = labels.FastRegexMatcher
type Label = labels.Label
type Labels = labels.Labels
type MatchType = labels.MatchType
type Matcher = labels.Matcher
type Selector = labels.Selector
type Slice = labels.Slice
