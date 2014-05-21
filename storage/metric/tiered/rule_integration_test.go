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

package tiered

import (
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility/test"
)

func GetValueAtTimeTests(persistenceMaker func() (metric.ViewablePersistence, test.Closer), t testing.TB) {
	type value struct {
		year  int
		month time.Month
		day   int
		hour  int
		value clientmodel.SampleValue
	}

	type input struct {
		year  int
		month time.Month
		day   int
		hour  int
	}

	type output []clientmodel.SampleValue

	type behavior struct {
		name   string
		input  input
		output output
	}

	var contexts = []struct {
		name      string
		values    []value
		behaviors []behavior
	}{
		{
			name:   "no values",
			values: []value{},
			behaviors: []behavior{
				{
					name: "random target",
					input: input{
						year:  1984,
						month: 3,
						day:   30,
						hour:  0,
					},
				},
			},
		},
		{
			name: "singleton",
			values: []value{
				{
					year:  1984,
					month: 3,
					day:   30,
					hour:  0,
					value: 0,
				},
			},
			behaviors: []behavior{
				{
					name: "exact",
					input: input{
						year:  1984,
						month: 3,
						day:   30,
						hour:  0,
					},
					output: output{
						0,
					},
				},
				{
					name: "before",
					input: input{
						year:  1984,
						month: 3,
						day:   29,
						hour:  0,
					},
					output: output{
						0,
					},
				},
				{
					name: "after",
					input: input{
						year:  1984,
						month: 3,
						day:   31,
						hour:  0,
					},
					output: output{
						0,
					},
				},
			},
		},
		{
			name: "double",
			values: []value{
				{
					year:  1984,
					month: 3,
					day:   30,
					hour:  0,
					value: 0,
				},
				{
					year:  1985,
					month: 3,
					day:   30,
					hour:  0,
					value: 1,
				},
			},
			behaviors: []behavior{
				{
					name: "exact first",
					input: input{
						year:  1984,
						month: 3,
						day:   30,
						hour:  0,
					},
					output: output{
						0,
					},
				},
				{
					name: "exact second",
					input: input{
						year:  1985,
						month: 3,
						day:   30,
						hour:  0,
					},
					output: output{
						1,
					},
				},
				{
					name: "before first",
					input: input{
						year:  1983,
						month: 9,
						day:   29,
						hour:  12,
					},
					output: output{
						0,
					},
				},
				{
					name: "after second",
					input: input{
						year:  1985,
						month: 9,
						day:   28,
						hour:  12,
					},
					output: output{
						1,
					},
				},
				{
					name: "middle",
					input: input{
						year:  1984,
						month: 9,
						day:   28,
						hour:  12,
					},
					output: output{
						0,
						1,
					},
				},
			},
		},
		{
			name: "triple",
			values: []value{
				{
					year:  1984,
					month: 3,
					day:   30,
					hour:  0,
					value: 0,
				},
				{
					year:  1985,
					month: 3,
					day:   30,
					hour:  0,
					value: 1,
				},
				{
					year:  1986,
					month: 3,
					day:   30,
					hour:  0,
					value: 2,
				},
			},
			behaviors: []behavior{
				{
					name: "exact first",
					input: input{
						year:  1984,
						month: 3,
						day:   30,
						hour:  0,
					},
					output: output{
						0,
					},
				},
				{
					name: "exact second",
					input: input{
						year:  1985,
						month: 3,
						day:   30,
						hour:  0,
					},
					output: output{
						1,
					},
				},
				{
					name: "exact third",
					input: input{
						year:  1986,
						month: 3,
						day:   30,
						hour:  0,
					},
					output: output{
						2,
					},
				},
				{
					name: "before first",
					input: input{
						year:  1983,
						month: 9,
						day:   29,
						hour:  12,
					},
					output: output{
						0,
					},
				},
				{
					name: "after third",
					input: input{
						year:  1986,
						month: 9,
						day:   28,
						hour:  12,
					},
					output: output{
						2,
					},
				},
				{
					name: "first middle",
					input: input{
						year:  1984,
						month: 9,
						day:   28,
						hour:  12,
					},
					output: output{
						0,
						1,
					},
				},
				{
					name: "second middle",
					input: input{
						year:  1985,
						month: 9,
						day:   28,
						hour:  12,
					},
					output: output{
						1,
						2,
					},
				},
			},
		},
	}

	for i, context := range contexts {
		// Wrapping in function to enable garbage collection of resources.
		func() {
			p, closer := persistenceMaker()

			defer closer.Close()
			defer p.Close()

			m := clientmodel.Metric{
				clientmodel.MetricNameLabel: "age_in_years",
			}

			for _, value := range context.values {
				testAppendSamples(p, &clientmodel.Sample{
					Value:     clientmodel.SampleValue(value.value),
					Timestamp: clientmodel.TimestampFromTime(time.Date(value.year, value.month, value.day, value.hour, 0, 0, 0, time.UTC)),
					Metric:    m,
				}, t)
			}

			for j, behavior := range context.behaviors {
				input := behavior.input
				time := clientmodel.TimestampFromTime(time.Date(input.year, input.month, input.day, input.hour, 0, 0, 0, time.UTC))
				fingerprint := &clientmodel.Fingerprint{}
				fingerprint.LoadFromMetric(m)
				actual := p.GetValueAtTime(fingerprint, time)

				if len(behavior.output) != len(actual) {
					t.Fatalf("%d.%d(%s.%s). Expected %d samples but got: %v\n", i, j, context.name, behavior.name, len(behavior.output), actual)
				}
				for k, samplePair := range actual {
					if samplePair.Value != behavior.output[k] {
						t.Fatalf("%d.%d.%d(%s.%s). Expected %s but got %s\n", i, j, k, context.name, behavior.name, behavior.output[k], samplePair)

					}
				}
			}
		}()
	}
}

func GetRangeValuesTests(persistenceMaker func() (metric.ViewablePersistence, test.Closer), onlyBoundaries bool, t testing.TB) {
	type value struct {
		year  int
		month time.Month
		day   int
		hour  int
		value clientmodel.SampleValue
	}

	type input struct {
		openYear  int
		openMonth time.Month
		openDay   int
		openHour  int
		endYear   int
		endMonth  time.Month
		endDay    int
		endHour   int
	}

	type output struct {
		year  int
		month time.Month
		day   int
		hour  int
		value clientmodel.SampleValue
	}

	type behavior struct {
		name   string
		input  input
		output []output
	}

	var contexts = []struct {
		name      string
		values    []value
		behaviors []behavior
	}{
		{
			name:   "no values",
			values: []value{},
			behaviors: []behavior{
				{
					name: "non-existent interval",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
				},
			},
		},
		{
			name: "singleton value",
			values: []value{
				{
					year:  1984,
					month: 3,
					day:   30,
					hour:  0,
					value: 0,
				},
			},
			behaviors: []behavior{
				{
					name: "start on first value",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
					output: []output{
						{
							year:  1984,
							month: 3,
							day:   30,
							hour:  0,
							value: 0,
						},
					},
				},
				{
					name: "end on first value",
					input: input{
						openYear:  1983,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1984,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
					output: []output{
						{
							year:  1984,
							month: 3,
							day:   30,
							hour:  0,
							value: 0,
						},
					},
				},
				{
					name: "overlap on first value",
					input: input{
						openYear:  1983,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
					output: []output{
						{
							year:  1984,
							month: 3,
							day:   30,
							hour:  0,
							value: 0,
						},
					},
				},
			},
		},
		{
			name: "two values",
			values: []value{
				{
					year:  1984,
					month: 3,
					day:   30,
					hour:  0,
					value: 0,
				},
				{
					year:  1985,
					month: 3,
					day:   30,
					hour:  0,
					value: 1,
				},
			},
			behaviors: []behavior{
				{
					name: "start on first value",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
					output: []output{
						{
							year:  1984,
							month: 3,
							day:   30,
							hour:  0,
							value: 0,
						},
						{
							year:  1985,
							month: 3,
							day:   30,
							hour:  0,
							value: 1,
						},
					},
				},
				{
					name: "start on second value",
					input: input{
						openYear:  1985,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1986,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
					output: []output{
						{
							year:  1985,
							month: 3,
							day:   30,
							hour:  0,
							value: 1,
						},
					},
				},
				{
					name: "end on first value",
					input: input{
						openYear:  1983,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1984,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
					output: []output{
						{
							year:  1984,
							month: 3,
							day:   30,
							hour:  0,
							value: 0,
						},
					},
				},
				{
					name: "end on second value",
					input: input{
						openYear:  1985,
						openMonth: 1,
						openDay:   1,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
					output: []output{
						{
							year:  1985,
							month: 3,
							day:   30,
							hour:  0,
							value: 1,
						},
					},
				},
				{
					name: "overlap on values",
					input: input{
						openYear:  1983,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1986,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
					output: []output{
						{
							year:  1984,
							month: 3,
							day:   30,
							hour:  0,
							value: 0,
						},
						{
							year:  1985,
							month: 3,
							day:   30,
							hour:  0,
							value: 1,
						},
					},
				},
			},
		},
		{
			name: "three values",
			values: []value{
				{
					year:  1984,
					month: 3,
					day:   30,
					hour:  0,
					value: 0,
				},
				{
					year:  1985,
					month: 3,
					day:   30,
					hour:  0,
					value: 1,
				},
				{
					year:  1986,
					month: 3,
					day:   30,
					hour:  0,
					value: 2,
				},
			},
			behaviors: []behavior{
				{
					name: "start on first value",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
					output: []output{
						{
							year:  1984,
							month: 3,
							day:   30,
							hour:  0,
							value: 0,
						},
						{
							year:  1985,
							month: 3,
							day:   30,
							hour:  0,
							value: 1,
						},
					},
				},
				{
					name: "start on second value",
					input: input{
						openYear:  1985,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1986,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
					output: []output{
						{
							year:  1985,
							month: 3,
							day:   30,
							hour:  0,
							value: 1,
						},
						{
							year:  1986,
							month: 3,
							day:   30,
							hour:  0,
							value: 2,
						},
					},
				},
				{
					name: "end on first value",
					input: input{
						openYear:  1983,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1984,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
					output: []output{
						{
							year:  1984,
							month: 3,
							day:   30,
							hour:  0,
							value: 0,
						},
					},
				},
				{
					name: "end on second value",
					input: input{
						openYear:  1985,
						openMonth: 1,
						openDay:   1,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
					output: []output{
						{
							year:  1985,
							month: 3,
							day:   30,
							hour:  0,
							value: 1,
						},
					},
				},
				{
					name: "overlap on values",
					input: input{
						openYear:  1983,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1986,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
					},
					output: []output{
						{
							year:  1984,
							month: 3,
							day:   30,
							hour:  0,
							value: 0,
						},
						{
							year:  1985,
							month: 3,
							day:   30,
							hour:  0,
							value: 1,
						},
						{
							year:  1986,
							month: 3,
							day:   30,
							hour:  0,
							value: 2,
						},
					},
				},
			},
		},
	}

	for i, context := range contexts {
		// Wrapping in function to enable garbage collection of resources.
		func() {
			p, closer := persistenceMaker()

			defer closer.Close()
			defer p.Close()

			m := clientmodel.Metric{
				clientmodel.MetricNameLabel: "age_in_years",
			}

			for _, value := range context.values {
				testAppendSamples(p, &clientmodel.Sample{
					Value:     clientmodel.SampleValue(value.value),
					Timestamp: clientmodel.TimestampFromTime(time.Date(value.year, value.month, value.day, value.hour, 0, 0, 0, time.UTC)),
					Metric:    m,
				}, t)
			}

			for j, behavior := range context.behaviors {
				input := behavior.input
				open := clientmodel.TimestampFromTime(time.Date(input.openYear, input.openMonth, input.openDay, input.openHour, 0, 0, 0, time.UTC))
				end := clientmodel.TimestampFromTime(time.Date(input.endYear, input.endMonth, input.endDay, input.endHour, 0, 0, 0, time.UTC))
				in := metric.Interval{
					OldestInclusive: open,
					NewestInclusive: end,
				}

				actualValues := metric.Values{}
				expectedValues := []output{}
				fp := &clientmodel.Fingerprint{}
				fp.LoadFromMetric(m)
				if onlyBoundaries {
					actualValues = p.GetBoundaryValues(fp, in)
					l := len(behavior.output)
					if l == 1 {
						expectedValues = behavior.output[0:1]
					}
					if l > 1 {
						expectedValues = append(behavior.output[0:1], behavior.output[l-1])
					}
				} else {
					actualValues = p.GetRangeValues(fp, in)
					expectedValues = behavior.output
				}

				if actualValues == nil && len(expectedValues) != 0 {
					t.Fatalf("%d.%d(%s). Expected %v but got: %v\n", i, j, behavior.name, expectedValues, actualValues)
				}

				if expectedValues == nil {
					if actualValues != nil {
						t.Fatalf("%d.%d(%s). Expected nil values but got: %s\n", i, j, behavior.name, actualValues)
					}
				} else {
					if len(expectedValues) != len(actualValues) {
						t.Fatalf("%d.%d(%s). Expected length %d but got: %d\n", i, j, behavior.name, len(expectedValues), len(actualValues))
					}

					for k, actual := range actualValues {
						expected := expectedValues[k]

						if actual.Value != clientmodel.SampleValue(expected.value) {
							t.Fatalf("%d.%d.%d(%s). Expected %v but got: %v\n", i, j, k, behavior.name, expected.value, actual.Value)
						}

						if actual.Timestamp.Time().Year() != expected.year {
							t.Fatalf("%d.%d.%d(%s). Expected %d but got: %d\n", i, j, k, behavior.name, expected.year, actual.Timestamp.Time().Year())
						}
						if actual.Timestamp.Time().Month() != expected.month {
							t.Fatalf("%d.%d.%d(%s). Expected %d but got: %d\n", i, j, k, behavior.name, expected.month, actual.Timestamp.Time().Month())
						}
						// XXX: Find problem here.
						// Mismatches occur in this and have for a long time in the LevelDB
						// case, however not im-memory.
						//
						// if actual.Timestamp.Day() != expected.day {
						// 	t.Fatalf("%d.%d.%d(%s). Expected %d but got: %d\n", i, j, k, behavior.name, expected.day, actual.Timestamp.Day())
						// }
						// if actual.Timestamp.Hour() != expected.hour {
						// 	t.Fatalf("%d.%d.%d(%s). Expected %d but got: %d\n", i, j, k, behavior.name, expected.hour, actual.Timestamp.Hour())
						// }
					}
				}
			}
		}()
	}
}

// Test Definitions Follow

func testMemoryGetValueAtTime(t testing.TB) {
	persistenceMaker := func() (metric.ViewablePersistence, test.Closer) {
		return NewMemorySeriesStorage(MemorySeriesOptions{}), test.NilCloser
	}

	GetValueAtTimeTests(persistenceMaker, t)
}

func TestMemoryGetValueAtTime(t *testing.T) {
	testMemoryGetValueAtTime(t)
}

func BenchmarkMemoryGetValueAtTime(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryGetValueAtTime(b)
	}
}

func TestMemoryGetBoundaryValues(t *testing.T) {
	testMemoryGetBoundaryValues(t)
}

func BenchmarkMemoryGetBoundaryValues(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryGetBoundaryValues(b)
	}
}

func testMemoryGetRangeValues(t testing.TB) {
	persistenceMaker := func() (metric.ViewablePersistence, test.Closer) {
		return NewMemorySeriesStorage(MemorySeriesOptions{}), test.NilCloser
	}

	GetRangeValuesTests(persistenceMaker, false, t)
}

func testMemoryGetBoundaryValues(t testing.TB) {
	persistenceMaker := func() (metric.ViewablePersistence, test.Closer) {
		return NewMemorySeriesStorage(MemorySeriesOptions{}), test.NilCloser
	}

	GetRangeValuesTests(persistenceMaker, true, t)
}

func TestMemoryGetRangeValues(t *testing.T) {
	testMemoryGetRangeValues(t)
}

func BenchmarkMemoryGetRangeValues(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryGetRangeValues(b)
	}
}
