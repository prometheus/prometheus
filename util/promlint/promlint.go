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

// Package promlint provides a linter for Prometheus metrics.
package promlint

import (
	"fmt"
	"io"
	"sort"
	"strings"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// A Linter is a Prometheus metrics linter.  It identifies issues with metric
// names, types, and metadata, and reports them to the caller.
type Linter struct {
	r io.Reader
}

// A Problem is an issue detected by a Linter.
type Problem struct {
	// The name of the metric indicated by this Problem.
	Metric string

	// A description of the issue for this Problem.
	Text string
}

// New creates a new Linter that reads an input stream of Prometheus metrics.
// Only the text exposition format is supported.
func New(r io.Reader) *Linter {
	return &Linter{
		r: r,
	}
}

// Lint performs a linting pass, returning a slice of Problems indicating any
// issues found in the metrics stream.  The slice is sorted by metric name
// and issue description.
func (l *Linter) Lint() ([]Problem, error) {
	// TODO(mdlayher): support for protobuf exposition format?
	d := expfmt.NewDecoder(l.r, expfmt.FmtText)

	var problems []Problem

	var mf dto.MetricFamily
	for {
		if err := d.Decode(&mf); err != nil {
			if err == io.EOF {
				break
			}

			return nil, err
		}

		problems = append(problems, lint(mf)...)
	}

	// Ensure deterministic output.
	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].Metric < problems[j].Metric {
			return true
		}

		return problems[i].Text < problems[j].Text
	})

	return problems, nil
}

// lint is the entry point for linting a single metric.
func lint(mf dto.MetricFamily) []Problem {
	fns := []func(mf dto.MetricFamily) []Problem{
		lintHelp,
		lintMetricUnits,
	}

	var problems []Problem
	for _, fn := range fns {
		problems = append(problems, fn(mf)...)
	}

	// TODO(mdlayher): lint rules for specific metrics types.
	return problems
}

// lintHelp detects issues related to the help text for a metric.
func lintHelp(mf dto.MetricFamily) []Problem {
	var problems []Problem

	// Expect all metrics to have help text available.
	if mf.Help == nil {
		problems = append(problems, Problem{
			Metric: *mf.Name,
			Text:   "no help text",
		})
	}

	return problems
}

// lintMetricUnits detects issues with metric unit names.
func lintMetricUnits(mf dto.MetricFamily) []Problem {
	var problems []Problem

	unit, base, ok := metricUnits(*mf.Name)
	if !ok {
		// No known units detected.
		return nil
	}

	// Unit is already a base unit.
	if unit == base {
		return nil
	}

	problems = append(problems, Problem{
		Metric: *mf.Name,
		Text:   fmt.Sprintf("use base unit %q instead of %q", base, unit),
	})

	return problems
}

// metricUnits attempts to detect known unit types used as part of a metric name,
// e.g. "foo_bytes_total" or "bar_baz_milligrams".
func metricUnits(m string) (unit string, base string, ok bool) {
	ss := strings.Split(m, "_")

	for _, u := range baseUnits {
		// Also check for "no prefix".
		for _, p := range append(unitPrefixes, "") {
			for _, s := range ss {
				// Attempt to explicitly match a known unit with a known prefix,
				// as some words may look like "units" when matching suffix.
				//
				// As an example, "thermometers" should not match "meters", but
				// "kilometers" should.
				if s == p+u {
					return p + u, u, true
				}
			}
		}
	}

	return "", "", false
}

// Units and their possible prefixes recognized by this library.  More can be
// added over time as needed.
var (
	baseUnits = []string{
		"amperes",
		"bytes",
		"candela",
		"grams",
		"kelvin", // Both plural and non-plural form allowed.
		"kelvins",
		"meters", // Both American and international spelling permitted.
		"metres",
		"moles",
		"seconds",
	}

	unitPrefixes = []string{
		"pico",
		"nano",
		"micro",
		"milli",
		"centi",
		"deci",
		"deca",
		"hecto",
		"kilo",
		"kibi",
		"mega",
		"mibi",
		"giga",
		"gibi",
		"tera",
		"tebi",
		"peta",
		"pebi",
	}
)
