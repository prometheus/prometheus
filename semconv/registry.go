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

// Package semconv reads the semantic convention registry that declares the
// metrics the Prometheus binary exposes.
package semconv

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	yaml "go.yaml.in/yaml/v3"
)

// Registry is a resolved semantic convention registry. References and
// inheritance are expected to be expanded already.
type Registry struct {
	Groups []Group `yaml:"groups"`
}

// Group types used in the registry.
const (
	// TypeMetric declares one metric.
	TypeMetric = "metric"
	// TypeAttributeGroup declares attributes that metric groups reference.
	TypeAttributeGroup = "attribute_group"
)

// Group declares either one metric or a set of reusable attributes, depending
// on its Type.
type Group struct {
	ID          string      `yaml:"id"`
	Type        string      `yaml:"type"`
	Stability   string      `yaml:"stability"`
	Brief       string      `yaml:"brief"`
	MetricName  string      `yaml:"metric_name"`
	Instrument  string      `yaml:"instrument"`
	Unit        string      `yaml:"unit"`
	Attributes  []Attribute `yaml:"attributes"`
	Annotations Annotations `yaml:"annotations"`
}

// Attribute is either the definition of a label, inside an attribute_group, or
// a reference to one, inside a metric group.
type Attribute struct {
	// ID names the label. It is set on definitions.
	ID string `yaml:"id"`
	// Ref points at the ID of a definition. It is set on references.
	Ref string `yaml:"ref"`
	// Type is the value type of the label, e.g. string.
	Type string `yaml:"type"`
	// Stability is the lifecycle stage of the label.
	Stability string `yaml:"stability"`
	// Brief describes the label.
	Brief string `yaml:"brief"`
	// Examples are sample values of the label.
	Examples []string `yaml:"examples"`
}

// Name returns the label name, whether the Attribute is a definition or a
// reference.
func (a Attribute) Name() string {
	if a.Ref != "" {
		return a.Ref
	}

	return a.ID
}

// Annotations carries details that semantic conventions cannot express.
type Annotations struct {
	Prometheus PrometheusAnnotations `yaml:"prometheus"`
}

// PrometheusAnnotations carries the Prometheus specific details of a metric.
type PrometheusAnnotations struct {
	// HistogramType is one of classic_histogram, native_histogram,
	// mixed_histogram or summary.
	HistogramType string `yaml:"histogram_type"`
	// OnlyOpts marks a metric that is implemented as a callback, so only its
	// Opts are generated rather than a full constructor.
	OnlyOpts bool `yaml:"only_opts"`
}

// Load parses a registry from s. It rejects input that does not declare any
// metric, so that pointing a caller at the wrong file fails loudly instead of
// yielding an empty registry that compares equal to nothing.
func Load(s []byte) (*Registry, error) {
	r := &Registry{}
	if err := yaml.Unmarshal(s, r); err != nil {
		return nil, err
	}
	metrics := 0
	for i, g := range r.Groups {
		if g.Type != TypeMetric {
			continue
		}
		if g.MetricName == "" {
			return nil, fmt.Errorf("group %d (%q) declares no metric_name", i, g.ID)
		}
		metrics++
	}
	if metrics == 0 {
		return nil, errors.New("registry declares no metric groups")
	}

	return r, nil
}

// LoadFile parses a registry from the given file.
func LoadFile(filename string) (*Registry, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	r, err := Load(content)
	if err != nil {
		return nil, fmt.Errorf("parsing registry %s: %w", filename, err)
	}

	return r, nil
}

// DescInfos returns the metrics the registry declares, keyed by metric name and
// shaped the way prometheus.Desc reports them, so the two can be compared.
func (r *Registry) DescInfos() map[string]prometheus.DescInfo {
	infos := make(map[string]prometheus.DescInfo, len(r.Groups))
	for _, g := range r.Groups {
		if g.Type != TypeMetric {
			continue
		}
		infos[g.MetricName] = g.DescInfo()
	}

	return infos
}

// DescInfo returns the group shaped the way prometheus.Desc reports it.
func (g Group) DescInfo() prometheus.DescInfo {
	var labels []string
	for _, a := range g.Attributes {
		labels = append(labels, a.Name())
	}

	return prometheus.DescInfo{
		FQName:         g.MetricName,
		Help:           g.Brief,
		Unit:           g.Unit,
		Type:           g.metricType(),
		VariableLabels: labels,
	}
}

// metricType maps the declared instrument to the type client_golang reports.
// Semantic conventions have no summary instrument, so a summary is declared as
// a histogram carrying annotations.prometheus.histogram_type: summary. An
// instrument that is missing or unrecognised maps to UNTYPED rather than to the
// zero value of dto.MetricType, which is COUNTER.
func (g Group) metricType() dto.MetricType {
	if g.Annotations.Prometheus.HistogramType == "summary" {
		return dto.MetricType_SUMMARY
	}

	t, ok := dto.MetricType_value[strings.ToUpper(g.Instrument)]
	if !ok {
		return dto.MetricType_UNTYPED
	}

	return dto.MetricType(t)
}

// sortLabels ignores label order, which is decided by the metric constructor
// rather than by the registry.
var sortLabels = cmpopts.SortSlices(func(a, b string) bool { return a < b })

// Diff reports how the declared metrics differ from the registry, in the format
// of cmp.Diff with the registry as the expected side. It returns an empty
// string when they agree. Only metrics that differ are reported, so the result
// stays readable for a registry covering the whole binary.
func (r *Registry) Diff(declared map[string]prometheus.DescInfo) string {
	want, got := map[string]prometheus.DescInfo{}, map[string]prometheus.DescInfo{}
	for name, w := range r.DescInfos() {
		d, ok := declared[name]
		if ok && cmp.Equal(w, d, sortLabels) {
			continue
		}
		want[name] = w
		if ok {
			got[name] = d
		}
	}
	for name, d := range declared {
		if _, ok := r.DescInfos()[name]; !ok {
			got[name] = d
		}
	}

	return cmp.Diff(want, got, sortLabels)
}
