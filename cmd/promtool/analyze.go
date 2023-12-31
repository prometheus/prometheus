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

type AnalyzeHistogramsConfig struct {
	histogramType  string
	lookback       time.Duration
	scrapeInterval time.Duration
}

// run retrieves metrics that look like conventional histograms (i.e. have _bucket
// suffixes) or native histograms, depending on histogramType flag.
func (c *AnalyzeHistogramsConfig) run(url *url.URL, roundtripper http.RoundTripper) error {
	if c.histogramType != "classic" && c.histogramType != "native" {
		return fmt.Errorf("histogram type is %s, must be 'classic' or 'native'", c.histogramType)
	}

	ctx := context.Background()

	api, err := newAPI(url, roundtripper, nil)
	if err != nil {
		return err
	}

	endTime := time.Now()
	startTime := endTime.Add(-c.lookback)

	if c.histogramType == "native" {
		histoMetrics, err := queryMetadataForHistograms(ctx, api, "", "1000")
		if err != nil {
			return err
		}
		return c.getStatsFromMetrics(ctx, api, startTime, endTime, os.Stdout, identity, identity, calcNativeBucketStatistics, histoMetrics)
	}

	baseMetrics, err := queryBaseMetricNames(ctx, api, startTime, endTime)
	if err != nil {
		return err
	}
	fmt.Printf("Potential histogram metrics found: %d\n", len(baseMetrics))
	return c.getStatsFromMetrics(ctx, api, startTime, endTime, os.Stdout, appendSum, appendBucket, calcClassicBucketStatistics, baseMetrics)
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

func (c *AnalyzeHistogramsConfig) getStatsFromMetrics(ctx context.Context, api v1.API, startTime, endTime time.Time, out io.Writer, transformNameForLabelSet, transformNameForSeriesSel transformNameFunc, calcBucketStatistics calcBucketStatisticsFunc, metricNames []string) error {
	metastats := newMetaStatistics()
	metahistogram := newStatsHistogram()

	for _, metricName := range metricNames {
		vector, err := queryLabelSets(ctx, api, transformNameForLabelSet(metricName), endTime, c.lookback)
		if err != nil {
			return err
		}

		for _, sample := range vector {
			// Get labels to match.
			lbs := model.LabelSet(sample.Metric)
			delete(lbs, labels.MetricName)
			seriesSel := seriesSelector(transformNameForSeriesSel(metricName), lbs)
			var step time.Duration
			if c.scrapeInterval == 0 {
				// Estimate the step.
				stepVal := int64(math.Round(c.lookback.Seconds() / float64(sample.Value)))
				step = time.Duration(stepVal) * time.Second
			} else {
				step = c.scrapeInterval
			}
			matrix, err := querySamples(ctx, api, seriesSel, startTime, endTime, step)
			if err != nil {
				return err
			}
			stats, err := calcBucketStatistics(matrix, step, metahistogram)
			if err != nil {
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

func queryBaseMetricNames(ctx context.Context, api v1.API, start, end time.Time) ([]string, error) {
	values, _, err := api.LabelValues(ctx, labels.MetricName, []string{"{__name__=~\".*_bucket\"}"}, start, end)
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, v := range values {
		basename, err := metricBasename(string(v))
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

func queryMetadataForHistograms(ctx context.Context, api v1.API, query, limit string) ([]string, error) {
	result, err := api.Metadata(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	metrics := make([]string, 0)
	for metric, metadata := range result {
		for _, metadatum := range metadata {
			if metadatum.Type == "histogram" || metadatum.Type == "gaugehistogram" {
				metrics = append(metrics, metric)
			}
		}
	}

	return metrics, nil
}

// Query the related count_over_time(*_sum[lookback]) series to double check that metricName is a
// histogram. This keeps the result small (avoids buckets) and the count gives scrape interval
// when dividing lookback with it.
func queryLabelSets(ctx context.Context, api v1.API, metricName string, end time.Time, lookback time.Duration) (model.Vector, error) {
	query := fmt.Sprintf("count_over_time(%s{}[%s])", metricName, lookback.String())

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

func seriesSelector(bucketMetricName string, lbs model.LabelSet) string {
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

	return builder.String()
}

func querySamples(ctx context.Context, api v1.API, query string, start, end time.Time, step time.Duration) (model.Matrix, error) {
	values, _, err := api.QueryRange(ctx, query, v1.Range{Start: start, End: end, Step: step})
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
// changed buckets.
type statistics struct {
	min, max, available int
	avg                 float64
	scrapeInterval      time.Duration
}

func (s statistics) String() string {
	return fmt.Sprintf("Bucket min/avg/max/avail@scrape: %d/%.3f/%d/%d@%v", s.min, s.avg, s.max, s.available, s.scrapeInterval)
}

type calcBucketStatisticsFunc func(matrix model.Matrix, step time.Duration, histo *statshistogram) (*statistics, error)

func calcClassicBucketStatistics(matrix model.Matrix, step time.Duration, histo *statshistogram) (*statistics, error) {
	numBuckets := len(matrix)

	stats := &statistics{
		min:            math.MaxInt,
		available:      numBuckets,
		scrapeInterval: step,
	}

	if numBuckets == 0 || len(matrix[0].Values) < 2 {
		// Not enough data.
		return stats, nil
	}

	numSamples := len(matrix[0].Values)

	sortMatrix(matrix)

	prev, err := getBucketCountsAtTime(matrix, numBuckets, 0)
	if err != nil {
		return stats, err
	}

	sumBucketsChanged := 0
	for timeIdx := 1; timeIdx < numSamples; timeIdx++ {
		curr, err := getBucketCountsAtTime(matrix, numBuckets, timeIdx)
		if err != nil {
			return stats, err
		}

		countBucketsChanged := 0
		for bIdx := range matrix {
			if curr[bIdx] != prev[bIdx] {
				countBucketsChanged++
			}
		}

		histo.update(countBucketsChanged)

		sumBucketsChanged += countBucketsChanged
		if stats.min > countBucketsChanged {
			stats.min = countBucketsChanged
		}
		if stats.max < countBucketsChanged {
			stats.max = countBucketsChanged
		}

		prev = curr
	}
	stats.avg = float64(sumBucketsChanged) / float64(numSamples-1)
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

func calcNativeBucketStatistics(matrix model.Matrix, step time.Duration, histo *statshistogram) (*statistics, error) {
	stats := &statistics{
		min:            math.MaxInt,
		scrapeInterval: step,
	}

	overall := make(map[bucketBounds]struct{})
	sumBucketsChanged := 0
	for _, vector := range matrix {
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
			for bucket, currCount := range curr {
				prevCount, ok := prev[bucket]
				if !ok {
					countBucketsChanged++
				} else if prevCount != currCount {
					countBucketsChanged++
				}
			}
			for bucket := range prev {
				_, ok := curr[bucket]
				if !ok {
					countBucketsChanged++
				}
			}

			histo.update(countBucketsChanged)

			sumBucketsChanged += countBucketsChanged
			if stats.min > countBucketsChanged {
				stats.min = countBucketsChanged
			}
			if stats.max < countBucketsChanged {
				stats.max = countBucketsChanged
			}

			prev = curr
		}
	}
	stats.avg = float64(sumBucketsChanged) / float64(len(matrix[0].Histograms)-1)
	stats.available = len(overall)
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
	min, avg, max, available distribution
}

func newMetaStatistics() *metaStatistics {
	return &metaStatistics{
		min:       newDistribution(),
		avg:       newDistribution(),
		max:       newDistribution(),
		available: newDistribution(),
	}
}

func (ms metaStatistics) String() string {
	return fmt.Sprintf("min - %v\navg - %v\nmax - %v\navail - %v\ncount - %d", ms.min, ms.avg, ms.max, ms.available, ms.min.count)
}

func (ms *metaStatistics) update(s *statistics) {
	ms.min.update(s.min)
	ms.avg.update(int(s.avg))
	ms.max.update(s.max)
	ms.available.update(s.available)
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
