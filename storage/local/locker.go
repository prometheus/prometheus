// Copyright 2016 The Prometheus Authors
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

package local

import (
	"sync"

	"github.com/prometheus/common/model"
)

// fingerprintLocker allows locking individual fingerprints. To limit the number
// of mutexes needed for that, only a fixed number of mutexes are
// allocated. Fingerprints to be locked are assigned to those pre-allocated
// mutexes by their value. (Note that fingerprints are calculated by a hash
// function, so that an approximately equal distribution over the mutexes is
// expected, even without additional hashing of the fingerprint value.)
// Collisions are not detected. If two fingerprints get assigned to the same
// mutex, only one of them can be locked at the same time. As long as the number
// of pre-allocated mutexes is much larger than the number of goroutines
// requiring a fingerprint lock concurrently, the loss in efficiency is
// small. However, a goroutine must never lock more than one fingerprint at the
// same time. (In that case a collision would try to acquire the same mutex
// twice).
type fingerprintLocker struct {
	fpMtxs    []sync.Mutex
	numFpMtxs uint
}

// newFingerprintLocker returns a new fingerprintLocker ready for use.
func newFingerprintLocker(preallocatedMutexes int) *fingerprintLocker {
	return &fingerprintLocker{
		make([]sync.Mutex, preallocatedMutexes),
		uint(preallocatedMutexes),
	}
}

// Lock locks the given fingerprint.
func (l *fingerprintLocker) Lock(fp model.Fingerprint) {
	l.fpMtxs[uint(fp)%l.numFpMtxs].Lock()
}

// Unlock unlocks the given fingerprint.
func (l *fingerprintLocker) Unlock(fp model.Fingerprint) {
	l.fpMtxs[uint(fp)%l.numFpMtxs].Unlock()
}
