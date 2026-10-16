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
	"io"
	"math"

	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

// errSnapshotSeriesRecovery prevents repair of WAL history skipped by a valid snapshot.
type errSnapshotSeriesRecovery struct{ err error }

func (e *errSnapshotSeriesRecovery) Error() string {
	return "recover snapshot series references: " + e.err.Error()
}
func (e *errSnapshotSeriesRecovery) Unwrap() error { return e.err }

// snapshotReplayReader tracks physical bytes consumed through a bounded reader.
type snapshotReplayReader struct {
	io.Reader
	offset int64
}

func (r *snapshotReplayReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.offset += int64(n)
	return n, err
}

// readSnapshotReplaySegment bounds records to a physical segment or a snapshot offset.
func (h *Head) readSnapshotReplaySegment(dir string, segment, offset, end int, visit func(*wlog.Reader, *record.Decoder) error) error {
	s, err := wlog.OpenReadSegment(wlog.SegmentName(dir, segment))
	if err != nil {
		return fmt.Errorf("open segment %s: %w", wlog.SegmentName(dir, segment), err)
	}
	defer s.Close()
	stat, err := s.Stat()
	if err != nil {
		return err
	}
	if offset < 0 || int64(offset) > stat.Size() || end < -1 || end >= 0 && (end < offset || int64(end) > stat.Size()) {
		return &wlog.CorruptionErr{Dir: dir, Segment: segment, Offset: int64(offset), Err: fmt.Errorf("snapshot range [%d, %d] exceeds segment size %d", offset, end, stat.Size())}
	}
	sr, err := wlog.NewSegmentBufReaderWithOffset(offset, s)
	if err != nil {
		return err
	}
	limit := stat.Size() - int64(offset)
	if end >= 0 {
		limit = int64(end - offset)
	}
	// A segment reader can synthesize page padding after EOF. The auxiliary
	// scan must not hide physical truncation of required history.
	input := &snapshotReplayReader{Reader: io.LimitReader(sr, limit), offset: int64(offset)}
	r := wlog.NewReader(input)
	// Decoder symbol tables must not retain unrelated historical labels across segments.
	dec := record.NewDecoder(nil, h.logger)
	for r.Next() {
		if err := visit(r, &dec); err != nil {
			return &wlog.CorruptionErr{Dir: dir, Segment: segment, Offset: input.offset, Err: err}
		}
	}
	if err := r.Err(); err != nil {
		// The bounded reader hides segment identity from wlog.Reader.Err.
		if cause, ok := errors.AsType[*wlog.CorruptionErr](err); ok {
			err = cause.Err
		}
		return &wlog.CorruptionErr{Dir: dir, Segment: segment, Offset: input.offset, Err: err}
	}
	return nil
}

// snapshotNeedsSeriesRecovery checks references before replay without changing the Head.
func (h *Head) snapshotNeedsSeriesRecovery(snapIdx, snapOffset, endAt int, snapshotSeries map[chunks.HeadSeriesRef]*memSeries, mapped, oooMapped map[chunks.HeadSeriesRef][]*mmappedChunk) bool {
	if h.wbl == nil && len(mapped) == 0 && len(oooMapped) == 0 {
		return false
	}
	known := make(map[chunks.HeadSeriesRef]struct{}, len(snapshotSeries))
	for ref := range snapshotSeries {
		known[ref] = struct{}{}
	}
	var series []record.RefSeries
	for segment := snapIdx; segment <= endAt; segment++ {
		offset := 0
		if segment == snapIdx {
			offset = snapOffset
		}
		err := h.readSnapshotReplaySegment(h.wal.Dir(), segment, offset, -1, func(r *wlog.Reader, dec *record.Decoder) error {
			if dec.Type(r.Record()) != record.Series {
				return nil
			}
			var err error
			series, err = dec.Series(r.Record(), series[:0])
			if err != nil {
				return err
			}
			for _, s := range series {
				known[s.Ref] = struct{}{}
			}
			clear(series)
			return nil
		})
		if err != nil {
			// Preserve references preceding corruption; normal replay owns tail repair.
			break
		}
	}
	for ref := range mapped {
		if _, ok := known[ref]; !ok {
			return true
		}
	}
	for ref := range oooMapped {
		if _, ok := known[ref]; !ok {
			return true
		}
	}
	if h.wbl == nil {
		return false
	}
	first, last, err := wlog.Segments(h.wbl.Dir())
	if err != nil {
		return false // Normal replay reports WBL errors.
	}
	var (
		samples    []record.RefSample
		hists      []record.RefHistogramSample
		floatHists []record.RefFloatHistogramSample
		markers    []record.RefMmapMarker
		unknown    bool
	)
	check := func(ref chunks.HeadSeriesRef) {
		if _, ok := known[ref]; !ok {
			unknown = true
		}
	}
	for segment := first; segment <= last && !unknown; segment++ {
		err := h.readSnapshotReplaySegment(h.wbl.Dir(), segment, 0, -1, func(r *wlog.Reader, dec *record.Decoder) error {
			var err error
			switch dec.Type(r.Record()) {
			case record.Samples, record.SamplesV2:
				samples, err = dec.Samples(r.Record(), samples[:0])
				for _, s := range samples {
					check(s.Ref)
				}
			case record.HistogramSamples, record.CustomBucketsHistogramSamples, record.HistogramSamplesV2:
				hists, err = dec.HistogramSamples(r.Record(), hists[:0])
				for _, s := range hists {
					check(s.Ref)
				}
				clear(hists)
			case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples, record.FloatHistogramSamplesV2:
				floatHists, err = dec.FloatHistogramSamples(r.Record(), floatHists[:0])
				for _, s := range floatHists {
					check(s.Ref)
				}
				clear(floatHists)
			case record.MmapMarkers:
				markers, err = dec.MmapMarkers(r.Record(), markers[:0])
				for _, m := range markers {
					check(m.Ref)
				}
			}
			return err
		})
		if err != nil {
			break
		}
	}
	return unknown
}

type snapshotSeriesGeneration struct {
	series *memSeries
	refs   []chunks.HeadSeriesRef // Definition order determines precedence for overlapping IO chunks.
}

// recoverSnapshotSeries restores skipped aliases without replaying historical samples.
func (h *Head) recoverSnapshotSeries(checkpoint string, checkpointIdx, snapIdx, snapOffset int, mmapped, oooMapped map[chunks.HeadSeriesRef][]*mmappedChunk, lastMmapRef chunks.ChunkDiskMapperRef) (map[chunks.HeadSeriesRef]chunks.HeadSeriesRef, error) {
	active := make(map[*memSeries]*snapshotSeriesGeneration)
	byRef := make(map[chunks.HeadSeriesRef]*snapshotSeriesGeneration)
	lastID := h.lastSeriesID.Load()
	var series []record.RefSeries
	var stones []tombstones.Stone
	visit := func(r *wlog.Reader, dec *record.Decoder) error {
		switch dec.Type(r.Record()) {
		case record.Series:
			var err error
			series, err = dec.Series(r.Record(), series[:0])
			if err != nil {
				return err
			}
			for _, s := range series {
				lastID = max(lastID, uint64(s.Ref))
				ms := h.series.getByHash(s.Labels.Hash(), s.Labels)
				if ms == nil {
					continue
				}
				g := active[ms]
				if g == nil {
					g = &snapshotSeriesGeneration{series: ms}
					active[ms] = g
				}
				g.refs = append(g.refs, s.Ref)
				byRef[s.Ref] = g
			}
			clear(series)
		case record.Tombstones:
			var err error
			stones, err = dec.Tombstones(r.Record(), stones[:0])
			if err != nil {
				return err
			}
			for _, stone := range stones {
				lastID = max(lastID, uint64(stone.Ref))
				if len(stone.Intervals) != 1 || stone.Intervals[0].Mint != math.MinInt64 || stone.Intervals[0].Maxt != math.MaxInt64 {
					continue
				}
				g := byRef[chunks.HeadSeriesRef(stone.Ref)]
				if g == nil {
					continue
				}
				delete(active, g.series)
				for _, ref := range g.refs {
					delete(byRef, ref)
				}
			}
			clear(stones)
		}
		return nil
	}
	if checkpoint != "" {
		first, last, err := wlog.Segments(checkpoint)
		if err != nil {
			return nil, err
		}
		for i := first; i <= last; i++ {
			if err := h.readSnapshotReplaySegment(checkpoint, i, 0, -1, visit); err != nil {
				return nil, err
			}
		}
	}
	_, _, err := wlog.Segments(h.wal.Dir())
	if err != nil {
		return nil, err
	}
	first := 0
	if checkpoint != "" {
		first = checkpointIdx + 1
	}
	for i := first; i <= snapIdx; i++ {
		end := -1
		if i == snapIdx {
			end = snapOffset
		}
		if err := h.readSnapshotReplaySegment(h.wal.Dir(), i, 0, end, visit); err != nil {
			return nil, err
		}
	}

	// Stage all metadata until the entire prefix is verified. Attach persisted chunks
	// before WAL-tail replay, which can write chunks newer than lastMmapRef.
	aliases := make(map[chunks.HeadSeriesRef]chunks.HeadSeriesRef)
	for ms, g := range active {
		canonicalIO := ms.mmappedChunks
		var canonicalOOO []*mmappedChunk
		if ms.ooo != nil {
			canonicalOOO = ms.ooo.oooMmappedChunks
		}
		for _, ref := range g.refs {
			ioChunks, oooChunks := mmapped[ref], oooMapped[ref]
			if ref == ms.ref {
				ioChunks, oooChunks = canonicalIO, canonicalOOO
			} else {
				aliases[ref] = ms.ref
			}
			h.mergeSeriesMMappedChunks(ms, ioChunks, oooChunks, lastMmapRef)
		}
	}
	h.lastSeriesID.Store(lastID)
	return aliases, nil
}
