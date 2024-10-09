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
	"errors"
	"mime"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
)

// Parser parses samples from a byte slice of samples in the official
// Prometheus and OpenMetrics text exposition formats.
type Parser interface {
	// Series returns the bytes of a series with a simple float64 as a
	// value, the timestamp if set, and the value of the current sample.
	Series() ([]byte, *int64, float64)

	// Histogram returns the bytes of a series with a sparse histogram as a
	// value, the timestamp if set, and the histogram in the current sample.
	// Depending on the parsed input, the function returns an (integer) Histogram
	// or a FloatHistogram, with the respective other return value being nil.
	Histogram() ([]byte, *int64, *histogram.Histogram, *histogram.FloatHistogram)

	// Help returns the metric name and help text in the current entry.
	// Must only be called after Next returned a help entry.
	// The returned byte slices become invalid after the next call to Next.
	Help() ([]byte, []byte)

	// Type returns the metric name and type in the current entry.
	// Must only be called after Next returned a type entry.
	// The returned byte slices become invalid after the next call to Next.
	Type() ([]byte, model.MetricType)

	// Unit returns the metric name and unit in the current entry.
	// Must only be called after Next returned a unit entry.
	// The returned byte slices become invalid after the next call to Next.
	Unit() ([]byte, []byte)

	// Comment returns the text of the current comment.
	// Must only be called after Next returned a comment entry.
	// The returned byte slice becomes invalid after the next call to Next.
	Comment() []byte

	// Metric writes the labels of the current sample into the passed labels.
	// It returns the string from which the metric was parsed.
	Metric(l *labels.Labels) string

	// Exemplar writes the exemplar of the current sample into the passed
	// exemplar. It can be called repeatedly to retrieve multiple exemplars
	// for the same sample. It returns false once all exemplars are
	// retrieved (including the case where no exemplars exist at all).
	Exemplar(l *exemplar.Exemplar) bool

	// CreatedTimestamp returns the created timestamp (in milliseconds) for the
	// current sample. It returns nil if it is unknown e.g. if it wasn't set,
	// if the scrape protocol or metric type does not support created timestamps.
	// Assume the CreatedTimestamp returned pointer is only valid until
	// the Next iteration.
	CreatedTimestamp() *int64

	// Next advances the parser to the next sample.
	// It returns (EntryInvalid, io.EOF) if no samples were read.
	Next() (Entry, error)
}

// New returns a new parser of the byte slice.
//
// This function always returns a valid parser, but might additionally
// return an error if the content type cannot be parsed.
func NewFallback(b []byte, contentType string, fallbackType string, parseClassicHistograms, skipOMCTSeries bool, st *labels.SymbolTable) (Parser, error) {
	var contentTypeErr error

	if contentType == "" {
		if fallbackType == "" {
			// No fallbackType set, so we default to standard NewPromParser()
			return NewPromParser(b, st), nil
		}
		// Otherwise we fall through and override the contentType
		contentType = fallbackType
		contentTypeErr = errors.New("Non-compliant scraper sending blank Content-Type")
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return NewPromParser(b, st), err
	}
	switch mediaType {
	case "application/openmetrics-text":
		return NewOpenMetricsParser(b, st, func(o *openMetricsParserOptions) {
			o.SkipCTSeries = skipOMCTSeries
		}), contentTypeErr
	case "application/vnd.google.protobuf":
		return NewProtobufParser(b, parseClassicHistograms, st), contentTypeErr
	case "text/plain":
		return NewPromParser(b, st), contentTypeErr
	default:
		if fallbackType == "" {
			// With no fallbackType provided we default to NewPromParser() without an error
			return NewPromParser(b, st), nil
		}

		// If we get here we have specified a fallback format but it is not recognised
		err = errors.New("Unrecognised `scrape.fallback-scrape-format` value")
		// TODO(alexg): Should we validate scrape.fallback-scrape-format at startup/config-parse?
		return NewPromParser(b, st), err
	}
}

func New(b []byte, contentType string, parseClassicHistograms, skipOMCTSeries bool, st *labels.SymbolTable) (Parser, error) {
	// TODO(alexg): What should be the default value we pass here, `text/plain` or empty?
	return NewFallback(b, contentType, "", parseClassicHistograms, skipOMCTSeries, st)
}

// Entry represents the type of a parsed entry.
type Entry int

const (
	EntryInvalid   Entry = -1
	EntryType      Entry = 0
	EntryHelp      Entry = 1
	EntrySeries    Entry = 2 // EntrySeries marks a series with a simple float64 as value.
	EntryComment   Entry = 3
	EntryUnit      Entry = 4
	EntryHistogram Entry = 5 // EntryHistogram marks a series with a native histogram as a value.
)
