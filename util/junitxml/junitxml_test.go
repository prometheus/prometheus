// Copyright 2021 Marty Pauley
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

package junitxml

import (
	"bytes"
	"errors"
	"testing"

	"github.com/grafana/regexp"
)

func TestJunitOutput(t *testing.T) {
	var buf bytes.Buffer
	FakeTestSuites().WriteXML(&buf)
	output := buf.Bytes()

	reTop := regexp.MustCompile(`(?s)^<testsuites\W.*</testsuites>$`)
	reSuites := regexp.MustCompile(`(?s)<testsuite .*?</testsuite>`)
	reCases := regexp.MustCompile(`(?s)<testcase .*?</testcase>`)

	if !reTop.Match(output) {
		t.Errorf("JUnit output has no outer <testsuites>\n")
	}
	suites := reSuites.FindAll(output, -1)
	if len(suites) != 3 {
		t.Errorf("JUnit output had %d testsuite elements; expected 3\n", len(suites))
	}
	cases := reCases.FindAll(output, -1)
	if len(cases) != 7 {
		t.Errorf("JUnit output had %d testcase; expected 7\n", len(cases))
	}
}

func FakeTestSuites() *JUnitXML {
	ju := &JUnitXML{}
	good := ju.Suite("all good")
	good.Case("alpha")
	good.Case("beta")
	good.Case("gamma")
	mixed := ju.Suite("mixed")
	mixed.Case("good")
	bad := mixed.Case("bad")
	bad.Fail("once")
	bad.Fail("twice")
	mixed.Case("ugly").Abort(errors.New("buggy"))
	ju.Suite("fast").Fail("fail early")
	return ju
}
