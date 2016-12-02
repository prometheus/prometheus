package test

import (
	"testing"

	"github.com/cespare/xxhash"
	sip13 "github.com/dgryski/go-sip13"
)

type pair struct {
	name, value string
}

var testInput = []pair{
	{"job", "node"},
	{"instance", "123.123.1.211:9090"},
	{"path", "/api/v1/namespaces/<namespace>/deployments/<name>"},
	{"method", "GET"},
	{"namespace", "system"},
	{"status", "500"},
}

func BenchmarkHash(b *testing.B) {
	input := []byte{}
	for _, v := range testInput {
		input = append(input, v.name...)
		input = append(input, '\xff')
		input = append(input, v.value...)
		input = append(input, '\xff')
	}

	var total uint64

	var k0 uint64 = 0x0706050403020100
	var k1 uint64 = 0x0f0e0d0c0b0a0908

	for name, f := range map[string]func(b []byte) uint64{
		"xxhash": xxhash.Sum64,
		"fnv64":  fnv64a,
		"sip13":  func(b []byte) uint64 { return sip13.Sum64(k0, k1, b) },
	} {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(input)))
			total = 0
			for i := 0; i < b.N; i++ {
				total += f(input)
			}
		})
	}

}

// hashAdd adds a string to a fnv64a hash value, returning the updated hash.
func fnv64a(b []byte) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)

	h := uint64(offset64)
	for x := range b {
		h ^= uint64(x)
		h *= prime64
	}
	return h
}
