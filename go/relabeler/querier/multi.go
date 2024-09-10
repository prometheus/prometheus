package querier

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
	"sort"
	"sync"
	"time"
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

func (q *MultiQuerier) LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	fmt.Println("MULTIQUERIER: LabelValues")
	start := time.Now()

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

	labelValues := DeduplicateAndSortStringSlices(labelValuesResults...)
	fmt.Println("MULTIQUERIER: LabelValues finished, duration: ", time.Since(start).Microseconds())
	return labelValues, nil, errors.Join(errs...)
}

func (q *MultiQuerier) LabelNames(ctx context.Context, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	fmt.Println("MULTIQUERIER: LabelNames")
	start := time.Now()

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

	fmt.Println("MULTIQUERIER: LabelNames finished, duration: ", time.Since(start).Microseconds())
	return labelNames, nil, errors.Join(errs...)
}

func (q *MultiQuerier) Close() (err error) {
	for _, querier := range q.queriers {
		err = errors.Join(err, querier.Close())
	}

	if q.closer != nil {
		err = errors.Join(err, q.closer())
	}
	fmt.Println("MULTIQUERIER: Closed")
	return err
}

func (q *MultiQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	fmt.Println("MULTIQUERIER: Select")
	start := time.Now()

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

	fmt.Println("MULTIQUERIER: Select finished, duration: ", time.Since(start))
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
