package test

import (
	"bytes"
	"testing"

	"github.com/fabxc/tsdb"
)

func BenchmarkLabelMapAccess(b *testing.B) {
	m := map[string]string{
		"job":       "node",
		"instance":  "123.123.1.211:9090",
		"path":      "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":    "GET",
		"namespace": "system",
		"status":    "500",
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
		"job":       "node",
		"instance":  "123.123.1.211:9090",
		"path":      "/api/v1/namespaces/<namespace>/deployments/<name>",
		"method":    "GET",
		"namespace": "system",
		"status":    "500",
	}
	ls := tsdb.LabelsFromMap(m)

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
	cases := []struct {
		name string
		a, b string
	}{
		{
			name: "equal",
			a:    "sdfn492cn9xwm0ws8r,4932x98f,uj594cxf594802h875hgzz0h3586x8xz,359",
			b:    "sdfn492cn9xwm0ws8r,4932x98f,uj594cxf594802h875hgzz0h3586x8xz,359",
		},
		{
			name: "1-flip-end",
			a:    "sdfn492cn9xwm0ws8r,4932x98f,uj594cxf594802h875hgzz0h3586x8xz,359",
			b:    "sdfn492cn9xwm0ws8r,4932x98f,uj594cxf594802h875hgzz0h3586x8xz,353",
		},
		{
			name: "1-flip-middle",
			a:    "sdfn492cn9xwm0ws8r,4932x98f,uj594cxf594802h875hgzz0h3586x8xz,359",
			b:    "sdfn492cn9xwm0ws8r,4932x98f,uj504cxf594802h875hgzz0h3586x8xz,359",
		},
		{
			name: "1-flip-start",
			a:    "sdfn492cn9xwm0ws8r,4932x98f,uj594cxf594802h875hgzz0h3586x8xz,359",
			b:    "adfn492cn9xwm0ws8r,4932x98f,uj594cxf594802h875hgzz0h3586x8xz,359",
		},
		{
			name: "different-length",
			a:    "sdfn492cn9xwm0ws8r,4932x98f,uj594cxf594802h875hgzz0h3586x8xz,359",
			b:    "sdfn492cn9xwm0ws8r,4932x98f,uj594cxf594802h875hgzz0h3586x8xz,35",
		},
	}

	for _, c := range cases {
		b.Run(c.name+"-strings", func(b *testing.B) {
			as, bs := c.a, c.b
			b.SetBytes(int64(len(as)))

			var r bool

			for i := 0; i < b.N; i++ {
				r = as == bs
			}
			_ = r
		})

		b.Run(c.name+"-bytes", func(b *testing.B) {
			ab, bb := []byte(c.a), []byte(c.b)
			b.SetBytes(int64(len(ab)))

			var r bool

			for i := 0; i < b.N; i++ {
				r = bytes.Equal(ab, bb)
			}
			_ = r
		})

		b.Run(c.name+"-bytes-length-check", func(b *testing.B) {
			ab, bb := []byte(c.a), []byte(c.b)
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
