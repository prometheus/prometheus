package tsdb

import "sync"

type bucketPool struct {
	buckets []sync.Pool
	sizes   []int
	new     func(sz int) interface{}
}

func newBucketPool(minSize, maxSize int, factor float64, f func(sz int) interface{}) *bucketPool {
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

	p := &bucketPool{
		buckets: make([]sync.Pool, len(sizes)),
		sizes:   sizes,
		new:     f,
	}

	return p
}

func (p *bucketPool) get(sz int) interface{} {
	for i, bktSize := range p.sizes {
		if sz > bktSize {
			continue
		}
		x := p.buckets[i].Get()
		if x == nil {
			x = p.new(sz)
		}
		return x
	}
	return p.new(sz)
}

func (p *bucketPool) put(x interface{}, sz int) {
	for i, bktSize := range p.sizes {
		if sz > bktSize {
			continue
		}
		p.buckets[i].Put(x)
		return
	}
}

type poolUint64 struct {
	p *bucketPool
}

func newPoolUint64(minSize, maxSize int, factor float64) poolUint64 {
	return poolUint64{
		p: newBucketPool(minSize, maxSize, factor, func(sz int) interface{} {
			return make([]uint64, 0, sz)
		}),
	}
}

func (p poolUint64) get(sz int) []uint64 {
	return p.p.get(sz).([]uint64)
}

func (p poolUint64) put(x []uint64) {
	p.p.put(x[:0], cap(x))
}
