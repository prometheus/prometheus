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

package scrape

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

func BenchmarkMutateSampleLabelsLabelMap(b *testing.B) {
	const sources = 64
	tests := []struct {
		name    string
		matches int
	}{
		{name: "sources=64/rules=1/matches=all", matches: sources},
		{name: "sources=64/rules=1/matches=none", matches: 0},
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			pairs := []string{"__name__", "labelmap_benchmark"}
			for i := range sources {
				name := fmt.Sprintf("source_%04d", i)
				if i >= test.matches {
					name = fmt.Sprintf("otherx_%04d", i)
				}
				pairs = append(pairs, name, fmt.Sprintf("value_%04d", i))
			}
			input := labels.FromStrings(pairs...)
			target := &Target{labels: labels.FromStrings("instance", "host:9090", "job", "benchmark")}

			cfg := relabel.DefaultRelabelConfig
			cfg.SourceLabels = nil
			cfg.TargetLabel = ""
			cfg.Action = relabel.LabelMap
			cfg.Regex = relabel.MustNewRegexp(`source_(.*)`)
			cfg.Replacement = `mapped_${1}`
			if err := cfg.Validate(model.LegacyValidation); err != nil {
				b.Fatal(err)
			}
			cfgs := []*relabel.Config{&cfg}

			expected := sources + 3 + test.matches
			if got := mutateSampleLabels(input, target, false, cfgs); got.Len() != expected {
				b.Fatalf("got %d labels, expected %d", got.Len(), expected)
			}

			b.ReportAllocs()
			var result labels.Labels
			for b.Loop() {
				result = mutateSampleLabels(input, target, false, cfgs)
			}
			runtime.KeepAlive(result)
		})
	}
}
