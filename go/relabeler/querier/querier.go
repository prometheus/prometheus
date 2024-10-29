package querier

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

type Deduplicator interface {
	Add(shard uint16, values ...string)
	Values() []string
}

type DeduplicatorFactory interface {
	Deduplicator(numberOfShards uint16) Deduplicator
}

type Querier struct {
	mint                int64
	maxt                int64
	head                relabeler.Head
	deduplicatorFactory DeduplicatorFactory
	closer              func() error
	metrics             *Metrics
}

func NewQuerier(head relabeler.Head, deduplicatorFactory DeduplicatorFactory, mint, maxt int64, closer func() error, metrics *Metrics) *Querier {
	return &Querier{
		mint:                mint,
		maxt:                maxt,
		head:                head,
		deduplicatorFactory: deduplicatorFactory,
		closer:              closer,
		metrics:             metrics,
	}
}

func (q *Querier) LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	start := time.Now()
	defer func() {
		if q.metrics != nil {
			q.metrics.LabelValuesDuration.With(
				prometheus.Labels{"generation": fmt.Sprintf("%d", q.head.Generation())},
			).Observe(float64(time.Since(start).Milliseconds()))
		}
	}()

	dedup := q.deduplicatorFactory.Deduplicator(q.head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)

	err := q.head.ForEachShard(func(shard relabeler.Shard) error {
		queryLabelValuesResult := shard.LSS().QueryLabelValues(name, convertedMatchers)
		if queryLabelValuesResult.Status() != cppbridge.LSSQueryStatusMatch {
			return fmt.Errorf("no matches on shard: %d", shard.ShardID())
		}

		dedup.Add(shard.ShardID(), queryLabelValuesResult.Values()...)
		return nil
	})

	anns := *annotations.New()
	if err != nil {
		anns.Add(err)
	}

	select {
	case <-ctx.Done():
		return nil, anns, context.Cause(ctx)
	default:
	}

	labelValues := dedup.Values()
	sort.Strings(labelValues)

	return labelValues, anns, nil
}

func (q *Querier) LabelNames(ctx context.Context, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	start := time.Now()
	defer func() {
		if q.metrics != nil {
			q.metrics.LabelNamesDuration.With(
				prometheus.Labels{"generation": fmt.Sprintf("%d", q.head.Generation())},
			).Observe(float64(time.Since(start).Milliseconds()))
		}
	}()

	dedup := q.deduplicatorFactory.Deduplicator(q.head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)

	err := q.head.ForEachShard(func(shard relabeler.Shard) error {
		queryLabelNamesResult := shard.LSS().QueryLabelNames(convertedMatchers)
		if queryLabelNamesResult.Status() != cppbridge.LSSQueryStatusMatch {
			return fmt.Errorf("no matches on shard: %d", shard.ShardID())
		}

		dedup.Add(shard.ShardID(), queryLabelNamesResult.Names()...)
		return nil
	})

	anns := *annotations.New()
	if err != nil {
		anns.Add(err)
	}

	select {
	case <-ctx.Done():
		return nil, anns, context.Cause(ctx)
	default:
	}

	labelNames := dedup.Values()
	sort.Strings(labelNames)

	return labelNames, anns, nil
}

func (q *Querier) Close() error {
	if q.closer != nil {
		return q.closer()
	}

	return nil
}

func (q *Querier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	start := time.Now()
	defer func() {
		if q.metrics != nil {
			q.metrics.SelectDuration.With(
				prometheus.Labels{"generation": fmt.Sprintf("%d", q.head.Generation())},
			).Observe(float64(time.Since(start).Milliseconds()))
		}
	}()

	seriesSets := make([]storage.SeriesSet, q.head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)

	err := q.head.ForEachShard(func(shard relabeler.Shard) error {
		lssQueryResult := shard.LSS().Query(convertedMatchers)

		if lssQueryResult.Status() != cppbridge.LSSQueryStatusMatch {
			seriesSets[shard.ShardID()] = &SeriesSet{}
			return fmt.Errorf("failed to query from shard: %d, query status: %d", shard.ShardID(), lssQueryResult.Status())
		}

		serializedChunks := shard.DataStorage().Query(cppbridge.HeadDataStorageQuery{
			StartTimestampMs: q.mint,
			EndTimestampMs:   q.maxt,
			LabelSetIDs:      lssQueryResult.Matches(),
		})

		if serializedChunks.NumberOfChunks() == 0 {
			seriesSets[shard.ShardID()] = &SeriesSet{}
			return fmt.Errorf("failed to query shard: %d, empty", shard.ShardID())
		}

		chunksIndex := serializedChunks.MakeIndex()
		getLabelSetsResult := shard.LSS().GetLabelSets(lssQueryResult.Matches())

		labelSetBySeriesID := make(map[uint32]labels.Labels)
		for index, labelSetID := range lssQueryResult.Matches() {
			if chunksIndex.Has(labelSetID) {
				labelSetBySeriesID[labelSetID] = getLabelSetsResult.LabelsSets()[index]
			}
		}

		localSeriesSets := make([]*Series, 0, chunksIndex.Len())
		deserializer := cppbridge.NewHeadDataStorageDeserializer(serializedChunks)
		for _, seriesID := range lssQueryResult.Matches() {
			chunksMetadata := chunksIndex.Chunks(serializedChunks, seriesID)
			if len(chunksMetadata) == 0 {
				continue
			}

			localSeriesSets = append(localSeriesSets, &Series{
				seriesID: seriesID,
				mint:     q.mint,
				maxt:     q.maxt,
				labelSet: cloneLabelSet(labelSetBySeriesID[seriesID]),
				sampleProvider: &DefaultSampleProvider{
					deserializer:   deserializer,
					chunksMetadata: chunksMetadata,
				},
			})
		}
		runtime.KeepAlive(getLabelSetsResult)

		seriesSets[shard.ShardID()] = NewSeriesSet(localSeriesSets)
		return nil
	})
	if err != nil {
		if !strings.Contains(err.Error(), "query status: 2") {
			logger.Warnf("QUERIER: Select failed: %s", err)
		}
		// todo: error
	}

	return storage.NewMergeSeriesSet(seriesSets, storage.ChainedSeriesMerge)
}

func convertPrometheusMatchersToOpcoreMatchers(matchers ...*labels.Matcher) (promppMatchers []model.LabelMatcher) {
	for _, matcher := range matchers {
		promppMatchers = append(promppMatchers, model.LabelMatcher{
			Name:        matcher.Name,
			Value:       matcher.Value,
			MatcherType: uint8(matcher.Type),
		})
	}

	return promppMatchers
}

func cloneLabelSet(labelSet labels.Labels) labels.Labels {
	n := 0
	for i := range labelSet {
		n += len(labelSet[i].Name) + len(labelSet[i].Value)
	}
	buf := make([]byte, n)
	offset := 0
	result := make(labels.Labels, len(labelSet))
	for i := range labelSet {
		n = copy(buf[offset:], *(*[]byte)(unsafe.Pointer(&labelSet[i].Name)))
		nb := buf[offset : offset+n]
		result[i].Name = *(*string)(unsafe.Pointer(&nb))
		offset += n

		n = copy(buf[offset:], *(*[]byte)(unsafe.Pointer(&labelSet[i].Value)))
		vb := buf[offset : offset+n]
		result[i].Value = *(*string)(unsafe.Pointer(&vb))
		offset += n
	}
	return result
}
