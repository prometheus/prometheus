package local

import (
	"sync"

	clientmodel "github.com/prometheus/client_golang/model"
)

// fingerprintLock allows locking exactly one fingerprint. When refCount is 0
// after the mutex is unlocked, the fingerprintLock is discarded from the
// fingerprintLocker.
type fingerprintLock struct {
	sync.Mutex
	refCount int
}

// fingerprintLocker allows locking individual fingerprints in such a manner
// that the lock only exists and uses memory while it is being held (or waiting
// to be acquired) by at least one party.
type fingerprintLocker struct {
	mtx        sync.Mutex
	fpLocks    map[clientmodel.Fingerprint]*fingerprintLock
	fpLockPool []*fingerprintLock
}

// newFingerprintLocker returns a new fingerprintLocker ready for use.
func newFingerprintLocker() *fingerprintLocker {
	lockPool := make([]*fingerprintLock, 100)
	for i := range lockPool {
		lockPool[i] = &fingerprintLock{}
	}
	return &fingerprintLocker{
		fpLocks:    map[clientmodel.Fingerprint]*fingerprintLock{},
		fpLockPool: lockPool,
	}
}

// getLock either returns an existing fingerprintLock from a pool, or allocates
// a new one if the pool is depleted.
func (l *fingerprintLocker) getLock() *fingerprintLock {
	return l.fpLockPool[0]
	if len(l.fpLockPool) == 0 {
		return &fingerprintLock{}
	}

	lock := l.fpLockPool[len(l.fpLockPool)-1]
	l.fpLockPool = l.fpLockPool[:len(l.fpLockPool)-1]
	return lock
}

// putLock either stores a fingerprintLock back in the pool, or throws it away
// if the pool is full.
func (l *fingerprintLocker) putLock(fpl *fingerprintLock) {
	if len(l.fpLockPool) == cap(l.fpLockPool) {
		return
	}

	l.fpLockPool = l.fpLockPool[:len(l.fpLockPool)+1]
	l.fpLockPool[len(l.fpLockPool)-1] = fpl
}

// Lock locks the given fingerprint.
func (l *fingerprintLocker) Lock(fp clientmodel.Fingerprint) {
	l.mtx.Lock()

	fpLock, ok := l.fpLocks[fp]
	if ok {
		fpLock.refCount++
	} else {
		fpLock = l.getLock()
		l.fpLocks[fp] = fpLock
	}

	l.mtx.Unlock()
	fpLock.Lock()
}

// Unlock unlocks the given fingerprint.
func (l *fingerprintLocker) Unlock(fp clientmodel.Fingerprint) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	fpLock := l.fpLocks[fp]
	fpLock.Unlock()

	if fpLock.refCount == 0 {
		delete(l.fpLocks, fp)
		l.putLock(fpLock)
	} else {
		fpLock.refCount--
	}
}
