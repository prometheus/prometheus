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

package remote

import (
	"testing"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

func benchRelabelConfigs(n int) []*relabel.Config {
	var cfgs []*relabel.Config
	for range n {
		cfgs = append(cfgs, &relabel.Config{
			SourceLabels:         model.LabelNames{"env"},
			Regex:                relabel.MustNewRegexp("(.*)"),
			TargetLabel:          "environment",
			Replacement:          "$1",
			Action:               relabel.Replace,
			NameValidationScheme: model.UTF8Validation,
		})
	}
	return cfgs
}

func BenchmarkRelabel_Uncached(b *testing.B) {
	l := labels.FromStrings("__name__", "http_requests_total", "job", "myapp", "instance", "1.2.3.4:9090", "env", "prod", "region", "us-east-1")
	for _, n := range []int{1, 5} {
		cfgs := benchRelabelConfigs(n)
		b.Run(nRulesName(n), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				relabelLabels(l, cfgs)
			}
		})
	}
}

func BenchmarkRelabel_CacheSteadyState(b *testing.B) {
	l := labels.FromStrings("__name__", "http_requests_total", "job", "myapp", "instance", "1.2.3.4:9090", "env", "prod", "region", "us-east-1")
	for _, n := range []int{1, 5} {
		cfgs := benchRelabelConfigs(n)
		b.Run(nRulesName(n), func(b *testing.B) {
			cache := NewRelabelCache()
			cache.relabel(l, cfgs)

			b.ReportAllocs()
			for range b.N {
				cache.relabel(l, cfgs)
			}
		})
	}
}

func BenchmarkRelabel_CacheSteadyStateParallel(b *testing.B) {
	l := labels.FromStrings("__name__", "http_requests_total", "job", "myapp", "instance", "1.2.3.4:9090", "env", "prod", "region", "us-east-1")
	for _, n := range []int{1, 5} {
		cfgs := benchRelabelConfigs(n)
		b.Run(nRulesName(n), func(b *testing.B) {
			cache := NewRelabelCache()
			cache.relabel(l, cfgs)

			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					cache.relabel(l, cfgs)
				}
			})
		})
	}
}

func nRulesName(n int) string {
	switch n {
	case 1:
		return "1_rule"
	default:
		return "5_rules"
	}
}
