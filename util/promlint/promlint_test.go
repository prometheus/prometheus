// Copyright 2017 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promlint_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/util/promlint"
)

func TestLintNoHelpText(t *testing.T) {
	const msg = "no help text"

	tests := []struct {
		name     string
		in       string
		problems []promlint.Problem
	}{
		{
			name: "no help",
			in: `
# TYPE go_goroutines gauge
go_goroutines 24
`,
			problems: []promlint.Problem{{
				Metric: "go_goroutines",
				Text:   msg,
			}},
		},
		{
			name: "empty help",
			in: `
# HELP go_goroutines
# TYPE go_goroutines gauge
go_goroutines 24
`,
			problems: []promlint.Problem{{
				Metric: "go_goroutines",
				Text:   msg,
			}},
		},
		{
			name: "no help and empty help",
			in: `
# HELP go_goroutines
# TYPE go_goroutines gauge
go_goroutines 24
# TYPE go_threads gauge
go_threads 10
`,
			problems: []promlint.Problem{
				{
					Metric: "go_goroutines",
					Text:   msg,
				},
				{
					Metric: "go_threads",
					Text:   msg,
				},
			},
		},
		{
			name: "OK",
			in: `
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 24
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := promlint.New(strings.NewReader(tt.in))

			problems, err := l.Lint()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if want, got := tt.problems, problems; !reflect.DeepEqual(want, got) {
				t.Fatalf("unexpected problems:\n- want: %v\n-  got: %v",
					want, got)
			}
		})
	}
}

func TestLintMetricUnits(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		problems []promlint.Problem
	}{
		{
			name: "amperes",
			in: `
# HELP x_milliamperes Test metric.
# TYPE x_milliamperes counter
x_milliamperes 10
`,
			problems: []promlint.Problem{{
				Metric: "x_milliamperes",
				Text:   `use base unit "amperes" instead of "milliamperes"`,
			}},
		},
		{
			name: "bytes",
			in: `
# HELP x_gigabytes Test metric.
# TYPE x_gigabytes counter
x_gigabytes 10
`,
			problems: []promlint.Problem{{
				Metric: "x_gigabytes",
				Text:   `use base unit "bytes" instead of "gigabytes"`,
			}},
		},
		{
			name: "candela",
			in: `
# HELP x_kilocandela Test metric.
# TYPE x_kilocandela counter
x_kilocandela 10
`,
			problems: []promlint.Problem{{
				Metric: "x_kilocandela",
				Text:   `use base unit "candela" instead of "kilocandela"`,
			}},
		},
		{
			name: "grams",
			in: `
# HELP x_kilograms Test metric.
# TYPE x_kilograms counter
x_kilograms 10
`,
			problems: []promlint.Problem{{
				Metric: "x_kilograms",
				Text:   `use base unit "grams" instead of "kilograms"`,
			}},
		},
		{
			name: "kelvin",
			in: `
# HELP x_nanokelvin Test metric.
# TYPE x_nanokelvin counter
x_nanokelvin 10
`,
			problems: []promlint.Problem{{
				Metric: "x_nanokelvin",
				Text:   `use base unit "kelvin" instead of "nanokelvin"`,
			}},
		},
		{
			name: "kelvins",
			in: `
# HELP x_nanokelvins Test metric.
# TYPE x_nanokelvins counter
x_nanokelvins 10
`,
			problems: []promlint.Problem{{
				Metric: "x_nanokelvins",
				Text:   `use base unit "kelvins" instead of "nanokelvins"`,
			}},
		},
		{
			name: "meters",
			in: `
# HELP x_kilometers Test metric.
# TYPE x_kilometers counter
x_kilometers 10
`,
			problems: []promlint.Problem{{
				Metric: "x_kilometers",
				Text:   `use base unit "meters" instead of "kilometers"`,
			}},
		},
		{
			name: "metres",
			in: `
# HELP x_kilometres Test metric.
# TYPE x_kilometres counter
x_kilometres 10
`,
			problems: []promlint.Problem{{
				Metric: "x_kilometres",
				Text:   `use base unit "metres" instead of "kilometres"`,
			}},
		},
		{
			name: "moles",
			in: `
# HELP x_picomoles Test metric.
# TYPE x_picomoles counter
x_picomoles 10
`,
			problems: []promlint.Problem{{
				Metric: "x_picomoles",
				Text:   `use base unit "moles" instead of "picomoles"`,
			}},
		},
		{
			name: "seconds",
			in: `
# HELP x_microseconds Test metric.
# TYPE x_microseconds counter
x_microseconds 10
`,
			problems: []promlint.Problem{{
				Metric: "x_microseconds",
				Text:   `use base unit "seconds" instead of "microseconds"`,
			}},
		},
		{
			name: "OK",
			in: `
# HELP thermometers_degrees_kelvin Test metric with name that looks like "meters".
# TYPE thermometers_degrees_kelvin counter
thermometers_degrees_kelvin 0
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := promlint.New(strings.NewReader(tt.in))

			problems, err := l.Lint()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if want, got := tt.problems, problems; !reflect.DeepEqual(want, got) {
				t.Fatalf("unexpected problems:\n- want: %v\n-  got: %v",
					want, got)
			}
		})
	}
}
