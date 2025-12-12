// Copyright 2025 The Prometheus Authors
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

//go:build slicelabels

package labels

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var expectedSizeOfLabels = []uint64{ // Values must line up with testCaseLabels.
	72,
	0,
	97,
	326,
	327,
	549,
}

var expectedByteSize = expectedSizeOfLabels // They are identical

func TestScratchBuilderAdd_Strings(t *testing.T) {
	t.Run("safe", func(t *testing.T) {
		n := []byte("__name__")
		v := []byte("metric1")

		l := NewScratchBuilder(0)
		l.Add(yoloString(n), yoloString(v))
		ret := l.Labels()

		// For slicelabels, in default mode strings are reused, so modifying the
		// intput will cause `ret` labels to change too.
		n[1] = byte('?')
		v[2] = byte('?')

		require.Empty(t, ret.Get("__name__"))
		require.Equal(t, "me?ric1", ret.Get("_?name__"))
	})
	t.Run("unsafe", func(t *testing.T) {
		n := []byte("__name__")
		v := []byte("metric1")

		l := NewScratchBuilder(0)
		l.SetUnsafeAdd(true)
		l.Add(yoloString(n), yoloString(v))
		ret := l.Labels()

		// Changing input strings should be now safe, because we marked adds as unsafe.
		n[1] = byte('?')
		v[2] = byte('?')

		require.Equal(t, "metric1", ret.Get("__name__"))
	})
}

/*
	export bench=unsafe && go test -tags=slicelabels \
	  -run '^$' -bench '^BenchmarkScratchBuilderUnsafeAdd' \
	  -benchtime 5s -count 6 -cpu 2 -timeout 999m \
	  | tee ${bench}.txt
*/
func BenchmarkScratchBuilderUnsafeAdd(b *testing.B) {
	l := NewScratchBuilder(0)
	l.SetUnsafeAdd(true)

	b.ReportAllocs()

	for b.Loop() {
		l.Add("__name__", "metric1")
		l.add = l.add[:0] // Reset slice so add can be repeated without side effects.
	}
}
