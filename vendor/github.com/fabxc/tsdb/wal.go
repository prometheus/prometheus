package tsdb

import (
	"bufio"
	"encoding/binary"
	"hash"
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
	// WALMagic is a 4 byte number every WAL segment file starts with.
	WALMagic = uint32(0x43AF00EF)

	// WALFormatDefault is the version flag for the default outer segment file format.
	WALFormatDefault = byte(1)
)

// Entry types in a segment file.
const (
	WALEntrySymbols WALEntryType = 1
	WALEntrySeries  WALEntryType = 2
	WALEntrySamples WALEntryType = 3
)

// WAL is a write ahead log for series data. It can only be written to.
// Use WALReader to read back from a write ahead log.
type WAL struct {
	mtx sync.Mutex

	dirFile *os.File
	files   []*os.File

	logger        log.Logger
	flushInterval time.Duration
	segmentSize   int64

	crc32 hash.Hash32
	cur   *bufio.Writer
	curN  int64

	stopc chan struct{}
	donec chan struct{}
}

const (
	walDirName          = "wal"
	walSegmentSizeBytes = 256 * 1024 * 1024 // 256 MB
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
		segmentSize:   walSegmentSizeBytes,
		crc32:         crc32.New(crc32.MakeTable(crc32.Castagnoli)),
	}
	if err := w.initSegments(); err != nil {
		return nil, err
	}

	go w.run(flushInterval)

	return w, nil
}

// Reader returns a new reader over the the write ahead log data.
// It must be completely consumed before writing to the WAL.
func (w *WAL) Reader() *WALReader {
	var rs []io.ReadCloser
	for _, f := range w.files {
		rs = append(rs, f)
	}
	return NewWALReader(rs...)
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
		for _, fn := range fns[:len(fns)-1] {
			f, err := os.Open(fn)
			if err != nil {
				return err
			}
			w.files = append(w.files, f)
		}
	}
	// The most recent WAL file is the one we have to keep appending to.
	f, err := os.OpenFile(fns[len(fns)-1], os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	w.files = append(w.files, f)

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
	// Sync current tail to disc and close.
	if tf := w.tail(); tf != nil {
		if err := w.sync(); err != nil {
			return err
		}
		off, err := tf.Seek(0, os.SEEK_CUR)
		if err != nil {
			return err
		}
		if err := tf.Truncate(off); err != nil {
			return err
		}
		if err := tf.Close(); err != nil {
			return err
		}
	}

	p, _, err := nextSequenceFile(w.dirFile.Name(), "")
	if err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	if err = fileutil.Preallocate(f, w.segmentSize, true); err != nil {
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
	w.curN = 8

	return nil
}

func (w *WAL) tail() *os.File {
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
	if w.cur == nil {
		return nil
	}
	if err := w.cur.Flush(); err != nil {
		return err
	}
	return fileutil.Fdatasync(w.tail())
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

	// Lock mutex and leave it locked so we panic if there's a bug causing
	// the block to be used afterwards.
	w.mtx.Lock()

	if err := w.sync(); err != nil {
		return err
	}
	// On opening, a WAL must be fully consumed once. Afterwards
	// only the current segment will still be open.
	if tf := w.tail(); tf != nil {
		return tf.Close()
	}
	return nil
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

	// Cut to the next segment if exceeds the file size unless it would also
	// exceed the size of a new segment.
	var (
		sz    = int64(6 + 4 + len(buf))
		newsz = w.curN + sz
	)
	if w.cur == nil || w.curN > w.segmentSize || newsz > w.segmentSize && sz <= w.segmentSize {
		if err := w.cut(); err != nil {
			return err
		}
	}

	w.crc32.Reset()
	wr := io.MultiWriter(w.crc32, w.cur)

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
	if _, err := w.cur.Write(w.crc32.Sum(nil)); err != nil {
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

// WALReader decodes and emits write ahead log entries.
type WALReader struct {
	rs    []io.ReadCloser
	cur   int
	buf   []byte
	crc32 hash.Hash32

	err     error
	labels  []labels.Labels
	samples []refdSample
}

// NewWALReader returns a new WALReader over the sequence of the given ReadClosers.
func NewWALReader(rs ...io.ReadCloser) *WALReader {
	return &WALReader{
		rs:    rs,
		buf:   make([]byte, 0, 128*4096),
		crc32: crc32.New(crc32.MakeTable(crc32.Castagnoli)),
	}
}

// At returns the last decoded entry of labels or samples.
func (r *WALReader) At() ([]labels.Labels, []refdSample) {
	return r.labels, r.samples
}

// Err returns the last error the reader encountered.
func (r *WALReader) Err() error {
	return r.err
}

// nextEntry retrieves the next entry. It is also used as a testing hook.
func (r *WALReader) nextEntry() (WALEntryType, byte, []byte, error) {
	if r.cur >= len(r.rs) {
		return 0, 0, nil, io.EOF
	}
	cr := r.rs[r.cur]

	et, flag, b, err := r.entry(cr)
	if err == io.EOF {
		// Current reader completed, close and move to the next one.
		if err := cr.Close(); err != nil {
			return 0, 0, nil, err
		}
		r.cur++
		return r.nextEntry()
	}
	return et, flag, b, err
}

// Next returns decodes the next entry pair and returns true
// if it was succesful.
func (r *WALReader) Next() bool {
	r.labels = r.labels[:0]
	r.samples = r.samples[:0]

	et, flag, b, err := r.nextEntry()
	if err != nil {
		if err != io.EOF {
			r.err = err
		}
		return false
	}

	switch et {
	case WALEntrySamples:
		if err := r.decodeSamples(flag, b); err != nil {
			r.err = err
		}
	case WALEntrySeries:
		if err := r.decodeSeries(flag, b); err != nil {
			r.err = err
		}
	default:
		r.err = errors.Errorf("unknown WAL entry type %d", et)
	}
	return r.err == nil
}

func (r *WALReader) entry(cr io.Reader) (WALEntryType, byte, []byte, error) {
	r.crc32.Reset()
	tr := io.TeeReader(cr, r.crc32)

	b := make([]byte, 6)
	if _, err := tr.Read(b); err != nil {
		return 0, 0, nil, err
	}

	var (
		etype  = WALEntryType(b[0])
		flag   = b[1]
		length = int(binary.BigEndian.Uint32(b[2:]))
	)
	// Exit if we reached pre-allocated space.
	if etype == 0 {
		return 0, 0, nil, io.EOF
	}

	if length > len(r.buf) {
		r.buf = make([]byte, length)
	}
	buf := r.buf[:length]

	if _, err := tr.Read(buf); err != nil {
		return 0, 0, nil, err
	}
	_, err := cr.Read(b[:4])
	if err != nil {
		return 0, 0, nil, err
	}
	if exp, has := binary.BigEndian.Uint32(b[:4]), r.crc32.Sum32(); has != exp {
		return 0, 0, nil, errors.Errorf("unexpected CRC32 checksum %x, want %x", has, exp)
	}

	return etype, flag, buf, nil
}

func (r *WALReader) decodeSeries(flag byte, b []byte) error {
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

		r.labels = append(r.labels, lset)
	}
	return nil
}

func (r *WALReader) decodeSamples(flag byte, b []byte) error {
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

		r.samples = append(r.samples, smpl)
	}
	return nil
}
