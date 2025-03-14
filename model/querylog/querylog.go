// Copyright 2022 The Prometheus Authors
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

package querylog

// QueryLog stores a query log entry information.
type QueryLog struct {
	Params    Params `json:"params"`
	Stats     Stats  `json:"stats"`
	Timestamp string `json:"ts"`
}

type Params struct {
	End   string `json:"end"`
	Query string `json:"query"`
	Start string `json:"start"`
	Step  uint64 `json:"step"`
}

type Stats struct {
	Timings Timings `json:"timings"`
	Samples Samples `json:"samples"`
}

type Timings struct {
	EvalTotalTime        float64 `json:"evalTotalTime"`
	ExecQueueTime        float64 `json:"execQueueTime"`
	ExecTotalTime        float64 `json:"execTotalTime"`
	InnerEvalTime        float64 `json:"innerEvalTime"`
	QueryPreparationTime float64 `json:"queryPreparationTime"`
	ResultSortTime       float64 `json:"resultSortTime"`
}

type Samples struct {
	TotalQueryableSamples uint64 `json:"totalQueryableSamples"`
	PeakSamples           uint64 `json:"peakSamples"`
}
