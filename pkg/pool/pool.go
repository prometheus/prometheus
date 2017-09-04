package pool

import "sync"

type BytesPool struct {
	buckets []sync.Pool
	sizes   []int
}

func NewBytesPool(minSize, maxSize int, factor float64) *BytesPool {
	if minSize < 1 {
		panic("invalid minimum pool size")
	}
	if maxSize < 1 {
		panic("invalid maximum pool size")
	}
	if factor < 1 {
		panic("invalid factor")
	}

	var sizes []int

	for s := minSize; s <= maxSize; s = int(float64(s) * factor) {
		sizes = append(sizes, s)
	}

	p := &BytesPool{
		buckets: make([]sync.Pool, len(sizes)),
		sizes:   sizes,
	}

	return p
}

func (p *BytesPool) Get(sz int) []byte {
	for i, bktSize := range p.sizes {
		if sz > bktSize {
			continue
		}
		b, ok := p.buckets[i].Get().([]byte)
		if !ok {
			b = make([]byte, 0, bktSize)
		}
		return b
	}
	return make([]byte, 0, sz)
}

func (p *BytesPool) Put(b []byte) {
	for i, bktSize := range p.sizes {
		if cap(b) > bktSize {
			continue
		}
		p.buckets[i].Put(b[:0])
		return
	}
}
