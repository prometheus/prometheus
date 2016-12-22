package tsdb

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/fabxc/tsdb/labels"
)

// WALEntryType indicates what data a WAL entry contains.
type WALEntryType byte

// The valid WAL entry types.
const (
	WALEntrySymbols = 1
	WALEntrySeries  = 2
	WALEntrySamples = 3
)

// WAL is a write ahead log for series data. It can only be written to.
// Use WALReader to read back from a write ahead log.
type WAL struct {
	f   *fileutil.LockedFile
	enc *walEncoder

	symbols map[string]uint32
}

// CreateWAL creates a new write ahead log in the given directory.
func CreateWAL(dir string) (*WAL, error) {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}

	p := filepath.Join(dir, "wal")

	f, err := fileutil.LockFile(p, os.O_WRONLY|os.O_CREATE, fileutil.PrivateFileMode)
	if err != nil {
		return nil, err
	}
	if _, err = f.Seek(0, os.SEEK_END); err != nil {
		return nil, err
	}

	w := &WAL{
		f:       f,
		enc:     newWALEncoder(f),
		symbols: map[string]uint32{},
	}
	return w, nil
}

// Log writes a batch of new series labels and samples to the log.
func (w *WAL) Log(series []labels.Labels, samples []hashedSample) error {
	if err := w.enc.encodeSeries(series); err != nil {
		return err
	}
	if err := w.enc.encodeSamples(samples); err != nil {
		return err
	}
	return nil
}

func (w *WAL) sync() error {
	return fileutil.Fdatasync(w.f.File)
}

// Close sync all data and closes the underlying resources.
func (w *WAL) Close() error {
	if err := w.sync(); err != nil {
		return err
	}
	return w.f.Close()
}

// OpenWAL does things.
func OpenWAL(dir string) (*WAL, error) {
	return nil, nil
}

type walEncoder struct {
	w io.Writer

	buf []byte
}

func newWALEncoder(w io.Writer) *walEncoder {
	return &walEncoder{
		w:   w,
		buf: make([]byte, 1024*1024),
	}
}

func (e *walEncoder) entry(et WALEntryType, flag byte, n int) error {
	h := crc32.NewIEEE()
	w := io.MultiWriter(h, e.w)

	b := make([]byte, 6)
	b[0] = byte(et)
	b[1] = flag

	binary.BigEndian.PutUint32(b[2:], uint32(len(e.buf)))

	if _, err := w.Write(b); err != nil {
		return err
	}
	if _, err := w.Write(e.buf[:n]); err != nil {
		return err
	}
	if _, err := e.w.Write(h.Sum(nil)); err != nil {
		return err
	}

	return nil
}

const (
	walSeriesSimple  = 1
	walSamplesSimple = 1
)

func (e *walEncoder) encodeSeries(series []labels.Labels) error {
	if len(series) == 0 {
		return nil
	}
	var (
		b   = make([]byte, binary.MaxVarintLen32)
		buf = e.buf[:0]
	)

	for _, lset := range series {
		n := binary.PutUvarint(b, uint64(len(lset)))
		buf = append(buf, b[:n]...)

		for _, l := range lset {
			n = binary.PutUvarint(b, uint64(len(l.Name)))
			buf = append(buf, b[:n]...)

			n = binary.PutUvarint(b, uint64(len(l.Value)))
			buf = append(buf, b[:n]...)
		}
	}

	return e.entry(WALEntrySeries, walSeriesSimple, len(buf))
}

func (e *walEncoder) encodeSamples(samples []hashedSample) error {
	if len(samples) == 0 {
		return nil
	}
	var (
		b   = make([]byte, binary.MaxVarintLen64)
		buf = e.buf[:0]
	)

	// Store base timestamp and base reference number of first sample.
	// All samples encode their timestamp and ref as delta to those.
	//
	// TODO(fabxc): optimize for all samples having the same timestamp.
	first := samples[0]

	binary.BigEndian.PutUint32(b, first.ref)
	buf = append(buf, b[:4]...)
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

	return e.entry(WALEntrySamples, walSamplesSimple, len(buf))
}

type walDecoder struct {
	r io.Reader

	handleSeries func(labels.Labels)
	handleSample func(hashedSample)
}
