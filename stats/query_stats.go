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

package stats

// QueryTiming identifies the code area or functionality in which time is spent
// during a query.
type QueryTiming int

// Query timings.
const (
	TotalEvalTime QueryTiming = iota
	ResultSortTime
	JsonEncodeTime
	PreloadTime
	TotalQueryPreparationTime
	InnerViewBuildingTime
	InnerEvalTime
	ResultAppendTime
	QueryAnalysisTime
	GetValueAtTimeTime
	GetBoundaryValuesTime
	GetRangeValuesTime
	ViewQueueTime
	ViewDiskPreparationTime
	ViewDataExtractionTime
	ViewDiskExtractionTime
)

// Return a string represenation of a QueryTiming identifier.
func (s QueryTiming) String() string {
	switch s {
	case TotalEvalTime:
		return "Total eval time"
	case ResultSortTime:
		return "Result sorting time"
	case JsonEncodeTime:
		return "JSON encoding time"
	case PreloadTime:
		return "Query preloading time"
	case TotalQueryPreparationTime:
		return "Total query preparation time"
	case InnerViewBuildingTime:
		return "Inner view building time"
	case InnerEvalTime:
		return "Inner eval time"
	case ResultAppendTime:
		return "Result append time"
	case QueryAnalysisTime:
		return "Query analysis time"
	case GetValueAtTimeTime:
		return "GetValueAtTime() time"
	case GetBoundaryValuesTime:
		return "GetBoundaryValues() time"
	case GetRangeValuesTime:
		return "GetRangeValues() time"
	case ViewQueueTime:
		return "View queue wait time"
	case ViewDiskPreparationTime:
		return "View building disk preparation time"
	case ViewDataExtractionTime:
		return "Total view data extraction time"
	case ViewDiskExtractionTime:
		return "View disk data extraction time"
	default:
		return "Unknown query timing"
	}
}
