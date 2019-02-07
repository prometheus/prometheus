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

package labels

import (
	"testing"
)

func TestLabels_MatchLabels(t *testing.T) {
	labels := Labels{
		{
			Name:  "__name__",
			Value: "ALERTS",
		},
		{
			Name:  "alertname",
			Value: "HTTPRequestRateLow",
		},
		{
			Name:  "alertstate",
			Value: "pending",
		},
		{
			Name:  "instance",
			Value: "0",
		},
		{
			Name:  "job",
			Value: "app-server",
		},
		{
			Name:  "severity",
			Value: "critical",
		},
	}

	providedNames := []string{
		"__name__",
		"alertname",
		"alertstate",
		"instance",
	}

	got := labels.MatchLabels(true, providedNames...)
	expected := Labels{
		{
			Name:  "__name__",
			Value: "ALERTS",
		},
		{
			Name:  "alertname",
			Value: "HTTPRequestRateLow",
		},
		{
			Name:  "alertstate",
			Value: "pending",
		},
		{
			Name:  "instance",
			Value: "0",
		},
	}

	assertSlice(t, got, expected)

	// Now try with 'on' set to false.
	got = labels.MatchLabels(false, providedNames...)

	expected = Labels{
		{
			Name:  "job",
			Value: "app-server",
		},
		{
			Name:  "severity",
			Value: "critical",
		},
	}

	assertSlice(t, got, expected)
}

func assertSlice(t *testing.T, got, expected Labels) {
	if len(expected) != len(got) {
		t.Errorf("expected the length of matched label names to be %d, but got %d", len(expected), len(got))
	}

	for i, expectedLabel := range expected {
		if expectedLabel.Name != got[i].Name {
			t.Errorf("expected to get Label with name %s, but got %s instead", expectedLabel.Name, got[i].Name)
		}
	}
}
