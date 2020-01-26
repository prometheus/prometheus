package api

import (
	"bytes"
	"strings"
	"testing"
)

func TestAPI_Snapshot(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	s.WaitForSerfCheck(t)
	// Place an initial key into the store.
	kv := c.KV()
	key := &KVPair{Key: testKey(), Value: []byte("hello")}
	if _, err := kv.Put(key, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Make sure it reads back.
	pair, _, err := kv.Get(key.Key, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pair == nil {
		t.Fatalf("expected value: %#v", pair)
	}
	if !bytes.Equal(pair.Value, []byte("hello")) {
		t.Fatalf("unexpected value: %#v", pair)
	}

	// Take a snapshot.
	snapshot := c.Snapshot()
	snap, qm, err := snapshot.Save(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer snap.Close()

	// Sanity check th query metadata.
	if qm.LastIndex == 0 || !qm.KnownLeader ||
		qm.RequestTime == 0 {
		t.Fatalf("bad: %v", qm)
	}

	// Overwrite the key's value.
	key.Value = []byte("goodbye")
	if _, err := kv.Put(key, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Read the key back and look for the new value.
	pair, _, err = kv.Get(key.Key, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pair == nil {
		t.Fatalf("expected value: %#v", pair)
	}
	if !bytes.Equal(pair.Value, []byte("goodbye")) {
		t.Fatalf("unexpected value: %#v", pair)
	}

	// Restore the snapshot.
	if err := snapshot.Restore(nil, snap); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Read the key back and look for the original value.
	pair, _, err = kv.Get(key.Key, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pair == nil {
		t.Fatalf("expected value: %#v", pair)
	}
	if !bytes.Equal(pair.Value, []byte("hello")) {
		t.Fatalf("unexpected value: %#v", pair)
	}
}

func TestAPI_Snapshot_Options(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	// Try to take a snapshot with a bad token.
	snapshot := c.Snapshot()
	_, _, err := snapshot.Save(&QueryOptions{Token: "anonymous"})
	if err == nil || !strings.Contains(err.Error(), "Permission denied") {
		t.Fatalf("err: %v", err)
	}

	// Now try an unknown DC.
	_, _, err = snapshot.Save(&QueryOptions{Datacenter: "nope"})
	if err == nil || !strings.Contains(err.Error(), "No path to datacenter") {
		t.Fatalf("err: %v", err)
	}

	// This should work with a valid token.
	snap, _, err := snapshot.Save(&QueryOptions{Token: "root"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer snap.Close()

	// This should work with a stale snapshot. This doesn't have good feedback
	// that the stale option was sent, but it makes sure nothing bad happens.
	snap, _, err = snapshot.Save(&QueryOptions{Token: "root", AllowStale: true})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer snap.Close()

	// Try to restore a snapshot with a bad token.
	null := bytes.NewReader([]byte(""))
	err = snapshot.Restore(&WriteOptions{Token: "anonymous"}, null)
	if err == nil || !strings.Contains(err.Error(), "Permission denied") {
		t.Fatalf("err: %v", err)
	}

	// Now try an unknown DC.
	null = bytes.NewReader([]byte(""))
	err = snapshot.Restore(&WriteOptions{Datacenter: "nope"}, null)
	if err == nil || !strings.Contains(err.Error(), "No path to datacenter") {
		t.Fatalf("err: %v", err)
	}

	// This should work.
	if err := snapshot.Restore(&WriteOptions{Token: "root"}, snap); err != nil {
		t.Fatalf("err: %v", err)
	}
}
