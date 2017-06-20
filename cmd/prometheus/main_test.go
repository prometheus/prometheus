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

package main

import "testing"

func TestComputeExternalURL(t *testing.T) {
	tests := []struct {
		extURL string
		valid  bool
	}{
		{
			extURL: "",
			valid:  true,
		},
		{
			extURL: "http://proxy.com/prometheus",
			valid:  true,
		},
		{
			extURL: "https:/proxy.com/prometheus",
			valid:  false,
		},
	}

	for i, test := range tests {
		r, err := computeExternalURL(test.extURL, "0.0.0.0:9090")
		if test.valid && err != nil {
			t.Errorf("%d. expected input to be valid, got %s", i, err)
		} else if !test.valid && err == nil {
			t.Logf("%+v", r)
			t.Errorf("%d. expected input to be invalid", i)
		}
	}
}
