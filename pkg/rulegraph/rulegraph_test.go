// Copyright 2017 The Prometheus Authors
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

package rulegraph

import (
	"sort"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/promql/parser"
)

func TestMetricFinder(t *testing.T) {
	cases := []struct {
		expr string
		want []string
	}{
		{"a+b", []string{"a", "b"}},
		{"max_over_time(a[1h]) > 3", []string{"a"}},
	}

	for ix, c := range cases {
		var mf metricFinder
		expr, err := parser.ParseExpr(c.expr)
		if err != nil {
			t.Errorf("Case #%d, unexpected error: %v", ix, err)
			continue
		}

		parser.Walk(&mf, expr, nil)
		want := strings.Join(c.want, ",")
		sort.Strings(mf.names)
		saw := strings.Join(mf.names, ",")
		if saw != want {
			t.Errorf("Case #%d, saw %s want %s", ix, saw, want)
		}
	}
}
