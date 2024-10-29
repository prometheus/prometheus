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
	h.reconfigureStages(numberOfShards)

	h.reconfigureLsses(numberOfShards)

	if err := h.reconfigureRelabelersData(inputRelabelerConfigs, numberOfShards); err != nil {
		return err
	}

	h.reconfigureDataStorages(numberOfShards)

	h.numberOfShards = numberOfShards

	return nil
}

// reconfiguringStages reconfiguring stages for all shards.
func (h *Head) reconfigureStages(numberOfShards uint16) {
	if h.numberOfShards == numberOfShards {
		return
	}

	h.stageInputRelabeling = make([]chan *TaskInputRelabeling, numberOfShards)
	h.stageAppendRelabelerSeries = make([]chan *TaskAppendRelabelerSeries, numberOfShards)
	h.genericTaskCh = make([]chan *GenericTask, numberOfShards)

	var shardID uint16
	for ; shardID < numberOfShards; shardID++ {
		h.stageInputRelabeling[shardID] = make(chan *TaskInputRelabeling, chanBufferSize)
		h.stageAppendRelabelerSeries[shardID] = make(chan *TaskAppendRelabelerSeries, chanBufferSize)
		h.genericTaskCh[shardID] = make(chan *GenericTask, chanBufferSize)
	}
}

// reconfiguringShardLsses reconfiguring lss for all shards.
func (h *Head) reconfigureLsses(numberOfShards uint16) {
	if h.numberOfShards == numberOfShards {
		return
	}

	if len(h.lsses) > int(numberOfShards) {
		for shardID := range h.lsses {
			if shardID >= int(numberOfShards) {
				// clear unnecessary
				h.lsses[shardID] = nil
				h.memoryInUse.Delete(prometheus.Labels{"allocator": "main_lss", "id": fmt.Sprintf("%d", shardID)})
				continue
			}
		}
		// cut
		h.lsses = h.lsses[:numberOfShards]
		return
	}

	// resize
	h.lsses = append(
		h.lsses,
		make([]*cppbridge.LabelSetStorage, int(numberOfShards)-len(h.lsses))...,
	)
	for shardID := 0; shardID < int(numberOfShards); shardID++ {
		if h.lsses[shardID] != nil {
			continue
		}

		// create if not exist
		h.lsses[shardID] = cppbridge.NewQueryableLssStorage()
	}
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

func (h *Head) reconfigureDataStorages(numberOfShards uint16) {
	if h.numberOfShards == numberOfShards {
		return
	}

	if len(h.dataStorages) > int(numberOfShards) {
		for shardID := range h.dataStorages {
			if shardID >= int(numberOfShards) {
				h.dataStorages[shardID] = nil
			}
		}
		h.dataStorages = h.dataStorages[:numberOfShards]
		return
	}

	h.dataStorages = append(
		h.dataStorages,
		make([]*DataStorage, int(numberOfShards)-len(h.dataStorages))...,
	)

	for shardID := 0; shardID < int(numberOfShards); shardID++ {
		if h.dataStorages[shardID] != nil {
			continue
		}

		dataStorage := cppbridge.NewHeadDataStorage()
		h.dataStorages[shardID] = &DataStorage{
			dataStorage: dataStorage,
			encoder:     cppbridge.NewHeadEncoderWithDataStorage(dataStorage),
		}
	}

	return
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

			var err error
			if task.WithStaleNans() {
				err = task.InputRelabelerByShard(shardID).InputRelabelingWithStalenans(
					task.Ctx(),
					h.lsses[shardID],
					task.CacheByShard(shardID),
					task.Options(),
					task.SourceStateByShard(shardID),
					task.StaleNansTS(),
					task.ShardedData(),
					shardsInnerSeries,
					shardsRelabeledSeries,
				)
			} else {
				err = task.InputRelabelerByShard(shardID).InputRelabeling(
					task.Ctx(),
					h.lsses[shardID],
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
				h.lsses[shardID],
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
				dataStorage: h.dataStorages[shardID],
				lssWrapper:  &lssWrapper{lss: h.lsses[shardID]},
			})
		}
	}
}

type lssWrapper struct {
	lss *cppbridge.LabelSetStorage
}

func (w *lssWrapper) Raw() *cppbridge.LabelSetStorage {
	return w.lss
}

func (w *lssWrapper) AllocatedMemory() uint64 {
	return w.lss.AllocatedMemory()
}

func (w *lssWrapper) QueryLabelValues(
	label_name string,
	matchers []model.LabelMatcher,
) *cppbridge.LSSQueryLabelValuesResult {
	return w.lss.QueryLabelValues(label_name, matchers)
}

func (w *lssWrapper) QueryLabelNames(matchers []model.LabelMatcher) *cppbridge.LSSQueryLabelNamesResult {
	return w.lss.QueryLabelNames(matchers)
}

func (w *lssWrapper) Query(matchers []model.LabelMatcher) *cppbridge.LSSQueryResult {
	return w.lss.Query(matchers)
}

func (w *lssWrapper) GetLabelSets(labelSetIDs []uint32) *cppbridge.LabelSetStorageGetLabelSetsResult {
	return w.lss.GetLabelSets(labelSetIDs)
}

type shard struct {
	id          uint16
	lssWrapper  *lssWrapper
	dataStorage *DataStorage
}

func (s *shard) ShardID() uint16 {
	return s.id
}

func (s *shard) DataStorage() relabeler.DataStorage {
	return s.dataStorage
}

func (s *shard) LSS() relabeler.LSS {
	return s.lssWrapper
}
