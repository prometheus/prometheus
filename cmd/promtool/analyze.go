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

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
)

var (
	errNotNativeHistogram = fmt.Errorf("not a native histogram")
	errNotEnoughData      = fmt.Errorf("not enough data")
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
	startTime := endTime.Add(-c.duration)

	if c.metricType == "nativehistograms" {
		histoMetrics, err := queryMetricNames(ctx, api, c.matchers, startTime, endTime)
		if err != nil {
			return err
		}
		return c.getStatsFromMetrics(ctx, api, endTime, os.Stdout, identity, identity, calcNativeBucketStatistics, histoMetrics)
	}

	baseMetrics, err := queryBaseMetricNames(ctx, api, c.matchers, startTime, endTime)
	if err != nil {
		return err
	}
	fmt.Printf("Potential histogram metrics found: %d\n", len(baseMetrics))
	return c.getStatsFromMetrics(ctx, api, endTime, os.Stdout, appendSum, appendBucket, calcClassicBucketStatistics, baseMetrics)
}

type transformNameFunc func(name string) string

func identity(name string) string {
	return name
}

func appendSum(name string) string {
	return fmt.Sprintf("%s_sum", name)
}

func appendBucket(name string) string {
	return fmt.Sprintf("%s_bucket", name)
}

func (c *QueryAnalyzeConfig) getStatsFromMetrics(ctx context.Context, api v1.API, endTime time.Time, out io.Writer, transformNameForLabelSet, transformNameForSeriesSel transformNameFunc, calcBucketStatistics calcBucketStatisticsFunc, metricNames []string) error {
	metastats := newMetaStatistics()
	metahistogram := newStatsHistogram()

	for _, metricName := range metricNames {
		vector, err := queryLabelSets(ctx, api, transformNameForLabelSet(metricName), endTime, c.duration)
		if err != nil {
			return err
		}

		for _, sample := range vector {
			// Get labels to match.
			lbs := model.LabelSet(sample.Metric)
			delete(lbs, labels.MetricName)
			seriesSel := seriesSelector(transformNameForSeriesSel(metricName), lbs, c.duration)
			matrix, err := querySamples(ctx, api, seriesSel, endTime)
			if err != nil {
				return err
			}
			stats, err := calcBucketStatistics(matrix, metahistogram)
			if err != nil {
				if err == errNotNativeHistogram || err == errNotEnoughData {
					continue
				}
				return err
			}
			fmt.Fprintf(out, "%s=%v\n", seriesSel, *stats)
			metastats.update(stats)
		}
	}
	fmt.Println(metastats)
	fmt.Println(metahistogram)
	return nil
}

func queryMetricNames(ctx context.Context, api v1.API, matchers []string, start, end time.Time) ([]string, error) {
	values, _, err := api.LabelValues(ctx, labels.MetricName, matchers, start, end)
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, v := range values {
		s := string(v)
		if !strings.HasSuffix(s, "_bucket") && !strings.HasSuffix(s, "_count") && !strings.HasSuffix(s, "_sum") {
			result = append(result, s)
		}
	}
	return result, err
}

func queryBaseMetricNames(ctx context.Context, api v1.API, matchers []string, start, end time.Time) ([]string, error) {
	values, _, err := api.LabelValues(ctx, labels.MetricName, matchers, start, end)
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, v := range values {
		s := string(v)
		if !strings.HasSuffix(s, "_bucket") {
			continue
		}
		basename, err := metricBasename(s)
		if err != nil {
			// Just skip if something doesn't look like a good name.
			continue
		}
		result = append(result, basename)
	}
	return result, nil
}

func metricBasename(bucketMetricName string) (string, error) {
	if len(bucketMetricName) <= len("_bucket") {
		return "", fmt.Errorf("potential metric name too short: %s", bucketMetricName)
	}
	return bucketMetricName[:len(bucketMetricName)-len("_bucket")], nil
}

// Query the related count_over_time(*_sum[duration]) series to double check that metricName is a
// histogram. This keeps the result small (avoids buckets) and the count gives scrape interval
// when dividing duration with it.
func queryLabelSets(ctx context.Context, api v1.API, metricName string, end time.Time, duration time.Duration) (model.Vector, error) {
	query := fmt.Sprintf("count_over_time(%s{}[%s])", metricName, duration.String())

	values, _, err := api.Query(ctx, query, end)
	if err != nil {
		return nil, err
	}

	vector, ok := values.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("query for metrics resulted in non-Vector")
	}
	return vector, nil
}

func seriesSelector(bucketMetricName string, lbs model.LabelSet, duration time.Duration) string {
	builder := strings.Builder{}
	builder.WriteString(bucketMetricName)
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

// available is the total number of buckets. min/avg/max is for the number of
// changed buckets. minDelta/avgDelta/maxDelta is for the total absolute change
// in the buckets across all timesteps.
type statistics struct {
	min, max, populated, available, minDelta, maxDelta int
	avg, avgDelta                                      float64
}

func (s statistics) String() string {
	return fmt.Sprintf("Bucket min/avg/max/pop/avail/minDelta/avgDelta/maxDelta: %d/%.3f/%d/%d/%d/%d/%.3f/%d", s.min, s.avg, s.max, s.populated, s.available, s.minDelta, s.avgDelta, s.maxDelta)
}

type calcBucketStatisticsFunc func(matrix model.Matrix, histo *statshistogram) (*statistics, error)

func calcClassicBucketStatistics(matrix model.Matrix, histo *statshistogram) (*statistics, error) {
	numBuckets := len(matrix)

	stats := &statistics{
		min:       math.MaxInt,
		minDelta:  math.MaxInt,
		available: numBuckets,
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
	isNotEmpty := make(map[int]struct{})
	for i, b := range prev {
		if b != 0 {
			isNotEmpty[i] = struct{}{}
		}
	}

	sumBucketsChanged := 0
	totalDelta := 0
	for timeIdx := 1; timeIdx < numSamples; timeIdx++ {
		curr, err := getBucketCountsAtTime(matrix, numBuckets, timeIdx)
		if err != nil {
			return stats, err
		}
		for i, b := range curr {
			if b != 0 {
				isNotEmpty[i] = struct{}{}
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
		if stats.min > countBucketsChanged {
			stats.min = countBucketsChanged
		}
		if stats.max < countBucketsChanged {
			stats.max = countBucketsChanged
		}
		if stats.minDelta > delta {
			stats.minDelta = delta
		}
		if stats.maxDelta < delta {
			stats.maxDelta = delta
		}

		prev = curr
	}
	stats.avg = float64(sumBucketsChanged) / float64(numSamples-1)
	stats.avgDelta = float64(totalDelta) / float64(numSamples-1)
	stats.populated = len(isNotEmpty)
	return stats, nil
}

func sortMatrix(matrix model.Matrix) {
	sort.SliceStable(matrix, func(i, j int) bool {
		return getLe(matrix[i]) < getLe(matrix[j])
	})
}

func getLe(vector *model.SampleStream) float64 {
	lbs := model.LabelSet(vector.Metric)
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

func calcNativeBucketStatistics(matrix model.Matrix, histo *statshistogram) (*statistics, error) {
	stats := &statistics{
		min:      math.MaxInt,
		minDelta: math.MaxInt,
	}

	overall := make(map[bucketBounds]struct{})
	sumBucketsChanged := 0
	totalDelta := 0
	if len(matrix) == 0 {
		return nil, errNotEnoughData
	}
	for _, vector := range matrix {
		if len(vector.Histograms) == 0 {
			return nil, errNotNativeHistogram
		}
		prev := make(map[bucketBounds]float64)
		for _, bucket := range vector.Histograms[0].Histogram.Buckets {
			bb := makeBucketBounds(bucket)
			prev[bb] = float64(bucket.Count)
			overall[bb] = struct{}{}
		}
		for _, histogram := range vector.Histograms[1:] {
			curr := make(map[bucketBounds]float64)
			for _, bucket := range histogram.Histogram.Buckets {
				bb := makeBucketBounds(bucket)
				curr[bb] = float64(bucket.Count)
				overall[bb] = struct{}{}
			}
			countBucketsChanged := 0
			delta := 0
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
			if stats.min > countBucketsChanged {
				stats.min = countBucketsChanged
			}
			if stats.max < countBucketsChanged {
				stats.max = countBucketsChanged
			}
			if stats.minDelta > delta {
				stats.minDelta = delta
			}
			if stats.maxDelta < delta {
				stats.maxDelta = delta
			}

			prev = curr
		}
	}
	stats.avg = float64(sumBucketsChanged) / float64(len(matrix[0].Histograms)-1)
	stats.avgDelta = float64(totalDelta) / float64(len(matrix[0].Histograms)-1)
	stats.available = len(overall)
	stats.populated = stats.available
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
	min, avg, max, populated, available, minDelta, avgDelta, maxDelta distribution
}

func newMetaStatistics() *metaStatistics {
	return &metaStatistics{
		min:       newDistribution(),
		avg:       newDistribution(),
		max:       newDistribution(),
		populated: newDistribution(),
		available: newDistribution(),
		minDelta:  newDistribution(),
		avgDelta:  newDistribution(),
		maxDelta:  newDistribution(),
	}
}

func (ms metaStatistics) String() string {
	return fmt.Sprintf("min - %v\navg - %v\nmax - %v\npopulated - %v\navail - %v\nminDelta - %v\navgDelta - %v\nmaxDelta - %v\ncount - %d", ms.min, ms.avg, ms.max, ms.populated, ms.available, ms.minDelta, ms.avgDelta, ms.maxDelta, ms.min.count)
}

func (ms *metaStatistics) update(s *statistics) {
	ms.min.update(s.min)
	ms.avg.update(int(s.avg))
	ms.max.update(s.max)
	ms.populated.update(s.populated)
	ms.available.update(s.available)
	ms.minDelta.update(s.minDelta)
	ms.avgDelta.update(int(s.avgDelta))
	ms.maxDelta.update(s.maxDelta)
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
