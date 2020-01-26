package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pascaldekloe/goe/verify"
)

func TestAPI_SessionCreateDestroy(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	s.WaitForSerfCheck(t)

	session := c.Session()

	id, meta, err := session.Create(nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if meta.RequestTime == 0 {
		t.Fatalf("bad: %v", meta)
	}

	if id == "" {
		t.Fatalf("invalid: %v", id)
	}

	meta, err = session.Destroy(id, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if meta.RequestTime == 0 {
		t.Fatalf("bad: %v", meta)
	}
}

func TestAPI_SessionCreateRenewDestroy(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	s.WaitForSerfCheck(t)

	session := c.Session()

	se := &SessionEntry{
		TTL: "10s",
	}

	id, meta, err := session.Create(se, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer session.Destroy(id, nil)

	if meta.RequestTime == 0 {
		t.Fatalf("bad: %v", meta)
	}

	if id == "" {
		t.Fatalf("invalid: %v", id)
	}

	if meta.RequestTime == 0 {
		t.Fatalf("bad: %v", meta)
	}

	renew, meta, err := session.Renew(id, nil)

	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if meta.RequestTime == 0 {
		t.Fatalf("bad: %v", meta)
	}

	if renew == nil {
		t.Fatalf("should get session")
	}

	if renew.ID != id {
		t.Fatalf("should have matching id")
	}

	if renew.TTL != "10s" {
		t.Fatalf("should get session with TTL")
	}
}

func TestAPI_SessionCreateRenewDestroyRenew(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	s.WaitForSerfCheck(t)

	session := c.Session()

	entry := &SessionEntry{
		Behavior: SessionBehaviorDelete,
		TTL:      "500s", // disable ttl
	}

	id, meta, err := session.Create(entry, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if meta.RequestTime == 0 {
		t.Fatalf("bad: %v", meta)
	}

	if id == "" {
		t.Fatalf("invalid: %v", id)
	}

	// Extend right after create. Everything should be fine.
	entry, _, err = session.Renew(id, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if entry == nil {
		t.Fatal("session unexpectedly vanished")
	}

	// Simulate TTL loss by manually destroying the session.
	meta, err = session.Destroy(id, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if meta.RequestTime == 0 {
		t.Fatalf("bad: %v", meta)
	}

	// Extend right after delete. The 404 should proxy as a nil.
	entry, _, err = session.Renew(id, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if entry != nil {
		t.Fatal("session still exists")
	}
}

func TestAPI_SessionCreateDestroyRenewPeriodic(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	s.WaitForSerfCheck(t)

	session := c.Session()

	entry := &SessionEntry{
		Behavior: SessionBehaviorDelete,
		TTL:      "500s", // disable ttl
	}

	id, meta, err := session.Create(entry, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if meta.RequestTime == 0 {
		t.Fatalf("bad: %v", meta)
	}

	if id == "" {
		t.Fatalf("invalid: %v", id)
	}

	// This only tests Create/Destroy/RenewPeriodic to avoid the more
	// difficult case of testing all of the timing code.

	// Simulate TTL loss by manually destroying the session.
	meta, err = session.Destroy(id, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if meta.RequestTime == 0 {
		t.Fatalf("bad: %v", meta)
	}

	// Extend right after delete. The 404 should terminate the loop quickly and return ErrSessionExpired.
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})
	go func() { errCh <- session.RenewPeriodic("1s", id, nil, doneCh) }()
	defer close(doneCh)

	select {
	case <-time.After(1 * time.Second):
		t.Fatal("timedout: missing session did not terminate renewal loop")
	case err = <-errCh:
		if err != ErrSessionExpired {
			t.Fatalf("err: %v", err)
		}
	}
}

func TestAPI_SessionRenewPeriodic_Cancel(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	s.WaitForSerfCheck(t)

	session := c.Session()
	entry := &SessionEntry{
		Behavior: SessionBehaviorDelete,
		TTL:      "500s", // disable ttl
	}

	t.Run("done channel", func(t *testing.T) {
		id, _, err := session.Create(entry, nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		errCh := make(chan error, 1)
		doneCh := make(chan struct{})
		go func() { errCh <- session.RenewPeriodic("1s", id, nil, doneCh) }()

		close(doneCh)

		select {
		case <-time.After(1 * time.Second):
			t.Fatal("renewal loop didn't terminate")
		case err = <-errCh:
			if err != nil {
				t.Fatalf("err: %v", err)
			}
		}

		sess, _, err := session.Info(id, nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if sess != nil {
			t.Fatalf("session was not expired")
		}
	})

	t.Run("context", func(t *testing.T) {
		id, _, err := session.Create(entry, nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		wo := new(WriteOptions).WithContext(ctx)

		errCh := make(chan error, 1)
		go func() { errCh <- session.RenewPeriodic("1s", id, wo, nil) }()

		cancel()

		select {
		case <-time.After(1 * time.Second):
			t.Fatal("renewal loop didn't terminate")
		case err = <-errCh:
			if err == nil || !strings.Contains(err.Error(), "context canceled") {
				t.Fatalf("err: %v", err)
			}
		}

		// See comment in session.go for why the session isn't removed
		// in this case.
		sess, _, err := session.Info(id, nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if sess == nil {
			t.Fatalf("session should not be expired")
		}
	})
}

func TestAPI_SessionInfo(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	s.WaitForSerfCheck(t)

	session := c.Session()

	id, _, err := session.Create(nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer session.Destroy(id, nil)

	info, qm, err := session.Info(id, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if qm.LastIndex == 0 {
		t.Fatalf("bad: %v", qm)
	}
	if !qm.KnownLeader {
		t.Fatalf("bad: %v", qm)
	}

	if info.CreateIndex == 0 {
		t.Fatalf("bad: %v", info)
	}
	info.CreateIndex = 0

	want := &SessionEntry{
		ID:        id,
		Node:      s.Config.NodeName,
		Checks:    []string{"serfHealth"},
		LockDelay: 15 * time.Second,
		Behavior:  SessionBehaviorRelease,
	}
	verify.Values(t, "", info, want)
}

func TestAPI_SessionInfo_NoChecks(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	session := c.Session()

	id, _, err := session.CreateNoChecks(nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer session.Destroy(id, nil)

	info, qm, err := session.Info(id, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if qm.LastIndex == 0 {
		t.Fatalf("bad: %v", qm)
	}
	if !qm.KnownLeader {
		t.Fatalf("bad: %v", qm)
	}

	if info.CreateIndex == 0 {
		t.Fatalf("bad: %v", info)
	}
	info.CreateIndex = 0

	want := &SessionEntry{
		ID:        id,
		Node:      s.Config.NodeName,
		Checks:    []string{},
		LockDelay: 15 * time.Second,
		Behavior:  SessionBehaviorRelease,
	}
	verify.Values(t, "", info, want)
}

func TestAPI_SessionNode(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	s.WaitForSerfCheck(t)

	session := c.Session()

	id, _, err := session.Create(nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer session.Destroy(id, nil)

	info, qm, err := session.Info(id, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	sessions, qm, err := session.Node(info.Node, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("bad: %v", sessions)
	}

	if qm.LastIndex == 0 {
		t.Fatalf("bad: %v", qm)
	}
	if !qm.KnownLeader {
		t.Fatalf("bad: %v", qm)
	}
}

func TestAPI_SessionList(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	s.WaitForSerfCheck(t)

	session := c.Session()

	id, _, err := session.Create(nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer session.Destroy(id, nil)

	sessions, qm, err := session.List(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("bad: %v", sessions)
	}

	if qm.LastIndex == 0 {
		t.Fatalf("bad: %v", qm)
	}
	if !qm.KnownLeader {
		t.Fatalf("bad: %v", qm)
	}
}
