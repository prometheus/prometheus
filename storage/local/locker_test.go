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
	"testing"

	"github.com/prometheus/common/model"
)

func BenchmarkFingerprintLockerParallel(b *testing.B) {
	numGoroutines := 10
	numFingerprints := 10
	numLockOps := b.N
	locker := newFingerprintLocker(100)

	wg := sync.WaitGroup{}
	b.ResetTimer()
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			for j := 0; j < numLockOps; j++ {
				fp1 := model.Fingerprint(j % numFingerprints)
				fp2 := model.Fingerprint(j%numFingerprints + numFingerprints)
				locker.Lock(fp1)
				locker.Lock(fp2)
				locker.Unlock(fp2)
				locker.Unlock(fp1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
}

func BenchmarkFingerprintLockerSerial(b *testing.B) {
	numFingerprints := 10
	locker := newFingerprintLocker(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fp := model.Fingerprint(i % numFingerprints)
		locker.Lock(fp)
		locker.Unlock(fp)
	}
}
