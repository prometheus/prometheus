// Copyright 2024 Prometheus Team
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

package writev2

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
)

func BenchmarkToLabels(b *testing.B) {
	symbols := []string{"", "__name__", "test_metric", "foo", "bar", "baz", "qux", "type", "counter", "unit", "bytes"}
	// labels: {__name__="test_metric", foo="bar", baz="qux"}
	// refs: 1, 2, 3, 4, 5, 6
	lblsRefs := []uint32{1, 2, 3, 4, 5, 6}

	ts := TimeSeries{
		LabelsRefs: lblsRefs,
		Metadata: Metadata{
			Type:    Metadata_METRIC_TYPE_COUNTER,
			UnitRef: 10, // "bytes"
		},
	}

	sb := labels.NewScratchBuilder(0)

	b.Run("WithoutTypeAndUnit", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = ts.ToLabels(&sb, symbols, false)
		}
	})

	b.Run("WithTypeAndUnit", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = ts.ToLabels(&sb, symbols, true)
		}
	})
}
