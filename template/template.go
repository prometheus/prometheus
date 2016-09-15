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
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	html_template "html/template"
	text_template "text/template"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/strutil"
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

func query(ctx context.Context, q string, timestamp model.Time, queryEngine *promql.Engine) (queryResult, error) {
	query, err := queryEngine.NewInstantQuery(q, timestamp)
	if err != nil {
		return nil, err
	}
	res := query.Exec(ctx)
	if res.Err != nil {
		return nil, res.Err
	}
	var vector model.Vector

	switch v := res.Value.(type) {
	case model.Matrix:
		return nil, errors.New("matrix return values not supported")
	case model.Vector:
		vector = v
	case *model.Scalar:
		vector = model.Vector{&model.Sample{
			Value:     v.Value,
			Timestamp: v.Timestamp,
		}}
	case *model.String:
		vector = model.Vector{&model.Sample{
			Metric:    model.Metric{"__value__": model.LabelValue(v.Value)},
			Timestamp: v.Timestamp,
		}}
	default:
		panic("template.query: unhandled result value type")
	}

	// promql.Vector is hard to work with in templates, so convert to
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

// Expander executes templates in text or HTML mode with a common set of Prometheus template functions.
type Expander struct {
	text    string
	name    string
	data    interface{}
	funcMap text_template.FuncMap
}

// NewTemplateExpander returns a template expander ready to use.
func NewTemplateExpander(ctx context.Context, text string, name string, data interface{}, timestamp model.Time, queryEngine *promql.Engine, pathPrefix string) *Expander {
	return &Expander{
		text: text,
		name: name,
		data: data,
		funcMap: text_template.FuncMap{
			"query": func(q string) (queryResult, error) {
				return query(ctx, q, timestamp, queryEngine)
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
					days := (int64(v) / 60 / 60 / 24)
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
					// For seconds, we display 4 significant digts.
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
			"humanizeTimestamp": func(v float64) string {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return fmt.Sprintf("%.4g", v)
				}
				t := model.TimeFromUnixNano(int64(v * 1e9)).Time().UTC()
				return fmt.Sprint(t)
			},
			"pathPrefix": func() string {
				return pathPrefix
			},
		},
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
				resultErr = fmt.Errorf("panic expanding template %v: %v", te.name, r)
			}
		}
	}()

	tmpl, err := text_template.New(te.name).Funcs(te.funcMap).Parse(te.text)
	tmpl.Option("missingkey=zero")
	if err != nil {
		return "", fmt.Errorf("error parsing template %v: %v", te.name, err)
	}
	var buffer bytes.Buffer
	err = tmpl.Execute(&buffer, te.data)
	if err != nil {
		return "", fmt.Errorf("error executing template %v: %v", te.name, err)
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
				resultErr = fmt.Errorf("panic expanding template %v: %v", te.name, r)
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
		return "", fmt.Errorf("error parsing template %v: %v", te.name, err)
	}
	if len(templateFiles) > 0 {
		_, err = tmpl.ParseFiles(templateFiles...)
		if err != nil {
			return "", fmt.Errorf("error parsing template files for %v: %v", te.name, err)
		}
	}
	var buffer bytes.Buffer
	err = tmpl.Execute(&buffer, te.data)
	if err != nil {
		return "", fmt.Errorf("error executing template %v: %v", te.name, err)
	}
	return buffer.String(), nil
}
