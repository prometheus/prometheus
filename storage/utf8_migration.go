package storage

import (
	"context"
	"slices"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/strutil"
)

type mixedUTF8BlockQuerier struct {
	Querier
	es model.EscapingScheme
}

func (q *mixedUTF8BlockQuerier) LabelValues(ctx context.Context, name string, hints *LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	vals, an, err := q.Querier.LabelValues(ctx, name, hints, matchers...)
	if err != nil {
		return nil, nil, err
	}
	newMatchers, escaped, original, ok := escapeUTF8NameMatcher(matchers, q.es)
	var vals2 []string
	if ok {
		vals2, _, err = q.Querier.LabelValues(ctx, name, hints, newMatchers...)
		if err == nil && name == model.MetricNameLabel {
			for i := range vals2 {
				if vals2[i] == escaped {
					vals2[i] = original
				}
			}
		}
		vals = strutil.MergeStrings(vals, vals2)
	}
	if ix := slices.Index(vals, ""); ix != -1 && len(vals) > 1 {
		vals = append(vals[:ix], vals[ix+1:]...)
	}
	return vals, an, err
}

func (q *mixedUTF8BlockQuerier) LabelNames(ctx context.Context, hints *LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	names, an, err := q.Querier.LabelNames(ctx, hints, matchers...)
	if err != nil {
		return nil, nil, err
	}
	newMatchers, _, _, ok := escapeUTF8NameMatcher(matchers, q.es)
	if ok {
		names2, _, err := q.Querier.LabelNames(ctx, hints, newMatchers...)
		if err == nil {
			names = strutil.MergeStrings(names, names2)
		}
	}
	return names, an, err
}

func (q *mixedUTF8BlockQuerier) Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	newMatchers, escaped, original, ok := escapeUTF8NameMatcher(matchers, q.es)

	if !ok {
		return q.Querier.Select(ctx, sortSeries, hints, matchers...)
	}

	// We need to sort for merge to work.
	return NewMergeSeriesSet([]SeriesSet{
		q.Querier.Select(ctx, true, hints, matchers...),
		&metricRenameSeriesSet{SeriesSet: q.Querier.Select(ctx, true, hints, newMatchers...), from: escaped, to: original},
	}, ChainedSeriesMerge)
}

type mixedUTF8BlockChunkQuerier struct {
	ChunkQuerier
	es model.EscapingScheme
}

func (q *mixedUTF8BlockChunkQuerier) LabelValues(ctx context.Context, name string, hints *LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	vals, an, err := q.ChunkQuerier.LabelValues(ctx, name, hints, matchers...)
	if err != nil {
		return nil, nil, err
	}
	newMatchers, escaped, original, ok := escapeUTF8NameMatcher(matchers, q.es)
	var vals2 []string
	if ok {
		vals2, _, err = q.ChunkQuerier.LabelValues(ctx, name, hints, newMatchers...)
		if err == nil && name == model.MetricNameLabel {
			for i := range vals2 {
				if vals2[i] == escaped {
					vals2[i] = original
				}
			}
		}
		vals = strutil.MergeStrings(vals, vals2)
	}
	if ix := slices.Index(vals, ""); ix != -1 && len(vals) > 1 {
		vals = append(vals[:ix], vals[ix+1:]...)
	}
	return vals, an, err
}

func (q *mixedUTF8BlockChunkQuerier) LabelNames(ctx context.Context, hints *LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	names, an, err := q.ChunkQuerier.LabelNames(ctx, hints, matchers...)
	if err != nil {
		return nil, nil, err
	}
	newMatchers, _, _, ok := escapeUTF8NameMatcher(matchers, q.es)
	if ok {
		names2, _, err := q.ChunkQuerier.LabelNames(ctx, hints, newMatchers...)
		if err == nil {
			names = strutil.MergeStrings(names, names2)
		}
	}
	return names, an, err
}

func (q *mixedUTF8BlockChunkQuerier) Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) ChunkSeriesSet {
	newMatchers, escaped, original, ok := escapeUTF8NameMatcher(matchers, q.es)

	if !ok {
		return q.ChunkQuerier.Select(ctx, sortSeries, hints, matchers...)
	}

	// We need to sort for merge to work.
	return NewMergeChunkSeriesSet([]ChunkSeriesSet{
		q.ChunkQuerier.Select(ctx, true, hints, matchers...),
		&metricRenameChunkSeriesSet{ChunkSeriesSet: q.ChunkQuerier.Select(ctx, true, hints, newMatchers...), from: escaped, to: original},
	}, NewCompactingChunkSeriesMerger(ChainedSeriesMerge))
}

type escapedUTF8BlockQuerier struct {
	Querier
	es model.EscapingScheme
}

func (q *escapedUTF8BlockQuerier) LabelValues(ctx context.Context, name string, hints *LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	panic("not implemented")
}

func (q *escapedUTF8BlockQuerier) LabelNames(ctx context.Context, hints *LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	panic("not implemented")
}

func (q *escapedUTF8BlockQuerier) Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	newMatchers, escaped, original, ok := escapeUTF8NameMatcher(matchers, q.es)
	if !ok {
		return q.Querier.Select(ctx, sortSeries, hints, matchers...)
	}
	return &metricRenameSeriesSet{SeriesSet: q.Querier.Select(ctx, sortSeries, hints, newMatchers...), from: escaped, to: original}
}

func escapeUTF8NameMatcher(matchers []*labels.Matcher, es model.EscapingScheme) (newMatchers []*labels.Matcher, escaped, original string, ok bool) {
	// TODO: avoid allocation if there is nothing to escape?
	newMatchers = make([]*labels.Matcher, len(matchers))

	for i, m := range matchers {
		m2 := *m
		if m.Type == labels.MatchEqual && m.Name == model.MetricNameLabel && !model.IsValidLegacyMetricName(m.Value) {
			// TODO: what if we get multiple and different __name__ matchers?
			// Leaning towards ignoring everything and querying the underlying querier as is. Results will and should be empty.
			original = m.Value
			m2.Value = model.EscapeName(m.Value, es)
			escaped = m2.Value
			ok = true
		}
		newMatchers[i] = &m2
	}
	return
}

type escapedUTF8BlockChunkQuerier struct {
	ChunkQuerier
	es model.EscapingScheme
}

func (q *escapedUTF8BlockChunkQuerier) LabelValues(ctx context.Context, name string, hints *LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	panic("not implemented")
}

func (q *escapedUTF8BlockChunkQuerier) LabelNames(ctx context.Context, hints *LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	newMatchers, _, _, ok := escapeUTF8NameMatcher(matchers, q.es)
	if ok {
		matchers = newMatchers
	}
	return q.ChunkQuerier.LabelNames(ctx, hints, matchers...)
}

func (q *escapedUTF8BlockChunkQuerier) Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) ChunkSeriesSet {
	newMatchers, escaped, original, ok := escapeUTF8NameMatcher(matchers, q.es)
	if !ok {
		return q.ChunkQuerier.Select(ctx, sortSeries, hints, matchers...)
	}
	return &metricRenameChunkSeriesSet{ChunkSeriesSet: q.ChunkQuerier.Select(ctx, sortSeries, hints,
		newMatchers...), from: escaped, to: original}
}

type metricRenameSeriesSet struct {
	SeriesSet
	from, to string
}

func (u *metricRenameSeriesSet) At() Series {
	lbls := labels.NewScratchBuilder(u.SeriesSet.At().Labels().Len())
	u.SeriesSet.At().Labels().Range(func(l labels.Label) {
		// TODO: what if we don't find the label we need to map? That would be
		// an important bug, because that would break our assumptions that keep
		// the series sorted. Panic? Return Next=false and Err= not nil?
		if l.Name == model.MetricNameLabel && l.Value == u.from {
			lbls.Add(l.Name, u.to)
		} else {
			lbls.Add(l.Name, l.Value)
		}
	})

	return &SeriesEntry{
		Lset: lbls.Labels(),
		SampleIteratorFn: func(it chunkenc.Iterator) chunkenc.Iterator {
			return u.SeriesSet.At().Iterator(it)
		},
	}
}

func (u *metricRenameSeriesSet) Warnings() annotations.Annotations {
	// Warnings are for the whole set, so no sorting needed. However:
	// TODO: can a warning be referencing a metric name? Would that be a problem? I think not, but would be confusing.
	// TODO: should we add a warning about the renaming?
	return u.SeriesSet.Warnings()
}

type metricRenameChunkSeriesSet struct {
	ChunkSeriesSet
	from, to string
}

func (u *metricRenameChunkSeriesSet) At() ChunkSeries {
	lbls := labels.NewScratchBuilder(u.ChunkSeriesSet.At().Labels().Len())
	u.ChunkSeriesSet.At().Labels().Range(func(l labels.Label) {
		// TODO: what if we don't find the label we need to map? That would be
		// an important bug, because that would break our assumptions that keep
		// the series sorted. Panic? Return Next=false and Err= not nil?
		if l.Name == model.MetricNameLabel && l.Value == u.from {
			lbls.Add(l.Name, u.to)
		} else {
			lbls.Add(l.Name, l.Value)
		}
	})

	return &ChunkSeriesEntry{
		Lset: lbls.Labels(),
		ChunkIteratorFn: func(it chunks.Iterator) chunks.Iterator {
			return u.ChunkSeriesSet.At().Iterator(it)
		},
	}
}

func (u *metricRenameChunkSeriesSet) Warnings() annotations.Annotations {
	// Warnings are for the whole set, so no sorting needed. However:
	// TODO: can a warning be referencing a metric name? Would that be a problem? I think not, but would be confusing.
	// TODO: should we add a warning about the renaming?
	return u.ChunkSeriesSet.Warnings()
}
