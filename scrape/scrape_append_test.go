// Copyright The Prometheus Authors
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

package scrape

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestBucketLimitAppender(t *testing.T) {
	example := histogram.Histogram{
		Schema:        0,
		Count:         21,
		Sum:           33,
		ZeroThreshold: 0.001,
		ZeroCount:     3,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 3},
		},
		PositiveBuckets: []int64{3, 0, 0},
		NegativeSpans: []histogram.Span{
			{Offset: 0, Length: 3},
		},
		NegativeBuckets: []int64{3, 0, 0},
	}

	bigGap := histogram.Histogram{
		Schema:        0,
		Count:         21,
		Sum:           33,
		ZeroThreshold: 0.001,
		ZeroCount:     3,
		PositiveSpans: []histogram.Span{
			{Offset: 1, Length: 1}, // in (1, 2]
			{Offset: 2, Length: 1}, // in (8, 16]
		},
		PositiveBuckets: []int64{1, 0}, // 1, 1
	}

	customBuckets := histogram.Histogram{
		Schema: histogram.CustomBucketsSchema,
		Count:  9,
		Sum:    33,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 3},
		},
		PositiveBuckets: []int64{3, 0, 0},
		CustomValues:    []float64{1, 2, 3},
	}

	cases := []struct {
		h                 histogram.Histogram
		limit             int
		expectError       bool
		expectBucketCount int
		expectSchema      int32
	}{
		{
			h:           example,
			limit:       3,
			expectError: true,
		},
		{
			h:                 example,
			limit:             4,
			expectError:       false,
			expectBucketCount: 4,
			expectSchema:      -1,
		},
		{
			h:                 example,
			limit:             10,
			expectError:       false,
			expectBucketCount: 6,
			expectSchema:      0,
		},
		{
			h:                 bigGap,
			limit:             1,
			expectError:       false,
			expectBucketCount: 1,
			expectSchema:      -2,
		},
		{
			h:           customBuckets,
			limit:       2,
			expectError: true,
		},
		{
			h:                 customBuckets,
			limit:             3,
			expectError:       false,
			expectBucketCount: 3,
			expectSchema:      histogram.CustomBucketsSchema,
		},
	}

	appTest := teststorage.NewAppender()

	for _, c := range cases {
		for _, floatHisto := range []bool{true, false} {
			t.Run(fmt.Sprintf("floatHistogram=%t", floatHisto), func(t *testing.T) {
				app := &bucketLimitAppender{Appender: appTest.Appender(t.Context()), limit: c.limit}
				ts := int64(10 * time.Minute / time.Millisecond)
				lbls := labels.FromStrings("__name__", "sparse_histogram_series")
				var err error
				if floatHisto {
					fh := c.h.Copy().ToFloat(nil)
					_, err = app.AppendHistogram(0, lbls, ts, nil, fh)
					if c.expectError {
						require.Error(t, err)
					} else {
						require.Equal(t, c.expectSchema, fh.Schema)
						require.Equal(t, c.expectBucketCount, len(fh.NegativeBuckets)+len(fh.PositiveBuckets))
						require.NoError(t, err)
					}
				} else {
					h := c.h.Copy()
					_, err = app.AppendHistogram(0, lbls, ts, h, nil)
					if c.expectError {
						require.Error(t, err)
					} else {
						require.Equal(t, c.expectSchema, h.Schema)
						require.Equal(t, c.expectBucketCount, len(h.NegativeBuckets)+len(h.PositiveBuckets))
						require.NoError(t, err)
					}
				}
				require.NoError(t, app.Commit())
			})
		}
	}
}

func TestMaxSchemaAppender(t *testing.T) {
	example := histogram.Histogram{
		Schema:        0,
		Count:         21,
		Sum:           33,
		ZeroThreshold: 0.001,
		ZeroCount:     3,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 3},
		},
		PositiveBuckets: []int64{3, 0, 0},
		NegativeSpans: []histogram.Span{
			{Offset: 0, Length: 3},
		},
		NegativeBuckets: []int64{3, 0, 0},
	}

	customBuckets := histogram.Histogram{
		Schema: histogram.CustomBucketsSchema,
		Count:  9,
		Sum:    33,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 3},
		},
		PositiveBuckets: []int64{3, 0, 0},
		CustomValues:    []float64{1, 2, 3},
	}

	cases := []struct {
		h            histogram.Histogram
		maxSchema    int32
		expectSchema int32
	}{
		{
			h:            example,
			maxSchema:    -1,
			expectSchema: -1,
		},
		{
			h:            example,
			maxSchema:    0,
			expectSchema: 0,
		},
		{
			h:            customBuckets,
			maxSchema:    -1,
			expectSchema: histogram.CustomBucketsSchema,
		},
	}

	appTest := teststorage.NewAppender()

	for _, c := range cases {
		for _, floatHisto := range []bool{true, false} {
			t.Run(fmt.Sprintf("floatHistogram=%t", floatHisto), func(t *testing.T) {
				app := &maxSchemaAppender{Appender: appTest.Appender(t.Context()), maxSchema: c.maxSchema}
				ts := int64(10 * time.Minute / time.Millisecond)
				lbls := labels.FromStrings("__name__", "sparse_histogram_series")
				var err error
				if floatHisto {
					fh := c.h.Copy().ToFloat(nil)
					_, err = app.AppendHistogram(0, lbls, ts, nil, fh)
					require.Equal(t, c.expectSchema, fh.Schema)
					require.NoError(t, err)
				} else {
					h := c.h.Copy()
					_, err = app.AppendHistogram(0, lbls, ts, h, nil)
					require.Equal(t, c.expectSchema, h.Schema)
					require.NoError(t, err)
				}
				require.NoError(t, app.Commit())
			})
		}
	}
}

// Test sample_limit when a scrape contains Native Histograms.
func TestAppendWithSampleLimitAndNativeHistogram(t *testing.T) {
	appTest := teststorage.NewAppender()

	now := time.Now()
	app := appenderWithLimits(appTest.Appender(t.Context()), 2, 0, 0)

	// sample_limit is set to 2, so first two scrapes should work
	_, err := app.Append(0, labels.FromStrings(model.MetricNameLabel, "foo"), timestamp.FromTime(now), 1)
	require.NoError(t, err)

	// Second sample, should be ok.
	_, err = app.AppendHistogram(
		0,
		labels.FromStrings(model.MetricNameLabel, "my_histogram1"),
		timestamp.FromTime(now),
		&histogram.Histogram{},
		nil,
	)
	require.NoError(t, err)

	// This is third sample with sample_limit=2, it should trigger errSampleLimit.
	_, err = app.AppendHistogram(
		0,
		labels.FromStrings(model.MetricNameLabel, "my_histogram2"),
		timestamp.FromTime(now),
		&histogram.Histogram{},
		nil,
	)
	require.ErrorIs(t, err, errSampleLimit)
}
