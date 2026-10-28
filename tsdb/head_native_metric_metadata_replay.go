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

package tsdb

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/nativemetadata"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

// nativeMetadataReplayInternerLimit bounds the distinct values replay shares.
const nativeMetadataReplayInternerLimit = 1 << 16

// nativeMetadataReplayRecord carries one decoded compact record to the replay
// loop.
type nativeMetadataReplayRecord struct {
	compact record.CompactNativeMetadata
}

// nativeMetadataReplay reduces native metadata WAL entries during replay, per
// original WAL ref: a retired incarnation may receive entries after its
// successor's Series record. A surviving series' current incarnation is the
// ref of its latest declaration: a Series record, or a handoff for a ref whose
// Series record has been replayed. Compact entries apply only while trust
// allows; latest-value entries and declarations always do.
type nativeMetadataReplay struct {
	states map[chunks.HeadSeriesRef]nativemetadata.State
	// current maps surviving refs to later declared duplicates. Without an
	// entry, a surviving series is its own current incarnation.
	current    map[chunks.HeadSeriesRef]chunks.HeadSeriesRef
	interner   *nativemetadata.Interner
	dictionary nativemetadata.Dictionary
	points     []nativemetadata.Point
	trust      nativemetadata.Trust
	// untrusted counts the entries skipped as untrusted.
	untrusted uint64
	// undeclared counts handoffs for refs without a replayed Series record.
	undeclared uint64
}

func newNativeMetadataReplay() *nativeMetadataReplay {
	return &nativeMetadataReplay{
		states:   map[chunks.HeadSeriesRef]nativemetadata.State{},
		current:  map[chunks.HeadSeriesRef]chunks.HeadSeriesRef{},
		interner: nativemetadata.NewInterner(nativeMetadataReplayInternerLimit),
	}
}

// declare records a declaration of walRef, replayed as series surviving.
func (r *nativeMetadataReplay) declare(surviving, walRef chunks.HeadSeriesRef) {
	if walRef == surviving {
		delete(r.current, surviving)
		return
	}
	r.current[surviving] = walRef
}

// applyLegacy reduces latest-value entries, each replacing its ref's history
// with one version whose start is unknown.
func (r *nativeMetadataReplay) applyLegacy(entries []record.RefMetadata) {
	for _, m := range entries {
		state := r.states[m.Ref]
		r.points = append(r.points[:0], nativemetadata.LegacyPoint(m, r.interner.Intern))
		state.Apply(record.NativeMetadataLegacy, false, r.points)
		r.states[m.Ref] = state
	}
	clear(r.points)
}

// applyCompact reduces a compact record's entries in WAL order, including
// entries for refs whose Series record has not been replayed yet, resolving
// each used value once. A handoff also declares its ref, if its Series record
// has been replayed, even if trust skips its history; multiRef maps duplicate
// WAL refs to their surviving series, and series holds the others.
func (r *nativeMetadataReplay) applyCompact(rec *record.CompactNativeMetadata, multiRef map[chunks.HeadSeriesRef]chunks.HeadSeriesRef, series *stripeSeries) {
	r.dictionary.Reset(rec.Values, r.interner.Intern)
	for _, e := range rec.Entries {
		switch e.Kind {
		case record.NativeMetadataProvenance, record.NativeMetadataBoundary:
			r.trust.Mark()
			continue
		}
		if e.Handoff {
			r.handoff(e.Ref, multiRef, series)
		}
		if !r.trust.Trusted() {
			r.untrusted++
			continue
		}
		r.points = r.dictionary.AppendPoints(r.points[:0], e.Points)
		state := r.states[e.Ref]
		state.Apply(e.Kind, e.Truncated, r.points)
		r.states[e.Ref] = state
	}
	clear(r.points)
	r.dictionary.Reset(nil, nil)
}

// handoff records a handoff for ref as its declaration, if ref's Series record
// has been replayed.
func (r *nativeMetadataReplay) handoff(ref chunks.HeadSeriesRef, multiRef map[chunks.HeadSeriesRef]chunks.HeadSeriesRef, series *stripeSeries) {
	switch surviving, ok := multiRef[ref]; {
	case ok:
		r.declare(surviving, ref)
	case series.getByID(ref) != nil:
		r.declare(ref, ref)
	default:
		r.undeclared++
	}
}

// seedNativeMetricMetadata gives each replayed series its current
// incarnation's reduced history and truncation flag, as committed native state.
// Replay must have finished; multiRef maps duplicate WAL refs to survivors.
func (h *Head) seedNativeMetricMetadata(r *nativeMetadataReplay, multiRef map[chunks.HeadSeriesRef]chunks.HeadSeriesRef) {
	store := h.nativeMetricMetadata
	var seeded, versions int64
	var buf [maxNativeMetricMetadataVersions]nativeMetricMetadataPoint
	for walRef, state := range r.states {
		if state.Metadata == nil {
			continue
		}
		surviving := walRef
		if ref, ok := multiRef[walRef]; ok {
			surviving = ref
		}
		if current, ok := r.current[surviving]; (ok && current != walRef) || (!ok && surviving != walRef) {
			continue
		}
		series := h.series.getByID(surviving)
		if series == nil {
			continue
		}
		points := state.AppendPoints(buf[:0])
		for i := range points {
			points[i].Metadata = store.values.resolve(*points[i].Metadata, nil)
		}
		series.Lock()
		if series.nativeMetadataLocked() != nil {
			series.Unlock()
			continue
		}
		native := series.initNativeMetadataLocked()
		// A reduced history is chronological, with adjacent values differing
		// and at most the version cap, so merging it evicts nothing.
		native.mergeLocked(points)
		native.truncated = state.Truncated
		store.publishLocked(series)
		series.Unlock()
		seeded++
		versions += int64(len(points))
	}
	store.series.Add(seeded)
	store.versions.Add(versions)
	h.metrics.nativeMetadataUntrustedEntries.Add(float64(r.untrusted))
	h.metrics.nativeMetadataUndeclaredHandoffs.Add(float64(r.undeclared))
}

// writeNativeMetadataBoundary writes a boundary record if replay ended
// untrusted: it read a checkpoint without a provenance marker, and no boundary
// after it. Later replays then trust the native metadata this run writes. It
// runs after replay, which applied no compact entry, and before the startup
// handoffs and any appender, so the boundary precedes every other record of
// the run.
func (h *Head) writeNativeMetadataBoundary(r *nativeMetadataReplay) error {
	if r.trust.Trusted() {
		return nil
	}
	var enc record.Encoder
	if err := h.wal.Log(enc.NativeMetadataMarker(record.NativeMetadataBoundary, nil)); err != nil {
		return fmt.Errorf("write native metadata boundary: %w", err)
	}
	h.metrics.nativeMetadataBoundariesWritten.Inc()
	r.trust.Mark()
	return nil
}

// writeNativeMetadataHandoffs writes a handoff for each replayed series that
// other WAL refs were remapped onto, carrying its seeded history, if any. A
// ref's repeated Series record, which a failed commit's rollback logs, maps
// the ref onto itself and isn't a remap.
// The sender then holds that history under the surviving ref, which later
// replays and checkpoints take as the series' current incarnation. It runs
// after seeding and before any appender exists, and stops at the first failed
// write; multiRef maps duplicate WAL refs to survivors.
func (h *Head) writeNativeMetadataHandoffs(multiRef map[chunks.HeadSeriesRef]chunks.HeadSeriesRef) error {
	survivors := make([]chunks.HeadSeriesRef, 0, len(multiRef))
	for walRef, ref := range multiRef {
		if walRef != ref {
			survivors = append(survivors, ref)
		}
	}
	slices.Sort(survivors)
	survivors = slices.Compact(survivors)

	var (
		rec  nativeMetadataOverrideRecord
		buf  []byte
		size int
	)
	flush := func() error {
		if rec.len() == 0 {
			return nil
		}
		buf = rec.encode(buf[:0])
		if err := h.wal.Log(buf); err != nil {
			return fmt.Errorf("write native metadata handoffs: %w", err)
		}
		h.metrics.nativeMetadataHandoffsWritten.WithLabelValues(nativeMetadataHandoffStartup).Add(float64(rec.len()))
		rec.reset()
		size = 0
		return nil
	}
	for _, ref := range survivors {
		s := h.series.getByID(ref)
		if s == nil {
			continue
		}
		var history [maxNativeMetricMetadataVersions]nativeMetricMetadataPoint
		count, truncated := 0, false
		s.Lock()
		if native := s.nativeMetadataLocked(); native != nil {
			count = len(native.AppendPoints(history[:0]))
			truncated = native.truncated
		}
		s.Unlock()
		entrySize := nativeMetadataEntrySizeEstimate(history[:count])
		if rec.len() == nativeMetadataRepairBatch || (rec.len() > 0 && size+entrySize > nativeMetadataRepairBatchBytes) {
			if err := flush(); err != nil {
				return err
			}
		}
		rec.add(ref, truncated, true, history[:count])
		size += entrySize
	}
	return flush()
}

// eligibleTornWALTail reports whether err, from replaying segment i of the
// segments up to last, is a torn final record that replay may end the segment
// at. That needs native metadata persisted, the WAL reader's own torn-record
// error, and the torn record in the last segment that holds any record. Every
// start opens a new, empty segment before replay, and each failed start leaves
// one behind, so every later segment must hold no record at all. Unsupported
// and malformed records, checksum failures and any other corruption are never
// eligible.
func (h *Head) eligibleTornWALTail(err error, i, last int) (*wlog.TornRecordError, bool) {
	if !h.opts.EnableNativeMetadata {
		return nil, false
	}
	var torn *wlog.TornRecordError
	if !errors.As(err, &torn) {
		return nil, false
	}
	for j := i + 1; j <= last; j++ {
		if empty, err := walSegmentEmpty(h.wal.Dir(), j); err != nil || !empty {
			return nil, false
		}
	}
	return torn, true
}

// walSegmentEmpty reports whether segment i of the WAL in dir holds no record.
func walSegmentEmpty(dir string, i int) (bool, error) {
	s, err := wlog.OpenReadSegment(wlog.SegmentName(dir, i))
	if err != nil {
		return false, err
	}
	defer s.Close()
	r := wlog.NewReader(wlog.NewSegmentBufReader(s))
	if r.Next() {
		return false, nil
	}
	return r.Err() == nil, r.Err()
}

// truncateTornWALTail removes a torn final record that starts at offset from
// WAL segment i, keeping every complete record before it byte for byte, and
// syncs the segment. If it fails, nothing has been written since, so a later
// start finds the torn record again, or a clean end.
func (h *Head) truncateTornWALTail(i int, offset int64) error {
	truncate, sync := (*os.File).Truncate, (*os.File).Sync
	if hooks := h.nativeMetadataTestHooks; hooks != nil {
		if hooks.tornTailTruncate != nil {
			truncate = hooks.tornTailTruncate
		}
		if hooks.tornTailSync != nil {
			sync = hooks.tornTailSync
		}
	}
	f, err := os.OpenFile(wlog.SegmentName(h.wal.Dir(), i), os.O_WRONLY, 0o666)
	if err != nil {
		return fmt.Errorf("open WAL segment %d to remove its torn record: %w", i, err)
	}
	if err := truncate(f, offset); err != nil {
		f.Close()
		return fmt.Errorf("remove the torn record of WAL segment %d: %w", i, err)
	}
	if err := sync(f); err != nil {
		f.Close()
		return fmt.Errorf("sync WAL segment %d after removing its torn record: %w", i, err)
	}
	return f.Close()
}
