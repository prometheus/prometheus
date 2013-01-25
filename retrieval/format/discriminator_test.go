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

package format

import (
	"fmt"
	"github.com/matttproud/prometheus/utility/test"
	"net/http"
	"testing"
)

func testDiscriminatorHttpHeader(t test.Tester) {
	var scenarios = []struct {
		input  map[string]string
		output Processor
		err    error
	}{
		{
			output: nil,
			err:    fmt.Errorf("Received illegal and nil header."),
		},
		{
			input:  map[string]string{"X-Prometheus-API-Version": "0.0.0"},
			output: nil,
			err:    fmt.Errorf("Unrecognized API version 0.0.0"),
		},
		{
			input:  map[string]string{"X-Prometheus-API-Version": "0.0.1"},
			output: Processor001,
			err:    nil,
		},
	}

	for i, scenario := range scenarios {
		var header http.Header

		if len(scenario.input) > 0 {
			header = http.Header{}
		}

		for key, value := range scenario.input {
			header.Add(key, value)
		}

		actual, err := DefaultRegistry.ProcessorForRequestHeader(header)

		if scenario.err != err {
			if scenario.err != nil && err != nil {
				if scenario.err.Error() != err.Error() {
					t.Errorf("%d. expected %s, got %s", i, scenario.err, err)
				}
			} else if scenario.err != nil || err != nil {
				t.Errorf("%d. expected %s, got %s", i, scenario.err, err)
			}
		}

		if scenario.output != actual {
			t.Errorf("%d. expected %s, got %s", i, scenario.output, actual)
		}
	}
}

func TestDiscriminatorHttpHeader(t *testing.T) {
	testDiscriminatorHttpHeader(t)
}

func BenchmarkDiscriminatorHttpHeader(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testDiscriminatorHttpHeader(b)
	}
}
