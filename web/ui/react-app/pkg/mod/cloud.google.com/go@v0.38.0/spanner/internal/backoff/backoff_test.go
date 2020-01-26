/*
Copyright 2017 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package backoff

import (
	"math"
	"testing"
	"time"
)

// Test if exponential backoff helper can produce correct series of
// retry delays.
func TestBackoff(t *testing.T) {
	b := ExponentialBackoff{minBackoff, maxBackoff}
	tests := []struct {
		retries int
		min     time.Duration
		max     time.Duration
	}{
		{
			retries: 0,
			min:     minBackoff,
			max:     minBackoff,
		},
		{
			retries: 1,
			min:     minBackoff,
			max:     time.Duration(rate * float64(minBackoff)),
		},
		{
			retries: 3,
			min:     time.Duration(math.Pow(rate, 3) * (1 - jitter) * float64(minBackoff)),
			max:     time.Duration(math.Pow(rate, 3) * float64(minBackoff)),
		},
		{
			retries: 1000,
			min:     time.Duration((1 - jitter) * float64(maxBackoff)),
			max:     maxBackoff,
		},
	}
	for _, test := range tests {
		got := b.Delay(test.retries)
		if float64(got) < float64(test.min) || float64(got) > float64(test.max) {
			t.Errorf("delay(%v) = %v, want in range [%v, %v]", test.retries, got, test.min, test.max)
		}
	}
}
