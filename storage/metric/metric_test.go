// Copyright 2014 The Prometheus Authors
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

package metric

import (
	"testing"

	"github.com/prometheus/common/model"
)

func TestMetric(t *testing.T) {
	testMetric := model.Metric{
		"to_delete": "test1",
		"to_change": "test2",
	}

	scenarios := []struct {
		fn  func(*Metric)
		out model.Metric
	}{
		{
			fn: func(cm *Metric) {
				cm.Del("to_delete")
			},
			out: model.Metric{
				"to_change": "test2",
			},
		},
		{
			fn: func(cm *Metric) {
				cm.Set("to_change", "changed")
			},
			out: model.Metric{
				"to_delete": "test1",
				"to_change": "changed",
			},
		},
	}

	for i, s := range scenarios {
		orig := testMetric.Clone()
		cm := &Metric{
			Metric: orig,
			Copied: false,
		}

		s.fn(cm)

		// Test that the original metric was not modified.
		if !orig.Equal(testMetric) {
			t.Fatalf("%d. original metric changed; expected %v, got %v", i, testMetric, orig)
		}

		// Test that the new metric has the right changes.
		if !cm.Metric.Equal(s.out) {
			t.Fatalf("%d. copied metric doesn't contain expected changes; expected %v, got %v", i, s.out, cm.Metric)
		}
	}
}
