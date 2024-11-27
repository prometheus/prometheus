package head

import (
	"fmt"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/client_golang/prometheus"
)

const chanBufferSize = 64

type LSS struct {
	input  *cppbridge.LabelSetStorage
	target *cppbridge.LabelSetStorage
}

func (w *LSS) Raw() *cppbridge.LabelSetStorage {
	return w.target
}

func (w *LSS) AllocatedMemory() uint64 {
	return w.input.AllocatedMemory() + w.target.AllocatedMemory()
}

func (w *LSS) QueryLabelValues(
	label_name string,
	matchers []model.LabelMatcher,
) *cppbridge.LSSQueryLabelValuesResult {
	return w.target.QueryLabelValues(label_name, matchers)
}

func (w *LSS) QueryLabelNames(matchers []model.LabelMatcher) *cppbridge.LSSQueryLabelNamesResult {
	return w.target.QueryLabelNames(matchers)
}

func (w *LSS) Query(matchers []model.LabelMatcher) *cppbridge.LSSQueryResult {
	return w.target.Query(matchers)
}

func (w *LSS) GetLabelSets(labelSetIDs []uint32) *cppbridge.LabelSetStorageGetLabelSetsResult {
	return w.target.GetLabelSets(labelSetIDs)
}

type DataStorage struct {
	dataStorage *cppbridge.HeadDataStorage
	encoder     *cppbridge.HeadEncoder
}

func (ds *DataStorage) AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries) {
	ds.encoder.EncodeInnerSeriesSlice(innerSeriesSlice)
}

func (ds *DataStorage) Raw() *cppbridge.HeadDataStorage {
	return ds.dataStorage
}

func (ds *DataStorage) MergeOutOfOrderChunks() {
	ds.encoder.MergeOutOfOrderChunks()
}

func (ds *DataStorage) Query(query cppbridge.HeadDataStorageQuery) *cppbridge.HeadDataStorageSerializedChunks {
	return ds.dataStorage.Query(query)
}

func (ds *DataStorage) AllocatedMemory() uint64 {
	return ds.dataStorage.AllocatedMemory()
}

// reshards changes the number of shards to the required amount.
func (h *Head) reconfigure(
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) error {
	return h.reconfigureRelabelersData(inputRelabelerConfigs, numberOfShards)
}

// reconfigureRelabelersData reconfiguring relabelers data for all shards.
func (h *Head) reconfigureRelabelersData(
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) error {
	updated := make(map[string]struct{})
	for _, cfgs := range inputRelabelerConfigs {
		relabelerID := cfgs.GetName()
		if rd, ok := h.relabelersData[relabelerID]; ok {
			if err := rd.Reconfigure(cfgs.GetConfigs(), h.generation, numberOfShards); err != nil {
				return err
			}
			updated[relabelerID] = struct{}{}
			continue
		}

		rd, err := NewRelabelerData(
			cfgs.GetConfigs(),
			h.generation,
			numberOfShards,
		)
		if err != nil {
			return err
		}
		h.relabelersData[relabelerID] = rd
		updated[relabelerID] = struct{}{}
	}

	for relabelerID := range h.relabelersData {
		if _, ok := updated[relabelerID]; !ok {
			// clear unnecessary
			h.memoryInUse.DeletePartialMatch(prometheus.Labels{
				"allocator": fmt.Sprintf("input_relabeler_%s", relabelerID),
			})
			delete(h.relabelersData, relabelerID)
		}
	}

	return nil
}

// shardLoop run relabeling on the shard.
//
//revive:disable-next-line:function-length long but readable.
//revive:disable-next-line:cognitive-complexity long but understandable.
//revive:disable-next-line:cyclomatic long but understandable.
func (h *Head) shardLoop(shardID uint16, stopc chan struct{}) {
	for {
		select {
		case <-stopc:
			return
		case task := <-h.stageInputRelabeling[shardID]:
			shardsInnerSeries := cppbridge.NewShardsInnerSeries(h.numberOfShards)
			shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(h.numberOfShards)

			var (
				err   error
				stats cppbridge.RelabelerStats
			)
			if task.WithStaleNans() {
				stats, err = task.InputRelabelerByShard(shardID).InputRelabelingWithStalenans(
					task.Ctx(),
					h.lsses[shardID].input,
					h.lsses[shardID].target,
					task.CacheByShard(shardID),
					task.Options(),
					task.StaleNansStateByShard(shardID),
					task.StaleNansTS(),
					task.ShardedData(),
					shardsInnerSeries,
					shardsRelabeledSeries,
				)
			} else {
				stats, err = task.InputRelabelerByShard(shardID).InputRelabeling(
					task.Ctx(),
					h.lsses[shardID].input,
					h.lsses[shardID].target,
					task.CacheByShard(shardID),
					task.Options(),
					task.ShardedData(),
					shardsInnerSeries,
					shardsRelabeledSeries,
				)
			}

			task.IncomingDataDestroy()
			if err != nil {
				task.AddError(shardID, fmt.Errorf("failed input relabeling shard %d: %w", shardID, err))
				continue
			}

			task.AddStats(stats)
			for sid, relabeledSeries := range shardsRelabeledSeries {
				if relabeledSeries.Size() == 0 {
					task.AddResult(uint16(sid), nil)
					continue
				}

				h.stageAppendRelabelerSeries[sid] <- NewTaskAppendRelabelerSeries(
					task.Ctx(),
					relabeledSeries,
					task.Promise(),
					task.RelabelerData(),
					task.State(),
					shardID,
				)
			}

			for sid, innerSeries := range shardsInnerSeries {
				task.AddResult(uint16(sid), innerSeries)
			}

		case task := <-h.stageAppendRelabelerSeries[shardID]:
			relabelerStateUpdate := cppbridge.NewRelabelerStateUpdate()
			innerSeries := cppbridge.NewInnerSeries()

			if err := task.InputRelabelerByShard(shardID).AppendRelabelerSeries(
				task.Ctx(),
				h.lsses[shardID].target,
				relabelerStateUpdate,
				innerSeries,
				task.RelabeledSeries(),
			); err != nil {
				task.AddError(shardID, fmt.Errorf("failed input append relabeler series shard %d: %w", shardID, err))
				continue
			}

			task.AddUpdateRelabelerTasks(NewTaskUpdateRelabelerState(
				task.Ctx(),
				relabelerStateUpdate,
				task.InputRelabelerByShard(task.SourceShardID()),
				task.CacheByShard(task.SourceShardID()),
				shardID,
			))

			task.AddResult(shardID, innerSeries)
		case task := <-h.genericTaskCh[shardID]:
			task.ExecuteOnShard(&shard{
				id:          shardID,
				lss:         h.lsses[shardID],
				dataStorage: h.dataStorages[shardID],
				wal:         h.wals[shardID],
			})
		}
	}
}

type shard struct {
	id          uint16
	lss         *LSS
	dataStorage *DataStorage
	wal         *ShardWal
}

func (s *shard) ShardID() uint16 {
	return s.id
}

func (s *shard) DataStorage() relabeler.DataStorage {
	return s.dataStorage
}

func (s *shard) LSS() relabeler.LSS {
	return s.lss
}

func (s *shard) Wal() relabeler.Wal {
	return s.wal
}
