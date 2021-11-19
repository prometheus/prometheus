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
