// Copyright 2024 The Prometheus Authors
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
	"encoding/xml"
	"errors"
	"testing"
)

func TestJunitOutput(t *testing.T) {
	var buf bytes.Buffer
	var test JUnitXML
	x := FakeTestSuites()
	if err := x.WriteXML(&buf); err != nil {
		t.Fatalf("Failed to encode XML: %v", err)
	}

	output := buf.Bytes()

	err := xml.Unmarshal(output, &test)
	if err != nil {
		t.Errorf("Unmarshal failed with error: %v", err)
	}
	var total int
	var cases int
	total = len(test.Suites)
	if total != 3 {
		t.Errorf("JUnit output had %d testsuite elements; expected 3\n", total)
	}
	for _, i := range test.Suites {
		cases += len(i.Cases)
	}

	if cases != 7 {
		t.Errorf("JUnit output had %d testcase; expected 7\n", cases)
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
