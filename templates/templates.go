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

        "github.com/golang/glog"

        clientmodel "github.com/prometheus/client_golang/model"

        "github.com/prometheus/prometheus/rules/ast"
        "github.com/prometheus/prometheus/stats"
        "github.com/prometheus/prometheus/storage/metric"
)

func Expand(text string, name string, data interface{}, timestamp clientmodel.Timestamp, storage metric.PreloadingPersistence) string {
        funcMap := template.FuncMap{
                "query": func(q string) (ast.Vector, error) {
                        exprNode, _ := LoadExprFromString(q)
                        queryStats := stats.NewTimerGroup()
                        result, _ := ast.EvalToVector(exprNode, timestamp, storage, queryStats)
                        return result, nil
                },
                "first": func(v ast.Vector) (*clientmodel.Sample, error) {
                        if len(v) > 0 {
                                return v[0], nil
                        } else {
                                return nil, errors.New("first() called on vector with no elements")
                        }
                },
                "label": func(label string, s clientmodel.Sample) string {
                        return string(s.Metric[clientmodel.LabelName(label)])
                },
                "value": func(s clientmodel.Sample) float64 {
                        return float64(s.Value)
                },
                "strvalue": func(s clientmodel.Sample) string {
                        return string(s.Metric["__value__"])
                },
                "timestamp": func(s clientmodel.Sample) float64 {
                        return float64(s.Timestamp)
                },
        }

        var buffer bytes.Buffer
        tmpl, err := template.New(name).Funcs(funcMap).Parse(text)
        if err != nil {
                return fmt.Sprintf("Error parsing alert template: %v", err)
                glog.Warning(fmt.Sprintf("Error parsing alert template for %v: %v", name, err))
        } else {
                err := tmpl.Execute(&buffer, data)
                if err != nil {
                        return fmt.Sprintf("Error executing alert template: %v", err)
                        glog.Warning(fmt.Sprintf("Error executing alert template for %v: %v", name, err))
                }
        }
        return buffer.String()
}
