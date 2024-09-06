package querier

import (
	"context"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
	"sort"
	"strings"
	"sync"
)

type Querier struct {
	mint   int64
	maxt   int64
	head   relabeler.Head
	closer func() error
}

func NewQuerier(head relabeler.Head, mint, maxt int64, closer func() error) *Querier {
	return &Querier{
		mint:   mint,
		maxt:   maxt,
		head:   head,
		closer: closer,
	}
}

func (q *Querier) LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	mtx := &sync.Mutex{}
	dedup := make(map[string]struct{})
	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)

	err := q.head.ForEachShard(func(shard relabeler.Shard) error {
		queryLabelValuesResult := shard.LSS().QueryLabelValues(name, convertedMatchers)
		if queryLabelValuesResult.Status() != cppbridge.LSSQueryStatusMatch {
			return fmt.Errorf("no matches on shard: %d", shard.ShardID())
		}
		mtx.Lock()
		defer mtx.Unlock()
		for _, labelValue := range queryLabelValuesResult.Values() {
			if _, ok := dedup[labelValue]; !ok {
				dedup[strings.Clone(labelValue)] = struct{}{}
			}
		}
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

	labelValues := make([]string, 0, len(dedup))
	for value := range dedup {
		labelValues = append(labelValues, value)
	}
	sort.Strings(labelValues)

	return labelValues, anns, nil
}

func (q *Querier) LabelNames(ctx context.Context, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	mtx := &sync.Mutex{}
	dedup := make(map[string]struct{})
	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)

	err := q.head.ForEachShard(func(shard relabeler.Shard) error {
		queryLabelNamesResult := shard.LSS().QueryLabelNames(convertedMatchers)
		if queryLabelNamesResult.Status() != cppbridge.LSSQueryStatusMatch {
			return fmt.Errorf("no matches on shard: %d", shard.ShardID())
		}

		mtx.Lock()
		defer mtx.Unlock()
		for _, labelName := range queryLabelNamesResult.Names() {
			if _, ok := dedup[labelName]; !ok {
				dedup[strings.Clone(labelName)] = struct{}{}
			}
		}

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

	labelNames := make([]string, 0, len(dedup))
	for label := range dedup {
		labelNames = append(labelNames, label)
	}
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
	seriesSets := make([]storage.SeriesSet, q.head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)

	err := q.head.ForEachShard(func(shard relabeler.Shard) error {
		lssQueryResult := shard.LSS().Query(convertedMatchers)

		if lssQueryResult.Status() != cppbridge.LSSQueryStatusMatch {
			seriesSets[shard.ShardID()] = &SeriesSet{}
			return fmt.Errorf("failed to query from shard: %d, query status: %d", shard.ShardID(), lssQueryResult.Status())
		}

		fmt.Printf("shard: %d, queried label sets count: %d\n", shard.ShardID(), len(lssQueryResult.Matches()))

		getLabelSetsResult := shard.LSS().GetLabelSets(lssQueryResult.Matches())
		serializedChunks := shard.DataStorage().Query(cppbridge.HeadDataStorageQuery{
			StartTimestampMs: q.mint,
			EndTimestampMs:   q.maxt,
			LabelSetIDs:      lssQueryResult.Matches(),
		})

		if len(serializedChunks.ChunkMetadataList()) == 0 {
			seriesSets[shard.ShardID()] = &SeriesSet{}
			return fmt.Errorf("failed to query shard: %d, empty", shard.ShardID())
		}

		fmt.Printf("shard: %d, queried chunks: %d\n", shard.ShardID(), len(serializedChunks.ChunkMetadataList()))

		chunksBySeriesID := make(map[uint32][]cppbridge.HeadDataStorageSerializedChunkMetadata)
		for _, chunkMetadata := range serializedChunks.ChunkMetadataList() {
			cm := chunkMetadata
			chunksBySeriesID[chunkMetadata.SeriesID()] = append(chunksBySeriesID[chunkMetadata.SeriesID()], cm)
		}

		labelSetBySeriesID := make(map[uint32]labels.Labels)
		for index, labelSetID := range lssQueryResult.Matches() {
			if _, ok := chunksBySeriesID[labelSetID]; ok {
				labelSetBySeriesID[labelSetID] = getLabelSetsResult.LabelsSets()[index]
			}
		}

		localSeriesSets := make([]*Series, 0, len(chunksBySeriesID))
		deserializer := cppbridge.NewHeadDataStorageDeserializer(serializedChunks)
		for _, seriesID := range lssQueryResult.Matches() {
			chunksMetadata, ok := chunksBySeriesID[seriesID]
			if !ok {
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

		seriesSets[shard.ShardID()] = NewSeriesSet(localSeriesSets)
		return nil
	})
	if err != nil {
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
	result := make(labels.Labels, 0, len(labelSet))
	for _, label := range labelSet {
		result = append(result, labels.Label{
			Name:  strings.Clone(label.Name),
			Value: strings.Clone(label.Value),
		})
	}
	return result
}
