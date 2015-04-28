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

package config

import (
	"path"
	"strings"
	"testing"
)

var fixturesPath = "fixtures"

var configTests = []struct {
	inputFile   string
	shouldFail  bool
	errContains string
}{
	{
		inputFile: "minimal.conf.input",
	}, {
		inputFile: "sample.conf.input",
	}, {
		inputFile: "empty.conf.input",
	}, {
		inputFile: "sd_targets.conf.input",
	},
	{
		inputFile:   "invalid_proto_format.conf.input",
		shouldFail:  true,
		errContains: "unknown field name",
	},
	{
		inputFile:   "invalid_scrape_interval.conf.input",
		shouldFail:  true,
		errContains: "invalid global scrape interval",
	},
	{
		inputFile:   "invalid_job_name.conf.input",
		shouldFail:  true,
		errContains: "invalid job name",
	},
	{
		inputFile:   "invalid_label_name.conf.input",
		shouldFail:  true,
		errContains: "invalid label name",
	},
	{
		inputFile:   "repeated_job_name.conf.input",
		shouldFail:  true,
		errContains: "found multiple scrape configs configured with the same job name: \"testjob1\"",
	},
}

func TestConfigs(t *testing.T) {
	for i, configTest := range configTests {
		_, err := LoadFromFile(path.Join(fixturesPath, configTest.inputFile))

		if err != nil {
			if !configTest.shouldFail {
				t.Fatalf("%d. Error parsing config %v: %v", i, configTest.inputFile, err)
			} else {
				if !strings.Contains(err.Error(), configTest.errContains) {
					t.Fatalf("%d. Expected error containing '%v', got: %v", i, configTest.errContains, err)
				}
			}
		}
	}
}
