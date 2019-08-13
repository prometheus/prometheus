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

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/prometheus/prometheus/tsdb/labels"
)

func BenchmarkMapClone(b *testing.B) {
	m := map[string]string{
		"job":        "node",
		"instance":   "123.123.1.211:9090",
		"path":       "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":     "GET",
		"namespace":  "system",
		"status":     "500",
		"prometheus": "prometheus-core-1",
		"datacenter": "eu-west-1",
		"pod_name":   "abcdef-99999-defee",
	}

	for i := 0; i < b.N; i++ {
		res := make(map[string]string, len(m))
		for k, v := range m {
			res[k] = v
		}
		m = res
	}
}

func BenchmarkLabelsClone(b *testing.B) {
	m := map[string]string{
		"job":        "node",
		"instance":   "123.123.1.211:9090",
		"path":       "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":     "GET",
		"namespace":  "system",
		"status":     "500",
		"prometheus": "prometheus-core-1",
		"datacenter": "eu-west-1",
		"pod_name":   "abcdef-99999-defee",
	}
	l := labels.FromMap(m)

	for i := 0; i < b.N; i++ {
		res := make(labels.Labels, len(l))
		copy(res, l)
		l = res
	}
}

func BenchmarkLabelMapAccess(b *testing.B) {
	m := map[string]string{
		"job":        "node",
		"instance":   "123.123.1.211:9090",
		"path":       "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":     "GET",
		"namespace":  "system",
		"status":     "500",
		"prometheus": "prometheus-core-1",
		"datacenter": "eu-west-1",
		"pod_name":   "abcdef-99999-defee",
	}

	var v string

	for k := range m {
		b.Run(k, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				v = m[k]
			}
		})
	}

	_ = v
}

func BenchmarkLabelSetAccess(b *testing.B) {
	m := map[string]string{
		"job":        "node",
		"instance":   "123.123.1.211:9090",
		"path":       "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":     "GET",
		"namespace":  "system",
		"status":     "500",
		"prometheus": "prometheus-core-1",
		"datacenter": "eu-west-1",
		"pod_name":   "abcdef-99999-defee",
	}
	ls := labels.FromMap(m)

	var v string

	for _, l := range ls {
		b.Run(l.Name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				v = ls.Get(l.Name)
			}
		})
	}

	_ = v
}

func BenchmarkStringBytesEquals(b *testing.B) {
	randBytes := func(n int) ([]byte, []byte) {
		buf1 := make([]byte, n)
		if _, err := rand.Read(buf1); err != nil {
			b.Fatal(err)
		}
		buf2 := make([]byte, n)
		copy(buf1, buf2)

		return buf1, buf2
	}

	cases := []struct {
		name string
		f    func() ([]byte, []byte)
	}{
		{
			name: "equal",
			f: func() ([]byte, []byte) {
				return randBytes(60)
			},
		},
		{
			name: "1-flip-end",
			f: func() ([]byte, []byte) {
				b1, b2 := randBytes(60)
				b2[59] ^= b2[59]
				return b1, b2
			},
		},
		{
			name: "1-flip-middle",
			f: func() ([]byte, []byte) {
				b1, b2 := randBytes(60)
				b2[29] ^= b2[29]
				return b1, b2
			},
		},
		{
			name: "1-flip-start",
			f: func() ([]byte, []byte) {
				b1, b2 := randBytes(60)
				b2[0] ^= b2[0]
				return b1, b2
			},
		},
		{
			name: "different-length",
			f: func() ([]byte, []byte) {
				b1, b2 := randBytes(60)
				return b1, b2[:59]
			},
		},
	}

	for _, c := range cases {
		b.Run(c.name+"-strings", func(b *testing.B) {
			ab, bb := c.f()
			as, bs := string(ab), string(bb)
			b.SetBytes(int64(len(as)))

			var r bool

			for i := 0; i < b.N; i++ {
				r = as == bs
			}
			_ = r
		})

		b.Run(c.name+"-bytes", func(b *testing.B) {
			ab, bb := c.f()
			b.SetBytes(int64(len(ab)))

			var r bool

			for i := 0; i < b.N; i++ {
				r = bytes.Equal(ab, bb)
			}
			_ = r
		})

		b.Run(c.name+"-bytes-length-check", func(b *testing.B) {
			ab, bb := c.f()
			b.SetBytes(int64(len(ab)))

			var r bool

			for i := 0; i < b.N; i++ {
				if len(ab) != len(bb) {
					continue
				}
				r = bytes.Equal(ab, bb)
			}
			_ = r
		})
	}
}
