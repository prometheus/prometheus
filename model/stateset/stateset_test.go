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

package stateset

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		ss := &StateSet{
			LabelName: "phase",
			Names:     []string{"Failed", "Pending", "Running"},
			Values:    0b010, // "Pending" active
		}
		require.NoError(t, ss.Validate())
	})

	t.Run("empty label name", func(t *testing.T) {
		ss := &StateSet{Names: []string{"a"}}
		require.Error(t, ss.Validate())
	})

	t.Run("unsorted names", func(t *testing.T) {
		ss := &StateSet{LabelName: "l", Names: []string{"b", "a"}, Values: 0}
		require.Error(t, ss.Validate())
	})

	t.Run("too many states", func(t *testing.T) {
		names := make([]string, MaxStates+1)
		for i := range names {
			names[i] = string(rune('a' + i%26))
			if i >= 26 {
				names[i] += strings.Repeat("a", i/26)
			}
		}
		ss := &StateSet{LabelName: "l", Names: names}
		require.ErrorIs(t, ss.Validate(), ErrTooManyStates)
	})

	t.Run("bits beyond names", func(t *testing.T) {
		ss := &StateSet{LabelName: "l", Names: []string{"a"}, Values: 0b11}
		require.Error(t, ss.Validate())
	})

	t.Run("exactly 64 states is valid", func(t *testing.T) {
		names := make([]string, MaxStates)
		for i := range names {
			names[i] = string([]byte{byte('a' + i/26), byte('a' + i%26)})
		}
		ss := &StateSet{LabelName: "l", Names: names, Values: 1}
		require.NoError(t, ss.Validate())
	})
}

func TestIsActive(t *testing.T) {
	ss := &StateSet{
		LabelName: "phase",
		Names:     []string{"Failed", "Pending", "Running", "Succeeded", "Unknown"},
		Values:    0b00100, // "Running" is bit 2
	}
	require.True(t, ss.IsActive("Running"))
	require.False(t, ss.IsActive("Pending"))
	require.False(t, ss.IsActive("Failed"))
	require.False(t, ss.IsActive("Succeeded"))
	require.False(t, ss.IsActive("Unknown"))
	require.False(t, ss.IsActive("nonexistent"))
}

func TestIsActiveZeroStates(t *testing.T) {
	ss := &StateSet{LabelName: "l", Names: nil, Values: 0}
	require.False(t, ss.IsActive("anything"))
}

func TestActiveNames(t *testing.T) {
	ss := &StateSet{
		LabelName: "phase",
		Names:     []string{"Failed", "Pending", "Running", "Succeeded", "Unknown"},
		Values:    0b00101, // "Failed" (bit 0) and "Running" (bit 2)
	}
	got := ss.ActiveNames()
	require.Equal(t, []string{"Failed", "Running"}, got)
}

func TestActiveNamesNoneActive(t *testing.T) {
	ss := &StateSet{
		LabelName: "phase",
		Names:     []string{"a", "b", "c"},
		Values:    0,
	}
	require.Empty(t, ss.ActiveNames())
}

func TestActiveNamesAllActive(t *testing.T) {
	ss := &StateSet{
		LabelName: "phase",
		Names:     []string{"a", "b", "c"},
		Values:    0b111,
	}
	require.Equal(t, []string{"a", "b", "c"}, ss.ActiveNames())
}

func TestCopy(t *testing.T) {
	orig := &StateSet{
		LabelName: "phase",
		Names:     []string{"Running", "Stopped"},
		Values:    0b01,
	}
	cp := orig.Copy()
	require.True(t, orig.Equals(cp))

	// Mutating the copy must not affect the original.
	cp.Names[0] = "Modified"
	require.Equal(t, "Running", orig.Names[0])
}

func TestEquals(t *testing.T) {
	a := &StateSet{LabelName: "l", Names: []string{"a", "b"}, Values: 0b01}
	b := a.Copy()
	require.True(t, a.Equals(b))

	b.Values = 0b10
	require.False(t, a.Equals(b))

	b.Values = 0b01
	b.LabelName = "other"
	require.False(t, a.Equals(b))

	b.LabelName = "l"
	b.Names = []string{"a", "c"}
	require.False(t, a.Equals(b))
}

func TestNamesMatch(t *testing.T) {
	a := &StateSet{LabelName: "l", Names: []string{"a", "b"}, Values: 0b01}
	b := &StateSet{LabelName: "other", Names: []string{"a", "b"}, Values: 0b10}
	require.True(t, a.NamesMatch(b))

	c := &StateSet{LabelName: "l", Names: []string{"a", "c"}, Values: 0b01}
	require.False(t, a.NamesMatch(c))

	d := &StateSet{LabelName: "l", Names: []string{"a"}, Values: 0b01}
	require.False(t, a.NamesMatch(d))
}

func TestFromActive(t *testing.T) {
	ss, err := FromActive("phase",
		[]string{"Running", "Pending", "Failed"}, // unsorted input is fine
		map[string]struct{}{"Running": {}},
	)
	require.NoError(t, err)
	require.Equal(t, "phase", ss.LabelName)
	// Names must be sorted.
	require.Equal(t, []string{"Failed", "Pending", "Running"}, ss.Names)
	// "Running" is at index 2 after sorting.
	require.Equal(t, uint64(0b100), ss.Values)
	require.True(t, ss.IsActive("Running"))
	require.False(t, ss.IsActive("Pending"))
}

func TestFromActiveTooManyStates(t *testing.T) {
	names := make([]string, MaxStates+1)
	for i := range names {
		names[i] = string([]byte{byte('a' + i/26), byte('a' + i%26)})
	}
	_, err := FromActive("l", names, nil)
	require.ErrorIs(t, err, ErrTooManyStates)
}

func TestFromActiveNilActive(t *testing.T) {
	ss, err := FromActive("phase", []string{"a", "b"}, nil)
	require.NoError(t, err)
	require.Equal(t, uint64(0), ss.Values)
}
