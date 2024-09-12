package querier

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
}

func NewQuerier(head relabeler.Head, deduplicatorFactory DeduplicatorFactory, mint, maxt int64, closer func() error) *Querier {
	return &Querier{
		mint:                mint,
		maxt:                maxt,
		head:                head,
		deduplicatorFactory: deduplicatorFactory,
		closer:              closer,
	}
}

func (q *Querier) LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
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
	// defer func() {
	// 	fmt.Println("QUERIER: HEAD {", q.head.Generation(), "} Closed")
	// }()

	if q.closer != nil {
		return q.closer()
	}

	return nil
}

func (q *Querier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	// start := time.Now()
	// fmt.Println("QUERIER: HEAD{", q.head.Generation(), "} SELECT")
	seriesSets := make([]storage.SeriesSet, q.head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)

	err := q.head.ForEachShard(func(shard relabeler.Shard) error {
		lssQueryResult := shard.LSS().Query(convertedMatchers)

		if lssQueryResult.Status() != cppbridge.LSSQueryStatusMatch {
			seriesSets[shard.ShardID()] = &SeriesSet{}
			return fmt.Errorf("failed to query from shard: %d, query status: %d", shard.ShardID(), lssQueryResult.Status())
		}

		// fmt.Println("QUERIER: HEAD{", q.head.Generation(), "} SELECT shard: ", shard.ShardID(), ", queried label sets count: ", len(lssQueryResult.Matches()))

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

		// fmt.Println("QUERIER: HEAD{", q.head.Generation(), "} SELECT shard: ", shard.ShardID(), ", queried chunks: ", len(serializedChunks.ChunkMetadataList()))

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
		// fmt.Println("QUERIER: HEAD{", q.head.Generation(), "} Select Deserializer created")
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
		logger.Warnf("QUERIER: Select failed: %w", err)
		// todo: error
	}

	// fmt.Println("QUERIER: HEAD{", q.head.Generation(), "} Select finished, duration: ", time.Since(start))
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
