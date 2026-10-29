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

package nativemetadata

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/metadata"
)

// value returns a fresh pointer, so equality never relies on pointer identity.
func value(help string) *metadata.Metadata {
	return &metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: strings.Clone(help)}
}

func points(spec string) []Point {
	var out []Point
	for field := range strings.FieldsSeq(spec) {
		var help string
		var from int64
		if _, err := fmt.Sscanf(strings.Replace(field, "@", " ", 1), "%s %d", &help, &from); err != nil {
			panic(err)
		}
		out = append(out, Point{EffectiveFrom: from, Metadata: value(help)})
	}
	return out
}

func format(h *History) string {
	var fields []string
	for _, p := range h.AppendPoints(nil) {
		fields = append(fields, fmt.Sprintf("%s@%d", p.Metadata.Help, p.EffectiveFrom))
	}
	return strings.Join(fields, " ")
}

func history(spec string) History {
	var h History
	h.Replace(points(spec))
	return h
}

func TestHistory(t *testing.T) {
	t.Run("groups merge in one call", func(t *testing.T) {
		grouped := history("B@200")
		grouped.Merge(points("B@150 C@180"))
		require.Equal(t, "B@150 C@180 B@200", format(&grouped))
		separate := history("B@200")
		for _, p := range points("B@150 C@180") {
			separate.Merge([]Point{p})
		}
		require.Equal(t, "B@150 C@180", format(&separate), "point-wise merges lose B@200")
	})
	t.Run("merge order matters", func(t *testing.T) {
		first := history("A@100")
		first.Merge(points("B@150 B@300"))
		first.Merge(points("C@200"))
		require.Equal(t, "A@100 B@150 C@200", format(&first))
		second := history("A@100")
		second.Merge(points("C@200"))
		second.Merge(points("B@150 B@300"))
		require.Equal(t, "A@100 B@150 C@200 B@300", format(&second))
	})
	t.Run("eviction", func(t *testing.T) {
		h := history("A@10 B@20 C@30 D@40 E@50")
		delta, evictions := h.Merge(points("F@60"))
		require.Equal(t, 0, delta)
		require.Equal(t, 1, evictions)
		require.Equal(t, "B@20 C@30 D@40 E@50 F@60", format(&h))
		delta, evictions = h.Merge(points("A@5 G@70"))
		require.Equal(t, 0, delta)
		require.Equal(t, 2, evictions)
		require.Equal(t, "C@30 D@40 E@50 F@60 G@70", format(&h))
	})
	t.Run("version selection", func(t *testing.T) {
		h := history("A@100 B@200")
		for timestamp, want := range map[int64]string{math.MinInt64: "", 99: "", 100: "A", 199: "A", 200: "B", math.MaxInt64: "B"} {
			got := h.At(timestamp)
			if want == "" {
				require.Nil(t, got, timestamp)
				continue
			}
			require.Equal(t, want, got.Help, timestamp)
		}
		var empty History
		require.Nil(t, empty.At(math.MaxInt64))
		require.Zero(t, empty.Len())
		require.Equal(t, 2, h.Len())
	})
	t.Run("replace coalesces adjacent values", func(t *testing.T) {
		h := history("A@1 A@2 B@3 B@4 A@5")
		require.Equal(t, "A@1 B@3 A@5", format(&h))
		h.Replace(nil)
		require.Nil(t, h.Metadata)
		require.Nil(t, h.Older)
		require.Empty(t, format(&h))
	})
	t.Run("equality", func(t *testing.T) {
		a := value("a")
		require.True(t, Equal(a, value("a")))
		require.True(t, Equal(nil, nil))
		require.False(t, Equal(a, nil))
		require.False(t, Equal(a, value("b")))
	})
}
