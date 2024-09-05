// Copyright 2018 The Prometheus Authors
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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/record"

	"github.com/prometheus/prometheus/tsdb/fileutil"
)

const (
	ProgressMarkerDirName string = "remote_write"
	ProgressMarkerFileExt string = ".json"
	unknownSegment        int    = -1
)

// ProgressMarker  add versioning? TODO.
type ProgressMarker struct {
	// TODO:
	// FirstSegment int `json:"firstSegmentIndex"`
	// LastSegment int `json:"lastSegmentIndex"`

	SegmentIndex int `json:"segmentIndex"`

	// Offset of read data into the segment.
	// Record at the offset is already processed.
	Offset int64 `json:"offset"`
}

func (pm *ProgressMarker) String() string {
	return fmt.Sprintf("%08d:%d", pm.SegmentIndex, pm.Offset)
}

func (pm *ProgressMarker) isUnknown() bool {
	return pm.SegmentIndex == unknownSegment
}

func (w *StatefulWatcher) progressMarkerFilePath() string {
	return filepath.Join(w.walDir, ProgressMarkerDirName, w.name+ProgressMarkerFileExt)
}

// TODO: copied from block.go.
func (w *StatefulWatcher) readProgressFile() error {
	b, err := os.ReadFile(w.progressMarkerFilePath())
	if err != nil {
		return err
	}
	var p ProgressMarker

	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}

	w.progressMarker = &p
	return nil
}

// TODO: copied from block.go.
func (w *StatefulWatcher) writeProgressFile() (int64, error) {
	// Make any changes to the file appear atomic.
	path := w.progressMarkerFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return 0, err
	}
	tmp := path + ".tmp"
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			level.Error(w.logger).Log("msg", "Remove tmp file", "err", err.Error())
		}
	}()

	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}

	jsonProgress, err := json.MarshalIndent(w.progressMarker, "", "\t")
	if err != nil {
		return 0, err
	}

	n, err := f.Write(jsonProgress)
	if err != nil {
		return 0, errors.Join(err, f.Close())
	}

	// Force the kernel to persist the file on disk to avoid data loss if the host crashes.
	// TODO: really need to Sync??
	if err := f.Sync(); err != nil {
		return 0, errors.Join(err, f.Close())
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	return int64(n), fileutil.Replace(tmp, path)
}

func (w *StatefulWatcher) deleteProgressFile() error {
	return os.Remove(w.progressMarkerFilePath())
}

type segmentsRange struct {
	firstSegment   int
	currentSegment int
	lastSegment    int
}

func (sr *segmentsRange) String() string {
	return fmt.Sprintf("range: %08d to %08d (current: %08d)", sr.firstSegment, sr.lastSegment, sr.currentSegment)
}

func (sr *segmentsRange) isUnknown() bool {
	return sr.firstSegment == unknownSegment || sr.currentSegment == unknownSegment || sr.lastSegment == unknownSegment
}

// StatefulWatcher watches the TSDB WAL for a given WriteTo.
// Unlike Watcher, it keeps track of its progress and resumes where it left off.
type StatefulWatcher struct {
	*Watcher

	// seriesOnly indicates if the watcher is currently only processing series samples
	// to build its state of the world, or if it has started forwarding data as well.
	seriesOnly     bool
	progressMarker *ProgressMarker
	segmentsState  *segmentsRange
}

// NewStatefulWatcher creates a new stateful WAL watcher for a given WriteTo.
func NewStatefulWatcher(metrics *WatcherMetrics, readerMetrics *LiveReaderMetrics, logger log.Logger, name string, writer WriteTo, dir string, sendExemplars, sendHistograms, sendMetadata bool) *StatefulWatcher {
	if name == "" {
		panic("name should be set as it's used as an indentifier.")
	}

	w := NewWatcher(metrics, readerMetrics, logger, name, writer, dir, sendExemplars, sendHistograms, sendMetadata)
	return &StatefulWatcher{
		Watcher: w,
		progressMarker: &ProgressMarker{
			SegmentIndex: unknownSegment,
		},
		segmentsState: &segmentsRange{
			firstSegment:   unknownSegment,
			currentSegment: unknownSegment,
			lastSegment:    unknownSegment,
		},
	}
}

func (w *StatefulWatcher) SetStartTime(t time.Time) {
	//
}

// Start the Watcher.
func (w *StatefulWatcher) Start() {
	w.setMetrics()

	// Look for previous persisted progress marker.
	err := w.readProgressFile()
	switch {
	case err != nil && os.IsNotExist(err):
		level.Debug(w.logger).Log(
			"msg", "previous progress marker was not found",
			"name", w.name,
		)
	case err != nil:
		level.Warn(w.logger).Log(
			"msg", "progress marker file could not be read, it will be reset",
			"file", w.progressMarkerFilePath(),
		)
	}
	// create the corresponding file as soon as possible for better visibility.
	_, err = w.writeProgressFile()
	if err != nil {
		// TODO
		panic("Return error?")
	}

	level.Info(w.logger).Log("msg", "Starting WAL watcher")
	go w.loop()
}

// Stop the Watcher.
func (w *StatefulWatcher) Stop() {
	close(w.quit)
	<-w.done

	// Persist the progressMarker marker.
	_, err := w.writeProgressFile()
	if err != nil {
		// TODO
		panic("Return error?")
	}

	// Records read metric has series and samples.
	if w.metrics != nil {
		w.metrics.recordsRead.DeleteLabelValues(w.name, "series")
		w.metrics.recordsRead.DeleteLabelValues(w.name, "samples")
		w.metrics.recordDecodeFails.DeleteLabelValues(w.name)
		w.metrics.currentSegment.DeleteLabelValues(w.name)
		// TODO: add marker progress related metrics, segment and offset
	}

	level.Info(w.logger).Log("msg", "WAL watcher stopped", "name", w.name)
}

func (w *StatefulWatcher) PurgeState() {
	<-w.done

	if err := w.deleteProgressFile(); err != nil {
		level.Error(w.logger).Log("msg", "Failed to delete the progress marker file", "file", w.progressMarkerFilePath())
	}
}

func (w *StatefulWatcher) loop() {
	defer close(w.done)

	// We may encounter failures processing the WAL; we should wait and retry.
	// TODO: should we increment the current segment,just skip the corrupted record or just keep failling?
	for !isClosed(w.quit) {
		if err := w.Run(); err != nil {
			level.Error(w.logger).Log("msg", "Error reading WAL", "err", err)
		}

		select {
		case <-w.quit:
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// syncOnIteration reacts to changes in the WAL segments (segment addition, deletion) to ensure appropriate segments are considered.
// It ensures that on each iteration:
// firstSegment <= w.progressMarker.segmentIndex <= lastSegment
// and
// firstSegment <= currentSegment <= lastSegment.
// When currentSegment is adjusted, the appropriate Checkpoint is read if available.
func (w *StatefulWatcher) syncOnIteration() error {
	// Look for the last segment.
	_, lastSegment, err := Segments(w.walDir)
	if err != nil {
		return fmt.Errorf("Segments: %w", err)
	}

	// Look for the first segment after the last checkpoint.
	lastCheckpoint, checkpointIndex, err := LastCheckpoint(w.walDir)
	if err != nil && !errors.Is(err, record.ErrNotFound) {
		return fmt.Errorf("LastCheckpoint: %w", err)
	}
	// Decision is made later whether the Checkpoint needs to be read.
	checkpointAvailable := err == nil
	firstSegment, err := w.findSegmentForIndex(checkpointIndex)
	if err != nil {
		return err
	}

	level.Debug(w.logger).Log(
		"msg", "syncOnIteration",
		"prevSegmentsState", w.segmentsState,
		"prevProgressMarker", w.progressMarker,
		"firstSegment", firstSegment,
		"lastSegment", lastSegment,
		"lastCheckpoint", lastCheckpoint,
		"checkpointIndex", checkpointIndex,
	)

	// Segments are identified by their names, if nothing changed, return early.
	if firstSegment == w.segmentsState.firstSegment && lastSegment >= w.segmentsState.lastSegment {
		w.segmentsState.lastSegment = lastSegment
		return nil
	}

	prevProgressMarker := *(w.progressMarker)
	prevSegmentsState := *(w.segmentsState)

	readCheckpoint := func() error {
		// If the current segment is changed, reading the Checkpoint is needed.
		if !checkpointAvailable || w.segmentsState.currentSegment == prevSegmentsState.currentSegment {
			return nil
		}
		if err = w.readCheckpoint(lastCheckpoint, checkpointIndex, (*StatefulWatcher).readRecords); err != nil {
			return fmt.Errorf("readCheckpoint: %w", err)
		}
		w.lastCheckpoint = lastCheckpoint
		return nil
	}

	// First iteration.
	if w.segmentsState.isUnknown() {
		switch {
		case w.progressMarker.isUnknown():
			// start from the last segment if progress marker is unknown.
			w.setProgress(lastSegment, 0)
		case prevProgressMarker.SegmentIndex < firstSegment:
			//   M
			//        [* * * *
			//         ^
			//         M'
			w.setProgress(firstSegment, 0)
		case prevProgressMarker.SegmentIndex > lastSegment:
			//             M
			//   * * * *]
			//         ^
			//         M'
			w.setProgress(lastSegment, 0)
		}
		w.setSegmentsState(firstSegment, firstSegment, lastSegment)

		return readCheckpoint()
	}

	// Adjust on the first segment changes.
	if firstSegment < prevSegmentsState.firstSegment {
		//        [* * * *                        [* * * *
		//         ^                               ^
		//         F                               F
		//      [* * * * *              [* * * * * * * * *
		//       ^                       ^
		//    F'=C'=M'                F'=C'=M'
		level.Info(w.logger).Log(
			"msg", "Segments were added before the previous first segment",
			"prevSegmentsState", &prevSegmentsState,
			"firstSegment", firstSegment,
		)
		w.setProgress(firstSegment, 0)
		w.segmentsState.currentSegment = firstSegment
	}
	if firstSegment > prevSegmentsState.firstSegment {
		if firstSegment <= prevProgressMarker.SegmentIndex {
			//  [* * * * *        [* * * * *
			//       ^                 ^
			//       M                 M
			//    [* * * *            [* * *
			//       ^                 ^
			//       M'                M'
			level.Debug(w.logger).Log(
				"msg", "Segments whose data was read and forwarded were removed",
				"prevSegmentsState", &prevSegmentsState,
				"prevProgressMarker", &prevProgressMarker,
				"firstSegment", firstSegment,
			)
		} else {
			//  [* * * * * * *           [* * * * *
			//       ^                    ^
			//       M                    M
			//        [* * * *             [* * * *
			//         ^                    ^
			//         M'                   M'
			w.setProgress(firstSegment, 0)
			level.Warn(w.logger).Log(
				"msg", "Segments were removed before the watcher ensured all data in them was read and forwarded",
				"prevSegmentsState", &prevSegmentsState,
				"prevProgressMarker", &prevProgressMarker,
				"firstSegment", firstSegment,
			)
		}

		if firstSegment <= prevSegmentsState.currentSegment {
			//  [* * * * *        [* * * * * *
			//       ^                 ^
			//       C                 C
			//    [* * * *            [* * * *
			//       ^                 ^
			//       C'                C'
			level.Debug(w.logger).Log(
				"msg", "Segments that were already replayed were removed",
				"prevSegmentsState", &prevSegmentsState,
				"firstSegment", firstSegment,
			)
		} else {
			//  [* * * * * * *              [* * * * * *
			//       ^                       ^
			//       C                       C
			//          [* * *                [* * * * *
			//           ^                     ^
			//           C'                    C'
			w.segmentsState.currentSegment = firstSegment
			level.Warn(w.logger).Log(
				"msg", "Segments were removed before they could be read",
				"prevSegmentsState", &prevSegmentsState,
				"firstSegment", firstSegment,
			)
		}
	}
	if firstSegment > prevSegmentsState.lastSegment {
		//   * * * * *]
		//           ^
		//           L
		//                [* * * *
		//                 ^
		//                 F'
		level.Debug(w.logger).Log(
			"msg", "All segments are subsequent to the previous last segment",
			"prevSegmentsState", &prevSegmentsState,
			"firstSegment", firstSegment,
		)
	}

	// Adjust on last segment changes.
	if lastSegment > prevSegmentsState.lastSegment {
		//  * * * * *]
		//          ^
		//          F
		//  * * * * * * *]
		//              ^
		//              F'
		level.Debug(w.logger).Log(
			"msg", "New segments were added after the previous last segment",
			"prevSegmentsState", &prevSegmentsState,
			"lastSegment", lastSegment,
		)
	}
	if lastSegment < prevSegmentsState.lastSegment {
		if lastSegment >= prevProgressMarker.SegmentIndex {
			//       * * * * *]        * * * * *]
			//           ^                   ^
			//           M                   M
			//       * * * *]          * * * *]
			//           ^                   ^
			//           M'                  M'
			level.Warn(w.logger).Log(
				"msg", "Segments were removed before the watcher ensured all data in them was read and forwarded",
				"prevSegmentsState", &prevSegmentsState,
				"prevProgressMarker", &prevProgressMarker,
				"lastSegment", lastSegment,
			)
		} else {
			if firstSegment >= prevSegmentsState.firstSegment {
				//  [* * * * * * *]
				//             ^
				//             M
				//  [* * * *]
				//         ^
				//         M'
				w.setProgress(lastSegment, 0)
			}
			level.Debug(w.logger).Log(
				"msg", "The progress marker points to a segment beyond the last segment",
				"prevSegmentsState", &prevSegmentsState,
				"prevProgressMarker", &prevProgressMarker,
				"lastSegment", lastSegment,
			)
		}

		if lastSegment >= prevSegmentsState.currentSegment {
			//       * * * * *]        * * * * *]
			//           ^                   ^
			//           C                   C
			//       * * * *]          * * * *]
			//           ^                   ^
			//           C'                  C'
			level.Warn(w.logger).Log(
				"msg", "Segments were removed before they could be read",
				"prevSegmentsState", &prevSegmentsState,
				"lastSegment", lastSegment,
			)
		} else {
			if firstSegment >= prevSegmentsState.firstSegment {
				//  [* * * * * * *]
				//             ^
				//             C
				//  [* * * *]
				//         ^
				//         C'
				w.segmentsState.currentSegment = lastSegment
			}
			level.Debug(w.logger).Log(
				"msg", "The segment to read is beyond the last segment",
				"prevSegmentsState", &prevSegmentsState,
				"prevProgressMarker", &prevProgressMarker,
				"lastSegment", lastSegment,
			)
		}
	}
	if lastSegment < prevSegmentsState.firstSegment {
		//                 [* * * * *
		//                  ^
		//                  F
		//   * * * * *]
		//           ^
		//           L'
		level.Debug(w.logger).Log(
			"msg", "All segments are prior to the previous first segment",
			"prevSegmentsState", &prevSegmentsState,
			"lastSegment", lastSegment,
		)
	}

	w.setSegmentsState(firstSegment, w.segmentsState.currentSegment, lastSegment)
	return readCheckpoint()
}

// Run the watcher, which will tail the last WAL segment until the quit channel is closed
// or an error case is hit.
func (w *StatefulWatcher) Run() error {
	level.Info(w.logger).Log("msg", "Replaying WAL")

	for !isClosed(w.quit) {
		// The watcher will decide when to start forwarding data while reading the segment.
		w.seriesOnly = true

		if err := w.syncOnIteration(); err != nil {
			return err
		}
		w.currentSegmentMetric.Set(float64(w.segmentsState.currentSegment))

		switch {
		case w.progressMarker.SegmentIndex <= w.segmentsState.currentSegment && w.segmentsState.currentSegment < w.segmentsState.lastSegment:
			level.Debug(w.logger).Log(
				"msg", "Reading a segment subsequent or equal to where the watcher left off",
				"segmentsState", w.segmentsState,
				"progressMarker", w.progressMarker,
			)
			if err := w.readSegmentStrict(w.walDir, w.segmentsState.currentSegment, (*StatefulWatcher).readRecords); err != nil {
				return err
			}
		case w.segmentsState.currentSegment < w.progressMarker.SegmentIndex:
			level.Debug(w.logger).Log(
				"msg", "Reading a segment prior to where the watcher left off",
				"segmentsState", w.segmentsState,
				"progressMarker", w.progressMarker,
			)
			w.readSegmentIgnoreErrors(w.walDir, w.segmentsState.currentSegment, (*StatefulWatcher).readRecords)
		default:
			level.Debug(w.logger).Log(
				"msg", "Reading and tailing the last segment",
				"segmentsState", w.segmentsState,
				"progressMarker", w.progressMarker,
			)
			if err := w.watch(w.segmentsState.currentSegment); err != nil {
				return err
			}
		}

		// For testing: stop when you hit a specific segment.
		if w.segmentsState.currentSegment == w.MaxSegment {
			return nil
		}

		w.segmentsState.currentSegment++
	}
	return nil
}

func (w *StatefulWatcher) watch(segmentIndex int) error {
	segment, err := OpenReadSegment(SegmentName(w.walDir, segmentIndex))
	if err != nil {
		return err
	}
	defer segment.Close()

	reader := NewLiveReader(w.logger, w.readerMetrics, segment)
	// Don't wait for tickers to process records that are already in the segment.
	err = w.readRecordsIgnoreEOF(reader, segmentIndex)
	if err != nil {
		return err
	}

	checkpointTicker := time.NewTicker(w.checkpointPeriod)
	defer checkpointTicker.Stop()

	segmentTicker := time.NewTicker(w.segmentCheckPeriod)
	defer segmentTicker.Stop()

	readTicker := time.NewTicker(w.readTimeout)
	defer readTicker.Stop()

	gcSem := make(chan struct{}, 1)
	for {
		select {
		case <-w.quit:
			return nil

		case <-checkpointTicker.C:
			// Periodically check if there is a new checkpoint so we can garbage
			// collect labels. As this is considered an optimisation, we ignore
			// errors during checkpoint processing. Doing the process asynchronously
			// allows the current WAL segment to be processed while reading the
			// checkpoint.
			select {
			case gcSem <- struct{}{}:
				go func() {
					defer func() {
						<-gcSem
					}()
					if err := w.garbageCollectSeries(segmentIndex); err != nil {
						level.Warn(w.logger).Log("msg", "Error process checkpoint", "err", err)
					}
				}()
			default:
				// Currently doing a garbage collection, try again later.
			}

		// if a newer segment is produced, read the current one until the end and move on.
		case <-segmentTicker.C:
			_, last, err := Segments(w.walDir)
			if err != nil {
				return fmt.Errorf("Segments: %w", err)
			}

			if last > segmentIndex {
				return w.readRecordsIgnoreEOF(reader, segmentIndex)
			}

		// we haven't read due to a notification in quite some time, try reading anyways
		case <-readTicker.C:
			level.Debug(w.logger).Log("msg", "Watcher is reading the WAL due to timeout, haven't received any write notifications recently", "timeout", w.readTimeout)
			err := w.readRecordsIgnoreEOF(reader, segmentIndex)
			if err != nil {
				return err
			}
			// reset the ticker so we don't read too often
			readTicker.Reset(w.readTimeout)

		case <-w.readNotify:
			err := w.readRecordsIgnoreEOF(reader, segmentIndex)
			if err != nil {
				return err
			}
			// reset the ticker so we don't read too often
			readTicker.Reset(w.readTimeout)
		}
	}
}

type recordsReadFn func(w *StatefulWatcher, r *LiveReader, segmentIndex int) error

// readSegmentStrict reads the segment and returns an error if the segment was not read completely.
func (w *StatefulWatcher) readSegmentStrict(segmentDir string, segmentIndex int, readFn recordsReadFn) error {
	segment, err := OpenReadSegment(SegmentName(segmentDir, segmentIndex))
	if err != nil {
		return fmt.Errorf("unable to open segment: %w", err)
	}
	defer segment.Close()

	size, err := getSegmentSize(segmentDir, segmentIndex)
	if err != nil {
		return fmt.Errorf("getSegmentSize: %w", err)
	}

	reader := NewLiveReader(w.logger, w.readerMetrics, segment)
	err = readFn(w, reader, segmentIndex)

	if errors.Is(err, io.EOF) {
		if reader.Offset() == size {
			return nil
		}
		return fmt.Errorf("readSegmentStrict wasn't able to read all data from the segment %08d, size: %d, totalRead: %d", segmentIndex, size, reader.Offset())
	}
	return err
}

// readSegmentIgnoreErrors reads the segment, it logs and ignores all eventuels errors.
// TODO: ducument why we need this, https://github.com/prometheus/prometheus/pull/5214/files
func (w *StatefulWatcher) readSegmentIgnoreErrors(segmentDir string, segmentIndex int, readFn recordsReadFn) {
	err := w.readSegmentStrict(segmentDir, segmentIndex, readFn)
	if err != nil {
		level.Warn(w.logger).Log("msg", "Ignoring error reading the segment, may have dropped data", "segment", segmentIndex, "err", err)
	}
}

func (w *StatefulWatcher) checkProgressMarkerReached(segmentIndex int, offset int64) {
	if !w.seriesOnly {
		return
	}
	// Check if we arrived back where we left off.
	// offset > w.progressMarker.Offset as the segment may have been altered and replaced, which is not detected nor supported.
	if segmentIndex > w.progressMarker.SegmentIndex || (segmentIndex == w.progressMarker.SegmentIndex && offset > w.progressMarker.Offset) {
		// make the watcher start forwarding data.
		w.seriesOnly = false
	}
}

func (w *StatefulWatcher) setSegmentsState(firstSegment, currentSegment, lastSegment int) {
	w.segmentsState.firstSegment = firstSegment
	w.segmentsState.currentSegment = currentSegment
	w.segmentsState.lastSegment = lastSegment
}

func (w *StatefulWatcher) setProgress(segmentIndex int, offset int64) {
	w.progressMarker.SegmentIndex = segmentIndex
	w.progressMarker.Offset = offset
}

func (w *StatefulWatcher) UpdateProgress(segmentIndex int, offset int64) {
	// The watcher isn't forwarding data yet.
	if w.seriesOnly {
		return
	}
	w.setProgress(segmentIndex, offset)
}

// Read records and pass the details to w.writer.
// Also used with readCheckpoint - implements recordsReadFn.
func (w *StatefulWatcher) readRecords(r *LiveReader, segmentIndex int) error {
	var (
		dec             = record.NewDecoder(labels.NewSymbolTable()) // One table per WAL segment means it won't grow indefinitely.
		series          []record.RefSeries
		samples         []record.RefSample
		exemplars       []record.RefExemplar
		histograms      []record.RefHistogramSample
		floatHistograms []record.RefFloatHistogramSample
		metadata        []record.RefMetadata
	)

	for r.Next() && !isClosed(w.quit) {
		w.checkProgressMarkerReached(segmentIndex, r.Offset())
		rec := r.Record()
		w.recordsReadMetric.WithLabelValues(dec.Type(rec).String()).Inc()

		switch dec.Type(rec) {
		case record.Series:
			series, err := dec.Series(rec, series[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			w.writer.StoreSeries(series, segmentIndex)

		case record.Samples:
			if w.seriesOnly {
				break
			}
			samples, err := dec.Samples(rec, samples[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			if len(samples) > 0 {
				w.writer.Append(samples)
			}

		case record.Exemplars:
			// Skip if experimental "exemplars over remote write" is not enabled.
			if !w.sendExemplars || w.seriesOnly {
				break
			}

			exemplars, err := dec.Exemplars(rec, exemplars[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			if len(exemplars) > 0 {
				w.writer.AppendExemplars(exemplars)
			}
		case record.HistogramSamples:
			// Skip if experimental "histograms over remote write" is not enabled.
			if !w.sendHistograms || w.seriesOnly {
				break
			}
			histograms, err := dec.HistogramSamples(rec, histograms[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			if len(histograms) > 0 {
				w.writer.AppendHistograms(histograms)
			}
		case record.FloatHistogramSamples:
			// Skip if experimental "histograms over remote write" is not enabled.
			if !w.sendHistograms || w.seriesOnly {
				break
			}
			floatHistograms, err := dec.FloatHistogramSamples(rec, floatHistograms[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			if len(floatHistograms) > 0 {
				w.writer.AppendFloatHistograms(floatHistograms)
			}
		case record.Metadata:
			if !w.sendMetadata {
				break
			}
			meta, err := dec.Metadata(rec, metadata[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			w.writer.StoreMetadata(meta)
		case record.Tombstones:

		default:
			// Could be corruption, or reading from a WAL from a newer Prometheus.
			w.recordDecodeFailsMetric.Inc()
		}

		// TODO: check AppendXXX returned values.
		// In the future, the shards will influence this.
		w.UpdateProgress(segmentIndex, r.Offset())
	}
	if err := r.Err(); err != nil {
		return fmt.Errorf("readRecods from segment %08d: %w", segmentIndex, err)
	}
	return nil
}

func (w *StatefulWatcher) readRecordsIgnoreEOF(r *LiveReader, segmentIndex int) error {
	if err := w.readRecords(r, segmentIndex); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// Read all the segments from a Checkpoint directory.
func (w *StatefulWatcher) readCheckpoint(checkpointDir string, checkpointIndex int, readFn recordsReadFn) error {
	level.Debug(w.logger).Log("msg", "Reading checkpoint", "dir", checkpointDir)
	// Records from these segments should be marked as originating from the Checkpoint index itself.
	indexAwareReadFn := func(w *StatefulWatcher, r *LiveReader, _ int) error {
		return readFn(w, r, checkpointIndex)
	}
	// Read all segments in the checkpoint dir.
	segs, err := listSegments(checkpointDir)
	if err != nil {
		return fmt.Errorf("Unable to get segments in checkpoint dir: %w", err)
	}
	for _, segRef := range segs {
		err := w.readSegmentStrict(checkpointDir, segRef.index, indexAwareReadFn)
		if err != nil {
			return fmt.Errorf("readSegmentStrict from the checkpoint %s: %w", checkpointDir, err)
		}
	}

	level.Debug(w.logger).Log("msg", "Read series references from checkpoint", "checkpoint", checkpointDir)
	return nil
}
