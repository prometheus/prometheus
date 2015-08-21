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
