package distributor

import (
	"context"
	"errors"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"sync"
)

// OutputRelabelingPromise - promise for processing output data.
type OutputRelabelingPromise struct {
	destinationGroups       *DestinationGroups
	mx                      *sync.Mutex
	done                    chan struct{}
	dgOutputInnerSeries     [][][]*cppbridge.InnerSeries   // [groupID[encoderShardID[mainShardID]]]
	dgOutputRelabeledSeries [][]*cppbridge.RelabeledSeries // [groupID[mainShardID]]
	errors                  []error                        // [groupID]
	shardDone               uint16
}

// NewOutputRelabelingPromise - init new *OutputRelabelingPromise.
func NewOutputRelabelingPromise(destinationGroups *DestinationGroups, numberOfShards uint16) *OutputRelabelingPromise {
	groups := len(*destinationGroups)
	p := &OutputRelabelingPromise{
		destinationGroups:       destinationGroups,
		mx:                      new(sync.Mutex),
		done:                    make(chan struct{}),
		dgOutputInnerSeries:     make([][][]*cppbridge.InnerSeries, groups),
		dgOutputRelabeledSeries: make([][]*cppbridge.RelabeledSeries, groups),
		errors:                  make([]error, groups),
		shardDone:               numberOfShards * uint16(groups),
	}

	for i := range p.dgOutputInnerSeries {
		// each destination group can have a different number of shards
		length := 1 << (*destinationGroups)[i].ShardsNumberPower()
		p.dgOutputInnerSeries[i] = make([][]*cppbridge.InnerSeries, length)
		p.shardDone += uint16(length)
		for j := range p.dgOutputInnerSeries[i] {
			p.dgOutputInnerSeries[i][j] = make([]*cppbridge.InnerSeries, 0, numberOfShards)
		}
	}

	for i := range p.dgOutputRelabeledSeries {
		p.dgOutputRelabeledSeries[i] = make([]*cppbridge.RelabeledSeries, numberOfShards)
	}

	return p
}

// AddError - add to promise error.
func (p *OutputRelabelingPromise) AddError(groupID int, dgShards uint16, err error) {
	p.mx.Lock()
	p.errors[groupID] = err
	p.shardDone--

	// if an error occurs in a group, there will be no processed data
	var shardID uint16
	for ; shardID < dgShards; shardID++ {
		p.dgOutputInnerSeries[groupID][shardID] = append(p.dgOutputInnerSeries[groupID][shardID], nil)
		if cap(p.dgOutputInnerSeries[groupID][shardID]) == len(p.dgOutputInnerSeries[groupID][shardID]) {
			p.shardDone--
		}

		if p.shardDone == 0 {
			close(p.done)
		}
	}
	p.mx.Unlock()
}

// AddOutputInnerSeries - add to promise inner series.
func (p *OutputRelabelingPromise) AddOutputInnerSeries(
	groupID int,
	shardID uint16,
	innerSeries *cppbridge.InnerSeries,
) {
	p.mx.Lock()
	if innerSeries.Size() == 0 {
		innerSeries = nil
	}

	p.dgOutputInnerSeries[groupID][shardID] = append(p.dgOutputInnerSeries[groupID][shardID], innerSeries)
	if cap(p.dgOutputInnerSeries[groupID][shardID]) == len(p.dgOutputInnerSeries[groupID][shardID]) {
		p.shardDone--
	}

	if p.shardDone == 0 {
		close(p.done)
	}
	p.mx.Unlock()
}

// AddOutputRelabeledSeries - add to promise relabeled series.
func (p *OutputRelabelingPromise) AddOutputRelabeledSeries(
	groupID int,
	shardID uint16,
	relabeledSeries *cppbridge.RelabeledSeries,
) {
	p.mx.Lock()
	if relabeledSeries.Size() == 0 {
		relabeledSeries = nil
	}

	p.dgOutputRelabeledSeries[groupID][shardID] = relabeledSeries
	p.shardDone--

	if p.shardDone == 0 {
		close(p.done)
	}
	p.mx.Unlock()
}

// DestinationGroups return pointer to DestinationGroups.
func (p *OutputRelabelingPromise) DestinationGroups() *DestinationGroups {
	return p.destinationGroups
}

// OutputInnerSeries - return OutputInnerSeries(series with ls id) on shards.
func (p *OutputRelabelingPromise) OutputInnerSeries(groupID int) [][]*cppbridge.InnerSeries {
	return p.dgOutputInnerSeries[groupID]
}

// OutputRelabeledSeries - return OutputRelabeledSeries(series with label set) on shards.
func (p *OutputRelabelingPromise) OutputRelabeledSeries(groupID int) []*cppbridge.RelabeledSeries {
	return p.dgOutputRelabeledSeries[groupID]
}

// Wait - wait until all results are received.
func (p *OutputRelabelingPromise) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-p.done:
		return errors.Join(p.errors...)
	}
}
