package codec

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"io"
	"sync"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

type codable interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

type byteReader interface {
	io.Reader
	io.ByteReader
}

var bufPool sync.Pool

func getBuf(l int) []byte {
	x := bufPool.Get()
	if x == nil {
		return make([]byte, l)
	}
	buf := x.([]byte)
	if cap(buf) < l {
		return make([]byte, l)
	}
	return buf[:l]
}

func putBuf(buf []byte) {
	bufPool.Put(buf)
}

func EncodeVarint(w io.Writer, i int64) error {
	buf := getBuf(binary.MaxVarintLen64)
	defer putBuf(buf)

	bytesWritten := binary.PutVarint(buf, i)
	_, err := w.Write(buf[:bytesWritten])
	return err
}

func EncodeUint64(w io.Writer, u uint64) error {
	buf := getBuf(8)
	defer putBuf(buf)

	binary.BigEndian.PutUint64(buf, u)
	_, err := w.Write(buf)
	return err
}

func DecodeUint64(r io.Reader) (uint64, error) {
	buf := getBuf(8)
	defer putBuf(buf)

	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(buf), nil
}

func encodeString(b *bytes.Buffer, s string) error {
	if err := EncodeVarint(b, int64(len(s))); err != nil {
		return err
	}
	if _, err := b.WriteString(s); err != nil {
		return err
	}
	return nil
}

func decodeString(b byteReader) (string, error) {
	length, err := binary.ReadVarint(b)
	if err != nil {
		return "", err
	}

	buf := getBuf(int(length))
	defer putBuf(buf)

	if _, err := io.ReadFull(b, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

type CodableMetric clientmodel.Metric

func (m CodableMetric) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := EncodeVarint(buf, int64(len(m))); err != nil {
		return nil, err
	}
	for l, v := range m {
		if err := encodeString(buf, string(l)); err != nil {
			return nil, err
		}
		if err := encodeString(buf, string(v)); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (m *CodableMetric) UnmarshalBinary(buf []byte) error {
	return m.UnmarshalFromReader(bytes.NewReader(buf))
}

func (m *CodableMetric) UnmarshalFromReader(r byteReader) error {
	numLabelPairs, err := binary.ReadVarint(r)
	if err != nil {
		return err
	}
	*m = make(CodableMetric, numLabelPairs)

	for ; numLabelPairs > 0; numLabelPairs-- {
		ln, err := decodeString(r)
		if err != nil {
			return err
		}
		lv, err := decodeString(r)
		if err != nil {
			return err
		}
		(*m)[clientmodel.LabelName(ln)] = clientmodel.LabelValue(lv)
	}
	return nil
}

type CodableFingerprint clientmodel.Fingerprint

func (fp CodableFingerprint) MarshalBinary() ([]byte, error) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(fp))
	return b, nil
}

func (fp *CodableFingerprint) UnmarshalBinary(buf []byte) error {
	*fp = CodableFingerprint(binary.BigEndian.Uint64(buf))
	return nil
}

type CodableFingerprints clientmodel.Fingerprints

func (fps CodableFingerprints) MarshalBinary() ([]byte, error) {
	b := bytes.NewBuffer(make([]byte, 0, binary.MaxVarintLen64+len(fps)*8))
	if err := EncodeVarint(b, int64(len(fps))); err != nil {
		return nil, err
	}

	buf := getBuf(8)
	defer putBuf(buf)

	for _, fp := range fps {
		binary.BigEndian.PutUint64(buf, uint64(fp))
		if _, err := b.Write(buf[:8]); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

func (fps *CodableFingerprints) UnmarshalBinary(buf []byte) error {
	r := bytes.NewReader(buf)
	numFPs, err := binary.ReadVarint(r)
	if err != nil {
		return err
	}
	*fps = make(CodableFingerprints, numFPs)

	offset := len(buf) - r.Len()
	for i, _ := range *fps {
		(*fps)[i] = clientmodel.Fingerprint(binary.BigEndian.Uint64(buf[offset+i*8:]))
	}
	return nil
}

type CodableLabelPair metric.LabelPair

func (lp CodableLabelPair) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := encodeString(buf, string(lp.Name)); err != nil {
		return nil, err
	}
	if err := encodeString(buf, string(lp.Value)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (lp *CodableLabelPair) UnmarshalBinary(buf []byte) error {
	r := bytes.NewReader(buf)
	n, err := decodeString(r)
	if err != nil {
		return err
	}
	v, err := decodeString(r)
	if err != nil {
		return err
	}
	lp.Name = clientmodel.LabelName(n)
	lp.Value = clientmodel.LabelValue(v)
	return nil
}

type CodableLabelName clientmodel.LabelName

func (l CodableLabelName) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := encodeString(buf, string(l)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (l *CodableLabelName) UnmarshalBinary(buf []byte) error {
	r := bytes.NewReader(buf)
	n, err := decodeString(r)
	if err != nil {
		return err
	}
	*l = CodableLabelName(n)
	return nil
}

type CodableLabelValues clientmodel.LabelValues

func (vs CodableLabelValues) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := EncodeVarint(buf, int64(len(vs))); err != nil {
		return nil, err
	}
	for _, v := range vs {
		if err := encodeString(buf, string(v)); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (vs *CodableLabelValues) UnmarshalBinary(buf []byte) error {
	r := bytes.NewReader(buf)
	numValues, err := binary.ReadVarint(r)
	if err != nil {
		return err
	}
	*vs = make(CodableLabelValues, numValues)

	for i, _ := range *vs {
		v, err := decodeString(r)
		if err != nil {
			return err
		}
		(*vs)[i] = clientmodel.LabelValue(v)
	}
	return nil
}

type CodableTimeRange struct {
	first, last clientmodel.Timestamp
}

func (tr CodableTimeRange) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := EncodeVarint(buf, int64(tr.first)); err != nil {
		return nil, err
	}
	if err := EncodeVarint(buf, int64(tr.last)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (tr *CodableTimeRange) UnmarshalBinary(buf []byte) error {
	r := bytes.NewReader(buf)
	first, err := binary.ReadVarint(r)
	if err != nil {
		return err
	}
	last, err := binary.ReadVarint(r)
	if err != nil {
		return err
	}
	tr.first = clientmodel.Timestamp(first)
	tr.last = clientmodel.Timestamp(last)
	return nil
}
