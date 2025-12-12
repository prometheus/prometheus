// Copyright 2025 The Prometheus Authors
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
	"slices"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

const schemaURLLabel = "__schema_url__"

// AwareStorage wraps given storage with a semconv awareness that
// performs versioned read when __schema_url__ matcher is provided.
// TODO(bwplotka): Technically we only need Querier?
func AwareStorage(s storage.Storage) (storage.Storage, func(config *config.Config) error) {
	e := newSchemaEngine()
	return &awareStorage{Storage: s, engine: e}, e.ApplyConfig
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
				fmt.Sprintf("schema: __schema_url__ matcher is ambiguous (not equal type), schematization logic is skipped for %v", matchers),
			)
		}
		schemaURL = m.Value
	}
	if schemaURL == "" {
		return q.Querier.Select(ctx, sort, hints, matchers...)
	}

	fmt.Printf("awareQuerier.Select - schemaURL: %s\n", schemaURL)
	variants, qCtx, err := q.engine.FindMatcherVariants(schemaURL, matchers)
	if err != nil {
		return annotateSeriesSet(
			q.Querier.Select(ctx, sort, hints, matchers...),
			fmt.Errorf("schema: failed to find variants schematization logic is skipped for %v: %w", matchers, err).Error(),
		)
	}

	fmt.Printf("%d variants\n", len(variants))
	if len(qCtx.changes) == 0 {
		// No changes detected, fast path without transformations.
		fmt.Printf("no changes detected\n")
		return q.Querier.Select(ctx, sort, hints, matchers...)
	}

	seriesSetChan := make(chan storage.SeriesSet, len(variants))
	seriesSet := make([]storage.SeriesSet, 0, len(variants))

	// TODO(bwplotka): Async limit?
	// Lookup alternative variants.
	for _, ms := range variants {
		go func() {
			// We need to sort for NewMergeSeriesSet to work.
			var matchers []string
			for _, m := range ms {
				matchers = append(matchers, m.String())
			}
			fmt.Printf("Adding awareSeriesSet for variants: %#v\n", matchers)
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
	lbls, vt, err := s.engine.TransformSeries(s.qCtx, at.Labels())
	if err != nil {
		s.err = err
		return false
	}

	s.at = &awareSeries{Series: at, lbls: lbls, vt: vt, magicSuffix: s.qCtx.magicSuffix}
	return true
}

type awareSeries struct {
	storage.Series

	lbls        labels.Labels
	vt          valueTransformer
	magicSuffix string
}

func (s *awareSeries) Labels() labels.Labels {
	return s.lbls
}

type awareIterator struct {
	chunkenc.Iterator

	typ         model.MetricType
	vt          valueTransformer
	magicSuffix string
}

func (s *awareSeries) Iterator(i chunkenc.Iterator) chunkenc.Iterator {
	metadata := schema.NewMetadataFromLabels(s.lbls)
	return &awareIterator{Iterator: s.Series.Iterator(i), typ: metadata.Type, vt: s.vt, magicSuffix: s.magicSuffix}
}

func (i *awareIterator) At() (int64, float64) {
	t, v := i.Iterator.At()
	// TODO(bwplotka): Do the same for summaries.
	if i.typ == model.MetricTypeHistogram && (i.magicSuffix == "_count" || i.magicSuffix == "_bucket") {
		return t, v
	}
	return t, i.vt.Transform(v)
}

func (i *awareIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	t, hist := i.Iterator.AtHistogram(h)
	// TODO: You can't really scale native histograms with exponential scheme. Handle this (error, approx, validation).

	if hist.UsesCustomBuckets() && len(i.vt.expr) > 0 {
		hist = hist.Copy()
		hist.Sum = i.vt.Transform(hist.Sum)
		// Copy CustomValues since histogram.Copy() shares it by reference.
		hist.CustomValues = slices.Clone(hist.CustomValues)
		for cvi := range hist.CustomValues {
			hist.CustomValues[cvi] = i.vt.Transform(hist.CustomValues[cvi])
		}
	}
	return t, hist
}

func (i *awareIterator) AtFloatHistogram(fh *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	t, hist := i.Iterator.AtFloatHistogram(fh)
	// TODO: You can't really scale native histograms with exponential scheme. Handle this (error, approx, validation).

	if hist.UsesCustomBuckets() && len(i.vt.expr) > 0 {
		hist = hist.Copy()
		hist.Sum = i.vt.Transform(hist.Sum)
		// Copy CustomValues since histogram.Copy() shares it by reference.
		hist.CustomValues = slices.Clone(hist.CustomValues)
		for cvi := range hist.CustomValues {
			hist.CustomValues[cvi] = i.vt.Transform(hist.CustomValues[cvi])
		}
	}
	return t, hist
}
