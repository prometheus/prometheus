// Copyright 2013 Prometheus Team
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

package metric

import (
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/utility/test"
	"sort"
	"testing"
	"time"
)

func buildSamples(from, to time.Time, interval time.Duration, m clientmodel.Metric) (v []clientmodel.Sample) {
	i := model.SampleValue(0)

	for from.Before(to) {
		v = append(v, clientmodel.Sample{
			Metric:    m,
			Value:     i,
			Timestamp: from,
		})

		from = from.Add(interval)
		i++
	}

	return
}

func testMakeView(t test.Tester, flushToDisk bool) {
	type in struct {
		atTime     []getValuesAtTimeOp
		atInterval []getValuesAtIntervalOp
		alongRange []getValuesAlongRangeOp
	}

	type out struct {
		atTime     []model.Values
		atInterval []model.Values
		alongRange []model.Values
	}
	var (
		instant     = time.Date(1984, 3, 30, 0, 0, 0, 0, time.Local)
		metric      = clientmodel.Metric{model.MetricNameLabel: "request_count"}
		fingerprint = *model.NewFingerprintFromMetric(metric)
		scenarios   = []struct {
			data []clientmodel.Sample
			in   in
			out  out
		}{
			// No sample, but query asks for one.
			{
				in: in{
					atTime: []getValuesAtTimeOp{
						{
							time: instant,
						},
					},
				},
				out: out{
					atTime: []model.Values{{}},
				},
			},
			// Single sample, query asks for exact sample time.
			{
				data: []clientmodel.Sample{
					{
						Metric:    metric,
						Value:     0,
						Timestamp: instant,
					},
				},
				in: in{
					atTime: []getValuesAtTimeOp{
						{
							time: instant,
						},
					},
				},
				out: out{
					atTime: []model.Values{
						{
							{
								Timestamp: instant,
								Value:     0,
							},
						},
					},
				},
			},
			// Single sample, query time before the sample.
			{
				data: []clientmodel.Sample{
					{
						Metric:    metric,
						Value:     0,
						Timestamp: instant.Add(time.Second),
					},
					{
						Metric:    metric,
						Value:     1,
						Timestamp: instant.Add(time.Second * 2),
					},
				},
				in: in{
					atTime: []getValuesAtTimeOp{
						{
							time: instant,
						},
					},
				},
				out: out{
					atTime: []model.Values{
						{
							{
								Timestamp: instant.Add(time.Second),
								Value:     0,
							},
						},
					},
				},
			},
			// Single sample, query time after the sample.
			{
				data: []clientmodel.Sample{
					{
						Metric:    metric,
						Value:     0,
						Timestamp: instant,
					},
				},
				in: in{
					atTime: []getValuesAtTimeOp{
						{
							time: instant.Add(time.Second),
						},
					},
				},
				out: out{
					atTime: []model.Values{
						{
							{
								Timestamp: instant,
								Value:     0,
							},
						},
					},
				},
			},
			// Two samples, query asks for first sample time.
			{
				data: []clientmodel.Sample{
					{
						Metric:    metric,
						Value:     0,
						Timestamp: instant,
					},
					{
						Metric:    metric,
						Value:     1,
						Timestamp: instant.Add(time.Second),
					},
				},
				in: in{
					atTime: []getValuesAtTimeOp{
						{
							time: instant,
						},
					},
				},
				out: out{
					atTime: []model.Values{
						{
							{
								Timestamp: instant,
								Value:     0,
							},
						},
					},
				},
			},
			// Three samples, query asks for second sample time.
			{
				data: []clientmodel.Sample{
					{
						Metric:    metric,
						Value:     0,
						Timestamp: instant,
					},
					{
						Metric:    metric,
						Value:     1,
						Timestamp: instant.Add(time.Second),
					},
					{
						Metric:    metric,
						Value:     2,
						Timestamp: instant.Add(time.Second * 2),
					},
				},
				in: in{
					atTime: []getValuesAtTimeOp{
						{
							time: instant.Add(time.Second),
						},
					},
				},
				out: out{
					atTime: []model.Values{
						{
							{
								Timestamp: instant.Add(time.Second),
								Value:     1,
							},
						},
					},
				},
			},
			// Three samples, query asks for time between first and second samples.
			{
				data: []clientmodel.Sample{
					{
						Metric:    metric,
						Value:     0,
						Timestamp: instant,
					},
					{
						Metric:    metric,
						Value:     1,
						Timestamp: instant.Add(time.Second * 2),
					},
					{
						Metric:    metric,
						Value:     2,
						Timestamp: instant.Add(time.Second * 4),
					},
				},
				in: in{
					atTime: []getValuesAtTimeOp{
						{
							time: instant.Add(time.Second),
						},
					},
				},
				out: out{
					atTime: []model.Values{
						{
							{
								Timestamp: instant,
								Value:     0,
							},
							{
								Timestamp: instant.Add(time.Second * 2),
								Value:     1,
							},
						},
					},
				},
			},
			// Three samples, query asks for time between second and third samples.
			{
				data: []clientmodel.Sample{
					{
						Metric:    metric,
						Value:     0,
						Timestamp: instant,
					},
					{
						Metric:    metric,
						Value:     1,
						Timestamp: instant.Add(time.Second * 2),
					},
					{
						Metric:    metric,
						Value:     2,
						Timestamp: instant.Add(time.Second * 4),
					},
				},
				in: in{
					atTime: []getValuesAtTimeOp{
						{
							time: instant.Add(time.Second * 3),
						},
					},
				},
				out: out{
					atTime: []model.Values{
						{
							{
								Timestamp: instant.Add(time.Second * 2),
								Value:     1,
							},
							{
								Timestamp: instant.Add(time.Second * 4),
								Value:     2,
							},
						},
					},
				},
			},
			// Two chunks of samples, query asks for values from first chunk.
			{
				data: buildSamples(instant, instant.Add(time.Duration(*leveldbChunkSize*2)*time.Second), time.Second, metric),
				in: in{
					atTime: []getValuesAtTimeOp{
						{
							time: instant.Add(time.Second*time.Duration(*leveldbChunkSize/2) + 1),
						},
					},
				},
				out: out{
					atTime: []model.Values{
						{
							{
								Timestamp: instant.Add(time.Second * time.Duration(*leveldbChunkSize/2)),
								Value:     100,
							},
							{
								Timestamp: instant.Add(time.Second * (time.Duration(*leveldbChunkSize/2) + 1)),
								Value:     101,
							},
						},
					},
				},
			},
		}
	)

	for i, scenario := range scenarios {
		tiered, closer := NewTestTieredStorage(t)

		err := tiered.AppendSamples(scenario.data)
		if err != nil {
			t.Fatalf("%d. failed to add fixture data: %s", i, err)
		}

		if flushToDisk {
			tiered.Flush()
		}

		requestBuilder := NewViewRequestBuilder()

		for _, atTime := range scenario.in.atTime {
			requestBuilder.GetMetricAtTime(fingerprint, atTime.time)
		}

		for _, atInterval := range scenario.in.atInterval {
			requestBuilder.GetMetricAtInterval(fingerprint, atInterval.from, atInterval.through, atInterval.interval)
		}

		for _, alongRange := range scenario.in.alongRange {
			requestBuilder.GetMetricRange(fingerprint, alongRange.from, alongRange.through)
		}

		v, err := tiered.MakeView(requestBuilder, time.Second*5, stats.NewTimerGroup())

		if err != nil {
			t.Fatalf("%d. failed due to %s", i, err)
		}

		for j, atTime := range scenario.in.atTime {
			actual := v.GetValueAtTime(&fingerprint, atTime.time)

			if len(actual) != len(scenario.out.atTime[j]) {
				t.Fatalf("%d.%d. expected %d output, got %d", i, j, len(scenario.out.atTime[j]), len(actual))
			}

			for k, value := range scenario.out.atTime[j] {
				if value.Value != actual[k].Value {
					t.Fatalf("%d.%d.%d expected %v value, got %v", i, j, k, value.Value, actual[k].Value)
				}
				if !value.Timestamp.Equal(actual[k].Timestamp) {
					t.Fatalf("%d.%d.%d expected %s timestamp, got %s", i, j, k, value.Timestamp, actual[k].Timestamp)
				}
			}
		}

		closer.Close()
	}
}

func TestMakeViewFlush(t *testing.T) {
	testMakeView(t, true)
}

func BenchmarkMakeViewFlush(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMakeView(b, true)
	}
}

func TestMakeViewNoFlush(t *testing.T) {
	testMakeView(t, false)
}

func BenchmarkMakeViewNoFlush(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMakeView(b, false)
	}
}

func TestGetAllValuesForLabel(t *testing.T) {
	type in struct {
		metricName     string
		appendToMemory bool
		appendToDisk   bool
	}

	scenarios := []struct {
		in  []in
		out []string
	}{
		{
		// Empty case.
		}, {
			in: []in{
				{
					metricName:     "request_count",
					appendToMemory: false,
					appendToDisk:   true,
				},
			},
			out: []string{
				"request_count",
			},
		}, {
			in: []in{
				{
					metricName:     "request_count",
					appendToMemory: true,
					appendToDisk:   false,
				},
				{
					metricName:     "start_time",
					appendToMemory: false,
					appendToDisk:   true,
				},
			},
			out: []string{
				"request_count",
				"start_time",
			},
		}, {
			in: []in{
				{
					metricName:     "request_count",
					appendToMemory: true,
					appendToDisk:   true,
				},
				{
					metricName:     "start_time",
					appendToMemory: true,
					appendToDisk:   true,
				},
			},
			out: []string{
				"request_count",
				"start_time",
			},
		},
	}

	for i, scenario := range scenarios {
		tiered, closer := NewTestTieredStorage(t)
		for j, metric := range scenario.in {
			sample := clientmodel.Sample{
				Metric: clientmodel.Metric{model.MetricNameLabel: model.LabelValue(metric.metricName)},
			}
			if metric.appendToMemory {
				if err := tiered.memoryArena.AppendSample(sample); err != nil {
					t.Fatalf("%d.%d. failed to add fixture data: %s", i, j, err)
				}
			}
			if metric.appendToDisk {
				if err := tiered.DiskStorage.AppendSample(sample); err != nil {
					t.Fatalf("%d.%d. failed to add fixture data: %s", i, j, err)
				}
			}
		}
		metricNames, err := tiered.GetAllValuesForLabel(model.MetricNameLabel)
		closer.Close()
		if err != nil {
			t.Fatalf("%d. Error getting metric names: %s", i, err)
		}
		if len(metricNames) != len(scenario.out) {
			t.Fatalf("%d. Expected metric count %d, got %d", i, len(scenario.out), len(metricNames))
		}

		sort.Sort(metricNames)
		for j, expected := range scenario.out {
			if expected != string(metricNames[j]) {
				t.Fatalf("%d.%d. Expected metric %s, got %s", i, j, expected, metricNames[j])
			}
		}
	}
}

func TestGetFingerprintsForLabelSet(t *testing.T) {
	tiered, closer := NewTestTieredStorage(t)
	defer closer.Close()
	memorySample := clientmodel.Sample{
		Metric: clientmodel.Metric{model.MetricNameLabel: "http_requests", "method": "/foo"},
	}
	diskSample := clientmodel.Sample{
		Metric: clientmodel.Metric{model.MetricNameLabel: "http_requests", "method": "/bar"},
	}
	if err := tiered.memoryArena.AppendSample(memorySample); err != nil {
		t.Fatalf("Failed to add fixture data: %s", err)
	}
	if err := tiered.DiskStorage.AppendSample(diskSample); err != nil {
		t.Fatalf("Failed to add fixture data: %s", err)
	}
	tiered.Flush()

	scenarios := []struct {
		labels  clientmodel.LabelSet
		fpCount int
	}{
		{
			labels:  clientmodel.LabelSet{},
			fpCount: 0,
		}, {
			labels: clientmodel.LabelSet{
				model.MetricNameLabel: "http_requests",
			},
			fpCount: 2,
		}, {
			labels: clientmodel.LabelSet{
				model.MetricNameLabel: "http_requests",
				"method":              "/foo",
			},
			fpCount: 1,
		}, {
			labels: clientmodel.LabelSet{
				model.MetricNameLabel: "http_requests",
				"method":              "/bar",
			},
			fpCount: 1,
		}, {
			labels: clientmodel.LabelSet{
				model.MetricNameLabel: "http_requests",
				"method":              "/baz",
			},
			fpCount: 0,
		},
	}

	for i, scenario := range scenarios {
		fingerprints, err := tiered.GetFingerprintsForLabelSet(scenario.labels)
		if err != nil {
			t.Fatalf("%d. Error getting metric names: %s", i, err)
		}
		if len(fingerprints) != scenario.fpCount {
			t.Fatalf("%d. Expected metric count %d, got %d", i, scenario.fpCount, len(fingerprints))
		}
	}
}

func testTruncateBefore(t test.Tester) {
	type in struct {
		values model.Values
		time   time.Time
	}
	instant := time.Now()
	var scenarios = []struct {
		in  in
		out model.Values
	}{
		{
			in: in{
				time: instant,
				values: model.Values{
					{
						Value:     0,
						Timestamp: instant,
					},
					{
						Value:     1,
						Timestamp: instant.Add(time.Second),
					},
					{
						Value:     2,
						Timestamp: instant.Add(2 * time.Second),
					},
					{
						Value:     3,
						Timestamp: instant.Add(3 * time.Second),
					},
					{
						Value:     4,
						Timestamp: instant.Add(4 * time.Second),
					},
				},
			},
			out: model.Values{
				{
					Value:     0,
					Timestamp: instant,
				},
				{
					Value:     1,
					Timestamp: instant.Add(time.Second),
				},
				{
					Value:     2,
					Timestamp: instant.Add(2 * time.Second),
				},
				{
					Value:     3,
					Timestamp: instant.Add(3 * time.Second),
				},
				{
					Value:     4,
					Timestamp: instant.Add(4 * time.Second),
				},
			},
		},
		{
			in: in{
				time: instant.Add(2 * time.Second),
				values: model.Values{
					{
						Value:     0,
						Timestamp: instant,
					},
					{
						Value:     1,
						Timestamp: instant.Add(time.Second),
					},
					{
						Value:     2,
						Timestamp: instant.Add(2 * time.Second),
					},
					{
						Value:     3,
						Timestamp: instant.Add(3 * time.Second),
					},
					{
						Value:     4,
						Timestamp: instant.Add(4 * time.Second),
					},
				},
			},
			out: model.Values{
				{
					Value:     1,
					Timestamp: instant.Add(time.Second),
				},
				{
					Value:     2,
					Timestamp: instant.Add(2 * time.Second),
				},
				{
					Value:     3,
					Timestamp: instant.Add(3 * time.Second),
				},
				{
					Value:     4,
					Timestamp: instant.Add(4 * time.Second),
				},
			},
		},
		{
			in: in{
				time: instant.Add(5 * time.Second),
				values: model.Values{
					{
						Value:     0,
						Timestamp: instant,
					},
					{
						Value:     1,
						Timestamp: instant.Add(time.Second),
					},
					{
						Value:     2,
						Timestamp: instant.Add(2 * time.Second),
					},
					{
						Value:     3,
						Timestamp: instant.Add(3 * time.Second),
					},
					{
						Value:     4,
						Timestamp: instant.Add(4 * time.Second),
					},
				},
			},
			out: model.Values{
				// Preserve the last value in case it needs to be used for the next set.
				{
					Value:     4,
					Timestamp: instant.Add(4 * time.Second),
				},
			},
		},
	}

	for i, scenario := range scenarios {
		actual := chunk(scenario.in.values).TruncateBefore(scenario.in.time)

		if len(actual) != len(scenario.out) {
			t.Fatalf("%d. expected length of %d, got %d", i, len(scenario.out), len(actual))
		}

		for j, actualValue := range actual {
			if !actualValue.Equal(scenario.out[j]) {
				t.Fatalf("%d.%d. expected %s, got %s", i, j, scenario.out[j], actualValue)
			}
		}
	}
}

func TestTruncateBefore(t *testing.T) {
	testTruncateBefore(t)
}
