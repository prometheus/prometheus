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

package rules

import (
	"reflect"
	"testing"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
)

func TestRuleEval(t *testing.T) {
	storage, closer := local.NewTestStorage(t, 1)
	defer closer.Close()
	engine := promql.NewEngine(storage, nil)
	now := model.Now()

	suite := []struct {
		name   string
		expr   promql.Expr
		labels model.LabelSet
		result model.Vector
	}{
		{
			name:   "nolabels",
			expr:   &promql.NumberLiteral{Val: 1},
			labels: model.LabelSet{},
			result: model.Vector{&model.Sample{
				Value:     1,
				Timestamp: now,
				Metric:    model.Metric{"__name__": "nolabels"},
			}},
		},
		{
			name:   "labels",
			expr:   &promql.NumberLiteral{Val: 1},
			labels: model.LabelSet{"foo": "bar"},
			result: model.Vector{&model.Sample{
				Value:     1,
				Timestamp: now,
				Metric:    model.Metric{"__name__": "labels", "foo": "bar"},
			}},
		},
	}

	for _, test := range suite {
		rule := NewRecordingRule(test.name, test.expr, test.labels)
		result, err := rule.eval(now, engine)
		if err != nil {
			t.Fatalf("Error evaluating %s", test.name)
		}
		if !reflect.DeepEqual(result, test.result) {
			t.Fatalf("Error: expected %q, got %q", test.result, result)
		}
	}
}
