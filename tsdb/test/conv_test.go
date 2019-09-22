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

package test

import "testing"

func BenchmarkMapConversion(b *testing.B) {
	type key string
	type val string

	m := map[key]val{
		"job":       "node",
		"instance":  "123.123.1.211:9090",
		"path":      "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":    "GET",
		"namespace": "system",
		"status":    "500",
	}

	var sm map[string]string

	for i := 0; i < b.N; i++ {
		sm = make(map[string]string, len(m))
		for k, v := range m {
			sm[string(k)] = string(v)
		}
	}
}

func BenchmarkListIter(b *testing.B) {
	var list []uint32
	for i := 0; i < 1e4; i++ {
		list = append(list, uint32(i))
	}

	b.ResetTimer()

	total := uint32(0)

	for j := 0; j < b.N; j++ {
		sum := uint32(0)
		for _, k := range list {
			sum += k
		}
		total += sum
	}
}
