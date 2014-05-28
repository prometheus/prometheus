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
	"text/template"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/metric"
)

// A version of vector that's easier to use from templates.
type sample struct {
	Labels    map[string]string
	Value     float64
	Timestamp float64
}
type queryResult []*sample

func Expand(text string, name string, data interface{}, timestamp clientmodel.Timestamp, storage metric.PreloadingPersistence) (result string, resultErr error) {

	// It'd better to have no alert description than to kill the whole process
	// if there's a bug in the template. Similarly with console templates.
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			resultErr, ok = r.(error)
			if !ok {
				resultErr = fmt.Errorf("Panic expanding template: %v", r)
			}
		}
	}()

	funcMap := template.FuncMap{
		"query": func(q string) (queryResult, error) {
			return query(q, timestamp, storage)
		},
		"first": func(v queryResult) (*sample, error) {
			if len(v) > 0 {
				return v[0], nil
			} else {
				return nil, errors.New("first() called on vector with no elements")
			}
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
		"timestamp": func(s *sample) float64 {
			return s.Timestamp
		},
	}

	var buffer bytes.Buffer
	tmpl, err := template.New(name).Funcs(funcMap).Parse(text)
	if err != nil {
		return "", fmt.Errorf("Error parsing template %v: %v", name, err)
	} else {
		err := tmpl.Execute(&buffer, data)
		if err != nil {
			return "", fmt.Errorf("Error executing template %v: %v", name, err)
		}
	}
	return buffer.String(), nil
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
	var result queryResult = make(queryResult, len(vector))
	for n, v := range vector {
		s := sample{
			Value:     float64(v.Value),
			Timestamp: float64(v.Timestamp),
			Labels:    make(map[string]string),
		}
		for label, value := range v.Metric {
			s.Labels[string(label)] = string(value)
		}
		result[n] = &s
	}
	return result, nil
}
