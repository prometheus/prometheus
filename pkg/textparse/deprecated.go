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
