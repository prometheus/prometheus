// Package retrier implements the "retriable" resiliency pattern for Go.
package retrier

import (
	"math/rand"
	"time"
)

// Retrier implements the "retriable" resiliency pattern, abstracting out the process of retrying a failed action
// a certain number of times with an optional back-off between each retry.
type Retrier struct {
	backoff []time.Duration
	class   Classifier
	jitter  float64
	rand    *rand.Rand
}

// New constructs a Retrier with the given backoff pattern and classifier. The length of the backoff pattern
// indicates how many times an action will be retried, and the value at each index indicates the amount of time
// waited before each subsequent retry. The classifier is used to determine which errors should be retried and
// which should cause the retrier to fail fast. The DefaultClassifier is used if nil is passed.
func New(backoff []time.Duration, class Classifier) *Retrier {
	if class == nil {
		class = DefaultClassifier{}
	}

	return &Retrier{
		backoff: backoff,
		class:   class,
		rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Run executes the given work function, then classifies its return value based on the classifier used
// to construct the Retrier. If the result is Succeed or Fail, the return value of the work function is
// returned to the caller. If the result is Retry, then Run sleeps according to the its backoff policy
// before retrying. If the total number of retries is exceeded then the return value of the work function
// is returned to the caller regardless.
func (r *Retrier) Run(work func() error) error {
	retries := 0
	for {
		ret := work()

		switch r.class.Classify(ret) {
		case Succeed, Fail:
			return ret
		case Retry:
			if retries >= len(r.backoff) {
				return ret
			}
			time.Sleep(r.calcSleep(retries))
			retries++
		}
	}
}

func (r *Retrier) calcSleep(i int) time.Duration {
	// take a random float in the range (-r.jitter, +r.jitter) and multiply it by the base amount
	return r.backoff[i] + time.Duration(((r.rand.Float64()*2)-1)*r.jitter*float64(r.backoff[i]))
}

// SetJitter sets the amount of jitter on each back-off to a factor between 0.0 and 1.0 (values outside this range
// are silently ignored). When a retry occurs, the back-off is adjusted by a random amount up to this value.
func (r *Retrier) SetJitter(jit float64) {
	if jit < 0 || jit > 1 {
		return
	}
	r.jitter = jit
}
