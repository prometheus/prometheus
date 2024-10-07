// Copyright 2018 The Prometheus Authors
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

package textparse

import (
	//"mime"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
)

// Parser parses samples from a byte slice of samples in the official
// Prometheus and OpenMetrics text exposition formats.
type Parser interface {
	// Parse returns the next metric with all information collected about it, such as
	// metric type, help text, unit, created timestamps, samples, etc.
	Next(d DropperCache, keepClassicHistogramSeries bool) (interface{}, error)
}

type DropperCache interface {
	Get(rawSeriesId []byte) (isDropped, isKnown bool)
	Set(rawSeriesId []byte, lbls labels.Labels) (isDropped bool)
}

// New returns a new parser of the byte slice.
//
// This function always returns a valid parser, but might additionally
// return an error if the content type cannot be parsed.
func New(b []byte, contentType string, parseClassicHistograms bool, st *labels.SymbolTable) (Parser, error) {
	if contentType == "" {
		return NewPromParser(b, st), nil
	}

	// mediaType, _, err := mime.ParseMediaType(contentType)
	// if err != nil {
	// 	return NewPromParser(b, st), err
	// }
	// switch mediaType {
	// case "application/openmetrics-text":
	// 	return NewOpenMetricsParser(b, st), nil
	// case "application/vnd.google.protobuf":
	// 	return NewProtobufParser(b, parseClassicHistograms, st), nil
	// default:
	// 	return NewPromParser(b, st), nil
	// }

	return NewPromParser(b, st), nil
}

type BaseExposedMetric interface {
	Name() string
	// Labels() labels.Labels
	Help() (string, bool)
	// Unit() (string, bool)
	// Timestamp() (int64, bool)
	// CreatedTimestamp() (int64, bool)
	// // A uniq identifier for the metric for putting into the dropper cache.
	// RawSeriesId() []byte
	// IsGauge() bool
	// Exemplars() []*exemplar.Exemplar
}

// Untyped float value
type FloatMetric interface {
	BaseExposedMetric
	Value() float64
}

type FloatCounterMetric interface {
	BaseExposedMetric
	Value() float64
}

type FloatGaugeMetric interface {
	BaseExposedMetric
	Value() float64
}

type HistogramCounterMetric interface {
	BaseExposedMetric
	SumValue() float64
	CountValue() float64
	CustomBuckets() bool
	Buckets() ExposedBucketIterator
	Native() (*histogram.Histogram, *histogram.FloatHistogram)
}

type HistogramGaugeMetric interface {
	BaseExposedMetric
	SumValue() float64
	CountValue() float64
	CustomBuckets() bool
	Buckets() ExposedBucketIterator
	Native() (*histogram.Histogram, *histogram.FloatHistogram)
}

type SummaryMetric interface {
	BaseExposedMetric
	SumValue() float64
	CountValue() float64
	Quantiles() ExposedQuantileIterator
}

type ExposedBucketIterator interface {
	Next() bool
	At() ExposedBucket
}

type ExposedBucket struct {
	UpperBound float64
	Count      float64
	Exemplar   *exemplar.Exemplar
}

type ExposedQuantileIterator interface {
	Next() bool
	At() ExposedQuantile
}

type ExposedQuantile struct {
	Quantile float64
	Value    float64
}


// // ExposedValues holds the values of a metric, the purpose is to group the values in one place and reuse the memory as much as possible.
// type ExposedValues struct {
// 	flags ExposureFlags
// 	timestamp int64
// 	createdTimestamp int64
// 	sum    float64  // For Counter value, Gauge value, Histogram Sum, Summary Sum and Unknown value.
// 	count  float64	// For Histogram Count, Summary Count.

//   counts []ExposedBoundaryCount   // For classic histogram bucket counts (cumulative) and summary quantile values.

// 	h  *histogram.Histogram  // For native histogram. There is no hasH as this is a pointer that can be nil.
// 	fh *histogram.FloatHistogram  // For native float histogram. There is no hasFH as this is a pointer that can be nil.
// 	// For native histograms and eventually summaries.
// 	exemplars []exemplar.Exemplar

// 	help string
// 	unit string
// }

// type ExposedBoundaryCount struct {
// 	store bool // Whether to store this bucket count value (according to relabel drop rules).
// 	boundary float64
// 	count uint64
// 	hasExemplar bool
// 	exemplar exemplar.Exemplar
// }

// type ExposureFlags uint64

// const (
// 	ExposureFlagHasTimestamp = 1 << iota
// 	ExposureFlagHasCreatedTimestamp
// 	ExposureFlagHasSum
// 	ExposureFlagStoreSum
// 	ExposureFlagHasCount
// 	ExposureFlagStoreCount
// 	ExposureFlagHasHelp
// 	ExposureFlagHasUnit

// 	// No need to have flag for native histograms, we can just check if h or fh is nil.
// )

// // Reset values to zero, reuse memory if possible.
// // For native histograms, the commit will use the value so we need to reset to nil.
// func (v *ExposedValues) Reset() {
// 	v.flags = 0
// 	v.h = nil
// 	v.fh = nil
// 	v.counts = v.counts[:0]
// 	v.exemplars = v.exemplars[:0]
// }

// func (v *ExposedValues) SetTimestamp(t int64) {
// 	v.timestamp = t
// 	v.flags |= ExposureFlagHasTimestamp
// }

// func (v *ExposedValues) SetCreatedTimestamp(t int64) {
// 	v.createdTimestamp = t
// 	v.flags |= ExposureFlagHasCreatedTimestamp
// }