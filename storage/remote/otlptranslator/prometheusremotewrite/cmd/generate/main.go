// Copyright 2024 The Prometheus Authors
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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

type context struct {
	Name                    string
	Package                 string
	PbPackagePath           string
	PbPackage               string
	LabelType               string
	SampleTimestampField    string
	ExemplarTimestampField  string
	HistogramTimestampField string
	UnknownType             string
	GaugeType               string
	CounterType             string
	HistogramType           string
	SummaryType             string
}

func main() {
	c := context{
		Name:                    "Prometheus",
		Package:                 "prometheusremotewrite",
		PbPackagePath:           "github.com/prometheus/prometheus/prompb",
		PbPackage:               "prompb",
		LabelType:               "Label",
		SampleTimestampField:    "Timestamp",
		ExemplarTimestampField:  "Timestamp",
		HistogramTimestampField: "Timestamp",
		UnknownType:             "MetricMetadata_UNKNOWN",
		GaugeType:               "MetricMetadata_GAUGE",
		CounterType:             "MetricMetadata_COUNTER",
		HistogramType:           "MetricMetadata_HISTOGRAM",
		SummaryType:             "MetricMetadata_SUMMARY",
	}

	ms, err := filepath.Glob("templates/*.go.tmpl")
	if err != nil {
		panic(err)
	}
	for _, m := range ms {
		t, err := template.ParseFiles(m)
		if err != nil {
			panic(err)
		}

		name, _, found := strings.Cut(filepath.Base(m), ".")
		if !found {
			panic(fmt.Errorf("invalid filename %q", m))
		}
		executeTemplate(t, name, c)
	}
}

func executeTemplate(t *template.Template, name string, c context) {
	f, err := os.Create(fmt.Sprintf("%s_generated.go", name))
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if _, err := f.WriteString("// Automatically generated file - do not edit!!\n\n"); err != nil {
		panic(err)
	}

	if err := t.Execute(f, c); err != nil {
		panic(err)
	}
}
