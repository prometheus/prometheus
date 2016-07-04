// Copyright 2016 The Prometheus Authors
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

package promql

import (
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage/metric"
)

func preloadQuery(querier Querier, expr Expr, from, to model.Time) error {
	var queryErr error
	Inspect(expr, func(node Node) bool {
		switch n := node.(type) {
		case *VectorSelector:
			iterators, err := querier.Query(
				from.Add(-n.Offset-StalenessDelta),
				to.Add(-n.Offset),
				n.LabelMatchers...,
			)
			if err != nil {
				queryErr = err
				return false
			}
			for fp, it := range iterators {
				n.iterators[fp] = it
				n.metrics[fp] = metric.Metric{Metric: it.Metric()}
			}
		case *MatrixSelector:
			iterators, err := querier.Query(
				from.Add(-n.Offset-n.Range),
				to.Add(-n.Offset),
				n.LabelMatchers...,
			)
			if err != nil {
				queryErr = err
				return false
			}
			for fp, it := range iterators {
				n.iterators[fp] = it
				n.metrics[fp] = metric.Metric{Metric: it.Metric()}
			}
		}
		return true
	})
	return queryErr
}
