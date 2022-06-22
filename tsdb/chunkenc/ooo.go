package chunkenc

import (
	"sort"
)

type sample struct {
	t int64
	v float64
}

// OOOChunk maintains samples in time-ascending order.
// Inserts for timestamps already seen, are dropped.
// Samples are stored uncompressed to allow easy sorting.
// Perhaps we can be more efficient later.
type OOOChunk struct {
	samples []sample
}

func NewOOOChunk(capacity int) *OOOChunk {
	return &OOOChunk{samples: make([]sample, 0, capacity)}
}

// Insert inserts the sample such that order is maintained.
// Returns false if insert was not possible due to the same timestamp already existing.
func (o *OOOChunk) Insert(t int64, v float64) bool {
	// find index of sample we should replace
	i := sort.Search(len(o.samples), func(i int) bool { return o.samples[i].t >= t })

	if i >= len(o.samples) {
		// none found. append it at the end
		o.samples = append(o.samples, sample{t, v})
		return true
	}

	if o.samples[i].t == t {
		return false
	}

	// expand length by 1 to make room. use a zero sample, we will overwrite it anyway
	o.samples = append(o.samples, sample{})
	copy(o.samples[i+1:], o.samples[i:])
	o.samples[i] = sample{t, v}

	return true
}

func (o *OOOChunk) NumSamples() int {
	return len(o.samples)
}

func (o *OOOChunk) ToXor() (*XORChunk, error) {
	x := NewXORChunk()
	app, err := x.Appender()
	if err != nil {
		return nil, err
	}
	for _, s := range o.samples {
		app.Append(s.t, s.v)
	}
	return x, nil
}

func (o *OOOChunk) ToXorBetweenTimestamps(mint, maxt int64) (*XORChunk, error) {
	x := NewXORChunk()
	app, err := x.Appender()
	if err != nil {
		return nil, err
	}
	for _, s := range o.samples {
		if s.t < mint {
			continue
		}
		if s.t > maxt {
			break
		}
		app.Append(s.t, s.v)
	}
	return x, nil
}
