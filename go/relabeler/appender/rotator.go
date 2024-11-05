package appender

import (
	"time"

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

type RotationTimer interface {
	Chan() <-chan time.Time
	Reset()
	Stop()
}

// Rotator is a rotation trigger.
type Rotator struct {
	rotatable     Rotatable
	timer         RotationTimer
	run           chan struct{}
	closer        *util.Closer
	rotateCounter prometheus.Counter
}

// NewRotator - Rotator constructor.
func NewRotator(rotatable Rotatable, timer RotationTimer, registerer prometheus.Registerer) *Rotator {
	factory := util.NewUnconflictRegisterer(registerer)
	r := &Rotator{
		rotatable: rotatable,
		timer:     timer,
		run:       make(chan struct{}),
		closer:    util.NewCloser(),
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

	select {
	case <-r.run:
		r.timer.Reset()
	case <-r.closer.Signal():
		return
	}

	for {
		select {
		case <-r.closer.Signal():
			return

		case <-r.timer.Chan():
			logger.Debugf("start rotation")
			if err := r.rotatable.Rotate(); err != nil {
				logger.Errorf("rotation failed: %s", err.Error())
			}
			r.rotateCounter.Inc()

			r.timer.Reset()
		}
	}
}

// Close - io.Closer interface implementation.
func (r *Rotator) Close() error {
	return r.closer.Close()
}
