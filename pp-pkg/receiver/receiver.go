// Copyright OpCore

package receiver

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/client_golang/prometheus"
	common_config "github.com/prometheus/common/config"
	"gopkg.in/yaml.v2"

	prom_config "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/relabel"
	op_config "github.com/prometheus/prometheus/op-pkg/config"
	"github.com/prometheus/prometheus/op-pkg/dialer"
)

const defaultShutdownTimeout = 40 * time.Second

type Receiver struct {
	ctx              context.Context
	managerRelabeler *relabeler.ManagerRelabeler
	hashdexFactory   relabeler.HashdexFactory
	hashdexLimits    cppbridge.WALHashdexLimits
	haTracker        relabeler.HATracker
	clock            clockwork.Clock
	registerer       prometheus.Registerer
	logger           log.Logger
	workingDir       string
	clientID         string
}

func NewReceiver(
	ctx context.Context,
	logger log.Logger,
	registerer prometheus.Registerer,
	receiverCfg *op_config.RemoteWriteReceiverConfig,
	workingDir string,
	remoteWriteCfgs []*prom_config.OpRemoteWriteConfig,
) (*Receiver, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	clientID, err := readClientID(logger, workingDir)
	if err != nil {
		level.Error(logger).Log("msg", "failed read client id", "err", err)
		return nil, err
	}

	initLogHandler(logger)
	clock := clockwork.NewRealClock()

	destinationGroups, err := makeDestinationGroups(
		ctx,
		clock,
		registerer,
		workingDir,
		clientID,
		remoteWriteCfgs,
		receiverCfg.NumberOfShards,
	)
	if err != nil {
		level.Error(logger).Log("msg", "failed to init DestinationGroups", "err", err)
		return nil, err
	}

	mr, err := relabeler.NewManagerRelabeler(
		clock,
		registerer,
		receiverCfg.Configs,
		destinationGroups,
		receiverCfg.NumberOfShards,
	)
	if err != nil {
		return nil, err
	}

	level.Info(logger).Log("msg", "created")

	return &Receiver{
		managerRelabeler: mr,
		hashdexFactory:   cppbridge.HashdexFactory{},
		hashdexLimits:    cppbridge.DefaultWALHashdexLimits(),
		haTracker:        relabeler.NewHighAvailabilityTracker(ctx, registerer, clock),
		clock:            clock,
		registerer:       registerer,
		logger:           logger,
		workingDir:       workingDir,
		clientID:         clientID,
	}, nil
}

// AppendProtobuf append Protobuf data to relabeling hashdex data.
func (rr *Receiver) AppendProtobuf(ctx context.Context, data relabeler.ProtobufData, relabelerID string) error {
	hx, err := rr.hashdexFactory.Protobuf(data.Bytes(), rr.hashdexLimits)
	if err != nil {
		data.Destroy()
		return err
	}

	if rr.haTracker.IsDrop(hx.Cluster(), hx.Replica()) {
		data.Destroy()
		return nil
	}
	incomingData := &relabeler.IncomingData{Hashdex: hx, Data: data}
	return rr.managerRelabeler.Append(ctx, incomingData, nil, relabelerID)
}

// AppendTimeSeries append TimeSeries data to relabeling hashdex data.
func (rr *Receiver) AppendTimeSeries(
	ctx context.Context,
	data relabeler.TimeSeriesData,
	metricLimits *cppbridge.MetricLimits,
	sourceStates *relabeler.SourceStates,
	staleNansTS int64,
	relabelerID string,
) error {
	hx, err := rr.hashdexFactory.GoModel(data.TimeSeries(), rr.hashdexLimits)
	if err != nil {
		data.Destroy()
		return err
	}

	if rr.haTracker.IsDrop(hx.Cluster(), hx.Replica()) {
		data.Destroy()
		return nil
	}
	incomingData := &relabeler.IncomingData{Hashdex: hx, Data: data}
	return rr.managerRelabeler.AppendWithStaleNans(
		ctx,
		incomingData,
		metricLimits,
		sourceStates,
		staleNansTS,
		relabelerID,
	)
}

// AppendHashdex append incoming Hashdex data to relabeling.
func (rr *Receiver) AppendHashdex(ctx context.Context, hashdex cppbridge.ShardedData, relabelerID string) error {
	if rr.haTracker.IsDrop(hashdex.Cluster(), hashdex.Replica()) {
		return nil
	}
	incomingData := &relabeler.IncomingData{Hashdex: hashdex}
	return rr.managerRelabeler.Append(ctx, incomingData, nil, relabelerID)
}

// ApplyConfig update config.
func (rr *Receiver) ApplyConfig(cfg *prom_config.Config) error {
	rCfg, err := cfg.GetReceiverConfig()
	if err != nil {
		return err
	}

	mxdgupds := new(sync.Mutex)
	dgupds, err := makeDestinationGroupUpdates(
		cfg.RemoteWriteConfigs,
		rr.workingDir,
		rr.clientID,
		rCfg.NumberOfShards,
	)
	if err != nil {
		level.Error(rr.logger).Log("msg", "failed to init destination group update", "err", err)
		return err
	}
	mxDelete := new(sync.Mutex)
	toDelete := []int{}

	rr.managerRelabeler.Lock()

	dgs := rr.managerRelabeler.DestinationGroups()
	if err = dgs.RangeGo(func(destinationGroupID int, dg *relabeler.DestinationGroup) error {
		var rangeErr error
		dgu, ok := dgupds[dg.Name()]
		if !ok {
			mxDelete.Lock()
			toDelete = append(toDelete, destinationGroupID)
			mxDelete.Unlock()
			ctxShutdown, cancel := context.WithTimeout(rr.ctx, defaultShutdownTimeout)
			if rangeErr = dg.Shutdown(ctxShutdown); err != nil {
				level.Error(rr.logger).Log("msg", "failed shutdown DestinationGroup", "err", rangeErr)
			}
			cancel()
			return nil
		}

		if !dg.Equal(dgu.DestinationGroupConfig) ||
			!dg.EqualDialers(dgu.DialersConfigs) {
			var dialers []relabeler.Dialer
			if !dg.EqualDialers(dgu.DialersConfigs) {
				dialers, rangeErr = makeDialers(rr.clock, rr.registerer, dgu.DialersConfigs)
				if rangeErr != nil {
					return rangeErr
				}
			}

			if rangeErr = dg.ResetTo(dgu.DestinationGroupConfig, dialers); err != nil {
				return rangeErr
			}
		}
		mxdgupds.Lock()
		delete(dgupds, dg.Name())
		mxdgupds.Unlock()
		return nil
	}); err != nil {
		level.Error(rr.logger).Log("msg", "failed to apply config DestinationGroups", "err", err)
		rr.managerRelabeler.Unlock()
		return err
	}
	// delete unused DestinationGroup
	dgs.RemoveByID(toDelete)

	// create new DestinationGroup
	for _, dgupd := range dgupds {
		dialers, err := makeDialers(rr.clock, rr.registerer, dgupd.DialersConfigs)
		if err != nil {
			level.Error(rr.logger).Log("msg", "failed to make new dialers", "err", err)
			rr.managerRelabeler.Unlock()
			return err
		}

		dg, err := relabeler.NewDestinationGroup(
			rr.ctx,
			dgupd.DestinationGroupConfig,
			encoderSelector,
			refillCtor,
			refillSenderCtor,
			rr.clock,
			dialers,
			rr.registerer,
		)
		if err != nil {
			level.Error(rr.logger).Log("msg", "failed to init DestinationGroup", "err", err)
			rr.managerRelabeler.Unlock()
			return err
		}

		dgs.Add(dg)
	}

	// apply cfg for relabelers
	if err := rr.managerRelabeler.ApplyConfig(
		rCfg.Configs,
		rCfg.NumberOfShards,
	); err != nil {
		level.Error(rr.logger).Log("msg", "failed to apply config manager relabeler", "err", err)
		return err
	}

	rr.managerRelabeler.Unlock()

	return nil
}

// RelabelerIDIsExist check on exist relabelerID.
func (rr *Receiver) RelabelerIDIsExist(relabelerID string) bool {
	return rr.managerRelabeler.RelabelerIDIsExist(relabelerID)
}

// Run main loop.
func (rr *Receiver) Run(ctx context.Context) {
	rr.managerRelabeler.Run(ctx)
}

// Shutdown safe shutdown Receiver.
func (rr *Receiver) Shutdown() error {
	ctxShutdown, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
	err := rr.managerRelabeler.Shutdown(ctxShutdown)
	cancel()
	level.Info(rr.logger).Log("msg", "stopped")
	return err
}

// makeDestinationGroups create DestinationGroups from configs.
func makeDestinationGroups(
	ctx context.Context,
	clock clockwork.Clock,
	registerer prometheus.Registerer,
	workingDir, clientID string,
	rwCfgs []*prom_config.OpRemoteWriteConfig,
	numberOfShards uint16,
) (*relabeler.DestinationGroups, error) {
	dgs := make(relabeler.DestinationGroups, 0, len(rwCfgs))

	for _, rwCfg := range rwCfgs {
		dgCfg, err := convertingDestinationGroupConfig(rwCfg, workingDir, numberOfShards)
		if err != nil {
			return nil, err
		}

		dialersConfigs, err := convertingConfigDialers(clientID, rwCfg.Destinations)
		if err != nil {
			return nil, err
		}
		dialers, err := makeDialers(clock, registerer, dialersConfigs)
		if err != nil {
			return nil, err
		}

		dg, err := relabeler.NewDestinationGroup(
			ctx,
			dgCfg,
			encoderSelector,
			refillCtor,
			refillSenderCtor,
			clock,
			dialers,
			registerer,
		)
		if err != nil {
			return nil, err
		}

		dgs = append(dgs, dg)
	}

	return &dgs, nil
}

// makeDestinationGroupUpdates create update for DestinationGroups.
func makeDestinationGroupUpdates(
	rwCfgs []*prom_config.OpRemoteWriteConfig,
	workingDir, clientID string,
	numberOfShards uint16,
) (map[string]*relabeler.DestinationGroupUpdate, error) {
	dgus := make(map[string]*relabeler.DestinationGroupUpdate, len(rwCfgs))

	for _, rwCfg := range rwCfgs {
		dgCfg, err := convertingDestinationGroupConfig(rwCfg, workingDir, numberOfShards)
		if err != nil {
			return nil, err
		}

		dialersConfigs, err := convertingConfigDialers(clientID, rwCfg.Destinations)
		if err != nil {
			return nil, err
		}

		dgus[rwCfg.Name] = &relabeler.DestinationGroupUpdate{
			DestinationGroupConfig: dgCfg,
			DialersConfigs:         dialersConfigs,
		}
	}

	return dgus, nil
}

// convertingDestinationGroupConfig converting incoming config to internal DestinationGroupConfig.
func convertingDestinationGroupConfig(
	rwCfg *prom_config.OpRemoteWriteConfig,
	workingDir string,
	numberOfShards uint16,
) (*relabeler.DestinationGroupConfig, error) {
	rCfgs, err := convertingRelabelersConfig(rwCfg.WriteRelabelConfigs)
	if err != nil {
		return nil, err
	}

	dgcfg := relabeler.NewDestinationGroupConfig(
		rwCfg.Name,
		workingDir,
		rCfgs,
		numberOfShards,
	)

	return dgcfg, nil
}

// convertingRelabelersConfig converting incoming relabel config to internal relabel config.
func convertingRelabelersConfig(rCfgs []*relabel.Config) ([]*cppbridge.RelabelConfig, error) {
	var crCfgs []*cppbridge.RelabelConfig
	raw, err := yaml.Marshal(rCfgs)
	if err != nil {
		return nil, err
	}

	if err = yaml.Unmarshal(raw, &crCfgs); err != nil {
		return nil, err
	}

	return crCfgs, nil
}

// convertingConfigDialers converting and make internal dialer configs.
func convertingConfigDialers(
	clientID string,
	sCfgs []*prom_config.OpDestinationConfig,
) ([]*relabeler.DialersConfig, error) {
	dialersConfigs := make([]*relabeler.DialersConfig, 0, len(sCfgs))
	for _, sCfg := range sCfgs {
		tlsCfg, err := common_config.NewTLSConfig(&sCfg.HTTPClientConfig.TLSConfig)
		if err != nil {
			return nil, err
		}
		ccfg := dialer.NewCommonConfig(
			sCfg.URL.URL,
			tlsCfg,
			sCfg.Name,
		)
		dialersConfigs = append(
			dialersConfigs,
			&relabeler.DialersConfig{
				DialerConfig: relabeler.NewDialerConfig(
					sCfg.URL.URL,
					clientID,
					string(sCfg.HTTPClientConfig.Authorization.Credentials),
				),
				ConnDialerConfig: ccfg,
			},
		)
	}

	return dialersConfigs, nil
}

// makeDialers create dialers from main config according to the specified parameters.
func makeDialers(
	clock clockwork.Clock,
	registerer prometheus.Registerer,
	dialersConfig []*relabeler.DialersConfig,
) ([]relabeler.Dialer, error) {
	dialers := make([]relabeler.Dialer, 0, len(dialersConfig))
	for i := range dialersConfig {
		ccfg, ok := dialersConfig[i].ConnDialerConfig.(*dialer.CommonConfig)
		if !ok {
			return nil, fmt.Errorf("invalid CommonConfig: %v", dialersConfig[i].ConnDialerConfig)
		}

		d := dialer.DefaultDialer(ccfg, registerer)

		tcpDialer := relabeler.NewWebSocketDialer(
			d,
			dialersConfig[i].DialerConfig,
			clock,
			registerer,
		)
		dialers = append(dialers, tcpDialer)
	}

	return dialers, nil
}

// encoderSelector selector for constructors for encoders.
func encoderSelector(isShrinkable bool) relabeler.ManagerEncoderCtor {
	if isShrinkable {
		return func(shardID uint16, shardsNumberPower uint8) relabeler.ManagerEncoder {
			return cppbridge.NewWALEncoderLightweight(shardID, shardsNumberPower)
		}
	}

	return func(shardID uint16, shardsNumberPower uint8) relabeler.ManagerEncoder {
		return cppbridge.NewWALEncoder(shardID, shardsNumberPower)
	}
}

// refillCtor default contructor for refill.
func refillCtor(
	workinDir string,
	blockID uuid.UUID,
	destinations []string,
	shardsNumberPower uint8,
	segmentEncodingVersion uint8,
	alwaysToRefill bool,
	name string,
	registerer prometheus.Registerer,
) (relabeler.ManagerRefill, error) {
	return relabeler.NewRefill(
		workinDir,
		shardsNumberPower,
		segmentEncodingVersion,
		blockID,
		alwaysToRefill,
		name,
		registerer,
		destinations...,
	)
}

// refillSenderCtor default contructor for manager sender.
func refillSenderCtor(
	rsmCfg relabeler.RefillSendManagerConfig,
	workingDir string,
	dialers []relabeler.Dialer,
	clock clockwork.Clock,
	name string,
	registerer prometheus.Registerer,
) (relabeler.ManagerRefillSender, error) {
	return relabeler.NewRefillSendManager(rsmCfg, workingDir, dialers, clock, name, registerer)
}

// initLogHandler init log handler for ManagerKeeper.
func initLogHandler(logger log.Logger) {
	relabeler.Debugf = func(template string, args ...interface{}) {
		level.Debug(logger).Log("msg", fmt.Sprintf(template, args...))
	}
	relabeler.Infof = func(template string, args ...interface{}) {
		level.Info(logger).Log("msg", fmt.Sprintf(template, args...))
	}
	relabeler.Warnf = func(template string, args ...interface{}) {
		level.Warn(logger).Log("msg", fmt.Sprintf(template, args...))
	}
	relabeler.Errorf = func(template string, args ...interface{}) {
		level.Error(logger).Log("msg", fmt.Sprintf(template, args...))
	}
}

// readClientID read ClientID.
func readClientID(logger log.Logger, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", filepath.Dir(dir), err)
	}
	clientIDPath := path.Join(dir, "client_id.uuid")
	// Try reading UUID from the file. If not present, generate new one and write to file
	data, err := os.ReadFile(clientIDPath)
	switch {
	case os.IsNotExist(err):
		proxyUUID := uuid.NewString()
		//revive:disable-next-line:add-constant file permissions simple readable as octa-number
		if err = os.WriteFile(clientIDPath, []byte(proxyUUID), 0o644); err != nil { //#nosec G306
			return "", fmt.Errorf("failed to write proxy id: %w", err)
		}

		level.Info(logger).Log("msg", "create new client id")
		return proxyUUID, nil

	case err == nil:
		//revive:disable-next-line:add-constant uuid len
		if len(data) < 36 {
			return "", fmt.Errorf("short client id: %d", len(data))
		}

		return string(data[:36]), nil

	default:
		return "", fmt.Errorf("failed to read client id: %w", err)
	}
}
