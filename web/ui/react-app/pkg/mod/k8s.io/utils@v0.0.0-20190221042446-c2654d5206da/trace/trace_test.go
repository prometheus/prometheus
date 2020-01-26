/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package trace

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"k8s.io/klog"
)

func TestStep(t *testing.T) {
	tests := []struct {
		name          string
		inputString   string
		expectedTrace *Trace
	}{
		{
			name:        "When string is empty",
			inputString: "",
			expectedTrace: &Trace{
				steps: []traceStep{
					{time.Now(), ""},
				},
			},
		},
		{
			name:        "When string is not empty",
			inputString: "test2",
			expectedTrace: &Trace{
				steps: []traceStep{
					{time.Now(), "test2"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sampleTrace := &Trace{}
			sampleTrace.Step(tt.inputString)
			if sampleTrace.steps[0].msg != tt.expectedTrace.steps[0].msg {
				t.Errorf("Expected %v \n Got %v \n", tt.expectedTrace, sampleTrace)
			}
		})
	}
}

func TestTotalTime(t *testing.T) {
	test := struct {
		name       string
		inputTrace *Trace
	}{
		name: "Test with current system time",
		inputTrace: &Trace{
			startTime: time.Now(),
		},
	}

	t.Run(test.name, func(t *testing.T) {
		got := test.inputTrace.TotalTime()
		if got == 0 {
			t.Errorf("Expected total time 0, got %d \n", got)
		}
	})
}

func TestLog(t *testing.T) {
	test := struct {
		name             string
		expectedMessages []string
		sampleTrace      *Trace
	}{
		name: "Check the log dump with 3 msg",
		expectedMessages: []string{
			"msg1", "msg2", "msg3",
		},
		sampleTrace: &Trace{
			name: "Sample Trace",
			steps: []traceStep{
				{time.Now(), "msg1"},
				{time.Now(), "msg2"},
				{time.Now(), "msg3"},
			},
		},
	}

	t.Run(test.name, func(t *testing.T) {
		var buf bytes.Buffer
		klog.SetOutput(&buf)
		test.sampleTrace.Log()
		for _, msg := range test.expectedMessages {
			if !strings.Contains(buf.String(), msg) {
				t.Errorf("\nMsg %q not found in log: \n%v\n", msg, buf.String())
			}
		}
	})
}

func TestLogIfLong(t *testing.T) {
	currentTime := time.Now()
	type mutate struct {
		delay time.Duration
		msg   string
	}

	tests := []*struct {
		name             string
		expectedMessages []string
		sampleTrace      *Trace
		threshold        time.Duration
		mutateInfo       []mutate // mutateInfo contains the information to mutate step's time to simulate multiple tests without waiting.

	}{
		{
			name: "When threshold is 500 and msg 2 has highest share",
			expectedMessages: []string{
				"msg2",
			},
			mutateInfo: []mutate{
				{10, "msg1"},
				{1000, "msg2"},
				{0, "msg3"},
			},
			threshold: 500,
		},
		{
			name: "When threshold is 10 and msg 3 has highest share",
			expectedMessages: []string{
				"msg3",
			},
			mutateInfo: []mutate{
				{0, "msg1"},
				{0, "msg2"},
				{50, "msg3"},
			},
			threshold: 10,
		},
		{
			name: "When threshold is 0 and all msg have same share",
			expectedMessages: []string{
				"msg1", "msg2", "msg3",
			},
			mutateInfo: []mutate{
				{0, "msg1"},
				{0, "msg2"},
				{0, "msg3"},
			},
			threshold: 0,
		},
		{
			name:             "When threshold is 20 and all msg 1 has highest share",
			expectedMessages: []string{},
			mutateInfo: []mutate{
				{10, "msg1"},
				{0, "msg2"},
				{0, "msg3"},
			},
			threshold: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			klog.SetOutput(&buf)

			tt.sampleTrace = New("Test trace")

			for index, mod := range tt.mutateInfo {
				tt.sampleTrace.Step(mod.msg)
				tt.sampleTrace.steps[index].stepTime = currentTime.Add(mod.delay)
			}

			tt.sampleTrace.LogIfLong(tt.threshold)

			for _, msg := range tt.expectedMessages {
				if msg != "" && !strings.Contains(buf.String(), msg) {
					t.Errorf("Msg %q expected in trace log: \n%v\n", msg, buf.String())
				}
			}
		})
	}
}
