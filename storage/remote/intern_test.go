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

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestIntern(t *testing.T) {
	interner := newPool(true)
	testString := "TestIntern_DeleteRef"
	ref := chunks.HeadSeriesRef(1234)

	lset := labels.FromStrings("name", testString)
	interner.intern(ref, lset)
	interned, ok := interner.pool[ref]

	require.True(t, ok)
	require.Equal(t, lset, interned.lset)
	require.Equal(t, int64(1), interned.refs.Load(), fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs.Load()))
}

func TestIntern_MultiRef(t *testing.T) {
	interner := newPool(true)
	testString := "TestIntern_DeleteRef"
	ref := chunks.HeadSeriesRef(1234)

	lset := labels.FromStrings("name", testString)
	interner.intern(ref, lset)
	interned, ok := interner.pool[ref]

	require.True(t, ok)
	require.Equal(t, lset, interned.lset)
	require.Equal(t, int64(1), interned.refs.Load(), fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs.Load()))

	interner.intern(ref, lset)
	interned, ok = interner.pool[ref]

	require.NotNil(t, interned)
	require.Equal(t, int64(2), interned.refs.Load(), fmt.Sprintf("expected refs to be 2 but it was %d", interned.refs.Load()))
}

func TestIntern_DeleteRef(t *testing.T) {
	interner := newPool(true)
	testString := "TestIntern_DeleteRef"
	ref := chunks.HeadSeriesRef(1234)
	interner.intern(ref, labels.FromStrings("name", testString))
	interned, ok := interner.pool[ref]

	require.NotNil(t, interned)
	require.Equal(t, int64(1), interned.refs.Load(), fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs.Load()))

	interner.release(ref)
	_, ok = interner.pool[ref]
	require.False(t, ok)
}

func TestIntern_MultiRef_Concurrent(t *testing.T) {
	interner := newPool(true)
	testString := "TestIntern_MultiRef_Concurrent"
	ref := chunks.HeadSeriesRef(1234)

	interner.intern(ref, labels.FromStrings("name", testString))
	interned, ok := interner.pool[ref]

	require.NotNil(t, interned)
	require.Equal(t, int64(1), interned.refs.Load(), fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs.Load()))

	go interner.release(ref)

	interner.intern(ref, labels.FromStrings("name", testString))

	time.Sleep(time.Millisecond)

	interner.mtx.RLock()
	interned, ok = interner.pool[ref]
	interner.mtx.RUnlock()
	require.True(t, ok)
	require.Equal(t, int64(1), interned.refs.Load(), fmt.Sprintf("expected refs to be 1 but it was %d", interned.refs.Load()))
}
