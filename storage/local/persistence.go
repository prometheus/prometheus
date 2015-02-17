// Copyright 2014 The Prometheus Authors
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

package local

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/local/codable"
	"github.com/prometheus/prometheus/storage/local/flock"
	"github.com/prometheus/prometheus/storage/local/index"
	"github.com/prometheus/prometheus/storage/metric"
)

const (
	seriesFileSuffix     = ".db"
	seriesTempFileSuffix = ".db.tmp"
	seriesDirNameLen     = 2 // How many bytes of the fingerprint in dir name.

	headsFileName      = "heads.db"
	headsTempFileName  = "heads.db.tmp"
	headsFormatVersion = 1
	headsMagicString   = "PrometheusHeads"

	dirtyFileName = "DIRTY"

	fileBufSize = 1 << 16 // 64kiB.

	chunkHeaderLen             = 17
	chunkHeaderTypeOffset      = 0
	chunkHeaderFirstTimeOffset = 1
	chunkHeaderLastTimeOffset  = 9

	indexingMaxBatchSize  = 1024 * 1024
	indexingBatchTimeout  = 500 * time.Millisecond // Commit batch when idle for that long.
	indexingQueueCapacity = 1024 * 16
)

var fpLen = len(clientmodel.Fingerprint(0).String()) // Length of a fingerprint as string.

const (
	flagHeadChunkPersisted byte = 1 << iota
	// Add more flags here like:
	// flagFoo
	// flagBar
)

type indexingOpType byte

const (
	add indexingOpType = iota
	remove
)

type indexingOp struct {
	fingerprint clientmodel.Fingerprint
	metric      clientmodel.Metric
	opType      indexingOpType
}

// A Persistence is used by a Storage implementation to store samples
// persistently across restarts. The methods are only goroutine-safe if
// explicitly marked as such below. The chunk-related methods persistChunks,
// dropChunks, loadChunks, and loadChunkDescs can be called concurrently with
// each other if each call refers to a different fingerprint.
type persistence struct {
	basePath string
	chunkLen int

	archivedFingerprintToMetrics   *index.FingerprintMetricIndex
	archivedFingerprintToTimeRange *index.FingerprintTimeRangeIndex
	labelPairToFingerprints        *index.LabelPairFingerprintIndex
	labelNameToLabelValues         *index.LabelNameLabelValuesIndex

	indexingQueue   chan indexingOp
	indexingStopped chan struct{}
	indexingFlush   chan chan int

	indexingQueueLength   prometheus.Gauge
	indexingQueueCapacity prometheus.Metric
	indexingBatchSizes    prometheus.Summary
	indexingBatchLatency  prometheus.Summary
	checkpointDuration    prometheus.Gauge

	dirtyMtx      sync.Mutex     // Protects dirty and becameDirty.
	dirty         bool           // true if persistence was started in dirty state.
	becameDirty   bool           // true if an inconsistency came up during runtime.
	dirtyFileName string         // The file used for locking and to mark dirty state.
	fLock         flock.Releaser // The file lock to protect against concurrent usage.
}

// newPersistence returns a newly allocated persistence backed by local disk storage, ready to use.
func newPersistence(basePath string, chunkLen int, dirty bool) (*persistence, error) {
	if err := os.MkdirAll(basePath, 0700); err != nil {
		return nil, err
	}
	dirtyPath := filepath.Join(basePath, dirtyFileName)

	fLock, dirtyfileExisted, err := flock.New(dirtyPath)
	if err != nil {
		glog.Errorf("Could not lock %s, Prometheus already running?", dirtyPath)
		return nil, err
	}
	if dirtyfileExisted {
		dirty = true
	}

	archivedFingerprintToMetrics, err := index.NewFingerprintMetricIndex(basePath)
	if err != nil {
		return nil, err
	}
	archivedFingerprintToTimeRange, err := index.NewFingerprintTimeRangeIndex(basePath)
	if err != nil {
		return nil, err
	}

	p := &persistence{
		basePath: basePath,
		chunkLen: chunkLen,

		archivedFingerprintToMetrics:   archivedFingerprintToMetrics,
		archivedFingerprintToTimeRange: archivedFingerprintToTimeRange,

		indexingQueue:   make(chan indexingOp, indexingQueueCapacity),
		indexingStopped: make(chan struct{}),
		indexingFlush:   make(chan chan int),

		indexingQueueLength: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "indexing_queue_length",
			Help:      "The number of metrics waiting to be indexed.",
		}),
		indexingQueueCapacity: prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, subsystem, "indexing_queue_capacity"),
				"The capacity of the indexing queue.",
				nil, nil,
			),
			prometheus.GaugeValue,
			float64(indexingQueueCapacity),
		),
		indexingBatchSizes: prometheus.NewSummary(
			prometheus.SummaryOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "indexing_batch_sizes",
				Help:      "Quantiles for indexing batch sizes (number of metrics per batch).",
			},
		),
		indexingBatchLatency: prometheus.NewSummary(
			prometheus.SummaryOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "indexing_batch_latency_milliseconds",
				Help:      "Quantiles for batch indexing latencies in milliseconds.",
			},
		),
		checkpointDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "checkpoint_duration_milliseconds",
			Help:      "The duration (in milliseconds) it took to checkpoint in-memory metrics and head chunks.",
		}),
		dirty:         dirty,
		dirtyFileName: dirtyPath,
		fLock:         fLock,
	}

	if p.dirty {
		// Blow away the label indexes. We'll rebuild them later.
		if err := index.DeleteLabelPairFingerprintIndex(basePath); err != nil {
			return nil, err
		}
		if err := index.DeleteLabelNameLabelValuesIndex(basePath); err != nil {
			return nil, err
		}
	}
	labelPairToFingerprints, err := index.NewLabelPairFingerprintIndex(basePath)
	if err != nil {
		return nil, err
	}
	labelNameToLabelValues, err := index.NewLabelNameLabelValuesIndex(basePath)
	if err != nil {
		return nil, err
	}
	p.labelPairToFingerprints = labelPairToFingerprints
	p.labelNameToLabelValues = labelNameToLabelValues

	go p.processIndexingQueue()
	return p, nil
}

// Describe implements prometheus.Collector.
func (p *persistence) Describe(ch chan<- *prometheus.Desc) {
	ch <- p.indexingQueueLength.Desc()
	ch <- p.indexingQueueCapacity.Desc()
	p.indexingBatchSizes.Describe(ch)
	p.indexingBatchLatency.Describe(ch)
	ch <- p.checkpointDuration.Desc()
}

// Collect implements prometheus.Collector.
func (p *persistence) Collect(ch chan<- prometheus.Metric) {
	p.indexingQueueLength.Set(float64(len(p.indexingQueue)))

	ch <- p.indexingQueueLength
	ch <- p.indexingQueueCapacity
	p.indexingBatchSizes.Collect(ch)
	p.indexingBatchLatency.Collect(ch)
	ch <- p.checkpointDuration
}

// isDirty returns the dirty flag in a goroutine-safe way.
func (p *persistence) isDirty() bool {
	p.dirtyMtx.Lock()
	defer p.dirtyMtx.Unlock()
	return p.dirty
}

// setDirty sets the dirty flag in a goroutine-safe way. Once the dirty flag was
// set to true with this method, it cannot be set to false again. (If we became
// dirty during our runtime, there is no way back. If we were dirty from the
// start, a clean-up might make us clean again.)
func (p *persistence) setDirty(dirty bool) {
	p.dirtyMtx.Lock()
	defer p.dirtyMtx.Unlock()
	if p.becameDirty {
		return
	}
	p.dirty = dirty
	if dirty {
		p.becameDirty = true
		glog.Error("The storage is now inconsistent. Restart Prometheus ASAP to initiate recovery.")
	}
}

// getFingerprintsForLabelPair returns the fingerprints for the given label
// pair. This method is goroutine-safe but take into account that metrics queued
// for indexing with IndexMetric might not have made it into the index
// yet. (Same applies correspondingly to UnindexMetric.)
func (p *persistence) getFingerprintsForLabelPair(lp metric.LabelPair) (clientmodel.Fingerprints, error) {
	fps, _, err := p.labelPairToFingerprints.Lookup(lp)
	if err != nil {
		return nil, err
	}
	return fps, nil
}

// getLabelValuesForLabelName returns the label values for the given label
// name. This method is goroutine-safe but take into account that metrics queued
// for indexing with IndexMetric might not have made it into the index
// yet. (Same applies correspondingly to UnindexMetric.)
func (p *persistence) getLabelValuesForLabelName(ln clientmodel.LabelName) (clientmodel.LabelValues, error) {
	lvs, _, err := p.labelNameToLabelValues.Lookup(ln)
	if err != nil {
		return nil, err
	}
	return lvs, nil
}

// persistChunks persists a number of consecutive chunks of a series. It is the
// caller's responsibility to not modify the chunks concurrently and to not
// persist or drop anything for the same fingerprint concurrently. It returns
// the (zero-based) index of the first persisted chunk within the series
// file. In case of an error, the returned index is -1 (to avoid the
// misconception that the chunk was written at position 0).
func (p *persistence) persistChunks(fp clientmodel.Fingerprint, chunks []chunk) (int, error) {

	f, err := p.openChunkFileForWriting(fp)
	if err != nil {
		return -1, err
	}
	defer f.Close()

	b := bufio.NewWriterSize(f, len(chunks)*(chunkHeaderLen+p.chunkLen))

	for _, c := range chunks {
		err = writeChunkHeader(b, c)
		if err != nil {
			return -1, err
		}

		err = c.marshal(b)
		if err != nil {
			return -1, err
		}
	}

	// Determine index within the file.
	b.Flush()
	offset, err := f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return -1, err
	}
	index, err := p.chunkIndexForOffset(offset)
	if err != nil {
		return -1, err
	}

	return index - len(chunks), err
}

// loadChunks loads a group of chunks of a timeseries by their index. The chunk
// with the earliest time will have index 0, the following ones will have
// incrementally larger indexes. The indexOffset denotes the offset to be added to
// each index in indexes. It is the caller's responsibility to not persist or
// drop anything for the same fingerprint concurrently.
func (p *persistence) loadChunks(fp clientmodel.Fingerprint, indexes []int, indexOffset int) ([]chunk, error) {
	f, err := p.openChunkFileForReading(fp)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	chunks := make([]chunk, 0, len(indexes))
	typeBuf := make([]byte, 1)
	for _, idx := range indexes {
		_, err := f.Seek(p.offsetForChunkIndex(idx+indexOffset), os.SEEK_SET)
		if err != nil {
			return nil, err
		}

		n, err := f.Read(typeBuf)
		if err != nil {
			return nil, err
		}
		if n != 1 {
			panic("read returned != 1 bytes")
		}

		_, err = f.Seek(chunkHeaderLen-1, os.SEEK_CUR)
		if err != nil {
			return nil, err
		}
		chunk := chunkForType(typeBuf[0])
		chunk.unmarshal(f)
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// loadChunkDescs loads chunkDescs for a series up until a given time.  It is
// the caller's responsibility to not persist or drop anything for the same
// fingerprint concurrently.
func (p *persistence) loadChunkDescs(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) ([]*chunkDesc, error) {
	f, err := p.openChunkFileForReading(fp)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	totalChunkLen := chunkHeaderLen + p.chunkLen
	if fi.Size()%int64(totalChunkLen) != 0 {
		p.setDirty(true)
		return nil, fmt.Errorf(
			"size of series file for fingerprint %v is %d, which is not a multiple of the chunk length %d",
			fp, fi.Size(), totalChunkLen,
		)
	}

	numChunks := int(fi.Size()) / totalChunkLen
	cds := make([]*chunkDesc, 0, numChunks)
	for i := 0; i < numChunks; i++ {
		_, err := f.Seek(p.offsetForChunkIndex(i)+chunkHeaderFirstTimeOffset, os.SEEK_SET)
		if err != nil {
			return nil, err
		}

		chunkTimesBuf := make([]byte, 16)
		_, err = io.ReadAtLeast(f, chunkTimesBuf, 16)
		if err != nil {
			return nil, err
		}
		cd := &chunkDesc{
			chunkFirstTime: clientmodel.Timestamp(binary.LittleEndian.Uint64(chunkTimesBuf)),
			chunkLastTime:  clientmodel.Timestamp(binary.LittleEndian.Uint64(chunkTimesBuf[8:])),
		}
		if !cd.chunkLastTime.Before(beforeTime) {
			// From here on, we have chunkDescs in memory already.
			break
		}
		cds = append(cds, cd)
	}
	chunkDescOps.WithLabelValues(load).Add(float64(len(cds)))
	numMemChunkDescs.Add(float64(len(cds)))
	return cds, nil
}

// checkpointSeriesMapAndHeads persists the fingerprint to memory-series mapping
// and all open (non-full) head chunks. Do not call concurrently with
// loadSeriesMapAndHeads.
//
// Description of the file format:
//
// (1) Magic string (const headsMagicString).
//
// (2) Varint-encoded format version (const headsFormatVersion).
//
// (3) Number of series in checkpoint as big-endian uint64.
//
// (4) Repeated once per series:
//
// (4.1) A flag byte, see flag constants above.
//
// (4.2) The fingerprint as big-endian uint64.
//
// (4.3) The metric as defined by codable.Metric.
//
// (4.4) The varint-encoded chunkDescsOffset.
//
// (4.5) The varint-encoded savedFirstTime.
//
// (4.6) The varint-encoded number of chunk descriptors.
//
// (4.7) Repeated once per chunk descriptor, oldest to most recent:
//
// (4.7.1) The varint-encoded first time.
//
// (4.7.2) The varint-encoded last time.
//
// (4.8) Exception to 4.7: If the most recent chunk is a non-persisted head chunk,
// the following is persisted instead of the most recent chunk descriptor:
//
// (4.8.1) A byte defining the chunk type.
//
// (4.8.2) The head chunk itself, marshaled with the marshal() method.
//
func (p *persistence) checkpointSeriesMapAndHeads(fingerprintToSeries *seriesMap, fpLocker *fingerprintLocker) (err error) {
	glog.Info("Checkpointing in-memory metrics and head chunks...")
	begin := time.Now()
	f, err := os.OpenFile(p.headsTempFileName(), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0640)
	if err != nil {
		return
	}

	defer func() {
		closeErr := f.Close()
		if err != nil {
			return
		}
		err = closeErr
		if err != nil {
			return
		}
		err = os.Rename(p.headsTempFileName(), p.headsFileName())
		duration := time.Since(begin)
		p.checkpointDuration.Set(float64(duration) / float64(time.Millisecond))
		glog.Infof("Done checkpointing in-memory metrics and head chunks in %v.", duration)
	}()

	w := bufio.NewWriterSize(f, fileBufSize)

	if _, err = w.WriteString(headsMagicString); err != nil {
		return
	}
	var numberOfSeriesOffset int
	if numberOfSeriesOffset, err = codable.EncodeVarint(w, headsFormatVersion); err != nil {
		return
	}
	numberOfSeriesOffset += len(headsMagicString)
	numberOfSeriesInHeader := uint64(fingerprintToSeries.length())
	// We have to write the number of series as uint64 because we might need
	// to overwrite it later, and a varint might change byte width then.
	if err = codable.EncodeUint64(w, numberOfSeriesInHeader); err != nil {
		return
	}

	iter := fingerprintToSeries.iter()
	defer func() {
		// Consume the iterator in any case to not leak goroutines.
		for range iter {
		}
	}()

	var realNumberOfSeries uint64
	for m := range iter {
		func() { // Wrapped in function to use defer for unlocking the fp.
			fpLocker.Lock(m.fp)
			defer fpLocker.Unlock(m.fp)

			if len(m.series.chunkDescs) == 0 {
				// This series was completely purged or archived in the meantime. Ignore.
				return
			}
			realNumberOfSeries++
			var seriesFlags byte
			if m.series.headChunkPersisted {
				seriesFlags |= flagHeadChunkPersisted
			}
			if err = w.WriteByte(seriesFlags); err != nil {
				return
			}
			if err = codable.EncodeUint64(w, uint64(m.fp)); err != nil {
				return
			}
			var buf []byte
			buf, err = codable.Metric(m.series.metric).MarshalBinary()
			if err != nil {
				return
			}
			w.Write(buf)
			if _, err = codable.EncodeVarint(w, int64(m.series.chunkDescsOffset)); err != nil {
				return
			}
			if _, err = codable.EncodeVarint(w, int64(m.series.savedFirstTime)); err != nil {
				return
			}
			if _, err = codable.EncodeVarint(w, int64(len(m.series.chunkDescs))); err != nil {
				return
			}
			for i, chunkDesc := range m.series.chunkDescs {
				if m.series.headChunkPersisted || i < len(m.series.chunkDescs)-1 {
					if _, err = codable.EncodeVarint(w, int64(chunkDesc.firstTime())); err != nil {
						return
					}
					if _, err = codable.EncodeVarint(w, int64(chunkDesc.lastTime())); err != nil {
						return
					}
				} else {
					// This is the non-persisted head chunk. Fully marshal it.
					if err = w.WriteByte(chunkType(chunkDesc.chunk)); err != nil {
						return
					}
					if err = chunkDesc.chunk.marshal(w); err != nil {
						return
					}
				}
			}
		}()
		if err != nil {
			return
		}
	}
	if err = w.Flush(); err != nil {
		return
	}
	if realNumberOfSeries != numberOfSeriesInHeader {
		// The number of series has changed in the meantime.
		// Rewrite it in the header.
		if _, err = f.Seek(int64(numberOfSeriesOffset), os.SEEK_SET); err != nil {
			return
		}
		if err = codable.EncodeUint64(f, realNumberOfSeries); err != nil {
			return
		}
	}
	return
}

// loadSeriesMapAndHeads loads the fingerprint to memory-series mapping and all
// open (non-full) head chunks. If recoverable corruption is detected, or if the
// dirty flag was set from the beginning, crash recovery is run, which might
// take a while. If an unrecoverable error is encountered, it is returned. Call
// this method during start-up while nothing else is running in storage
// land. This method is utterly goroutine-unsafe.
func (p *persistence) loadSeriesMapAndHeads() (sm *seriesMap, err error) {
	var chunksTotal, chunkDescsTotal int64
	fingerprintToSeries := make(map[clientmodel.Fingerprint]*memorySeries)
	sm = &seriesMap{m: fingerprintToSeries}

	defer func() {
		if sm != nil && p.dirty {
			glog.Warning("Persistence layer appears dirty.")
			err = p.recoverFromCrash(fingerprintToSeries)
			if err != nil {
				sm = nil
			}
		}
		if err == nil {
			atomic.AddInt64(&numMemChunks, chunksTotal)
			numMemChunkDescs.Add(float64(chunkDescsTotal))
		}
	}()

	f, err := os.Open(p.headsFileName())
	if os.IsNotExist(err) {
		return sm, nil
	}
	if err != nil {
		glog.Warning("Could not open heads file:", err)
		p.dirty = true
		return
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, fileBufSize)

	buf := make([]byte, len(headsMagicString))
	if _, err := io.ReadFull(r, buf); err != nil {
		glog.Warning("Could not read from heads file:", err)
		p.dirty = true
		return sm, nil
	}
	magic := string(buf)
	if magic != headsMagicString {
		glog.Warningf(
			"unexpected magic string, want %q, got %q",
			headsMagicString, magic,
		)
		p.dirty = true
		return
	}
	if version, err := binary.ReadVarint(r); version != headsFormatVersion || err != nil {
		glog.Warningf("unknown heads format version, want %d", headsFormatVersion)
		p.dirty = true
		return sm, nil
	}
	numSeries, err := codable.DecodeUint64(r)
	if err != nil {
		glog.Warning("Could not decode number of series:", err)
		p.dirty = true
		return sm, nil
	}

	for ; numSeries > 0; numSeries-- {
		seriesFlags, err := r.ReadByte()
		if err != nil {
			glog.Warning("Could not read series flags:", err)
			p.dirty = true
			return sm, nil
		}
		headChunkPersisted := seriesFlags&flagHeadChunkPersisted != 0
		fp, err := codable.DecodeUint64(r)
		if err != nil {
			glog.Warning("Could not decode fingerprint:", err)
			p.dirty = true
			return sm, nil
		}
		var metric codable.Metric
		if err := metric.UnmarshalFromReader(r); err != nil {
			glog.Warning("Could not decode metric:", err)
			p.dirty = true
			return sm, nil
		}
		chunkDescsOffset, err := binary.ReadVarint(r)
		if err != nil {
			glog.Warning("Could not decode chunk descriptor offset:", err)
			p.dirty = true
			return sm, nil
		}
		savedFirstTime, err := binary.ReadVarint(r)
		if err != nil {
			glog.Warning("Could not decode saved first time:", err)
			p.dirty = true
			return sm, nil
		}
		numChunkDescs, err := binary.ReadVarint(r)
		if err != nil {
			glog.Warning("Could not decode number of chunk descriptors:", err)
			p.dirty = true
			return sm, nil
		}
		chunkDescs := make([]*chunkDesc, numChunkDescs)
		chunkDescsTotal += numChunkDescs

		for i := int64(0); i < numChunkDescs; i++ {
			if headChunkPersisted || i < numChunkDescs-1 {
				firstTime, err := binary.ReadVarint(r)
				if err != nil {
					glog.Warning("Could not decode first time:", err)
					p.dirty = true
					return sm, nil
				}
				lastTime, err := binary.ReadVarint(r)
				if err != nil {
					glog.Warning("Could not decode last time:", err)
					p.dirty = true
					return sm, nil
				}
				chunkDescs[i] = &chunkDesc{
					chunkFirstTime: clientmodel.Timestamp(firstTime),
					chunkLastTime:  clientmodel.Timestamp(lastTime),
				}
			} else {
				// Non-persisted head chunk.
				chunksTotal++
				chunkType, err := r.ReadByte()
				if err != nil {
					glog.Warning("Could not decode chunk type:", err)
					p.dirty = true
					return sm, nil
				}
				chunk := chunkForType(chunkType)
				if err := chunk.unmarshal(r); err != nil {
					glog.Warning("Could not decode chunk type:", err)
					p.dirty = true
					return sm, nil
				}
				chunkDescs[i] = newChunkDesc(chunk)
			}
		}

		fingerprintToSeries[clientmodel.Fingerprint(fp)] = &memorySeries{
			metric:             clientmodel.Metric(metric),
			chunkDescs:         chunkDescs,
			chunkDescsOffset:   int(chunkDescsOffset),
			savedFirstTime:     clientmodel.Timestamp(savedFirstTime),
			headChunkPersisted: headChunkPersisted,
		}
	}
	return sm, nil
}

// dropChunks deletes all chunks from a series whose last sample time is before
// beforeTime. It returns the timestamp of the first sample in the oldest chunk
// _not_ dropped, the number of deleted chunks, and true if all chunks of the
// series have been deleted (in which case the returned timestamp will be 0 and
// must be ignored).  It is the caller's responsibility to make sure nothing is
// persisted or loaded for the same fingerprint concurrently.
func (p *persistence) dropChunks(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) (
	firstTimeNotDropped clientmodel.Timestamp,
	numDropped int,
	allDropped bool,
	err error,
) {
	defer func() {
		if err != nil {
			p.setDirty(true)
		}
	}()
	f, err := p.openChunkFileForReading(fp)
	if os.IsNotExist(err) {
		return 0, 0, true, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	defer f.Close()

	// Find the first chunk that should be kept.
	var i int
	var firstTime clientmodel.Timestamp
	for ; ; i++ {
		_, err := f.Seek(p.offsetForChunkIndex(i)+chunkHeaderFirstTimeOffset, os.SEEK_SET)
		if err != nil {
			return 0, 0, false, err
		}
		timeBuf := make([]byte, 16)
		_, err = io.ReadAtLeast(f, timeBuf, 16)
		if err == io.EOF {
			// We ran into the end of the file without finding any chunks that should
			// be kept. Remove the whole file.
			chunkOps.WithLabelValues(purge).Add(float64(i))
			if err := os.Remove(f.Name()); err != nil {
				return 0, 0, true, err
			}
			return 0, i, true, nil
		}
		if err != nil {
			return 0, 0, false, err
		}
		lastTime := clientmodel.Timestamp(binary.LittleEndian.Uint64(timeBuf[8:]))
		if !lastTime.Before(beforeTime) {
			firstTime = clientmodel.Timestamp(binary.LittleEndian.Uint64(timeBuf))
			chunkOps.WithLabelValues(purge).Add(float64(i))
			break
		}
	}

	// We've found the first chunk that should be kept. Seek backwards to the
	// beginning of its header and start copying everything from there into a new
	// file.
	_, err = f.Seek(-(chunkHeaderFirstTimeOffset + 16), os.SEEK_CUR)
	if err != nil {
		return 0, 0, false, err
	}

	temp, err := os.OpenFile(p.tempFileNameForFingerprint(fp), os.O_WRONLY|os.O_CREATE, 0640)
	if err != nil {
		return 0, 0, false, err
	}
	defer temp.Close()

	if _, err := io.Copy(temp, f); err != nil {
		return 0, 0, false, err
	}

	if err := os.Rename(p.tempFileNameForFingerprint(fp), p.fileNameForFingerprint(fp)); err != nil {
		return 0, 0, false, err
	}
	return firstTime, i, false, nil
}

// indexMetric queues the given metric for addition to the indexes needed by
// getFingerprintsForLabelPair, getLabelValuesForLabelName, and
// getFingerprintsModifiedBefore.  If the queue is full, this method blocks
// until the metric can be queued.  This method is goroutine-safe.
func (p *persistence) indexMetric(fp clientmodel.Fingerprint, m clientmodel.Metric) {
	p.indexingQueue <- indexingOp{fp, m, add}
}

// unindexMetric queues references to the given metric for removal from the
// indexes used for getFingerprintsForLabelPair, getLabelValuesForLabelName, and
// getFingerprintsModifiedBefore. The index of fingerprints to archived metrics
// is not affected by this removal. (In fact, never call this method for an
// archived metric. To drop an archived metric, call dropArchivedFingerprint.)
// If the queue is full, this method blocks until the metric can be queued. This
// method is goroutine-safe.
func (p *persistence) unindexMetric(fp clientmodel.Fingerprint, m clientmodel.Metric) {
	p.indexingQueue <- indexingOp{fp, m, remove}
}

// waitForIndexing waits until all items in the indexing queue are processed. If
// queue processing is currently on hold (to gather more ops for batching), this
// method will trigger an immediate start of processing. This method is
// goroutine-safe.
func (p *persistence) waitForIndexing() {
	wait := make(chan int)
	for {
		p.indexingFlush <- wait
		if <-wait == 0 {
			break
		}
	}
}

// archiveMetric persists the mapping of the given fingerprint to the given
// metric, together with the first and last timestamp of the series belonging to
// the metric. The caller must have locked the fingerprint.
func (p *persistence) archiveMetric(
	fp clientmodel.Fingerprint, m clientmodel.Metric, first, last clientmodel.Timestamp,
) error {
	if err := p.archivedFingerprintToMetrics.Put(codable.Fingerprint(fp), codable.Metric(m)); err != nil {
		p.setDirty(true)
		return err
	}
	if err := p.archivedFingerprintToTimeRange.Put(codable.Fingerprint(fp), codable.TimeRange{First: first, Last: last}); err != nil {
		p.setDirty(true)
		return err
	}
	return nil
}

// hasArchivedMetric returns whether the archived metric for the given
// fingerprint exists and if yes, what the first and last timestamp in the
// corresponding series is. This method is goroutine-safe.
func (p *persistence) hasArchivedMetric(fp clientmodel.Fingerprint) (
	hasMetric bool, firstTime, lastTime clientmodel.Timestamp, err error,
) {
	firstTime, lastTime, hasMetric, err = p.archivedFingerprintToTimeRange.Lookup(fp)
	return
}

// updateArchivedTimeRange updates an archived time range. The caller must make
// sure that the fingerprint is currently archived (the time range will
// otherwise be added without the corresponding metric in the archive).
func (p *persistence) updateArchivedTimeRange(
	fp clientmodel.Fingerprint, first, last clientmodel.Timestamp,
) error {
	return p.archivedFingerprintToTimeRange.Put(codable.Fingerprint(fp), codable.TimeRange{First: first, Last: last})
}

// getFingerprintsModifiedBefore returns the fingerprints of archived timeseries
// that have live samples before the provided timestamp. This method is
// goroutine-safe.
func (p *persistence) getFingerprintsModifiedBefore(beforeTime clientmodel.Timestamp) ([]clientmodel.Fingerprint, error) {
	var fp codable.Fingerprint
	var tr codable.TimeRange
	fps := []clientmodel.Fingerprint{}
	p.archivedFingerprintToTimeRange.ForEach(func(kv index.KeyValueAccessor) error {
		if err := kv.Value(&tr); err != nil {
			return err
		}
		if tr.First.Before(beforeTime) {
			if err := kv.Key(&fp); err != nil {
				return err
			}
			fps = append(fps, clientmodel.Fingerprint(fp))
		}
		return nil
	})
	return fps, nil
}

// getArchivedMetric retrieves the archived metric with the given
// fingerprint. This method is goroutine-safe.
func (p *persistence) getArchivedMetric(fp clientmodel.Fingerprint) (clientmodel.Metric, error) {
	metric, _, err := p.archivedFingerprintToMetrics.Lookup(fp)
	return metric, err
}

// dropArchivedMetric deletes an archived fingerprint and its corresponding
// metric entirely. It also queues the metric for un-indexing (no need to call
// unindexMetric for the deleted metric.)  The caller must have locked the
// fingerprint.
func (p *persistence) dropArchivedMetric(fp clientmodel.Fingerprint) (err error) {
	defer func() {
		if err != nil {
			p.setDirty(true)
		}
	}()

	metric, err := p.getArchivedMetric(fp)
	if err != nil || metric == nil {
		return err
	}
	deleted, err := p.archivedFingerprintToMetrics.Delete(codable.Fingerprint(fp))
	if err != nil {
		return err
	}
	if !deleted {
		glog.Errorf("Tried to delete non-archived fingerprint %s from archivedFingerprintToMetrics index. This should never happen.", fp)
	}
	deleted, err = p.archivedFingerprintToTimeRange.Delete(codable.Fingerprint(fp))
	if err != nil {
		return err
	}
	if !deleted {
		glog.Errorf("Tried to delete non-archived fingerprint %s from archivedFingerprintToTimeRange index. This should never happen.", fp)
	}
	p.unindexMetric(fp, metric)
	return nil
}

// unarchiveMetric deletes an archived fingerprint and its metric, but (in
// contrast to dropArchivedMetric) does not un-index the metric.  If a metric
// was actually deleted, the method returns true and the first time of the
// deleted metric. The caller must have locked the fingerprint.
func (p *persistence) unarchiveMetric(fp clientmodel.Fingerprint) (
	deletedAnything bool,
	firstDeletedTime clientmodel.Timestamp,
	err error,
) {
	defer func() {
		if err != nil {
			p.setDirty(true)
		}
	}()

	firstTime, _, has, err := p.archivedFingerprintToTimeRange.Lookup(fp)
	if err != nil || !has {
		return false, firstTime, err
	}
	deleted, err := p.archivedFingerprintToMetrics.Delete(codable.Fingerprint(fp))
	if err != nil {
		return false, firstTime, err
	}
	if !deleted {
		glog.Errorf("Tried to delete non-archived fingerprint %s from archivedFingerprintToMetrics index. This should never happen.", fp)
	}
	deleted, err = p.archivedFingerprintToTimeRange.Delete(codable.Fingerprint(fp))
	if err != nil {
		return false, firstTime, err
	}
	if !deleted {
		glog.Errorf("Tried to delete non-archived fingerprint %s from archivedFingerprintToTimeRange index. This should never happen.", fp)
	}
	return true, firstTime, nil
}

// close flushes the indexing queue and other buffered data and releases any
// held resources. It also removes the dirty marker file if successful and if
// the persistence is currently not marked as dirty.
func (p *persistence) close() error {
	close(p.indexingQueue)
	<-p.indexingStopped

	var lastError, dirtyFileRemoveError error
	if err := p.archivedFingerprintToMetrics.Close(); err != nil {
		lastError = err
		glog.Error("Error closing archivedFingerprintToMetric index DB: ", err)
	}
	if err := p.archivedFingerprintToTimeRange.Close(); err != nil {
		lastError = err
		glog.Error("Error closing archivedFingerprintToTimeRange index DB: ", err)
	}
	if err := p.labelPairToFingerprints.Close(); err != nil {
		lastError = err
		glog.Error("Error closing labelPairToFingerprints index DB: ", err)
	}
	if err := p.labelNameToLabelValues.Close(); err != nil {
		lastError = err
		glog.Error("Error closing labelNameToLabelValues index DB: ", err)
	}
	if lastError == nil && !p.isDirty() {
		dirtyFileRemoveError = os.Remove(p.dirtyFileName)
	}
	if err := p.fLock.Release(); err != nil {
		lastError = err
		glog.Error("Error releasing file lock: ", err)
	}
	if dirtyFileRemoveError != nil {
		// On Windows, removing the dirty file before unlocking is not
		// possible.  So remove it here if it failed above.
		lastError = os.Remove(p.dirtyFileName)
	}
	return lastError
}

func (p *persistence) dirNameForFingerprint(fp clientmodel.Fingerprint) string {
	fpStr := fp.String()
	return path.Join(p.basePath, fpStr[0:seriesDirNameLen])
}

func (p *persistence) fileNameForFingerprint(fp clientmodel.Fingerprint) string {
	fpStr := fp.String()
	return path.Join(p.basePath, fpStr[0:seriesDirNameLen], fpStr[seriesDirNameLen:]+seriesFileSuffix)
}

func (p *persistence) tempFileNameForFingerprint(fp clientmodel.Fingerprint) string {
	fpStr := fp.String()
	return path.Join(p.basePath, fpStr[0:seriesDirNameLen], fpStr[seriesDirNameLen:]+seriesTempFileSuffix)
}

func (p *persistence) openChunkFileForWriting(fp clientmodel.Fingerprint) (*os.File, error) {
	if err := os.MkdirAll(p.dirNameForFingerprint(fp), 0700); err != nil {
		return nil, err
	}
	return os.OpenFile(p.fileNameForFingerprint(fp), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
	// NOTE: Although the file was opened for append,
	//     f.Seek(0, os.SEEK_CUR)
	// would now return '0, nil', so we cannot check for a consistent file length right now.
	// However, the chunkIndexForOffset method is doing that check, so a wrong file length
	// would still be detected.
}

func (p *persistence) openChunkFileForReading(fp clientmodel.Fingerprint) (*os.File, error) {
	return os.Open(p.fileNameForFingerprint(fp))
}

func writeChunkHeader(w io.Writer, c chunk) error {
	header := make([]byte, chunkHeaderLen)
	header[chunkHeaderTypeOffset] = chunkType(c)
	binary.LittleEndian.PutUint64(header[chunkHeaderFirstTimeOffset:], uint64(c.firstTime()))
	binary.LittleEndian.PutUint64(header[chunkHeaderLastTimeOffset:], uint64(c.lastTime()))
	_, err := w.Write(header)
	return err
}

func (p *persistence) offsetForChunkIndex(i int) int64 {
	return int64(i * (chunkHeaderLen + p.chunkLen))
}

func (p *persistence) chunkIndexForOffset(offset int64) (int, error) {
	if int(offset)%(chunkHeaderLen+p.chunkLen) != 0 {
		return -1, fmt.Errorf(
			"offset %d is not a multiple of on-disk chunk length %d",
			offset, chunkHeaderLen+p.chunkLen,
		)
	}
	return int(offset) / (chunkHeaderLen + p.chunkLen), nil
}

func (p *persistence) headsFileName() string {
	return path.Join(p.basePath, headsFileName)
}

func (p *persistence) headsTempFileName() string {
	return path.Join(p.basePath, headsTempFileName)
}

func (p *persistence) processIndexingQueue() {
	batchSize := 0
	nameToValues := index.LabelNameLabelValuesMapping{}
	pairToFPs := index.LabelPairFingerprintsMapping{}
	batchTimeout := time.NewTimer(indexingBatchTimeout)
	defer batchTimeout.Stop()

	commitBatch := func() {
		p.indexingBatchSizes.Observe(float64(batchSize))
		defer func(begin time.Time) {
			p.indexingBatchLatency.Observe(float64(time.Since(begin) / time.Millisecond))
		}(time.Now())

		if err := p.labelPairToFingerprints.IndexBatch(pairToFPs); err != nil {
			glog.Error("Error indexing label pair to fingerprints batch: ", err)
		}
		if err := p.labelNameToLabelValues.IndexBatch(nameToValues); err != nil {
			glog.Error("Error indexing label name to label values batch: ", err)
		}
		batchSize = 0
		nameToValues = index.LabelNameLabelValuesMapping{}
		pairToFPs = index.LabelPairFingerprintsMapping{}
		batchTimeout.Reset(indexingBatchTimeout)
	}

	var flush chan chan int
loop:
	for {
		// Only process flush requests if the queue is currently empty.
		if len(p.indexingQueue) == 0 {
			flush = p.indexingFlush
		} else {
			flush = nil
		}
		select {
		case <-batchTimeout.C:
			// Only commit if we have something to commit _and_
			// nothing is waiting in the queue to be picked up. That
			// prevents a death spiral if the LookupSet calls below
			// are slow for some reason.
			if batchSize > 0 && len(p.indexingQueue) == 0 {
				commitBatch()
			} else {
				batchTimeout.Reset(indexingBatchTimeout)
			}
		case r := <-flush:
			if batchSize > 0 {
				commitBatch()
			}
			r <- len(p.indexingQueue)
		case op, ok := <-p.indexingQueue:
			if !ok {
				if batchSize > 0 {
					commitBatch()
				}
				break loop
			}

			batchSize++
			for ln, lv := range op.metric {
				lp := metric.LabelPair{Name: ln, Value: lv}
				baseFPs, ok := pairToFPs[lp]
				if !ok {
					var err error
					baseFPs, _, err = p.labelPairToFingerprints.LookupSet(lp)
					if err != nil {
						glog.Errorf("Error looking up label pair %v: %s", lp, err)
						continue
					}
					pairToFPs[lp] = baseFPs
				}
				baseValues, ok := nameToValues[ln]
				if !ok {
					var err error
					baseValues, _, err = p.labelNameToLabelValues.LookupSet(ln)
					if err != nil {
						glog.Errorf("Error looking up label name %v: %s", ln, err)
						continue
					}
					nameToValues[ln] = baseValues
				}
				switch op.opType {
				case add:
					baseFPs[op.fingerprint] = struct{}{}
					baseValues[lv] = struct{}{}
				case remove:
					delete(baseFPs, op.fingerprint)
					if len(baseFPs) == 0 {
						delete(baseValues, lv)
					}
				default:
					panic("unknown op type")
				}
			}

			if batchSize >= indexingMaxBatchSize {
				commitBatch()
			}
		}
	}
	close(p.indexingStopped)
}
