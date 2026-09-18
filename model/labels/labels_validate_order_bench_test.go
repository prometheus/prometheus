// Copyright The Prometheus Authors
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
	"fmt"
	"slices"
	"testing"
)

func BenchmarkLabels_ValidateOrder(b *testing.B) {
	names := []string{
		"__name__", "cluster", "container", "endpoint", "env", "handler",
		"host", "instance", "job", "le", "method", "namespace", "node",
		"operation", "path", "pod", "port", "protocol", "region", "replica",
		"route", "scheme", "service", "source", "status", "target",
		"team", "tenant", "version", "zone",
	}
	for _, size := range []int{5, 10, 30} {
		for _, scenario := range []string{"valid", "duplicate", "descending"} {
			b.Run(fmt.Sprintf("labels=%d/%s", size, scenario), func(b *testing.B) {
				labelNames := slices.Clone(names[:size])
				switch scenario {
				case "duplicate":
					labelNames[size-1] = labelNames[size-2]
				case "descending":
					labelNames[size-1], labelNames[size-2] = labelNames[size-2], labelNames[size-1]
				}
				builder := NewScratchBuilder(size)
				for _, name := range labelNames {
					builder.Add(name, "prometheus")
				}
				ls := builder.Labels()
				b.ReportAllocs()
				for b.Loop() {
					_ = ls.ValidateOrder()
				}
			})
		}
	}
}
