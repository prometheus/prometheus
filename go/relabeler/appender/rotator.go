package appender

import (
	"github.com/jonboulle/clockwork"
	"time"

	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
)

// DefaultRotateDuration - default block duration.
const (
	DefaultRotateDuration = 2 * time.Hour
	DefaultCommitTimeout  = time.Second * 5
)

// Rotatable is something that can be rotated.
type RotateCommitable interface {
	Rotate() error
	CommitToWal() error
}

type Timer interface {
	Chan() <-chan time.Time
	Reset()
	Stop()
}

// Rotator is a rotation trigger.
type RotateCommiter struct {
	rotateCommitable RotateCommitable
	rotateTimer      Timer
	commitTimer      Timer
	run              chan struct{}
	closer           *util.Closer
	rotateCounter    prometheus.Counter
}

// NewRotator - Rotator constructor.
func NewRotateCommiter(
	rotateCommitable RotateCommitable,
	rotateTimer Timer,
	commitTimer Timer,
	registerer prometheus.Registerer,
) *RotateCommiter {
	factory := util.NewUnconflictRegisterer(registerer)
	r := &RotateCommiter{
		rotateCommitable: rotateCommitable,
		rotateTimer:      rotateTimer,
		commitTimer:      commitTimer,
		run:              make(chan struct{}),
		closer:           util.NewCloser(),
		rotateCounter: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "prompp_rotator_rotate_count",
				Help: "Total counter of rotate rotatable object.",
			},
		),
	}
	go r.loop()

	return r
}

// Run - runs rotation loop.
func (r *RotateCommiter) Run() {
	close(r.run)
}

func (r *RotateCommiter) loop() {
	defer r.closer.Done()

	select {
	case <-r.run:
		r.rotateTimer.Reset()
		r.commitTimer.Reset()
	case <-r.closer.Signal():
		return
	}

	for {
		select {
		case <-r.closer.Signal():
			return
		case <-r.commitTimer.Chan():
			if err := r.rotateCommitable.CommitToWal(); err != nil {
				logger.Errorf("wal commit failed: %v", err)
			}
			r.commitTimer.Reset()
		case <-r.rotateTimer.Chan():
			logger.Debugf("start rotation")
			if err := r.rotateCommitable.Rotate(); err != nil {
				logger.Errorf("rotation failed: %v", err)
			}
			r.rotateCounter.Inc()

			r.rotateTimer.Reset()
			r.commitTimer.Reset()
		}
	}
}

// Close - io.Closer interface implementation.
func (r *RotateCommiter) Close() error {
	return r.closer.Close()
}

type ConstantIntervalTimer struct {
	timer    clockwork.Timer
	interval time.Duration
}

func NewConstantIntervalTimer(clock clockwork.Clock, interval time.Duration) *ConstantIntervalTimer {
	return &ConstantIntervalTimer{
		timer:    clock.NewTimer(interval),
		interval: interval,
	}
}

func (t *ConstantIntervalTimer) Chan() <-chan time.Time {
	return t.timer.Chan()
}

func (t *ConstantIntervalTimer) Reset() {
	t.timer.Reset(t.interval)
}

func (t *ConstantIntervalTimer) Stop() {
	t.timer.Stop()
}
