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

// Package codable provides types that implement encoding.BinaryMarshaler and
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
package codable

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/prometheus/common/model"
)

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
// It returns the number of bytes written.
// This is a GC-friendly implementation that takes the required staging buffer
// from a buffer pool.
func EncodeVarint(w io.Writer, i int64) (int, error) {
	buf := getBuf(binary.MaxVarintLen64)
	defer putBuf(buf)

	bytesWritten := binary.PutVarint(buf, i)
	_, err := w.Write(buf[:bytesWritten])
	return bytesWritten, err
}

// EncodeUvarint encodes an uint64 as a varint and writes it to an io.Writer.
// It returns the number of bytes written.
// This is a GC-friendly implementation that takes the required staging buffer
// from a buffer pool.
func EncodeUvarint(w io.Writer, i uint64) (int, error) {
	buf := getBuf(binary.MaxVarintLen64)
	defer putBuf(buf)

	bytesWritten := binary.PutUvarint(buf, i)
	_, err := w.Write(buf[:bytesWritten])
	return bytesWritten, err
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
	// Note that this should have used EncodeUvarint but a glitch happened
	// while designing the checkpoint format.
	if _, err := EncodeVarint(b, int64(len(s))); err != nil {
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
	if length < 0 {
		err = fmt.Errorf("found negative string length during decoding: %d", length)
	}
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

// A Metric is a model.Metric that implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler.
type Metric model.Metric

// MarshalBinary implements encoding.BinaryMarshaler.
func (m Metric) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	// Note that this should have used EncodeUvarint but a glitch happened
	// while designing the checkpoint format.
	if _, err := EncodeVarint(buf, int64(len(m))); err != nil {
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
// zero value of Metric.
func (m *Metric) UnmarshalBinary(buf []byte) error {
	return m.UnmarshalFromReader(bytes.NewReader(buf))
}

// UnmarshalFromReader unmarshals a Metric from a reader that implements
// both, io.Reader and io.ByteReader. It can be used with the zero value of
// Metric.
func (m *Metric) UnmarshalFromReader(r byteReader) error {
	numLabelPairs, err := binary.ReadVarint(r)
	if numLabelPairs < 0 {
		err = fmt.Errorf("found negative numLabelPairs during unmarshaling: %d", numLabelPairs)
	}
	if err != nil {
		return err
	}
	*m = make(Metric, numLabelPairs)

	for ; numLabelPairs > 0; numLabelPairs-- {
		ln, err := decodeString(r)
		if err != nil {
			return err
		}
		lv, err := decodeString(r)
		if err != nil {
			return err
		}
		(*m)[model.LabelName(ln)] = model.LabelValue(lv)
	}
	return nil
}

// A Fingerprint is a model.Fingerprint that implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler. The implementation
// depends on model.Fingerprint to be convertible to uint64. It encodes
// the fingerprint as a big-endian uint64.
type Fingerprint model.Fingerprint

// MarshalBinary implements encoding.BinaryMarshaler.
func (fp Fingerprint) MarshalBinary() ([]byte, error) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(fp))
	return b, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (fp *Fingerprint) UnmarshalBinary(buf []byte) error {
	*fp = Fingerprint(binary.BigEndian.Uint64(buf))
	return nil
}

// FingerprintSet is a map[model.Fingerprint]struct{} that
// implements encoding.BinaryMarshaler and encoding.BinaryUnmarshaler. Its
// binary form is identical to that of Fingerprints.
type FingerprintSet map[model.Fingerprint]struct{}

// MarshalBinary implements encoding.BinaryMarshaler.
func (fps FingerprintSet) MarshalBinary() ([]byte, error) {
	b := make([]byte, binary.MaxVarintLen64+len(fps)*8)
	lenBytes := binary.PutVarint(b, int64(len(fps)))
	offset := lenBytes

	for fp := range fps {
		binary.BigEndian.PutUint64(b[offset:], uint64(fp))
		offset += 8
	}
	return b[:len(fps)*8+lenBytes], nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (fps *FingerprintSet) UnmarshalBinary(buf []byte) error {
	numFPs, offset := binary.Varint(buf)
	if offset <= 0 {
		return fmt.Errorf("could not decode length of Fingerprints, varint decoding returned %d", offset)
	}
	*fps = make(FingerprintSet, numFPs)

	for i := 0; i < int(numFPs); i++ {
		(*fps)[model.Fingerprint(binary.BigEndian.Uint64(buf[offset+i*8:]))] = struct{}{}
	}
	return nil
}

// Fingerprints is a model.Fingerprints that implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler. Its binary form is
// identical to that of FingerprintSet.
type Fingerprints model.Fingerprints

// MarshalBinary implements encoding.BinaryMarshaler.
func (fps Fingerprints) MarshalBinary() ([]byte, error) {
	b := make([]byte, binary.MaxVarintLen64+len(fps)*8)
	lenBytes := binary.PutVarint(b, int64(len(fps)))

	for i, fp := range fps {
		binary.BigEndian.PutUint64(b[i*8+lenBytes:], uint64(fp))
	}
	return b[:len(fps)*8+lenBytes], nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (fps *Fingerprints) UnmarshalBinary(buf []byte) error {
	numFPs, offset := binary.Varint(buf)
	if offset <= 0 {
		return fmt.Errorf("could not decode length of Fingerprints, varint decoding returned %d", offset)
	}
	*fps = make(Fingerprints, numFPs)

	for i := range *fps {
		(*fps)[i] = model.Fingerprint(binary.BigEndian.Uint64(buf[offset+i*8:]))
	}
	return nil
}

// LabelPair is a model.LabelPair that implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler.
type LabelPair model.LabelPair

// MarshalBinary implements encoding.BinaryMarshaler.
func (lp LabelPair) MarshalBinary() ([]byte, error) {
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
func (lp *LabelPair) UnmarshalBinary(buf []byte) error {
	r := bytes.NewReader(buf)
	n, err := decodeString(r)
	if err != nil {
		return err
	}
	v, err := decodeString(r)
	if err != nil {
		return err
	}
	lp.Name = model.LabelName(n)
	lp.Value = model.LabelValue(v)
	return nil
}

// LabelName is a model.LabelName that implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler.
type LabelName model.LabelName

// MarshalBinary implements encoding.BinaryMarshaler.
func (l LabelName) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := encodeString(buf, string(l)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (l *LabelName) UnmarshalBinary(buf []byte) error {
	r := bytes.NewReader(buf)
	n, err := decodeString(r)
	if err != nil {
		return err
	}
	*l = LabelName(n)
	return nil
}

// LabelValueSet is a map[model.LabelValue]struct{} that implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler. Its binary form is
// identical to that of LabelValues.
type LabelValueSet map[model.LabelValue]struct{}

// MarshalBinary implements encoding.BinaryMarshaler.
func (vs LabelValueSet) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	// Note that this should have used EncodeUvarint but a glitch happened
	// while designing the checkpoint format.
	if _, err := EncodeVarint(buf, int64(len(vs))); err != nil {
		return nil, err
	}
	for v := range vs {
		if err := encodeString(buf, string(v)); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (vs *LabelValueSet) UnmarshalBinary(buf []byte) error {
	r := bytes.NewReader(buf)
	numValues, err := binary.ReadVarint(r)
	if numValues < 0 {
		err = fmt.Errorf("found negative number of values during unmarshaling: %d", numValues)
	}
	if err != nil {
		return err
	}
	*vs = make(LabelValueSet, numValues)

	for i := int64(0); i < numValues; i++ {
		v, err := decodeString(r)
		if err != nil {
			return err
		}
		(*vs)[model.LabelValue(v)] = struct{}{}
	}
	return nil
}

// LabelValues is a model.LabelValues that implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler. Its binary form is
// identical to that of LabelValueSet.
type LabelValues model.LabelValues

// MarshalBinary implements encoding.BinaryMarshaler.
func (vs LabelValues) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	// Note that this should have used EncodeUvarint but a glitch happened
	// while designing the checkpoint format.
	if _, err := EncodeVarint(buf, int64(len(vs))); err != nil {
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
func (vs *LabelValues) UnmarshalBinary(buf []byte) error {
	r := bytes.NewReader(buf)
	numValues, err := binary.ReadVarint(r)
	if numValues < 0 {
		err = fmt.Errorf("found negative number of values during unmarshaling: %d", numValues)
	}
	if err != nil {
		return err
	}
	*vs = make(LabelValues, numValues)

	for i := range *vs {
		v, err := decodeString(r)
		if err != nil {
			return err
		}
		(*vs)[i] = model.LabelValue(v)
	}
	return nil
}

// TimeRange is used to define a time range and implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler.
type TimeRange struct {
	First, Last model.Time
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (tr TimeRange) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	if _, err := EncodeVarint(buf, int64(tr.First)); err != nil {
		return nil, err
	}
	if _, err := EncodeVarint(buf, int64(tr.Last)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (tr *TimeRange) UnmarshalBinary(buf []byte) error {
	r := bytes.NewReader(buf)
	first, err := binary.ReadVarint(r)
	if err != nil {
		return err
	}
	last, err := binary.ReadVarint(r)
	if err != nil {
		return err
	}
	tr.First = model.Time(first)
	tr.Last = model.Time(last)
	return nil
}
