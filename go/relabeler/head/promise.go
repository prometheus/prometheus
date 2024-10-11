package head

import (
	"context"
	"errors"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"sync"
)

// InputRelabelingPromise - promise for processing incoming data.
type InputRelabelingPromise struct {
	mx        *sync.Mutex
	done      chan struct{}
	data      [][]*cppbridge.InnerSeries
	errors    []error
	shardDone uint16
}

// NewInputRelabelingPromise - init new *InputRelabelingPromise.
func NewInputRelabelingPromise(numberOfShards uint16) *InputRelabelingPromise {
	// id slice - shard id
	data := make([][]*cppbridge.InnerSeries, numberOfShards)
	for i := range data {
		// amount of data = x2 numberOfShards
		data[i] = make([]*cppbridge.InnerSeries, 0, 2*numberOfShards)
	}
	return &InputRelabelingPromise{
		data:      data,
		errors:    make([]error, numberOfShards),
		shardDone: numberOfShards,
		done:      make(chan struct{}),
		mx:        new(sync.Mutex),
	}
}

// AddError - add to promise error.
func (p *InputRelabelingPromise) AddError(shardID uint16, err error) {
	// error on shard
	p.mx.Lock()
	p.errors[shardID] = err
	p.shardDone--

	if p.shardDone == 0 {
		close(p.done)
	}
	p.mx.Unlock()
}

// AddResult - add to promise result.
func (p *InputRelabelingPromise) AddResult(shardID uint16, innerSeries *cppbridge.InnerSeries) {
	p.mx.Lock()
	if innerSeries != nil && innerSeries.Size() == 0 {
		innerSeries = nil
	}
	p.data[shardID] = append(p.data[shardID], innerSeries)
	if cap(p.data[shardID]) == len(p.data[shardID]) {
		p.shardDone--
	}
	if p.shardDone == 0 {
		close(p.done)
	}
	p.mx.Unlock()
}

// ShardsInnerSeries - return slice with the results of relabeling per shards.
func (p *InputRelabelingPromise) ShardsInnerSeries(shardID uint16) []*cppbridge.InnerSeries {
	return p.data[shardID]
}

// Wait - wait until all results are received.
func (p *InputRelabelingPromise) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-p.done:
		return errors.Join(p.errors...)
	}
}
