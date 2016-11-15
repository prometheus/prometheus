package index

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestPageDelta(t *testing.T) {
	var (
		vals []DocID
		last DocID
	)
	for i := 0; i < 10000; i++ {
		vals = append(vals, last)
		last += DocID(rand.Int63n(1<<9) + 1)
	}
	data := make([]byte, pageSize)
	page := newPageDelta(data)

	if err := page.init(vals[0]); err != nil {
		t.Fatal(err)
	}

	var num int
	pc := page.cursor()

	for _, v := range vals[1:] {
		if err := pc.append(v); err != nil {
			if err == errPageFull {
				break
			}
			t.Fatal(err)
		}
		num++
	}

	res, err := ExpandIterator(pc)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res, vals[:num+1]) {
		t.Errorf("output did not match")
		t.Errorf("expected: %v", vals[:num+1])
		t.Errorf("received: %v", res)
	}
}

func BenchmarkPageDeltaAppend(b *testing.B) {
	var (
		vals []DocID
		last DocID
	)
	for i := 0; i < 10000; i++ {
		vals = append(vals, last)
		last += DocID(rand.Int63n(1<<10) + 1)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data := make([]byte, pageSize)
		page := newPageDelta(data)

		if err := page.init(vals[0]); err != nil {
			b.Fatal(err)
		}

		pc := page.cursor()

		for _, v := range vals[1:] {
			if err := pc.append(v); err != nil {
				if err == errPageFull {
					break
				}
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkPageDeltaRead(b *testing.B) {
	var (
		vals []DocID
		last DocID
	)
	for i := 0; i < 10000; i++ {
		vals = append(vals, last)
		last += DocID(rand.Int63n(1<<10) + 1)
	}
	data := make([]byte, pageSize)
	page := newPageDelta(data)

	if err := page.init(vals[0]); err != nil {
		b.Fatal(err)
	}

	pc := page.cursor()

	for _, v := range vals[1:] {
		if err := pc.append(v); err != nil {
			if err == errPageFull {
				break
			}
			b.Fatal(err)
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := ExpandIterator(pc); err != nil {
			b.Fatal(err)
		}
	}
}
