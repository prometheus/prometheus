package storage_ng

import (
	"bufio"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/golang/glog"

	//"github.com/prometheus/prometheus/storage/metric"

	//"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility"
)

const (
	seriesFileName     = "series.db"
	seriesTempFileName = "series.db.tmp"
	indexFileName      = "index.db"
	headsFileName      = "heads.db"

	chunkHeaderLen             = 17
	chunkHeaderTypeOffset      = 0
	chunkHeaderFirstTimeOffset = 1
	chunkHeaderLastTimeOffset  = 9

	headsHeaderLen               = 9
	headsHeaderFingerprintOffset = 0
	headsHeaderTypeOffset        = 8
)

type diskPersistence struct {
	basePath string
	chunkLen int
}

func NewDiskPersistence(basePath string, chunkLen int) (Persistence, error) {
	gob.Register(clientmodel.Fingerprint(0))
	gob.Register(clientmodel.LabelValue(""))

	err := os.MkdirAll(basePath, 0700)
	if err != nil {
		return nil, err
	}
	return &diskPersistence{
		basePath: basePath,
		chunkLen: chunkLen,
	}, nil
}

func (p *diskPersistence) dirForFingerprint(fp clientmodel.Fingerprint) string {
	fpStr := fp.String()
	return fmt.Sprintf("%s/%c%c/%s", p.basePath, fpStr[0], fpStr[1], fpStr[2:])
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

func (p *diskPersistence) offsetForChunkIndex(i int) int64 {
	return int64(i * (chunkHeaderLen + p.chunkLen))
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
		for _, c := range chunks {
			c.close()
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

func (p *diskPersistence) indexPath() string {
	return path.Join(p.basePath, indexFileName)
}

func (p *diskPersistence) PersistIndexes(i *Indexes) error {
	f, err := os.OpenFile(p.indexPath(), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0640)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := gob.NewEncoder(f)
	if err := enc.Encode(i); err != nil {
		return err
	}

	return nil
}

func (p *diskPersistence) LoadIndexes() (*Indexes, error) {
	f, err := os.Open(p.indexPath())
	if os.IsNotExist(err) {
		return &Indexes{
			FingerprintToSeries:     make(map[clientmodel.Fingerprint]*memorySeries),
			LabelPairToFingerprints: make(map[metric.LabelPair]utility.Set),
			LabelNameToLabelValues:  make(map[clientmodel.LabelName]utility.Set),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := gob.NewDecoder(f)
	var i Indexes
	if err := dec.Decode(&i); err != nil {
		return nil, err
	}

	return &i, nil
}

func (p *diskPersistence) headsPath() string {
	return path.Join(p.basePath, headsFileName)
}

func (p *diskPersistence) PersistHeads(fpToSeries map[clientmodel.Fingerprint]*memorySeries) error {
	f, err := os.OpenFile(p.headsPath(), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0640)
	if err != nil {
		return err
	}
	header := make([]byte, 9)
	for fp, series := range fpToSeries {
		head := series.head().chunk

		binary.LittleEndian.PutUint64(header[headsHeaderFingerprintOffset:], uint64(fp))
		header[headsHeaderTypeOffset] = chunkType(head)
		_, err := f.Write(header)
		if err != nil {
			return err
		}
		err = head.marshal(f)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *diskPersistence) DropChunks(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) error {
	f, err := p.openChunkFileForReading(fp)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	// Find the first chunk that should be kept.
	for i := 0; ; i++ {
		_, err := f.Seek(p.offsetForChunkIndex(i)+chunkHeaderLastTimeOffset, os.SEEK_SET)
		if err != nil {
			return err
		}
		lastTimeBuf := make([]byte, 8)
		_, err = io.ReadAtLeast(f, lastTimeBuf, 8)
		if err == io.EOF {
			// We ran into the end of the file without finding any chunks that should
			// be kept. Remove the whole file.
			if err := os.Remove(f.Name()); err != nil {
				return err
			}
			return nil
		}
		if err != nil {
			return err
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
		return err
	}

	dirname := p.dirForFingerprint(fp)
	temp, err := os.OpenFile(path.Join(dirname, seriesTempFileName), os.O_WRONLY|os.O_CREATE, 0640)
	if err != nil {
		return err
	}
	defer temp.Close()

	if _, err := io.Copy(temp, f); err != nil {
		return err
	}

	os.Rename(path.Join(dirname, seriesTempFileName), path.Join(dirname, seriesFileName))
	return nil
}

func (p *diskPersistence) LoadHeads(fpToSeries map[clientmodel.Fingerprint]*memorySeries) error {
	f, err := os.Open(p.headsPath())
	if os.IsNotExist(err) {
		// TODO: this should only happen if there never was a shutdown before. In
		// that case, all heads should be in order already, since the series got
		// created during this process' runtime.
		// Still, make this more robust.
		return nil
	}

	header := make([]byte, headsHeaderLen)
	for {
		_, err := io.ReadAtLeast(f, header, headsHeaderLen)
		if err == io.ErrUnexpectedEOF {
			// TODO: this should only be ok if n is 0.
			break
		}
		if err != nil {
			return nil
		}
		// TODO: this relies on the implementation (uint64) of Fingerprint.
		fp := clientmodel.Fingerprint(binary.LittleEndian.Uint64(header[headsHeaderFingerprintOffset:]))
		chunk := chunkForType(header[headsHeaderTypeOffset])
		chunk.unmarshal(f)
		fpToSeries[fp].chunkDescs = append(fpToSeries[fp].chunkDescs, &chunkDesc{
			chunk:    chunk,
			refCount: 1,
		})
	}
	return nil
}
