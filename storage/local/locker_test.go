package local

import (
	"sync"
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"
)

var httpServerStarted bool

func BenchmarkFingerprintLockerParallel(b *testing.B) {
	numGoroutines := 10
	numFingerprints := 10
	numLockOps := b.N
	locker := newFingerprintLocker()

	wg := sync.WaitGroup{}
	b.ResetTimer()
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			for j := 0; j < numLockOps; j++ {
				fp1 := clientmodel.Fingerprint(j % numFingerprints)
				fp2 := clientmodel.Fingerprint(j%numFingerprints + numFingerprints)
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
	locker := newFingerprintLocker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fp := clientmodel.Fingerprint(i % numFingerprints)
		locker.Lock(fp)
		locker.Unlock(fp)
	}
}
