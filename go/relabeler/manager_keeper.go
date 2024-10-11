package relabeler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
)

// ManagerCtor - func-constructor for Manager.
type ManagerCtor func(
	ctx context.Context,
	dialers []Dialer,
	encoderCtor ManagerEncoderCtor,
	refillCtor ManagerRefillCtor,
	shardsNumberPower uint8,
	refillInterval time.Duration,
	workingDir string,
	name string,
	limits *Limits,
	rejectNotifyer RejectNotifyer,
	clock clockwork.Clock,
	registerer prometheus.Registerer,
) (*Manager, error)

// MangerRefillSenderCtor - func-constructor for MangerRefillSender.
type MangerRefillSenderCtor func(
	RefillSendManagerConfig,
	string,
	[]Dialer,
	clockwork.Clock,
	string,
	prometheus.Registerer,
) (ManagerRefillSender, error)

// ManagerRefillSender - interface for refill Send manager.
type ManagerRefillSender interface {
	Run(context.Context)
	ResetTo(workingDir string, dialers []Dialer, rsmCfg *RefillSendManagerConfig)
	Shutdown(ctx context.Context) error
}

// ManagerKeeperConfig - config for ManagerKeeper.
//
// Block - config for block.
// RefillSenderManager - config for refill sender manager.
// ShutdownTimeout - timeout to cancel context on shutdown, must be greater than the value UncommittedTimeWindow*2.
// UncommittedTimeWindow - interval for holding a segment in memory before
// sending it to refill (-2 always save to refill).
type ManagerKeeperConfig struct {
	ShutdownTimeout       time.Duration           `yaml:"shutdown_timeout" validate:"required"`
	UncommittedTimeWindow time.Duration           `yaml:"uncommitted_time_window" validate:"eq=-2|gte=20ms"`
	RefillSenderManager   RefillSendManagerConfig `yaml:"refill_sender"`
}

// DefaultManagerKeeperConfig - generate default ManagerKeeperConfig.
func DefaultManagerKeeperConfig() *ManagerKeeperConfig {
	return &ManagerKeeperConfig{
		ShutdownTimeout:       DefaultShutdownTimeout,
		UncommittedTimeWindow: DefaultUncommittedTimeWindow,
		RefillSenderManager:   DefaultRefillSendManagerConfig(),
	}
}

// Equal check for complete coincidence of values.
func (c *ManagerKeeperConfig) Equal(cfg *ManagerKeeperConfig) bool {
	return *c == *cfg
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *ManagerKeeperConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	c = DefaultManagerKeeperConfig()
	type plain ManagerKeeperConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return c.Validate()
}

// Validate - check the config for correct parameters.
func (c *ManagerKeeperConfig) Validate() error {
	//revive:disable-next-line:add-constant x2 value
	if c.ShutdownTimeout < c.UncommittedTimeWindow*2 {
		return fmt.Errorf("%w: %s < %s", ErrShutdownTimeout, c.ShutdownTimeout, c.UncommittedTimeWindow*2)
	}

	validate := validator.New()
	return validate.Struct(c)
}

// ManagerKeeper - a global object through which all writing and sending of data takes place.
type ManagerKeeper struct {
	stop chan struct{}
	done chan struct{}

	mangerRefillSender ManagerRefillSender
	clock              clockwork.Clock
	registerer         prometheus.Registerer

	managerCtor        ManagerCtor
	managerEncoderCtor ManagerEncoderCtor
	managerRefillCtor  ManagerRefillCtor

	manager        *Manager
	rotateNotifyer *RotateNotifyer
	currentState   *CurrentState
	autosharder    *Autosharder
	rwm            *sync.RWMutex
	cgogc          *cppbridge.CGOGC
	dialers        []Dialer
	workingDir     string
	name           string
	cfg            ManagerKeeperConfig
	generation     uint32

	// stat
	promiseDuration prometheus.Histogram
	inFlight        prometheus.Gauge
	sendDuration    *prometheus.HistogramVec
	appendDuration  *prometheus.HistogramVec
}

// NewManagerKeeper - init new DeliveryKeeper.
//
//revive:disable-next-line:function-length long but readable
//revive:disable-next-line:argument-limit  but readable and convenient
func NewManagerKeeper(
	ctx context.Context,
	cfg *ManagerKeeperConfig,
	managerCtor ManagerCtor,
	managerEncoderCtor ManagerEncoderCtor,
	managerRefillCtor ManagerRefillCtor,
	mangerRefillSenderCtor MangerRefillSenderCtor,
	clock clockwork.Clock,
	dialers []Dialer,
	workingDir string,
	name string,
	registerer prometheus.Registerer,
) (*ManagerKeeper, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, err
	}
	factory := util.NewUnconflictRegisterer(registerer)
	constLabels := prometheus.Labels{"name": name}
	cs := NewCurrentState(workingDir)
	if err = cs.Read(); err != nil {
		Warnf("fail read current state: %s", err)
	}
	dk := &ManagerKeeper{
		cfg:                *cfg,
		managerCtor:        managerCtor,
		managerEncoderCtor: managerEncoderCtor,
		managerRefillCtor:  managerRefillCtor,
		clock:              clock,
		rwm:                new(sync.RWMutex),
		dialers:            dialers,
		workingDir:         workingDir,
		name:               name,
		generation:         0,
		rotateNotifyer:     NewRotateNotifyer(clock, cs.Block()),
		currentState:       cs,
		stop:               make(chan struct{}),
		done:               make(chan struct{}),
		cgogc:              cppbridge.NewCGOGC(registerer),
		registerer:         registerer,
		sendDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_manager_keeper_send_duration_seconds",
				Help:        "Duration of sending data(s).",
				Buckets:     prometheus.ExponentialBucketsRange(0.1, 20, 10),
				ConstLabels: constLabels,
			},
			[]string{"state"},
		),
		appendDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_manager_keeper_append_duration_seconds",
				Help:        "Duration of sending data(s).",
				Buckets:     prometheus.ExponentialBucketsRange(0.1, 20, 10),
				ConstLabels: constLabels,
			},
			[]string{"state"},
		),
		promiseDuration: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_manager_keeper_promise_duration_seconds",
				Help:        "Duration of promise await.",
				Buckets:     prometheus.ExponentialBucketsRange(0.1, 20, 10),
				ConstLabels: constLabels,
			},
		),
		inFlight: factory.NewGauge(
			prometheus.GaugeOpts{
				Name:        "prompp_delivery_manager_keeper_in_flight",
				Help:        "The number of requests being processed.",
				ConstLabels: constLabels,
			},
		),
	}

	snp := cs.ShardsNumberPower()
	dk.manager, err = dk.managerCtor(
		ctx,
		dk.dialers,
		dk.managerEncoderCtor,
		dk.managerRefillCtor,
		snp,
		cfg.UncommittedTimeWindow,
		dk.workingDir,
		dk.name,
		cs.Limits(),
		dk.rotateNotifyer,
		dk.clock,
		dk.registerer,
	)
	if err != nil {
		return nil, err
	}
	dk.manager.Open(ctx)

	dk.mangerRefillSender, err = mangerRefillSenderCtor(
		cfg.RefillSenderManager,
		dk.workingDir,
		dialers,
		clock,
		dk.name,
		dk.registerer,
	)
	if err != nil {
		return nil, err
	}

	dk.autosharder = NewAutosharder(clock, cs.Block(), snp)
	go dk.mangerRefillSender.Run(ctx)
	go dk.rotateLoop(ctx)

	return dk, nil
}

// AppendOpenHead - add to open head incoming data.
func (dk *ManagerKeeper) AppendOpenHead(
	ctx context.Context,
	outputInnerSeries [][]*cppbridge.InnerSeries,
	outputRelabeledSeries []*cppbridge.RelabeledSeries,
	outputStateUpdates [][]*cppbridge.RelabelerStateUpdate,
) (bool, error) {
	dk.inFlight.Inc()
	defer dk.inFlight.Dec()
	start := time.Now()

	select {
	case <-dk.stop:
		return false, ErrShutdown
	default:
	}

	_, err := dk.manager.AppendOpenHead(ctx, outputInnerSeries, outputRelabeledSeries, outputStateUpdates)
	if dk.manager.MaxBlockBytes() >= dk.currentState.Block().DesiredBlockSizeBytes {
		dk.rotateNotifyer.NotifyOnLimits()
	}
	if err != nil {
		if errors.Is(err, ErrHADropped) {
			dk.appendDuration.With(prometheus.Labels{"state": "ha_dropped"}).Observe(time.Since(start).Seconds())
			return true, nil
		}
		dk.appendDuration.With(prometheus.Labels{"state": "error"}).Observe(time.Since(start).Seconds())
		return false, err
	}
	dk.appendDuration.With(prometheus.Labels{"state": "success"}).Observe(time.Since(start).Seconds())
	return true, nil
}

// Generation return generation of manager.
func (dk *ManagerKeeper) Generation() uint32 {
	return dk.generation
}

// ObserveEncodersMemory - observe encoders memory.
func (dk *ManagerKeeper) ObserveEncodersMemory(set func(id int, val float64)) {
	dk.rwm.RLock()
	dk.manager.ObserveEncodersMemory(set)
	dk.rwm.RUnlock()
}

// ShardsNumberPower - return current shards number of power.
func (dk *ManagerKeeper) ShardsNumberPower() uint8 {
	return dk.currentState.ShardsNumberPower()
}

func (dk *ManagerKeeper) Dialers() []Dialer {
	return dk.dialers
}

// rotateLoop - loop for rotate Manage.
func (dk *ManagerKeeper) rotateLoop(ctx context.Context) {
	defer dk.rotateNotifyer.DrainedTriggers(dk.generation)
	defer close(dk.done)

	for {
		select {
		case <-dk.rotateNotifyer.TriggerOnReject():
			if err := dk.rotate(ctx); err != nil {
				Errorf("failed rotate block on reject: %s", err)
			}
			dk.rotateNotifyer.DrainedTriggers(dk.generation)
		case callBack := <-dk.rotateNotifyer.TriggerOnRotate():
			if err := dk.rotate(ctx); err != nil {
				Errorf("failed rotate block: %s", err)
			}
			callBack(dk.generation)
			dk.rotateNotifyer.DrainedTriggers(dk.generation)
		case <-dk.stop:
			return
		case <-ctx.Done():
			if !errors.Is(context.Cause(ctx), ErrShutdown) {
				Errorf("rotate loop context canceled: %s", context.Cause(ctx))
			}
			return
		}
	}
}

// ResetTo reset to changed attributes.
func (dk *ManagerKeeper) ResetTo(
	dialers []Dialer,
	cfg *ManagerKeeperConfig,
	managerEncoderCtor ManagerEncoderCtor,
	workingDir string,
) error {
	dk.rwm.Lock()

	dk.managerEncoderCtor = managerEncoderCtor

	dk.mangerRefillSender.ResetTo(workingDir, dialers, &cfg.RefillSenderManager)

	if len(dialers) != 0 {
		dk.dialers = dialers
	}

	if dk.workingDir != workingDir {
		dk.workingDir = workingDir
	}

	if dk.cfg.Equal(cfg) {
		dk.rwm.Unlock()
		return nil
	}

	err := cfg.Validate()
	if err == nil {
		dk.cfg = *cfg
	}

	dk.rwm.Unlock()
	return err
}

func (dk *ManagerKeeper) rotate(ctx context.Context) error {
	dk.rwm.Lock()
	prevManager := dk.manager
	if err := prevManager.Close(); err != nil {
		dk.rwm.Unlock()
		return fmt.Errorf("fail close manager: %w", err)
	}
	snp := dk.autosharder.ShardsNumberPower(prevManager.MaxBlockBytes())
	limits := prevManager.Limits()
	newManager, err := dk.managerCtor(
		ctx,
		dk.dialers,
		dk.managerEncoderCtor,
		dk.managerRefillCtor,
		snp,
		dk.cfg.UncommittedTimeWindow,
		dk.workingDir,
		dk.name,
		limits,
		dk.rotateNotifyer,
		dk.clock,
		dk.registerer,
	)
	if err != nil {
		dk.rwm.Unlock()
		return fmt.Errorf("fail create manager: %w", err)
	}
	dk.autosharder.Reset(dk.currentState.Block())
	if err = dk.currentState.Write(snp, limits); err != nil {
		Errorf("fail write current state: %s", err)
	}
	dk.manager = newManager
	dk.generation++
	dk.rwm.Unlock()
	shutdownCtx, cancel := context.WithTimeout(ctx, dk.cfg.ShutdownTimeout)
	if err := prevManager.Shutdown(shutdownCtx); err != nil {
		Errorf("fail shutdown manager: %s", err)
	}
	cancel()
	dk.manager.Open(ctx)
	return nil
}

// RotateLock - lock the manager keeper from rotation.
func (dk *ManagerKeeper) RotateLock() {
	dk.rwm.RLock()
}

// RotateUnlock - unlock the manager keeper from rotation.
func (dk *ManagerKeeper) RotateUnlock() {
	dk.rwm.RUnlock()
}

// EncodersLock - lock encoders on rw.
func (dk *ManagerKeeper) EncodersLock() {
	dk.manager.EncodersLock()
}

// EncodersUnlock - unlock encoders on rw.
func (dk *ManagerKeeper) EncodersUnlock() {
	dk.manager.EncodersUnlock()
}

// NotifyOnRotate - notify rotate on commands from outside.
func (dk *ManagerKeeper) NotifyOnRotate(callBack func(generation uint32)) {
	dk.rotateNotifyer.NotifyOnRotate(callBack)
}

// Shutdown - stop ticker and waits until Manager end to work and then exits.
func (dk *ManagerKeeper) Shutdown(ctx context.Context) error {
	close(dk.stop)
	<-dk.done
	dk.rotateNotifyer.DrainedTriggers(dk.generation)

	var errs error
	dk.rwm.RLock()
	errs = errors.Join(errs, dk.manager.Close(), dk.manager.Shutdown(ctx))
	dk.rwm.RUnlock()

	return errors.Join(errs, dk.mangerRefillSender.Shutdown(ctx), dk.cgogc.Shutdown(ctx))
}

var emptyCallBack = func(_ uint32) {}

// RotateNotifyer - notifyer for rotate for the event:
//   - limit reached incoming data;
//   - rotate on commands from outside;
//   - to receive reject with the delay time;
type RotateNotifyer struct {
	rotateTrigger    chan func(generation uint32)
	clock            clockwork.Clock
	timer            clockwork.Timer
	mx               *sync.Mutex
	delayAfterNotify time.Duration
}

// NewRotateNotifyer - init new *RotateNotifyer.
func NewRotateNotifyer(clock clockwork.Clock, cfg BlockLimits) *RotateNotifyer {
	rn := &RotateNotifyer{
		rotateTrigger:    make(chan func(generation uint32), 1),
		clock:            clock,
		delayAfterNotify: cfg.DelayAfterNotify,
		mx:               new(sync.Mutex),
		timer:            clock.NewTimer(0),
	}

	// drain init timer
	if !rn.timer.Stop() {
		select {
		case <-rn.timer.Chan():
		default:
		}
	}

	return rn
}

// NotifyOnLimits - notify on limit reached incoming data.
func (rn *RotateNotifyer) NotifyOnLimits() {
	select {
	case rn.rotateTrigger <- emptyCallBack:
	default:
	}
}

// NotifyOnReject - reset the timer for the delay time if the time does not exceed the duration Block.
func (rn *RotateNotifyer) NotifyOnReject() {
	rn.mx.Lock()
	defer rn.mx.Unlock()
	rn.timer.Reset(rn.delayAfterNotify)
}

// NotifyOnRotate - notify rotate on commands from outside.
func (rn *RotateNotifyer) NotifyOnRotate(callBack func(generation uint32)) {
	select {
	case rn.rotateTrigger <- callBack:
	default:
	}
}

// TriggerOnReject - return chan with ticker time.
func (rn *RotateNotifyer) TriggerOnReject() <-chan time.Time {
	return rn.timer.Chan()
}

// TriggerOnRotate - return chan notify rotate on commands from outside or on limit reached incoming data.
func (rn *RotateNotifyer) TriggerOnRotate() <-chan func(generation uint32) {
	return rn.rotateTrigger
}

// DrainedTriggers - drained trigger channels.
func (rn *RotateNotifyer) DrainedTriggers(generation uint32) {
	select {
	case callBack := <-rn.rotateTrigger:
		callBack(generation)
	default:
	}

	rn.mx.Lock()
	if !rn.timer.Stop() {
		select {
		case <-rn.timer.Chan():
		default:
		}
	}
	rn.mx.Unlock()
}
