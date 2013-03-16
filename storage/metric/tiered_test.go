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
	"fmt"
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/utility/test"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func buildSamples(from, to time.Time, interval time.Duration, m model.Metric) (v []model.Sample) {
	i := model.SampleValue(0)

	for from.Before(to) {
		v = append(v, model.Sample{
			Metric:    m,
			Value:     i,
			Timestamp: from,
		})

		from = from.Add(interval)
		i++
	}

	return
}

func testMakeView(t test.Tester) {
	type in struct {
		atTime     []getValuesAtTimeOp
		atInterval []getValuesAtIntervalOp
		alongRange []getValuesAlongRangeOp
	}

	type out struct {
		atTime     [][]model.SamplePair
		atInterval [][]model.SamplePair
		alongRange [][]model.SamplePair
	}
	var (
		instant     = time.Date(1984, 3, 30, 0, 0, 0, 0, time.Local)
		metric      = model.Metric{"name": "request_count"}
		fingerprint = model.NewFingerprintFromMetric(metric)
		scenarios   = []struct {
			data []model.Sample
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
					atTime: [][]model.SamplePair{{}},
				},
			},
			// Single sample, query asks for exact sample time.
			{
				data: []model.Sample{
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
					atTime: [][]model.SamplePair{
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
				data: []model.Sample{
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
					atTime: [][]model.SamplePair{
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
				data: []model.Sample{
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
					atTime: [][]model.SamplePair{
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
				data: []model.Sample{
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
					atTime: [][]model.SamplePair{
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
				data: []model.Sample{
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
					atTime: [][]model.SamplePair{
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
				data: []model.Sample{
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
					atTime: [][]model.SamplePair{
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
				data: []model.Sample{
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
					atTime: [][]model.SamplePair{
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
					atTime: [][]model.SamplePair{
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
		var (
			temporary, _ = ioutil.TempDir("", "test_make_view")
			tiered       = NewTieredStorage(5000000, 2500, 1000, 5*time.Second, 15*time.Second, 0*time.Second, temporary)
		)

		if tiered == nil {
			t.Fatalf("%d. tiered == nil", i)
		}

		go tiered.Serve()
		defer tiered.Drain()

		defer func() {
			os.RemoveAll(temporary)
		}()

		for j, datum := range scenario.data {
			err := tiered.AppendSample(datum)
			if err != nil {
				t.Fatalf("%d.%d. failed to add fixture data: %s", i, j, err)
			}
		}

		start := time.Now()
		tiered.Flush()
		fmt.Printf("Took %s to flush %d items...\n", time.Since(start), len(scenario.data))

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

		v, err := tiered.MakeView(requestBuilder, time.Second*5)

		if err != nil {
			t.Fatalf("%d. failed due to %s", i, err)
		}

		for j, atTime := range scenario.in.atTime {
			actual := v.GetValueAtTime(fingerprint, atTime.time)

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

		tiered.Drain()
	}
}

func TestMakeView(t *testing.T) {
	testMakeView(t)
}

func BenchmarkMakeView(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMakeView(b)
	}
}
