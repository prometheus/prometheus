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

const EntryInvalid = textparse.EntryInvalid
const EntryType = textparse.EntryType
const EntryHelp = textparse.EntryHelp
const EntrySeries = textparse.EntrySeries
const EntryComment = textparse.EntryComment
const EntryUnit = textparse.EntryUnit

const MetricTypeCounter = textparse.MetricTypeCounter
const MetricTypeGauge = textparse.MetricTypeGauge
const MetricTypeHistogram = textparse.MetricTypeHistogram
const MetricTypeGaugeHistogram = textparse.MetricTypeGaugeHistogram
const MetricTypeSummary = textparse.MetricTypeSummary
const MetricTypeInfo = textparse.MetricTypeInfo
const MetricTypeStateset = textparse.MetricTypeStateset
const MetricTypeUnknown = textparse.MetricTypeUnknown

type Entry = textparse.Entry
type MetricType = textparse.MetricType
type OpenMetricsParser = textparse.OpenMetricsParser
type Parser = textparse.Parser
type PromParser = textparse.PromParser

var New = textparse.New
var NewOpenMetricsParser = textparse.NewOpenMetricsParser
var NewPromParser = textparse.NewPromParser
