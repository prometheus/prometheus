package appender

import (
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
)

// DefaultRotateDuration - default block duration.
const DefaultRotateDuration = 2 * time.Hour

// Rotatable is something that can be rotated.
type Rotatable interface {
	Rotate() error
}

// Rotator is a rotation trigger.
type Rotator struct {
	rotatable     Rotatable
	rotateTimer   *relabeler.RotateTimer
	run           chan struct{}
	closer        *util.Closer
	rotateCounter prometheus.Counter
}

// NewRotator - Rotator constructor.
func NewRotator(
	rotatable Rotatable,
	clock clockwork.Clock,
	rotateDuration time.Duration,
	registerer prometheus.Registerer,
) *Rotator {
	factory := util.NewUnconflictRegisterer(registerer)
	r := &Rotator{
		rotatable:   rotatable,
		rotateTimer: relabeler.NewRotateTimer(clock, rotateDuration),
		run:         make(chan struct{}),
		closer:      util.NewCloser(),
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
func (r *Rotator) Run() {
	close(r.run)
}

func (r *Rotator) loop() {
	defer r.closer.Done()
	defer r.rotateTimer.Stop()

	select {
	case <-r.run:
		r.rotateTimer.Reset()
	case <-r.closer.Signal():
		return
	}

	for {
		select {
		case <-r.closer.Signal():
			return

		case <-r.rotateTimer.Chan():
			logger.Debugf("start rotation")
			if err := r.rotatable.Rotate(); err != nil {
				logger.Errorf("rotation failed: %s", err.Error())
			}
			r.rotateCounter.Inc()

			r.rotateTimer.Reset()
		}
	}
}

// Close - io.Closer interface implementation.
func (r *Rotator) Close() error {
	return r.closer.Close()
}
