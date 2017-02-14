package tsdb

import (
	"bufio"
	"encoding/binary"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/fabxc/tsdb/labels"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
)

// WALEntryType indicates what data a WAL entry contains.
type WALEntryType byte

const (
	WALMagic = 0x43AF00EF

	// Format versioning flag of a WAL segment file.
	WALFormatDefault byte = 1

	// Entry types in a segment file.
	WALEntrySymbols = 1
	WALEntrySeries  = 2
	WALEntrySamples = 3
)

// WAL is a write ahead log for series data. It can only be written to.
// Use WALReader to read back from a write ahead log.
type WAL struct {
	mtx sync.Mutex

	dirFile *os.File
	files   []*fileutil.LockedFile

	logger        log.Logger
	flushInterval time.Duration

	cur  *bufio.Writer
	curN int

	stopc chan struct{}
	donec chan struct{}
}

const (
	walDirName          = "wal"
	walSegmentSizeBytes = 64 * 1000 * 1000 // 64 MB
)

// OpenWAL opens or creates a write ahead log in the given directory.
// The WAL must be read completely before new data is written.
func OpenWAL(dir string, l log.Logger, flushInterval time.Duration) (*WAL, error) {
	dir = filepath.Join(dir, walDirName)

	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}
	df, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, err
	}

	w := &WAL{
		dirFile:       df,
		logger:        l,
		flushInterval: flushInterval,
		donec:         make(chan struct{}),
		stopc:         make(chan struct{}),
	}
	if err := w.initSegments(); err != nil {
		return nil, err
	}
	// If there are no existing segments yet, create the initial one.
	if len(w.files) == 0 {
		if err := w.cut(); err != nil {
			return nil, err
		}
	}

	go w.run(flushInterval)

	return w, nil
}

type walHandler struct {
	sample func(refdSample) error
	series func(labels.Labels) error
}

// ReadAll consumes all entries in the WAL and triggers the registered handlers.
func (w *WAL) ReadAll(h *walHandler) error {
	for _, f := range w.files {
		dec := newWALDecoder(f, h)

		for {
			if err := dec.entry(); err != nil {
				if err == io.EOF {
					// If file end was reached, move on to the next segment.
					break
				}
				return err
			}
		}
	}
	return nil
}

// Log writes a batch of new series labels and samples to the log.
func (w *WAL) Log(series []labels.Labels, samples []refdSample) error {
	if err := w.encodeSeries(series); err != nil {
		return err
	}
	if err := w.encodeSamples(samples); err != nil {
		return err
	}
	if w.flushInterval <= 0 {
		return w.Sync()
	}
	return nil
}

// initSegments finds all existing segment files and opens them in the
// appropriate file modes.
func (w *WAL) initSegments() error {
	fns, err := sequenceFiles(w.dirFile.Name(), "")
	if err != nil {
		return err
	}
	if len(fns) == 0 {
		return nil
	}
	if len(fns) > 1 {
		for _, fn := range fns[:len(fns)-2] {
			lf, err := fileutil.TryLockFile(fn, os.O_RDONLY, 0666)
			if err != nil {
				return err
			}
			w.files = append(w.files, lf)
		}
	}
	// The most recent WAL file is the one we have to keep appending to.
	lf, err := fileutil.TryLockFile(fns[len(fns)-1], os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	w.files = append(w.files, lf)

	// Consume and validate meta headers.
	for _, f := range w.files {
		metab := make([]byte, 8)

		if n, err := f.Read(metab); err != nil {
			return errors.Wrapf(err, "validate meta %q", f.Name())
		} else if n != 8 {
			return errors.Errorf("invalid header size %d in %q", n, f.Name())
		}

		if m := binary.BigEndian.Uint32(metab[:4]); m != WALMagic {
			return errors.Errorf("invalid magic header %x in %q", m, f.Name())
		}
		if metab[4] != WALFormatDefault {
			return errors.Errorf("unknown WAL segment format %d in %q", metab[4], f.Name())
		}
	}

	return nil
}

// cut finishes the currently active segments and open the next one.
// The encoder is reset to point to the new segment.
func (w *WAL) cut() error {
	// If there's a previous segment, truncate it to its final size
	// and sync everything to disc.
	if tf := w.tail(); tf != nil {
		off, err := tf.Seek(0, os.SEEK_CUR)
		if err != nil {
			return err
		}
		if err := tf.Truncate(off); err != nil {
			return err
		}
		if err := w.sync(); err != nil {
			return err
		}
	}

	p, _, err := nextSequenceFile(w.dirFile.Name(), "")
	if err != nil {
		return err
	}
	f, err := fileutil.LockFile(p, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	if _, err = f.Seek(0, os.SEEK_SET); err != nil {
		return err
	}
	if err = fileutil.Preallocate(f.File, walSegmentSizeBytes, true); err != nil {
		return err
	}
	if err = w.dirFile.Sync(); err != nil {
		return err
	}

	// Write header metadata for new file.
	metab := make([]byte, 8)
	binary.BigEndian.PutUint32(metab[:4], WALMagic)
	metab[4] = WALFormatDefault

	if _, err := f.Write(metab); err != nil {
		return err
	}

	w.files = append(w.files, f)
	w.cur = bufio.NewWriterSize(f, 4*1024*1024)
	w.curN = len(metab)

	return nil
}

func (w *WAL) tail() *fileutil.LockedFile {
	if len(w.files) == 0 {
		return nil
	}
	return w.files[len(w.files)-1]
}

func (w *WAL) Sync() error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	return w.sync()
}

func (w *WAL) sync() error {
	if err := w.cur.Flush(); err != nil {
		return err
	}
	return fileutil.Fdatasync(w.tail().File)
}

func (w *WAL) run(interval time.Duration) {
	var tick <-chan time.Time

	if interval > 0 {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		tick = ticker.C
	}
	defer close(w.donec)

	for {
		select {
		case <-w.stopc:
			return
		case <-tick:
			if err := w.Sync(); err != nil {
				w.logger.Log("msg", "sync failed", "err", err)
			}
		}
	}
}

// Close sync all data and closes the underlying resources.
func (w *WAL) Close() error {
	close(w.stopc)
	<-w.donec

	w.mtx.Lock()
	defer w.mtx.Unlock()

	var merr MultiError

	if err := w.sync(); err != nil {
		return err
	}
	for _, f := range w.files {
		merr.Add(f.Close())
	}
	return merr.Err()
}

const (
	minSectorSize = 512

	// walPageBytes is the alignment for flushing records to the backing Writer.
	// It should be a multiple of the minimum sector size so that WAL can safely
	// distinguish between torn writes and ordinary data corruption.
	walPageBytes = 16 * minSectorSize
)

func (w *WAL) entry(et WALEntryType, flag byte, buf []byte) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	sz := 6 + 4 + len(buf)

	if w.curN+sz > walSegmentSizeBytes {
		if err := w.cut(); err != nil {
			return err
		}
	}

	h := crc32.NewIEEE()
	wr := io.MultiWriter(h, w.cur)

	b := make([]byte, 6)
	b[0] = byte(et)
	b[1] = flag

	binary.BigEndian.PutUint32(b[2:], uint32(len(buf)))

	if _, err := wr.Write(b); err != nil {
		return err
	}
	if _, err := wr.Write(buf); err != nil {
		return err
	}
	if _, err := w.cur.Write(h.Sum(nil)); err != nil {
		return err
	}

	w.curN += sz

	putWALBuffer(buf)
	return nil
}

const (
	walSeriesSimple  = 1
	walSamplesSimple = 1
)

var walBuffers = sync.Pool{}

func getWALBuffer() []byte {
	b := walBuffers.Get()
	if b == nil {
		return make([]byte, 0, 64*1024)
	}
	return b.([]byte)
}

func putWALBuffer(b []byte) {
	b = b[:0]
	walBuffers.Put(b)
}

func (w *WAL) encodeSeries(series []labels.Labels) error {
	if len(series) == 0 {
		return nil
	}

	b := make([]byte, binary.MaxVarintLen32)
	buf := getWALBuffer()

	for _, lset := range series {
		n := binary.PutUvarint(b, uint64(len(lset)))
		buf = append(buf, b[:n]...)

		for _, l := range lset {
			n = binary.PutUvarint(b, uint64(len(l.Name)))
			buf = append(buf, b[:n]...)
			buf = append(buf, l.Name...)

			n = binary.PutUvarint(b, uint64(len(l.Value)))
			buf = append(buf, b[:n]...)
			buf = append(buf, l.Value...)
		}
	}

	return w.entry(WALEntrySeries, walSeriesSimple, buf)
}

func (w *WAL) encodeSamples(samples []refdSample) error {
	if len(samples) == 0 {
		return nil
	}

	b := make([]byte, binary.MaxVarintLen64)
	buf := getWALBuffer()

	// Store base timestamp and base reference number of first sample.
	// All samples encode their timestamp and ref as delta to those.
	//
	// TODO(fabxc): optimize for all samples having the same timestamp.
	first := samples[0]

	binary.BigEndian.PutUint64(b, first.ref)
	buf = append(buf, b[:8]...)
	binary.BigEndian.PutUint64(b, uint64(first.t))
	buf = append(buf, b[:8]...)

	for _, s := range samples {
		n := binary.PutVarint(b, int64(s.ref)-int64(first.ref))
		buf = append(buf, b[:n]...)

		n = binary.PutVarint(b, s.t-first.t)
		buf = append(buf, b[:n]...)

		binary.BigEndian.PutUint64(b, math.Float64bits(s.v))
		buf = append(buf, b[:8]...)
	}

	return w.entry(WALEntrySamples, walSamplesSimple, buf)
}

type walDecoder struct {
	r       io.Reader
	handler *walHandler

	buf []byte
}

// newWALDecoder returns a new decoder for the default WAL format. The meta
// headers of a segment must already have been consumed.
func newWALDecoder(r io.Reader, h *walHandler) *walDecoder {
	return &walDecoder{
		r:       r,
		handler: h,
		buf:     make([]byte, 0, 1024*1024),
	}
}

func (d *walDecoder) decodeSeries(flag byte, b []byte) error {
	for len(b) > 0 {
		l, n := binary.Uvarint(b)
		if n < 1 {
			return errors.Wrap(errInvalidSize, "number of labels")
		}
		b = b[n:]
		lset := make(labels.Labels, l)

		for i := 0; i < int(l); i++ {
			nl, n := binary.Uvarint(b)
			if n < 1 || len(b) < n+int(nl) {
				return errors.Wrap(errInvalidSize, "label name")
			}
			lset[i].Name = string(b[n : n+int(nl)])
			b = b[n+int(nl):]

			vl, n := binary.Uvarint(b)
			if n < 1 || len(b) < n+int(vl) {
				return errors.Wrap(errInvalidSize, "label value")
			}
			lset[i].Value = string(b[n : n+int(vl)])
			b = b[n+int(vl):]
		}

		if err := d.handler.series(lset); err != nil {
			return err
		}
	}
	return nil
}

func (d *walDecoder) decodeSamples(flag byte, b []byte) error {
	if len(b) < 16 {
		return errors.Wrap(errInvalidSize, "header length")
	}
	var (
		baseRef  = binary.BigEndian.Uint64(b)
		baseTime = int64(binary.BigEndian.Uint64(b[8:]))
	)
	b = b[16:]

	for len(b) > 0 {
		var smpl refdSample

		dref, n := binary.Varint(b)
		if n < 1 {
			return errors.Wrap(errInvalidSize, "sample ref delta")
		}
		b = b[n:]

		smpl.ref = uint64(int64(baseRef) + dref)

		dtime, n := binary.Varint(b)
		if n < 1 {
			return errors.Wrap(errInvalidSize, "sample timestamp delta")
		}
		b = b[n:]
		smpl.t = baseTime + dtime

		if len(b) < 8 {
			return errors.Wrapf(errInvalidSize, "sample value bits %d", len(b))
		}
		smpl.v = float64(math.Float64frombits(binary.BigEndian.Uint64(b)))
		b = b[8:]

		if err := d.handler.sample(smpl); err != nil {
			return err
		}
	}
	return nil
}

func (d *walDecoder) entry() error {
	b := make([]byte, 6)
	if _, err := d.r.Read(b); err != nil {
		return err
	}

	var (
		etype  = WALEntryType(b[0])
		flag   = b[1]
		length = int(binary.BigEndian.Uint32(b[2:]))
	)
	// Exit if we reached pre-allocated space.
	if etype == 0 {
		return io.EOF
	}

	if length > len(d.buf) {
		d.buf = make([]byte, length)
	}
	buf := d.buf[:length]

	if _, err := d.r.Read(buf); err != nil {
		return err
	}
	// Read away checksum.
	// TODO(fabxc): verify it
	if _, err := d.r.Read(b[:4]); err != nil {
		return err
	}

	switch etype {
	case WALEntrySeries:
		return d.decodeSeries(flag, buf)
	case WALEntrySamples:
		return d.decodeSamples(flag, buf)
	}

	return errors.Errorf("unknown WAL entry type %q", etype)
}
