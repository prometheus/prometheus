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
	"github.com/prometheus/prometheus/utility/test"
	"io"
	"io/ioutil"
	"testing"
	"time"
)

func GetValueAtTimeTests(persistenceMaker func() (MetricPersistence, io.Closer), t test.Tester) {
	type value struct {
		year  int
		month time.Month
		day   int
		hour  int
		value float32
	}

	type input struct {
		year      int
		month     time.Month
		day       int
		hour      int
		staleness time.Duration
	}

	type output struct {
		value model.SampleValue
	}

	type behavior struct {
		name   string
		input  input
		output *output
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
						year:      1984,
						month:     3,
						day:       30,
						hour:      0,
						staleness: time.Duration(0),
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
					name: "exact without staleness policy",
					input: input{
						year:      1984,
						month:     3,
						day:       30,
						hour:      0,
						staleness: time.Duration(0),
					},
					output: &output{
						value: 0,
					},
				},
				{
					name: "exact with staleness policy",
					input: input{
						year:      1984,
						month:     3,
						day:       30,
						hour:      0,
						staleness: time.Duration(365*24) * time.Hour,
					},
					output: &output{
						value: 0,
					},
				},
				{
					name: "before without staleness policy",
					input: input{
						year:      1984,
						month:     3,
						day:       29,
						hour:      0,
						staleness: time.Duration(0),
					},
				},
				{
					name: "before within staleness policy",
					input: input{
						year:      1984,
						month:     3,
						day:       29,
						hour:      0,
						staleness: time.Duration(365*24) * time.Hour,
					},
				},
				{
					name: "before outside staleness policy",
					input: input{
						year:      1984,
						month:     3,
						day:       29,
						hour:      0,
						staleness: time.Duration(1) * time.Hour,
					},
				},
				{
					name: "after without staleness policy",
					input: input{
						year:      1984,
						month:     3,
						day:       31,
						hour:      0,
						staleness: time.Duration(0),
					},
				},
				{
					name: "after within staleness policy",
					input: input{
						year:      1984,
						month:     3,
						day:       31,
						hour:      0,
						staleness: time.Duration(365*24) * time.Hour,
					},
					output: &output{
						value: 0,
					},
				},
				{
					name: "after outside staleness policy",
					input: input{
						year:      1984,
						month:     4,
						day:       7,
						hour:      0,
						staleness: time.Duration(7*24) * time.Hour,
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
					name: "exact first without staleness policy",
					input: input{
						year:      1984,
						month:     3,
						day:       30,
						hour:      0,
						staleness: time.Duration(0),
					},
					output: &output{
						value: 0,
					},
				},
				{
					name: "exact first with staleness policy",
					input: input{
						year:      1984,
						month:     3,
						day:       30,
						hour:      0,
						staleness: time.Duration(365*24) * time.Hour,
					},
					output: &output{
						value: 0,
					},
				},
				{
					name: "exact second without staleness policy",
					input: input{
						year:      1985,
						month:     3,
						day:       30,
						hour:      0,
						staleness: time.Duration(0),
					},
					output: &output{
						value: 1,
					},
				},
				{
					name: "exact second with staleness policy",
					input: input{
						year:      1985,
						month:     3,
						day:       30,
						hour:      0,
						staleness: time.Duration(365*24) * time.Hour,
					},
					output: &output{
						value: 1,
					},
				},
				{
					name: "before first without staleness policy",
					input: input{
						year:      1983,
						month:     9,
						day:       29,
						hour:      12,
						staleness: time.Duration(0),
					},
				},
				{
					name: "before first with staleness policy",
					input: input{
						year:      1983,
						month:     9,
						day:       29,
						hour:      12,
						staleness: time.Duration(365*24) * time.Hour,
					},
				},
				{
					name: "after second with staleness policy",
					input: input{
						year:      1985,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(365*24) * time.Hour,
					},
					output: &output{
						value: 1,
					},
				},
				{
					name: "after second without staleness policy",
					input: input{
						year:      1985,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(0),
					},
				},
				{
					name: "middle without staleness policy",
					input: input{
						year:      1984,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(0),
					},
				},
				{
					name: "middle with insufficient staleness policy",
					input: input{
						year:      1984,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(364*24) * time.Hour,
					},
				},
				{
					name: "middle with sufficient staleness policy",
					input: input{
						year:      1984,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(365*24) * time.Hour,
					},
					output: &output{
						value: 0.5,
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
					name: "exact first without staleness policy",
					input: input{
						year:      1984,
						month:     3,
						day:       30,
						hour:      0,
						staleness: time.Duration(0),
					},
					output: &output{
						value: 0,
					},
				},
				{
					name: "exact first with staleness policy",
					input: input{
						year:      1984,
						month:     3,
						day:       30,
						hour:      0,
						staleness: time.Duration(365*24) * time.Hour,
					},
					output: &output{
						value: 0,
					},
				},
				{
					name: "exact second without staleness policy",
					input: input{
						year:      1985,
						month:     3,
						day:       30,
						hour:      0,
						staleness: time.Duration(0),
					},
					output: &output{
						value: 1,
					},
				},
				{
					name: "exact second with staleness policy",
					input: input{
						year:      1985,
						month:     3,
						day:       30,
						hour:      0,
						staleness: time.Duration(365*24) * time.Hour,
					},
					output: &output{
						value: 1,
					},
				},
				{
					name: "exact third without staleness policy",
					input: input{
						year:      1986,
						month:     3,
						day:       30,
						hour:      0,
						staleness: time.Duration(0),
					},
					output: &output{
						value: 2,
					},
				},
				{
					name: "exact third with staleness policy",
					input: input{
						year:      1986,
						month:     3,
						day:       30,
						hour:      0,
						staleness: time.Duration(365*24) * time.Hour,
					},
					output: &output{
						value: 2,
					},
				},
				{
					name: "before first without staleness policy",
					input: input{
						year:      1983,
						month:     9,
						day:       29,
						hour:      12,
						staleness: time.Duration(0),
					},
				},
				{
					name: "before first with staleness policy",
					input: input{
						year:      1983,
						month:     9,
						day:       29,
						hour:      12,
						staleness: time.Duration(365*24) * time.Hour,
					},
				},
				{
					name: "after third within staleness policy",
					input: input{
						year:      1986,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(365*24) * time.Hour,
					},
					output: &output{
						value: 2,
					},
				},
				{
					name: "after third outside staleness policy",
					input: input{
						year:      1986,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(1*24) * time.Hour,
					},
				},
				{
					name: "after third without staleness policy",
					input: input{
						year:      1986,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(0),
					},
				},
				{
					name: "first middle without staleness policy",
					input: input{
						year:      1984,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(0),
					},
				},
				{
					name: "first middle with insufficient staleness policy",
					input: input{
						year:      1984,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(364*24) * time.Hour,
					},
				},
				{
					name: "first middle with sufficient staleness policy",
					input: input{
						year:      1984,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(365*24) * time.Hour,
					},
					output: &output{
						value: 0.5,
					},
				},
				{
					name: "second middle without staleness policy",
					input: input{
						year:      1985,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(0),
					},
				},
				{
					name: "second middle with insufficient staleness policy",
					input: input{
						year:      1985,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(364*24) * time.Hour,
					},
				},
				{
					name: "second middle with sufficient staleness policy",
					input: input{
						year:      1985,
						month:     9,
						day:       28,
						hour:      12,
						staleness: time.Duration(365*24) * time.Hour,
					},
					output: &output{
						value: 1.5,
					},
				},
			},
		},
	}

	for i, context := range contexts {
		// Wrapping in function to enable garbage collection of resources.
		func() {
			p, closer := persistenceMaker()

			defer func() {
				err := p.Close()
				if err != nil {
					t.Fatalf("Encountered anomaly closing persistence: %q\n", err)
				}

				err = closer.Close()
				if err != nil {
					t.Fatalf("Encountered anomaly purging persistence: %q\n", err)
				}
			}()

			m := model.Metric{
				"name": "age_in_years",
			}

			for _, value := range context.values {
				testAppendSample(p, model.Sample{
					Value:     model.SampleValue(value.value),
					Timestamp: time.Date(value.year, value.month, value.day, value.hour, 0, 0, 0, time.UTC),
					Metric:    m,
				}, t)
			}

			for j, behavior := range context.behaviors {
				input := behavior.input
				time := time.Date(input.year, input.month, input.day, input.hour, 0, 0, 0, time.UTC)
				sp := StalenessPolicy{
					DeltaAllowance: input.staleness,
				}

				actual, err := p.GetValueAtTime(model.NewFingerprintFromMetric(m), time, sp)
				if err != nil {
					t.Fatalf("%d.%d(%s). Could not query for value: %q\n", i, j, behavior.name, err)
				}

				if behavior.output == nil {
					if actual != nil {
						t.Fatalf("%d.%d(%s). Expected nil but got: %q\n", i, j, behavior.name, actual)
					}
				} else {
					if actual == nil {
						t.Fatalf("%d.%d(%s). Expected %s but got nil\n", i, j, behavior.name, behavior.output)
					} else {
						if actual.Value != behavior.output.value {
							t.Fatalf("%d.%d(%s). Expected %s but got %s\n", i, j, behavior.name, behavior.output, actual)

						}
					}
				}
			}
		}()
	}
}

func GetBoundaryValuesTests(persistenceMaker func() (MetricPersistence, io.Closer), t test.Tester) {
	type value struct {
		year  int
		month time.Month
		day   int
		hour  int
		value float32
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
		staleness time.Duration
	}

	type output struct {
		open model.SampleValue
		end  model.SampleValue
	}

	type behavior struct {
		name   string
		input  input
		output *output
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
					name: "non-existent interval without staleness policy",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
						staleness: time.Duration(0),
					},
				},
				{
					name: "non-existent interval with staleness policy",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
						staleness: time.Duration(365*24) * time.Hour,
					},
				},
			},
		},
		{
			name: "single value",
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
					name: "on start but missing end without staleness policy",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
						staleness: time.Duration(0),
					},
				},
				{
					name: "non-existent interval after within staleness policy",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   31,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
						staleness: time.Duration(4380) * time.Hour,
					},
				},
				{
					name: "non-existent interval after without staleness policy",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   31,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
						staleness: time.Duration(0),
					},
				},
				{
					name: "non-existent interval before with staleness policy",
					input: input{
						openYear:  1983,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1984,
						endMonth:  3,
						endDay:    29,
						endHour:   0,
						staleness: time.Duration(365*24) * time.Hour,
					},
				},
				{
					name: "non-existent interval before without staleness policy",
					input: input{
						openYear:  1983,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1984,
						endMonth:  3,
						endDay:    29,
						endHour:   0,
						staleness: time.Duration(0),
					},
				},
				{
					name: "on end but not start without staleness policy",
					input: input{
						openYear:  1983,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1984,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
						staleness: time.Duration(0),
					},
				},
				{
					name: "on end but not start without staleness policy",
					input: input{
						openYear:  1983,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1984,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
						staleness: time.Duration(365*24) * time.Hour,
					},
				},
				{
					name: "before point without staleness policy",
					input: input{
						openYear:  1982,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1983,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
						staleness: time.Duration(0),
					},
				},
				{
					name: "before point with staleness policy",
					input: input{
						openYear:  1982,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1983,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
						staleness: time.Duration(365*24) * time.Hour,
					},
				},
				{
					name: "after point without staleness policy",
					input: input{
						openYear:  1985,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1986,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
						staleness: time.Duration(0),
					},
				},
				{
					name: "after point with staleness policy",
					input: input{
						openYear:  1985,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1986,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
						staleness: time.Duration(365*24) * time.Hour,
					},
				},
				{
					name: "spanning point without staleness policy",
					input: input{
						openYear:  1983,
						openMonth: 9,
						openDay:   29,
						openHour:  12,
						endYear:   1984,
						endMonth:  9,
						endDay:    28,
						endHour:   12,
						staleness: time.Duration(0),
					},
				},
				{
					name: "spanning point with staleness policy",
					input: input{
						openYear:  1983,
						openMonth: 9,
						openDay:   29,
						openHour:  12,
						endYear:   1984,
						endMonth:  9,
						endDay:    28,
						endHour:   12,
						staleness: time.Duration(365*24) * time.Hour,
					},
				},
			},
		},
		{
			name: "double values",
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
					name: "on points without staleness policy",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
						staleness: time.Duration(0),
					},
					output: &output{
						open: 0,
						end:  1,
					},
				},
				{
					name: "on points with staleness policy",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1985,
						endMonth:  3,
						endDay:    30,
						endHour:   0,
						staleness: time.Duration(365*24) * time.Hour,
					},
					output: &output{
						open: 0,
						end:  1,
					},
				},
				{
					name: "on first before second outside of staleness",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1984,
						endMonth:  6,
						endDay:    29,
						endHour:   6,
						staleness: time.Duration(2190) * time.Hour,
					},
				},
				{
					name: "on first before second within staleness",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1984,
						endMonth:  6,
						endDay:    29,
						endHour:   6,
						staleness: time.Duration(356*24) * time.Hour,
					},
				},
				{
					name: "on first after second outside of staleness",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1985,
						endMonth:  6,
						endDay:    29,
						endHour:   6,
						staleness: time.Duration(1) * time.Hour,
					},
				},
				{
					name: "on first after second within staleness",
					input: input{
						openYear:  1984,
						openMonth: 3,
						openDay:   30,
						openHour:  0,
						endYear:   1985,
						endMonth:  6,
						endDay:    29,
						endHour:   6,
						staleness: time.Duration(356*24) * time.Hour,
					},
					output: &output{
						open: 0,
						end:  1,
					},
				},
			},
		},
	}

	for i, context := range contexts {
		// Wrapping in function to enable garbage collection of resources.
		func() {
			p, closer := persistenceMaker()

			defer func() {
				err := p.Close()
				if err != nil {
					t.Fatalf("Encountered anomaly closing persistence: %q\n", err)
				}
				err = closer.Close()
				if err != nil {
					t.Fatalf("Encountered anomaly purging persistence: %q\n", err)
				}
			}()

			m := model.Metric{
				"name": "age_in_years",
			}

			for _, value := range context.values {
				testAppendSample(p, model.Sample{
					Value:     model.SampleValue(value.value),
					Timestamp: time.Date(value.year, value.month, value.day, value.hour, 0, 0, 0, time.UTC),
					Metric:    m,
				}, t)
			}

			for j, behavior := range context.behaviors {
				input := behavior.input
				open := time.Date(input.openYear, input.openMonth, input.openDay, input.openHour, 0, 0, 0, time.UTC)
				end := time.Date(input.endYear, input.endMonth, input.endDay, input.endHour, 0, 0, 0, time.UTC)
				interval := model.Interval{
					OldestInclusive: open,
					NewestInclusive: end,
				}
				po := StalenessPolicy{
					DeltaAllowance: input.staleness,
				}

				openValue, endValue, err := p.GetBoundaryValues(model.NewFingerprintFromMetric(m), interval, po)
				if err != nil {
					t.Fatalf("%d.%d(%s). Could not query for value: %q\n", i, j, behavior.name, err)
				}

				if behavior.output == nil {
					if openValue != nil {
						t.Fatalf("%d.%d(%s). Expected open to be nil but got: %q\n", i, j, behavior.name, openValue)
					}
					if endValue != nil {
						t.Fatalf("%d.%d(%s). Expected end to be nil but got: %q\n", i, j, behavior.name, endValue)
					}
				} else {
					if openValue == nil {
						t.Fatalf("%d.%d(%s). Expected open to be %s but got nil\n", i, j, behavior.name, behavior.output)
					}
					if endValue == nil {
						t.Fatalf("%d.%d(%s). Expected end to be %s but got nil\n", i, j, behavior.name, behavior.output)
					}
					if openValue.Value != behavior.output.open {
						t.Fatalf("%d.%d(%s). Expected open to be %s but got %s\n", i, j, behavior.name, behavior.output.open, openValue.Value)
					}

					if endValue.Value != behavior.output.end {
						t.Fatalf("%d.%d(%s). Expected end to be %s but got %s\n", i, j, behavior.name, behavior.output.end, endValue.Value)
					}
				}
			}
		}()
	}
}

func GetRangeValuesTests(persistenceMaker func() (MetricPersistence, io.Closer), t test.Tester) {
	type value struct {
		year  int
		month time.Month
		day   int
		hour  int
		value float32
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
		value float32
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
	}

	for i, context := range contexts {
		// Wrapping in function to enable garbage collection of resources.
		func() {
			p, closer := persistenceMaker()

			defer func() {
				err := p.Close()
				if err != nil {
					t.Fatalf("Encountered anomaly closing persistence: %q\n", err)
				}
				err = closer.Close()
				if err != nil {
					t.Fatalf("Encountered anomaly purging persistence: %q\n", err)
				}
			}()

			m := model.Metric{
				"name": "age_in_years",
			}

			for _, value := range context.values {
				testAppendSample(p, model.Sample{
					Value:     model.SampleValue(value.value),
					Timestamp: time.Date(value.year, value.month, value.day, value.hour, 0, 0, 0, time.UTC),
					Metric:    m,
				}, t)
			}

			for j, behavior := range context.behaviors {
				input := behavior.input
				open := time.Date(input.openYear, input.openMonth, input.openDay, input.openHour, 0, 0, 0, time.UTC)
				end := time.Date(input.endYear, input.endMonth, input.endDay, input.endHour, 0, 0, 0, time.UTC)
				in := model.Interval{
					OldestInclusive: open,
					NewestInclusive: end,
				}

				values, err := p.GetRangeValues(model.NewFingerprintFromMetric(m), in)
				if err != nil {
					t.Fatalf("%d.%d(%s). Could not query for value: %q\n", i, j, behavior.name, err)
				}

				if values == nil && len(behavior.output) != 0 {
					t.Fatalf("%d.%d(%s). Expected %s but got: %s\n", i, j, behavior.name, behavior.output, values)
				}

				if behavior.output == nil {
					if values != nil {
						t.Fatalf("%d.%d(%s). Expected nil values but got: %s\n", i, j, behavior.name, values)
					}
				} else {
					if len(behavior.output) != len(values.Values) {
						t.Fatalf("%d.%d(%s). Expected length %d but got: %d\n", i, j, behavior.name, len(behavior.output), len(values.Values))
					}

					for k, actual := range values.Values {
						expected := behavior.output[k]

						if actual.Value != model.SampleValue(expected.value) {
							t.Fatalf("%d.%d.%d(%s). Expected %d but got: %d\n", i, j, k, behavior.name, expected.value, actual.Value)
						}

						if actual.Timestamp.Year() != expected.year {
							t.Fatalf("%d.%d.%d(%s). Expected %d but got: %d\n", i, j, k, behavior.name, expected.year, actual.Timestamp.Year())
						}
						if actual.Timestamp.Month() != expected.month {
							t.Fatalf("%d.%d.%d(%s). Expected %d but got: %d\n", i, j, k, behavior.name, expected.month, actual.Timestamp.Month())
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

func testLevelDBGetValueAtTime(t test.Tester) {
	persistenceMaker := buildLevelDBTestPersistencesMaker("get_value_at_time", t)
	GetValueAtTimeTests(persistenceMaker, t)
}

func TestLevelDBGetValueAtTime(t *testing.T) {
	testLevelDBGetValueAtTime(t)
}

func BenchmarkLevelDBGetValueAtTime(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBGetValueAtTime(b)
	}
}

func testLevelDBGetBoundaryValues(t test.Tester) {
	persistenceMaker := buildLevelDBTestPersistencesMaker("get_boundary_values", t)

	GetBoundaryValuesTests(persistenceMaker, t)
}

func TestLevelDBGetBoundaryValues(t *testing.T) {
	testLevelDBGetBoundaryValues(t)
}

func BenchmarkLevelDBGetBoundaryValues(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBGetBoundaryValues(b)
	}
}

func testLevelDBGetRangeValues(t test.Tester) {
	persistenceMaker := buildLevelDBTestPersistencesMaker("get_range_values", t)

	GetRangeValuesTests(persistenceMaker, t)
}

func TestLevelDBGetRangeValues(t *testing.T) {
	testLevelDBGetRangeValues(t)
}

func BenchmarkLevelDBGetRangeValues(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBGetRangeValues(b)
	}
}

func testMemoryGetValueAtTime(t test.Tester) {
	persistenceMaker := func() (MetricPersistence, io.Closer) {
		return NewMemorySeriesStorage(), ioutil.NopCloser(nil)
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

func testMemoryGetBoundaryValues(t test.Tester) {
	persistenceMaker := func() (MetricPersistence, io.Closer) {
		return NewMemorySeriesStorage(), ioutil.NopCloser(nil)
	}

	GetBoundaryValuesTests(persistenceMaker, t)
}

func TestMemoryGetBoundaryValues(t *testing.T) {
	testMemoryGetBoundaryValues(t)
}

func BenchmarkMemoryGetBoundaryValues(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryGetBoundaryValues(b)
	}
}

func testMemoryGetRangeValues(t test.Tester) {
	persistenceMaker := func() (MetricPersistence, io.Closer) {
		return NewMemorySeriesStorage(), ioutil.NopCloser(nil)
	}

	GetRangeValuesTests(persistenceMaker, t)
}

func TestMemoryGetRangeValues(t *testing.T) {
	testMemoryGetRangeValues(t)
}

func BenchmarkMemoryGetRangeValues(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryGetRangeValues(b)
	}
}
