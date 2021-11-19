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

// Package textparse is deprecated and replaced with github.com/prometheus/prometheus/model/textparse.
package textparse

import "github.com/prometheus/prometheus/model/textparse"

const (
	EntryInvalid = textparse.EntryInvalid
	EntryType    = textparse.EntryType
	EntryHelp    = textparse.EntryHelp
	EntrySeries  = textparse.EntrySeries
	EntryComment = textparse.EntryComment
	EntryUnit    = textparse.EntryUnit
)

const (
	MetricTypeCounter        = textparse.MetricTypeCounter
	MetricTypeGauge          = textparse.MetricTypeGauge
	MetricTypeHistogram      = textparse.MetricTypeHistogram
	MetricTypeGaugeHistogram = textparse.MetricTypeGaugeHistogram
	MetricTypeSummary        = textparse.MetricTypeSummary
	MetricTypeInfo           = textparse.MetricTypeInfo
	MetricTypeStateset       = textparse.MetricTypeStateset
	MetricTypeUnknown        = textparse.MetricTypeUnknown
)

type (
	Entry             = textparse.Entry
	MetricType        = textparse.MetricType
	OpenMetricsParser = textparse.OpenMetricsParser
	Parser            = textparse.Parser
	PromParser        = textparse.PromParser
)

var (
	New                  = textparse.New
	NewOpenMetricsParser = textparse.NewOpenMetricsParser
	NewPromParser        = textparse.NewPromParser
)
