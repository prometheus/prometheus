package head

import (
	"context"
	"errors"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
)

// InputRelabelingPromise - promise for processing incoming data.
type InputRelabelingPromise struct {
	mx                   *sync.Mutex
	statsMX              *sync.Mutex
	done                 chan struct{}
	data                 [][]*cppbridge.InnerSeries
	updateRelabelerTasks []*TaskUpdateRelabelerState
	errors               []error
	stats                cppbridge.RelabelerStats
	shardDone            uint16
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
		data:                 data,
		updateRelabelerTasks: make([]*TaskUpdateRelabelerState, 0, numberOfShards),
		errors:               make([]error, numberOfShards),
		shardDone:            numberOfShards,
		done:                 make(chan struct{}),
		mx:                   new(sync.Mutex),
		statsMX:              new(sync.Mutex),
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

// AddUpdateRelabelerTasks add to promise TaskUpdateRelabelerState.
func (p *InputRelabelingPromise) AddUpdateRelabelerTasks(updateTask *TaskUpdateRelabelerState) {
	p.mx.Lock()
	p.updateRelabelerTasks = append(p.updateRelabelerTasks, updateTask)
	p.mx.Unlock()
}

// AddUpdateTasks add to promise UpdateTasks.
func (p *InputRelabelingPromise) UpdateRelabeler() {
	for _, t := range p.updateRelabelerTasks {
		if err := t.Update(); err != nil {
			logger.Errorf("failed input update relabeler state: %s", err)
		}
	}
}

// AddStats add returned relabler stats.
func (p *InputRelabelingPromise) AddStats(stats cppbridge.RelabelerStats) {
	p.statsMX.Lock()
	p.stats.SamplesAdded += stats.SamplesAdded
	p.stats.SeriesAdded += stats.SeriesAdded
	p.statsMX.Unlock()
}

// Stats return relabler stats.
func (p *InputRelabelingPromise) Stats() cppbridge.RelabelerStats {
	return p.stats
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
