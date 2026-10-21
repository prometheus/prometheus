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

package wlog

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/nativemetadata"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

// CheckpointStats returns stats about a created checkpoint.
type CheckpointStats struct {
	DroppedSeries     int
	DroppedSamples    int // Includes histograms.
	DroppedTombstones int
	DroppedExemplars  int
	DroppedMetadata   int
	TotalSeries       int // Processed series including dropped ones.
	TotalSamples      int // Processed float and histogram samples including dropped ones.
	TotalTombstones   int // Processed tombstones including dropped ones.
	TotalExemplars    int // Processed exemplars including dropped ones.
	TotalMetadata     int // Processed metadata including dropped ones.
	// UntrustedMetadata counts kept native metadata entries that the
	// reduction skipped because it didn't trust them.
	UntrustedMetadata int
	// Provenance reports whether the checkpoint carries a provenance marker.
	Provenance bool
	// Handoffs counts the handoffs written: one per label set whose current
	// incarnation a kept handoff declared after its latest kept series record.
	Handoffs int
	// UndeclaredHandoffs counts kept handoffs for refs without an earlier kept
	// series record, which declare nothing.
	UndeclaredHandoffs int
}

// LastCheckpoint returns the directory name and index of the most recent checkpoint.
// If dir does not contain any checkpoints, ErrNotFound is returned.
func LastCheckpoint(dir string) (string, int, error) {
	checkpoints, err := listCheckpoints(dir)
	if err != nil {
		return "", 0, err
	}

	if len(checkpoints) == 0 {
		return "", 0, record.ErrNotFound
	}

	checkpoint := checkpoints[len(checkpoints)-1]
	return filepath.Join(dir, checkpoint.name), checkpoint.index, nil
}

// ReadMinValidTime returns the mint recorded by the most recent checkpoint in dir, i.e. the
// highest mint any WAL truncation has used so far. ok is false, with no error, if dir has no
// checkpoint yet, or its most recent checkpoint predates this record (e.g. it was written by
// an older Prometheus version).
func ReadMinValidTime(dir string) (mint int64, ok bool, err error) {
	cpdir, _, err := LastCheckpoint(dir)
	if errors.Is(err, record.ErrNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("find last checkpoint: %w", err)
	}

	sr, err := NewSegmentsReader(cpdir)
	if err != nil {
		return 0, false, fmt.Errorf("open checkpoint: %w", err)
	}
	defer sr.Close()

	r := NewReader(sr)
	if !r.Next() {
		return 0, false, r.Err()
	}

	dec := record.NewDecoder(nil, promslog.NewNopLogger())
	rec := r.Record()
	if dec.Type(rec) != record.MinValidTime {
		// An older checkpoint, written before this record existed.
		return 0, false, nil
	}
	mint, err = dec.MinValidTime(rec)
	if err != nil {
		return 0, false, fmt.Errorf("decode min valid time: %w", err)
	}
	return mint, true, nil
}

// DeleteCheckpoints deletes all checkpoints in a directory below a given index.
func DeleteCheckpoints(dir string, maxIndex int) error {
	checkpoints, err := listCheckpoints(dir)
	if err != nil {
		return err
	}

	var errs []error
	for _, checkpoint := range checkpoints {
		if checkpoint.index >= maxIndex {
			break
		}
		errs = append(errs, os.RemoveAll(filepath.Join(dir, checkpoint.name)))
	}
	return errors.Join(errs...)
}

// CheckpointTempFileSuffix is the suffix used when creating temporary checkpoint files.
const CheckpointTempFileSuffix = ".tmp"

// DeleteTempCheckpoints deletes all temporary checkpoint directories in the given directory.
func DeleteTempCheckpoints(logger *slog.Logger, dir string) error {
	if err := tsdbutil.RemoveTmpDirs(logger, dir, isTempDir); err != nil {
		return fmt.Errorf("remove previous temporary checkpoint dirs: %w", err)
	}
	return nil
}

const (
	// metadataBatchSize is how many series a single checkpoint metadata record covers.
	metadataBatchSize = 1000
	// flushThreshold is how many encoded bytes accumulate before they are written out.
	flushThreshold = 1 * 1024 * 1024
	// checkpointMetadataInternerLimit bounds the distinct values a checkpoint shares.
	checkpointMetadataInternerLimit = 1 << 16
)

// checkpointMetadata is a series' metadata reduced from WAL entries. Series
// that only had legacy entries keep a legacy entry with the latest value.
type checkpointMetadata struct {
	nativemetadata.State
	native bool
}

// checkpointLabelSet is a label set with kept series records. Its current
// incarnation is the ref of its latest kept series record or handoff, in input
// order.
type checkpointLabelSet struct {
	labels  labels.Labels
	current chunks.HeadSeriesRef
	// series is the ref of its latest kept series record.
	series chunks.HeadSeriesRef
}

// checkpointLabelSets tracks the label sets of kept series records.
type checkpointLabelSets struct {
	byHash map[uint64][]*checkpointLabelSet
	byRef  map[chunks.HeadSeriesRef]*checkpointLabelSet
}

// declare records a kept series record for ref.
func (c *checkpointLabelSets) declare(ref chunks.HeadSeriesRef, lset labels.Labels) {
	set := c.byRef[ref]
	if set == nil {
		hash := lset.Hash()
		for _, candidate := range c.byHash[hash] {
			if labels.Equal(candidate.labels, lset) {
				set = candidate
				break
			}
		}
		if set == nil {
			set = &checkpointLabelSet{labels: lset}
			c.byHash[hash] = append(c.byHash[hash], set)
		}
		c.byRef[ref] = set
	}
	set.current, set.series = ref, ref
}

// handoff records a kept handoff for ref as its declaration, if a series record
// for ref has been kept.
func (c *checkpointLabelSets) handoff(ref chunks.HeadSeriesRef, stats *CheckpointStats) {
	if set := c.byRef[ref]; set != nil {
		set.current = ref
	} else {
		stats.UndeclaredHandoffs++
	}
}

// winners returns the current incarnation of every label set that a handoff
// declared after its latest kept series record. A checkpoint keeps series
// records in input order, so it needs handoffs for those only.
func (c *checkpointLabelSets) winners() map[chunks.HeadSeriesRef]struct{} {
	out := map[chunks.HeadSeriesRef]struct{}{}
	for _, sets := range c.byHash {
		for _, set := range sets {
			if set.current != set.series {
				out[set.current] = struct{}{}
			}
		}
	}
	return out
}

// Checkpoint creates a compacted checkpoint of segments in range [from, to] in the given WAL.
// It includes the most recent checkpoint if it exists.
// All series not satisfying keep, samples/exemplars below mint, and tombstones not
// satisfying keep or with all intervals below mint are dropped. Metadata entries
// are reduced per kept series to the latest one. With reduceNativeMetadata,
// native metadata entries, in compact records, are reduced per kept series to
// an override carrying the reduced history and truncation flag; without it,
// they are dropped, as records of an unknown type are. A native reduction
// applies only the entries it trusts, by nativemetadata.Trust, and marks the
// checkpoint with a provenance marker if it ends trusted. A compact record that
// doesn't decode fails the checkpoint, whatever the reason.
// Metadata follows every series record, so a label set whose current
// incarnation, the ref of its latest kept series record or handoff in the
// input, was declared by a handoff gets a handoff for it, even an empty one.
//
// keep is evaluated per record as segments are read, so its result for a given ref
// must not change while Checkpoint runs. Otherwise records for the same ref could be
// treated inconsistently, e.g. a series record kept but its tombstone dropped. The
// Head satisfies this by serializing checkpointing with every series-deleting path
// (GC and series truncation) via chunkSnapshotMtx.
//
// The checkpoint is stored in a directory named checkpoint.N in the same
// segmented format as the original WAL itself.
// This makes it easy to read it through the WAL package and concatenate
// it with the original WAL.
//
// writeMinValidTime controls whether the checkpoint also carries a record of mint, readable
// back with ReadMinValidTime. It is opt-in because not every caller's own replay path
// tolerates an unrecognized record type equally gracefully; callers that don't consume
// ReadMinValidTime should pass false.
func Checkpoint(logger *slog.Logger, w *WL, from, to int, keep func(id chunks.HeadSeriesRef) bool, mint int64, enableSTStorage, writeMinValidTime, reduceNativeMetadata bool) (*CheckpointStats, error) {
	stats := &CheckpointStats{}
	var sgmReader io.ReadCloser
	var trust nativemetadata.Trust

	logger.Info("Creating checkpoint", "from_segment", from, "to_segment", to, "mint", mint)

	{
		var sgmRange []SegmentRange
		dir, idx, err := LastCheckpoint(w.Dir())
		if err != nil && !errors.Is(err, record.ErrNotFound) {
			return nil, fmt.Errorf("find last checkpoint: %w", err)
		}
		last := idx + 1
		if err == nil {
			if from > last {
				return nil, fmt.Errorf("unexpected gap to last checkpoint. expected:%v, requested:%v", last, from)
			}
			// Ignore WAL files below the checkpoint. They shouldn't exist to begin with.
			from = last
			trust.StartCheckpoint()

			sgmRange = append(sgmRange, SegmentRange{Dir: dir, Last: math.MaxInt32})
		}

		sgmRange = append(sgmRange, SegmentRange{Dir: w.Dir(), First: from, Last: to})
		sgmReader, err = NewSegmentsRangeReader(sgmRange...)
		if err != nil {
			return nil, fmt.Errorf("create segment reader: %w", err)
		}
		defer sgmReader.Close()
	}

	if err := DeleteTempCheckpoints(logger, w.Dir()); err != nil {
		return nil, err
	}

	cpdir := CheckpointDir(w.Dir(), to)
	cpdirtmp := cpdir + CheckpointTempFileSuffix

	if err := os.MkdirAll(cpdirtmp, 0o777); err != nil {
		return nil, fmt.Errorf("create checkpoint dir: %w", err)
	}
	cp, err := New(nil, nil, cpdirtmp, w.CompressionType())
	if err != nil {
		return nil, fmt.Errorf("open checkpoint: %w", err)
	}

	// Ensures that an early return caused by an error doesn't leave any tmp files.
	defer func() {
		cp.Close()
		os.RemoveAll(cpdirtmp)
	}()

	if writeMinValidTime {
		// Persist mint as the checkpoint's first record, so a restart can recover it without
		// depending on whether any block on disk happens to reflect it: a block produced by
		// CompactSelectedSeries or CompactStaleHead is deliberately excluded from that search,
		// and a truncation whose range had nothing left to write never produces a block at all.
		//
		// mint by itself is only guaranteed to be the highest ever used within this process:
		// the guard that makes truncateWAL's calls strictly increasing lives in memory
		// (Head.lastWALTruncationTime) and resets on every restart, so a later checkpoint
		// could otherwise persist a lower mint than an earlier one already did. Read back
		// whatever the previous checkpoint recorded and keep the higher of the two, so the
		// persisted value never regresses across restarts. This is also what makes it safe to
		// unconditionally drop the previous checkpoint's own copy of this record further down:
		// its value has already been folded into the one written here.
		persistedMinValidTime := mint
		previous, ok, err := ReadMinValidTime(w.Dir())
		if err != nil {
			return nil, fmt.Errorf("read previous min valid time: %w", err)
		}
		if ok && previous > persistedMinValidTime {
			persistedMinValidTime = previous
		}
		var minValidTimeEnc record.Encoder
		if err := cp.Log(minValidTimeEnc.MinValidTime(persistedMinValidTime, nil)); err != nil {
			return nil, fmt.Errorf("write min valid time record: %w", err)
		}
	}

	r := NewReader(sgmReader)

	var (
		series                []record.RefSeries
		samples               []record.RefSample
		histogramSamples      []record.RefHistogramSample
		floatHistogramSamples []record.RefFloatHistogramSample
		tstones               []tombstones.Stone
		exemplars             []record.RefExemplar
		metadata              []record.RefMetadata
		metadataValues        []nativemetadata.Point
		st                    = labels.NewSymbolTable() // Needed for decoding; labels do not outlive this function.
		dec                   = record.NewDecoder(st, logger)
		enc                   = record.Encoder{EnableSTStorage: enableSTStorage}
		buf                   []byte
		recs                  [][]byte

		reducedMetadata = make(map[chunks.HeadSeriesRef]checkpointMetadata)
		interner        = nativemetadata.NewInterner(checkpointMetadataInternerLimit)
		labelSets       = checkpointLabelSets{byHash: map[uint64][]*checkpointLabelSet{}, byRef: map[chunks.HeadSeriesRef]*checkpointLabelSet{}}
		compact         record.CompactNativeMetadata
		dictionary      nativemetadata.Dictionary
	)
	for r.Next() {
		series, samples, histogramSamples, floatHistogramSamples, tstones, exemplars, metadata = series[:0], samples[:0], histogramSamples[:0], floatHistogramSamples[:0], tstones[:0], exemplars[:0], metadata[:0]

		// We don't reset the buffer since we batch up multiple records
		// before writing them to the checkpoint.
		// Remember where the record for this iteration starts.
		start := len(buf)
		rec := r.Record()

		switch dec.Type(rec) {
		case record.Series:
			series, err = dec.Series(rec, series)
			if err != nil {
				return nil, fmt.Errorf("decode series: %w", err)
			}
			// Drop irrelevant series in place.
			repl := series[:0]
			for _, s := range series {
				if keep(s.Ref) {
					repl = append(repl, s)
					if reduceNativeMetadata {
						labelSets.declare(s.Ref, s.Labels)
					}
				}
			}
			if len(repl) > 0 {
				buf = enc.Series(repl, buf)
			}
			stats.TotalSeries += len(series)
			stats.DroppedSeries += len(series) - len(repl)

		case record.Samples, record.SamplesV2:
			samples, err = dec.Samples(rec, samples)
			if err != nil {
				return nil, fmt.Errorf("decode samples: %w", err)
			}
			// Drop irrelevant samples in place.
			repl := samples[:0]
			for _, s := range samples {
				if s.T >= mint {
					repl = append(repl, s)
				}
			}
			if len(repl) > 0 {
				buf = enc.Samples(repl, buf)
			}
			stats.TotalSamples += len(samples)
			stats.DroppedSamples += len(samples) - len(repl)

		case record.HistogramSamples, record.HistogramSamplesV2:
			histogramSamples, err = dec.HistogramSamples(rec, histogramSamples)
			if err != nil {
				return nil, fmt.Errorf("decode histogram samples: %w", err)
			}
			// Drop irrelevant histogramSamples in place.
			repl := histogramSamples[:0]
			for _, h := range histogramSamples {
				if h.T >= mint {
					repl = append(repl, h)
				}
			}
			if len(repl) > 0 {
				var leftover []record.RefHistogramSample
				buf, leftover = enc.HistogramSamples(repl, buf)
				if len(leftover) > 0 {
					// Flush the exponential-histogram record before
					// appending the custom-bucket record so they are
					// written as two separate WAL records.
					if expEnd := len(buf); expEnd > start {
						recs = append(recs, buf[start:expEnd])
						start = expEnd
					}
					buf = enc.CustomBucketsHistogramSamples(leftover, buf)
				}
			}
			stats.TotalSamples += len(histogramSamples)
			stats.DroppedSamples += len(histogramSamples) - len(repl)
		case record.CustomBucketsHistogramSamples:
			histogramSamples, err = dec.HistogramSamples(rec, histogramSamples)
			if err != nil {
				return nil, fmt.Errorf("decode histogram samples: %w", err)
			}
			// Drop irrelevant histogramSamples in place.
			repl := histogramSamples[:0]
			for _, h := range histogramSamples {
				if h.T >= mint {
					repl = append(repl, h)
				}
			}
			if len(repl) > 0 {
				buf = enc.CustomBucketsHistogramSamples(repl, buf)
			}
			stats.TotalSamples += len(histogramSamples)
			stats.DroppedSamples += len(histogramSamples) - len(repl)
		case record.FloatHistogramSamples, record.FloatHistogramSamplesV2:
			floatHistogramSamples, err = dec.FloatHistogramSamples(rec, floatHistogramSamples)
			if err != nil {
				return nil, fmt.Errorf("decode float histogram samples: %w", err)
			}
			// Drop irrelevant floatHistogramSamples in place.
			repl := floatHistogramSamples[:0]
			for _, fh := range floatHistogramSamples {
				if fh.T >= mint {
					repl = append(repl, fh)
				}
			}
			if len(repl) > 0 {
				var floatLeftover []record.RefFloatHistogramSample
				buf, floatLeftover = enc.FloatHistogramSamples(repl, buf)
				if len(floatLeftover) > 0 {
					// Flush the exponential-float-histogram record before
					// appending the custom-bucket record so they are
					// written as two separate WAL records.
					if expEnd := len(buf); expEnd > start {
						recs = append(recs, buf[start:expEnd])
						start = expEnd
					}
					buf = enc.CustomBucketsFloatHistogramSamples(floatLeftover, buf)
				}
			}
			stats.TotalSamples += len(floatHistogramSamples)
			stats.DroppedSamples += len(floatHistogramSamples) - len(repl)
		case record.CustomBucketsFloatHistogramSamples:
			floatHistogramSamples, err = dec.FloatHistogramSamples(rec, floatHistogramSamples)
			if err != nil {
				return nil, fmt.Errorf("decode float histogram samples: %w", err)
			}
			// Drop irrelevant floatHistogramSamples in place.
			repl := floatHistogramSamples[:0]
			for _, fh := range floatHistogramSamples {
				if fh.T >= mint {
					repl = append(repl, fh)
				}
			}
			if len(repl) > 0 {
				buf = enc.CustomBucketsFloatHistogramSamples(repl, buf)
			}
			stats.TotalSamples += len(floatHistogramSamples)
			stats.DroppedSamples += len(floatHistogramSamples) - len(repl)
		case record.Tombstones:
			tstones, err = dec.Tombstones(rec, tstones)
			if err != nil {
				return nil, fmt.Errorf("decode deletes: %w", err)
			}
			// Drop irrelevant tombstones in place. A tombstone is dropped together with
			// its series record, or once all its intervals age out of the WAL.
			repl := tstones[:0]
			for _, s := range tstones {
				if !keep(chunks.HeadSeriesRef(s.Ref)) {
					continue
				}
				for _, iv := range s.Intervals {
					if iv.Maxt >= mint {
						repl = append(repl, s)
						break
					}
				}
			}
			if len(repl) > 0 {
				buf = enc.Tombstones(repl, buf)
			}
			stats.TotalTombstones += len(tstones)
			stats.DroppedTombstones += len(tstones) - len(repl)

		case record.Exemplars:
			exemplars, err = dec.Exemplars(rec, exemplars)
			if err != nil {
				return nil, fmt.Errorf("decode exemplars: %w", err)
			}
			// Drop irrelevant exemplars in place.
			repl := exemplars[:0]
			for _, e := range exemplars {
				if e.T >= mint {
					repl = append(repl, e)
				}
			}
			if len(repl) > 0 {
				buf = enc.Exemplars(repl, buf)
			}
			stats.TotalExemplars += len(exemplars)
			stats.DroppedExemplars += len(exemplars) - len(repl)
		case record.Metadata:
			metadata, err = dec.Metadata(rec, metadata)
			if err != nil {
				return nil, fmt.Errorf("decode metadata: %w", err)
			}
			// Keep one reduced entry per series, in WAL order.
			repl := 0
			for _, m := range metadata {
				if !keep(m.Ref) {
					continue
				}
				reduced, ok := reducedMetadata[m.Ref]
				if !ok {
					repl++
				}
				reduced.Apply(record.NativeMetadataLegacy, false, append(metadataValues[:0], nativemetadata.LegacyPoint(m, interner.Intern)))
				reducedMetadata[m.Ref] = reduced
			}
			clear(metadataValues)
			stats.TotalMetadata += len(metadata)
			stats.DroppedMetadata += len(metadata) - repl
		case record.MinValidTime:
			// The previous checkpoint's own copy. Safe to drop unconditionally: its value was
			// already read back and folded into the record written above, via max(mint, this
			// same value), so nothing it could contribute is lost.
			continue
		case record.NativeMetadataCompact:
			if !reduceNativeMetadata {
				continue
			}
			if err := dec.CompactNativeMetadata(rec, &compact); err != nil {
				return nil, fmt.Errorf("decode compact native metadata: %w", err)
			}
			// Reduce as above, resolving each used value once. Untrusted
			// entries are skipped, but their handoffs still declare.
			dictionary.Reset(compact.Values, interner.Intern)
			repl := 0
			for _, m := range compact.Entries {
				switch {
				case m.Kind == record.NativeMetadataProvenance || m.Kind == record.NativeMetadataBoundary:
					trust.Mark()
					continue
				case !keep(m.Ref):
					continue
				}
				if m.Handoff {
					labelSets.handoff(m.Ref, stats)
				}
				if !trust.Trusted() {
					stats.UntrustedMetadata++
					continue
				}
				reduced, ok := reducedMetadata[m.Ref]
				if !ok {
					repl++
				}
				metadataValues = dictionary.AppendPoints(metadataValues[:0], m.Points)
				reduced.Apply(m.Kind, m.Truncated, metadataValues)
				reduced.native = true
				reducedMetadata[m.Ref] = reduced
			}
			clear(metadataValues)
			stats.TotalMetadata += len(compact.Entries)
			stats.DroppedMetadata += len(compact.Entries) - repl
			dictionary.Reset(nil, nil)
			compact.Reset()
		default:
			// Unknown record type, probably from a future Prometheus version.
			continue
		}
		if len(buf[start:]) == 0 {
			continue // All contents discarded.
		}
		recs = append(recs, buf[start:])

		if len(buf) > flushThreshold {
			if err := cp.Log(recs...); err != nil {
				return nil, fmt.Errorf("flush records: %w", err)
			}
			buf, recs = buf[:0], recs[:0]
		}
	}
	// If we hit any corruption during checkpointing, repairing is not an option.
	// The head won't know which series records are lost.
	if r.Err() != nil {
		return nil, fmt.Errorf("read segments: %w", r.Err())
	}

	// Flush remaining records.
	if err := cp.Log(recs...); err != nil {
		return nil, fmt.Errorf("flush records: %w", err)
	}

	// Flush the reduced metadata of each series, in bounded records: native
	// histories as overrides in compact records, whose dictionaries hold each
	// distinct value once, and legacy values as legacy entries. A trusted
	// reduction's provenance marker precedes its compact records, so that
	// readers trust them. Every series record precedes it, so only a handoff
	// can declare a current incarnation other than the latest series record's.
	// A winner was declared by a kept handoff for it. In an unmarked checkpoint,
	// which only an untrusted reduction writes and so holds no native history,
	// a winner's handoff is empty, so that readers take only its declaration.
	winners := labelSets.winners()
	metadata = metadata[:0]
	marked := reduceNativeMetadata && trust.Trusted()
	if marked {
		if err := cp.Log(enc.NativeMetadataMarker(record.NativeMetadataProvenance, buf[:0])); err != nil {
			return nil, fmt.Errorf("flush provenance marker: %w", err)
		}
		stats.Provenance = true
	}
	var (
		overrides      []record.RefCompactNativeMetadata
		overridePoints []record.CompactNativeMetadataPoint
		values         []record.NativeMetadataValue
		valueIndices   = map[record.NativeMetadataValue]uint32{}
	)
	flushOverrides := func() error {
		buf = enc.CompactNativeMetadata(values, overrides, buf[:0])
		if err := cp.Log(buf); err != nil {
			return fmt.Errorf("flush metadata records: %w", err)
		}
		clear(overrides)
		clear(values)
		clear(valueIndices)
		overrides, overridePoints, values = overrides[:0], overridePoints[:0], values[:0]
		return nil
	}
	flushLegacy := func() error {
		if err := cp.Log(enc.Metadata(metadata, buf[:0])); err != nil {
			return fmt.Errorf("flush metadata records: %w", err)
		}
		clear(metadata)
		metadata = metadata[:0]
		return nil
	}
	for ref := range winners {
		if _, ok := reducedMetadata[ref]; !ok {
			reducedMetadata[ref] = checkpointMetadata{}
		}
	}
	for ref, reduced := range reducedMetadata {
		_, winner := winners[ref]
		if winner {
			stats.Handoffs++
		}
		if reduced.native || winner {
			entry := record.RefCompactNativeMetadata{Ref: ref, Kind: record.NativeMetadataOverride, Truncated: reduced.Truncated, Handoff: winner}
			points := reduced.AppendPoints(metadataValues[:0])
			if !marked && reduced.Metadata != nil {
				// Readers skip the compact entries of an unmarked checkpoint,
				// but always apply legacy entries: the legacy value stays a
				// legacy entry, and the handoff only declares.
				metadata = append(metadata, record.RefMetadata{Ref: ref, Type: record.GetMetricType(reduced.Metadata.Type), Unit: reduced.Metadata.Unit, Help: reduced.Metadata.Help})
				if len(metadata) == metadataBatchSize {
					if err := flushLegacy(); err != nil {
						return nil, err
					}
				}
				points, entry.Truncated = nil, false
			}
			start := len(overridePoints)
			for _, p := range points {
				v := record.NativeMetadataValue{Type: record.GetMetricType(p.Metadata.Type), Unit: p.Metadata.Unit, Help: p.Metadata.Help}
				index, ok := valueIndices[v]
				if !ok {
					index = uint32(len(values))
					valueIndices[v] = index
					values = append(values, v)
				}
				overridePoints = append(overridePoints, record.CompactNativeMetadataPoint{EffectiveFrom: p.EffectiveFrom, Value: index})
			}
			entry.Points = overridePoints[start:len(overridePoints):len(overridePoints)]
			overrides = append(overrides, entry)
			if len(overrides) == metadataBatchSize {
				if err := flushOverrides(); err != nil {
					return nil, err
				}
			}
			continue
		}
		metadata = append(metadata, record.RefMetadata{Ref: ref, Type: record.GetMetricType(reduced.Metadata.Type), Unit: reduced.Metadata.Unit, Help: reduced.Metadata.Help})
		if len(metadata) == metadataBatchSize {
			if err := flushLegacy(); err != nil {
				return nil, err
			}
		}
	}
	if len(overrides) > 0 {
		if err := flushOverrides(); err != nil {
			return nil, err
		}
	}
	if len(metadata) > 0 {
		if err := flushLegacy(); err != nil {
			return nil, err
		}
	}

	if err := cp.Close(); err != nil {
		return nil, fmt.Errorf("close checkpoint: %w", err)
	}

	// Sync temporary directory before rename.
	df, err := fileutil.OpenDir(cpdirtmp)
	if err != nil {
		return nil, fmt.Errorf("open temporary checkpoint directory: %w", err)
	}
	if err := df.Sync(); err != nil {
		df.Close()
		return nil, fmt.Errorf("sync temporary checkpoint directory: %w", err)
	}
	if err = df.Close(); err != nil {
		return nil, fmt.Errorf("close temporary checkpoint directory: %w", err)
	}

	if err := fileutil.Replace(cpdirtmp, cpdir); err != nil {
		return nil, fmt.Errorf("rename checkpoint directory: %w", err)
	}

	return stats, nil
}

// checkpointPrefix is the prefix used for checkpoint files.
const checkpointPrefix = "checkpoint."

func CheckpointDir(dir string, i int) string {
	return filepath.Join(dir, fmt.Sprintf(checkpointPrefix+"%08d", i))
}

type checkpointRef struct {
	name  string
	index int
}

func listCheckpoints(dir string) (refs []checkpointRef, err error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for i := range files {
		fi := files[i]
		if !strings.HasPrefix(fi.Name(), checkpointPrefix) {
			continue
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("checkpoint %s is not a directory", fi.Name())
		}
		idx, err := strconv.Atoi(fi.Name()[len(checkpointPrefix):])
		if err != nil {
			continue
		}

		refs = append(refs, checkpointRef{name: fi.Name(), index: idx})
	}

	slices.SortFunc(refs, func(a, b checkpointRef) int {
		return a.index - b.index
	})

	return refs, nil
}

func isTempDir(fi fs.DirEntry) bool {
	return strings.HasPrefix(fi.Name(), checkpointPrefix) && strings.HasSuffix(fi.Name(), CheckpointTempFileSuffix)
}
