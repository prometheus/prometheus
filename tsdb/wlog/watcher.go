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
	segmentCheckPeriod = 100 * time.Millisecond
	consumer           = "consumer"
)

// WriteTo is an interface used by the Watcher to send the samples it's read
// from the WAL on to somewhere else.
//
// Implementations must:
// * Ensure it's safe for concurrent goroutine use.
// * Ensure slices are not reused after method calls.
type WriteTo interface {
	// Append and all the rest Append* methods should block until
	// the samples are fully accepted e.g. enqueued in memory.
	//
	// Once returned, the WAL Watcher will not attempt to pass that data again.
	Append([]record.RefSample) bool
	AppendExemplars([]record.RefExemplar) bool
	AppendHistograms([]record.RefHistogramSample) bool
	AppendFloatHistograms([]record.RefFloatHistogramSample) bool
	StoreSeries([]record.RefSeries, int)
	StoreMetadata([]record.RefMetadata)

	// UpdateSeriesSegment and SeriesReset are intended for
	// garbage-collection:
	// * First we call UpdateSeriesSegment on all current series.
	// * Then SeriesReset is called.
	UpdateSeriesSegment([]record.RefSeries, int)
	// SeriesReset is called to allow the deletion of all series
	// created in a segment lower than the argument.
	SeriesReset(int)
}

// WriteNotified notifies the watcher that data has been written so that it can read.
type WriteNotified interface {
	Notify()
}

type WatcherMetrics struct {
	reg                   prometheus.Registerer
	recordsRead           *prometheus.CounterVec
	recordDecodeFails     *prometheus.CounterVec
	samplesSentPreTailing *prometheus.CounterVec
	currentSegment        *prometheus.GaugeVec
	notificationsSkipped  *prometheus.CounterVec
}

// Watcher watches the TSDB WAL for a given WriteTo.
type Watcher struct {
	name           string
	writer         WriteTo
	recordBuf      *record.BuffersPool
	dec            record.Decoder
	logger         *slog.Logger
	walDir         string
	lastCheckpoint string
	sendExemplars  bool
	sendHistograms bool
	sendMetadata   bool
	metrics        *WatcherMetrics
	readerMetrics  *LiveReaderMetrics

	startTime      time.Time
	startTimestamp int64 // the start time as a Prometheus timestamp
	sendSamples    bool

	recordsReadMetric       *prometheus.CounterVec
	recordDecodeFailsMetric prometheus.Counter
	samplesSentPreTailing   prometheus.Counter
	currentSegmentMetric    prometheus.Gauge
	notificationsSkipped    prometheus.Counter

	readNotify       chan struct{}
	quit             chan struct{}
	done             chan struct{}
	checkpointPeriod time.Duration
	readTimeout      time.Duration

	// For testing, stop when we hit this segment.
	MaxSegment int
}

func NewWatcherMetrics(reg prometheus.Registerer) *WatcherMetrics {
	return &WatcherMetrics{
		reg: reg,
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
}

// Unregister unregisters metrics emitted by this instance.
func (m *WatcherMetrics) Unregister() {
	if m.reg == nil {
		return
	}

	m.reg.Unregister(m.recordsRead)
	m.reg.Unregister(m.recordDecodeFails)
	m.reg.Unregister(m.samplesSentPreTailing)
	m.reg.Unregister(m.currentSegment)
	m.reg.Unregister(m.notificationsSkipped)
}

// NewWatcher creates a new WAL watcher for a given WriteTo.
func NewWatcher(
	metrics *WatcherMetrics,
	readerMetrics *LiveReaderMetrics,
	logger *slog.Logger,
	name string,
	writer WriteTo,
	dir string,
	sendExemplars, sendHistograms, sendMetadata bool,
	recordBuf *record.BuffersPool,
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
	if recordBuf == nil {
		recordBuf = record.NewBuffersPool()
	}
	return &Watcher{
		logger:         logger,
		recordBuf:      recordBuf,
		dec:            record.NewDecoder(labels.NewSymbolTable(), logger),
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

		MaxSegment: -1,
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

// SetMetrics initializes the shared watcher metrics for this Watcher instance.
// We do this here rather than in the constructor because of the ordering of
// creating QueueManagers, stopping them, and then starting new ones in
// storage/remote/storage.go ApplyConfig.
func (w *Watcher) SetMetrics() {
	if w.metrics != nil {
		w.recordsReadMetric = w.metrics.recordsRead.MustCurryWith(prometheus.Labels{consumer: w.name})
		w.recordDecodeFailsMetric = w.metrics.recordDecodeFails.WithLabelValues(w.name)
		w.samplesSentPreTailing = w.metrics.samplesSentPreTailing.WithLabelValues(w.name)
		w.currentSegmentMetric = w.metrics.currentSegment.WithLabelValues(w.name)
		w.notificationsSkipped = w.metrics.notificationsSkipped.WithLabelValues(w.name)
	}
}

// Start the Watcher.
func (w *Watcher) Start() {
	w.SetMetrics()
	w.logger.Info("Starting WAL watcher", "queue", w.name)

	go w.loop()
}

// Stop the Watcher.
func (w *Watcher) Stop() {
	close(w.quit)
	<-w.done

	if w.metrics != nil {
		for _, t := range []record.Type{
			record.Series,
			record.Samples,
			record.SamplesV2,
			record.Tombstones,
			record.Exemplars,
			record.MmapMarkers,
			record.Metadata,
			record.HistogramSamples,
			record.FloatHistogramSamples,
			record.CustomBucketsHistogramSamples,
			record.CustomBucketsFloatHistogramSamples,
			record.HistogramSamplesV2,
			record.FloatHistogramSamplesV2,
			record.MinValidTime,
		} {
			w.metrics.recordsRead.DeleteLabelValues(w.name, t.String())
		}
		w.metrics.recordDecodeFails.DeleteLabelValues(w.name)
		w.metrics.samplesSentPreTailing.DeleteLabelValues(w.name)
		w.metrics.currentSegment.DeleteLabelValues(w.name)
		w.metrics.notificationsSkipped.DeleteLabelValues(w.name)
	}

	w.logger.Info("WAL watcher stopped", "queue", w.name)
}

func (w *Watcher) loop() {
	defer close(w.done)

	if w.startTime.IsZero() {
		w.SetStartTime(time.Now())
	}

	// We may encounter failures processing the WAL; we should wait and retry.
	for !isClosed(w.quit) {
		if err := w.Run(); err != nil {
			w.logger.Error("error tailing WAL", "err", err)
		}

		select {
		case <-w.quit:
			return
		case <-time.After(5 * time.Second):
			w.SetStartTime(time.Now())
		}
	}
}

// Run the watcher, which will tail the WAL until the quit channel is closed
// or an error case is hit.
func (w *Watcher) Run() error {
	if w.recordsReadMetric == nil {
		w.SetMetrics()
	}

	_, lastSegment, err := Segments(w.walDir)
	if err != nil {
		return fmt.Errorf("Segments: %w", err)
	}

	// We want to ensure this is false across iterations since
	// Run will be called again if there was a failure to read the WAL.
	w.sendSamples = false

	w.logger.Info("Replaying WAL", "queue", w.name)

	// Backfill from the checkpoint first if it exists.
	lastCheckpoint, checkpointIndex, err := LastCheckpoint(w.walDir)
	if err != nil && !errors.Is(err, record.ErrNotFound) {
		return fmt.Errorf("tsdb.LastCheckpoint: %w", err)
	}

	if err == nil {
		if err = w.readCheckpoint(lastCheckpoint, (*Watcher).readSegment); err != nil {
			return fmt.Errorf("readCheckpoint: %w", err)
		}
	}
	w.lastCheckpoint = lastCheckpoint

	currentSegment, err := w.findSegmentForIndex(checkpointIndex)
	if err != nil {
		return err
	}

	w.logger.Debug("Tailing WAL", "lastCheckpoint", lastCheckpoint, "checkpointIndex", checkpointIndex, "currentSegment", currentSegment, "lastSegment", lastSegment)
	for !isClosed(w.quit) {
		w.currentSegmentMetric.Set(float64(currentSegment))

		// On start, after reading the existing WAL for series records, we have a pointer to what is the latest segment.
		// On subsequent calls to this function, currentSegment will have been incremented and we should open that segment.
		w.logger.Debug("Processing segment", "currentSegment", currentSegment)
		if err := w.watch(currentSegment, currentSegment < lastSegment); err != nil {
			return err
		}

		// For testing: stop when you hit a specific segment.
		if currentSegment == w.MaxSegment {
			return nil
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

// Use onlySeries false to indicate that the reader is currently on a segment
// that is actively being written to. If true, assume it's a full segment and
// we're replaying it on start to cache the series records.
func (w *Watcher) watch(segmentNum int, onlySeries bool) error {
	segment, err := OpenReadSegment(SegmentName(w.walDir, segmentNum))
	if err != nil {
		return err
	}
	defer segment.Close()

	reader := NewLiveReader(w.logger, w.readerMetrics, segment)
	// One symbol table per WAL segment means it won't grow indefinitely.
	w.dec = record.NewDecoder(labels.NewSymbolTable(), w.logger)

	if onlySeries {
		if err := handleFullSegmentPartialReads(w.readSegment(reader, segmentNum, true), reader, getSegmentSizeFn(w.walDir, segmentNum)); err != nil {
			// Ignore all errors reading to end of segment whilst replaying the WAL.
			w.logger.Warn("Ignoring error reading to end of segment, may have dropped data", "segment", segmentNum, "err", err)
		}
		return nil
	}

	// Always try to read from the new segment before we wait for the emergency read timeout,
	// new segments, or notifications. EOFs are not fatal as there might be another
	// routine writing to the segment.
	if err := w.readSegment(reader, segmentNum, onlySeries); err != nil && !errors.Is(err, io.EOF) {
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

		// If a newer segment is produced, read the current one until the end and move on.
		case <-segmentTicker.C:
			_, last, err := Segments(w.walDir)
			if err != nil {
				return fmt.Errorf("Segments: %w", err)
			}

			if last > segmentNum {
				// At this point we expect a full segment, so handle LiveReader EOFs that
				// indicate a partial read.
				if err := handleFullSegmentPartialReads(w.readSegment(reader, segmentNum, onlySeries), reader, getSegmentSizeFn(w.walDir, segmentNum)); err != nil {
					return fmt.Errorf("read on a new segment: %w", err)
				}
				return nil
			}
			continue

		// We haven't read due to a notification in quite some time, try reading anyway.
		case <-readTicker.C:
			w.logger.Debug("Watcher is reading the WAL due to timeout, haven't received any write notifications recently", "timeout", w.readTimeout)
			if err := w.readSegment(reader, segmentNum, onlySeries); err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			// Reset the ticker so we don't read too often.
			readTicker.Reset(w.readTimeout)

		case <-w.readNotify:
			if err := w.readSegment(reader, segmentNum, onlySeries); err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			// Reset the ticker so we don't read too often.
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
	w.lastCheckpoint = dir

	// Clear series with a checkpoint or segment index # lower than the checkpoint we just read.
	w.writer.SeriesReset(index)
	return nil
}

func (w *Watcher) markReplayDone() {
	if !w.sendSamples {
		w.sendSamples = true
		w.logger.Info("Done replaying WAL", "duration", time.Since(w.startTime))
	}
}

// Read from a segment and pass the details to w.writer.
// Also used with readCheckpoint - implements segmentReadFn.
func (w *Watcher) readSegment(r *LiveReader, segmentNum int, onlySeries bool) error {
	var (
		series          = w.recordBuf.GetRefSeries(512)
		samples         []record.RefSample
		exemplars       []record.RefExemplar
		histograms      []record.RefHistogramSample
		floatHistograms []record.RefFloatHistogramSample
		metadata        []record.RefMetadata
	)
	if w.sendMetadata {
		metadata = w.recordBuf.GetMetadata(512)
	}
	if !onlySeries {
		samples = w.recordBuf.GetSamples(512)
		if w.sendExemplars {
			exemplars = w.recordBuf.GetExemplars(512)
		}
		if w.sendHistograms {
			histograms = w.recordBuf.GetHistograms(512)
			floatHistograms = w.recordBuf.GetFloatHistograms(512)
		}
	}
	defer func() {
		w.recordBuf.PutRefSeries(series)
		if w.sendMetadata {
			w.recordBuf.PutMetadata(metadata)
		}
		if !onlySeries {
			w.recordBuf.PutSamples(samples)
			if w.sendExemplars {
				w.recordBuf.PutExemplars(exemplars)
			}
			if w.sendHistograms {
				w.recordBuf.PutHistograms(histograms)
				w.recordBuf.PutFloatHistograms(floatHistograms)
			}
		}
	}()

	for r.Next() && !isClosed(w.quit) {
		var err error
		rec := r.Record()
		recType := w.dec.Type(rec)
		w.recordsReadMetric.WithLabelValues(recType.String()).Inc()

		switch recType {
		case record.Series:
			series, err = w.dec.Series(rec, series[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			w.writer.StoreSeries(series, segmentNum)

		case record.Samples, record.SamplesV2:
			// When only caching series we can ignore any samples records we see.
			// This speeds up replay of the WAL by > 10x.
			if onlySeries {
				break
			}
			samples, err = w.dec.Samples(rec, samples[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			// Reuse the underlying array for efficiency.
			// It's valid to do, because we override elements that we no longer need to read when filtering.
			samplesToSend := samples[:0]
			for _, s := range samples {
				if s.T > w.startTimestamp {
					samplesToSend = append(samplesToSend, s)
				}
			}
			if len(samplesToSend) > 0 {
				w.markReplayDone()
				w.writer.Append(samplesToSend)
			}

		case record.Exemplars:
			// Skip if experimental "exemplars over remote write" is not enabled.
			if !w.sendExemplars || onlySeries {
				break
			}
			exemplars, err = w.dec.Exemplars(rec, exemplars[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			w.writer.AppendExemplars(exemplars)

		case record.HistogramSamples, record.CustomBucketsHistogramSamples, record.HistogramSamplesV2:
			// Skip if "native histograms over remote write" is not enabled.
			if !w.sendHistograms || onlySeries {
				break
			}
			histograms, err = w.dec.HistogramSamples(rec, histograms[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			// Reuse the underlying array for efficiency.
			// It's valid to do, because we override elements that we no longer need to read when filtering.
			histogramsToSend := histograms[:0]
			for _, h := range histograms {
				if h.T > w.startTimestamp {
					histogramsToSend = append(histogramsToSend, h)
				}
			}
			if len(histogramsToSend) > 0 {
				w.markReplayDone()
				w.writer.AppendHistograms(histogramsToSend)
			}

		case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples, record.FloatHistogramSamplesV2:
			// Skip if "native histograms over remote write" is not enabled.
			if !w.sendHistograms || onlySeries {
				break
			}
			floatHistograms, err = w.dec.FloatHistogramSamples(rec, floatHistograms[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			// Reuse the underlying array for efficiency.
			// It's valid to do, because we override elements that we no longer need to read when filtering.
			floatHistogramsToSend := floatHistograms[:0]
			for _, fh := range floatHistograms {
				if fh.T > w.startTimestamp {
					floatHistogramsToSend = append(floatHistogramsToSend, fh)
				}
			}
			if len(floatHistogramsToSend) > 0 {
				w.markReplayDone()
				w.writer.AppendFloatHistograms(floatHistogramsToSend)
			}

		case record.Metadata:
			if !w.sendMetadata {
				break
			}
			metadata, err = w.dec.Metadata(rec, metadata[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			w.writer.StoreMetadata(metadata)

		case record.Unknown:
			// Could be corruption, or reading from a WAL from a newer Prometheus.
			w.recordDecodeFailsMetric.Inc()

		default:
			// We're not interested in other types of records.
		}
	}
	// NOTE: r.Err == io.EOF is a common case when tailing.
	// Don't wrap error, callers are expected to handle EOF and wrap accordingly.
	return r.Err()
}

// Go through all series in a segment updating the segmentNum, so we can delete older series.
// Used with readCheckpoint - implements segmentReadFn.
func (w *Watcher) readSegmentForGC(r *LiveReader, segmentNum int, _ bool) error {
	series := w.recordBuf.GetRefSeries(512)
	defer func() {
		w.recordBuf.PutRefSeries(series)
	}()

	dec := record.NewDecoder(labels.NewSymbolTable(), w.logger) // Needed for decoding; labels do not outlive this function.
	for r.Next() && !isClosed(w.quit) {
		var err error
		rec := r.Record()
		w.recordsReadMetric.WithLabelValues(dec.Type(rec).String()).Inc()

		switch dec.Type(rec) {
		case record.Series:
			series, err = dec.Series(rec, series[:0])
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
	// NOTE: r.Err == io.EOF is a common case when tailing.
	// Don't wrap error, callers are expected to handle EOF and wrap accordingly.
	return r.Err()
}

func (w *Watcher) SetStartTime(t time.Time) {
	w.startTime = t
	w.startTimestamp = timestamp.FromTime(t)
}

type segmentReadFn func(w *Watcher, r *LiveReader, segmentNum int, onlySeries bool) error

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
		err = handleFullSegmentPartialReads(readFn(w, r, index, true), r, getSegmentSizeFn(checkpointDir, segRef.index))
		sr.Close()
		if err != nil {
			return fmt.Errorf("readCheckpoint %s/%08d: %w", checkpointDir, segRef.index, err)
		}
	}

	w.logger.Debug("Read series references from checkpoint", "checkpoint", checkpointDir)
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

// handleFullSegmentPartialReads handles LiveReader derived errors in case of a known
// full segment read. This is needed because LiveReader always returns io.EOF, even
// for full, successful segment reads.
func handleFullSegmentPartialReads(err error, r *LiveReader, getSize func() (int64, error)) error {
	if err == nil {
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
