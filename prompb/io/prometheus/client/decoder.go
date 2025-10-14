// Copyright 2025 The Prometheus Authors
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

package io_prometheus_client //nolint:revive

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
	"unsafe"

	proto "github.com/gogo/protobuf/proto"
	"github.com/prometheus/common/model"
)

type MetricStreamingDecoder struct {
	in    []byte
	inPos int

	// TODO(bwplotka): Switch to generator/plugin that won't have those fields accessible e.g. OpaqueAPI
	// We leverage the fact those two don't collide.
	*MetricFamily // Without Metric, guarded by overridden GetMetric method.
	*Metric       // Without Label, guarded by overridden GetLabel method.

	mfData      []byte
	metrics     []pos
	metricIndex int

	mData  []byte
	labels []pos
}

// NewMetricStreamingDecoder returns a Go iterator that unmarshals given protobuf bytes one
// metric family and metric at the time, allowing efficient streaming.
//
// Do not modify MetricStreamingDecoder between iterations as it's reused to save allocations.
// GetGauge, GetCounter, etc are also cached, which means GetGauge will work for counter
// if previously gauge was parsed. It's up to the caller to use Type to decide what
// method to use when checking the value.
//
// TODO(bwplotka): io.Reader approach is possible too, but textparse has access to whole scrape for now.
func NewMetricStreamingDecoder(data []byte) *MetricStreamingDecoder {
	return &MetricStreamingDecoder{
		in:           data,
		MetricFamily: &MetricFamily{},
		Metric:       &Metric{},
		metrics:      make([]pos, 0, 100),
	}
}

var errInvalidVarint = errors.New("clientpb: invalid varint encountered")

// NextMetricFamily decodes the next metric family from the input without metrics.
// Use NextMetric() to decode metrics. The MetricFamily fields Name, Help and Unit
// are only valid until NextMetricFamily is called again.
func (m *MetricStreamingDecoder) NextMetricFamily() error {
	b := m.in[m.inPos:]
	if len(b) == 0 {
		return io.EOF
	}
	messageLength, varIntLength := proto.DecodeVarint(b) // TODO(bwplotka): Get rid of gogo.
	if varIntLength == 0 || varIntLength > binary.MaxVarintLen32 {
		return errInvalidVarint
	}
	totalLength := varIntLength + int(messageLength)
	if totalLength > len(b) {
		return fmt.Errorf("clientpb: insufficient length of buffer, expected at least %d bytes, got %d bytes", totalLength, len(b))
	}
	m.resetMetricFamily()
	m.mfData = b[varIntLength:totalLength]

	m.inPos += totalLength
	return m.unmarshalWithoutMetrics(m, m.mfData)
}

// resetMetricFamily resets all the fields in m to equal the zero value, but re-using slice memory.
func (m *MetricStreamingDecoder) resetMetricFamily() {
	m.metrics = m.metrics[:0]
	m.metricIndex = 0
	m.MetricFamily.Reset()
}

func (m *MetricStreamingDecoder) NextMetric() error {
	if m.metricIndex >= len(m.metrics) {
		return io.EOF
	}

	m.resetMetric()
	m.mData = m.mfData[m.metrics[m.metricIndex].start:m.metrics[m.metricIndex].end]
	if err := m.unmarshalWithoutLabels(m, m.mData); err != nil {
		return err
	}
	m.metricIndex++
	return nil
}

// resetMetric resets all the fields in m to equal the zero value, but re-using slices memory.
func (m *MetricStreamingDecoder) resetMetric() {
	m.labels = m.labels[:0]
	m.TimestampMs = 0

	// TODO(bwplotka): Autogenerate reset functions.
	if m.Counter != nil {
		m.Counter.Value = 0
		m.Counter.CreatedTimestamp = nil
		m.Counter.Exemplar = nil
	}
	if m.Gauge != nil {
		m.Gauge.Value = 0
	}
	if m.Histogram != nil {
		m.Histogram.SampleCount = 0
		m.Histogram.SampleCountFloat = 0
		m.Histogram.SampleSum = 0
		m.Histogram.Bucket = m.Histogram.Bucket[:0]
		m.Histogram.CreatedTimestamp = nil
		m.Histogram.Schema = 0
		m.Histogram.ZeroThreshold = 0
		m.Histogram.ZeroCount = 0
		m.Histogram.ZeroCountFloat = 0
		m.Histogram.NegativeSpan = m.Histogram.NegativeSpan[:0]
		m.Histogram.NegativeDelta = m.Histogram.NegativeDelta[:0]
		m.Histogram.NegativeCount = m.Histogram.NegativeCount[:0]
		m.Histogram.PositiveSpan = m.Histogram.PositiveSpan[:0]
		m.Histogram.PositiveDelta = m.Histogram.PositiveDelta[:0]
		m.Histogram.PositiveCount = m.Histogram.PositiveCount[:0]
		m.Histogram.Exemplars = m.Histogram.Exemplars[:0]
	}
	if m.Summary != nil {
		m.Summary.SampleCount = 0
		m.Summary.SampleSum = 0
		m.Summary.Quantile = m.Summary.Quantile[:0]
		m.Summary.CreatedTimestamp = nil
	}
}

func (*MetricStreamingDecoder) GetMetric() {
	panic("don't use GetMetric, use Metric directly")
}

func (*MetricStreamingDecoder) GetLabel() {
	panic("don't use GetLabel, use Label instead")
}

// unsafeLabelAdder adds labels for a single metric.
// The "unsafe" word highlights that some strings must not be retained on a
// caller side. When used with labels.ScratchBuilder ensure it's used
// with SetUnsafeAdd set to true.
type unsafeLabelAdder interface {
	Add(name, value string)
}

// Label parses labels into unsafeLabelAdder. Metric name is missing
// given the protobuf metric model and has to be deduced from the metric family name.
//
// TODO: The Label method name intentionally hide MetricStreamingDecoder.Metric.Label
// field to avoid direct use (it's not parsed). In future generator will generate
// structs tailored for streaming decoding.
//
// Unsafe in this context means that bytes and strings are reused across iterations.
// They are live only until the next NextMetric() or NextMetricFamily() call.
func (m *MetricStreamingDecoder) Label(b unsafeLabelAdder) error {
	for _, l := range m.labels {
		if err := parseLabel(m.mData[l.start:l.end], b); err != nil {
			return err
		}
	}
	return nil
}

// parseLabel is essentially LabelPair.Unmarshal but directly adding into unsafeLabelAdder.
func parseLabel(dAtA []byte, b unsafeLabelAdder) error {
	var unsafeName, unsafeValue string
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowMetrics
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return errors.New("proto: LabelPair: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: LabelPair: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Name", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthMetrics
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthMetrics
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			unsafeName = yoloString(dAtA[iNdEx:postIndex])
			if !model.UTF8Validation.IsValidLabelName(unsafeName) {
				return fmt.Errorf("invalid label name: %s", unsafeName)
			}
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Value", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthMetrics
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthMetrics
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			unsafeValue = yoloString(dAtA[iNdEx:postIndex])
			if !utf8.ValidString(unsafeValue) {
				return fmt.Errorf("invalid label value: %s", unsafeValue)
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipMetrics(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthMetrics
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}
	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	b.Add(unsafeName, unsafeValue)
	return nil
}

func yoloString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

type pos struct {
	start, end int
}

func (m *Metric) unmarshalWithoutLabels(p *MetricStreamingDecoder, dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowMetrics
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return errors.New("proto: Metric: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Metric: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Label", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthMetrics
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthMetrics
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			p.labels = append(p.labels, pos{start: iNdEx, end: postIndex})
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Gauge", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthMetrics
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthMetrics
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Gauge == nil {
				m.Gauge = &Gauge{}
			}
			if err := m.Gauge.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Counter", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthMetrics
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthMetrics
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Counter == nil {
				m.Counter = &Counter{}
			}
			if err := m.Counter.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Summary", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthMetrics
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthMetrics
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Summary == nil {
				m.Summary = &Summary{}
			}
			if err := m.Summary.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Untyped", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthMetrics
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthMetrics
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Untyped == nil {
				m.Untyped = &Untyped{}
			}
			if err := m.Untyped.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 6:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field TimestampMs", wireType)
			}
			m.TimestampMs = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.TimestampMs |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 7:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Histogram", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthMetrics
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthMetrics
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Histogram == nil {
				m.Histogram = &Histogram{}
			}
			if err := m.Histogram.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipMetrics(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthMetrics
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			m.XXX_unrecognized = append(m.XXX_unrecognized, dAtA[iNdEx:iNdEx+skippy]...)
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func (m *MetricFamily) unmarshalWithoutMetrics(buf *MetricStreamingDecoder, dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowMetrics
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return errors.New("proto: MetricFamily: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: MetricFamily: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Name", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthMetrics
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthMetrics
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Name = yoloString(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Help", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthMetrics
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthMetrics
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Help = yoloString(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Type", wireType)
			}
			m.Type = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Type |= MetricType(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Metric", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthMetrics
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthMetrics
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			buf.metrics = append(buf.metrics, pos{start: iNdEx, end: postIndex})
			iNdEx = postIndex
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Unit", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetrics
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthMetrics
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthMetrics
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Unit = yoloString(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipMetrics(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthMetrics
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			m.XXX_unrecognized = append(m.XXX_unrecognized, dAtA[iNdEx:iNdEx+skippy]...)
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
