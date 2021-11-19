// Copyright 2021 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package labels is deprecated and replaced with github.com/prometheus/prometheus/model/labels.
package labels

import "github.com/prometheus/prometheus/model/labels"

const (
	MetricName     = labels.MetricName
	MatchEqual     = labels.MatchEqual
	MatchNotEqual  = labels.MatchNotEqual
	MatchRegexp    = labels.MatchRegexp
	MatchNotRegexp = labels.MatchNotRegexp
)

var (
	Compare             = labels.Compare
	Equal               = labels.Equal
	NewBuilder          = labels.NewBuilder
	NewFastRegexMatcher = labels.NewFastRegexMatcher
	FromMap             = labels.FromMap
	FromStrings         = labels.FromStrings
	New                 = labels.New
	ReadLabels          = labels.ReadLabels
	MustNewMatcher      = labels.MustNewMatcher
	NewMatcher          = labels.NewMatcher
)

type (
	Builder          = labels.Builder
	FastRegexMatcher = labels.FastRegexMatcher
	Label            = labels.Label
	Labels           = labels.Labels
	MatchType        = labels.MatchType
	Matcher          = labels.Matcher
	Selector         = labels.Selector
	Slice            = labels.Slice
)
