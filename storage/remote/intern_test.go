// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Inspired / copied / modified from https://gitlab.com/cznic/strutil/blob/master/strutil.go,
// which is MIT licensed, so:
//
// Copyright (c) 2014 The strutil Authors. All rights reserved.

package remote

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestIntern(t *testing.T) {
	interner := newPool()
	testString := "TestIntern"
	interner.intern(testString)
	interned, ok := interner.pool[testString]

	testutil.Equals(t, true, ok)
	testutil.Assert(t, interned.refs.Load() == 1, fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs))
}

func TestIntern_MultiRef(t *testing.T) {
	mgr := &QueueManager{interner: newPool()}
	testString := "TestIntern_MultiRef"

	mgr.interner.intern(testString)
	interned, ok := mgr.interner.pool[testString]

	testutil.Equals(t, true, ok)
	testutil.Assert(t, interned.refs.Load() == 1, fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs))

	mgr.interner.intern(testString)
	interned, ok = mgr.interner.pool[testString]

	testutil.Equals(t, true, ok)
	testutil.Assert(t, interned.refs.Load() == 2, fmt.Sprintf("expected refs to be 2 but it was %d", interned.refs))
}

func TestIntern_DeleteRef(t *testing.T) {
	mgr := &QueueManager{interner: newPool()}
	testString := "TestIntern_DeleteRef"

	mgr.interner.intern(testString)
	interned, ok := mgr.interner.pool[testString]

	testutil.Equals(t, true, ok)
	testutil.Assert(t, interned.refs.Load() == 1, fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs))

	mgr.interner.release(testString)
	_, ok = mgr.interner.pool[testString]
	testutil.Equals(t, false, ok)
}

func TestIntern_MultiRef_Concurrent(t *testing.T) {
	mgr := &QueueManager{interner: newPool()}
	testString := "TestIntern_MultiRef_Concurrent"

	mgr.interner.intern(testString)
	interned, ok := mgr.interner.pool[testString]
	testutil.Equals(t, true, ok)
	testutil.Assert(t, interned.refs.Load() == 1, fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs))

	go mgr.interner.release(testString)

	mgr.interner.intern(testString)

	time.Sleep(time.Millisecond)

	mgr.interner.mtx.RLock()
	interned, ok = mgr.interner.pool[testString]
	mgr.interner.mtx.RUnlock()
	testutil.Equals(t, true, ok)
	testutil.Assert(t, interned.refs.Load() == 1, fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs))
}
