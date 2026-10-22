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
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

const (
	checkpointPeriod   = 5 * time.Second
	segmentCheckPeriod = 100 * time.Millisecond
	consumer           = "consumer"
)

var (
	ErrIgnorable = errors.New("ignore me")
	readTimeout  = 15 * time.Second
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

// NativeMetadataWriteTo is a WriteTo that forwards native metadata histories.
// A watcher passes it every native metadata record, and every latest-value
// metadata entry, in WAL order, instead of legacy metadata. Merge groups are
// not idempotent, so each replay starts from reset state and reads segments
// only after the last checkpoint's own.
type NativeMetadataWriteTo interface {
	WriteTo
	// ResetNativeMetadata discards all native metadata before a replay, which
	// starts with a checkpoint if checkpoint is set.
	ResetNativeMetadata(checkpoint bool)
	// StoreNativeMetadataRecord applies rec's entries read from segment in WAL
	// order; their points index rec.Values. A checkpoint's entries are read
	// from its index. The values' strings alias the watcher's reused record
	// buffer, and the watcher reuses rec after the call, so rec and all it
	// refers to are valid only during the call. It must not retain or modify
	// any of them, and must copy any string contents it retains into memory it
	// owns.
	StoreNativeMetadataRecord(rec *record.CompactNativeMetadata, segment int)
	// StoreLegacyNativeMetadata applies latest-value metadata entries read
	// from segment in WAL order, each replacing its series' history with one
	// version whose start is unknown. It must not retain meta.
	StoreLegacyNativeMetadata(meta []record.RefMetadata, segment int)
	// UpdateNativeMetadataSegment records that the checkpoint at segment keeps
	// the metadata of refs, as UpdateSeriesSegment does for series records. It
	// must not retain refs.
	UpdateNativeMetadataSegment(refs []chunks.HeadSeriesRef, segment int)
}

// NativeMetadataDecodeError reports a native metadata record that a watcher
// forwarding native metadata couldn't decode: unsupported or malformed, as Err
// tells. Delivering what follows it would label samples with a history that
// misses the record, so the watcher stops at it, whichever segment holds it,
// and retries without getting past it.
type NativeMetadataDecodeError struct {
	Segment int
	Err     error
}

func (e *NativeMetadataDecodeError) Error() string {
	return fmt.Sprintf("decode native metadata record in segment %d: %v", e.Segment, e.Err)
}

func (e *NativeMetadataDecodeError) Unwrap() error { return e.Err }

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
	nativeMetadataHeld    *prometheus.GaugeVec
}

// Watcher watches the TSDB WAL for a given WriteTo.
type Watcher struct {
	name           string
	writer         WriteTo
	native         NativeMetadataWriteTo // Set when writer forwards native metadata.
	recordBuf      *record.BuffersPool
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
	nativeMetadataHeld      prometheus.Gauge
	// heldSegment is the segment, or checkpoint, holding the native metadata
	// record the watcher last failed to decode, or -1.
	heldSegment int

	readNotify chan struct{}
	quit       chan struct{}
	done       chan struct{}

	// For testing, stop when we hit this segment.
	MaxSegment int
	// For testing, called after each replay of the last checkpoint.
	testAfterCheckpoint func()
}

func NewWatcherMetrics(reg prometheus.Registerer) *WatcherMetrics {
	m := &WatcherMetrics{
		reg: reg,
		recordsRead: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "prometheus",
				Subsystem: "wal_watcher",
				Name:      "records_read_total",
				Help:      "Number of records read by the WAL watcher from the WAL.",
			},
			[]string{consumer, "type"},
		),
		recordDecodeFails: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "prometheus",
				Subsystem: "wal_watcher",
				Name:      "record_decode_failures_total",
				Help:      "Number of records read by the WAL watcher that resulted in an error when decoding.",
			},
			[]string{consumer},
		),
		samplesSentPreTailing: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "prometheus",
				Subsystem: "wal_watcher",
				Name:      "samples_sent_pre_tailing_total",
				Help:      "Number of sample records read by the WAL watcher and sent to remote write during replay of existing WAL.",
			},
			[]string{consumer},
		),
		currentSegment: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "prometheus",
				Subsystem: "wal_watcher",
				Name:      "current_segment",
				Help:      "Current segment the WAL watcher is reading records from.",
			},
			[]string{consumer},
		),
		notificationsSkipped: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "prometheus",
				Subsystem: "wal_watcher",
				Name:      "notifications_skipped_total",
				Help:      "The number of WAL write notifications that the Watcher has skipped due to already being in a WAL read routine.",
			},
			[]string{consumer},
		),
		nativeMetadataHeld: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "prometheus",
				Subsystem: "wal_watcher",
				Name:      "native_metadata_held",
				Help:      "1 while the WAL watcher holds at a native metadata record it couldn't decode, delivering nothing from it on.",
			},
			[]string{consumer},
		),
	}

	if reg != nil {
		reg.MustRegister(m.recordsRead)
		reg.MustRegister(m.recordDecodeFails)
		reg.MustRegister(m.samplesSentPreTailing)
		reg.MustRegister(m.currentSegment)
		reg.MustRegister(m.notificationsSkipped)
		reg.MustRegister(m.nativeMetadataHeld)
	}

	return m
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
	m.reg.Unregister(m.nativeMetadataHeld)
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
	if recordBuf == nil {
		recordBuf = record.NewBuffersPool()
	}
	var native NativeMetadataWriteTo
	if sendMetadata {
		native, _ = writer.(NativeMetadataWriteTo)
	}
	return &Watcher{
		logger:         logger,
		recordBuf:      recordBuf,
		writer:         writer,
		native:         native,
		metrics:        metrics,
		readerMetrics:  readerMetrics,
		walDir:         filepath.Join(dir, "wal"),
		name:           name,
		sendExemplars:  sendExemplars,
		sendHistograms: sendHistograms,
		sendMetadata:   sendMetadata,

		readNotify: make(chan struct{}),
		quit:       make(chan struct{}),
		done:       make(chan struct{}),

		MaxSegment:  -1,
		heldSegment: -1,
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

func (w *Watcher) SetMetrics() {
	// Setup the WAL Watchers metrics. We do this here rather than in the
	// constructor because of the ordering of creating Queue Managers's,
	// stopping them, and then starting new ones in storage/remote/storage.go ApplyConfig.
	if w.metrics != nil {
		w.recordsReadMetric = w.metrics.recordsRead.MustCurryWith(prometheus.Labels{consumer: w.name})
		w.recordDecodeFailsMetric = w.metrics.recordDecodeFails.WithLabelValues(w.name)
		w.samplesSentPreTailing = w.metrics.samplesSentPreTailing.WithLabelValues(w.name)
		w.currentSegmentMetric = w.metrics.currentSegment.WithLabelValues(w.name)
		w.notificationsSkipped = w.metrics.notificationsSkipped.WithLabelValues(w.name)
		w.nativeMetadataHeld = w.metrics.nativeMetadataHeld.WithLabelValues(w.name)
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

	// Records read metric has series and samples.
	if w.metrics != nil {
		w.metrics.recordsRead.DeleteLabelValues(w.name, "series")
		w.metrics.recordsRead.DeleteLabelValues(w.name, "samples")
		w.metrics.recordDecodeFails.DeleteLabelValues(w.name)
		w.metrics.samplesSentPreTailing.DeleteLabelValues(w.name)
		w.metrics.currentSegment.DeleteLabelValues(w.name)
		w.metrics.nativeMetadataHeld.DeleteLabelValues(w.name)
	}

	w.logger.Info("WAL watcher stopped", "queue", w.name)
}

func (w *Watcher) loop() {
	defer close(w.done)

	// We may encounter failures processing the WAL; we should wait and retry.
	for !isClosed(w.quit) {
		w.SetStartTime(time.Now())
		if err := w.Run(); err != nil {
			if decodeErr := (*NativeMetadataDecodeError)(nil); errors.As(err, &decodeErr) {
				w.heldSegment = decodeErr.Segment
				if w.nativeMetadataHeld != nil {
					w.nativeMetadataHeld.Set(1)
				}
			}
			w.logger.Error("error tailing WAL", "err", err)
		}

		select {
		case <-w.quit:
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// Run the watcher, which will tail the WAL until the quit channel is closed
// or an error case is hit.
func (w *Watcher) Run() error {
	_, lastSegment, err := Segments(w.walDir)
	if err != nil {
		return fmt.Errorf("Segments: %w", err)
	}

	// We want to ensure this is false across iterations since
	// Run will be called again if there was a failure to read the WAL.
	w.sendSamples = false

	w.logger.Info("Replaying WAL", "queue", w.name)

	// Backfill from the checkpoint first if it exists.
	currentSegment, err := w.replayCheckpoint()
	if err != nil {
		return err
	}

	w.logger.Debug("Tailing WAL", "lastCheckpoint", w.lastCheckpoint, "currentSegment", currentSegment, "lastSegment", lastSegment)
	for !isClosed(w.quit) {
		w.currentSegmentMetric.Set(float64(currentSegment))

		// On start, after reading the existing WAL for series records, we have a pointer to what is the latest segment.
		// On subsequent calls to this function, currentSegment will have been incremented and we should open that segment.
		w.logger.Debug("Processing segment", "currentSegment", currentSegment)
		if w.heldSegment >= 0 && currentSegment > w.heldSegment {
			// The watcher got past the record it held at.
			w.heldSegment = -1
			if w.nativeMetadataHeld != nil {
				w.nativeMetadataHeld.Set(0)
			}
		}
		if err := w.watch(currentSegment, currentSegment < lastSegment); err != nil && !errors.Is(err, ErrIgnorable) {
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

// replayCheckpoint reads the last checkpoint, if any, and returns the first
// segment to read after it. For native metadata, that is the segment after the
// checkpoint's own. If a newer checkpoint deleted it, replay restarts from the
// newer checkpoint.
func (w *Watcher) replayCheckpoint() (int, error) {
	for {
		lastCheckpoint, checkpointIndex, err := LastCheckpoint(w.walDir)
		if err != nil && !errors.Is(err, record.ErrNotFound) {
			return -1, fmt.Errorf("tsdb.LastCheckpoint: %w", err)
		}
		if w.native != nil {
			w.native.ResetNativeMetadata(err == nil)
		}
		if err == nil {
			if err = w.readCheckpoint(lastCheckpoint, (*Watcher).readSegment); err != nil {
				return -1, fmt.Errorf("readCheckpoint: %w", err)
			}
		}
		w.lastCheckpoint = lastCheckpoint
		if hook := w.testAfterCheckpoint; hook != nil {
			hook()
		}
		if w.native == nil || lastCheckpoint == "" {
			return w.findSegmentForIndex(checkpointIndex)
		}
		segment, err := w.findSegmentForIndex(checkpointIndex + 1)
		if err != nil || segment == checkpointIndex+1 {
			return segment, err
		}
		if newer, _, err := LastCheckpoint(w.walDir); err == nil && newer != lastCheckpoint {
			continue
		}
		w.logger.Warn("WAL segments are missing after the last checkpoint", "checkpoint", lastCheckpoint, "next_segment", segment)
		return segment, nil
	}
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

func (w *Watcher) readAndHandleError(r *LiveReader, segmentNum int, onlySeries bool, size int64) error {
	err := w.readSegment(r, segmentNum, onlySeries)

	// A native metadata record the watcher couldn't decode is never skipped,
	// in replay as in tailing: delivering what follows it would label samples
	// with a history that misses it.
	if decodeErr := (*NativeMetadataDecodeError)(nil); errors.As(err, &decodeErr) {
		return err
	}
	// Ignore all errors reading to end of segment whilst replaying the WAL.
	if onlySeries {
		if err != nil && !errors.Is(err, io.EOF) {
			w.logger.Warn("Ignoring error reading to end of segment, may have dropped data", "segment", segmentNum, "err", err)
		} else if r.Offset() != size {
			w.logger.Warn("Expected to have read whole segment, may have dropped data", "segment", segmentNum, "read", r.Offset(), "size", size)
		}
		return ErrIgnorable
	}

	// Otherwise, when we are tailing, non-EOFs are fatal.
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
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

	size := int64(math.MaxInt64)
	if onlySeries {
		var err error
		size, err = getSegmentSize(w.walDir, segmentNum)
		if err != nil {
			return fmt.Errorf("getSegmentSize: %w", err)
		}

		return w.readAndHandleError(reader, segmentNum, onlySeries, size)
	}

	checkpointTicker := time.NewTicker(checkpointPeriod)
	defer checkpointTicker.Stop()

	segmentTicker := time.NewTicker(segmentCheckPeriod)
	defer segmentTicker.Stop()

	readTicker := time.NewTicker(readTimeout)
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

		// if a newer segment is produced, read the current one until the end and move on.
		case <-segmentTicker.C:
			_, last, err := Segments(w.walDir)
			if err != nil {
				return fmt.Errorf("Segments: %w", err)
			}

			if last > segmentNum {
				return w.readAndHandleError(reader, segmentNum, onlySeries, size)
			}
			continue

		// we haven't read due to a notification in quite some time, try reading anyways
		case <-readTicker.C:
			w.logger.Debug("Watcher is reading the WAL due to timeout, haven't received any write notifications recently", "timeout", readTimeout)
			err := w.readAndHandleError(reader, segmentNum, onlySeries, size)
			if err != nil {
				return err
			}
			// reset the ticker so we don't read too often
			readTicker.Reset(readTimeout)

		case <-w.readNotify:
			err := w.readAndHandleError(reader, segmentNum, onlySeries, size)
			if err != nil {
				return err
			}
			// reset the ticker so we don't read too often
			readTicker.Reset(readTimeout)
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

// Read from a segment and pass the details to w.writer.
// Also used with readCheckpoint - implements segmentReadFn.
func (w *Watcher) readSegment(r *LiveReader, segmentNum int, onlySeries bool) error {
	series := w.recordBuf.GetRefSeries(512)
	samples := w.recordBuf.GetSamples(512)
	exemplars := w.recordBuf.GetExemplars(512)
	histograms := w.recordBuf.GetHistograms(512)
	floatHistograms := w.recordBuf.GetFloatHistograms(512)
	metadata := w.recordBuf.GetMetadata(512)
	defer func() {
		w.recordBuf.PutRefSeries(series)
		w.recordBuf.PutSamples(samples)
		w.recordBuf.PutExemplars(exemplars)
		w.recordBuf.PutHistograms(histograms)
		w.recordBuf.PutFloatHistograms(floatHistograms)
		w.recordBuf.PutMetadata(metadata)
	}()

	var compact record.CompactNativeMetadata
	dec := record.NewDecoder(labels.NewSymbolTable(), w.logger) // One table per WAL segment means it won't grow indefinitely.
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
			w.writer.StoreSeries(series, segmentNum)

		case record.Samples, record.SamplesV2:
			// When only caching series we can ignore any samples records we see.
			// This speeds up replay of the WAL by > 10x.
			if onlySeries {
				break
			}
			samples, err = dec.Samples(rec, samples[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			// Reuse the underlying array for efficiency.
			// It's valid to do, because we override elements that we no longer need to read when filtering.
			samplesToSend := samples[:0]
			for _, s := range samples {
				if s.T > w.startTimestamp {
					if !w.sendSamples {
						w.sendSamples = true
						duration := time.Since(w.startTime)
						w.logger.Info("Done replaying WAL", "duration", duration)
					}
					samplesToSend = append(samplesToSend, s)
				}
			}
			if len(samplesToSend) > 0 {
				w.writer.Append(samplesToSend)
			}

		case record.Exemplars:
			// Skip if experimental "exemplars over remote write" is not enabled.
			if !w.sendExemplars {
				break
			}
			// When only caching series we can ignore any exemplars records we see.
			// This speeds up replay of the WAL significantly.
			if onlySeries {
				break
			}
			exemplars, err = dec.Exemplars(rec, exemplars[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			w.writer.AppendExemplars(exemplars)

		case record.HistogramSamples, record.CustomBucketsHistogramSamples, record.HistogramSamplesV2:
			// Skip if "native histograms over remote write" is not enabled.
			if !w.sendHistograms {
				break
			}
			if onlySeries {
				break
			}
			histograms, err = dec.HistogramSamples(rec, histograms[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			// Reuse the underlying array for efficiency.
			// It's valid to do, because we override elements that we no longer need to read when filtering.
			histogramsToSend := histograms[:0]
			for _, h := range histograms {
				if h.T > w.startTimestamp {
					if !w.sendSamples {
						w.sendSamples = true
						duration := time.Since(w.startTime)
						w.logger.Info("Done replaying WAL", "duration", duration)
					}
					histogramsToSend = append(histogramsToSend, h)
				}
			}
			if len(histogramsToSend) > 0 {
				w.writer.AppendHistograms(histogramsToSend)
			}

		case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples, record.FloatHistogramSamplesV2:
			// Skip if "native histograms over remote write" is not enabled.
			if !w.sendHistograms {
				break
			}
			if onlySeries {
				break
			}
			floatHistograms, err = dec.FloatHistogramSamples(rec, floatHistograms[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			// Reuse the underlying array for efficiency.
			// It's valid to do, because we override elements that we no longer need to read when filtering.
			floatHistogramsToSend := floatHistograms[:0]
			for _, fh := range floatHistograms {
				if fh.T > w.startTimestamp {
					if !w.sendSamples {
						w.sendSamples = true
						duration := time.Since(w.startTime)
						w.logger.Info("Done replaying WAL", "duration", duration)
					}
					floatHistogramsToSend = append(floatHistogramsToSend, fh)
				}
			}
			if len(floatHistogramsToSend) > 0 {
				w.writer.AppendFloatHistograms(floatHistogramsToSend)
			}

		case record.Metadata:
			if !w.sendMetadata {
				break
			}
			metadata, err = dec.Metadata(rec, metadata[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			if w.native != nil {
				w.native.StoreLegacyNativeMetadata(metadata, segmentNum)
				break
			}
			w.writer.StoreMetadata(metadata)

		case record.NativeMetadataCompact:
			// Without native metadata forwarding, compact records are opaque.
			if w.native == nil {
				break
			}
			// The record's strings alias rec, which the reader reuses after
			// this record.
			if err := dec.CompactNativeMetadataBorrowed(rec, &compact); err != nil {
				w.recordDecodeFailsMetric.Inc()
				return &NativeMetadataDecodeError{Segment: segmentNum, Err: err}
			}
			w.native.StoreNativeMetadataRecord(&compact, segmentNum)
			compact.Reset()

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
// For native metadata writers, the refs of metadata entries are updated too,
// whether or not the writer trusts them: that can only keep a history longer.
// A native metadata record that doesn't decode fails the scan before any
// series is reset, so nothing is released early.
// Used with readCheckpoint - implements segmentReadFn.
func (w *Watcher) readSegmentForGC(r *LiveReader, segmentNum int, _ bool) error {
	series := w.recordBuf.GetRefSeries(512)
	defer w.recordBuf.PutRefSeries(series)
	metadata := w.recordBuf.GetMetadata(512)
	defer w.recordBuf.PutMetadata(metadata)
	var refs []chunks.HeadSeriesRef
	var compact record.CompactNativeMetadata

	dec := record.NewDecoder(labels.NewSymbolTable(), w.logger) // Needed for decoding; labels do not outlive this function.
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

		case record.Metadata:
			if w.native == nil {
				break
			}
			var err error
			metadata, err = dec.Metadata(rec, metadata[:0])
			if err != nil {
				w.recordDecodeFailsMetric.Inc()
				return err
			}
			refs = refs[:0]
			for _, m := range metadata {
				refs = append(refs, m.Ref)
			}
			w.native.UpdateNativeMetadataSegment(refs, segmentNum)

		case record.NativeMetadataCompact:
			if w.native == nil {
				break
			}
			// Only the refs are needed; the values alias rec.
			if err := dec.CompactNativeMetadataBorrowed(rec, &compact); err != nil {
				w.recordDecodeFailsMetric.Inc()
				return &NativeMetadataDecodeError{Segment: segmentNum, Err: err}
			}
			refs = refs[:0]
			for _, e := range compact.Entries {
				// Provenance markers and boundaries have no ref.
				if e.Ref != 0 {
					refs = append(refs, e.Ref)
				}
			}
			w.native.UpdateNativeMetadataSegment(refs, segmentNum)
			compact.Reset()

		case record.Unknown:
			// Could be corruption, or reading from a WAL from a newer Prometheus.
			w.recordDecodeFailsMetric.Inc()

		default:
			// We're only interested in series and metadata.
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
		size, err := getSegmentSize(checkpointDir, segRef.index)
		if err != nil {
			return fmt.Errorf("getSegmentSize: %w", err)
		}

		sr, err := OpenReadSegment(SegmentName(checkpointDir, segRef.index))
		if err != nil {
			return fmt.Errorf("unable to open segment: %w", err)
		}

		r := NewLiveReader(w.logger, w.readerMetrics, sr)
		err = readFn(w, r, index, true)
		sr.Close()
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("readSegment %d: %w", index, err)
		}

		if r.Offset() != size {
			return fmt.Errorf("readCheckpoint wasn't able to read all data from the checkpoint %s/%08d, size: %d, totalRead: %d", checkpointDir, segRef.index, size, r.Offset())
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

// Get size of segment.
func getSegmentSize(dir string, index int) (int64, error) {
	i := int64(-1)
	fi, err := os.Stat(SegmentName(dir, index))
	if err == nil {
		i = fi.Size()
	}
	return i, err
}

func isClosed(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}
