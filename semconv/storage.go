package semconv

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

const schemaURLLabel = "__schema_url__"

// AwareStorage wraps given storage with a semconv awareness that
// performs versioned read when __schema_url__ matcher is provided.
// TODO(bwplotka): Technically we only need Querier?
func AwareStorage(s storage.Storage) storage.Storage {
	return &awareStorage{Storage: s, engine: newSchemaEngine()}
}

type awareStorage struct {
	storage.Storage

	engine *schemaEngine
}

type awareQuerier struct {
	storage.Querier

	engine *schemaEngine
}

func (s *awareStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &awareQuerier{Querier: q, engine: s.engine}, nil
}

type annotatedSeriesSet struct {
	storage.SeriesSet

	warning string
}

func annotateSeriesSet(s storage.SeriesSet, warning string) storage.SeriesSet {
	return &annotatedSeriesSet{warning: warning, SeriesSet: s}
}

func (s *annotatedSeriesSet) Warnings() annotations.Annotations {
	got := s.SeriesSet.Warnings()
	return got.Add(errors.New(s.warning))
}

func (q *awareQuerier) Select(ctx context.Context, sort bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	var schemaURL string

	for _, m := range matchers {
		if m.Name != schemaURLLabel {
			continue
		}
		if schemaURL != "" {
			return annotateSeriesSet(
				q.Querier.Select(ctx, sort, hints, matchers...),
				fmt.Sprintf("schema: __schema_url__ matcher was used more than once, schematization logic is skipped for %v", matchers),
			)
		}
		if m.Type != labels.MatchEqual {
			return annotateSeriesSet(
				q.Querier.Select(ctx, sort, hints, matchers...),
				fmt.Sprintf("schema: __schema_url__ matcher is ambigious (not equal type), schematization logic is skipped for %v", matchers),
			)
		}
		schemaURL = m.Value
	}
	if schemaURL == "" {
		return q.Querier.Select(ctx, sort, hints, matchers...)
	}

	variants, err := q.engine.FindVariants(schemaURL, matchers)
	if err != nil {
		return annotateSeriesSet(
			q.Querier.Select(ctx, sort, hints, matchers...),
			fmt.Errorf("schema: failed to find variants %w, schematization logic is skipped for %v", err, matchers).Error(),
		)
	}

	var (
		wg            sync.WaitGroup
		seriesSetChan = make(chan storage.SeriesSet)
		seriesSet     = make([]storage.SeriesSet, 0, len(variants))
	)

	// TODO(bwplotka): Async limit?
	// Lookup alternative variants.
	for _, v := range variants {
		wg.Add(1)
		go func(m []*labels.Matcher) {
			defer wg.Done()

			// We need to sort for NewMergeSeriesSet to work.
			seriesSetChan <- v.SeriesSet(q.Querier.Select(ctx, true, hints, m...))
		}(v.matchers)
	}
	go func() {
		wg.Wait()
		close(seriesSetChan)
	}()

	for r := range seriesSetChan {
		seriesSet = append(seriesSet, r)
	}
	return storage.NewMergeSeriesSet(seriesSet, 0, storage.ChainedSeriesMerge)
}
