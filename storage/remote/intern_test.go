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

	"github.com/stretchr/testify/require"
)

func TestIntern(t *testing.T) {
	interner := newPool()
	testString := "TestIntern"
	interner.intern(testString)
	interned, ok := interner.pool[testString]

	require.Equal(t, true, ok)
	require.Equal(t, int64(1), interned.refs.Load(), fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs.Load()))
}

func TestIntern_MultiRef(t *testing.T) {
	interner := newPool()
	testString := "TestIntern_MultiRef"

	interner.intern(testString)
	interned, ok := interner.pool[testString]

	require.Equal(t, true, ok)
	require.Equal(t, int64(1), interned.refs.Load(), fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs.Load()))

	interner.intern(testString)
	interned, ok = interner.pool[testString]

	require.Equal(t, true, ok)
	require.Equal(t, int64(2), interned.refs.Load(), fmt.Sprintf("expected refs to be 2 but it was %d", interned.refs.Load()))
}

func TestIntern_DeleteRef(t *testing.T) {
	interner := newPool()
	testString := "TestIntern_DeleteRef"

	interner.intern(testString)
	interned, ok := interner.pool[testString]

	require.Equal(t, true, ok)
	require.Equal(t, int64(1), interned.refs.Load(), fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs.Load()))

	interner.release(testString)
	_, ok = interner.pool[testString]
	require.Equal(t, false, ok)
}

func TestIntern_MultiRef_Concurrent(t *testing.T) {
	interner := newPool()
	testString := "TestIntern_MultiRef_Concurrent"

	interner.intern(testString)
	interned, ok := interner.pool[testString]
	require.Equal(t, true, ok)
	require.Equal(t, int64(1), interned.refs.Load(), fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs.Load()))

	go interner.release(testString)

	interner.intern(testString)

	time.Sleep(time.Millisecond)

	interner.mtx.RLock()
	interned, ok = interner.pool[testString]
	interner.mtx.RUnlock()
	require.Equal(t, true, ok)
	require.Equal(t, int64(1), interned.refs.Load(), fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs.Load()))
}
