// Copyright The Prometheus Authors
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
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestIntern(t *testing.T) {
	interner := newPool(nil)
	testString := "TestIntern"
	interner.intern(testString)
	interned, ok := interner.pool[testString]

	require.True(t, ok)
	require.Equalf(t, int64(1), interned.refs.Load(), "expected refs to be 1 but it was %d", interned.refs.Load())
}

func TestIntern_MultiRef(t *testing.T) {
	interner := newPool(nil)
	testString := "TestIntern_MultiRef"

	interner.intern(testString)
	interned, ok := interner.pool[testString]

	require.True(t, ok)
	require.Equalf(t, int64(1), interned.refs.Load(), "expected refs to be 1 but it was %d", interned.refs.Load())

	interner.intern(testString)
	interned, ok = interner.pool[testString]

	require.True(t, ok)
	require.Equalf(t, int64(2), interned.refs.Load(), "expected refs to be 2 but it was %d", interned.refs.Load())
}

func TestIntern_DeleteRef(t *testing.T) {
	interner := newPool(nil)
	testString := "TestIntern_DeleteRef"

	interner.intern(testString)
	interned, ok := interner.pool[testString]

	require.True(t, ok)
	require.Equalf(t, int64(1), interned.refs.Load(), "expected refs to be 1 but it was %d", interned.refs.Load())

	interner.release(testString)
	_, ok = interner.pool[testString]
	require.False(t, ok)
}

func TestIntern_MultiRef_Concurrent(t *testing.T) {
	interner := newPool(nil)
	testString := "TestIntern_MultiRef_Concurrent"

	interner.intern(testString)
	interned, ok := interner.pool[testString]
	require.True(t, ok)
	require.Equal(t, int64(1), interned.refs.Load(), "wrong interned refs count")

	for range 1000 {
		released := make(chan struct{})
		go func() {
			interner.release(testString)
			close(released)
		}()
		interner.intern(testString)
		<-released
	}

	interner.mtx.RLock()
	interned, ok = interner.pool[testString]
	interner.mtx.RUnlock()
	require.True(t, ok)
	require.Equal(t, int64(1), interned.refs.Load(), "wrong interned refs count")
}

func TestIntern_ConcurrentPoolsShareOneCounter(t *testing.T) {
	const pools = 4
	reg := prometheus.NewPedanticRegistry()

	created := make([]*pool, pools)
	var wg sync.WaitGroup
	for i := range pools {
		wg.Go(func() {
			created[i] = newPool(reg)
		})
	}
	wg.Wait()

	// Only one pool registers the counter. The others reuse it, so a release of
	// a string that was never interned counts on the same series.
	for _, p := range created {
		p.release("TestIntern_ConcurrentPoolsShareOneCounter")
	}

	for _, p := range created {
		require.Equal(t, float64(pools), client_testutil.ToFloat64(p.noReferenceReleases))
	}
	require.Equal(t, 1, client_testutil.CollectAndCount(created[0].noReferenceReleases))
}
