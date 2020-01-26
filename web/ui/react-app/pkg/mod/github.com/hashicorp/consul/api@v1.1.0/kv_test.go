package api

import (
	"bytes"
	"path"
	"strings"
	"testing"
	"time"
)

func TestAPI_ClientPutGetDelete(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	kv := c.KV()

	s.WaitForSerfCheck(t)
	// Get a get without a key
	key := testKey()
	pair, _, err := kv.Get(key, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pair != nil {
		t.Fatalf("unexpected value: %#v", pair)
	}

	value := []byte("test")

	// Put a key that begins with a '/', this should fail
	invalidKey := "/test"
	p := &KVPair{Key: invalidKey, Flags: 42, Value: value}
	if _, err := kv.Put(p, nil); err == nil {
		t.Fatalf("Invalid key not detected: %s", invalidKey)
	}

	// Put the key
	p = &KVPair{Key: key, Flags: 42, Value: value}
	if _, err := kv.Put(p, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Get should work
	pair, meta, err := kv.Get(key, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pair == nil {
		t.Fatalf("expected value: %#v", pair)
	}
	if !bytes.Equal(pair.Value, value) {
		t.Fatalf("unexpected value: %#v", pair)
	}
	if pair.Flags != 42 {
		t.Fatalf("unexpected value: %#v", pair)
	}
	if meta.LastIndex == 0 {
		t.Fatalf("unexpected value: %#v", meta)
	}

	// Delete
	if _, err := kv.Delete(key, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Get should fail
	pair, _, err = kv.Get(key, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pair != nil {
		t.Fatalf("unexpected value: %#v", pair)
	}
}

func TestAPI_ClientList_DeleteRecurse(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	kv := c.KV()

	// Generate some test keys
	prefix := testKey()
	var keys []string
	for i := 0; i < 100; i++ {
		keys = append(keys, path.Join(prefix, testKey()))
	}

	// Set values
	value := []byte("test")
	for _, key := range keys {
		p := &KVPair{Key: key, Value: value}
		if _, err := kv.Put(p, nil); err != nil {
			t.Fatalf("err: %v", err)
		}
	}

	// List the values
	pairs, meta, err := kv.List(prefix, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(pairs) != len(keys) {
		t.Fatalf("got %d keys", len(pairs))
	}
	for _, pair := range pairs {
		if !bytes.Equal(pair.Value, value) {
			t.Fatalf("unexpected value: %#v", pair)
		}
	}
	if meta.LastIndex == 0 {
		t.Fatalf("unexpected value: %#v", meta)
	}

	// Delete all
	if _, err := kv.DeleteTree(prefix, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	// List the values
	pairs, _, err = kv.List(prefix, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(pairs) != 0 {
		t.Fatalf("got %d keys", len(pairs))
	}
}

func TestAPI_ClientDeleteCAS(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	kv := c.KV()

	// Put the key
	key := testKey()
	value := []byte("test")
	p := &KVPair{Key: key, Value: value}
	if work, _, err := kv.CAS(p, nil); err != nil {
		t.Fatalf("err: %v", err)
	} else if !work {
		t.Fatalf("CAS failure")
	}

	// Get should work
	pair, meta, err := kv.Get(key, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pair == nil {
		t.Fatalf("expected value: %#v", pair)
	}
	if meta.LastIndex == 0 {
		t.Fatalf("unexpected value: %#v", meta)
	}

	// CAS update with bad index
	p.ModifyIndex = 1
	if work, _, err := kv.DeleteCAS(p, nil); err != nil {
		t.Fatalf("err: %v", err)
	} else if work {
		t.Fatalf("unexpected CAS")
	}

	// CAS update with valid index
	p.ModifyIndex = meta.LastIndex
	if work, _, err := kv.DeleteCAS(p, nil); err != nil {
		t.Fatalf("err: %v", err)
	} else if !work {
		t.Fatalf("unexpected CAS failure")
	}
}

func TestAPI_ClientCAS(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	kv := c.KV()

	// Put the key
	key := testKey()
	value := []byte("test")
	p := &KVPair{Key: key, Value: value}
	if work, _, err := kv.CAS(p, nil); err != nil {
		t.Fatalf("err: %v", err)
	} else if !work {
		t.Fatalf("CAS failure")
	}

	// Get should work
	pair, meta, err := kv.Get(key, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pair == nil {
		t.Fatalf("expected value: %#v", pair)
	}
	if meta.LastIndex == 0 {
		t.Fatalf("unexpected value: %#v", meta)
	}

	// CAS update with bad index
	newVal := []byte("foo")
	p.Value = newVal
	p.ModifyIndex = 1
	if work, _, err := kv.CAS(p, nil); err != nil {
		t.Fatalf("err: %v", err)
	} else if work {
		t.Fatalf("unexpected CAS")
	}

	// CAS update with valid index
	p.ModifyIndex = meta.LastIndex
	if work, _, err := kv.CAS(p, nil); err != nil {
		t.Fatalf("err: %v", err)
	} else if !work {
		t.Fatalf("unexpected CAS failure")
	}
}

func TestAPI_ClientWatchGet(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	kv := c.KV()

	s.WaitForSerfCheck(t)
	// Get a get without a key
	key := testKey()
	pair, meta, err := kv.Get(key, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pair != nil {
		t.Fatalf("unexpected value: %#v", pair)
	}
	if meta.LastIndex == 0 {
		t.Fatalf("unexpected value: %#v", meta)
	}

	// Put the key
	value := []byte("test")
	doneCh := make(chan struct{})
	go func() {
		kv := c.KV()

		time.Sleep(100 * time.Millisecond)
		p := &KVPair{Key: key, Flags: 42, Value: value}
		if _, err := kv.Put(p, nil); err != nil {
			t.Fatalf("err: %v", err)
		}
		doneCh <- struct{}{}
	}()

	// Get should work
	options := &QueryOptions{WaitIndex: meta.LastIndex}
	pair, meta2, err := kv.Get(key, options)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pair == nil {
		t.Fatalf("expected value: %#v", pair)
	}
	if !bytes.Equal(pair.Value, value) {
		t.Fatalf("unexpected value: %#v", pair)
	}
	if pair.Flags != 42 {
		t.Fatalf("unexpected value: %#v", pair)
	}
	if meta2.LastIndex <= meta.LastIndex {
		t.Fatalf("unexpected value: %#v", meta2)
	}

	// Block until put finishes to avoid a race between it and deferred s.Stop()
	<-doneCh
}

func TestAPI_ClientWatchList(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	kv := c.KV()

	// Get a get without a key
	prefix := testKey()
	key := path.Join(prefix, testKey())
	pairs, meta, err := kv.List(prefix, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(pairs) != 0 {
		t.Fatalf("unexpected value: %#v", pairs)
	}
	if meta.LastIndex == 0 {
		t.Fatalf("unexpected value: %#v", meta)
	}

	// Put the key
	value := []byte("test")
	doneCh := make(chan struct{})
	go func() {
		kv := c.KV()

		time.Sleep(100 * time.Millisecond)
		p := &KVPair{Key: key, Flags: 42, Value: value}
		if _, err := kv.Put(p, nil); err != nil {
			t.Fatalf("err: %v", err)
		}
		doneCh <- struct{}{}
	}()

	// Get should work
	options := &QueryOptions{WaitIndex: meta.LastIndex}
	pairs, meta2, err := kv.List(prefix, options)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected value: %#v", pairs)
	}
	if !bytes.Equal(pairs[0].Value, value) {
		t.Fatalf("unexpected value: %#v", pairs)
	}
	if pairs[0].Flags != 42 {
		t.Fatalf("unexpected value: %#v", pairs)
	}
	if meta2.LastIndex <= meta.LastIndex {
		t.Fatalf("unexpected value: %#v", meta2)
	}

	// Block until put finishes to avoid a race between it and deferred s.Stop()
	<-doneCh
}

func TestAPI_ClientKeys_DeleteRecurse(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	kv := c.KV()

	// Generate some test keys
	prefix := testKey()
	var keys []string
	for i := 0; i < 100; i++ {
		keys = append(keys, path.Join(prefix, testKey()))
	}

	// Set values
	value := []byte("test")
	for _, key := range keys {
		p := &KVPair{Key: key, Value: value}
		if _, err := kv.Put(p, nil); err != nil {
			t.Fatalf("err: %v", err)
		}
	}

	// List the values
	out, meta, err := kv.Keys(prefix, "", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != len(keys) {
		t.Fatalf("got %d keys", len(out))
	}
	if meta.LastIndex == 0 {
		t.Fatalf("unexpected value: %#v", meta)
	}

	// Delete all
	if _, err := kv.DeleteTree(prefix, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	// List the values
	out, _, err = kv.Keys(prefix, "", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("got %d keys", len(out))
	}
}

func TestAPI_ClientAcquireRelease(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	session := c.Session()
	kv := c.KV()

	// Make a session
	id, _, err := session.CreateNoChecks(nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer session.Destroy(id, nil)

	// Acquire the key
	key := testKey()
	value := []byte("test")
	p := &KVPair{Key: key, Value: value, Session: id}
	if work, _, err := kv.Acquire(p, nil); err != nil {
		t.Fatalf("err: %v", err)
	} else if !work {
		t.Fatalf("Lock failure")
	}

	// Get should work
	pair, meta, err := kv.Get(key, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pair == nil {
		t.Fatalf("expected value: %#v", pair)
	}
	if pair.LockIndex != 1 {
		t.Fatalf("Expected lock: %v", pair)
	}
	if pair.Session != id {
		t.Fatalf("Expected lock: %v", pair)
	}
	if meta.LastIndex == 0 {
		t.Fatalf("unexpected value: %#v", meta)
	}

	// Release
	if work, _, err := kv.Release(p, nil); err != nil {
		t.Fatalf("err: %v", err)
	} else if !work {
		t.Fatalf("Release fail")
	}

	// Get should work
	pair, meta, err = kv.Get(key, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pair == nil {
		t.Fatalf("expected value: %#v", pair)
	}
	if pair.LockIndex != 1 {
		t.Fatalf("Expected lock: %v", pair)
	}
	if pair.Session != "" {
		t.Fatalf("Expected unlock: %v", pair)
	}
	if meta.LastIndex == 0 {
		t.Fatalf("unexpected value: %#v", meta)
	}
}

func TestAPI_KVClientTxn(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	session := c.Session()
	kv := c.KV()

	// Make a session.
	id, _, err := session.CreateNoChecks(nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer session.Destroy(id, nil)

	// Acquire and get the key via a transaction, but don't supply a valid
	// session.
	key := testKey()
	value := []byte("test")
	txn := KVTxnOps{
		&KVTxnOp{
			Verb:  KVLock,
			Key:   key,
			Value: value,
		},
		&KVTxnOp{
			Verb: KVGet,
			Key:  key,
		},
	}
	ok, ret, _, err := kv.Txn(txn, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	} else if ok {
		t.Fatalf("transaction should have failed")
	}

	if ret == nil || len(ret.Errors) != 2 || len(ret.Results) != 0 {
		t.Fatalf("bad: %v", ret)
	}
	if ret.Errors[0].OpIndex != 0 ||
		!strings.Contains(ret.Errors[0].What, "missing session") ||
		!strings.Contains(ret.Errors[1].What, "doesn't exist") {
		t.Fatalf("bad: %v", ret.Errors[0])
	}

	// Now poke in a real session and try again.
	txn[0].Session = id
	ok, ret, _, err = kv.Txn(txn, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	} else if !ok {
		t.Fatalf("transaction failure")
	}

	if ret == nil || len(ret.Errors) != 0 || len(ret.Results) != 2 {
		t.Fatalf("bad: %v", ret)
	}
	for i, result := range ret.Results {
		var expected []byte
		if i == 1 {
			expected = value
		}

		if result.Key != key ||
			!bytes.Equal(result.Value, expected) ||
			result.Session != id ||
			result.LockIndex != 1 {
			t.Fatalf("bad: %v", result)
		}
	}

	// Run a read-only transaction.
	txn = KVTxnOps{
		&KVTxnOp{
			Verb: KVGet,
			Key:  key,
		},
	}
	ok, ret, _, err = kv.Txn(txn, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	} else if !ok {
		t.Fatalf("transaction failure")
	}

	if ret == nil || len(ret.Errors) != 0 || len(ret.Results) != 1 {
		t.Fatalf("bad: %v", ret)
	}
	for _, result := range ret.Results {
		if result.Key != key ||
			!bytes.Equal(result.Value, value) ||
			result.Session != id ||
			result.LockIndex != 1 {
			t.Fatalf("bad: %v", result)
		}
	}

	// Sanity check using the regular GET API.
	pair, meta, err := kv.Get(key, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pair == nil {
		t.Fatalf("expected value: %#v", pair)
	}
	if pair.LockIndex != 1 {
		t.Fatalf("Expected lock: %v", pair)
	}
	if pair.Session != id {
		t.Fatalf("Expected lock: %v", pair)
	}
	if meta.LastIndex == 0 {
		t.Fatalf("unexpected value: %#v", meta)
	}
}
