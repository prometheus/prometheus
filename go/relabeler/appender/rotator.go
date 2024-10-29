package appender

import (
	"time"

	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/util"
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
	rotatable Rotatable
	timer     RotationTimer
	run       chan struct{}
	closer    *util.Closer
}

// NewRotator - Rotator constructor.
func NewRotator(rotatable Rotatable, timer RotationTimer) *Rotator {
	r := &Rotator{
		rotatable: rotatable,
		timer:     timer,
		run:       make(chan struct{}),
		closer:    util.NewCloser(),
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

			r.timer.Reset()
		}
	}
}

// Close - io.Closer interface implementation.
func (r *Rotator) Close() error {
	return r.closer.Close()
}
