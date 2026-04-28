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
	"errors"
	"io"
	"sort"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/stateset"
)

// StateSetParser wraps a Parser and converts OpenMetrics stateset float series
// into native EntryStateset entries. It aggregates all float series belonging
// to the same stateset metric family and dimension-label group into a single
// StateSet sample.
//
// OpenMetrics stateset format:
//
//	# TYPE foo stateset
//	foo{foo="running"} 1
//	foo{foo="stopped"} 0
//
// The state-carrying label has the same name as the metric family. All other
// labels form the "dimension" key that groups series into a single stateset.
//
// StateSetParser must be applied after NHCBParser when both conversions are
// active, since it wraps any Parser and is agnostic to histogram conversion.
type StateSetParser struct {
	parser  Parser
	builder labels.ScratchBuilder

	// State machine: stateStart, stateCollecting, or stateEmitting.
	// These constants are shared with NHCBParser (defined in nhcbparse.go).
	state collectionState

	// Saved underlying entry and error to replay when state == stateEmitting.
	entry Entry
	err   error

	// Cached values from the wrapped parser for pass-through methods.
	bytes []byte
	ts    *int64
	value float64
	h     *histogram.Histogram
	fh    *histogram.FloatHistogram
	lset  labels.Labels
	bName []byte
	typ   model.MetricType

	// Output fields populated when EntryStateset is ready to be returned.
	ssBytes []byte
	ssTS    *int64
	ssSS    *stateset.StateSet
	ssLset  labels.Labels // dimension labels for the emitted stateset

	// Accumulation fields for the stateset group being built.
	statesetFamilyName string // non-empty when inside a TYPE stateset block
	tempTS             *int64
	tempDimLset        labels.Labels
	tempDimHash        uint64
	tempNames          []string
	tempActive         []bool

	// Reusable buffer for label hashing.
	hBuffer []byte
}

// NewStateSetParser returns a Parser that wraps p and converts OpenMetrics
// stateset float series into EntryStateset entries. st is the symbol table
// used when building dimension label sets.
func NewStateSetParser(p Parser, st *labels.SymbolTable) Parser {
	return &StateSetParser{
		parser:  p,
		builder: labels.NewScratchBuilderWithSymbolTable(st, 16),
	}
}

// Series returns the cached series data. Valid after Next returns EntrySeries.
func (p *StateSetParser) Series() ([]byte, *int64, float64) {
	return p.bytes, p.ts, p.value
}

// Histogram returns the cached histogram data. Valid after Next returns EntryHistogram.
func (p *StateSetParser) Histogram() ([]byte, *int64, *histogram.Histogram, *histogram.FloatHistogram) {
	return p.bytes, p.ts, p.h, p.fh
}

// Stateset returns the aggregated stateset. Valid after Next returns EntryStateset.
func (p *StateSetParser) Stateset() ([]byte, *int64, *stateset.StateSet) {
	return p.ssBytes, p.ssTS, p.ssSS
}

// Help delegates to the wrapped parser.
func (p *StateSetParser) Help() ([]byte, []byte) { return p.parser.Help() }

// Type returns the cached type metadata. Valid after Next returns EntryType.
func (p *StateSetParser) Type() ([]byte, model.MetricType) { return p.bName, p.typ }

// Unit delegates to the wrapped parser.
func (p *StateSetParser) Unit() ([]byte, []byte) { return p.parser.Unit() }

// Comment delegates to the wrapped parser.
func (p *StateSetParser) Comment() []byte { return p.parser.Comment() }

// Labels writes the labels of the current entry into l.
func (p *StateSetParser) Labels(l *labels.Labels) {
	if p.state == stateEmitting {
		*l = p.ssLset
		return
	}
	*l = p.lset
}

// Exemplar delegates to the wrapped parser.
func (p *StateSetParser) Exemplar(ex *exemplar.Exemplar) bool { return p.parser.Exemplar(ex) }

// StartTimestamp delegates to the wrapped parser.
func (p *StateSetParser) StartTimestamp() int64 { return p.parser.StartTimestamp() }

// Next advances the parser to the next entry, aggregating stateset series as
// it goes. When the last member of a stateset group has been consumed it emits
// EntryStateset before continuing with the next underlying entry.
func (p *StateSetParser) Next() (Entry, error) {
	for {
		if p.state == stateEmitting {
			p.state = stateStart
			// Replay the saved underlying entry. If it looks like a member
			// of a stateset family, try to start collecting a new group.
			if p.entry == EntrySeries && p.statesetFamilyName != "" {
				if p.tryCollect() {
					continue
				}
			}
			return p.entry, p.err
		}

		p.entry, p.err = p.parser.Next()
		if p.err != nil {
			if errors.Is(p.err, io.EOF) && p.state == stateCollecting {
				if p.emitStateset() {
					return EntryStateset, nil
				}
			}
			return EntryInvalid, p.err
		}

		switch p.entry {
		case EntrySeries:
			p.bytes, p.ts, p.value = p.parser.Series()
			p.parser.Labels(&p.lset)

			if p.statesetFamilyName == "" {
				// Not inside a stateset family; flush any pending group first.
				if p.state == stateCollecting && p.emitStateset() {
					p.state = stateEmitting
					return EntryStateset, nil
				}
				return EntrySeries, nil
			}

			if p.tryCollect() {
				// tryCollect may have emitted a stateset (dimension group change).
				if p.state == stateEmitting {
					return EntryStateset, nil
				}
				continue
			}

			// Series doesn't belong to the current stateset (wrong name or
			// missing state label). Flush any pending group, then replay.
			if p.state == stateCollecting && p.emitStateset() {
				p.state = stateEmitting
				return EntryStateset, nil
			}
			return EntrySeries, nil

		case EntryHistogram:
			p.bytes, p.ts, p.h, p.fh = p.parser.Histogram()
			p.parser.Labels(&p.lset)
			if p.state == stateCollecting && p.emitStateset() {
				p.state = stateEmitting
				return EntryStateset, nil
			}
			return EntryHistogram, nil

		case EntryType:
			p.bName, p.typ = p.parser.Type()
			// Compute the new family name. The emit below must still use the
			// old statesetFamilyName, so we apply the update after the emit
			// but before stateEmitting replay resumes (which just returns
			// p.entry without re-executing this branch).
			newFamilyName := ""
			if p.typ == model.MetricTypeStateset {
				newFamilyName = string(p.bName)
			}
			if p.state == stateCollecting && p.emitStateset() {
				p.statesetFamilyName = newFamilyName
				p.state = stateEmitting
				return EntryStateset, nil
			}
			p.statesetFamilyName = newFamilyName
			return EntryType, nil

		default:
			if p.state == stateCollecting && p.emitStateset() {
				p.state = stateEmitting
				return EntryStateset, nil
			}
			return p.entry, p.err
		}
	}
}

// tryCollect attempts to incorporate the current series into the accumulating
// stateset group. It returns true if the series was consumed (the caller must
// loop rather than return it). The current series data must already be cached
// in p.bytes/p.ts/p.value/p.lset before calling.
func (p *StateSetParser) tryCollect() bool {
	metricName := p.lset.Get(labels.MetricName)
	if metricName != p.statesetFamilyName {
		return false
	}
	stateValue := p.lset.Get(metricName)
	if stateValue == "" {
		return false
	}

	// Hash all labels except __name__ and the state-carrying label so that
	// series belonging to the same dimension group share the same hash.
	dimHash, _ := p.lset.HashWithoutLabels(p.hBuffer, labels.MetricName, metricName)

	if p.state == stateCollecting && dimHash != p.tempDimHash {
		// Different dimension group: emit what we have, then replay this series.
		if p.emitStateset() {
			// p.entry/lset/bytes/ts/value still hold the current series;
			// stateEmitting will call tryCollect again after yielding the stateset.
			return true
		}
		// emitStateset produced nothing (empty accumulation); start fresh below.
		p.resetAccumulation()
	}

	if p.state != stateCollecting {
		p.resetAccumulation()
		p.tempTS = p.ts
		p.tempDimHash = dimHash

		// Build dimension labels: all labels except __name__ and state label.
		p.builder.Reset()
		p.lset.Range(func(l labels.Label) {
			if l.Name != labels.MetricName && l.Name != metricName {
				p.builder.Add(l.Name, l.Value)
			}
		})
		p.tempDimLset = p.builder.Labels()
		p.state = stateCollecting
	}

	if len(p.tempNames) < stateset.MaxStates {
		p.tempNames = append(p.tempNames, stateValue)
		p.tempActive = append(p.tempActive, p.value > 0.5)
	}
	return true
}

// resetAccumulation clears the per-group accumulation state.
func (p *StateSetParser) resetAccumulation() {
	p.tempNames = p.tempNames[:0]
	p.tempActive = p.tempActive[:0]
	p.tempTS = nil
	p.tempDimHash = 0
	p.tempDimLset = labels.EmptyLabels()
}

// emitStateset builds the output StateSet from the accumulated state. It
// transitions to stateEmitting and returns true on success, or resets to
// stateStart and returns false if there is nothing to emit.
func (p *StateSetParser) emitStateset() bool {
	if len(p.tempNames) == 0 {
		p.state = stateStart
		p.resetAccumulation()
		return false
	}

	// Sort states lexicographically, keeping active flags aligned.
	type nameActive struct {
		name   string
		active bool
	}
	pairs := make([]nameActive, len(p.tempNames))
	for i, n := range p.tempNames {
		pairs[i] = nameActive{n, p.tempActive[i]}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].name < pairs[j].name })

	names := make([]string, len(pairs))
	var values uint64
	for i, pa := range pairs {
		names[i] = pa.name
		if pa.active {
			values |= 1 << uint(i)
		}
	}

	p.ssSS = &stateset.StateSet{
		LabelName: p.statesetFamilyName,
		Names:     names,
		Values:    values,
	}
	p.ssTS = p.tempTS
	p.ssBytes = []byte(p.statesetFamilyName)

	// ssLset for the emitted entry is the dimension label set (no __name__, no
	// state label) so that callers can read it via Labels().
	p.ssLset = p.tempDimLset

	p.state = stateEmitting
	p.resetAccumulation()
	return true
}
