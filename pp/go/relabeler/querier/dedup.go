package querier

import (
	"strings"
	"sync"
)

type DeduplicatorFactoryFunc func(numberOfShards uint16) Deduplicator

func (fn DeduplicatorFactoryFunc) Deduplicator(numberOfShards uint16) Deduplicator {
	return fn(numberOfShards)
}

type InstantDeduplicator struct {
	mtx    sync.Mutex
	values map[string]struct{}
}

func NewInstantDeduplcator() *InstantDeduplicator {
	return &InstantDeduplicator{
		values: make(map[string]struct{}),
	}
}

func (d *InstantDeduplicator) Add(_ uint16, values ...string) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	for _, value := range values {
		if _, ok := d.values[value]; !ok {
			d.values[strings.Clone(value)] = struct{}{}
		}
	}
}

func (d *InstantDeduplicator) Values() []string {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	values := make([]string, 0, len(d.values))
	for value := range d.values {
		values = append(values, value)
	}
	return values
}

func (d *InstantDeduplicator) Reset() {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.values = make(map[string]struct{})
}

type NoOpShardedDeduplicator struct {
	shardedValues [][]string
}

func NewNoOpShardedDeduplicator(numberOfShards uint16) *NoOpShardedDeduplicator {
	return &NoOpShardedDeduplicator{shardedValues: make([][]string, numberOfShards)}
}

func (d *NoOpShardedDeduplicator) Add(shard uint16, values ...string) {
	clonedValues := make([]string, 0, len(values))
	for _, value := range values {
		clonedValues = append(clonedValues, strings.Clone(value))
	}
	d.shardedValues[shard] = clonedValues
}

func (d *NoOpShardedDeduplicator) Values() []string {
	var values []string
	for _, shardedValues := range d.shardedValues {
		values = append(values, shardedValues...)
	}
	return values
}

func (d *NoOpShardedDeduplicator) Reset() {
	d.shardedValues = make([][]string, len(d.shardedValues))
}

func InstantDeduplicatorFactory() DeduplicatorFactoryFunc {
	return func(_ uint16) Deduplicator {
		return NewInstantDeduplcator()
	}
}

func NoOpShardedDeduplicatorFactory() DeduplicatorFactoryFunc {
	return func(numberOfShards uint16) Deduplicator {
		return NewNoOpShardedDeduplicator(numberOfShards)
	}
}
