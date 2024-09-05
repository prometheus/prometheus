package appender

import (
	"fmt"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/util"
	"time"
)

const DefaultRotateDuration = 2 * time.Hour

type Rotatable interface {
	Rotate() error
}

type Rotator struct {
	rotatable   Rotatable
	rotateTimer *relabeler.RotateTimer
	run         chan struct{}
	closer      *util.Closer
}

func NewRotator(rotatable Rotatable, clock clockwork.Clock, rotateDuration time.Duration) *Rotator {
	r := &Rotator{
		rotatable:   rotatable,
		rotateTimer: relabeler.NewRotateTimer(clock, rotateDuration),
		run:         make(chan struct{}),
		closer:      util.NewCloser(),
	}
	go r.loop()

	return r
}

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
			fmt.Println("ROTATION!")
			if err := r.rotatable.Rotate(); err != nil {
				// todo: log rotation error
				fmt.Println("ROTATION FAILED: ", err.Error())
			} else {
				fmt.Println("ROTATION COMPLETED")
			}

			r.rotateTimer.Reset()
		}
	}
}

func (r *Rotator) Close() error {
	return r.closer.Close()
}
