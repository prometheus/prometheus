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

package config

import (
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"testing"
)

var fixturesPath = "fixtures"

var configTests = []struct {
	inputFile   string
	printedFile string
	shouldFail  bool
	errContains string
}{
	{
		inputFile:   "minimal.conf.input",
		printedFile: "minimal.conf.printed",
	}, {
		inputFile:   "sample.conf.input",
		printedFile: "sample.conf.printed",
	}, {
		// TODO: Options that are not provided should be set to sane defaults or
		// create errors during config loading (as appropriate). Right now, these
		// options remain at their zero-values, which is probably not what we want.
		inputFile:   "empty.conf.input",
		printedFile: "empty.conf.printed",
	},
	// TODO: To enable testing of bad configs, we first need to change config
	// loading so that it doesn't exit when loading a bad config. Instead, the
	// configuration error should be passed back all the way to the caller.
	//
	//{
	//	inputFile: "bad_job_option.conf.input",
	//	shouldFail: true,
	//	errContains: "Missing job name",
	//},
}

func TestConfigs(t *testing.T) {
	for i, configTest := range configTests {
		testConfig, err := LoadFromFile(path.Join(fixturesPath, configTest.inputFile))

		if err != nil {
			if !configTest.shouldFail {
				t.Fatalf("%d. Error parsing config %v: %v", i, configTest.inputFile, err)
			} else {
				if !strings.Contains(err.Error(), configTest.errContains) {
					t.Fatalf("%d. Expected error containing '%v', got: %v", i, configTest.errContains, err)
				}
			}
		} else {
			printedConfig, err := ioutil.ReadFile(path.Join(fixturesPath, configTest.printedFile))
			if err != nil {
				t.Fatalf("%d. Error reading config %v: %v", i, configTest.inputFile, err)
				continue
			}
			expected := string(printedConfig)
			actual := testConfig.ToString(0)

			if actual != expected {
				t.Errorf("%d. %v: printed config doesn't match expected output", i, configTest.inputFile)
				t.Errorf("Expected:\n%v\n\nActual:\n%v\n", expected, actual)
				t.Fatalf("Writing expected and actual printed configs to /tmp for diffing (see test source for paths)")
				ioutil.WriteFile(fmt.Sprintf("/tmp/%s.expected", configTest.printedFile), []byte(expected), 0600)
				ioutil.WriteFile(fmt.Sprintf("/tmp/%s.actual", configTest.printedFile), []byte(actual), 0600)
			}
		}
	}
}
