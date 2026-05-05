// Copyright 2026 The Prometheus Authors
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

package chunkenc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/stateset"
)

// makeStateset is a test helper that builds a StateSet with the given active states.
func makeStateset(labelName string, allNames []string, activeNames ...string) *stateset.StateSet {
	active := make(map[string]struct{}, len(activeNames))
	for _, n := range activeNames {
		active[n] = struct{}{}
	}
	ss, err := stateset.FromActive(labelName, allNames, active)
	if err != nil {
		panic(err)
	}
	return ss
}

func TestStateSetChunk_RoundTrip(t *testing.T) {
	allNames := []string{"Failed", "Pending", "Running", "Succeeded", "Unknown"}

	samples := []*stateset.StateSet{
		makeStateset("phase", allNames, "Pending"),
		makeStateset("phase", allNames, "Running"),
		makeStateset("phase", allNames, "Running"),
		makeStateset("phase", allNames, "Succeeded"),
		makeStateset("phase", allNames),
	}
	timestamps := []int64{1000, 2000, 3000, 4000, 5000}

	c := NewStateSetChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	for i, ss := range samples {
		newChunk, newApp, err := app.AppendStateset(timestamps[i], ss)
		require.NoError(t, err)
		require.Nil(t, newChunk, "unexpected recode at sample %d", i)
		app = newApp
	}

	require.Equal(t, len(samples), c.NumSamples())

	it := c.Iterator(nil)
	for i, want := range samples {
		vt := it.Next()
		require.Equal(t, ValStateset, vt, "sample %d: unexpected value type", i)
		gotT, gotSS := it.AtStateset(nil)
		require.Equal(t, timestamps[i], gotT, "sample %d: timestamp mismatch", i)
		require.Equal(t, want.LabelName, gotSS.LabelName, "sample %d: LabelName mismatch", i)
		require.Equal(t, want.Names, gotSS.Names, "sample %d: Names mismatch", i)
		require.Equal(t, want.Values, gotSS.Values, "sample %d: Values mismatch", i)
	}
	require.Equal(t, ValNone, it.Next())
	require.NoError(t, it.Err())
}

func TestStateSetChunk_ReuseIterator(t *testing.T) {
	allNames := []string{"healthy", "unhealthy"}
	c := NewStateSetChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	for i := range int64(5) {
		_, app, err = app.AppendStateset(i*1000, makeStateset("status", allNames, "healthy"))
		require.NoError(t, err)
	}

	// Reuse the same iterator object.
	var it Iterator
	it = c.Iterator(it)
	count := 0
	for it.Next() != ValNone {
		count++
	}
	require.Equal(t, 5, count)
}

func TestStateSetChunk_Recode_OnNewStateName(t *testing.T) {
	c := NewStateSetChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	ss1 := makeStateset("phase", []string{"Pending", "Running"}, "Running")
	newChunk, newApp, err := app.AppendStateset(1000, ss1)
	require.NoError(t, err)
	require.Nil(t, newChunk)
	app = newApp

	// Different state names → recode.
	ss2 := makeStateset("phase", []string{"Failed", "Pending", "Running"}, "Running")
	newChunk, newApp, err = app.AppendStateset(2000, ss2)
	require.NoError(t, err)
	require.NotNil(t, newChunk, "expected a new chunk on state name change")
	require.Equal(t, EncStateset, newChunk.Encoding())
	_ = newApp
}

func TestStateSetChunk_Seek(t *testing.T) {
	allNames := []string{"a", "b", "c"}
	c := NewStateSetChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	for i := range int64(10) {
		_, app, err = app.AppendStateset(i*1000, makeStateset("l", allNames, "a"))
		require.NoError(t, err)
	}

	it := c.Iterator(nil)
	vt := it.Seek(5000)
	require.Equal(t, ValStateset, vt)
	gotT, _ := it.AtStateset(nil)
	require.Equal(t, int64(5000), gotT)
}

func TestStateSetChunk_XORDelta(t *testing.T) {
	// Only one bit changes between samples — XOR delta should be very small.
	allNames := []string{"a", "b", "c", "d"}
	c := NewStateSetChunk()
	app, err := c.Appender()
	require.NoError(t, err)

	states := []string{"a", "b", "c", "d", "a"}
	for i, active := range states {
		_, app, err = app.AppendStateset(int64(i)*1000, makeStateset("l", allNames, active))
		require.NoError(t, err)
	}

	it := c.Iterator(nil)
	for i, active := range states {
		require.Equal(t, ValStateset, it.Next(), "sample %d", i)
		_, ss := it.AtStateset(nil)
		require.True(t, ss.IsActive(active), "sample %d: expected %q active, got values=%b names=%v", i, active, ss.Values, ss.Names)
	}
	require.Equal(t, ValNone, it.Next())
}

func TestStateSetChunk_EmptyChunk(t *testing.T) {
	c := NewStateSetChunk()
	require.Equal(t, 0, c.NumSamples())
	it := c.Iterator(nil)
	require.Equal(t, ValNone, it.Next())
}

func TestStateSetChunk_ValidEncoding(t *testing.T) {
	require.True(t, IsValidEncoding(EncStateset))
}

func TestStateSetChunk_PoolRoundTrip(t *testing.T) {
	pool := NewPool()
	c := NewStateSetChunk()
	app, err := c.Appender()
	require.NoError(t, err)
	_, _, err = app.AppendStateset(1000, makeStateset("l", []string{"a", "b"}, "a"))
	require.NoError(t, err)

	// Get a chunk from the pool using the encoded bytes.
	c2, err := pool.Get(EncStateset, c.Bytes())
	require.NoError(t, err)
	require.Equal(t, 1, c2.NumSamples())

	// Return to pool.
	require.NoError(t, pool.Put(c2))
}
