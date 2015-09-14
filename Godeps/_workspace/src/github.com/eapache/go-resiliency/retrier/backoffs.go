package retrier

import "time"

// ConstantBackoff generates a simple back-off strategy of retrying 'n' times, and waiting 'amount' time after each one.
func ConstantBackoff(n int, amount time.Duration) []time.Duration {
	ret := make([]time.Duration, n)
	for i := range ret {
		ret[i] = amount
	}
	return ret
}

// ExponentialBackoff generates a simple back-off strategy of retrying 'n' times, and doubling the amount of
// time waited after each one.
func ExponentialBackoff(n int, initialAmount time.Duration) []time.Duration {
	ret := make([]time.Duration, n)
	next := initialAmount
	for i := range ret {
		ret[i] = next
		next *= 2
	}
	return ret
}
