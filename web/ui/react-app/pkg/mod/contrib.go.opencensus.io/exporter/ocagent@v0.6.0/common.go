// Copyright 2018, OpenCensus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ocagent

import (
	"math/rand"
	"time"
)

var randSrc = rand.New(rand.NewSource(time.Now().UnixNano()))

// retries function fn upto n times, if fn returns an error lest it returns nil early.
// It applies exponential backoff in units of (1<<n) + jitter microsends.
func nTriesWithExponentialBackoff(nTries int64, timeBaseUnit time.Duration, fn func() error) (err error) {
	for i := int64(0); i < nTries; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		// Backoff for a time period with a pseudo-random jitter
		jitter := time.Duration(randSrc.Float64()*100) * time.Microsecond
		ts := jitter + ((1 << uint64(i)) * timeBaseUnit)
		<-time.After(ts)
	}
	return err
}
