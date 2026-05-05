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

package textparse

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/stateset"
)

// parseAllStatesets drives p until EOF and returns the sequence of entries.
// For EntryStateset it captures the parsed stateset. For EntrySeries it records
// the float value so callers can verify pass-through of non-stateset series.
type statesetEntry struct {
	typ Entry
	// EntrySeries fields
	seriesName string
	seriesVal  float64
	// EntryStateset fields
	ssName   string
	ssDimKey string // sorted dim-label string
	ss       *stateset.StateSet
}

func driveStateSetParser(t *testing.T, input string, opts ParserOptions) []statesetEntry {
	t.Helper()
	st := labels.NewSymbolTable()
	p, err := New([]byte(input), "application/openmetrics-text;version=1.0.0", st, opts)
	require.NoError(t, err)
	var out []statesetEntry
	var lset labels.Labels
	for {
		et, err := p.Next()
		if isEOF(err) {
			break
		}
		require.NoError(t, err)
		switch et {
		case EntryStateset:
			b, _, ss := p.Stateset()
			p.Labels(&lset)
			out = append(out, statesetEntry{
				typ:      EntryStateset,
				ssName:   string(b),
				ssDimKey: lset.String(),
				ss:       ss,
			})
		case EntrySeries:
			b, _, v := p.Series()
			out = append(out, statesetEntry{
				typ:        EntrySeries,
				seriesName: string(b),
				seriesVal:  v,
			})
		case EntryType, EntryHelp, EntryUnit, EntryComment:
			// ignore metadata
		}
	}
	return out
}

func isEOF(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "EOF" || err.Error() == "unexpected EOF"
}

// statesetActiveNames returns the names of active states in sorted order.
func statesetActiveNames(ss *stateset.StateSet) []string {
	return ss.ActiveNames()
}

func TestStateSetParser_Basic(t *testing.T) {
	input := `# HELP foo A stateset.
# TYPE foo stateset
foo{foo="running"} 1
foo{foo="stopped"} 0
# EOF
`
	opts := ParserOptions{ConvertStatesets: true}
	entries := driveStateSetParser(t, input, opts)

	require.Len(t, entries, 1, "expected exactly one EntryStateset")
	e := entries[0]
	require.Equal(t, EntryStateset, e.typ)
	require.Equal(t, "foo", e.ssName)
	require.Equal(t, "foo", e.ss.LabelName)
	require.Equal(t, []string{"running", "stopped"}, e.ss.Names, "states must be sorted lexicographically")
	require.Equal(t, []string{"running"}, statesetActiveNames(e.ss))
	require.Equal(t, "{}", e.ssDimKey, "no dimension labels for plain foo stateset")
}

func TestStateSetParser_MultipleDimensions(t *testing.T) {
	// Two stateset groups differentiated by a "job" dimension label.
	input := `# HELP phase Phase stateset.
# TYPE phase stateset
phase{job="a",phase="failed"} 0
phase{job="a",phase="running"} 1
phase{job="b",phase="failed"} 1
phase{job="b",phase="running"} 0
# EOF
`
	opts := ParserOptions{ConvertStatesets: true}
	entries := driveStateSetParser(t, input, opts)

	require.Len(t, entries, 2, "expected two EntryStateset entries (one per job dimension)")

	// Both must be statesets named "phase".
	for _, e := range entries {
		require.Equal(t, EntryStateset, e.typ)
		require.Equal(t, "phase", e.ssName)
		require.Equal(t, []string{"failed", "running"}, e.ss.Names)
	}

	// Find job="a" and job="b" entries.
	var entryA, entryB *statesetEntry
	for i := range entries {
		switch entries[i].ssDimKey {
		case `{job="a"}`:
			entryA = &entries[i]
		case `{job="b"}`:
			entryB = &entries[i]
		}
	}
	require.NotNil(t, entryA, "expected entry for job=a")
	require.NotNil(t, entryB, "expected entry for job=b")

	require.Equal(t, []string{"running"}, statesetActiveNames(entryA.ss))
	require.Equal(t, []string{"failed"}, statesetActiveNames(entryB.ss))
}

func TestStateSetParser_MultipleStatesets(t *testing.T) {
	// Two independent stateset families in one scrape.
	input := `# TYPE color stateset
color{color="blue"} 1
color{color="red"} 0
# TYPE phase stateset
phase{phase="pending"} 0
phase{phase="running"} 1
phase{phase="stopped"} 0
# EOF
`
	opts := ParserOptions{ConvertStatesets: true}
	entries := driveStateSetParser(t, input, opts)

	require.Len(t, entries, 2)

	byName := map[string]*statesetEntry{}
	for i := range entries {
		byName[entries[i].ssName] = &entries[i]
	}

	color := byName["color"]
	require.NotNil(t, color)
	require.Equal(t, []string{"blue", "red"}, color.ss.Names)
	require.Equal(t, []string{"blue"}, statesetActiveNames(color.ss))

	phase := byName["phase"]
	require.NotNil(t, phase)
	require.Equal(t, []string{"pending", "running", "stopped"}, phase.ss.Names)
	require.Equal(t, []string{"running"}, statesetActiveNames(phase.ss))
}

func TestStateSetParser_StatesAreSortedLexicographically(t *testing.T) {
	// States appear in reverse order in the input; output must be sorted.
	input := `# TYPE status stateset
status{status="stopped"} 0
status{status="running"} 1
status{status="failed"} 0
# EOF
`
	opts := ParserOptions{ConvertStatesets: true}
	entries := driveStateSetParser(t, input, opts)

	require.Len(t, entries, 1)
	require.Equal(t, []string{"failed", "running", "stopped"}, entries[0].ss.Names)
}

func TestStateSetParser_DisabledByOption(t *testing.T) {
	// When ConvertStatesets is false the raw float series should pass through.
	input := `# TYPE foo stateset
foo{foo="running"} 1
foo{foo="stopped"} 0
# EOF
`
	opts := ParserOptions{ConvertStatesets: false}
	entries := driveStateSetParser(t, input, opts)

	// Should be two plain float series, no stateset entries.
	for _, e := range entries {
		require.Equal(t, EntrySeries, e.typ, "expected plain series when ConvertStatesets=false")
	}
	require.Len(t, entries, 2)
}

func TestStateSetParser_NonStatesetSeriesPassThrough(t *testing.T) {
	// Mix of a gauge and a stateset in the same scrape.
	input := `# TYPE requests counter
requests_total 42
# TYPE phase stateset
phase{phase="running"} 1
phase{phase="stopped"} 0
# EOF
`
	opts := ParserOptions{ConvertStatesets: true}
	entries := driveStateSetParser(t, input, opts)

	var series []statesetEntry
	var ss []statesetEntry
	for _, e := range entries {
		if e.typ == EntrySeries {
			series = append(series, e)
		} else {
			ss = append(ss, e)
		}
	}

	require.Len(t, series, 1, "gauge should pass through as EntrySeries")
	require.Equal(t, float64(42), series[0].seriesVal)

	require.Len(t, ss, 1)
	require.Equal(t, "phase", ss[0].ssName)
}

func TestStateSetParser_AllActiveAllInactive(t *testing.T) {
	input := `# TYPE s stateset
s{s="a"} 1
s{s="b"} 1
s{s="c"} 1
# EOF
`
	opts := ParserOptions{ConvertStatesets: true}
	entries := driveStateSetParser(t, input, opts)
	require.Len(t, entries, 1)
	require.Equal(t, []string{"a", "b", "c"}, statesetActiveNames(entries[0].ss))

	input2 := `# TYPE s stateset
s{s="a"} 0
s{s="b"} 0
# EOF
`
	entries2 := driveStateSetParser(t, input2, opts)
	require.Len(t, entries2, 1)
	require.Empty(t, statesetActiveNames(entries2[0].ss))
}
