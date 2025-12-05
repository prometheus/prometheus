// Copyright 2017 The Prometheus Authors
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

package tsdb

import (
	"testing"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

const (
	float                       = "float"
	intHistogram                = "integer histogram"
	floatHistogram              = "float histogram"
	customBucketsIntHistogram   = "custom buckets int histogram"
	customBucketsFloatHistogram = "custom buckets float histogram"
	gaugeIntHistogram           = "gauge int histogram"
	gaugeFloatHistogram         = "gauge float histogram"
)

type testValue struct {
	Ts                 int64
	V                  int64
	CounterResetHeader histogram.CounterResetHint
}

type sampleTypeScenario struct {
	sampleType string
	appendFunc func(appender storage.LimitedAppenderV1, lbls labels.Labels, ts, value int64) (storage.SeriesRef, sample, error)
	sampleFunc func(ts, value int64) sample
}

var sampleTypeScenarios = map[string]sampleTypeScenario{
	float: {
		sampleType: sampleMetricTypeFloat,
		appendFunc: func(appender storage.LimitedAppenderV1, lbls labels.Labels, ts, value int64) (storage.SeriesRef, sample, error) {
			s := sample{t: ts, f: float64(value)}
			ref, err := appender.Append(0, lbls, ts, s.f)
			return ref, s, err
		},
		sampleFunc: func(ts, value int64) sample {
			return sample{t: ts, f: float64(value)}
		},
	},
	intHistogram: {
		sampleType: sampleMetricTypeHistogram,
		appendFunc: func(appender storage.LimitedAppenderV1, lbls labels.Labels, ts, value int64) (storage.SeriesRef, sample, error) {
			s := sample{t: ts, h: tsdbutil.GenerateTestHistogram(value)}
			ref, err := appender.AppendHistogram(0, lbls, ts, s.h, nil)
			return ref, s, err
		},
		sampleFunc: func(ts, value int64) sample {
			return sample{t: ts, h: tsdbutil.GenerateTestHistogram(value)}
		},
	},
	floatHistogram: {
		sampleType: sampleMetricTypeHistogram,
		appendFunc: func(appender storage.LimitedAppenderV1, lbls labels.Labels, ts, value int64) (storage.SeriesRef, sample, error) {
			s := sample{t: ts, fh: tsdbutil.GenerateTestFloatHistogram(value)}
			ref, err := appender.AppendHistogram(0, lbls, ts, nil, s.fh)
			return ref, s, err
		},
		sampleFunc: func(ts, value int64) sample {
			return sample{t: ts, fh: tsdbutil.GenerateTestFloatHistogram(value)}
		},
	},
	customBucketsIntHistogram: {
		sampleType: sampleMetricTypeHistogram,
		appendFunc: func(appender storage.LimitedAppenderV1, lbls labels.Labels, ts, value int64) (storage.SeriesRef, sample, error) {
			s := sample{t: ts, h: tsdbutil.GenerateTestCustomBucketsHistogram(value)}
			ref, err := appender.AppendHistogram(0, lbls, ts, s.h, nil)
			return ref, s, err
		},
		sampleFunc: func(ts, value int64) sample {
			return sample{t: ts, h: tsdbutil.GenerateTestCustomBucketsHistogram(value)}
		},
	},
	customBucketsFloatHistogram: {
		sampleType: sampleMetricTypeHistogram,
		appendFunc: func(appender storage.LimitedAppenderV1, lbls labels.Labels, ts, value int64) (storage.SeriesRef, sample, error) {
			s := sample{t: ts, fh: tsdbutil.GenerateTestCustomBucketsFloatHistogram(value)}
			ref, err := appender.AppendHistogram(0, lbls, ts, nil, s.fh)
			return ref, s, err
		},
		sampleFunc: func(ts, value int64) sample {
			return sample{t: ts, fh: tsdbutil.GenerateTestCustomBucketsFloatHistogram(value)}
		},
	},
	gaugeIntHistogram: {
		sampleType: sampleMetricTypeHistogram,
		appendFunc: func(appender storage.LimitedAppenderV1, lbls labels.Labels, ts, value int64) (storage.SeriesRef, sample, error) {
			s := sample{t: ts, h: tsdbutil.GenerateTestGaugeHistogram(value)}
			ref, err := appender.AppendHistogram(0, lbls, ts, s.h, nil)
			return ref, s, err
		},
		sampleFunc: func(ts, value int64) sample {
			return sample{t: ts, h: tsdbutil.GenerateTestGaugeHistogram(value)}
		},
	},
	gaugeFloatHistogram: {
		sampleType: sampleMetricTypeHistogram,
		appendFunc: func(appender storage.LimitedAppenderV1, lbls labels.Labels, ts, value int64) (storage.SeriesRef, sample, error) {
			s := sample{t: ts, fh: tsdbutil.GenerateTestGaugeFloatHistogram(value)}
			ref, err := appender.AppendHistogram(0, lbls, ts, nil, s.fh)
			return ref, s, err
		},
		sampleFunc: func(ts, value int64) sample {
			return sample{t: ts, fh: tsdbutil.GenerateTestGaugeFloatHistogram(value)}
		},
	},
}

// requireEqualSeries checks that the actual series are equal to the expected ones. It ignores the counter reset hints for histograms.
func requireEqualSeries(t *testing.T, expected, actual map[string][]chunks.Sample, ignoreCounterResets bool) {
	for name, expectedItem := range expected {
		actualItem, ok := actual[name]
		require.True(t, ok, "Expected series %s not found", name)
		if ignoreCounterResets {
			requireEqualSamples(t, name, expectedItem, actualItem, requireEqualSamplesIgnoreCounterResets)
		} else {
			requireEqualSamples(t, name, expectedItem, actualItem)
		}
	}
	for name := range actual {
		_, ok := expected[name]
		require.True(t, ok, "Unexpected series %s", name)
	}
}

func requireEqualOOOSamples(t *testing.T, expectedSamples int, db *DB) {
	require.Equal(t, float64(expectedSamples),
		prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamplesAppended.WithLabelValues(sampleMetricTypeFloat))+
			prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamplesAppended.WithLabelValues(sampleMetricTypeHistogram)),
		"number of ooo appended samples mismatch")
}

type requireEqualSamplesOption int

const (
	requireEqualSamplesNoOption requireEqualSamplesOption = iota
	requireEqualSamplesIgnoreCounterResets
	requireEqualSamplesInUseBucketCompare
)

func requireEqualSamples(t *testing.T, name string, expected, actual []chunks.Sample, options ...requireEqualSamplesOption) {
	var (
		ignoreCounterResets bool
		inUseBucketCompare  bool
	)
	for _, option := range options {
		switch option {
		case requireEqualSamplesIgnoreCounterResets:
			ignoreCounterResets = true
		case requireEqualSamplesInUseBucketCompare:
			inUseBucketCompare = true
		}
	}

	require.Len(t, actual, len(expected), "Length not equal to expected for %s", name)
	for i, s := range expected {
		expectedSample := s
		actualSample := actual[i]
		require.Equal(t, expectedSample.T(), actualSample.T(), "Different timestamps for %s[%d]", name, i)
		require.Equal(t, expectedSample.Type().String(), actualSample.Type().String(), "Different types for %s[%d] at ts %d", name, i, expectedSample.T())
		switch {
		case s.H() != nil:
			{
				expectedHist := expectedSample.H()
				actualHist := actualSample.H()
				if ignoreCounterResets && expectedHist.CounterResetHint != histogram.GaugeType {
					expectedHist.CounterResetHint = histogram.UnknownCounterReset
					actualHist.CounterResetHint = histogram.UnknownCounterReset
				} else {
					require.Equal(t, expectedHist.CounterResetHint, actualHist.CounterResetHint, "Sample header doesn't match for %s[%d] at ts %d, expected: %s, actual: %s", name, i, expectedSample.T(), counterResetAsString(expectedHist.CounterResetHint), counterResetAsString(actualHist.CounterResetHint))
				}
				if inUseBucketCompare {
					expectedSample.H().Compact(0)
					actualSample.H().Compact(0)
				}
				require.Equal(t, expectedHist, actualHist, "Sample doesn't match for %s[%d] at ts %d", name, i, expectedSample.T())
			}
		case s.FH() != nil:
			{
				expectedHist := expectedSample.FH()
				actualHist := actualSample.FH()
				if ignoreCounterResets {
					expectedHist.CounterResetHint = histogram.UnknownCounterReset
					actualHist.CounterResetHint = histogram.UnknownCounterReset
				} else {
					require.Equal(t, expectedHist.CounterResetHint, actualHist.CounterResetHint, "Sample header doesn't match for %s[%d] at ts %d, expected: %s, actual: %s", name, i, expectedSample.T(), counterResetAsString(expectedHist.CounterResetHint), counterResetAsString(actualHist.CounterResetHint))
				}
				if inUseBucketCompare {
					expectedSample.FH().Compact(0)
					actualSample.FH().Compact(0)
				}
				require.Equal(t, expectedHist, actualHist, "Sample doesn't match for %s[%d] at ts %d", name, i, expectedSample.T())
			}
		default:
			expectedFloat := expectedSample.F()
			actualFloat := actualSample.F()
			require.Equal(t, expectedFloat, actualFloat, "Sample doesn't match for %s[%d] at ts %d", name, i, expectedSample.T())
		}
	}
}

func counterResetAsString(h histogram.CounterResetHint) string {
	switch h {
	case histogram.UnknownCounterReset:
		return "UnknownCounterReset"
	case histogram.CounterReset:
		return "CounterReset"
	case histogram.NotCounterReset:
		return "NotCounterReset"
	case histogram.GaugeType:
		return "GaugeType"
	}
	panic("Unexpected counter reset type")
}
