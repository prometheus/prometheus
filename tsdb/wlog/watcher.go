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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/tsdb/record"
)

const (
	// TODO(bwplotka): Checking every 100ms feels too frequent. It might be enough
	// to check on notify AND with emergency 15s read only.
	segmentCheckPeriod = 100 * time.Millisecond
	consumer           = "consumer"
)

// WriteTo is an interface used by the Watcher to send the samples it's read
// from the WAL on to somewhere else. Methods must be concurrency safe.
//
// All record.Ref* slices are only valid until each method finished, implementers
// must not try to reuse the underlying arrays.
type WriteTo interface {
	// Append should block until the samples are fully accepted,
	// whether enqueued in memory or successfully written to its final destination.
	// Once returned, the WAL Watcher will not attempt to pass that data again.
	Append([]record.RefSample) bool
	// AppendExemplars should block until the samples are fully accepted,
	// whether enqueued in memory or successfully written to its final destination.
	// Once returned, the WAL Watcher will not attempt to pass that data again.
	AppendExemplars([]record.RefExemplar) bool
	// AppendHistograms should block until the samples are fully accepted,
	// whether enqueued in memory or successfully written to its final destination.
	// Once returned, the WAL Watcher will not attempt to pass that data again.
	AppendHistograms([]record.RefHistogramSample) bool
	// AppendFloatHistograms should block until the samples are fully accepted,
	// whether enqueued in memory or successfully written to its final destination.
	// Once returned, the WAL Watcher will not attempt to pass that data again.
	AppendFloatHistograms([]record.RefFloatHistogramSample) bool

	StoreSeries([]record.RefSeries, int)
	StoreMetadata([]record.RefMetadata)

	// UpdateSeriesSegment is intended for GC.
	// First we call UpdateSeriesSegment on all the current series, then SeriesReset
	// is called to allow the deletion of all series created in a segment lower
	// than the argument.
	UpdateSeriesSegment([]record.RefSeries, int)
	// SeriesReset is intended for GC.
	// First we call UpdateSeriesSegment on all the current series, then SeriesReset
	// is called to allow the deletion of all series created in a segment lower
	// than the argument.
	SeriesReset(int)
}

// WriteNotified notifies the watcher that data has been written so that it can read.
type WriteNotified interface {
	Notify()
}

// WatcherMetrics allows sharing metrics across multiple watcher instances.
type WatcherMetrics struct {
	recordsRead           *prometheus.CounterVec
	recordDecodeFails     *prometheus.CounterVec
	samplesSentPreTailing *prometheus.CounterVec
	currentSegment        *prometheus.GaugeVec
	notificationsSkipped  *prometheus.CounterVec
}

// Watcher watches the TSDB WAL and writes the data to a given WriteTo.
// See Start and Watch for details.
type Watcher struct {
	name   string
	writer WriteTo
	logger *slog.Logger
	walDir string

	sendExemplars  bool
	sendHistograms bool
	sendMetadata   bool
	replayDone     bool

	lastCheckpoint string

	metrics                 *WatcherMetrics
	readerMetrics           *LiveReaderMetrics
	recordsReadMetric       *prometheus.CounterVec
	recordDecodeFailsMetric prometheus.Counter
	samplesSentPreTailing   prometheus.Counter
	currentSegmentMetric    prometheus.Gauge
	notificationsSkipped    prometheus.Counter

	readNotify chan struct{}
	quit       chan struct{}
	done       chan struct{}

	checkpointPeriod time.Duration
	readTimeout      time.Duration
}

func NewWatcherMetrics(reg prometheus.Registerer) *WatcherMetrics {
	m := &WatcherMetrics{
		recordsRead: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "prometheus",
				Subsystem: "wal_watcher",
				Name:      "records_read_total",
				Help:      "Number of records read by the WAL watcher from the WAL.",
			},
			[]string{consumer, "type"},
		),
		recordDecodeFails: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "prometheus",
				Subsystem: "wal_watcher",
				Name:      "record_decode_failures_total",
				Help:      "Number of records read by the WAL watcher that resulted in an error when decoding.",
			},
			[]string{consumer},
		),
		samplesSentPreTailing: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "prometheus",
				Subsystem: "wal_watcher",
				Name:      "samples_sent_pre_tailing_total",
				Help:      "Number of sample records read by the WAL watcher and sent to remote write during replay of existing WAL.",
			},
			[]string{consumer},
		),
		currentSegment: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "prometheus",
				Subsystem: "wal_watcher",
				Name:      "current_segment",
				Help:      "Current segment the WAL watcher is reading records from.",
			},
			[]string{consumer},
		),
		notificationsSkipped: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "prometheus",
				Subsystem: "wal_watcher",
				Name:      "notifications_skipped_total",
				Help:      "The number of WAL write notifications that the Watcher has skipped due to already being in a WAL read routine.",
			},
			[]string{consumer},
		),
	}
	return m
}

// NewWatcher creates a new WAL watcher that watches the for a given WriteTo.
func NewWatcher(
	metrics *WatcherMetrics,
	readerMetrics *LiveReaderMetrics,
	logger *slog.Logger,
	name string,
	writer WriteTo,
	dir string,
	sendExemplars, sendHistograms, sendMetadata bool,
) *Watcher {
	if logger == nil {
		logger = promslog.NewNopLogger()
	}
	if metrics == nil {
		metrics = NewWatcherMetrics(nil)
	}
	if readerMetrics == nil {
		readerMetrics = NewLiveReaderMetrics(nil)
	}
	return &Watcher{
		logger:         logger,
		writer:         writer,
		metrics:        metrics,
		readerMetrics:  readerMetrics,
		walDir:         filepath.Join(dir, "wal"),
		name:           name,
		sendExemplars:  sendExemplars,
		sendHistograms: sendHistograms,
		sendMetadata:   sendMetadata,

		readNotify:       make(chan struct{}),
		quit:             make(chan struct{}),
		done:             make(chan struct{}),
		checkpointPeriod: 5 * time.Second,
		readTimeout:      15 * time.Second,
	}
}

func (w *Watcher) Notify() {
	select {
	case w.readNotify <- struct{}{}:
		return
	default: // default so we can exit
		// we don't need a buffered channel or any buffering since
		// for each notification it recv's the watcher will read until EOF
		w.notificationsSkipped.Inc()
	}
}

// startMetrics initialize the mandatory shared watcher metrics.
// We do this here rather than in the constructor because of the ordering of
// creating Queue Managers's, stopping them, and then starting new ones in
// storage/remote/storage.go ApplyConfig.
func (w *Watcher) initMetrics() {
	w.recordsReadMetric = w.metrics.recordsRead.MustCurryWith(prometheus.Labels{consumer: w.name})
	w.recordDecodeFailsMetric = w.metrics.recordDecodeFails.WithLabelValues(w.name)
	w.samplesSentPreTailing = w.metrics.samplesSentPreTailing.WithLabelValues(w.name)
	w.currentSegmentMetric = w.metrics.currentSegment.WithLabelValues(w.name)
	w.notificationsSkipped = w.metrics.notificationsSkipped.WithLabelValues(w.name)
}

// Stop the Watcher and waits until it fully stops.
func (w *Watcher) Stop() {
	close(w.quit)
	<-w.done

	for _, t := range []record.Type{record.Series, record.Samples, record.Tombstones, record.Exemplars, record.MmapMarkers, record.Metadata, record.HistogramSamples, record.FloatHistogramSamples, record.CustomBucketsHistogramSamples, record.CustomBucketsFloatHistogramSamples} {
		w.metrics.recordsRead.DeleteLabelValues(w.name, t.String())
	}
	w.metrics.recordDecodeFails.DeleteLabelValues(w.name)
	w.metrics.samplesSentPreTailing.DeleteLabelValues(w.name)
	w.metrics.currentSegment.DeleteLabelValues(w.name)

	w.logger.Info("WAL watcher stopped", "queue", w.name)
}

// Start starts the routine that tails the WAL with time.Now start time, until
// the quit channel is closed. If the tailing returns error it retries with the
// error log. Non-series data. Read Watch for tailing logic details.
func (w *Watcher) Start() {
	w.initMetrics()
	w.logger.Info("Starting WAL watcher", "queue", w.name)

	go func() {
		defer close(w.done)

		// We may encounter failures processing the WAL; we should wait and retry.
		for !isClosed(w.quit) {
			if err := w.Watch(timestamp.FromTime(time.Now())); err != nil {
				w.logger.Error("error tailing WAL", "err", err)
			}

			select {
			case <-w.quit:
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
}

// Watch tails the WAL until the quit channel is closed or an error.
//
// Tailing logic writes the known types of WAL records to WriteTo interface.
// - Series are gathered from all the available WAL segments.
// - Other type of data is gathered only from the last segment and waits for the new data to come in.
// - For samples and histograms startT controls after what timestamp samples should be written to WriteTo.
// This allows retrying watching without reading overlapping data.
func (w *Watcher) Watch(startT int64) error {
	if metricsNotInitialized := w.recordsReadMetric == nil; metricsNotInitialized {
		w.initMetrics()
	}

	_, lastSegment, err := Segments(w.walDir)
	if err != nil {
		return fmt.Errorf("segments: %w", err)
	}
	w.replayDone = false
	w.logger.Info("Replaying WAL", "queue", w.name)

	// Backfill series from the checkpoint first if it exists.
	lastCheckpoint, checkpointIndex, err := LastCheckpoint(w.walDir)
	if err != nil && !errors.Is(err, record.ErrNotFound) {
		return fmt.Errorf("tsdb.LastCheckpoint: %w", err)
	}

	if err == nil {
		if err = w.readCheckpoint(lastCheckpoint, (*Watcher).readSegmentSeries); err != nil {
			return fmt.Errorf("readCheckpoint: %w", err)
		}
	}
	w.lastCheckpoint = lastCheckpoint

	currentSegment, err := w.findSegmentForIndex(checkpointIndex)
	if err != nil {
		return err
	}

	w.logger.Debug("Tailing WAL", "lastCheckpoint", strings.TrimPrefix(lastCheckpoint, w.walDir), "checkpointIndex", checkpointIndex, "currentSegment", currentSegment, "lastSegment", lastSegment)
	for !isClosed(w.quit) {
		w.currentSegmentMetric.Set(float64(currentSegment))
		w.logger.Debug("Processing segment", "currentSegment", currentSegment)
		if err := w.watchSegment(startT, currentSegment, currentSegment >= lastSegment); err != nil {
			return err
		}
		currentSegment++
	}
	return nil
}

// findSegmentForIndex finds the first segment greater than or equal to index.
func (w *Watcher) findSegmentForIndex(index int) (int, error) {
	refs, err := listSegments(w.walDir)
	if err != nil {
		return -1, err
	}

	for _, r := range refs {
		if r.index >= index {
			return r.index, nil
		}
	}
	return -1, errors.New("failed to find segment for index")
}

// watchSegment tails a single WAL segment.
// Tail parameter indicates that the reader is currently on a segment that is
// actively being written to and watcher should tail it until the quit channel
// is closed. If false, assume it's a full segment, and we're replaying it only
// to only cache the series records.
func (w *Watcher) watchSegment(startT int64, segmentNum int, tail bool) error {
	segment, err := OpenReadSegment(SegmentName(w.walDir, segmentNum))
	if err != nil {
		return err
	}
	defer segment.Close()

	reader := NewLiveReader(w.logger, w.readerMetrics, segment)

	if !tail {
		if err := handleFullSegmentPartialReads(w.readSegmentSeries(reader, segmentNum), reader, getSegmentSizeFn(w.walDir, segmentNum)); err != nil {
			// Ignore all errors reading to end of segment whilst replaying the WAL.
			w.logger.Warn("Ignoring error reading to end of segment, may have dropped data", "segment", segmentNum, "err", err)
		}
		return nil
	}

	// Always try to read from the new segment before we wait for emergency read timeout,
	// new segments or notifications. EOFs are not fatal as there might be other
	// routine writing to a segment.
	if err := w.readSegment(reader, startT, segmentNum); err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	checkpointTicker := time.NewTicker(w.checkpointPeriod)
	defer checkpointTicker.Stop()

	segmentTicker := time.NewTicker(segmentCheckPeriod)
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
					if err := w.garbageCollectSeries(segmentNum); err != nil {
						w.logger.Warn("Error process checkpoint", "err", err)
					}
				}()
			default:
				// Currently doing a garbage collect, try again later.
			}

		// If a newer segment is produced, read the current one until the end and return
		// so we can watch the next segment.
		case <-segmentTicker.C:
			_, last, err := Segments(w.walDir)
			if err != nil {
				return fmt.Errorf("segments: %w", err)
			}
			if last > segmentNum {
				// At this point we expect full segment, so handle LiveReader EOF that
				// are false (read more in handleFullSegmentPartialReads).
				if err := handleFullSegmentPartialReads(w.readSegment(reader, startT, segmentNum), reader, getSegmentSizeFn(w.walDir, segmentNum)); err != nil {
					return fmt.Errorf("read on a new segment: %w", err)
				}
				return nil
			}
			// No new segments, continue normal flow.
			continue

		// We haven't read due to a notification in quite some time, try reading anyway.
		case <-readTicker.C:
			w.logger.Debug("Watcher is reading the WAL due to timeout, haven't received any write notifications recently", "timeout", w.readTimeout)

			// EOFs are not fatal as there might be other routine writing to a segment.
			if err := w.readSegment(reader, startT, segmentNum); err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			readTicker.Reset(w.readTimeout)

		case <-w.readNotify:
			// EOFs are not fatal as there might be other routine writing to a segment.
			if err := w.readSegment(reader, startT, segmentNum); err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			readTicker.Reset(w.readTimeout)
		}
	}
}

func (w *Watcher) garbageCollectSeries(segmentNum int) error {
	dir, _, err := LastCheckpoint(w.walDir)
	if err != nil && !errors.Is(err, record.ErrNotFound) {
		return fmt.Errorf("tsdb.LastCheckpoint: %w", err)
	}

	if dir == "" || dir == w.lastCheckpoint {
		return nil
	}
	w.lastCheckpoint = dir

	index, err := checkpointNum(dir)
	if err != nil {
		return fmt.Errorf("error parsing checkpoint filename: %w", err)
	}

	if index >= segmentNum {
		w.logger.Debug("Current segment is behind the checkpoint, skipping reading of checkpoint", "current", fmt.Sprintf("%08d", segmentNum), "checkpoint", dir)
		return nil
	}

	w.logger.Debug("New checkpoint detected", "new", dir, "currentSegment", segmentNum)

	if err = w.readCheckpoint(dir, (*Watcher).readSegmentForGC); err != nil {
		return fmt.Errorf("readCheckpoint: %w", err)
	}

	// Clear series with a checkpoint or segment index # lower than the checkpoint we just read.
	w.writer.SeriesReset(index)
	return nil
}

// readSegmentSeries reads the series records into w.writer from a segment.
// It returns the EOF error if the segment is corrupted or partially written.
func (w *Watcher) readSegmentSeries(r *LiveReader, segmentNum int) error {
	var (
		dec    = record.NewDecoder(labels.NewSymbolTable()) // One table per WAL segment means it won't grow indefinitely.
		series []record.RefSeries
	)
	for r.Next() && !isClosed(w.quit) {
		rec := r.Record()
		w.recordsReadMetric.WithLabelValues(dec.Type(rec).String()).Inc()

		switch dec.Type(rec) {
		case record.Series:
			series, err := dec.Series(rec, series[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			w.writer.StoreSeries(series, segmentNum)
		case record.Unknown:
			// Could be corruption, or reading from a WAL from a newer Prometheus.
			w.recordDecodeFailsMetric.Inc()
		default:
			// We're not interested in other types of records.
		}
	}
	if err := r.Err(); err != nil {
		return fmt.Errorf("segment %d: %w", segmentNum, err)
	}
	return nil
}

// readSegment reads all known records into w.writer from a segment.
// It returns the EOF error if the segment is corrupted or partially written.
func (w *Watcher) readSegment(r *LiveReader, startT int64, segmentNum int) (err error) {
	var (
		// One table per WAL segment means it won't grow indefinitely.
		dec = record.NewDecoder(labels.NewSymbolTable())

		// TODO(bwplotka): Consider zeropools.
		series                []record.RefSeries
		samples               []record.RefSample
		samplesToSend         []record.RefSample
		exemplars             []record.RefExemplar
		histograms            []record.RefHistogramSample
		histogramsToSend      []record.RefHistogramSample
		floatHistograms       []record.RefFloatHistogramSample
		floatHistogramsToSend []record.RefFloatHistogramSample
		metadata              []record.RefMetadata
	)

	for r.Next() && !isClosed(w.quit) {
		rec := r.Record()
		w.recordsReadMetric.WithLabelValues(dec.Type(rec).String()).Inc()

		switch dec.Type(rec) {
		case record.Series:
			series, err = dec.Series(rec, series[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			w.writer.StoreSeries(series, segmentNum)

		case record.Samples:
			samples, err = dec.Samples(rec, samples[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			for _, s := range samples {
				if s.T > startT {
					if !w.replayDone {
						w.replayDone = true
						w.logger.Info("Done replaying WAL", "duration", time.Since(timestamp.Time(startT)))
					}
					samplesToSend = append(samplesToSend, s)
				}
			}
			if len(samplesToSend) > 0 {
				w.writer.Append(samplesToSend)
				samplesToSend = samplesToSend[:0]
			}

		case record.Exemplars:
			// Skip if experimental "exemplars over remote write" is not enabled.
			if !w.sendExemplars {
				break
			}
			exemplars, err = dec.Exemplars(rec, exemplars[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			w.writer.AppendExemplars(exemplars)

		case record.HistogramSamples, record.CustomBucketsHistogramSamples:
			// Skip if experimental "histograms over remote write" is not enabled.
			if !w.sendHistograms {
				break
			}
			histograms, err = dec.HistogramSamples(rec, histograms[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			for _, h := range histograms {
				if h.T > startT {
					if !w.replayDone {
						w.replayDone = true
						w.logger.Info("Done replaying WAL", "duration", time.Since(timestamp.Time(startT)))
					}
					histogramsToSend = append(histogramsToSend, h)
				}
			}
			if len(histogramsToSend) > 0 {
				w.writer.AppendHistograms(histogramsToSend)
				histogramsToSend = histogramsToSend[:0]
			}

		case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples:
			// Skip if experimental "histograms over remote write" is not enabled.
			if !w.sendHistograms {
				break
			}
			floatHistograms, err = dec.FloatHistogramSamples(rec, floatHistograms[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			for _, fh := range floatHistograms {
				if fh.T > startT {
					if !w.replayDone {
						w.replayDone = true
						w.logger.Info("Done replaying WAL", "duration", time.Since(timestamp.Time(startT)))
					}
					floatHistogramsToSend = append(floatHistogramsToSend, fh)
				}
			}
			if len(floatHistogramsToSend) > 0 {
				w.writer.AppendFloatHistograms(floatHistogramsToSend)
				floatHistogramsToSend = floatHistogramsToSend[:0]
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

		case record.Unknown:
			// Could be corruption, or reading from a WAL from a newer Prometheus.
			w.recordDecodeFailsMetric.Inc()

		default:
			// We're not interested in other types of records.
		}
	}
	if err := r.Err(); err != nil {
		return fmt.Errorf("segment %d: %w", segmentNum, err)
	}
	return nil
}

// readSegmentForGC goes through all series in a segment updating the segmentNum, so we can delete older series.
// It returns the EOF error if the segment is corrupted or partially written.
func (w *Watcher) readSegmentForGC(r *LiveReader, segmentNum int) error {
	var (
		dec    = record.NewDecoder(labels.NewSymbolTable()) // Needed for decoding; labels do not outlive this function.
		series []record.RefSeries
	)
	for r.Next() && !isClosed(w.quit) {
		rec := r.Record()
		w.recordsReadMetric.WithLabelValues(dec.Type(rec).String()).Inc()

		switch dec.Type(rec) {
		case record.Series:
			series, err := dec.Series(rec, series[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			w.writer.UpdateSeriesSegment(series, segmentNum)

		case record.Unknown:
			// Could be corruption, or reading from a WAL from a newer Prometheus.
			w.recordDecodeFailsMetric.Inc()

		default:
			// We're only interested in series.
		}
	}
	if err := r.Err(); err != nil {
		return fmt.Errorf("segment %d: %w", segmentNum, err)
	}
	return nil
}

type segmentReadFn func(w *Watcher, r *LiveReader, segmentNum int) error

// Read all the series records from a Checkpoint directory.
func (w *Watcher) readCheckpoint(checkpointDir string, readFn segmentReadFn) error {
	w.logger.Debug("Reading checkpoint", "dir", checkpointDir)
	index, err := checkpointNum(checkpointDir)
	if err != nil {
		return fmt.Errorf("checkpointNum: %w", err)
	}

	// Ensure we read the whole contents of every segment in the checkpoint dir.
	segs, err := listSegments(checkpointDir)
	if err != nil {
		return fmt.Errorf("unable to get segments checkpoint dir: %w", err)
	}
	for _, segRef := range segs {
		sr, err := OpenReadSegment(SegmentName(checkpointDir, segRef.index))
		if err != nil {
			return fmt.Errorf("unable to open segment: %w", err)
		}

		r := NewLiveReader(w.logger, w.readerMetrics, sr)
		err = handleFullSegmentPartialReads(readFn(w, r, index), r, getSegmentSizeFn(checkpointDir, segRef.index))
		sr.Close()
		if err != nil {
			return fmt.Errorf("readCheckpoint: %w", err)
		}
	}

	w.logger.Debug("Done reading series references from checkpoint", "checkpoint", checkpointDir)
	return nil
}

func checkpointNum(dir string) (int, error) {
	// Checkpoint dir names are in the format checkpoint.000001
	// dir may contain a hidden directory, so only check the base directory
	chunks := strings.Split(filepath.Base(dir), ".")
	if len(chunks) != 2 {
		return 0, fmt.Errorf("invalid checkpoint dir string: %s", dir)
	}

	result, err := strconv.Atoi(chunks[1])
	if err != nil {
		return 0, fmt.Errorf("invalid checkpoint dir string: %s", dir)
	}

	return result, nil
}

func isClosed(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

// handleFullSegmentPartialReads handles LiveReader derived errors in case of knowingly
// full segment read. This is needed because LiveReader always returns EOF, even
// for full, successful segment reads.
func handleFullSegmentPartialReads(err error, r *LiveReader, getSize func() (int64, error)) error {
	if err == nil {
		// LiveReader never returns non-nil errors, but handle this, might happen in the future.
		return nil
	}
	if !errors.Is(err, io.EOF) {
		return err
	}

	size, err := getSize()
	if err != nil {
		return err
	}
	if r.Offset() == size {
		return nil
	}
	return fmt.Errorf("expected to read the segment fully, but got EOF, may have dropped data; read %v/%v", r.Offset(), size)
}

func getSegmentSizeFn(dir string, segmentNum int) func() (int64, error) {
	return func() (int64, error) {
		fi, err := os.Stat(SegmentName(dir, segmentNum))
		if err != nil {
			return 0, fmt.Errorf("get segment size: %w", err)
		}
		return fi.Size(), nil
	}
}
