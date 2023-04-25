// Copyright 2023 The Prometheus Authors
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

// If we decide to employ this auto generation of markdown documentation for
// amtool and alertmanager, this package could potentially be moved to
// prometheus/common. However, it is crucial to note that this functionality is
// tailored specifically to the way in which the Prometheus documentation is
// rendered, and should be avoided for use by third-party users.

package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDictCache(t *testing.T) {
	dc := NewDictCache()
	dc.RunGC()
	values := []string{
		"abc",
		"123",
		"abc",
		"edf",
		"jdf",
		"123",
	}
	keys := make([]int64, len(values))
	for i := 0; i < len(values); i++ {
		v := dc.Get(values[i])
		keys[i] = v
	}
	for i := 0; i < len(keys); i++ {
		v, ok := dc.Value(keys[i])
		if !ok || v != values[i] {
			t.Fatal("not match")
		}
	}
	dc.del(values)
	for _, db := range dc.dbv {
		if len(db) > 0 {
			t.Fatal("not empty dict cache")
		}
	}
	require.False(t, dc.stopped, "dict gc thread not running well")
	dc.Stop()
	require.True(t, dc.stopped, "dict gc thread not stop well")
}
