package appender

import (
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"time"
)

const DefaultRotateDuration = 2 * time.Hour

type Rotatable interface {
	Rotate(headRotator relabeler.HeadRotator) error
}

type Rotator struct {
	rotatable   Rotatable
	headRotator relabeler.HeadRotator
	rotateTimer *relabeler.RotateTimer
	done        chan struct{}
	close       chan struct{}
}

func NewRotator(rotatable Rotatable, headRotator relabeler.HeadRotator, clock clockwork.Clock, rotateDuration time.Duration) *Rotator {
	r := &Rotator{
		rotatable:   rotatable,
		headRotator: headRotator,
		rotateTimer: relabeler.NewRotateTimer(clock, rotateDuration),
		done:        make(chan struct{}),
		close:       make(chan struct{}),
	}

	go r.rotateLoop()

	return r
}

func (r *Rotator) rotateLoop() {
	defer close(r.done)
	defer r.rotateTimer.Stop()
	for {
		select {
		case <-r.close:
			return

		case <-r.rotateTimer.Chan():
			if err := r.rotatable.Rotate(r.headRotator); err != nil {
				// todo: log rotation error
			}
			r.rotateTimer.Reset()
		}
	}
}

func (r *Rotator) Close() error {
	close(r.close)
	<-r.done
	return nil
}
