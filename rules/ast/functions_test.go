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

package ast

import (
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

type emptyRangeNode struct{}

func (node emptyRangeNode) Type() ExprType             { return MATRIX }
func (node emptyRangeNode) NodeTreeToDotGraph() string { return "" }
func (node emptyRangeNode) String() string             { return "" }
func (node emptyRangeNode) Children() Nodes            { return Nodes{} }

func (node emptyRangeNode) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) Matrix {
	return Matrix{
		metric.SampleSet{
			Metric: clientmodel.Metric{clientmodel.MetricNameLabel: "empty_metric"},
			Values: metric.Values{},
		},
	}
}

func (node emptyRangeNode) EvalBoundaries(timestamp clientmodel.Timestamp, view *viewAdapter) Matrix {
	return Matrix{
		metric.SampleSet{
			Metric: clientmodel.Metric{clientmodel.MetricNameLabel: "empty_metric"},
			Values: metric.Values{},
		},
	}
}

func TestDeltaWithEmptyElementDoesNotCrash(t *testing.T) {
	now := clientmodel.Now()
	vector := deltaImpl(now, nil, []Node{emptyRangeNode{}, &ScalarLiteral{value: 0}}).(Vector)
	if len(vector) != 0 {
		t.Fatalf("Expected empty result vector, got: %v", vector)
	}
	vector = deltaImpl(now, nil, []Node{emptyRangeNode{}, &ScalarLiteral{value: 1}}).(Vector)
	if len(vector) != 0 {
		t.Fatalf("Expected empty result vector, got: %v", vector)
	}
}
