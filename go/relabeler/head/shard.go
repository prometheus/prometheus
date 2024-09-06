package head

import (
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/client_golang/prometheus"
)

const chanBufferSize = 64

// relabelerKey - key for relabeler.
type relabelerKey struct {
	relabelerID string
	shardID     uint16
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

// reshards changes the number of shards to the required amount.
func (h *Head) reconfigure(
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) error {
	h.reconfigureStages(numberOfShards)

	h.reconfigureLsses(numberOfShards)

	if err := h.reconfigureInputRelabeler(inputRelabelerConfigs, numberOfShards); err != nil {
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
	h.stageUpdateRelabelerState = make([]chan *TaskUpdateRelabelerState, numberOfShards)
	h.genericTaskCh = make([]chan *GenericTask, numberOfShards)

	var shardID uint16
	for ; shardID < numberOfShards; shardID++ {
		h.stageInputRelabeling[shardID] = make(chan *TaskInputRelabeling, chanBufferSize)
		h.stageAppendRelabelerSeries[shardID] = make(chan *TaskAppendRelabelerSeries, chanBufferSize)
		h.stageUpdateRelabelerState[shardID] = make(chan *TaskUpdateRelabelerState, chanBufferSize)
		h.genericTaskCh[shardID] = make(chan *GenericTask, chanBufferSize)
	}
}

//// relabelerIDIsExist check on exist relabelerID.
//func (h *ShardHead) relabelerIDIsExist(relabelerID string) bool {
//	s.irrwm.RLock()
//	_, ok := s.inputRelabelers[relabelerKey{relabelerID, 0}]
//	s.irrwm.RUnlock()
//	return ok
//}

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

// reconfiguringInputRelabeler reconfiguring input relabelers for all shards.
func (h *Head) reconfigureInputRelabeler(
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) error {
	updated := make(map[relabelerKey]struct{})
	for _, cfgs := range inputRelabelerConfigs {
		key := relabelerKey{cfgs.GetName(), 0}
		statelessRelabeler, err := h.updateOrCreateStatelessRelabeler(key, cfgs.GetConfigs())
		if err != nil {
			return err
		}

		for ; key.shardID < numberOfShards; key.shardID++ {
			updated[key] = struct{}{}
			if inputRelabeler, ok := h.inputRelabelers[key]; ok {
				if numberOfShards != inputRelabeler.NumberOfShards() ||
					h.lsses[key.shardID].Generation() != inputRelabeler.Generation() {
					inputRelabeler.ResetTo(h.lsses[key.shardID].Generation(), numberOfShards)
				}
				continue
			}

			if h.inputRelabelers[key], err = cppbridge.NewInputPerShardRelabeler(
				statelessRelabeler,
				h.lsses[key.shardID].Generation(),
				numberOfShards,
				key.shardID,
			); err != nil {
				return err
			}
		}
	}

	for key := range h.inputRelabelers {
		if _, ok := updated[key]; !ok {
			// clear unnecessary
			h.memoryInUse.Delete(
				prometheus.Labels{
					"allocator": fmt.Sprintf("input_relabeler_%s", key.relabelerID),
					"id":        fmt.Sprintf("%d", key.shardID),
				},
			)
			delete(h.inputRelabelers, key)
		}
	}

	return nil
}

// updateOrCreateStatelessRelabeler check inputRelabeler(shardID == 0) for key
// and update configs for StatelessRelabeler, if not exist - create new.
func (h *Head) updateOrCreateStatelessRelabeler(
	key relabelerKey,
	rCfgs []*cppbridge.RelabelConfig,
) (*cppbridge.StatelessRelabeler, error) {
	inputRelabeler, ok := h.inputRelabelers[key]
	if !ok {
		statelessRelabeler, err := cppbridge.NewStatelessRelabeler(rCfgs)
		if err != nil {
			return nil, err
		}
		return statelessRelabeler, nil
	}

	sr := inputRelabeler.StatelessRelabeler()
	if sr.EqualConfigs(rCfgs) {
		return sr, nil
	}

	if err := sr.ResetTo(rCfgs); err != nil {
		return nil, err
	}

	return sr, nil
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
				err = h.inputRelabelers[relabelerKey{task.RelabelerID(), shardID}].InputRelabelingWithStalenans(
					task.Ctx(),
					h.lsses[shardID],
					task.MetricLimits(),
					task.SourceStateByShard(shardID),
					task.StaleNansTS(),
					task.ShardedData(),
					shardsInnerSeries,
					shardsRelabeledSeries,
				)

			} else {
				err = h.inputRelabelers[relabelerKey{task.RelabelerID(), shardID}].InputRelabeling(
					task.Ctx(),
					h.lsses[shardID],
					task.MetricLimits(),
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
					task.RelabelerID(),
					shardID,
				)
			}

			for sid, innerSeries := range shardsInnerSeries {
				task.AddResult(uint16(sid), innerSeries)
			}

		case task := <-h.stageAppendRelabelerSeries[shardID]:
			relabelerStateUpdate := cppbridge.NewRelabelerStateUpdate()
			innerSeries := cppbridge.NewInnerSeries()

			if err := h.inputRelabelers[relabelerKey{task.RelabelerID(), shardID}].AppendRelabelerSeries(
				task.Ctx(),
				h.lsses[shardID],
				relabelerStateUpdate,
				innerSeries,
				task.RelabeledSeries(),
			); err != nil {
				task.AddError(shardID, fmt.Errorf("failed input append relabeler series shard %d: %w", shardID, err))
				continue
			}

			h.stageUpdateRelabelerState[task.SourceShardID()] <- NewTaskUpdateRelabelerState(
				task.Ctx(),
				relabelerStateUpdate,
				task.RelabelerID(),
				shardID,
			)

			task.AddResult(shardID, innerSeries)
		case task := <-h.stageUpdateRelabelerState[shardID]:
			if err := h.inputRelabelers[relabelerKey{task.RelabelerID(), shardID}].UpdateRelabelerState(
				task.Ctx(),
				task.RelabelerStateUpdate(),
				task.RelabeledShardID(),
			); err != nil {
				//Errorf("failed input update relabeler state %d: %s", shardID, err)
				continue
			}
		case task := <-h.genericTaskCh[shardID]:
			inputRelabelers := make(map[string]*cppbridge.InputPerShardRelabeler)
			for key, inputRelabeler := range h.inputRelabelers {
				if key.shardID != shardID {
					continue
				}
				ir := inputRelabeler
				inputRelabelers[key.relabelerID] = ir
			}
			task.errs[shardID] = task.shardFn(&shard{
				id:              shardID,
				dataStorage:     h.dataStorages[shardID],
				lssWrapper:      &lssWrapper{lss: h.lsses[shardID]},
				inputRelabelers: inputRelabelers,
			})
			task.wg.Done()
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

type shard struct {
	id              uint16
	lssWrapper      *lssWrapper
	dataStorage     *DataStorage
	inputRelabelers map[string]*cppbridge.InputPerShardRelabeler
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

func (s *shard) InputRelabelers() map[string]relabeler.InputRelabeler {
	result := make(map[string]relabeler.InputRelabeler)
	for key, inputRelabeler := range s.inputRelabelers {
		ir := inputRelabeler
		result[key] = ir
	}
	return result
}
