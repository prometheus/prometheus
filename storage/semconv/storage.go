// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package semconv

import (
	"context"
	"errors"
	"fmt"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	semconvURLLabel = "__semconv_url__"
	schemaURLLabel  = "__schema_url__"
)

// AwareStorage wraps given storage with a semconv awareness that
// performs versioned read when __semconv_url__ matcher is provided.
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
	var schemaURL, semconvURL string
	for _, m := range matchers {
		switch m.Name {
		case semconvURLLabel:
			if semconvURL != "" {
				return annotateSeriesSet(
					q.Querier.Select(ctx, sort, hints, matchers...),
					fmt.Sprintf("schema: __semconv_url__ matcher was used more than once, schematization logic is skipped for %v", matchers),
				)
			}
			if m.Type != labels.MatchEqual {
				return annotateSeriesSet(
					q.Querier.Select(ctx, sort, hints, matchers...),
					fmt.Sprintf("schema: __semconv_url__ matcher is ambiguous (not equal type), schematization logic is skipped for %v", matchers),
				)
			}
			semconvURL = m.Value
		case schemaURLLabel:
			if schemaURL != "" {
				return annotateSeriesSet(
					q.Querier.Select(ctx, sort, hints, matchers...),
					fmt.Sprintf("schema: __schema_url__ matcher was used more than once, schematization logic is skipped for %v", matchers),
				)
			}
			if m.Type != labels.MatchEqual {
				return annotateSeriesSet(
					q.Querier.Select(ctx, sort, hints, matchers...),
					fmt.Sprintf("schema: __schema_url__ matcher is ambiguous (not equal type), schematization logic is skipped for %v", matchers),
				)
			}
			schemaURL = m.Value
		}
	}
	if semconvURL == "" && schemaURL == "" {
		return q.Querier.Select(ctx, sort, hints, matchers...)
	}

	variants, qCtx, err := q.engine.FindMatcherVariants(semconvURL, schemaURL, matchers)
	if err != nil {
		return annotateSeriesSet(
			q.Querier.Select(ctx, sort, hints, matchers...),
			fmt.Errorf("schema: failed to find variants schematization logic is skipped for %v: %w", matchers, err).Error(),
		)
	}

	if qCtx.labelMapping == nil {
		// No changes detected, fast path without transformations.
		return q.Querier.Select(ctx, sort, hints, matchers...)
	}

	seriesSetChan := make(chan storage.SeriesSet, len(variants))
	seriesSet := make([]storage.SeriesSet, 0, len(variants))

	// TODO(bwplotka): Async limit?
	// Lookup alternative variants.
	for _, ms := range variants {
		go func() {
			// We need to sort for NewMergeSeriesSet to work.
			seriesSetChan <- &awareSeriesSet{
				SeriesSet: q.Querier.Select(ctx, true, hints, ms...),
				engine:    q.engine,
				qCtx:      qCtx,
			}
		}()
	}
	for range len(variants) {
		seriesSet = append(seriesSet, <-seriesSetChan)
	}
	return storage.NewMergeSeriesSet(seriesSet, 0, storage.ChainedSeriesMerge)
}

type awareSeriesSet struct {
	storage.SeriesSet

	qCtx   queryContext
	engine *schemaEngine

	at  storage.Series
	err error
}

func (s *awareSeriesSet) Err() error {
	if s.err != nil {
		return s.err
	}
	return s.SeriesSet.Err()
}

func (s *awareSeriesSet) At() storage.Series {
	return s.at
}

func (s *awareSeriesSet) Next() bool {
	if s.Err() != nil {
		return false
	}
	if !s.SeriesSet.Next() {
		return false
	}

	at := s.SeriesSet.At()
	lbls, err := s.engine.TransformSeries(s.qCtx, at.Labels())
	if err != nil {
		s.err = err
		return false
	}

	s.at = &awareSeries{Series: at, lbls: lbls}
	return true
}

type awareSeries struct {
	storage.Series

	lbls labels.Labels
}

func (s *awareSeries) Labels() labels.Labels {
	return s.lbls
}
