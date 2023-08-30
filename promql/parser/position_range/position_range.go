// Copyright 2023 The Prometheus Authors
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

package position_range

import "fmt"

// Pos is the position in a string.
// Negative numbers indicate undefined positions.
type Pos int

// PositionRange describes a position in the input string of the parser.
type PositionRange struct {
	Start Pos
	End   Pos
}

func (p PositionRange) FullInfo(query string, lineOffset int) string {
	pos := int(p.Start)
	lastLineBreak := -1
	line := lineOffset + 1

	var positionStr string

	if pos < 0 || pos > len(query) {
		positionStr = "invalid position"
	} else {

		for i, c := range query[:pos] {
			if c == '\n' {
				lastLineBreak = i
				line++
			}
		}

		col := pos - lastLineBreak
		positionStr = fmt.Sprintf("%d:%d", line, col)
	}
	return positionStr
}
