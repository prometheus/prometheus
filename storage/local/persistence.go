package local

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/local/codec"
	"github.com/prometheus/prometheus/storage/local/index"
	"github.com/prometheus/prometheus/storage/metric"
)

const (
	seriesFileName     = "series.db"
	seriesTempFileName = "series.db.tmp"

	headsFileName      = "heads.db"
	headsFormatVersion = 1
	headsMagicString   = "PrometheusHeads"

	fileBufSize = 1 << 16 // 64kiB. TODO: Tweak.

	chunkHeaderLen             = 17
	chunkHeaderTypeOffset      = 0
	chunkHeaderFirstTimeOffset = 1
	chunkHeaderLastTimeOffset  = 9
)

const (
	_                         = iota
	flagChunkDescsLoaded byte = 1 << iota
	flagHeadChunkPersisted
)

type diskPersistence struct {
	basePath string
	chunkLen int

	archivedFingerprintToMetrics   *index.FingerprintMetricIndex
	archivedFingerprintToTimeRange *index.FingerprintTimeRangeIndex
	labelPairToFingerprints        *index.LabelPairFingerprintIndex
	labelNameToLabelValues         *index.LabelNameLabelValuesIndex
}

// NewDiskPersistence returns a newly allocated Persistence backed by local disk storage, ready to use.
func NewDiskPersistence(basePath string, chunkLen int) (Persistence, error) {
	if err := os.MkdirAll(basePath, 0700); err != nil {
		return nil, err
	}
	dp := &diskPersistence{
		basePath: basePath,
		chunkLen: chunkLen,
	}
	var err error
	dp.archivedFingerprintToMetrics, err = index.NewFingerprintMetricIndex(basePath)
	if err != nil {
		return nil, err
	}
	dp.archivedFingerprintToTimeRange, err = index.NewFingerprintTimeRangeIndex(basePath)
	if err != nil {
		return nil, err
	}
	dp.labelPairToFingerprints, err = index.NewLabelPairFingerprintIndex(basePath)
	if err != nil {
		return nil, err
	}
	dp.labelNameToLabelValues, err = index.NewLabelNameLabelValuesIndex(basePath)
	if err != nil {
		return nil, err
	}

	return dp, nil
}

func (p *diskPersistence) GetFingerprintsForLabelPair(lp metric.LabelPair) (clientmodel.Fingerprints, error) {
	fps, _, err := p.labelPairToFingerprints.Lookup(lp)
	if err != nil {
		return nil, err
	}
	return fps, nil
}

func (p *diskPersistence) GetLabelValuesForLabelName(ln clientmodel.LabelName) (clientmodel.LabelValues, error) {
	lvs, _, err := p.labelNameToLabelValues.Lookup(ln)
	if err != nil {
		return nil, err
	}
	return lvs, nil
}

func (p *diskPersistence) PersistChunk(fp clientmodel.Fingerprint, c chunk) error {
	// 1. Open chunk file.
	f, err := p.openChunkFileForWriting(fp)
	if err != nil {
		return err
	}
	defer f.Close()

	b := bufio.NewWriterSize(f, chunkHeaderLen+p.chunkLen)
	defer b.Flush()

	// 2. Write the header (chunk type and first/last times).
	err = writeChunkHeader(b, c)
	if err != nil {
		return err
	}

	// 3. Write chunk into file.
	return c.marshal(b)
}

func (p *diskPersistence) LoadChunks(fp clientmodel.Fingerprint, indexes []int) (chunks, error) {
	// TODO: we need to verify at some point that file length is a multiple of
	// the chunk size. When is the best time to do this, and where to remember
	// it? Right now, we only do it when loading chunkDescs.
	f, err := p.openChunkFileForReading(fp)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	chunks := make(chunks, 0, len(indexes))
	defer func() {
		if err == nil {
			return
		}
	}()

	typeBuf := make([]byte, 1)
	for _, idx := range indexes {
		_, err := f.Seek(p.offsetForChunkIndex(idx), os.SEEK_SET)
		if err != nil {
			return nil, err
		}
		// TODO: check seek offset too?

		n, err := f.Read(typeBuf)
		if err != nil {
			return nil, err
		}
		if n != 1 {
			// Shouldn't happen?
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

func (p *diskPersistence) LoadChunkDescs(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) (chunkDescs, error) {
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
		// TODO: record number of encountered corrupt series files in a metric?

		// Truncate the file size to the nearest multiple of chunkLen.
		truncateTo := fi.Size() - fi.Size()%int64(totalChunkLen)
		glog.Infof("Bad series file size for %s: %d bytes (no multiple of %d). Truncating to %d bytes.", fp, fi.Size(), totalChunkLen, truncateTo)
		// TODO: this doesn't work, as this is a read-only file handle.
		if err := f.Truncate(truncateTo); err != nil {
			return nil, err
		}
	}

	numChunks := int(fi.Size()) / totalChunkLen
	cds := make(chunkDescs, 0, numChunks)
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
			firstTimeField: clientmodel.Timestamp(binary.LittleEndian.Uint64(chunkTimesBuf)),
			lastTimeField:  clientmodel.Timestamp(binary.LittleEndian.Uint64(chunkTimesBuf[8:])),
		}
		if !cd.firstTime().Before(beforeTime) {
			// From here on, we have chunkDescs in memory already.
			break
		}
		cds = append(cds, cd)
	}
	return cds, nil
}

func (p *diskPersistence) PersistSeriesMapAndHeads(fingerprintToSeries SeriesMap) error {
	f, err := os.OpenFile(p.headsPath(), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0640)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, fileBufSize)

	if _, err := w.WriteString(headsMagicString); err != nil {
		return err
	}
	if err := codec.EncodeVarint(w, headsFormatVersion); err != nil {
		return err
	}
	if err := codec.EncodeVarint(w, int64(len(fingerprintToSeries))); err != nil {
		return err
	}

	for fp, series := range fingerprintToSeries {
		var seriesFlags byte
		if series.chunkDescsLoaded {
			seriesFlags |= flagChunkDescsLoaded
		}
		if series.headChunkPersisted {
			seriesFlags |= flagHeadChunkPersisted
		}
		if err := w.WriteByte(seriesFlags); err != nil {
			return err
		}
		if err := codec.EncodeUint64(w, uint64(fp)); err != nil {
			return err
		}
		buf, err := codec.CodableMetric(series.metric).MarshalBinary()
		if err != nil {
			return err
		}
		w.Write(buf)
		if err := codec.EncodeVarint(w, int64(len(series.chunkDescs))); err != nil {
			return err
		}
		for i, chunkDesc := range series.chunkDescs {
			if series.headChunkPersisted || i < len(series.chunkDescs)-1 {
				if err := codec.EncodeVarint(w, int64(chunkDesc.firstTime())); err != nil {
					return err
				}
				if err := codec.EncodeVarint(w, int64(chunkDesc.lastTime())); err != nil {
					return err
				}
			} else {
				// This is the non-persisted head chunk. Fully marshal it.
				if err := w.WriteByte(chunkType(chunkDesc.chunk)); err != nil {
					return err
				}
				if err := chunkDesc.chunk.marshal(w); err != nil {
					return err
				}
			}
		}
	}
	return w.Flush()
}

func (p *diskPersistence) LoadSeriesMapAndHeads() (SeriesMap, error) {
	f, err := os.Open(p.headsPath())
	if os.IsNotExist(err) {
		return SeriesMap{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, fileBufSize)

	buf := make([]byte, len(headsMagicString))
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	magic := string(buf)
	if magic != headsMagicString {
		return nil, fmt.Errorf(
			"unexpected magic string, want %q, got %q",
			headsMagicString, magic,
		)
	}
	if version, err := binary.ReadVarint(r); version != headsFormatVersion || err != nil {
		return nil, fmt.Errorf("unknown heads format version, want %d", headsFormatVersion)
	}
	numSeries, err := binary.ReadVarint(r)
	if err != nil {
		return nil, err
	}
	fingerprintToSeries := make(SeriesMap, numSeries)

	for ; numSeries > 0; numSeries-- {
		seriesFlags, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		headChunkPersisted := seriesFlags&flagHeadChunkPersisted != 0
		fp, err := codec.DecodeUint64(r)
		if err != nil {
			return nil, err
		}
		var metric codec.CodableMetric
		if err := metric.UnmarshalFromReader(r); err != nil {
			return nil, err
		}
		numChunkDescs, err := binary.ReadVarint(r)
		if err != nil {
			return nil, err
		}
		chunkDescs := make(chunkDescs, numChunkDescs)

		for i := int64(0); i < numChunkDescs; i++ {
			if headChunkPersisted || i < numChunkDescs-1 {
				firstTime, err := binary.ReadVarint(r)
				if err != nil {
					return nil, err
				}
				lastTime, err := binary.ReadVarint(r)
				if err != nil {
					return nil, err
				}
				chunkDescs[i] = &chunkDesc{
					firstTimeField: clientmodel.Timestamp(firstTime),
					lastTimeField:  clientmodel.Timestamp(lastTime),
				}
			} else {
				// Non-persisted head chunk.
				chunkType, err := r.ReadByte()
				if err != nil {
					return nil, err
				}
				chunk := chunkForType(chunkType)
				if err := chunk.unmarshal(r); err != nil {
					return nil, err
				}
				chunkDescs[i] = &chunkDesc{
					chunk:    chunk,
					refCount: 1,
				}
			}
		}

		fingerprintToSeries[clientmodel.Fingerprint(fp)] = &memorySeries{
			metric:             clientmodel.Metric(metric),
			chunkDescs:         chunkDescs,
			chunkDescsLoaded:   seriesFlags&flagChunkDescsLoaded != 0,
			headChunkPersisted: headChunkPersisted,
		}
	}
	return fingerprintToSeries, nil
}

func (p *diskPersistence) DropChunks(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) (bool, error) {
	f, err := p.openChunkFileForReading(fp)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Find the first chunk that should be kept.
	for i := 0; ; i++ {
		_, err := f.Seek(p.offsetForChunkIndex(i)+chunkHeaderLastTimeOffset, os.SEEK_SET)
		if err != nil {
			return false, err
		}
		lastTimeBuf := make([]byte, 8)
		_, err = io.ReadAtLeast(f, lastTimeBuf, 8)
		if err == io.EOF {
			// We ran into the end of the file without finding any chunks that should
			// be kept. Remove the whole file.
			if err := os.Remove(f.Name()); err != nil {
				return true, err
			}
			return true, nil
		}
		if err != nil {
			return false, err
		}
		lastTime := clientmodel.Timestamp(binary.LittleEndian.Uint64(lastTimeBuf))
		if !lastTime.Before(beforeTime) {
			break
		}
	}

	// We've found the first chunk that should be kept. Seek backwards to the
	// beginning of its header and start copying everything from there into a new
	// file.
	_, err = f.Seek(-(chunkHeaderLastTimeOffset + 8), os.SEEK_CUR)
	if err != nil {
		return false, err
	}

	dirname := p.dirForFingerprint(fp)
	temp, err := os.OpenFile(path.Join(dirname, seriesTempFileName), os.O_WRONLY|os.O_CREATE, 0640)
	if err != nil {
		return false, err
	}
	defer temp.Close()

	if _, err := io.Copy(temp, f); err != nil {
		return false, err
	}

	os.Rename(path.Join(dirname, seriesTempFileName), path.Join(dirname, seriesFileName))
	return false, nil
}

func (p *diskPersistence) IndexMetric(m clientmodel.Metric, fp clientmodel.Fingerprint) error {
	// TODO: Don't do it directly, but add it to a queue (which needs to be
	// drained before shutdown). Queuing would make this asynchronously, and
	// then batches could be created easily.
	if err := p.labelNameToLabelValues.Extend(m); err != nil {
		return err
	}
	return p.labelPairToFingerprints.Extend(m, fp)
}

func (p *diskPersistence) UnindexMetric(m clientmodel.Metric, fp clientmodel.Fingerprint) error {
	// TODO: Don't do it directly, but add it to a queue (which needs to be
	// drained before shutdown). Queuing would make this asynchronously, and
	// then batches could be created easily.
	labelPairs, err := p.labelPairToFingerprints.Reduce(m, fp)
	if err != nil {
		return err
	}
	return p.labelNameToLabelValues.Reduce(labelPairs)
}

func (p *diskPersistence) ArchiveMetric(
	// TODO: Two step process, make sure this happens atomically.
	fp clientmodel.Fingerprint, m clientmodel.Metric, first, last clientmodel.Timestamp,
) error {
	if err := p.archivedFingerprintToMetrics.Put(codec.CodableFingerprint(fp), codec.CodableMetric(m)); err != nil {
		return err
	}
	if err := p.archivedFingerprintToTimeRange.Put(codec.CodableFingerprint(fp), codec.CodableTimeRange{First: first, Last: last}); err != nil {
		return err
	}
	return nil
}

func (p *diskPersistence) HasArchivedMetric(fp clientmodel.Fingerprint) (
	hasMetric bool, firstTime, lastTime clientmodel.Timestamp, err error,
) {
	firstTime, lastTime, hasMetric, err = p.archivedFingerprintToTimeRange.Lookup(fp)
	return
}

func (p *diskPersistence) GetArchivedMetric(fp clientmodel.Fingerprint) (clientmodel.Metric, error) {
	metric, _, err := p.archivedFingerprintToMetrics.Lookup(fp)
	return metric, err
}

func (p *diskPersistence) DropArchivedMetric(fp clientmodel.Fingerprint) error {
	// TODO: Multi-step process, make sure this happens atomically.
	metric, err := p.GetArchivedMetric(fp)
	if err != nil || metric == nil {
		return err
	}
	if err := p.archivedFingerprintToMetrics.Delete(codec.CodableFingerprint(fp)); err != nil {
		return err
	}
	if err := p.archivedFingerprintToTimeRange.Delete(codec.CodableFingerprint(fp)); err != nil {
		return err
	}
	return p.UnindexMetric(metric, fp)
}

func (p *diskPersistence) UnarchiveMetric(fp clientmodel.Fingerprint) (bool, error) {
	// TODO: Multi-step process, make sure this happens atomically.
	has, err := p.archivedFingerprintToTimeRange.Has(fp)
	if err != nil || !has {
		return false, err
	}
	if err := p.archivedFingerprintToMetrics.Delete(codec.CodableFingerprint(fp)); err != nil {
		return false, err
	}
	if err := p.archivedFingerprintToTimeRange.Delete(codec.CodableFingerprint(fp)); err != nil {
		return false, err
	}
	return true, nil
}

func (p *diskPersistence) Close() error {
	var lastError error
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
	return lastError
}

func (p *diskPersistence) dirForFingerprint(fp clientmodel.Fingerprint) string {
	fpStr := fp.String()
	return fmt.Sprintf("%s/%c%c/%s", p.basePath, fpStr[0], fpStr[1], fpStr[2:])
}

func (p *diskPersistence) openChunkFileForWriting(fp clientmodel.Fingerprint) (*os.File, error) {
	dirname := p.dirForFingerprint(fp)
	ex, err := exists(dirname)
	if err != nil {
		return nil, err
	}
	if !ex {
		if err := os.MkdirAll(dirname, 0700); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path.Join(dirname, seriesFileName), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
}

func (p *diskPersistence) openChunkFileForReading(fp clientmodel.Fingerprint) (*os.File, error) {
	dirname := p.dirForFingerprint(fp)
	return os.Open(path.Join(dirname, seriesFileName))
}

func writeChunkHeader(w io.Writer, c chunk) error {
	header := make([]byte, chunkHeaderLen)
	header[chunkHeaderTypeOffset] = chunkType(c)
	binary.LittleEndian.PutUint64(header[chunkHeaderFirstTimeOffset:], uint64(c.firstTime()))
	binary.LittleEndian.PutUint64(header[chunkHeaderLastTimeOffset:], uint64(c.lastTime()))
	_, err := w.Write(header)
	return err
}

func (p *diskPersistence) offsetForChunkIndex(i int) int64 {
	return int64(i * (chunkHeaderLen + p.chunkLen))
}

func (p *diskPersistence) headsPath() string {
	return path.Join(p.basePath, headsFileName)
}

// exists returns true when the given file or directory exists.
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}
