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

package templates

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	html_template "html/template"
	text_template "text/template"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/metric"
)

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

func query(q string, timestamp clientmodel.Timestamp, storage metric.PreloadingPersistence) (queryResult, error) {
	exprNode, err := rules.LoadExprFromString(q)
	if err != nil {
		return nil, err
	}
	queryStats := stats.NewTimerGroup()
	vector, err := ast.EvalToVector(exprNode, timestamp, storage, queryStats)
	if err != nil {
		return nil, err
	}

	// ast.Vector is hard to work with in templates, so convert to
	// base data types.
	var result = make(queryResult, len(vector))
	for n, v := range vector {
		s := sample{
			Value:  float64(v.Value),
			Labels: make(map[string]string),
		}
		for label, value := range v.Metric {
			s.Labels[string(label)] = string(value)
		}
		result[n] = &s
	}
	return result, nil
}

type templateExpander struct {
	text    string
	name    string
	data    interface{}
	funcMap text_template.FuncMap
}

func NewTemplateExpander(text string, name string, data interface{}, timestamp clientmodel.Timestamp, storage metric.PreloadingPersistence) *templateExpander {
	return &templateExpander{
		text: text,
		name: name,
		data: data,
		funcMap: text_template.FuncMap{
			"query": func(q string) (queryResult, error) {
				return query(q, timestamp, storage)
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
			"match": regexp.MatchString,
			"title": strings.Title,
			"sortByLabel": func(label string, v queryResult) queryResult {
				sorter := queryResultByLabelSorter{v[:], label}
				sort.Stable(sorter)
				return v
			},
			"humanize": func(v float64) string {
				if v == 0 {
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
				} else {
					prefix := ""
					for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
						if math.Abs(v) >= 1 {
							break
						}
						prefix = p
						v *= 1000
					}
					return fmt.Sprintf("%.4g%s", v, prefix)
				}
			},
			"humanize1024": func(v float64) string {
				if math.Abs(v) <= 1 {
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
				if v == 0 {
					return fmt.Sprintf("%.4gs", v)
				}
				if math.Abs(v) >= 1 {
					sign := ""
					if v < 0 {
						sign = "-"
						v = -v
					}
					seconds := math.Mod(v, 60)
					minutes := (int64(v) / 60) % 60
					hours := (int64(v) / 60 / 60) % 24
					days := (int64(v) / 60 / 60 / 24)
					// For days to minutes, we display seconds as an integer.
					if days != 0 {
						return fmt.Sprintf("%s%dd %dh %dm %.0fs", sign, days, hours, minutes, seconds)
					}
					if hours != 0 {
						return fmt.Sprintf("%s%dh %dm %.0fs", sign, hours, minutes, seconds)
					}
					if minutes != 0 {
						return fmt.Sprintf("%s%dm %.0fs", sign, minutes, seconds)
					}
					// For seconds, we display 4 significant digts.
					return fmt.Sprintf("%s%.4gs", sign, math.Floor(seconds*1000+.5)/1000)
				} else {
					prefix := ""
					for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
						if math.Abs(v) >= 1 {
							break
						}
						prefix = p
						v *= 1000
					}
					return fmt.Sprintf("%.4g%ss", v, prefix)
				}
			},
		},
	}
}

// Expand a template.
func (te templateExpander) Expand() (result string, resultErr error) {
	// It'd better to have no alert description than to kill the whole process
	// if there's a bug in the template.
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			resultErr, ok = r.(error)
			if !ok {
				resultErr = fmt.Errorf("Panic expanding template %v: %v", te.name, r)
			}
		}
	}()

	var buffer bytes.Buffer
	tmpl, err := text_template.New(te.name).Funcs(te.funcMap).Parse(te.text)
	if err != nil {
		return "", fmt.Errorf("Error parsing template %v: %v", te.name, err)
	}
	err = tmpl.Execute(&buffer, te.data)
	if err != nil {
		return "", fmt.Errorf("Error executing template %v: %v", te.name, err)
	}
	return buffer.String(), nil
}

// Expand a template with HTML escaping, with templates read from the given files.
func (te templateExpander) ExpandHTML(templateFiles []string) (result string, resultErr error) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			resultErr, ok = r.(error)
			if !ok {
				resultErr = fmt.Errorf("Panic expanding template %v: %v", te.name, r)
			}
		}
	}()

	var buffer bytes.Buffer
	tmpl, err := html_template.New(te.name).Funcs(html_template.FuncMap(te.funcMap)).Parse(te.text)
	if err != nil {
		return "", fmt.Errorf("Error parsing template %v: %v", te.name, err)
	}
	if len(templateFiles) > 0 {
		_, err = tmpl.ParseFiles(templateFiles...)
		if err != nil {
			return "", fmt.Errorf("Error parsing template files for %v: %v", te.name, err)
		}
	}
	err = tmpl.Execute(&buffer, te.data)
	if err != nil {
		return "", fmt.Errorf("Error executing template %v: %v", te.name, err)
	}
	return buffer.String(), nil
}
