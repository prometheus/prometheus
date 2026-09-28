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

package promql

import (
	"slices"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// onMatching is a parsed `on` function argument: the labels that key series
// for pairing and grouping.
type onMatching struct {
	// explicit is false for the function's default key (all labels except
	// function-specific ones such as __name__).
	explicit bool
	// labels is the sorted, de-duplicated key when explicit. It is empty for
	// "()", which gives every series the same signature.
	labels []string
}

// parseOnMatching parses an `on` argument, mirroring the binary-operator
// on(...) clause. "" selects the default key; "job,instance" or
// "(job,instance)" key on exactly those labels; "()" keys on the empty label
// set, so all series match each other. ok is false for unbalanced
// parentheses or a bare list without names (the empty set is spelled "()").
func parseOnMatching(s string) (m onMatching, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return onMatching{}, true
	}
	parens := strings.HasPrefix(s, "(")
	if parens != strings.HasSuffix(s, ")") {
		return onMatching{}, false
	}
	if parens {
		s = s[1 : len(s)-1]
	}
	if strings.ContainsAny(s, "()") {
		return onMatching{}, false
	}
	var names []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			names = append(names, p)
		}
	}
	if len(names) == 0 && !parens {
		return onMatching{}, false
	}
	// HashForLabels walks names in sorted order, so they must be sorted.
	slices.Sort(names)
	return onMatching{explicit: true, labels: slices.Compact(names)}, true
}

// onArg parses the optional `on` argument at e.Args[idx]. For a malformed
// value it adds a warning to ws and returns ok=false.
func onArg(e *parser.Call, idx int, ws *annotations.Annotations) (onMatching, bool) {
	if len(e.Args) <= idx {
		return onMatching{}, true
	}
	s := stringFromArg(e.Args[idx])
	m, ok := parseOnMatching(s)
	if !ok {
		ws.Add(annotations.NewInvalidOnLabelsWarning(s, e.Args[idx].PositionRange()))
	}
	return m, ok
}

// sig returns lb's matching signature: exactly the `on` labels when explicit,
// otherwise every label except dropLabels. buf is reused across calls to
// avoid per-series allocations.
func (m onMatching) sig(lb labels.Labels, buf []byte, dropLabels ...string) (uint64, []byte) {
	if m.explicit {
		return lb.HashForLabels(buf, m.labels...)
	}
	return lb.HashWithoutLabels(buf, dropLabels...)
}

// keep returns lb reduced to the `on` labels when explicit, so that output
// series of a group carry the group key rather than one member's labels.
// With the default key lb is returned unchanged.
func (m onMatching) keep(lb labels.Labels) labels.Labels {
	if !m.explicit {
		return lb
	}
	return labels.NewBuilder(lb).Keep(m.labels...).Labels()
}

// uniqueSigIndex maps each matching signature to the index of the one series
// carrying it. A signature shared by several series is ambiguous — which one
// should be paired? — so it is left out rather than guessed, adding one
// warning per such signature via newWarning.
func uniqueSigIndex(
	series []storage.Series, m onMatching, dropLabels []string,
	pos posrange.PositionRange, newWarning func(string, posrange.PositionRange) error,
	ws *annotations.Annotations,
) map[uint64]int {
	idx := make(map[uint64]int, len(series))
	ambiguous := make(map[uint64]struct{})
	var buf []byte
	for i, s := range series {
		var sig uint64
		sig, buf = m.sig(s.Labels(), buf, dropLabels...)
		if _, amb := ambiguous[sig]; amb {
			continue
		}
		if _, dup := idx[sig]; dup {
			delete(idx, sig)
			ambiguous[sig] = struct{}{}
			ws.Add(newWarning(s.Labels().DropReserved(schema.IsMetadataLabel).String(), pos))
			continue
		}
		idx[sig] = i
	}
	return idx
}
