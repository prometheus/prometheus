// Copyright 2015 The Prometheus Authors
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

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		input []string
		valid bool
	}{
		{
			input: []string{},
			valid: true,
		},
		{
			input: []string{"-web.external-url", ""},
			valid: true,
		},
		{
			input: []string{"-web.external-url", "http://proxy.com/prometheus"},
			valid: true,
		},
		{
			input: []string{"-web.external-url", "'https://url/prometheus'"},
			valid: false,
		},
		{
			input: []string{"-storage.remote.influxdb-url", ""},
			valid: true,
		},
		{
			input: []string{"-storage.remote.influxdb-url", "http://localhost:8086/"},
			valid: true,
		},
		{
			input: []string{"-storage.remote.influxdb-url", "'https://some-url/'"},
			valid: false,
		},
	}

	for i, test := range tests {
		// reset "immutable" config
		cfg.prometheusURL = ""
		cfg.influxdbURL = ""

		err := parse(test.input)
		if test.valid && err != nil {
			t.Errorf("%d. expected input to be valid, got %s", i, err)
		} else if !test.valid && err == nil {
			t.Errorf("%d. expected input to be invalid", i)
		}
	}
}
