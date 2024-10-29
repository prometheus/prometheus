package relabeler

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
)

// RotateTimer - custom timer with reset the timer for the delay time.
type RotateTimer struct {
	clock            clockwork.Clock
	timer            clockwork.Timer
	rotateAt         time.Time
	mx               *sync.Mutex
	durationBlock    int64
	rndDurationBlock int64
}

// NewRotateTimer - init new RotateTimer. The duration durationBlock and delayAfterNotify must be greater than zero;
// if not, Ticker will panic. Stop the ticker to release associated resources.
func NewRotateTimer(clock clockwork.Clock, desiredBlockFormationDuration time.Duration) *RotateTimer {
	return NewRotateTimerWithSeed(clock, desiredBlockFormationDuration, uint64(clock.Now().UnixNano()))
}

// NewRotateTimerWithSeed - init new RotateTimer. The duration durationBlock and delayAfterNotify must be greater than zero;
// if not, Ticker will panic. Stop the ticker to release associated resources.
func NewRotateTimerWithSeed(clock clockwork.Clock, desiredBlockFormationDuration time.Duration, seed uint64) *RotateTimer {
	bd := desiredBlockFormationDuration.Milliseconds()
	//nolint:gosec // there is no need for cryptographic strength here
	rnd := rand.New(rand.NewSource(int64(seed)))
	rt := &RotateTimer{
		clock:            clock,
		durationBlock:    bd,
		rndDurationBlock: rnd.Int63n(bd),
		mx:               new(sync.Mutex),
	}

	rt.rotateAt = rt.RotateAtNext()
	rt.timer = clock.NewTimer(rt.rotateAt.Sub(rt.clock.Now()))

	return rt
}

// Chan - return chan with ticker time.
func (rt *RotateTimer) Chan() <-chan time.Time {
	return rt.timer.Chan()
}

// Reset - changes the timer to expire after duration Block and clearing channels.
func (rt *RotateTimer) Reset() {
	rt.mx.Lock()
	rt.rotateAt = rt.RotateAtNext()
	if !rt.timer.Stop() {
		select {
		case <-rt.timer.Chan():
		default:
		}
	}
	rt.timer.Reset(rt.rotateAt.Sub(rt.clock.Now()))
	rt.mx.Unlock()
}

// RotateAtNext - calculated next rotate time.
func (rt *RotateTimer) RotateAtNext() time.Time {
	now := rt.clock.Now().UnixMilli()
	k := now % rt.durationBlock
	startBlock := math.Floor(float64(now)/float64(rt.durationBlock)) * float64(rt.durationBlock)

	if rt.rndDurationBlock > k {
		return time.UnixMilli(int64(startBlock) + rt.rndDurationBlock)
	}

	return time.UnixMilli(int64(startBlock) + rt.durationBlock + rt.rndDurationBlock)
}

// Stop - prevents the Timer from firing.
// Stop does not close the channel, to prevent a read from the channel succeeding incorrectly.
func (rt *RotateTimer) Stop() {
	if !rt.timer.Stop() {
		<-rt.timer.Chan()
	}
}
