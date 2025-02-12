// Copyright OpCore

package receiver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/appender"
	"github.com/prometheus/prometheus/pp/go/relabeler/block"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/distributor"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	headmanager "github.com/prometheus/prometheus/pp/go/relabeler/head/manager"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/ready"
	rlogger "github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/prometheus/prometheus/pp/go/util"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	common_config "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"go.uber.org/atomic"
	"gopkg.in/yaml.v2"

	prom_config "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	op_config "github.com/prometheus/prometheus/op-pkg/config"
	"github.com/prometheus/prometheus/op-pkg/dialer"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	defaultShutdownTimeout        = 40 * time.Second
	defaultNumberOfShards         = 2
	defaultMaxSegmentSize  uint32 = 10000
)

type HeadConfig struct {
	inputRelabelerConfigs []*config.InputRelabelerConfig
	numberOfShards        uint16
}

type HeadConfigStorage struct {
	ptr atomic.Pointer[HeadConfig]
}

func (s *HeadConfigStorage) Load() *HeadConfig {
	return s.ptr.Load()
}

func (s *HeadConfigStorage) Get() ([]*config.InputRelabelerConfig, uint16) {
	cfg := s.ptr.Load()
	return cfg.inputRelabelerConfigs, cfg.numberOfShards
}

func (s *HeadConfigStorage) Store(headConfig *HeadConfig) {
	s.ptr.Store(headConfig)
}

type Receiver struct {
	ctx context.Context

	distributor         *distributor.Distributor
	appender            *appender.QueryableAppender
	storage             *appender.QueryableStorage
	rotator             *appender.RotateCommiter
	metricsWriteTrigger *appender.MetricsWriteTrigger

	headConfigStorage *HeadConfigStorage
	hashdexFactory    relabeler.HashdexFactory
	hashdexLimits     cppbridge.WALHashdexLimits
	haTracker         relabeler.HATracker
	clock             clockwork.Clock
	registerer        prometheus.Registerer
	logger            log.Logger
	workingDir        string
	clientID          string
	cgogc             *cppbridge.CGOGC
	shutdowner        *util.GracefulShutdowner
}

func (rr *Receiver) Appender(ctx context.Context) storage.Appender {
	return newPromAppender(ctx, rr, prom_config.TransparentRelabeler)
}

type RotationInfo struct {
	BlockDuration time.Duration
	Seed          uint64
}

type HeadActivator struct {
	catalog *catalog.Catalog
}

func (ha *HeadActivator) Activate(headID string) error {
	_, err := ha.catalog.SetStatus(headID, catalog.StatusActive)
	return err
}

func newHeadActivator(catalog *catalog.Catalog) *HeadActivator {
	return &HeadActivator{catalog: catalog}
}

func NewReceiver(
	ctx context.Context,
	logger log.Logger,
	registerer prometheus.Registerer,
	receiverCfg *op_config.RemoteWriteReceiverConfig,
	workingDir string,
	remoteWriteCfgs []*prom_config.OpRemoteWriteConfig,
	dataDir string,
	rotationInfo RotationInfo,
	headCatalog *catalog.Catalog,
	triggerNotifier *ReloadBlocksTriggerNotifier,
	readyNotifier ready.Notifier,
	commitInterval time.Duration,
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

	numberOfShards := receiverCfg.NumberOfShards
	if numberOfShards == 0 {
		numberOfShards = defaultNumberOfShards
	}

	destinationGroups, err := makeDestinationGroups(
		ctx,
		clock,
		registerer,
		workingDir,
		clientID,
		remoteWriteCfgs,
		numberOfShards,
	)
	if err != nil {
		level.Error(logger).Log("msg", "failed to init DestinationGroups", "err", err)
		return nil, err
	}

	headConfigStorage := &HeadConfigStorage{}

	headConfigStorage.Store(&HeadConfig{
		inputRelabelerConfigs: receiverCfg.Configs,
		numberOfShards:        numberOfShards,
	})

	querierMetrics := querier.NewMetrics(registerer)

	dataDir, err = filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}

	headManager, err := headmanager.New(
		dataDir,
		clock,
		headConfigStorage,
		headCatalog,
		defaultMaxSegmentSize,
		registerer,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create head manager: %w", err)
	}

	activeHead, rotatedHeads, err := headManager.Restore(rotationInfo.BlockDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to restore heads: %w", err)
	}
	readyNotifier.NotifyReady()
	queryableStorage := appender.NewQueryableStorageWithWriteNotifier(block.NewBlockWriter(dataDir, block.DefaultChunkSegmentSize, rotationInfo.BlockDuration, registerer), registerer, querierMetrics, triggerNotifier, rotatedHeads...)

	hd := appender.NewRotatableHead(activeHead, queryableStorage, headManager, newHeadActivator(headCatalog))

	var appenderHead relabeler.Head = hd
	if len(os.Getenv("OPCORE_ROTATION_HEAP_DEBUG")) > 0 {
		heapProfileWriter := util.NewHeapProfileWriter(filepath.Join(dataDir, "heap_profiles"))
		appenderHead = appender.NewHeapProfileWritableHead(appenderHead, heapProfileWriter)
	}

	dstrb := distributor.NewDistributor(*destinationGroups)
	app := appender.NewQueryableAppender(appenderHead, dstrb, querierMetrics)
	mwt := appender.NewMetricsWriteTrigger(appender.DefaultMetricWriteInterval, app, queryableStorage)

	r := &Receiver{
		ctx:               ctx,
		distributor:       dstrb,
		appender:          app,
		storage:           queryableStorage,
		headConfigStorage: headConfigStorage,
		rotator: appender.NewRotateCommiter(
			app,
			relabeler.NewRotateTimerWithSeed(clock, rotationInfo.BlockDuration, rotationInfo.Seed),
			appender.NewConstantIntervalTimer(clock, commitInterval),
			registerer,
		),

		metricsWriteTrigger: mwt,
		hashdexFactory:      cppbridge.HashdexFactory{},
		hashdexLimits:       cppbridge.DefaultWALHashdexLimits(),
		haTracker:           relabeler.NewHighAvailabilityTracker(ctx, registerer, clock),
		clock:               clock,
		registerer:          registerer,
		logger:              logger,
		workingDir:          workingDir,
		clientID:            clientID,
		cgogc:               cppbridge.NewCGOGC(registerer),
		shutdowner:          util.NewGracefulShutdowner(),
	}

	level.Info(logger).Log("msg", "created")

	return r, nil
}

// AppendProtobuf append Protobuf data to relabeling hashdex data.
func (rr *Receiver) AppendProtobuf(
	ctx context.Context,
	data relabeler.ProtobufData,
	relabelerID string,
	commitToWal bool,
) error {
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
	_, err = rr.appender.Append(ctx, incomingData, nil, relabelerID, commitToWal)
	return err
}

// AppendTimeSeries append TimeSeries data to relabeling hashdex data.
func (rr *Receiver) AppendTimeSeries(
	ctx context.Context,
	data relabeler.TimeSeriesData,
	state *cppbridge.State,
	relabelerID string,
	commitToWal bool,
) (cppbridge.RelabelerStats, error) {
	hx, err := rr.hashdexFactory.GoModel(data.TimeSeries(), rr.hashdexLimits)
	if err != nil {
		data.Destroy()
		return cppbridge.RelabelerStats{}, err
	}

	if rr.haTracker.IsDrop(hx.Cluster(), hx.Replica()) {
		data.Destroy()
		return cppbridge.RelabelerStats{}, nil
	}
	incomingData := &relabeler.IncomingData{Hashdex: hx, Data: data}
	return rr.appender.AppendWithStaleNans(
		ctx,
		incomingData,
		state,
		relabelerID,
		commitToWal,
	)
}

func (rr *Receiver) AppendTimeSeriesHashdex(
	ctx context.Context,
	hashdex cppbridge.ShardedData,
	state *cppbridge.State,
	relabelerID string,
	commitToWal bool,
) (cppbridge.RelabelerStats, error) {
	return rr.appender.AppendWithStaleNans(
		ctx,
		&relabeler.IncomingData{Hashdex: hashdex},
		state,
		relabelerID,
		commitToWal,
	)
}

// AppendHashdex append incoming Hashdex data to relabeling.
func (rr *Receiver) AppendHashdex(
	ctx context.Context,
	hashdex cppbridge.ShardedData,
	relabelerID string,
	commitToWal bool,
) error {
	if rr.haTracker.IsDrop(hashdex.Cluster(), hashdex.Replica()) {
		return nil
	}
	incomingData := &relabeler.IncomingData{Hashdex: hashdex}
	_, err := rr.appender.Append(ctx, incomingData, nil, relabelerID, commitToWal)
	return err
}

// ApplyConfig update config.
func (rr *Receiver) ApplyConfig(cfg *prom_config.Config) error {
	level.Info(rr.logger).Log("msg", "reconfiguration start")
	defer level.Info(rr.logger).Log("msg", "reconfiguration completed")

	rCfg, err := cfg.GetReceiverConfig()
	if err != nil {
		return err
	}

	numberOfShards := rCfg.NumberOfShards
	if numberOfShards == 0 {
		numberOfShards = defaultNumberOfShards
	}

	rr.headConfigStorage.Store(&HeadConfig{
		inputRelabelerConfigs: rCfg.Configs,
		numberOfShards:        numberOfShards,
	})

	err = rr.appender.Reconfigure(
		HeadConfigureFunc(func(head relabeler.Head) error {
			return head.Reconfigure(rCfg.Configs, numberOfShards)
		}),
		DistributorConfigureFunc(func(dstrb relabeler.Distributor) error {
			mxdgupds := new(sync.Mutex)
			dgupds, err := makeDestinationGroupUpdates(
				cfg.RemoteWriteConfigs,
				rr.workingDir,
				rr.clientID,
				numberOfShards,
			)
			if err != nil {
				level.Error(rr.logger).Log("msg", "failed to init destination group update", "err", err)
				return err
			}
			mxDelete := new(sync.Mutex)
			toDelete := []int{}

			dgs := dstrb.DestinationGroups()
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
				return err
			}
			// delete unused DestinationGroup
			dgs.RemoveByID(toDelete)

			// create new DestinationGroup
			for _, dgupd := range dgupds {
				dialers, err := makeDialers(rr.clock, rr.registerer, dgupd.DialersConfigs)
				if err != nil {
					level.Error(rr.logger).Log("msg", "failed to make new dialers", "err", err)
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
					return err
				}

				dgs.Add(dg)
			}
			dstrb.SetDestinationGroups(dgs)

			return nil
		}),
	)
	if err != nil {
		return err
	}

	return nil
}

// GetState create new state.
func (rr *Receiver) GetState() *cppbridge.State {
	return cppbridge.NewState(rr.headConfigStorage.Load().numberOfShards)
}

// RelabelerIDIsExist check on exist relabelerID.
func (rr *Receiver) RelabelerIDIsExist(relabelerID string) bool {
	cs := rr.headConfigStorage.Load()
	for _, cfg := range cs.inputRelabelerConfigs {
		if cfg.Name == relabelerID {
			return true
		}
	}

	return false
}

// Run main loop.
func (rr *Receiver) Run(_ context.Context) (err error) {
	defer rr.shutdowner.Done(err)
	rr.storage.Run()
	rr.rotator.Run()
	<-rr.shutdowner.Signal()
	return nil
}

func (rr *Receiver) HeadStatus(limit int) relabeler.HeadStatus {
	return rr.appender.HeadStatus(limit)
}

// Querier calls f() with the given parameters.
// Returns a querier.MultiQuerier combining of appenderQuerier and storageQuerier.
func (rr *Receiver) Querier(mint, maxt int64) (storage.Querier, error) {
	appenderQuerier, err := rr.appender.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}

	storageQuerier, err := rr.storage.Querier(mint, maxt)
	if err != nil {
		return nil, errors.Join(err, appenderQuerier.Close())
	}

	return querier.NewMultiQuerier([]storage.Querier{appenderQuerier, storageQuerier}, nil), nil
}

func (rr *Receiver) HeadQueryable() storage.Queryable {
	return rr.appender
}

// LowestSentTimestamp returns the lowest sent timestamp across all queues.
func (*Receiver) LowestSentTimestamp() int64 {
	return 0
}

// Shutdown safe shutdown Receiver.
func (rr *Receiver) Shutdown(ctx context.Context) error {
	cgogcErr := rr.cgogc.Shutdown(ctx)
	metricWriteErr := rr.metricsWriteTrigger.Close()
	rotatorErr := rr.rotator.Close()
	storageErr := rr.storage.Close()
	distributorErr := rr.distributor.Shutdown(ctx)
	appendErr := rr.appender.Close()
	err := rr.shutdowner.Shutdown(ctx)
	return errors.Join(cgogcErr, metricWriteErr, rotatorErr, storageErr, distributorErr, appendErr, err)
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
		if rwCfg.IsPrometheusProtocol() {
			continue
		}

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
		if rwCfg.IsPrometheusProtocol() {
			continue
		}

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
					extractAccessToken(sCfg.HTTPClientConfig.Authorization),
				),
				ConnDialerConfig: ccfg,
			},
		)
	}

	return dialersConfigs, nil
}

// extractAccessToken extract access token from Authorization config.
func extractAccessToken(authorization *common_config.Authorization) string {
	if authorization == nil {
		return ""
	}

	return string(authorization.Credentials)
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
	logger = log.With(logger, "op_caller", log.Caller(4))
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

	rlogger.Debugf = func(template string, args ...interface{}) {
		level.Debug(logger).Log("msg", fmt.Sprintf(template, args...))
	}
	rlogger.Infof = func(template string, args ...interface{}) {
		level.Info(logger).Log("msg", fmt.Sprintf(template, args...))
	}
	rlogger.Warnf = func(template string, args ...interface{}) {
		level.Warn(logger).Log("msg", fmt.Sprintf(template, args...))
	}
	rlogger.Errorf = func(template string, args ...interface{}) {
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

//
// NoopQuerier
//

type NoopQuerier struct{}

var _ storage.Querier = (*NoopQuerier)(nil)

func (*NoopQuerier) Select(_ context.Context, _ bool, _ *storage.SelectHints, _ ...*labels.Matcher) storage.SeriesSet {
	return &NoopSeriesSet{}
}

func (q *NoopQuerier) LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return []string{}, *annotations.New(), nil
}

func (q *NoopQuerier) LabelNames(ctx context.Context, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return []string{}, *annotations.New(), nil
}

func (*NoopQuerier) Close() error {
	return nil
}

//
// NoopSeriesSet
//

type NoopSeriesSet struct{}

func (*NoopSeriesSet) Next() bool {
	return false
}

func (*NoopSeriesSet) At() storage.Series {
	return nil
}

func (*NoopSeriesSet) Err() error {
	return nil
}

func (*NoopSeriesSet) Warnings() annotations.Annotations {
	return nil
}
