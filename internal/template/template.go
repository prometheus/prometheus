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

package template

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	html_template "html/template"
	"math"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	text_template "text/template"
	"time"

	"github.com/grafana/regexp"
	"github.com/prometheus/client_golang/prometheus"
	common_templates "github.com/prometheus/common/helpers/templates"
	"github.com/prometheus/common/model"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

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

	errNaNOrInf = errors.New("value is NaN or Inf")
)

func init() {
	prometheus.MustRegister(templateTextExpansionFailures)
	prometheus.MustRegister(templateTextExpansionTotal)
}

// FuncMap contains the standard functions used by Prometheus templates.
type FuncMap = text_template.FuncMap

type sample struct {
	Labels map[string]string
	Value  any
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

func query(ctx context.Context, q string, ts time.Time, queryFn func(context.Context, string, time.Time) (promql.Vector, error)) (queryResult, error) {
	vector, err := queryFn(ctx, q, ts)
	if err != nil {
		return nil, err
	}

	// promql.Vector is hard to work with in templates, so convert to
	// base data types.
	// TODO(fabxc): probably not true anymore after type rework.
	result := make(queryResult, len(vector))
	for n, v := range vector {
		s := sample{
			Value:  v.F,
			Labels: v.Metric.Map(),
		}
		if v.H != nil {
			s.Value = v.H
		}
		result[n] = &s
	}
	return result, nil
}

// NewFuncMap returns the standard Prometheus template functions bound to the supplied evaluation context.
func NewFuncMap(
	ctx context.Context,
	timestamp model.Time,
	queryFunc func(context.Context, string, time.Time) (promql.Vector, error),
	externalURL *url.URL,
) text_template.FuncMap {
	return text_template.FuncMap{
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
		"value": func(s *sample) any {
			return s.Value
		},
		"strvalue": func(s *sample) string {
			return s.Labels["__value__"]
		},
		"args": func(args ...any) map[string]any {
			result := make(map[string]any)
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
		"title":     cases.Title(language.AmericanEnglish, cases.NoLower).String,
		"toUpper":   strings.ToUpper,
		"toLower":   strings.ToLower,
		"graphLink": strutil.GraphLinkForExpression,
		"tableLink": strutil.TableLinkForExpression,
		"sortByLabel": func(label string, v queryResult) queryResult {
			sorter := queryResultByLabelSorter{v[:], label}
			sort.Stable(sorter)
			return v
		},
		"stripPort": func(hostPort string) string {
			host, _, err := net.SplitHostPort(hostPort)
			if err != nil {
				return hostPort
			}
			return host
		},
		"stripDomain": func(hostPort string) string {
			host, port, err := net.SplitHostPort(hostPort)
			if err != nil {
				host = hostPort
			}
			if _, err := netip.ParseAddr(host); err == nil {
				return hostPort
			}
			host = strings.Split(host, ".")[0]
			if port != "" {
				return net.JoinHostPort(host, port)
			}
			return host
		},
		"humanize": func(i any) (string, error) {
			v, err := common_templates.ConvertToFloat(i)
			if err != nil {
				return "", err
			}
			if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Sprintf("%.4g", v), nil
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
				return fmt.Sprintf("%.4g%s", v, prefix), nil
			}
			prefix := ""
			for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
				if math.Abs(v) >= 1 {
					break
				}
				prefix = p
				v *= 1000
			}
			return fmt.Sprintf("%.4g%s", v, prefix), nil
		},
		"humanize1024": func(i any) (string, error) {
			v, err := common_templates.ConvertToFloat(i)
			if err != nil {
				return "", err
			}
			if math.Abs(v) <= 1 || math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Sprintf("%.4g", v), nil
			}
			prefix := ""
			for _, p := range []string{"ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi", "Yi"} {
				if math.Abs(v) < 1024 {
					break
				}
				prefix = p
				v /= 1024
			}
			return fmt.Sprintf("%.4g%s", v, prefix), nil
		},
		"humanizeDuration": common_templates.HumanizeDuration,
		"humanizePercentage": func(i any) (string, error) {
			v, err := common_templates.ConvertToFloat(i)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%.4g%%", v*100), nil
		},
		"humanizeTimestamp": common_templates.HumanizeTimestamp,
		"toTime": func(i any) (*time.Time, error) {
			v, err := common_templates.ConvertToFloat(i)
			if err != nil {
				return nil, err
			}

			return floatToTime(v)
		},
		"toDuration": func(i any) (*time.Duration, error) {
			v, err := common_templates.ConvertToFloat(i)
			if err != nil {
				return nil, err
			}
			d := time.Duration(v * float64(time.Second))
			return &d, nil
		},
		"now": func() float64 {
			return float64(timestamp) / 1000.0
		},
		"pathPrefix": func() string {
			return externalURL.Path
		},
		"externalURL": func() string {
			return externalURL.String()
		},
		"parseDuration": func(d string) (float64, error) {
			v, err := model.ParseDurationAllowNegative(d)
			if err != nil {
				return 0, err
			}
			return float64(time.Duration(v)) / float64(time.Second), nil
		},
		"urlQueryEscape": url.QueryEscape,
	}
}

func floatToTime(v float64) (*time.Time, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, errNaNOrInf
	}
	timestamp := v * 1e9
	if timestamp > math.MaxInt64 || timestamp < math.MinInt64 {
		return nil, fmt.Errorf("%v cannot be represented as a nanoseconds timestamp since it overflows int64", v)
	}
	t := model.TimeFromUnixNano(int64(timestamp)).Time().UTC()
	return &t, nil
}

// BoundTemplate is a parsed template with one evaluation's function bindings.
type BoundTemplate struct {
	name     string
	template *text_template.Template
	parseErr error
}

// CompileBoundTemplate parses text with one evaluation's functions and retains any parse error for expansion-time reporting.
func CompileBoundTemplate(name, text string, funcMap text_template.FuncMap, options []string) *BoundTemplate {
	if options == nil {
		options = []string{"missingkey=zero"}
	}
	tmpl := text_template.New(name).Funcs(funcMap)
	tmpl.Option(options...)
	parsed, err := tmpl.Parse(text)
	return &BoundTemplate{name: name, template: parsed, parseErr: err}
}

// ExpandBoundTemplate executes a bound template with the data for one result sample.
func ExpandBoundTemplate(bound *BoundTemplate, data any) (string, error) {
	return expand(bound.name, func() (string, error) {
		if bound.parseErr != nil {
			return "", fmt.Errorf("error parsing template %v: %w", bound.name, bound.parseErr)
		}
		return execute(bound.name, bound.template, data)
	})
}

// ExpandText parses and executes one text template with the supplied evaluation functions.
func ExpandText(name, text string, data any, funcMap text_template.FuncMap, options []string) (string, error) {
	return expand(name, func() (string, error) {
		bound := CompileBoundTemplate(name, text, funcMap, options)
		if bound.parseErr != nil {
			return "", fmt.Errorf("error parsing template %v: %w", name, bound.parseErr)
		}
		return execute(name, bound.template, data)
	})
}

func execute(name string, tmpl *text_template.Template, data any) (string, error) {
	var buffer bytes.Buffer
	err := tmpl.Execute(&buffer, data)
	if err != nil {
		return "", fmt.Errorf("error executing template %v: %w", name, err)
	}
	return buffer.String(), nil
}

func expand(name string, execute func() (string, error)) (result string, resultErr error) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			resultErr, ok = r.(error)
			if !ok {
				resultErr = fmt.Errorf("panic expanding template %v: %v", name, r)
			}
		}
		if resultErr != nil {
			templateTextExpansionFailures.Inc()
		}
	}()

	templateTextExpansionTotal.Inc()
	return execute()
}
