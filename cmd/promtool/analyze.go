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

package main

import (
	"context"
	"errors"
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
	errNotNativeHistogram = errors.New("not a native histogram")
	errNotEnoughData      = errors.New("not enough data")

	outputHeader = `Bucket stats for each histogram series over time
------------------------------------------------
First the min, avg, and max number of populated buckets, followed by the total
number of buckets (only if different from the max number of populated buckets
which is typical for classic but not native histograms).`
	outputFooter = `Aggregated bucket stats
-----------------------
Each line shows min/avg/max over the series above.`
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
	if c.metricType != "histogram" {
		return fmt.Errorf("analyze type is %s, must be 'histogram'", c.metricType)
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

	return c.getStatsFromMetrics(ctx, api, endTime, os.Stdout, c.matchers)
}

func (c *QueryAnalyzeConfig) getStatsFromMetrics(ctx context.Context, api v1.API, endTime time.Time, out io.Writer, matchers []string) error {
	fmt.Fprintf(out, "%s\n\n", outputHeader)
	metastatsNative := newMetaStatistics()
	metastatsClassic := newMetaStatistics()
	for _, matcher := range matchers {
		seriesSel := seriesSelector(matcher, c.duration)
		matrix, err := querySamples(ctx, api, seriesSel, endTime)
		if err != nil {
			return err
		}

		matrices := make(map[string]model.Matrix)
		for _, series := range matrix {
			// We do not handle mixed types. If there are float values, we assume it is a
			// classic histogram, otherwise we assume it is a native histogram, and we
			// ignore series with errors if they do not match the expected type.
			if len(series.Values) == 0 {
				stats, err := calcNativeBucketStatistics(series)
				if err != nil {
					if errors.Is(err, errNotNativeHistogram) || errors.Is(err, errNotEnoughData) {
						continue
					}
					return err
				}
				fmt.Fprintf(out, "- %s (native): %v\n", series.Metric, *stats)
				metastatsNative.update(stats)
			} else {
				lbs := model.LabelSet(series.Metric).Clone()
				if _, ok := lbs["le"]; !ok {
					continue
				}
				metricName := string(lbs[labels.MetricName])
				if !strings.HasSuffix(metricName, "_bucket") {
					continue
				}
				delete(lbs, labels.MetricName)
				delete(lbs, "le")
				key := formatSeriesName(metricName, lbs)
				matrices[key] = append(matrices[key], series)
			}
		}

		for key, matrix := range matrices {
			stats, err := calcClassicBucketStatistics(matrix)
			if err != nil {
				if errors.Is(err, errNotEnoughData) {
					continue
				}
				return err
			}
			fmt.Fprintf(out, "- %s (classic): %v\n", key, *stats)
			metastatsClassic.update(stats)
		}
	}
	fmt.Fprintf(out, "\n%s\n", outputFooter)
	if metastatsNative.Count() > 0 {
		fmt.Fprintf(out, "\nNative %s\n", metastatsNative)
	}
	if metastatsClassic.Count() > 0 {
		fmt.Fprintf(out, "\nClassic %s\n", metastatsClassic)
	}
	return nil
}

func seriesSelector(metricName string, duration time.Duration) string {
	builder := strings.Builder{}
	builder.WriteString(metricName)
	builder.WriteRune('[')
	builder.WriteString(duration.String())
	builder.WriteRune(']')
	return builder.String()
}

func formatSeriesName(metricName string, lbs model.LabelSet) string {
	builder := strings.Builder{}
	builder.WriteString(metricName)
	builder.WriteString(lbs.String())
	return builder.String()
}

func querySamples(ctx context.Context, api v1.API, query string, end time.Time) (model.Matrix, error) {
	values, _, err := api.Query(ctx, query, end)
	if err != nil {
		return nil, err
	}

	matrix, ok := values.(model.Matrix)
	if !ok {
		return nil, errors.New("query of buckets resulted in non-Matrix")
	}

	return matrix, nil
}

// minPop/avgPop/maxPop is for the number of populated (non-zero) buckets.
// total is the total number of buckets across all samples in the series,
// populated or not.
type statistics struct {
	minPop, maxPop, total int
	avgPop                float64
}

func (s statistics) String() string {
	if s.maxPop == s.total {
		return fmt.Sprintf("%d/%.3f/%d", s.minPop, s.avgPop, s.maxPop)
	}
	return fmt.Sprintf("%d/%.3f/%d/%d", s.minPop, s.avgPop, s.maxPop, s.total)
}

func calcClassicBucketStatistics(matrix model.Matrix) (*statistics, error) {
	numBuckets := len(matrix)

	stats := &statistics{
		minPop: math.MaxInt,
		total:  numBuckets,
	}

	if numBuckets == 0 || len(matrix[0].Values) < 2 {
		return stats, errNotEnoughData
	}

	numSamples := len(matrix[0].Values)

	sortMatrix(matrix)

	totalPop := 0
	for timeIdx := range numSamples {
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

		totalPop += countPop
		if stats.minPop > countPop {
			stats.minPop = countPop
		}
		if stats.maxPop < countPop {
			stats.maxPop = countPop
		}
	}
	stats.avgPop = float64(totalPop) / float64(numSamples)
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
	if timeIdx >= len(matrix[0].Values) {
		// Just return zeroes instead of erroring out so we can get partial results.
		return counts, nil
	}
	counts[0] = int(matrix[0].Values[timeIdx].Value)
	for i, bucket := range matrix[1:] {
		if timeIdx >= len(bucket.Values) {
			// Just return zeroes instead of erroring out so we can get partial results.
			return counts, nil
		}
		curr := bucket.Values[timeIdx]
		prev := matrix[i].Values[timeIdx]
		// Assume the results are nicely aligned.
		if curr.Timestamp != prev.Timestamp {
			return counts, errors.New("matrix result is not time aligned")
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

func calcNativeBucketStatistics(series *model.SampleStream) (*statistics, error) {
	stats := &statistics{
		minPop: math.MaxInt,
	}

	overall := make(map[bucketBounds]struct{})
	totalPop := 0
	if len(series.Histograms) == 0 {
		return nil, errNotNativeHistogram
	}
	if len(series.Histograms) == 1 {
		return nil, errNotEnoughData
	}
	for _, histogram := range series.Histograms {
		for _, bucket := range histogram.Histogram.Buckets {
			bb := makeBucketBounds(bucket)
			overall[bb] = struct{}{}
		}
		countPop := len(histogram.Histogram.Buckets)

		totalPop += countPop
		if stats.minPop > countPop {
			stats.minPop = countPop
		}
		if stats.maxPop < countPop {
			stats.maxPop = countPop
		}
	}
	stats.avgPop = float64(totalPop) / float64(len(series.Histograms))
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
	return fmt.Sprintf("%d/%.3f/%d", d.min, d.avg, d.max)
}

type metaStatistics struct {
	minPop, avgPop, maxPop, total distribution
}

func newMetaStatistics() *metaStatistics {
	return &metaStatistics{
		minPop: newDistribution(),
		avgPop: newDistribution(),
		maxPop: newDistribution(),
		total:  newDistribution(),
	}
}

func (ms metaStatistics) Count() int {
	return ms.minPop.count
}

func (ms metaStatistics) String() string {
	if ms.maxPop == ms.total {
		return fmt.Sprintf("histogram series (%d in total):\n- min populated: %v\n- avg populated: %v\n- max populated: %v", ms.Count(), ms.minPop, ms.avgPop, ms.maxPop)
	}
	return fmt.Sprintf("histogram series (%d in total):\n- min populated: %v\n- avg populated: %v\n- max populated: %v\n- total: %v", ms.Count(), ms.minPop, ms.avgPop, ms.maxPop, ms.total)
}

func (ms *metaStatistics) update(s *statistics) {
	ms.minPop.update(s.minPop)
	ms.avgPop.update(int(s.avgPop))
	ms.maxPop.update(s.maxPop)
	ms.total.update(s.total)
}
