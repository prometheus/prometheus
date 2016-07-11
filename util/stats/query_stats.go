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

package stats

// QueryTiming identifies the code area or functionality in which time is spent
// during a query.
type QueryTiming int

// Query timings.
const (
	TotalEvalTime QueryTiming = iota
	ResultSortTime
	QueryPreparationTime
	InnerEvalTime
	ResultAppendTime
	ExecQueueTime
)

// Return a string representation of a QueryTiming identifier.
func (s QueryTiming) String() string {
	switch s {
	case TotalEvalTime:
		return "Total eval time"
	case ResultSortTime:
		return "Result sorting time"
	case QueryPreparationTime:
		return "Query preparation time"
	case InnerEvalTime:
		return "Inner eval time"
	case ResultAppendTime:
		return "Result append time"
	case ExecQueueTime:
		return "Exec queue wait time"
	default:
		return "Unknown query timing"
	}
}
