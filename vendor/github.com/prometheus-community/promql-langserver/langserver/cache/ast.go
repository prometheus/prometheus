// Copyright 2019 Tobias Guggenmos
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

package cache

import (
	"go/token"

	promql "github.com/prometheus/prometheus/promql/parser"
)

// getSmallestSurroundingNode searches the syntax tree of the provided PromQL
// query for the smallest node that contains the given position.
func getSmallestSurroundingNode(query *CompiledQuery, tokenPos token.Pos) promql.Node {
	ast := query.Ast

	pos := promql.Pos(tokenPos - query.Pos)

	if ast == nil || pos < ast.PositionRange().Start || pos > ast.PositionRange().End {
		return nil
	}

	ret := ast

BIG_LOOP:
	for {
		for _, child := range promql.Children(ret) {
			if child.PositionRange().Start <= pos && child.PositionRange().End >= pos {
				ret = child
				continue BIG_LOOP
			}
		}
		break
	}

	return ret
}
