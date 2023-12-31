// Copyright 2023 The Prometheus Authors
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

package main

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/regexp"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
)

var (
	errNotNativeHistogram = fmt.Errorf("not a native histogram")
	errNotEnoughData      = fmt.Errorf("not enough data")

	sumSuffix = regexp.MustCompile(`_sum\b`)
)

type QueryAnalyzeConfig struct {
	metricType string
	duration   time.Duration
	time       string
	matchers   []string
}

// run retrieves metrics that look like conventional histograms (i.e. have _bucket
// suffixes) or native histograms, depending on metricType flag.
func (c *QueryAnalyzeConfig) run(url *url.URL, roundtripper http.RoundTripper) error {
	if c.metricType != "classichistograms" && c.metricType != "nativehistograms" {
		return fmt.Errorf("analyze type is %s, must be 'classichistograms' or 'nativehistograms'", c.metricType)
	}

	ctx := context.Background()

	api, err := newAPI(url, roundtripper, nil)
	if err != nil {
		return err
	}

	var endTime time.Time
	if c.time != "" {
		endTime, err = parseTime(c.time)
		if err != nil {
			return fmt.Errorf("error parsing time '%s': %w", c.time, err)
		}
	} else {
		endTime = time.Now()
	}

	if c.metricType == "nativehistograms" {
		return c.getStatsFromMetrics(ctx, api, endTime, os.Stdout, identity, true, c.matchers)
	}
	for _, matcher := range c.matchers {
		if !sumSuffix.MatchString(matcher) {
			return fmt.Errorf("every classic histogram matcher must have a '_sum' suffix in the metric name, but the matcher '%s' does not", matcher)
		}
	}
	return c.getStatsFromMetrics(ctx, api, endTime, os.Stdout, replaceSumWithBucket, false, c.matchers)
}

type transformNameFunc func(name string) string

func identity(name string) string {
	return name
}

func replaceSumWithBucket(name string) string {
	return sumSuffix.ReplaceAllString(name, "_bucket")
}

func (c *QueryAnalyzeConfig) getStatsFromMetrics(ctx context.Context, api v1.API, endTime time.Time, out io.Writer, transformNameForSeriesSel transformNameFunc, isNative bool, matchers []string) error {
	metastats := newMetaStatistics()
	metahistogram := newStatsHistogram()
	if isNative {
		for _, matcher := range matchers {
			seriesSel := seriesSelector(matcher, c.duration)
			matrix, err := querySamples(ctx, api, seriesSel, endTime)
			if err != nil {
				return err
			}
			for _, series := range matrix {
				stats, err := calcNativeBucketStatistics(series, metahistogram)
				if err != nil {
					if err == errNotNativeHistogram || err == errNotEnoughData {
						continue
					}
					return err
				}
				fmt.Fprintf(out, "%s=%v\n", series.Metric, *stats)
				metastats.update(stats)
			}
		}
	} else {
		for _, matcher := range matchers {
			series, err := queryLabelSets(ctx, api, matcher, endTime, c.duration)
			if err != nil {
				return err
			}
			for _, sample := range series {
				// Get labels to match.
				lbs := model.LabelSet(sample.Metric)
				metricName := string(lbs[labels.MetricName])
				delete(lbs, labels.MetricName)
				seriesSel := seriesSelectorWithLabels(transformNameForSeriesSel(metricName), lbs, c.duration)
				matrix, err := querySamples(ctx, api, seriesSel, endTime)
				if err != nil {
					return err
				}
				stats, err := calcClassicBucketStatistics(matrix, metahistogram)
				if err != nil {
					if err == errNotEnoughData {
						continue
					}
					return err
				}
				fmt.Fprintf(out, "%s=%v\n", seriesSel, *stats)
				metastats.update(stats)
			}
		}
	}
	fmt.Println(metastats)
	fmt.Println(metahistogram)
	return nil
}

// Query the last_over_time(*_sum[duration]) series related to the matcher,
// keeping the result small (avoids buckets).
func queryLabelSets(ctx context.Context, api v1.API, metricName string, end time.Time, duration time.Duration) (model.Vector, error) {
	query := fmt.Sprintf("last_over_time(%s[%s])", metricName, duration.String())

	values, _, err := api.Query(ctx, query, end)
	if err != nil {
		return nil, err
	}

	series, ok := values.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("query for metrics resulted in non-Vector")
	}
	return series, nil
}

func seriesSelector(metricName string, duration time.Duration) string {
	builder := strings.Builder{}
	builder.WriteString(metricName)
	builder.WriteRune('[')
	builder.WriteString(duration.String())
	builder.WriteRune(']')

	return builder.String()
}

func seriesSelectorWithLabels(metricName string, lbs model.LabelSet, duration time.Duration) string {
	builder := strings.Builder{}
	builder.WriteString(metricName)
	builder.WriteRune('{')
	first := true
	for l, v := range lbs {
		if first {
			first = false
		} else {
			builder.WriteRune(',')
		}
		builder.WriteString(string(l))
		builder.WriteString("=\"")
		builder.WriteString(string(v))
		builder.WriteRune('"')
	}
	builder.WriteRune('}')
	builder.WriteRune('[')
	builder.WriteString(duration.String())
	builder.WriteRune(']')

	return builder.String()
}

func querySamples(ctx context.Context, api v1.API, query string, end time.Time) (model.Matrix, error) {
	values, _, err := api.Query(ctx, query, end)
	if err != nil {
		return nil, err
	}

	matrix, ok := values.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("query of buckets resulted in non-Matrix")
	}

	return matrix, nil
}

// minChanged/avgChanged/maxChanged is for the number of changed buckets.
// minDelta/avgDelta/maxDelta is for the total absolute change
// in the buckets across all timesteps.
// minPop/avgPop/maxPop is for the number of populated (non-zero) buckets.
// total is the total number of buckets across all samples in the series,
// populated or not.
type statistics struct {
	minChanged, maxChanged, minDelta, maxDelta, minPop, maxPop, total int
	avgChanged, avgDelta, avgPop                                      float64
}

func (s statistics) String() string {
	return fmt.Sprintf("Bucket stats (min/avg/max) - number of buckets changed: %d/%.3f/%d total delta of changed buckets: %d/%.3f/%d number of populated buckets: %d/%.3f/%d. total number of buckets: %d", s.minChanged, s.avgChanged, s.maxChanged, s.minDelta, s.avgDelta, s.maxDelta, s.minPop, s.avgPop, s.maxPop, s.total)
}

func calcClassicBucketStatistics(matrix model.Matrix, histo *statshistogram) (*statistics, error) {
	numBuckets := len(matrix)

	stats := &statistics{
		minChanged: math.MaxInt,
		minDelta:   math.MaxInt,
		minPop:     math.MaxInt,
		total:      numBuckets,
	}

	if numBuckets == 0 || len(matrix[0].Values) < 2 {
		return stats, errNotEnoughData
	}

	numSamples := len(matrix[0].Values)

	sortMatrix(matrix)

	prev, err := getBucketCountsAtTime(matrix, numBuckets, 0)
	if err != nil {
		return stats, err
	}

	sumBucketsChanged := 0
	totalDelta := 0
	totalPop := 0
	for timeIdx := 1; timeIdx < numSamples; timeIdx++ {
		curr, err := getBucketCountsAtTime(matrix, numBuckets, timeIdx)
		if err != nil {
			return stats, err
		}
		countPop := 0
		for _, b := range curr {
			if b != 0 {
				countPop++
			}
		}

		countBucketsChanged := 0
		delta := 0
		for bIdx := range matrix {
			if curr[bIdx] != prev[bIdx] {
				countBucketsChanged++
				delta += abs(curr[bIdx] - prev[bIdx])
			}
		}

		histo.update(countBucketsChanged)

		sumBucketsChanged += countBucketsChanged
		totalDelta += delta
		totalPop += countPop
		if stats.minChanged > countBucketsChanged {
			stats.minChanged = countBucketsChanged
		}
		if stats.maxChanged < countBucketsChanged {
			stats.maxChanged = countBucketsChanged
		}
		if stats.minDelta > delta {
			stats.minDelta = delta
		}
		if stats.maxDelta < delta {
			stats.maxDelta = delta
		}
		if stats.minPop > countPop {
			stats.minPop = countPop
		}
		if stats.maxPop < countPop {
			stats.maxPop = countPop
		}

		prev = curr
	}
	stats.avgChanged = float64(sumBucketsChanged) / float64(numSamples-1)
	stats.avgDelta = float64(totalDelta) / float64(numSamples-1)
	stats.avgPop = float64(totalPop) / float64(numSamples-1) // for simplicity, we ignore the populated buckets in the first timestamp
	return stats, nil
}

func sortMatrix(matrix model.Matrix) {
	sort.SliceStable(matrix, func(i, j int) bool {
		return getLe(matrix[i]) < getLe(matrix[j])
	})
}

func getLe(series *model.SampleStream) float64 {
	lbs := model.LabelSet(series.Metric)
	le, _ := strconv.ParseFloat(string(lbs["le"]), 64)
	return le
}

func getBucketCountsAtTime(matrix model.Matrix, numBuckets, timeIdx int) ([]int, error) {
	counts := make([]int, numBuckets)
	counts[0] = int(matrix[0].Values[timeIdx].Value)
	for i, bucket := range matrix[1:] {
		curr := bucket.Values[timeIdx]
		prev := matrix[i].Values[timeIdx]
		// Assume the results are nicely aligned.
		if curr.Timestamp != prev.Timestamp {
			return counts, fmt.Errorf("matrix result is not time aligned")
		}
		counts[i+1] = int(curr.Value - prev.Value)
	}
	return counts, nil
}

type bucketBounds struct {
	boundaries   int32
	upper, lower float64
}

func makeBucketBounds(b *model.HistogramBucket) bucketBounds {
	return bucketBounds{
		boundaries: b.Boundaries,
		upper:      float64(b.Upper),
		lower:      float64(b.Lower),
	}
}

func abs(num int) int {
	if num < 0 {
		return -num
	}
	return num
}

func calcNativeBucketStatistics(series *model.SampleStream, histo *statshistogram) (*statistics, error) {
	stats := &statistics{
		minChanged: math.MaxInt,
		minDelta:   math.MaxInt,
		minPop:     math.MaxInt,
	}

	overall := make(map[bucketBounds]struct{})
	sumBucketsChanged := 0
	totalDelta := 0
	totalPop := 0
	if len(series.Histograms) == 0 {
		return nil, errNotNativeHistogram
	}
	if len(series.Histograms) == 1 {
		return nil, errNotEnoughData
	}
	prev := make(map[bucketBounds]float64)
	for _, bucket := range series.Histograms[0].Histogram.Buckets {
		bb := makeBucketBounds(bucket)
		prev[bb] = float64(bucket.Count)
		overall[bb] = struct{}{}
	}
	for _, histogram := range series.Histograms[1:] {
		curr := make(map[bucketBounds]float64)
		for _, bucket := range histogram.Histogram.Buckets {
			bb := makeBucketBounds(bucket)
			curr[bb] = float64(bucket.Count)
			overall[bb] = struct{}{}
		}
		countBucketsChanged := 0
		delta := 0
		countPop := len(histogram.Histogram.Buckets)
		for bucket, currCount := range curr {
			prevCount, ok := prev[bucket]
			if !ok {
				countBucketsChanged++
				delta += int(currCount)
			} else if prevCount != currCount {
				countBucketsChanged++
				delta += int(math.Abs(prevCount - currCount))
			}
		}
		for bucket, prevCount := range prev {
			_, ok := curr[bucket]
			if !ok {
				countBucketsChanged++
				delta += int(prevCount)
			}
		}

		histo.update(countBucketsChanged)

		sumBucketsChanged += countBucketsChanged
		totalDelta += delta
		totalPop += countPop
		if stats.minChanged > countBucketsChanged {
			stats.minChanged = countBucketsChanged
		}
		if stats.maxChanged < countBucketsChanged {
			stats.maxChanged = countBucketsChanged
		}
		if stats.minDelta > delta {
			stats.minDelta = delta
		}
		if stats.maxDelta < delta {
			stats.maxDelta = delta
		}
		if stats.minPop > countPop {
			stats.minPop = countPop
		}
		if stats.maxPop < countPop {
			stats.maxPop = countPop
		}

		prev = curr
	}
	stats.avgChanged = float64(sumBucketsChanged) / float64(len(series.Histograms)-1)
	stats.avgDelta = float64(totalDelta) / float64(len(series.Histograms)-1)
	stats.avgPop = float64(totalPop) / float64(len(series.Histograms)-1) // for simplicity, we ignore the populated buckets in the first timestamp
	stats.total = len(overall)
	return stats, nil
}

type distribution struct {
	min, max, count int
	avg             float64
}

func newDistribution() distribution {
	return distribution{
		min: math.MaxInt,
	}
}

func (d *distribution) update(num int) {
	if d.min > num {
		d.min = num
	}
	if d.max < num {
		d.max = num
	}
	d.count++
	d.avg += float64(num)/float64(d.count) - d.avg/float64(d.count)
}

func (d distribution) String() string {
	return fmt.Sprintf("min/avg/max: %d/%.3f/%d", d.min, d.avg, d.max)
}

type metaStatistics struct {
	minChanged, avgChanged, maxChanged, minDelta, avgDelta, maxDelta, minPop, avgPop, maxPop, total distribution
}

func newMetaStatistics() *metaStatistics {
	return &metaStatistics{
		minChanged: newDistribution(),
		avgChanged: newDistribution(),
		maxChanged: newDistribution(),
		minDelta:   newDistribution(),
		avgDelta:   newDistribution(),
		maxDelta:   newDistribution(),
		minPop:     newDistribution(),
		avgPop:     newDistribution(),
		maxPop:     newDistribution(),
		total:      newDistribution(),
	}
}

func (ms metaStatistics) String() string {
	return fmt.Sprintf("minChanged - %v\navgChanged - %v\nmaxChanged - %v\nminDelta - %v\navgDelta - %v\nmaxDelta - %v\nminPop - %v\navgPop - %v\nmaxPop - %v\ntotal - %v\ncount - %d", ms.minChanged, ms.avgChanged, ms.maxChanged, ms.minDelta, ms.avgDelta, ms.maxDelta, ms.minPop, ms.avgPop, ms.maxPop, ms.total, ms.minChanged.count)
}

func (ms *metaStatistics) update(s *statistics) {
	ms.minChanged.update(s.minChanged)
	ms.avgChanged.update(int(s.avgChanged))
	ms.maxChanged.update(s.maxChanged)
	ms.minDelta.update(s.minDelta)
	ms.avgDelta.update(int(s.avgDelta))
	ms.maxDelta.update(s.maxDelta)
	ms.minPop.update(s.minPop)
	ms.avgPop.update(int(s.avgPop))
	ms.maxPop.update(s.maxPop)
	ms.total.update(s.total)
}

type statshistogram struct {
	counts map[int]int
}

func newStatsHistogram() *statshistogram {
	return &statshistogram{
		counts: make(map[int]int, 0),
	}
}

func (sh *statshistogram) update(num int) {
	sh.counts[num]++
}
