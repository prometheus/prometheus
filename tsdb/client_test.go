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

package tsdb

import (
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"
)

func TestTagsFromMetric(t *testing.T) {
	input := clientmodel.Metric{
		clientmodel.MetricNameLabel: "testmetric",
		"test:label":                "test:value",
		"many_chars":                "abc!ABC:012-3!45ö67~89./",
	}
	expected := map[string]string{
		"test:label": "test_value",
		"many_chars": "abc_ABC_012-3_45_67_89./",
	}
	actual := tagsFromMetric(input)
	if len(actual) != len(expected) {
		t.Fatalf("Expected %v, got %v", expected, actual)
	}
	for k, v := range expected {
		if v != actual[k] {
			t.Fatalf("Expected %s => %s, got %s => %s", k, v, k, actual[k])
		}
	}
}
