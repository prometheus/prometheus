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

package remote

import (
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/tsdb"
	"github.com/prometheus/tsdb/fileutil"
	"github.com/prometheus/tsdb/wal"
)

const (
	readPeriod         = 10 * time.Millisecond
	checkpointPeriod   = 5 * time.Second
	segmentCheckPeriod = 100 * time.Millisecond
)

var (
	watcherRecordsRead = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "wal_watcher",
			Name:      "records_read_total",
			Help:      "Number of records read by the WAL watcher from the WAL.",
		},
		[]string{queue, "type"},
	)
	watcherRecordDecodeFails = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "wal_watcher",
			Name:      "record_decode_failures_total",
			Help:      "Number of records read by the WAL watcher that resulted in an error when decoding.",
		},
		[]string{queue},
	)
	watcherSamplesSentPreTailing = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "wal_watcher",
			Name:      "samples_sent_pre_tailing_total",
			Help:      "Number of sample records read by the WAL watcher and sent to remote write during replay of existing WAL.",
		},
		[]string{queue},
	)
	watcherCurrentSegment = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "prometheus",
			Subsystem: "wal_watcher",
			Name:      "current_segment",
			Help:      "Current segment the WAL watcher is reading records from.",
		},
		[]string{queue},
	)
)

func init() {
	prometheus.MustRegister(watcherRecordsRead)
	prometheus.MustRegister(watcherRecordDecodeFails)
	prometheus.MustRegister(watcherSamplesSentPreTailing)
	prometheus.MustRegister(watcherCurrentSegment)
}

type writeTo interface {
	Append([]tsdb.RefSample) bool
	StoreSeries([]tsdb.RefSeries, int)
	SeriesReset(int)
}

// WALWatcher watches the TSDB WAL for a given WriteTo.
type WALWatcher struct {
	name   string
	writer writeTo
	logger log.Logger
	walDir string

	startTime int64

	recordsReadMetric       *prometheus.CounterVec
	recordDecodeFailsMetric prometheus.Counter
	samplesSentPreTailing   prometheus.Counter
	currentSegmentMetric    prometheus.Gauge

	quit chan struct{}
	done chan struct{}
}

// NewWALWatcher creates a new WAL watcher for a given WriteTo.
func NewWALWatcher(logger log.Logger, name string, writer writeTo, walDir string, startTime int64) *WALWatcher {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &WALWatcher{
		logger:    logger,
		writer:    writer,
		walDir:    path.Join(walDir, "wal"),
		startTime: startTime,
		name:      name,
		quit:      make(chan struct{}),
		done:      make(chan struct{}),

		recordsReadMetric:       watcherRecordsRead.MustCurryWith(prometheus.Labels{queue: name}),
		recordDecodeFailsMetric: watcherRecordDecodeFails.WithLabelValues(name),
		samplesSentPreTailing:   watcherSamplesSentPreTailing.WithLabelValues(name),
		currentSegmentMetric:    watcherCurrentSegment.WithLabelValues(name),
	}
}

func (w *WALWatcher) Start() {
	level.Info(w.logger).Log("msg", "starting WAL watcher", "queue", w.name)
	go w.loop()
}

func (w *WALWatcher) Stop() {
	level.Info(w.logger).Log("msg", "stopping WAL watcher", "queue", w.name)
	close(w.quit)
	<-w.done
	level.Info(w.logger).Log("msg", "WAL watcher stopped", "queue", w.name)
}

func (w *WALWatcher) loop() {
	defer close(w.done)

	// We may encourter failures processing the WAL; we should wait and retry.
	for !isClosed(w.quit) {
		if err := w.run(); err != nil {
			level.Error(w.logger).Log("msg", "error tailing WAL", "err", err)
		}

		select {
		case <-w.quit:
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (w *WALWatcher) run() error {
	nw, err := wal.New(nil, nil, w.walDir)
	if err != nil {
		return errors.Wrap(err, "wal.New")
	}

	_, last, err := nw.Segments()
	if err != nil {
		return err
	}

	// Backfill from the checkpoint first if it exists.
	lastCheckpoint, nextIndex, err := tsdb.LastCheckpoint(w.walDir)
	if err != nil && err != tsdb.ErrNotFound {
		return err
	}

	level.Info(w.logger).Log("msg", "reading checkpoint", "dir", lastCheckpoint, "startFrom", nextIndex)
	if err == nil {
		if err = w.readCheckpoint(lastCheckpoint); err != nil {
			return err
		}
	}

	currentSegment, err := w.findSegmentForIndex(nextIndex)
	if err != nil {
		return err
	}

	level.Info(w.logger).Log("msg", "starting from", "currentSegment", currentSegment, "last", last)
	for {
		w.currentSegmentMetric.Set(float64(currentSegment))
		level.Info(w.logger).Log("msg", "process segment", "segment", currentSegment)

		// On start, after reading the existing WAL for series records, we have a pointer to what is the latest segment.
		// On subsequent calls to this function, currentSegment will have been incremented and we should open that segment.
		if err := w.watch(nw, currentSegment, currentSegment >= last); err != nil {
			level.Error(w.logger).Log("msg", "runWatcher is ending", "err", err)
			return err
		}

		currentSegment++
	}
}

// findSegmentForIndex finds the first segment greater than or equal to index.
func (w *WALWatcher) findSegmentForIndex(index int) (int, error) {
	files, err := fileutil.ReadDir(w.walDir)
	if err != nil {
		return -1, err
	}

	var refs []int
	var last int
	for _, fn := range files {
		k, err := strconv.Atoi(fn)
		if err != nil {
			continue
		}
		if len(refs) > 0 && k > last+1 {
			return -1, errors.New("segments are not sequential")
		}
		refs = append(refs, k)
		last = k
	}
	sort.Ints(refs)

	for _, r := range refs {
		if r >= index {
			return r, nil
		}
	}

	return -1, errors.New("failed to find segment for index")
}

// Use tail true to indicate thatreader is currently on a segment that is
// actively being written to. If false, assume it's a full segment and we're
// replaying it on start to cache the series records.
func (w *WALWatcher) watch(wl *wal.WAL, segmentNum int, tail bool) error {
	segment, err := wal.OpenReadSegment(wal.SegmentName(w.walDir, segmentNum))
	if err != nil {
		return err
	}
	defer segment.Close()

	reader := wal.NewLiveReader(segment)

	readTicker := time.NewTicker(readPeriod)
	defer readTicker.Stop()

	checkpointTicker := time.NewTicker(checkpointPeriod)
	defer checkpointTicker.Stop()

	segmentTicker := time.NewTicker(segmentCheckPeriod)
	defer segmentTicker.Stop()

	// If we're replaying the segment we need to know the size of the file to know
	// when to return from watch and move on to the next segment.
	size := int64(math.MaxInt64)
	if !tail {
		segmentTicker.Stop()
		checkpointTicker.Stop()
		var err error
		size, err = getSegmentSize(w.walDir, segmentNum)
		if err != nil {
			level.Error(w.logger).Log("msg", "error getting segment size", "segment", segmentNum)
			return errors.Wrap(err, "get segment size")
		}
	}

	for {
		select {
		case <-w.quit:
			level.Info(w.logger).Log("msg", "quitting WAL watcher watch loop")
			return errors.New("quit channel")

		case <-checkpointTicker.C:
			// Periodically check if there is a new checkpoint so we can garbage
			// collect labels. As this is considered an optimisation, we ignore
			// errors during checkpoint processing.
			if err := w.garbageCollectSeries(segmentNum); err != nil {
				level.Warn(w.logger).Log("msg", "error process checkpoint", "err", err)
			}

		case <-segmentTicker.C:
			_, last, err := wl.Segments()
			if err != nil {
				return errors.Wrap(err, "segments")
			}

			// Check if new segments exists.
			if last <= segmentNum {
				continue
			}

			if err := w.readSegment(reader, segmentNum); err != nil {
				// Ignore errors reading to end of segment, as we're going to move to
				// next segment now.
				level.Error(w.logger).Log("msg", "error reading to end of segment", "err", err)
			}

			level.Info(w.logger).Log("msg", "a new segment exists, we should start reading it", "current", fmt.Sprintf("%08d", segmentNum), "new", fmt.Sprintf("%08d", last))
			return nil

		case <-readTicker.C:
			err := w.readSegment(reader, segmentNum)

			// If we're reading to completion, stop when we hit an EOF.
			if err == io.EOF && !tail {
				level.Info(w.logger).Log("msg", "done replaying segment", "segment", segmentNum, "size", size, "read", reader.TotalRead())
				return nil
			}

			if err != nil && err != io.EOF {
				return err
			}
		}
	}
}

func (w *WALWatcher) garbageCollectSeries(segmentNum int) error {
	dir, _, err := tsdb.LastCheckpoint(w.walDir)
	if err != nil && err != tsdb.ErrNotFound {
		return errors.Wrap(err, "tsdb.LastCheckpoint")
	}

	if dir == "" {
		return nil
	}

	index, err := checkpointNum(dir)
	if err != nil {
		return errors.Wrap(err, "error parsing checkpoint filename")
	}

	if index >= segmentNum {
		level.Debug(w.logger).Log("msg", "current segment is behind the checkpoint, skipping reading of checkpoint", "current", fmt.Sprintf("%08d", segmentNum), "checkpoint", dir)
		return nil
	}

	level.Debug(w.logger).Log("msg", "new checkpoint detected", "new", dir, "currentSegment", segmentNum)

	// This potentially takes a long time, should we run it in another go routine?
	if err = w.readCheckpoint(dir); err != nil {
		return errors.Wrap(err, "readCheckpoint")
	}

	// Clear series with a checkpoint or segment index # lower than the checkpoint we just read.
	w.writer.SeriesReset(index)
	return nil
}

func (w *WALWatcher) readSegment(r *wal.LiveReader, segmentNum int) error {
	for r.Next() && !isClosed(w.quit) {
		err := w.decodeRecord(r.Record(), segmentNum)

		// Intentionally skip over record decode errors.
		if err != nil {
			level.Error(w.logger).Log("err", err)
		}
	}
	return r.Err()
}

func recordType(rt tsdb.RecordType) string {
	switch rt {
	case tsdb.RecordInvalid:
		return "invalid"
	case tsdb.RecordSeries:
		return "series"
	case tsdb.RecordSamples:
		return "samples"
	case tsdb.RecordTombstones:
		return "tombstones"
	default:
		return "unkown"
	}
}

func (w *WALWatcher) decodeRecord(rec []byte, segmentNum int) error {
	var (
		dec     tsdb.RecordDecoder
		series  []tsdb.RefSeries
		samples []tsdb.RefSample
	)

	w.recordsReadMetric.WithLabelValues(recordType(dec.Type(rec))).Inc()

	switch dec.Type(rec) {
	case tsdb.RecordSeries:
		series, err := dec.Series(rec, series[:0])
		if err != nil {
			w.recordDecodeFailsMetric.Inc()
			return err
		}
		w.writer.StoreSeries(series, segmentNum)
		return nil

	case tsdb.RecordSamples:
		samples, err := dec.Samples(rec, samples[:0])
		if err != nil {
			w.recordDecodeFailsMetric.Inc()
			return err
		}
		var send []tsdb.RefSample
		for _, s := range samples {
			if s.T > w.startTime {
				send = append(send, s)
			}
		}
		if len(send) > 0 {
			// Blocks  until the sample is sent to all remote write endpoints or closed (because enqueue blocks).
			w.writer.Append(send)
		}
		return nil

	case tsdb.RecordTombstones:
		return nil

	case tsdb.RecordInvalid:
		return errors.New("invalid record")

	default:
		w.recordDecodeFailsMetric.Inc()
		return errors.New("unknown TSDB record type")
	}
}

// Read all the series records from a Checkpoint directory.
func (w *WALWatcher) readCheckpoint(checkpointDir string) error {
	level.Info(w.logger).Log("msg", "reading checkpoint", "dir", checkpointDir)
	index, err := checkpointNum(checkpointDir)
	if err != nil {
		return err
	}

	sr, err := wal.NewSegmentsReader(checkpointDir)
	if err != nil {
		return errors.Wrap(err, "open checkpoint")
	}
	defer sr.Close()

	size, err := getCheckpointSize(checkpointDir)
	if err != nil {
		level.Error(w.logger).Log("msg", "error getting checkpoint size", "checkpoint", checkpointDir)
		return errors.Wrap(err, "get checkpoint size")
	}

	// w.readSeriesRecords(wal.NewLiveReader(sr), i, size)
	r := wal.NewLiveReader(sr)
	if err := w.readSegment(r, index); err != nil {
		return errors.Wrap(err, "readSegment")
	}

	if r.TotalRead() != size {
		level.Warn(w.logger).Log("msg", "may not have read all data from checkpoint", "totalRead", r.TotalRead(), "size", size)
	}
	level.Debug(w.logger).Log("msg", "read series references from checkpoint", "checkpoint", checkpointDir)

	return nil
}

func checkpointNum(dir string) (int, error) {
	// Checkpoint dir names are in the format checkpoint.000001
	chunks := strings.Split(dir, ".")
	if len(chunks) != 2 {
		return 0, errors.Errorf("invalid checkpoint dir string: %s", dir)
	}

	result, err := strconv.Atoi(chunks[1])
	if err != nil {
		return 0, errors.Errorf("invalid checkpoint dir string: %s", dir)
	}

	return result, nil
}

func getCheckpointSize(dir string) (int64, error) {
	i := int64(0)
	segs, err := fileutil.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	for _, fn := range segs {
		num, err := strconv.Atoi(fn)
		if err != nil {
			return i, err
		}
		sz, err := getSegmentSize(dir, num)
		if err != nil {
			return i, err
		}
		i += sz
	}
	return i, nil
}

// Get size of segment.
func getSegmentSize(dir string, index int) (int64, error) {
	i := int64(-1)
	fi, err := os.Stat(wal.SegmentName(dir, index))
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
