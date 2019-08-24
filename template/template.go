// Copyright 2013 The Prometheus Authors
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

package template

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	html_template "html/template"
	text_template "text/template"

	"github.com/pkg/errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/strutil"
)

var (
	templateTextExpansionFailures = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_template_text_expansion_failures_total",
		Help: "The total number of template text expansion failures.",
	})
	templateTextExpansionTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_template_text_expansions_total",
		Help: "The total number of template text expansions.",
	})
)

func init() {
	prometheus.MustRegister(templateTextExpansionFailures)
	prometheus.MustRegister(templateTextExpansionTotal)
}

// A version of vector that's easier to use from templates.
type sample struct {
	Labels map[string]string
	Value  float64
}
type queryResult []*sample

type queryResultByLabelSorter struct {
	results queryResult
	by      string
}

func (q queryResultByLabelSorter) Len() int {
	return len(q.results)
}

func (q queryResultByLabelSorter) Less(i, j int) bool {
	return q.results[i].Labels[q.by] < q.results[j].Labels[q.by]
}

func (q queryResultByLabelSorter) Swap(i, j int) {
	q.results[i], q.results[j] = q.results[j], q.results[i]
}

// QueryFunc executes a PromQL query at the given time.
type QueryFunc func(context.Context, string, time.Time) (promql.Vector, error)

func query(ctx context.Context, q string, ts time.Time, queryFn QueryFunc) (queryResult, error) {
	vector, err := queryFn(ctx, q, ts)
	if err != nil {
		return nil, err
	}

	// promql.Vector is hard to work with in templates, so convert to
	// base data types.
	// TODO(fabxc): probably not true anymore after type rework.
	var result = make(queryResult, len(vector))
	for n, v := range vector {
		s := sample{
			Value:  v.V,
			Labels: v.Metric.Map(),
		}
		result[n] = &s
	}
	return result, nil
}

// Expander executes templates in text or HTML mode with a common set of Prometheus template functions.
type Expander struct {
	text    string
	name    string
	data    interface{}
	funcMap text_template.FuncMap
}

// NewTemplateExpander returns a template expander ready to use.
func NewTemplateExpander(
	ctx context.Context,
	text string,
	name string,
	data interface{},
	timestamp model.Time,
	queryFunc QueryFunc,
	externalURL *url.URL,
) *Expander {
	return &Expander{
		text: text,
		name: name,
		data: data,
		funcMap: text_template.FuncMap{
			"query": func(q string) (queryResult, error) {
				return query(ctx, q, timestamp.Time(), queryFunc)
			},
			"first": func(v queryResult) (*sample, error) {
				if len(v) > 0 {
					return v[0], nil
				}
				return nil, errors.New("first() called on vector with no elements")
			},
			"label": func(label string, s *sample) string {
				return s.Labels[label]
			},
			"value": func(s *sample) float64 {
				return s.Value
			},
			"strvalue": func(s *sample) string {
				return s.Labels["__value__"]
			},
			"args": func(args ...interface{}) map[string]interface{} {
				result := make(map[string]interface{})
				for i, a := range args {
					result[fmt.Sprintf("arg%d", i)] = a
				}
				return result
			},
			"reReplaceAll": func(pattern, repl, text string) string {
				re := regexp.MustCompile(pattern)
				return re.ReplaceAllString(text, repl)
			},
			"safeHtml": func(text string) html_template.HTML {
				return html_template.HTML(text)
			},
			"match":     regexp.MatchString,
			"title":     strings.Title,
			"toUpper":   strings.ToUpper,
			"toLower":   strings.ToLower,
			"graphLink": strutil.GraphLinkForExpression,
			"tableLink": strutil.TableLinkForExpression,
			"sortByLabel": func(label string, v queryResult) queryResult {
				sorter := queryResultByLabelSorter{v[:], label}
				sort.Stable(sorter)
				return v
			},
			"humanize": func(v float64) string {
				if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
					return fmt.Sprintf("%.4g", v)
				}
				if math.Abs(v) >= 1 {
					prefix := ""
					for _, p := range []string{"k", "M", "G", "T", "P", "E", "Z", "Y"} {
						if math.Abs(v) < 1000 {
							break
						}
						prefix = p
						v /= 1000
					}
					return fmt.Sprintf("%.4g%s", v, prefix)
				}
				prefix := ""
				for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
					if math.Abs(v) >= 1 {
						break
					}
					prefix = p
					v *= 1000
				}
				return fmt.Sprintf("%.4g%s", v, prefix)
			},
			"humanize1024": func(v float64) string {
				if math.Abs(v) <= 1 || math.IsNaN(v) || math.IsInf(v, 0) {
					return fmt.Sprintf("%.4g", v)
				}
				prefix := ""
				for _, p := range []string{"ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi", "Yi"} {
					if math.Abs(v) < 1024 {
						break
					}
					prefix = p
					v /= 1024
				}
				return fmt.Sprintf("%.4g%s", v, prefix)
			},
			"humanizeDuration": func(v float64) string {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return fmt.Sprintf("%.4g", v)
				}
				if v == 0 {
					return fmt.Sprintf("%.4gs", v)
				}
				if math.Abs(v) >= 1 {
					sign := ""
					if v < 0 {
						sign = "-"
						v = -v
					}
					seconds := int64(v) % 60
					minutes := (int64(v) / 60) % 60
					hours := (int64(v) / 60 / 60) % 24
					days := int64(v) / 60 / 60 / 24
					// For days to minutes, we display seconds as an integer.
					if days != 0 {
						return fmt.Sprintf("%s%dd %dh %dm %ds", sign, days, hours, minutes, seconds)
					}
					if hours != 0 {
						return fmt.Sprintf("%s%dh %dm %ds", sign, hours, minutes, seconds)
					}
					if minutes != 0 {
						return fmt.Sprintf("%s%dm %ds", sign, minutes, seconds)
					}
					// For seconds, we display 4 significant digits.
					return fmt.Sprintf("%s%.4gs", sign, v)
				}
				prefix := ""
				for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
					if math.Abs(v) >= 1 {
						break
					}
					prefix = p
					v *= 1000
				}
				return fmt.Sprintf("%.4g%ss", v, prefix)
			},
			"humanizePercentage": func(v float64) string {
				return fmt.Sprintf("%.4g%%", v*100)
			},
			"humanizeTimestamp": func(v float64) string {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return fmt.Sprintf("%.4g", v)
				}
				t := model.TimeFromUnixNano(int64(v * 1e9)).Time().UTC()
				return fmt.Sprint(t)
			},
			"pathPrefix": func() string {
				return externalURL.Path
			},
			"externalURL": func() string {
				return externalURL.String()
			},
		},
	}
}

// AlertTemplateData returns the interface to be used in expanding the template.
func AlertTemplateData(labels map[string]string, externalLabels map[string]string, value float64) interface{} {
	return struct {
		Labels         map[string]string
		ExternalLabels map[string]string
		Value          float64
	}{
		Labels:         labels,
		ExternalLabels: externalLabels,
		Value:          value,
	}
}

// Funcs adds the functions in fm to the Expander's function map.
// Existing functions will be overwritten in case of conflict.
func (te Expander) Funcs(fm text_template.FuncMap) {
	for k, v := range fm {
		te.funcMap[k] = v
	}
}

// Expand expands a template in text (non-HTML) mode.
func (te Expander) Expand() (result string, resultErr error) {
	// It'd better to have no alert description than to kill the whole process
	// if there's a bug in the template.
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			resultErr, ok = r.(error)
			if !ok {
				resultErr = errors.Errorf("panic expanding template %v: %v", te.name, r)
			}
		}
		if resultErr != nil {
			templateTextExpansionFailures.Inc()
		}
	}()

	templateTextExpansionTotal.Inc()

	tmpl, err := text_template.New(te.name).Funcs(te.funcMap).Option("missingkey=zero").Parse(te.text)
	if err != nil {
		return "", errors.Wrapf(err, "error parsing template %v", te.name)
	}
	var buffer bytes.Buffer
	err = tmpl.Execute(&buffer, te.data)
	if err != nil {
		return "", errors.Wrapf(err, "error executing template %v", te.name)
	}
	return buffer.String(), nil
}

// ExpandHTML expands a template with HTML escaping, with templates read from the given files.
func (te Expander) ExpandHTML(templateFiles []string) (result string, resultErr error) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			resultErr, ok = r.(error)
			if !ok {
				resultErr = errors.Errorf("panic expanding template %s: %v", te.name, r)
			}
		}
	}()

	tmpl := html_template.New(te.name).Funcs(html_template.FuncMap(te.funcMap))
	tmpl.Option("missingkey=zero")
	tmpl.Funcs(html_template.FuncMap{
		"tmpl": func(name string, data interface{}) (html_template.HTML, error) {
			var buffer bytes.Buffer
			err := tmpl.ExecuteTemplate(&buffer, name, data)
			return html_template.HTML(buffer.String()), err
		},
	})
	tmpl, err := tmpl.Parse(te.text)
	if err != nil {
		return "", errors.Wrapf(err, "error parsing template %v", te.name)
	}
	if len(templateFiles) > 0 {
		_, err = tmpl.ParseFiles(templateFiles...)
		if err != nil {
			return "", errors.Wrapf(err, "error parsing template files for %v", te.name)
		}
	}
	var buffer bytes.Buffer
	err = tmpl.Execute(&buffer, te.data)
	if err != nil {
		return "", errors.Wrapf(err, "error executing template %v", te.name)
	}
	return buffer.String(), nil
}

// ParseTest parses the templates and returns the error if any.
func (te Expander) ParseTest() error {
	_, err := text_template.New(te.name).Funcs(te.funcMap).Option("missingkey=zero").Parse(te.text)
	if err != nil {
		return err
	}
	return nil
}
