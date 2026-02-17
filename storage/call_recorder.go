package storage

import (
	"context"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/annotations"
)

// CallRecorderQueryable is used to record the queries made to the storage layer during tests.
type CallRecorderQueryable struct {
	q Queryable

	mtx      sync.Mutex
	queriers []*CallRecorderQuerier
}

func (c *CallRecorderQueryable) Queriers() []*CallRecorderQuerier {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	queriers := make([]*CallRecorderQuerier, 0, len(c.queriers))
	for _, q := range c.queriers {
		queriers = append(queriers, q)
	}
	return queriers
}

// CallRecorderQuerier is used to record the queries made to the storage layer during tests.
type CallRecorderQuerier struct {
	q Querier

	mtx             sync.RWMutex
	labelValuesCall []labelValuesCall
	labelNamesCall  []labelNamesCall
	selectCall      []SelectCall
}

func (c *CallRecorderQuerier) LabelValuesCalls() []labelValuesCall {
	c.mtx.RLock()
	defer c.mtx.RUnlock()
	return append([]labelValuesCall(nil), c.labelValuesCall...)
}

func (c *CallRecorderQuerier) LabelNamesCalls() []labelNamesCall {
	c.mtx.RLock()
	defer c.mtx.RUnlock()
	return append([]labelNamesCall(nil), c.labelNamesCall...)
}

func (c *CallRecorderQuerier) SelectCalls() []SelectCall {
	c.mtx.RLock()
	defer c.mtx.RUnlock()
	return append([]SelectCall(nil), c.selectCall...)
}

type labelValuesCall struct {
	name     string
	hints    *LabelHints
	matchers []*labels.Matcher
}

type labelNamesCall struct {
	hints    *LabelHints
	matchers []*labels.Matcher
}

type SelectCall struct {
	SortSeries bool
	Hints      *SelectHints
	Matchers   []*labels.Matcher
}

// NewCallRecorderQueryable creates a new CallRecorderQueryable that wraps the provided Queryable.
func NewCallRecorderQueryable(q Queryable) *CallRecorderQueryable {
	return &CallRecorderQueryable{
		q: q,
	}
}

// NewCallRecorderQuerier creates a new CallRecorderQuerier that wraps the provided Querier.
func NewCallRecorderQuerier(q Querier) *CallRecorderQuerier {
	return &CallRecorderQuerier{
		q: q,
	}
}

// Querier returns a Querier that records the calls made to it.
func (c *CallRecorderQueryable) Querier(mint, maxt int64) (Querier, error) {
	q, err := c.q.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()
	crQuerier := NewCallRecorderQuerier(q)
	c.queriers = append(c.queriers, crQuerier)
	return crQuerier, nil
}

func (c *CallRecorderQuerier) LabelValues(ctx context.Context, name string, hints *LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.labelValuesCall = append(c.labelValuesCall, labelValuesCall{
		name:     name,
		hints:    hints,
		matchers: matchers,
	})
	return c.q.LabelValues(ctx, name, hints, matchers...)
}

func (c *CallRecorderQuerier) LabelNames(ctx context.Context, hints *LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.labelNamesCall = append(c.labelNamesCall, labelNamesCall{
		hints:    hints,
		matchers: matchers,
	})
	return c.q.LabelNames(ctx, hints, matchers...)
}

func (c *CallRecorderQuerier) Close() error {
	return c.q.Close()
}

func (c *CallRecorderQuerier) Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.selectCall = append(c.selectCall, SelectCall{
		SortSeries: sortSeries,
		Hints:      hints,
		Matchers:   matchers,
	})
	return c.q.Select(ctx, sortSeries, hints, matchers...)
}

var _ Queryable = (*CallRecorderQueryable)(nil)
var _ Querier = (*CallRecorderQuerier)(nil)
