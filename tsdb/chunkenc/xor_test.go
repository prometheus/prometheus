package chunkenc

import (
	"io"
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestXor(t *testing.T) {
	timestamp := int64(1234123324)
	v := 83665193758603.123
	num := 10
	var samples []pair
	for i := 0; i < num; i++ {
		timestamp += int64(1000)
		v += float64(100)
		samples = append(samples, pair{t: timestamp, v: v})
	}

	chunk := NewXORChunk()
	a, err := chunk.Appender()
	if err != nil {
		t.Fatalf("get appender: %s", err)
	}
	for _, p := range samples {
		a.Append(p.t, p.v)
	}

	res := make([]pair, 0)
	var it Iterator
	it = chunk.Iterator(it)
	for it.Next() {
		t, v := it.At()
		res = append(res, pair{t: t, v: v})
	}
	if it.Err() != io.EOF {
		testutil.Ok(t, it.Err())
	}

	for i := 0; i < len(samples); i++ {
		s := samples[i]
		r := res[i]
		if s.t != r.t {
			t.Fatal("test compact timestamp fail ,delta of delta", s, r)
		}
		if s.v != r.v {
			t.Fatal("test compact value fail ,xor", s, r)
		}
	}
}
