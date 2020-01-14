// Copyright 2020 The Prometheus Authors
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

package promql

import (
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
)

var (
	testLabels = []labels.Labels{
		labels.Labels{},
		labels.Labels{
			labels.Label{Name: "job", Value: "prometheus"},
			labels.Label{Name: "__name__", Value: "up"},
		},
		labels.Labels{
			labels.Label{Name: "job", Value: "prometheus"},
			labels.Label{Name: "__name__", Value: "go_memstats_buck_hash_sys_bytes"},
		},
		labels.Labels{
			labels.Label{Name: "job", Value: "httpd"},
			labels.Label{Name: "instance", Value: "127.0.0.1:8080"},
			labels.Label{Name: "__name__", Value: "up"},
		},
		labels.Labels{
			labels.Label{Name: "job", Value: "httpd"},
			labels.Label{Name: "instance", Value: "127.0.0.1:8080"},
			labels.Label{Name: "__name__", Value: "threads"},
		},
		labels.Labels{
			labels.Label{Name: "instance", Value: "localhost:9090"},
			labels.Label{Name: "job", Value: "prometheus"},
			labels.Label{Name: "__name__", Value: "prometheus_http_request_duration_seconds_bucket"},
			labels.Label{Name: "handler", Value: "/api/v1/*path"},
			labels.Label{Name: "le", Value: "+Inf"},
		},
		labels.Labels{
			labels.Label{Name: "instance", Value: "localhost:9090"},
			labels.Label{Name: "job", Value: "prometheus"},
			labels.Label{Name: "__name__", Value: "prometheus_http_request_queue_seconds_bucket"},
			labels.Label{Name: "handler", Value: "/api/v1/*path"},
			labels.Label{Name: "le", Value: "+Inf"},
		},
	}
)

func BenchmarkSignature(b *testing.B) {
	for _, l := range testLabels {
		for _, r := range testLabels {
			for _, card := range []VectorMatchCardinality{CardManyToOne, CardOneToOne} {
				b.Run(r.String()+card.String()+l.String(), func(b *testing.B) {
					b.ReportAllocs()
					for n := 0; n < b.N; n++ {
						enh := &EvalNodeHelper{out: make(Vector, 0, 1)}
						resultMetric(r, l, ADD, &VectorMatching{Card: card}, enh)
					}
				})
			}
		}
	}
}

func BenchmarkSignatureReuse(b *testing.B) {
	ehn := &EvalNodeHelper{out: make(Vector, 0, 1)}
	for _, l := range testLabels {
		for _, r := range testLabels {
			for _, card := range []VectorMatchCardinality{CardManyToOne, CardOneToOne} {
				b.Run(r.String()+card.String()+l.String(), func(b *testing.B) {
					b.ReportAllocs()
					for n := 0; n < b.N; n++ {
						resultMetric(r, l, ADD, &VectorMatching{Card: card}, ehn)
					}
				})
			}
		}
	}
}
