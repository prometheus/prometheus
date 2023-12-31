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
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/model/labels"
)

type AnalyzeClassicHistogramsConfig struct {
	address        *url.URL
	user           string
	key            string
	lookback       time.Duration
	readTimeout    time.Duration
	scrapeInterval time.Duration
	outputFile     string
}

// run retrieves metrics that look like conventional histograms, i.e. have _bucket
// suffixes.
func (c *AnalyzeClassicHistogramsConfig) run() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.readTimeout)
	defer cancel()

	api, err := newAPI(c.address, c.user, c.key)
	if err != nil {
		return err
	}

	endTime := time.Now()
	startTime := endTime.Add(-c.lookback)

	bucketMetrics, err := queryBucketMetricNames(ctx, api, startTime, endTime)
	if err != nil {
		return err
	}
	fmt.Printf("Potential histogram metrics found: %d\n", len(bucketMetrics))

	var out io.Writer
	if c.outputFile != "" {
		var err error
		out, err = os.Create(c.outputFile)
		if err != nil {
			return fmt.Errorf("failed to open %q for writing: ", c.outputFile)
		}
	} else {
		out = os.Stdout
	}

	metastats := newMetastatistics()
	for _, bucketMetricName := range bucketMetrics {
		basename, err := metricBasename(bucketMetricName)
		if err != nil {
			// Just skip if something doesn't look like a good name.
			continue
		}
		sumName := fmt.Sprintf("%s_sum", basename)
		vector, err := queryLabelSets(ctx, api, sumName, endTime, c.lookback)
		if err != nil {
			return err
		}

		for _, sample := range vector {
			// Get labels to match.
			lbs := model.LabelSet(sample.Metric)
			delete(lbs, labels.MetricName)
			seriesSel := seriesSelector(bucketMetricName, lbs)
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
			stats, err := calcBucketStatistics(matrix, step)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%s=%v\n", seriesSel, *stats)
			metastats.update(stats)
		}
	}
	fmt.Println(metastats)
	return nil
}

func newAPI(address *url.URL, username, password string) (v1.API, error) {
	rt := api.DefaultRoundTripper
	rt = config.NewUserAgentRoundTripper("promtool/"+version.Version, rt)
	if username != "" {
		rt = config.NewBasicAuthRoundTripper(username, config.Secret(password), "", rt)
	}

	client, err := api.NewClient(api.Config{
		Address:      address.String(),
		RoundTripper: rt,
	})
	if err != nil {
		return nil, err
	}

	return v1.NewAPI(client), nil
}

func queryBucketMetricNames(ctx context.Context, api v1.API, start, end time.Time) ([]string, error) {
	values, _, err := api.LabelValues(ctx, labels.MetricName, []string{"{__name__=~\".*_bucket\"}"}, start, end)
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, v := range values {
		result = append(result, string(v))
	}
	return result, nil
}

func metricBasename(bucketMetricName string) (string, error) {
	if len(bucketMetricName) <= len("_bucket") {
		return "", fmt.Errorf("potential metric name too short: %s", bucketMetricName)
	}
	return bucketMetricName[:len(bucketMetricName)-len("_bucket")], nil
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

type statistics struct {
	min, avg, max, available int
	scrapeInterval           time.Duration
}

func (s statistics) String() string {
	return fmt.Sprintf("Bucket min/avg/max/avail@scrape: %d/%d/%d/%d@%v", s.min, s.avg, s.max, s.available, s.scrapeInterval)
}

func calcBucketStatistics(matrix model.Matrix, step time.Duration) (*statistics, error) {
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
			if curr[bIdx] > prev[bIdx] {
				countBucketsChanged++
			} else if curr[bIdx] < prev[bIdx] {
				return stats, fmt.Errorf("unexpected decrease in bucket %d at time %d", bIdx, timeIdx)
			}
		}

		sumBucketsChanged += countBucketsChanged
		if stats.min > countBucketsChanged {
			stats.min = countBucketsChanged
		}
		if stats.max < countBucketsChanged {
			stats.max = countBucketsChanged
		}

		prev = curr
	}
	stats.avg = sumBucketsChanged / (numSamples - 1)
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

type metastatistics struct {
	min, avg, max, available distribution
}

func newMetastatistics() *metastatistics {
	return &metastatistics{
		min:       newDistribution(),
		avg:       newDistribution(),
		max:       newDistribution(),
		available: newDistribution(),
	}
}

func (ms metastatistics) String() string {
	return fmt.Sprintf("min - %v\navg - %v\nmax - %v\navail - %v\ncount - %d", ms.min, ms.avg, ms.max, ms.available, ms.min.count)
}

func (ms *metastatistics) update(s *statistics) {
	ms.min.update(s.min)
	ms.avg.update(s.avg)
	ms.max.update(s.max)
	ms.available.update(s.available)
}
