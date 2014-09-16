// Copyright 2014 Prometheus Team
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

// Package codec provides types that implement encoding.BinaryMarshaler and
// encoding.BinaryUnmarshaler and functions that help to encode and decode
// primitives. The Prometheus storage backend uses them to persist objects to
// files and to save objects in LevelDB.
//
// The encodings used in this package are designed in a way that objects can be
// unmarshaled from a continuous byte stream, i.e. the information when to stop
// reading is determined by the format. No separate termination information is
// needed.
//
// Strings are encoded as the length of their bytes as a varint followed by
// their bytes.
//
// Slices are encoded as their length as a varint followed by their elements.
//
// Maps are encoded as the number of mappings as a varint, followed by the
// mappings, each of which consists of the key followed by the value.
package codec

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

// codable implements both, encoding.BinaryMarshaler and
// encoding.BinaryUnmarshaler, which is only needed internally and therefore not
// exported for now.
type codable interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

// A byteReader is an io.ByteReader that also implements the vanilla io.Reader
// interface.
type byteReader interface {
	io.Reader
	io.ByteReader
}

// bufPool is a pool for staging buffers. Using a pool allows concurrency-safe
// reuse of buffers
var bufPool sync.Pool

// getBuf returns a buffer from the pool. The length of the returned slice is l.
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

// putBuf returns a buffer to the pool.
func putBuf(buf []byte) {
	bufPool.Put(buf)
}

// EncodeVarint encodes an int64 as a varint and writes it to an io.Writer.
// This is a GC-friendly implementation that takes the required staging buffer
// from a buffer pool.
func EncodeVarint(w io.Writer, i int64) error {
	buf := getBuf(binary.MaxVarintLen64)
	defer putBuf(buf)

	bytesWritten := binary.PutVarint(buf, i)
	_, err := w.Write(buf[:bytesWritten])
	return err
}

// EncodeUint64 writes an uint64 to an io.Writer in big-endian byte-order.
// This is a GC-friendly implementation that takes the required staging buffer
// from a buffer pool.
func EncodeUint64(w io.Writer, u uint64) error {
	buf := getBuf(8)
	defer putBuf(buf)

	binary.BigEndian.PutUint64(buf, u)
	_, err := w.Write(buf)
	return err
}

// DecodeUint64 reads an uint64 from an io.Reader in big-endian byte-order.
// This is a GC-friendly implementation that takes the required staging buffer
// from a buffer pool.
func DecodeUint64(r io.Reader) (uint64, error) {
	buf := getBuf(8)
	defer putBuf(buf)

	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(buf), nil
}

// encodeString writes the varint encoded length followed by the bytes of s to
// b.
func encodeString(b *bytes.Buffer, s string) error {
	if err := EncodeVarint(b, int64(len(s))); err != nil {
		return err
	}
	if _, err := b.WriteString(s); err != nil {
		return err
	}
	return nil
}

// decodeString decodes a string encoded by encodeString.
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

// A CodableMetric is a clientmodel.Metric that implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler.
type CodableMetric clientmodel.Metric

// MarshalBinary implements encoding.BinaryMarshaler.
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

// UnmarshalBinary implements encoding.BinaryUnmarshaler. It can be used with the
// zero value of CodableMetric.
func (m *CodableMetric) UnmarshalBinary(buf []byte) error {
	return m.UnmarshalFromReader(bytes.NewReader(buf))
}

// UnmarshalFromReader unmarshals a CodableMetric from a reader that implements
// both, io.Reader and io.ByteReader. It can be used with the zero value of
// CodableMetric.
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

// A CodableFingerprint is a clientmodel.Fingerprint that implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler. The implementation
// depends on clientmodel.Fingerprint to be convertible to uint64. It encodes
// the fingerprint as a big-endian uint64.
type CodableFingerprint clientmodel.Fingerprint

// MarshalBinary implements encoding.BinaryMarshaler.
func (fp CodableFingerprint) MarshalBinary() ([]byte, error) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(fp))
	return b, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (fp *CodableFingerprint) UnmarshalBinary(buf []byte) error {
	*fp = CodableFingerprint(binary.BigEndian.Uint64(buf))
	return nil
}

// CodableFingerprints is a clientmodel.Fingerprints that implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler.
type CodableFingerprints clientmodel.Fingerprints

// MarshalBinary implements encoding.BinaryMarshaler.
func (fps CodableFingerprints) MarshalBinary() ([]byte, error) {
	b := make([]byte, binary.MaxVarintLen64+len(fps)*8)
	lenBytes := binary.PutVarint(b, int64(len(fps)))

	for i, fp := range fps {
		binary.BigEndian.PutUint64(b[i*8+lenBytes:], uint64(fp))
	}
	return b[:len(fps)*8+lenBytes], nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (fps *CodableFingerprints) UnmarshalBinary(buf []byte) error {
	numFPs, offset := binary.Varint(buf)
	if offset <= 0 {
		return fmt.Errorf("could not decode length of CodableFingerprints, varint decoding returned %d", offset)
	}
	*fps = make(CodableFingerprints, numFPs)

	for i := range *fps {
		(*fps)[i] = clientmodel.Fingerprint(binary.BigEndian.Uint64(buf[offset+i*8:]))
	}
	return nil
}

// CodableLabelPair is a metric.LabelPair that implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler.
type CodableLabelPair metric.LabelPair

// MarshalBinary implements encoding.BinaryMarshaler.
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

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
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

// CodableLabelName is a clientmodel.LabelName that implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler.
type CodableLabelName clientmodel.LabelName

// MarshalBinary implements encoding.BinaryMarshaler.
func (l CodableLabelName) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := encodeString(buf, string(l)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (l *CodableLabelName) UnmarshalBinary(buf []byte) error {
	r := bytes.NewReader(buf)
	n, err := decodeString(r)
	if err != nil {
		return err
	}
	*l = CodableLabelName(n)
	return nil
}

// CodableLabelValues is a clientmodel.LabelValues that implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler.
type CodableLabelValues clientmodel.LabelValues

// MarshalBinary implements encoding.BinaryMarshaler.
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

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (vs *CodableLabelValues) UnmarshalBinary(buf []byte) error {
	r := bytes.NewReader(buf)
	numValues, err := binary.ReadVarint(r)
	if err != nil {
		return err
	}
	*vs = make(CodableLabelValues, numValues)

	for i := range *vs {
		v, err := decodeString(r)
		if err != nil {
			return err
		}
		(*vs)[i] = clientmodel.LabelValue(v)
	}
	return nil
}

// CodableTimeRange is used to define a time range and implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler.
type CodableTimeRange struct {
	First, Last clientmodel.Timestamp
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (tr CodableTimeRange) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := EncodeVarint(buf, int64(tr.First)); err != nil {
		return nil, err
	}
	if err := EncodeVarint(buf, int64(tr.Last)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
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
	tr.First = clientmodel.Timestamp(first)
	tr.Last = clientmodel.Timestamp(last)
	return nil
}
