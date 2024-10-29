package querier

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

type MultiQuerier struct {
	mint     int64
	maxt     int64
	queriers []storage.Querier
	closer   func() error
}

func NewMultiQuerier(queriers []storage.Querier, closer func() error) *MultiQuerier {
	return &MultiQuerier{
		queriers: queriers,
		closer:   closer,
	}
}

func (q *MultiQuerier) LabelValues(
	ctx context.Context,
	name string,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	labelValuesResults := make([][]string, len(q.queriers))
	annotationResults := make([]annotations.Annotations, len(q.queriers))
	errs := make([]error, len(q.queriers))

	wg := &sync.WaitGroup{}
	for index, querier := range q.queriers {
		wg.Add(1)
		go func(index int, querier storage.Querier) {
			defer wg.Done()
			labelValuesResults[index], annotationResults[index], errs[index] = querier.LabelValues(
				ctx,
				name,
				matchers...,
			)
		}(index, querier)
	}

	wg.Wait()

	labelValues := DeduplicateAndSortStringSlices(labelValuesResults...)
	return labelValues, nil, errors.Join(errs...)
}

func (q *MultiQuerier) LabelNames(
	ctx context.Context,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	labelNamesResults := make([][]string, len(q.queriers))
	annotationResults := make([]annotations.Annotations, len(q.queriers))
	errs := make([]error, len(q.queriers))

	wg := &sync.WaitGroup{}
	for index, querier := range q.queriers {
		wg.Add(1)
		go func(index int, querier storage.Querier) {
			defer wg.Done()
			labelNamesResults[index], annotationResults[index], errs[index] = querier.LabelNames(ctx, matchers...)
		}(index, querier)
	}

	wg.Wait()

	labelNames := DeduplicateAndSortStringSlices(labelNamesResults...)

	return labelNames, nil, errors.Join(errs...)
}

func (q *MultiQuerier) Close() (err error) {
	for _, querier := range q.queriers {
		err = errors.Join(err, querier.Close())
	}

	if q.closer != nil {
		err = errors.Join(err, q.closer())
	}
	return err
}

func (q *MultiQuerier) Select(
	ctx context.Context,
	sortSeries bool,
	hints *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.SeriesSet {
	seriesSets := make([]storage.SeriesSet, len(q.queriers))
	wg := &sync.WaitGroup{}

	for index, querier := range q.queriers {
		wg.Add(1)
		go func(index int, querier storage.Querier) {
			defer wg.Done()
			seriesSets[index] = querier.Select(ctx, sortSeries, hints, matchers...)
		}(index, querier)
	}

	wg.Wait()

	return storage.NewMergeSeriesSet(seriesSets, storage.ChainedSeriesMerge)
}

func DeduplicateAndSortStringSlices(stringSlices ...[]string) []string {
	dedup := make(map[string]struct{})
	for _, stringSlice := range stringSlices {
		for _, value := range stringSlice {
			dedup[value] = struct{}{}
		}
	}

	result := make([]string, 0, len(dedup))
	for value := range dedup {
		result = append(result, value)
	}

	sort.Strings(result)
	return result
}
