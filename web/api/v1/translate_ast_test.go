// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/parser"
)

func TestTranslateASTDurationExpressions(t *testing.T) {
	p := parser.NewParser(parser.Options{})

	type tc struct {
		name       string
		query      string
		wantType   string
		wantFields map[string]any
	}

	testcases := []tc{
		{
			name:     "regular matrix selector",
			query:    `foo[5m]`,
			wantType: "matrixSelector",
			wantFields: map[string]any{
				"range":      int64(5 * 60 * 1000),
				"rangeExpr":  nil,
				"offset":     int64(0),
				"offsetExpr": nil,
			},
		},
		{
			name:     "regular matrix selector with offset",
			query:    `foo[5m] offset 1m`,
			wantType: "matrixSelector",
			wantFields: map[string]any{
				"range":      int64(5 * 60 * 1000),
				"rangeExpr":  nil,
				"offset":     int64(60 * 1000),
				"offsetExpr": nil,
			},
		},
		{
			name:     "matrix selector range expression",
			query:    `foo[5m+1m]`,
			wantType: "matrixSelector",
			wantFields: map[string]any{
				"range":     int64(0),
				"rangeExpr": durationExpr("+", durationNumber("300", true), durationNumber("60", true), false),
			},
		},
		{
			name:     "matrix selector range expression with offset expression",
			query:    `foo[5m+1m] offset (10m/2)`,
			wantType: "matrixSelector",
			wantFields: map[string]any{
				"rangeExpr":  durationExpr("+", durationNumber("300", true), durationNumber("60", true), false),
				"offset":     int64(0),
				"offsetExpr": durationExpr("/", durationNumber("600", true), durationNumber("2", false), true),
			},
		},
		{
			name:     "complex matrix selector range expression",
			query:    `foo[max(step(),5m+3m) ]`,
			wantType: "matrixSelector",
			wantFields: map[string]any{
				"range": int64(0),
				"rangeExpr": durationExpr("max",
					durationExpr("step", nil, nil, false),
					durationExpr("+", durationNumber("300", true), durationNumber("180", true), false),
					false,
				),
			},
		},
		{
			name:     "nested min and max matrix selector range expression",
			query:    `foo[min(max(step(),5m+3m),10m-2m)]`,
			wantType: "matrixSelector",
			wantFields: map[string]any{
				"range": int64(0),
				"rangeExpr": durationExpr("min",
					durationExpr("max",
						durationExpr("step", nil, nil, false),
						durationExpr("+", durationNumber("300", true), durationNumber("180", true), false),
						false,
					),
					durationExpr("-", durationNumber("600", true), durationNumber("120", true), false),
					false,
				),
			},
		},
		{
			name:     "range preprocessor expression",
			query:    `foo[range()]`,
			wantType: "matrixSelector",
			wantFields: map[string]any{
				"range":     int64(0),
				"rangeExpr": durationExpr("range", nil, nil, false),
			},
		},
		{
			name:     "regular subquery selector",
			query:    `foo[5m:1m]`,
			wantType: "subquery",
			wantFields: map[string]any{
				"range":      int64(5 * 60 * 1000),
				"rangeExpr":  nil,
				"step":       int64(60 * 1000),
				"stepExpr":   nil,
				"offset":     int64(0),
				"offsetExpr": nil,
			},
		},
		{
			name:     "subquery selector duration expressions",
			query:    `foo[4s+4s:1s*2] offset (5s-8)`,
			wantType: "subquery",
			wantFields: map[string]any{
				"range":      int64(0),
				"rangeExpr":  durationExpr("+", durationNumber("4", true), durationNumber("4", true), false),
				"step":       int64(0),
				"stepExpr":   durationExpr("*", durationNumber("1", true), durationNumber("2", false), false),
				"offset":     int64(0),
				"offsetExpr": durationExpr("-", durationNumber("5", true), durationNumber("8", false), true),
			},
		},
		{
			name:     "regular vector selector offset",
			query:    `foo offset 5m`,
			wantType: "vectorSelector",
			wantFields: map[string]any{
				"offset":     int64(5 * 60 * 1000),
				"offsetExpr": nil,
			},
		},
		{
			name:     "vector selector offset expression",
			query:    `foo offset -min(5s,step()+8s)`,
			wantType: "vectorSelector",
			wantFields: map[string]any{
				"offset": int64(0),
				"offsetExpr": durationExpr("-", nil,
					durationExpr("min",
						durationNumber("5", true),
						durationExpr("+", durationExpr("step", nil, nil, false), durationNumber("8", true), false),
						false,
					),
					false,
				),
			},
		},
	}

	for _, tcase := range testcases {
		t.Run(tcase.name, func(t *testing.T) {
			expr, err := p.ParseExpr(tcase.query)
			require.NoError(t, err)

			got := translateAST(expr).(map[string]any)
			require.Equal(t, tcase.wantType, got["type"])
			for field, want := range tcase.wantFields {
				require.Contains(t, got, field)
				require.Equal(t, want, got[field], field)
			}
		})
	}
}

func durationExpr(op string, lhs, rhs any, wrapped bool) map[string]any {
	return map[string]any{
		"type":    "durationExpr",
		"op":      op,
		"lhs":     lhs,
		"rhs":     rhs,
		"wrapped": wrapped,
	}
}

func durationNumber(val string, duration bool) map[string]any {
	return map[string]any{
		"type":     "numberLiteral",
		"val":      val,
		"duration": duration,
	}
}
