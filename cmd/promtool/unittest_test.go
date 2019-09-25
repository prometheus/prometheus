// Copyright 2019 The Prometheus Authors
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

package main

import (
	"io/ioutil"
	"strings"
	"testing"

	yaml "gopkg.in/yaml.v2"
)

func TestUnmarshalUnitTestFile(t *testing.T) {
	for _, tc := range []struct {
		file   string
		errMsg string
	}{
		{
			file: "testdata/empty.yml",
		},
		{
			file: "testdata/good.yml",
		},
		{
			file:   "testdata/invalid_sample_labels.yml",
			errMsg: "unexpected end of input inside braces",
		},
		{
			file:   "testdata/empty_alertname.yml",
			errMsg: "'alertname' can't be empty",
		},
		{
			file:   "testdata/duplicate_group_eval_order.yml",
			errMsg: "group name repeated",
		},
	} {
		t.Run(tc.file, func(t *testing.T) {
			b, err := ioutil.ReadFile(tc.file)
			if err != nil {
				t.Fatal(err)
			}
			var tf unitTestFile
			err = yaml.UnmarshalStrict(b, &tf)
			if tc.errMsg != "" {
				if err == nil {
					t.Fatal("expecting error but got nil")
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("expecting error containing %q but got %v", tc.errMsg, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expecting no error but got %v", err)
			}
		})
	}
}

func TestRunUnitTestFile(t *testing.T) {
	for _, tc := range []struct {
		file    string
		errMsgs []string
	}{
		{
			file: "testdata/good.yml",
		},
		{
			file: "testdata/invalid_alert_expression.yml",
			errMsgs: []string{
				`"InstanceDown": could not parse expression`,
			},
		},
		{
			file: "testdata/invalid_promql_expression.yml",
			errMsgs: []string{
				`expr: "go_goroutines >", time: 4m0s, err: parse error`,
			},
		},
		{
			file: "testdata/failed_alert_test.yml",
			errMsgs: []string{
				`alertname: "AnotherInstanceDown", time: 5m0s`,
				`alertname: "InstanceDown", time: 5m0s`,
				`alertname: "NotDefined", time: 5m0s`,
			},
		},
		{
			file: "testdata/failed_promql_test.yml",
			errMsgs: []string{
				`expr: "go_goroutines >= 50", time: 1m0s`,
				`expr: "go_goroutines >= 50", time: 4m0s`,
				`expr: "go_goroutines >= 50", time: 5m0s`,
			},
		},
		{
			file: "testdata/invalid_input_series.yml",
			errMsgs: []string{
				"expected number or 'stale'",
			},
		},
	} {
		t.Run(tc.file, func(t *testing.T) {
			errs := ruleUnitTest(tc.file)
			if len(tc.errMsgs) > 0 {
				if len(errs) != len(tc.errMsgs) {
					t.Fatalf("expecting %d error(s) but got %d\n%#v", len(tc.errMsgs), len(errs), errs)
				}
			OuterLoop:
				for _, msg := range tc.errMsgs {
					for _, err := range errs {
						if strings.Contains(err.Error(), msg) {
							continue OuterLoop
						}
					}
					t.Fatalf("expecting error containing %q but got\n%#v", msg, errs)
				}
				return
			}
			if len(errs) > 0 {
				t.Fatalf("expecting no error but got %d\n%#v", len(errs), errs)
			}
		})
	}
}
