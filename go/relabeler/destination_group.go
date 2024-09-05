package relabeler

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
)

// DestinationGroupUpdate config for update DestinationGroup.
type DestinationGroupUpdate struct {
	DestinationGroupConfig *DestinationGroupConfig
	DialersConfigs         []*DialersConfig
}

// DestinationGroupConfig - config for DestinationGroup.
type DestinationGroupConfig struct {
	Name           string                     `yaml:"name"`
	Dir            string                     `yaml:"dir"`
	ManagerKeeper  *ManagerKeeperConfig       `yaml:"manager_keeper"`
	Relabeling     []*cppbridge.RelabelConfig `yaml:"relabel_configs"`
	ExternalLabels []cppbridge.Label          `yaml:"external_labels"`
	NumberOfShards uint16                     `yaml:"mumber_of_shards"`
}

// NewDestinationGroupConfig init new DestinationGroupConfig.
func NewDestinationGroupConfig(
	destinationName string,
	dir string,
	rCfgs []*cppbridge.RelabelConfig,
	numberOfShards uint16,
) *DestinationGroupConfig {
	dgcfg := &DestinationGroupConfig{
		Name:           destinationName,
		Dir:            dir,
		Relabeling:     rCfgs,
		ManagerKeeper:  DefaultManagerKeeperConfig(),
		NumberOfShards: numberOfShards,
	}

	return dgcfg
}

// Equal check for complete coincidence of values.
func (c *DestinationGroupConfig) Equal(cfg *DestinationGroupConfig) bool {
	if c.Name != cfg.Name || c.Dir != cfg.Dir || c.NumberOfShards != cfg.NumberOfShards {
		return false
	}

	if len(c.ExternalLabels) != len(cfg.ExternalLabels) {
		return false
	}

	for i := range c.ExternalLabels {
		if c.ExternalLabels[i] != cfg.ExternalLabels[i] {
			return false
		}
	}

	if len(c.Relabeling) != len(cfg.Relabeling) {
		return false
	}

	for i := range c.Relabeling {
		if !c.Relabeling[i].Equal(cfg.Relabeling[i]) {
			return false
		}
	}

	return c.ManagerKeeper.Equal(cfg.ManagerKeeper)
}

// EncoderSelector func-selector for constuctor for ManagerEncoder.
type EncoderSelector func(isShrinkable bool) ManagerEncoderCtor

// DestinationGroup - group with manager keeper and output relabelers(count main shards).
type DestinationGroup struct {
	managerKeeper    *ManagerKeeper
	encoderSelector  EncoderSelector
	outputRelabelers []*cppbridge.OutputPerShardRelabeler
	cfg              DestinationGroupConfig
	// stat
	memoryInUse *prometheus.GaugeVec
}

// NewDestinationGroup - init new *DestinationGroup.
func NewDestinationGroup(
	ctx context.Context,
	dgCfg *DestinationGroupConfig,
	encoderSelector EncoderSelector,
	managerRefillCtor ManagerRefillCtor,
	mangerRefillSenderCtor MangerRefillSenderCtor,
	clock clockwork.Clock,
	dialers []Dialer,
	registerer prometheus.Registerer,
) (*DestinationGroup, error) {
	factory := util.NewUnconflictRegisterer(registerer)
	dg := &DestinationGroup{
		cfg:              *dgCfg,
		encoderSelector:  encoderSelector,
		outputRelabelers: make([]*cppbridge.OutputPerShardRelabeler, 0, dgCfg.NumberOfShards),
		// stat
		memoryInUse: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_distributor_cgo_memory_bytes",
				Help: "Current value memory in use in bytes.",
			},
			[]string{"allocator", "id"},
		),
	}
	statelessRelabeler, err := dg.updateOrCreateStatelessRelabeler(dgCfg.Relabeling)
	if err != nil {
		return nil, err
	}

	if err = dg.reshardingOutputRelabelers(statelessRelabeler, dgCfg, dgCfg.NumberOfShards); err != nil {
		return nil, err
	}

	dg.managerKeeper, err = NewManagerKeeper(
		ctx,
		dgCfg.ManagerKeeper,
		NewManager,
		dg.encoderSelector(isShrinkable(dgCfg)),
		managerRefillCtor,
		mangerRefillSenderCtor,
		clock,
		dialers,
		filepath.Join(dgCfg.Dir, dg.cfg.Name),
		dgCfg.Name,
		registerer,
	)
	if err != nil {
		return nil, err
	}

	return dg, nil
}

// AppendOpenHead - append metrics data to encode.
func (dg *DestinationGroup) AppendOpenHead(
	ctx context.Context,
	outputInnerSeries [][]*cppbridge.InnerSeries,
	outputRelabeledSeries []*cppbridge.RelabeledSeries,
	outputStateUpdates [][]*cppbridge.RelabelerStateUpdate,
) (bool, error) {
	return dg.managerKeeper.AppendOpenHead(ctx, outputInnerSeries, outputRelabeledSeries, outputStateUpdates)
}

// CacheAllocatedMemory - return size of allocated memory for cache map.
func (dg *DestinationGroup) CacheAllocatedMemory(shardID uint16) uint64 {
	return dg.outputRelabelers[shardID].CacheAllocatedMemory()
}

// EncodersLock - lock encoders on rw.
func (dg *DestinationGroup) EncodersLock() {
	dg.managerKeeper.EncodersLock()
}

// EncodersUnlock - unlock encoders on rw.
func (dg *DestinationGroup) EncodersUnlock() {
	dg.managerKeeper.EncodersUnlock()
}

// Equal check for complete coincidence of values.
func (dg *DestinationGroup) Equal(dgCfg *DestinationGroupConfig) bool {
	return dg.cfg.Equal(dgCfg)
}

// EqualDialers check dialers for config changes.
func (dg *DestinationGroup) EqualDialers(dCfgs []*DialersConfig) bool {
	if len(dg.managerKeeper.dialers) != len(dCfgs) {
		return false
	}

	for i := range dg.managerKeeper.dialers {
		if !dg.managerKeeper.dialers[i].Equal(dCfgs[i].DialerConfig) ||
			!dg.managerKeeper.dialers[i].ConnDialer().Equal(dCfgs[i].ConnDialerConfig) {
			return false
		}
	}

	return true
}

// Name of destination group.
func (dg *DestinationGroup) Name() string {
	return dg.cfg.Name
}

// ObserveCacheAllocatedMemory - observe cache allocated memory.
func (dg *DestinationGroup) ObserveCacheAllocatedMemory(shardID uint16) {
	dg.memoryInUse.With(
		prometheus.Labels{
			"allocator": fmt.Sprintf("output_relabeler_%s", dg.cfg.Name),
			"id":        fmt.Sprintf("%d", shardID)},
	).Set(float64(dg.CacheAllocatedMemory(shardID)))
}

// ObserveEncodersMemory - observe encoders memory.
func (dg *DestinationGroup) ObserveEncodersMemory() {
	dg.managerKeeper.ObserveEncodersMemory(func(id int, val float64) {
		dg.memoryInUse.With(
			prometheus.Labels{
				"allocator": fmt.Sprintf("encoder_%s", dg.cfg.Name),
				"id":        fmt.Sprintf("%d", id)},
		).Set(val)
	})
}

// OutputRelabeling - relabeling output series on shard.
func (dg *DestinationGroup) OutputRelabeling(
	ctx context.Context,
	inputLss *cppbridge.LabelSetStorage,
	data []*cppbridge.InnerSeries,
	encodersInnerSeries []*cppbridge.InnerSeries,
	relabeledSeries *cppbridge.RelabeledSeries,
	shardID uint16,
) error {
	return dg.outputRelabelers[shardID].OutputRelabeling(
		ctx,
		inputLss,
		data,
		encodersInnerSeries,
		relabeledSeries,
		dg.managerKeeper.Generation(),
	)
}

// OutputStateUpdates - make container for output state updates.
func (dg *DestinationGroup) OutputStateUpdates() [][]*cppbridge.RelabelerStateUpdate {
	// encodersStateUpdates[mainShardID[encoderShardID]]
	encodersStateUpdates := make([][]*cppbridge.RelabelerStateUpdate, len(dg.outputRelabelers))
	for i := range encodersStateUpdates {
		encodersStateUpdates[i] = make([]*cppbridge.RelabelerStateUpdate, 1<<dg.managerKeeper.ShardsNumberPower())
		for j := range encodersStateUpdates[i] {
			// set current DestinationGroup(lss) generation
			encodersStateUpdates[i][j] = cppbridge.NewRelabelerStateUpdateWithGeneration(dg.managerKeeper.Generation())
		}
	}

	return encodersStateUpdates
}

// ResetTo reset to changed attributes.
func (dg *DestinationGroup) ResetTo(
	dgCfg *DestinationGroupConfig,
	dialers []Dialer,
) error {
	statelessRelabeler, err := dg.updateOrCreateStatelessRelabeler(dgCfg.Relabeling)
	if err != nil {
		return err
	}

	if err = dg.reshardingOutputRelabelers(statelessRelabeler, dgCfg, dgCfg.NumberOfShards); err != nil {
		return err
	}

	if err := dg.managerKeeper.ResetTo(
		dialers,
		dgCfg.ManagerKeeper,
		dg.encoderSelector(isShrinkable(dgCfg)),
		dg.cfg.Dir,
	); err != nil {
		return err
	}
	dg.Rotate()
	dg.cfg = *dgCfg

	return nil
}

// Rotate - call rotate on DestinationGroup.
func (dg *DestinationGroup) Rotate() {
	done := make(chan struct{})
	prevGeneration := dg.managerKeeper.Generation()
	dg.managerKeeper.NotifyOnRotate(
		func(generation uint32) {
			if prevGeneration == generation {
				close(done)
				return
			}
			for _, outputRelabeler := range dg.outputRelabelers {
				outputRelabeler.ResetTo(dg.cfg.ExternalLabels, generation, dg.cfg.NumberOfShards)
			}
			close(done)
		},
	)
	<-done
}

// RotateLock - lock the manager keeper from rotation.
func (dg *DestinationGroup) RotateLock() {
	dg.managerKeeper.RotateLock()
}

// RotateUnlock - unlock the manager keeper from rotation.
func (dg *DestinationGroup) RotateUnlock() {
	dg.managerKeeper.RotateUnlock()
}

// ShardsNumberPower - return current shards number of power.
func (dg *DestinationGroup) ShardsNumberPower() uint8 {
	return dg.managerKeeper.ShardsNumberPower()
}

// UpdateRelabelerState - updating the state of the caches on shards OutputRelabeler.
func (dg *DestinationGroup) UpdateRelabelerState(
	ctx context.Context,
	shardID uint16,
	encodersStateUpdates []*cppbridge.RelabelerStateUpdate,
) error {
	errs := make([]error, len(encodersStateUpdates))

	for encoderShardID, encoderStateUpdates := range encodersStateUpdates {
		errs[encoderShardID] = dg.outputRelabelers[shardID].UpdateRelabelerState(
			ctx,
			encoderStateUpdates,
			uint16(encoderShardID),
		)
	}

	return errors.Join(errs...)
}

// Shutdown - stop ManagerKeeper then exits.
func (dg *DestinationGroup) Shutdown(ctx context.Context) error {
	var shardID uint16
	for ; shardID < dg.cfg.NumberOfShards; shardID++ {
		dg.memoryInUse.Delete(
			prometheus.Labels{
				"allocator": fmt.Sprintf("output_relabeler_%s", dg.cfg.Name),
				"id":        fmt.Sprintf("%d", shardID)},
		)

		dg.managerKeeper.ObserveEncodersMemory(func(id int, _ float64) {
			dg.memoryInUse.Delete(
				prometheus.Labels{
					"allocator": fmt.Sprintf("encoder_%s", dg.cfg.Name),
					"id":        fmt.Sprintf("%d", id)},
			)
		})
	}

	return dg.managerKeeper.Shutdown(ctx)
}

// updateOrCreateStatelessRelabeler check outputRelabelers(shardID == 0)
// and update configs for StatelessRelabeler, if not exist - create new.
func (dg *DestinationGroup) updateOrCreateStatelessRelabeler(
	rCfgs []*cppbridge.RelabelConfig,
) (*cppbridge.StatelessRelabeler, error) {
	if len(dg.outputRelabelers) == 0 {
		statelessRelabeler, err := cppbridge.NewStatelessRelabeler(rCfgs)
		if err != nil {
			return nil, err
		}
		return statelessRelabeler, nil
	}

	sr := dg.outputRelabelers[0].StatelessRelabeler()
	if sr.EqualConfigs(rCfgs) {
		return sr, nil
	}

	if err := sr.ResetTo(rCfgs); err != nil {
		return nil, err
	}

	return sr, nil
}

// reshardingOutputRelabelers change count of OutputRelabelers.
func (dg *DestinationGroup) reshardingOutputRelabelers(
	statelessRelabeler *cppbridge.StatelessRelabeler,
	dgCfg *DestinationGroupConfig,
	numberOfShards uint16,
) error {
	if len(dg.outputRelabelers) == int(numberOfShards) {
		return nil
	}

	if len(dg.outputRelabelers) > int(numberOfShards) {
		for shardID := range dg.outputRelabelers {
			if shardID >= int(numberOfShards) {
				// clear unnecessary
				dg.outputRelabelers[shardID] = nil
				dg.memoryInUse.Delete(
					prometheus.Labels{
						"allocator": fmt.Sprintf("output_relabeler_%s", dg.cfg.Name),
						"id":        fmt.Sprintf("%d", shardID)},
				)
				continue
			}
		}
		// cut
		dg.outputRelabelers = dg.outputRelabelers[:numberOfShards]
		return nil
	}

	// resize
	dg.outputRelabelers = append(
		dg.outputRelabelers,
		make([]*cppbridge.OutputPerShardRelabeler, int(numberOfShards)-len(dg.outputRelabelers))...,
	)
	var shardID uint16
	for ; shardID < numberOfShards; shardID++ {
		if dg.outputRelabelers[shardID] != nil {
			continue
		}

		outputRelabeler, err := cppbridge.NewOutputPerShardRelabeler(
			dgCfg.ExternalLabels,
			statelessRelabeler,
			0,
			dg.cfg.NumberOfShards,
			shardID,
		)
		if err != nil {
			return err
		}
		dg.outputRelabelers[shardID] = outputRelabeler
	}

	return nil
}

func isShrinkable(dgCfg *DestinationGroupConfig) bool {
	if len(dgCfg.ExternalLabels) != 0 {
		return false
	}

	for _, r := range dgCfg.Relabeling {
		if r.Action != cppbridge.Drop &&
			r.Action != cppbridge.Keep &&
			r.Action != cppbridge.DropEqual &&
			r.Action != cppbridge.KeepEqual {
			return false
		}
	}

	return true
}
