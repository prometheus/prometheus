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
//
// +build !windows

package stats

import "testing"

// TestTimerGroup does not work on Windows since its timer is not accurate
// enough.
func TestTimerGroup(t *testing.T) {
	tg := NewTimerGroup()
	execTotalTimer := tg.GetTimer(ExecTotalTime)
	if tg.GetTimer(ExecTotalTime).String() != "Exec total time: 0s" {
		t.Fatalf("Expected string %s, but got %s", "", execTotalTimer.String())
	}
	execQueueTimer := tg.GetTimer(ExecQueueTime)
	if tg.GetTimer(ExecQueueTime).String() != "Exec queue wait time: 0s" {
		t.Fatalf("Expected string %s, but got %s", "", execQueueTimer.String())
	}
	innerEvalTimer := tg.GetTimer(InnerEvalTime)
	if tg.GetTimer(InnerEvalTime).String() != "Inner eval time: 0s" {
		t.Fatalf("Expected string %s, but got %s", "", innerEvalTimer.String())
	}
	queryPreparationTimer := tg.GetTimer(QueryPreparationTime)
	if tg.GetTimer(QueryPreparationTime).String() != "Query preparation time: 0s" {
		t.Fatalf("Expected string %s, but got %s", "", queryPreparationTimer.String())
	}
	resultSortTimer := tg.GetTimer(ResultSortTime)
	if tg.GetTimer(ResultSortTime).String() != "Result sorting time: 0s" {
		t.Fatalf("Expected string %s, but got %s", "", resultSortTimer.String())
	}
	evalTotalTimer := tg.GetTimer(EvalTotalTime)
	if tg.GetTimer(EvalTotalTime).String() != "Eval total time: 0s" {
		t.Fatalf("Expected string %s, but got %s", "", evalTotalTimer.String())
	}

	actual := tg.String()
	expected := "Exec total time: 0s\nExec queue wait time: 0s\nInner eval time: 0s\nQuery preparation time: 0s\nResult sorting time: 0s\nEval total time: 0s\n"

	if actual != expected {
		t.Fatalf("Expected timerGroup string %s, but got %s.", expected, actual)
	}
}
