package serf

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestSnapshotter(t *testing.T) {
	td, err := ioutil.TempDir("", "serf")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer os.RemoveAll(td)

	clock := new(LamportClock)
	outCh := make(chan Event, 64)
	stopCh := make(chan struct{})
	logger := log.New(os.Stderr, "", log.LstdFlags)
	inCh, snap, err := NewSnapshotter(td+"snap", snapshotSizeLimit, false,
		logger, clock, outCh, stopCh)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Write some user events
	ue := UserEvent{
		LTime: 42,
		Name:  "bar",
	}
	inCh <- ue

	// Write some queries
	q := &Query{
		LTime: 50,
		Name:  "uptime",
	}
	inCh <- q

	// Write some member events
	clock.Witness(100)
	meJoin := MemberEvent{
		Type: EventMemberJoin,
		Members: []Member{
			Member{
				Name: "foo",
				Addr: []byte{127, 0, 0, 1},
				Port: 5000,
			},
		},
	}
	meFail := MemberEvent{
		Type: EventMemberFailed,
		Members: []Member{
			Member{
				Name: "foo",
				Addr: []byte{127, 0, 0, 1},
				Port: 5000,
			},
		},
	}
	inCh <- meJoin
	inCh <- meFail
	inCh <- meJoin

	// Check these get passed through
	select {
	case e := <-outCh:
		if !reflect.DeepEqual(e, ue) {
			t.Fatalf("expected user event: %#v", e)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout")
	}

	select {
	case e := <-outCh:
		if !reflect.DeepEqual(e, q) {
			t.Fatalf("expected query event: %#v", e)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout")
	}

	select {
	case e := <-outCh:
		if !reflect.DeepEqual(e, meJoin) {
			t.Fatalf("expected member event: %#v", e)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout")
	}

	select {
	case e := <-outCh:
		if !reflect.DeepEqual(e, meFail) {
			t.Fatalf("expected member event: %#v", e)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout")
	}

	select {
	case e := <-outCh:
		if !reflect.DeepEqual(e, meJoin) {
			t.Fatalf("expected member event: %#v", e)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout")
	}

	// Close the snapshoter
	close(stopCh)
	snap.Wait()

	// Open the snapshoter
	stopCh = make(chan struct{})
	_, snap, err = NewSnapshotter(td+"snap", snapshotSizeLimit, false,
		logger, clock, outCh, stopCh)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Check the values
	if snap.LastClock() != 100 {
		t.Fatalf("bad clock %d", snap.LastClock())
	}
	if snap.LastEventClock() != 42 {
		t.Fatalf("bad clock %d", snap.LastEventClock())
	}
	if snap.LastQueryClock() != 50 {
		t.Fatalf("bad clock %d", snap.LastQueryClock())
	}

	prev := snap.AliveNodes()
	if len(prev) != 1 {
		t.Fatalf("expected alive: %#v", prev)
	}
	if prev[0].Name != "foo" {
		t.Fatalf("bad name: %#v", prev[0])
	}
	if prev[0].Addr != "127.0.0.1:5000" {
		t.Fatalf("bad addr: %#v", prev[0])
	}

	// Close the snapshotter.
	close(stopCh)
	snap.Wait()

	// Open the snapshotter, make sure nothing dies reading with coordinates
	// disabled.
	stopCh = make(chan struct{})
	_, snap, err = NewSnapshotter(td+"snap", snapshotSizeLimit, false,
		logger, clock, outCh, stopCh)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	close(stopCh)
	snap.Wait()
}

func TestSnapshotter_forceCompact(t *testing.T) {
	td, err := ioutil.TempDir("", "serf")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer os.RemoveAll(td)

	clock := new(LamportClock)
	stopCh := make(chan struct{})
	logger := log.New(os.Stderr, "", log.LstdFlags)

	// Create a very low limit
	inCh, snap, err := NewSnapshotter(td+"snap", 1024, false,
		logger, clock, nil, stopCh)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Write lots of user events
	for i := 0; i < 1024; i++ {
		ue := UserEvent{
			LTime: LamportTime(i),
		}
		inCh <- ue
	}

	// Write lots of queries
	for i := 0; i < 1024; i++ {
		q := &Query{
			LTime: LamportTime(i),
		}
		inCh <- q
	}

	// Wait for drain
	for len(inCh) > 0 {
		time.Sleep(20 * time.Millisecond)
	}

	// Close the snapshoter
	close(stopCh)
	snap.Wait()

	// Open the snapshoter
	stopCh = make(chan struct{})
	_, snap, err = NewSnapshotter(td+"snap", snapshotSizeLimit, false,
		logger, clock, nil, stopCh)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Check the values
	if snap.LastEventClock() != 1023 {
		t.Fatalf("bad clock %d", snap.LastEventClock())
	}

	if snap.LastQueryClock() != 1023 {
		t.Fatalf("bad clock %d", snap.LastQueryClock())
	}

	close(stopCh)
	snap.Wait()
}

func TestSnapshotter_leave(t *testing.T) {
	td, err := ioutil.TempDir("", "serf")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer os.RemoveAll(td)

	clock := new(LamportClock)
	stopCh := make(chan struct{})
	logger := log.New(os.Stderr, "", log.LstdFlags)
	inCh, snap, err := NewSnapshotter(td+"snap", snapshotSizeLimit, false,
		logger, clock, nil, stopCh)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Write a user event
	ue := UserEvent{
		LTime: 42,
		Name:  "bar",
	}
	inCh <- ue

	// Write a query
	q := &Query{
		LTime: 50,
		Name:  "uptime",
	}
	inCh <- q

	// Write some member events
	clock.Witness(100)
	meJoin := MemberEvent{
		Type: EventMemberJoin,
		Members: []Member{
			Member{
				Name: "foo",
				Addr: []byte{127, 0, 0, 1},
				Port: 5000,
			},
		},
	}
	inCh <- meJoin

	// Wait for drain
	for len(inCh) > 0 {
		time.Sleep(20 * time.Millisecond)
	}

	// Leave the cluster!
	snap.Leave()

	// Close the snapshoter
	close(stopCh)
	snap.Wait()

	// Open the snapshoter
	stopCh = make(chan struct{})
	_, snap, err = NewSnapshotter(td+"snap", snapshotSizeLimit, false,
		logger, clock, nil, stopCh)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Check the values
	if snap.LastClock() != 0 {
		t.Fatalf("bad clock %d", snap.LastClock())
	}
	if snap.LastEventClock() != 0 {
		t.Fatalf("bad clock %d", snap.LastEventClock())
	}
	if snap.LastQueryClock() != 0 {
		t.Fatalf("bad clock %d", snap.LastQueryClock())
	}

	prev := snap.AliveNodes()
	if len(prev) != 0 {
		t.Fatalf("expected none alive: %#v", prev)
	}
}

func TestSnapshotter_leave_rejoin(t *testing.T) {
	td, err := ioutil.TempDir("", "serf")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer os.RemoveAll(td)

	clock := new(LamportClock)
	stopCh := make(chan struct{})
	logger := log.New(os.Stderr, "", log.LstdFlags)
	inCh, snap, err := NewSnapshotter(td+"snap", snapshotSizeLimit, true,
		logger, clock, nil, stopCh)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Write a user event
	ue := UserEvent{
		LTime: 42,
		Name:  "bar",
	}
	inCh <- ue

	// Write a query
	q := &Query{
		LTime: 50,
		Name:  "uptime",
	}
	inCh <- q

	// Write some member events
	clock.Witness(100)
	meJoin := MemberEvent{
		Type: EventMemberJoin,
		Members: []Member{
			Member{
				Name: "foo",
				Addr: []byte{127, 0, 0, 1},
				Port: 5000,
			},
		},
	}
	inCh <- meJoin

	// Wait for drain
	for len(inCh) > 0 {
		time.Sleep(20 * time.Millisecond)
	}

	// Leave the cluster!
	snap.Leave()

	// Close the snapshoter
	close(stopCh)
	snap.Wait()

	// Open the snapshoter
	stopCh = make(chan struct{})
	_, snap, err = NewSnapshotter(td+"snap", snapshotSizeLimit, true,
		logger, clock, nil, stopCh)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Check the values
	if snap.LastClock() != 100 {
		t.Fatalf("bad clock %d", snap.LastClock())
	}
	if snap.LastEventClock() != 42 {
		t.Fatalf("bad clock %d", snap.LastEventClock())
	}
	if snap.LastQueryClock() != 50 {
		t.Fatalf("bad clock %d", snap.LastQueryClock())
	}

	prev := snap.AliveNodes()
	if len(prev) == 0 {
		t.Fatalf("expected alive: %#v", prev)
	}
}

func TestSnapshotter_slowDiskNotBlockingEventCh(t *testing.T) {
	td, err := ioutil.TempDir("", "serf")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	t.Log("Temp dir", td)
	defer os.RemoveAll(td)

	clock := new(LamportClock)
	stopCh := make(chan struct{})
	logger := log.New(os.Stderr, "", log.LstdFlags)

	outCh := make(chan Event, 1024)
	inCh, snap, err := NewSnapshotter(td+"snap", snapshotSizeLimit, true,
		logger, clock, outCh, stopCh)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// We need enough events to be much more than the buffers used which are size
	// 1024. This number processes easily within the 500ms we allow below on my
	// host provided there is no disk IO on the path (I verified that by just
	// returning early in tryAppend using the old blocking code). The new async
	// method should pass without disabling disk writes too!
	numEvents := 10000

	// Write lots of member updates (way bigger than our chan buffers)
	startCh := make(chan struct{})
	go func() {
		<-startCh
		for i := 0; i < numEvents; i++ {
			e := MemberEvent{
				Type: EventMemberJoin,
				Members: []Member{
					Member{
						Name: fmt.Sprintf("foo%d", i),
						Addr: []byte{127, 0, byte((i / 256) % 256), byte(i % 256)},
						Port: 5000,
					},
				},
			}
			if i%10 == 0 {
				// 1/10 events is a leave
				e.Type = EventMemberLeave
			}
			inCh <- e
			// Pace ourselves - if we just throw these out as fast as possible the
			// read loop below can't keep up and we end up dropping messages due to
			// backpressure. But we need to still send them all in well less than the
			// timeout, 10k messages at 1 microsecond should take 10 ms minimum. In
			// practice it's quite a bit more to actually process and because the
			// buffer here blocks.
			time.Sleep(1 * time.Microsecond)
		}
	}()

	// Wait for them all to process through and it should be in a lot less time
	// than if the disk IO was in serial. This was verified by running this test
	// against the old serial implementation and seeing it never come close to
	// passing on my laptop with an SSD. It's not the most robust thing ever but
	// it's at least a sanity check that we are non-blocking now, and it passes
	// reliably at least on my machine. I typically see this complete in around
	// 115ms on my machine so this should give plenty of headroom for slower CI
	// environments while still being low enough that actual disk IO would
	// reliably blow it.
	deadline := time.After(500 * time.Millisecond)
	numRecvd := 0
	start := time.Now()
	for numRecvd < numEvents {
		select {
		// Now we are ready to listen, start the generator goroutine off
		case startCh <- struct{}{}:
			continue
		case <-outCh:
			numRecvd++
		case <-deadline:
			t.Fatalf("timed out after %s waiting for messages blocked on fake disk IO? "+
				"got %d of %d", time.Since(start), numRecvd, numEvents)
		}
	}

	// Close the snapshoter
	close(stopCh)
	snap.Wait()
}

func TestSnapshotter_blockedUpstreamNotBlockingMemberlist(t *testing.T) {
	td, err := ioutil.TempDir("", "serf")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	t.Log("Temp dir", td)
	defer os.RemoveAll(td)

	clock := new(LamportClock)
	stopCh := make(chan struct{})
	logger := log.New(os.Stderr, "", log.LstdFlags)

	// OutCh is unbuffered simulating a slow upstream
	outCh := make(chan Event)

	inCh, snap, err := NewSnapshotter(td+"snap", snapshotSizeLimit, true,
		logger, clock, outCh, stopCh)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// We need enough events to be more than the internal buffer sizes
	numEvents := 3000

	// Send some updates
	for i := 0; i < numEvents; i++ {
		e := MemberEvent{
			Type: EventMemberJoin,
			Members: []Member{
				Member{
					Name: fmt.Sprintf("foo%d", i),
					Addr: []byte{127, 0, byte((i / 256) % 256), byte(i % 256)},
					Port: 5000,
				},
			},
		}
		if i%10 == 0 {
			// 1/10 events is a leave
			e.Type = EventMemberLeave
		}
		select {
		case inCh <- e:
		default:
			t.Fatalf("inCh should never block")
		}
		// Allow just the tiniest time so that the runtime can schedule the
		// goroutine that's reading this even if they are both on the same physical
		// core (like in CI).
		time.Sleep(1 * time.Microsecond)
	}

	// Close the snapshoter
	close(stopCh)
	snap.Wait()
}
