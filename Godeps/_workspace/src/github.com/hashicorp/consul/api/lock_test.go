package api

import (
	"log"
	"sync"
	"testing"
	"time"
)

func TestLock_LockUnlock(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	lock, err := c.LockKey("test/lock")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Initial unlock should fail
	err = lock.Unlock()
	if err != ErrLockNotHeld {
		t.Fatalf("err: %v", err)
	}

	// Should work
	leaderCh, err := lock.Lock(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if leaderCh == nil {
		t.Fatalf("not leader")
	}

	// Double lock should fail
	_, err = lock.Lock(nil)
	if err != ErrLockHeld {
		t.Fatalf("err: %v", err)
	}

	// Should be leader
	select {
	case <-leaderCh:
		t.Fatalf("should be leader")
	default:
	}

	// Initial unlock should work
	err = lock.Unlock()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Double unlock should fail
	err = lock.Unlock()
	if err != ErrLockNotHeld {
		t.Fatalf("err: %v", err)
	}

	// Should loose leadership
	select {
	case <-leaderCh:
	case <-time.After(time.Second):
		t.Fatalf("should not be leader")
	}
}

func TestLock_ForceInvalidate(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	lock, err := c.LockKey("test/lock")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Should work
	leaderCh, err := lock.Lock(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if leaderCh == nil {
		t.Fatalf("not leader")
	}
	defer lock.Unlock()

	go func() {
		// Nuke the session, simulator an operator invalidation
		// or a health check failure
		session := c.Session()
		session.Destroy(lock.lockSession, nil)
	}()

	// Should loose leadership
	select {
	case <-leaderCh:
	case <-time.After(time.Second):
		t.Fatalf("should not be leader")
	}
}

func TestLock_DeleteKey(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	lock, err := c.LockKey("test/lock")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Should work
	leaderCh, err := lock.Lock(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if leaderCh == nil {
		t.Fatalf("not leader")
	}
	defer lock.Unlock()

	go func() {
		// Nuke the key, simulate an operator intervention
		kv := c.KV()
		kv.Delete("test/lock", nil)
	}()

	// Should loose leadership
	select {
	case <-leaderCh:
	case <-time.After(time.Second):
		t.Fatalf("should not be leader")
	}
}

func TestLock_Contend(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	wg := &sync.WaitGroup{}
	acquired := make([]bool, 3)
	for idx := range acquired {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			lock, err := c.LockKey("test/lock")
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			// Should work eventually, will contend
			leaderCh, err := lock.Lock(nil)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if leaderCh == nil {
				t.Fatalf("not leader")
			}
			defer lock.Unlock()
			log.Printf("Contender %d acquired", idx)

			// Set acquired and then leave
			acquired[idx] = true
		}(idx)
	}

	// Wait for termination
	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	// Wait for everybody to get a turn
	select {
	case <-doneCh:
	case <-time.After(3 * DefaultLockRetryTime):
		t.Fatalf("timeout")
	}

	for idx, did := range acquired {
		if !did {
			t.Fatalf("contender %d never acquired", idx)
		}
	}
}

func TestLock_Destroy(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	lock, err := c.LockKey("test/lock")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Should work
	leaderCh, err := lock.Lock(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if leaderCh == nil {
		t.Fatalf("not leader")
	}

	// Destroy should fail
	if err := lock.Destroy(); err != ErrLockHeld {
		t.Fatalf("err: %v", err)
	}

	// Should be able to release
	err = lock.Unlock()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Acquire with a different lock
	l2, err := c.LockKey("test/lock")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Should work
	leaderCh, err = l2.Lock(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if leaderCh == nil {
		t.Fatalf("not leader")
	}

	// Destroy should still fail
	if err := lock.Destroy(); err != ErrLockInUse {
		t.Fatalf("err: %v", err)
	}

	// Should relese
	err = l2.Unlock()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Destroy should work
	err = lock.Destroy()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Double destroy should work
	err = l2.Destroy()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestLock_Conflict(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	sema, err := c.SemaphorePrefix("test/lock/", 2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Should work
	lockCh, err := sema.Acquire(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if lockCh == nil {
		t.Fatalf("not hold")
	}
	defer sema.Release()

	lock, err := c.LockKey("test/lock/.lock")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Should conflict with semaphore
	_, err = lock.Lock(nil)
	if err != ErrLockConflict {
		t.Fatalf("err: %v", err)
	}

	// Should conflict with semaphore
	err = lock.Destroy()
	if err != ErrLockConflict {
		t.Fatalf("err: %v", err)
	}
}

func TestLock_ReclaimLock(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	session, _, err := c.Session().Create(&SessionEntry{}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	lock, err := c.LockOpts(&LockOptions{Key: "test/lock", Session: session})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Should work
	leaderCh, err := lock.Lock(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if leaderCh == nil {
		t.Fatalf("not leader")
	}
	defer lock.Unlock()

	l2, err := c.LockOpts(&LockOptions{Key: "test/lock", Session: session})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	reclaimed := make(chan (<-chan struct{}), 1)
	go func() {
		l2Ch, err := l2.Lock(nil)
		if err != nil {
			t.Fatalf("not locked: %v", err)
		}
		reclaimed <- l2Ch
	}()

	// Should reclaim the lock
	var leader2Ch <-chan struct{}

	select {
	case leader2Ch = <-reclaimed:
	case <-time.After(time.Second):
		t.Fatalf("should have locked")
	}

	// unlock should work
	err = l2.Unlock()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	//Both locks should see the unlock
	select {
	case <-leader2Ch:
	case <-time.After(time.Second):
		t.Fatalf("should not be leader")
	}

	select {
	case <-leaderCh:
	case <-time.After(time.Second):
		t.Fatalf("should not be leader")
	}
}
