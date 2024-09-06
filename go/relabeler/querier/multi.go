package querier

import (
	"context"
	"errors"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
	"sync"
)

type MultiQuerier struct {
	mint     int64
	maxt     int64
	queriers []storage.Querier
	closer   func() error
}

func NewMultiQuerier(queriers ...storage.Querier) *MultiQuerier {
	return &MultiQuerier{
		queriers: queriers,
	}
}

func (q *MultiQuerier) LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	labelValuesResults := make([][]string, len(q.queriers))
	annotationResults := make([]annotations.Annotations, len(q.queriers))
	errs := make([]error, len(q.queriers))

	wg := &sync.WaitGroup{}
	for index, querier := range q.queriers {
		wg.Add(1)
		go func(index int, querier storage.Querier) {
			defer wg.Done()
			labelValuesResults[index], annotationResults[index], errs[index] = querier.LabelValues(ctx, name, matchers...)

		}(index, querier)
	}

	wg.Wait()

	dedup := make(map[string]struct{})
	for _, labelValues := range labelValuesResults {
		for _, labelValue := range labelValues {
			if _, ok := dedup[labelValue]; !ok {
				dedup[labelValue] = struct{}{}
			}
		}
	}

	labelValues := make([]string, 0, len(dedup))
	for labelValue := range dedup {
		labelValues = append(labelValues, labelValue)
	}

	return labelValues, nil, errors.Join(errs...)
}

func (q *MultiQuerier) LabelNames(ctx context.Context, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
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

	dedup := make(map[string]struct{})
	for _, labelNames := range labelNamesResults {
		for _, labelName := range labelNames {
			if _, ok := dedup[labelName]; !ok {
				dedup[labelName] = struct{}{}
			}
		}
	}

	labelNames := make([]string, 0, len(dedup))
	for labelName := range dedup {
		labelNames = append(labelNames, labelName)
	}

	return labelNames, nil, errors.Join(errs...)
}

func (q *MultiQuerier) Close() (err error) {
	for _, querier := range q.queriers {
		err = errors.Join(err, querier.Close())
	}
	return err
}

func (q *MultiQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
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
