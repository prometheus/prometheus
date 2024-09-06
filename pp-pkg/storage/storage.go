package storage

import (
	"context"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

type QueryableStorage struct {
	queryable storage.Queryable
}

func (s *QueryableStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	return s.queryable.Querier(mint, maxt)
}

func (s *QueryableStorage) ChunkQuerier(_, _ int64) (storage.ChunkQuerier, error) {
	return noOpChunkQuerier{}, nil
}

func (s *QueryableStorage) Appender(_ context.Context) storage.Appender {
	return noOpAppender{}
}

func (s *QueryableStorage) StartTime() (int64, error) {
	return 0, nil
}

func (s *QueryableStorage) Close() error {
	return nil
}

func NewQueryableStorage(queryable storage.Queryable) *QueryableStorage {
	return &QueryableStorage{queryable: queryable}
}

type noOpAppender struct {
}

func (noOpAppender) Append(_ storage.SeriesRef, _ labels.Labels, _ int64, _ float64) (storage.SeriesRef, error) {
	return 0, nil
}

func (noOpAppender) Commit() error {
	return nil
}

func (noOpAppender) Rollback() error {
	return nil
}

func (noOpAppender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	return 0, nil
}

func (noOpAppender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	return 0, nil
}

func (noOpAppender) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	return 0, nil
}

func (noOpAppender) AppendCTZeroSample(ref storage.SeriesRef, l labels.Labels, t, ct int64) (storage.SeriesRef, error) {
	return 0, nil
}

type noOpChunkQuerier struct {
}

func (noOpChunkQuerier) LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (noOpChunkQuerier) LabelNames(ctx context.Context, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

func (noOpChunkQuerier) Close() error {
	return nil
}

func (noOpChunkQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.ChunkSeriesSet {
	return noOpChunkSeriesSet{}
}

type noOpChunkSeriesSet struct {
}

func (noOpChunkSeriesSet) Next() bool {
	return false
}

func (n noOpChunkSeriesSet) At() storage.ChunkSeries {
	return nil
}

func (n noOpChunkSeriesSet) Err() error {
	return nil
}

func (n noOpChunkSeriesSet) Warnings() annotations.Annotations {
	return nil
}
