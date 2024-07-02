// Copyright 2024 Prometheus Team
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

package writev2

import "github.com/prometheus/prometheus/model/labels"

// SymbolsTable implements table for easy symbol use.
type SymbolsTable struct {
	strings    []string
	symbolsMap map[string]uint32
}

// NewSymbolTable returns a symbol table.
func NewSymbolTable() SymbolsTable {
	return SymbolsTable{
		// Empty string is required as a first element.
		symbolsMap: map[string]uint32{"": 0},
		strings:    []string{""},
	}
}

// Symbolize adds (if not added before) a string to the symbols table,
// while returning its reference number.
func (t *SymbolsTable) Symbolize(str string) uint32 {
	if ref, ok := t.symbolsMap[str]; ok {
		return ref
	}
	ref := uint32(len(t.strings))
	t.strings = append(t.strings, str)
	t.symbolsMap[str] = ref
	return ref
}

// SymbolizeLabels symbolize Prometheus labels.
func (t *SymbolsTable) SymbolizeLabels(lbls labels.Labels, buf []uint32) []uint32 {
	result := buf[:0]
	lbls.Range(func(l labels.Label) {
		off := t.Symbolize(l.Name)
		result = append(result, off)
		off = t.Symbolize(l.Value)
		result = append(result, off)
	})
	return result
}

// Symbols returns computes symbols table to put in e.g. Request.Symbols.
// As per spec, order does not matter.
func (t *SymbolsTable) Symbols() []string {
	return t.strings
}

// Reset clears symbols table.
func (t *SymbolsTable) Reset() {
	// NOTE: Make sure to keep empty symbol.
	t.strings = t.strings[:1]
	for k := range t.symbolsMap {
		if k == "" {
			continue
		}
		delete(t.symbolsMap, k)
	}
}

// desymbolizeLabels decodes label references, with given symbols to labels.
func desymbolizeLabels(b *labels.ScratchBuilder, labelRefs []uint32, symbols []string) labels.Labels {
	b.Reset()
	for i := 0; i < len(labelRefs); i += 2 {
		b.Add(symbols[labelRefs[i]], symbols[labelRefs[i+1]])
	}
	b.Sort()
	return b.Labels()
}
