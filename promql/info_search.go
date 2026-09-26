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

package promql

import (
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
)

// InfoSearchHints returns the storage range used by an instant info() evaluation.
// Call before executing the query, which can mutate subquery nodes.
func InfoSearchHints(stmt *parser.EvalStmt) storage.SelectHints {
	timestamp, offset := infoSeriesSelectTimestampAndOffset(stmt.Expr)
	ev := evaluator{
		startTimestamp: stmt.Start.UnixMilli(),
		endTimestamp:   stmt.End.UnixMilli(),
		lookbackDelta:  stmt.LookbackDelta,
	}
	return ev.infoSelectHints(timestamp, offset)
}

// InfoSearchMatchers derives candidate info-series selectors from an input vector.
// A nil vector requests unscoped discovery; an empty non-nil vector matches nothing.
// Scope matchers use the same name defaults and identifying-label rules as info().
func InfoSearchMatchers(vector Vector, matchers []*labels.Matcher) [][]*labels.Matcher {
	var names, data []*labels.Matcher
	for _, m := range matchers {
		if m.Name == labels.MetricName {
			names = append(names, m)
		} else {
			data = append(data, m)
		}
	}
	names = effectiveInfoNameMatchers(names)
	common := append(data, names...)
	if vector == nil {
		return [][]*labels.Matcher{common}
	}
	mat := make(Matrix, 0, len(vector))
	for _, s := range vector {
		isInfo := true
		for _, m := range names {
			if !m.Matches(s.Metric.Get(labels.MetricName)) {
				isInfo = false
				break
			}
		}
		if !isInfo {
			mat = append(mat, Series{Metric: s.Metric})
		}
	}
	sets := infoIdentifyingMatcherSets(mat, nil)
	for i := range sets {
		sets[i] = append(sets[i], common...)
	}
	return sets
}
