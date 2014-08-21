package index

import (
	"bytes"
	"encoding/binary"
	"io"
	"sync"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

type codable interface {
	encodable
	decodable
}

type encodable interface {
	encode() []byte
}

type decodable interface {
	decode([]byte)
}

// TODO: yeah, this ain't ideal. A lot of locking and possibly even contention.
var tmpBufMtx sync.Mutex
var tmpBuf = make([]byte, binary.MaxVarintLen64)

func setTmpBufLen(l int) {
	if cap(tmpBuf) >= l {
		tmpBuf = tmpBuf[:l]
	} else {
		tmpBuf = make([]byte, l)
	}
}

func encodeVarint(b *bytes.Buffer, i int) {
	tmpBufMtx.Lock()
	defer tmpBufMtx.Unlock()

	bytesWritten := binary.PutVarint(tmpBuf, int64(i))
	if _, err := b.Write(tmpBuf[:bytesWritten]); err != nil {
		panic(err)
	}
}

func encodeString(b *bytes.Buffer, s string) {
	encodeVarint(b, len(s))
	if _, err := b.WriteString(s); err != nil {
		panic(err)
	}
}

func decodeString(b *bytes.Reader) string {
	length, err := binary.ReadVarint(b)
	if err != nil {
		panic(err)
	}

	tmpBufMtx.Lock()
	defer tmpBufMtx.Unlock()

	setTmpBufLen(int(length))
	if _, err := io.ReadFull(b, tmpBuf); err != nil {
		panic(err)
	}
	return string(tmpBuf)
}

type codableMetric clientmodel.Metric

func (m codableMetric) encode() []byte {
	buf := &bytes.Buffer{}
	encodeVarint(buf, len(m))
	for l, v := range m {
		encodeString(buf, string(l))
		encodeString(buf, string(v))
	}
	return buf.Bytes()
}

func (m codableMetric) decode(buf []byte) {
	r := bytes.NewReader(buf)
	numLabelPairs, err := binary.ReadVarint(r)
	if err != nil {
		panic(err)
	}
	for ; numLabelPairs > 0; numLabelPairs-- {
		ln := decodeString(r)
		lv := decodeString(r)
		m[clientmodel.LabelName(ln)] = clientmodel.LabelValue(lv)
	}
}

type codableFingerprint clientmodel.Fingerprint

func (fp codableFingerprint) encode() []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(fp))
	return b
}

func (fp *codableFingerprint) decode(buf []byte) {
	*fp = codableFingerprint(binary.BigEndian.Uint64(buf))
}

type codableFingerprints clientmodel.Fingerprints

func (fps codableFingerprints) encode() []byte {
	buf := bytes.NewBuffer(make([]byte, 0, binary.MaxVarintLen64+len(fps)*8))
	encodeVarint(buf, len(fps))

	tmpBufMtx.Lock()
	defer tmpBufMtx.Unlock()

	setTmpBufLen(8)
	for _, fp := range fps {
		binary.BigEndian.PutUint64(tmpBuf, uint64(fp))
		if _, err := buf.Write(tmpBuf[:8]); err != nil {
			panic(err)
		}
	}
	return buf.Bytes()
}

func (fps *codableFingerprints) decode(buf []byte) {
	r := bytes.NewReader(buf)
	numFPs, err := binary.ReadVarint(r)
	if err != nil {
		panic(err)
	}
	*fps = make(codableFingerprints, numFPs)

	offset := len(buf) - r.Len()
	for i, _ := range *fps {
		(*fps)[i] = clientmodel.Fingerprint(binary.BigEndian.Uint64(buf[offset+i*8:]))
	}
}

type codableLabelPair metric.LabelPair

func (lp codableLabelPair) encode() []byte {
	buf := &bytes.Buffer{}
	encodeString(buf, string(lp.Name))
	encodeString(buf, string(lp.Value))
	return buf.Bytes()
}

func (lp *codableLabelPair) decode(buf []byte) {
	r := bytes.NewReader(buf)
	lp.Name = clientmodel.LabelName(decodeString(r))
	lp.Value = clientmodel.LabelValue(decodeString(r))
}

type codableLabelName clientmodel.LabelName

func (l codableLabelName) encode() []byte {
	buf := &bytes.Buffer{}
	encodeString(buf, string(l))
	return buf.Bytes()
}

func (l *codableLabelName) decode(buf []byte) {
	r := bytes.NewReader(buf)
	*l = codableLabelName(decodeString(r))
}

type codableLabelValues clientmodel.LabelValues

func (vs codableLabelValues) encode() []byte {
	buf := &bytes.Buffer{}
	encodeVarint(buf, len(vs))
	for _, v := range vs {
		encodeString(buf, string(v))
	}
	return buf.Bytes()
}

func (vs *codableLabelValues) decode(buf []byte) {
	r := bytes.NewReader(buf)
	numValues, err := binary.ReadVarint(r)
	if err != nil {
		panic(err)
	}
	*vs = make(codableLabelValues, numValues)

	for i, _ := range *vs {
		(*vs)[i] = clientmodel.LabelValue(decodeString(r))
	}
}

type codableMembership struct{}

func (m codableMembership) encode() []byte {
	return []byte{}
}

func (m codableMembership) decode(buf []byte) {}
