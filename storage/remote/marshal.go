package remote

import (
	encoding_binary "encoding/binary"
	"fmt"
	"math"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
)

const (
	// These are the tag values of the original field encoded in protobuf
	// wire format. Refer to the specifications of protobuf encoding.

	// timeseries (repeated TimeSeries) has field value 1 in WriteRequest
	timeseriesTag = 0xa
	// labels (repeated Label) has field value 1 in TimeSeries
	labelTag = 0xa
	// value (string) has field value 2 in Label
	labelValueTag = 0x12
	// name (string) has field value 1 in Label
	labelNameTag = 0xa
	// samples (repeated Sample) has field value 2 in TimeSeries
	sampleTag = 0x12
	// timeStamp (int64) has field value 2 in Sample
	sampleTimestampTag = 0x10
	// value (double) has field value 1 in Sample
	sampleValueTag = 0x9
	// examplars (repeated Exemplar) has field value 3 in TimeSeries
	exemplarTag = 0x1a
	// timestamp (int64) has field value 3 in Exemplar
	exemplarTimestampTag = 0x18
	// value (double) has field value 2 in Exemplar
	exemplarValueTag = 0x11
	// histograms (repeated Histogram) has field value 4 in TimeSeries
	histogramsTag = 0x22
	// count_int (uint64) has field value 1 in Histogram
	histogramCountIntTag = 0x9
	// count_float (double) has field value 2 in Histogram
	histogramCountFloatTag = 0x11
	// sum (double) has field value 3 in Histogram
	histogramSumTag = 0x19
	// schema (sint32) has field value 4 in Histogram
	histogramSchemaTag = 0x20
	// zero_threshold (double) has field value 5 in Histogram
	histogramZeroThresholdTag = 0x29
	// zero_count_int (uint64) has field value 6 in Histogram
	histogramZeroCountIntTag = 0x30
	// zero_count_float (double) has field value 7 in Histogram
	histogramZeroCountFloatTag = 0x39
	// negative_spans (repeatedBucketSpan) has field value 8 in Histogram
	histogramNegativeSpansTag = 0x42
	// negative_deltas (repeated sint64) has field value 9 in Histogram
	histogramNegativeDeltasTag = 0x4a
	// negative_counts (repeated double) has field value 10 in Histogram
	histogramNegativeCountsTag = 0x52
	// positive_spans (repeated BucketSpan) has field value 11 in Histogram
	histogramPositiveSpansTag = 0x5a
	// positive_deltas (repeated sint64) has field value 12 in Histogram
	histogramPositiveDeltasTag = 0x62
	// positive_counts (repeated double) has field value 13 in Histogram
	histogramPositiveCountsTag = 0x6a
	// reset_hint (ResetHint) has field value 14 in Histogram
	histogramResetHintTag = 0x70
	// timestamp (int64) has field value 15 in Histogram
	histogramTimestampTag = 0x78
	// offset (sint32) has field value 1 in BucketSpan
	bucketSpanOffsetTag = 0x8
	// length (uint32) has field value 2 in BucketSpan
	bucketLengthTag = 0x10
)

// Turn a set of timeSeries into the expected protobuf encoding of WriteRequest for RemoteWrite.
// Similar to protoc-generated code, we expect a buffer of exactly the right size, and go
// backwards through the data. This makes it easier to insert the length before each element.
func marshalTimeSeriesSliceToBuffer(in []timeSeries, data []byte) (int, error) {
	i := len(data)
	for index := len(in) - 1; index >= 0; index-- {
		size, err := marshalTimeSeriesToSizedBuffer(&in[index], data[:i])
		if err != nil {
			return 0, fmt.Errorf("failed to marshal timeSeries[%v]: %w", index, err)
		}
		i -= size
		i = encodeVarint(data, i, uint64(size))
		i--
		data[i] = timeseriesTag
	}
	return len(data) - i, nil
}

func marshalTimeSeriesToSizedBuffer(t *timeSeries, data []byte) (int, error) {
	i := len(data)
	switch t.sType {
	case tSample:
		size, err := marshalSampleToSizedBuffer(t, data[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarint(data, i, uint64(size))
		i--
		data[i] = sampleTag
	case tExemplar:
		size, err := marshalExemplarToSizedBuffer(t, data[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarint(data, i, uint64(size))
		data[i] = exemplarTag
	case tHistogram:
		size, err := marshalHistogramToSizedBuffer(t.histogram, t.timestamp, data[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarint(data, i, uint64(size))
		data[i] = histogramsTag
	case tFloatHistogram:
		size, err := marshalFloatHistogramToSizedBuffer(t.floatHistogram, t.timestamp, data[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarint(data, i, uint64(size))
		data[i] = histogramsTag
	}
	size, err := marshalLabelsToSizedBuffer(t.seriesLabels, data[:i])
	if err != nil {
		return 0, fmt.Errorf("failed to marshal Labels: %w", err)
	}
	i -= size
	return len(data) - i, nil
}

func marshalSampleToSizedBuffer(t *timeSeries, data []byte) (int, error) {
	i := len(data)
	if t.timestamp != 0 {
		i = encodeVarint(data, i, uint64(t.timestamp))
		i--
		data[i] = sampleTimestampTag
	}
	if t.value != 0 {
		i -= 8
		encoding_binary.LittleEndian.PutUint64(data[i:], math.Float64bits(t.value))
		i--
		data[i] = sampleValueTag
	}
	return len(data) - i, nil
}

func marshalExemplarToSizedBuffer(t *timeSeries, data []byte) (int, error) {
	i := len(data)
	if t.timestamp != 0 {
		i = encodeVarint(data, i, uint64(t.timestamp))
		i--
		data[i] = exemplarTimestampTag
	}
	if t.value != 0 {
		i -= 8
		encoding_binary.LittleEndian.PutUint64(data[i:], math.Float64bits(t.value))
		i--
		data[i] = exemplarValueTag
	}
	size, err := marshalLabelsToSizedBuffer(t.exemplarLabels, data[:i])
	if err != nil {
		return 0, fmt.Errorf("failed to marshal Exemplar: %w", err)
	}
	i -= size
	return len(data) - i, nil
}

func marshalLabelsToSizedBuffer(lbls labels.Labels, data []byte) (int, error) {
	i := len(data)
	for index := len(lbls) - 1; index >= 0; index-- {
		size, err := marshalLabelToSizedBuffer(&lbls[index], data)
		if err != nil {
			return 0, fmt.Errorf("cannot marshalize lbls[%v]: %w", index, err)
		}
		i -= size
		i = encodeSize(data, i, size)
		i--
		data[i] = labelTag
	}
	return len(data) - i, nil
}

func marshalLabelToSizedBuffer(l *labels.Label, data []byte) (int, error) {
	i := len(data)
	if len(l.Value) > 0 {
		i -= len(l.Value)
		copy(data[i:], l.Value)
		i = encodeSize(data, i, len(l.Value))
		i--
		data[i] = labelValueTag
	}
	if len(l.Name) > 0 {
		i -= len(l.Name)
		copy(data[i:], l.Name)
		i = encodeSize(data, i, len(l.Name))
		i--
		data[i] = labelNameTag
	}
	return len(data) - i, nil
}

func marshalHistogramToSizedBuffer(h *histogram.Histogram, timestamp int64, data []byte) (int, error) {
	// count
	i := len(data)
	{
		i = encodeVarint(data, i, h.Count)
		i--
		data[i] = histogramCountIntTag
	}
	if h.Sum != 0 {
		i -= 8
		encoding_binary.LittleEndian.PutUint64(data[i:], math.Float64bits(h.Sum))
		i--
		data[i] = histogramSumTag
	}
	if h.Schema != 0 {
		i = encodeVarint(data, i, uint64(h.Schema))
		i--
		data[i] = histogramSchemaTag
	}
	if h.ZeroThreshold != 0 {
		i -= 8
		encoding_binary.LittleEndian.PutUint64(data[i:], math.Float64bits(h.ZeroThreshold))
		i--
		data[i] = histogramZeroThresholdTag
	}
	{
		i = encodeVarint(data, i, h.ZeroCount)
		i--
		data[i] = histogramZeroCountIntTag
	}
	for index := len(h.NegativeSpans) - 1; index >= 0; index-- {
		size, err := marshalSpanToSizedBuffer(&h.NegativeSpans[index], data[:i])
		if err != nil {
			return 0, fmt.Errorf("failed to marshal negativeSpans[%v]: %w", index, err)
		}
		i -= size
		i = encodeVarint(data, i, uint64(size))
		i--
		data[i] = histogramNegativeSpansTag
	}
	if len(h.NegativeBuckets) > 0 {
		base := i
		for index := len(h.NegativeBuckets); index >= 0; index-- {
			i = encodeVarint(data, i, encodeZigZag64(h.NegativeBuckets[index]))
		}
		i = encodeVarint(data, i, uint64(base-i))
		i--
		data[i] = histogramNegativeDeltasTag
	}
	for index := len(h.PositiveSpans) - 1; index >= 0; index-- {
		size, err := marshalSpanToSizedBuffer(&h.PositiveSpans[index], data[:i])
		if err != nil {
			return 0, fmt.Errorf("failed to marshal positiveSpan[%v]: %w", index, err)
		}
		i -= size
		i = encodeVarint(data, i, uint64(size))
		i--
		data[i] = histogramPositiveSpansTag
	}
	if len(h.PositiveBuckets) > 0 {
		base := i
		for index := len(h.PositiveBuckets); index >= 0; index-- {
			i = encodeVarint(data, i, encodeZigZag64(h.PositiveBuckets[index]))
		}
		i = encodeVarint(data, i, uint64(base-i))
		i--
		data[i] = histogramPositiveDeltasTag
	}
	if h.CounterResetHint != 0 {
		i = encodeVarint(data, i, uint64(h.CounterResetHint))
		i--
		data[i] = histogramResetHintTag
	}
	if timestamp != 0 {
		i = encodeVarint(data, i, uint64(timestamp))
		i--
		data[i] = histogramTimestampTag
	}
	return len(data) - i, nil
}

func marshalFloatHistogramToSizedBuffer(fh *histogram.FloatHistogram, timestamp int64, data []byte) (int, error) {
	// count
	i := len(data)
	{
		i -= 8
		encoding_binary.LittleEndian.PutUint64(data[i:], math.Float64bits(fh.Count))
		i--
		data[i] = histogramCountFloatTag
	}
	if fh.Sum != 0 {
		i -= 8
		encoding_binary.LittleEndian.PutUint64(data[i:], math.Float64bits(fh.Sum))
		i--
		data[i] = histogramSumTag
	}
	if fh.Schema != 0 {
		i = encodeVarint(data, i, uint64(fh.Schema))
		i--
		data[i] = histogramSchemaTag
	}
	if fh.ZeroThreshold != 0 {
		i -= 8
		encoding_binary.LittleEndian.PutUint64(data[i:], math.Float64bits(fh.ZeroThreshold))
		i--
		data[i] = histogramZeroThresholdTag
	}
	{
		i -= 8
		encoding_binary.LittleEndian.PutUint64(data[i:], math.Float64bits(fh.ZeroCount))
		i--
		data[i] = histogramZeroCountFloatTag
	}
	for index := len(fh.NegativeSpans) - 1; index >= 0; index-- {
		size, err := marshalSpanToSizedBuffer(&fh.NegativeSpans[index], data[:i])
		if err != nil {
			return 0, fmt.Errorf("failed to marshal negativeSpans[%v]: %w", index, err)
		}
		i -= size
		i = encodeVarint(data, i, uint64(size))
		i--
		data[i] = histogramNegativeSpansTag
	}
	if len(fh.NegativeBuckets) > 0 {
		for index := len(fh.NegativeBuckets); index >= 0; index-- {
			i -= 8
			encoding_binary.LittleEndian.PutUint64(data[i:], math.Float64bits(fh.NegativeBuckets[index]))
		}
		i = encodeVarint(data, i, uint64(8*len(fh.NegativeBuckets)))
		i--
		data[i] = histogramNegativeDeltasTag
	}
	for index := len(fh.PositiveSpans) - 1; index >= 0; index-- {
		size, err := marshalSpanToSizedBuffer(&fh.PositiveSpans[index], data[:i])
		if err != nil {
			return 0, fmt.Errorf("failed to marshal positiveSpan[%v]: %w", index, err)
		}
		i -= size
		i = encodeVarint(data, i, uint64(size))
		i--
		data[i] = histogramPositiveSpansTag
	}
	if len(fh.PositiveBuckets) > 0 {
		for index := len(fh.PositiveBuckets); index >= 0; index-- {
			i -= 8
			encoding_binary.LittleEndian.PutUint64(data[i:], math.Float64bits(fh.PositiveBuckets[index]))
		}
		i = encodeVarint(data, i, uint64(8*len(fh.PositiveBuckets)))
		i--
		data[i] = histogramPositiveDeltasTag
	}
	if fh.CounterResetHint != 0 {
		i = encodeVarint(data, i, uint64(fh.CounterResetHint))
		i--
		data[i] = histogramResetHintTag
	}
	if timestamp != 0 {
		i = encodeVarint(data, i, uint64(timestamp))
		i--
		data[i] = histogramTimestampTag
	}
	return len(data) - i, nil
}

func marshalSpanToSizedBuffer(s *histogram.Span, data []byte) (int, error) {
	i := len(data)
	if s.Offset == 0 && s.Length == 0 {
		data[i-1] = 0
		return 1, nil
	}
	if s.Offset != 0 {
		i = encodeVarint(data, i, uint64(s.Offset))
		i--
		data[i] = bucketSpanOffsetTag
	}
	if s.Length != 0 {
		i = encodeVarint(data, i, uint64(s.Length))
		i--
		data[i] = bucketLengthTag
	}
	return len(data) - i, nil
}

func sizeVarint(x uint64) (n int) {
	// Most common case first
	if x < 1<<7 {
		return 1
	}
	if x >= 1<<56 {
		return 9
	}
	if x >= 1<<28 {
		x >>= 28
		n = 4
	}
	if x >= 1<<14 {
		x >>= 14
		n += 2
	}
	if x >= 1<<7 {
		n++
	}
	return n + 1
}

// encodeVarint v into data that has last byte at data[offset - 1]
//
//	i.e. data[offset-size(v):offset].
func encodeVarint(data []byte, offset int, v uint64) int {
	offset -= sizeVarint(v)
	base := offset
	for v >= 1<<7 {
		data[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	data[offset] = uint8(v)
	return base
}

// Special code for the common case that a size is less than 128
func encodeSize(data []byte, offset, v int) int {
	if v < 1<<7 {
		offset--
		data[offset] = uint8(v)
		return offset
	}
	return encodeVarint(data, offset, uint64(v))
}

func timeSeriesSliceSize(data []timeSeries) (n int) {
	for _, e := range data {
		l := timeSeriesSize(&e)
		l += 1 + sizeVarint(uint64(l))     // for the timeSeries struct that wraps it
		n += 1 + l + sizeVarint(uint64(l)) // for the Timeseries field in WriteRequest
	}
	return n
}

func timeSeriesSize(t *timeSeries) (n int) {
	n = labelsSize(&t.seriesLabels)
	switch t.sType {
	case tSample:
		if t.value != 0 {
			n += 9
		}
		if t.timestamp != 0 {
			n += 1 + sizeVarint(uint64(t.timestamp))
		}
	case tExemplar:
		if t.value != 0 {
			n += 9
		}
		if t.timestamp != 0 {
			n += 1 + sizeVarint(uint64(t.timestamp))
		}
		n += labelsSize(&t.exemplarLabels)
	case tHistogram:
		n += histogramSize(t.timestamp, t.histogram)
	case tFloatHistogram:
		n += floatHistogramSize(t.timestamp, t.floatHistogram)
	}
	return n
}

func labelsSize(lbls *labels.Labels) (size int) {
	for _, e := range *lbls {
		labelSize := 0
		l := len(e.Name)
		if l > 0 {
			labelSize += 1 + l + sizeVarint(uint64(l))
		}
		l = len(e.Value)
		if l > 0 {
			labelSize += 1 + l + sizeVarint(uint64(l))
		}
		size += 1 + labelSize + sizeVarint(uint64(labelSize))
	}
	return size
}

func histogramSize(timestamp int64, h *histogram.Histogram) (size int) {
	// count
	size = 1 + sizeVarint(h.Count)
	// sum
	if h.Sum != 0 {
		size += 9
	}
	// schema
	if h.Schema != 0 {
		size += 1 + sizeVarint(uint64(h.Schema))
	}
	// zeroThreshold
	if h.ZeroThreshold != 0 {
		size += 9
	}
	// ZeroCount
	size += 1 + sizeVarint(h.ZeroCount)
	// NegativeSpans
	size += bucketSpansSize(h.NegativeSpans)
	// NegativeDeltas
	if len(h.NegativeBuckets) > 0 {
		negativeBucketsSize := 0
		for _, bucket := range h.NegativeBuckets {
			negativeBucketsSize += sizeVarint(encodeZigZag64(bucket))
		}
		size += 1 + sizeVarint(uint64(negativeBucketsSize)) + negativeBucketsSize
	}
	// PositiveSpans
	size += bucketSpansSize(h.PositiveSpans)
	// PositiveDeltas
	if len(h.PositiveBuckets) > 0 {
		positiveBucketsSize := 0
		for _, bucket := range h.PositiveBuckets {
			positiveBucketsSize += sizeVarint(encodeZigZag64(bucket))
		}
		size += 1 + sizeVarint(uint64(positiveBucketsSize)) + positiveBucketsSize
	}
	// ResetHint
	if h.CounterResetHint != 0 {
		size += 2
	}
	// Timestamp
	if timestamp != 0 {
		size += 1 + sizeVarint(uint64(timestamp))
	}
	return size
}

func floatHistogramSize(timestamp int64, fh *histogram.FloatHistogram) (size int) {
	// count
	size = 9
	// sum
	if fh.Sum != 0 {
		size += 9
	}
	// schema
	if fh.Schema != 0 {
		size += 1 + sizeVarint(uint64(fh.Schema))
	}
	// zeroThreshold
	if fh.ZeroThreshold != 0 {
		size += 9
	}
	// ZeroCount
	size += 9
	// NegativeSpans
	size += bucketSpansSize(fh.NegativeSpans)
	// NegativeDeltas, in packed format
	if len(fh.NegativeBuckets) > 0 {
		size += 1 + sizeVarint(uint64(8*len(fh.NegativeBuckets))) + 8*len(fh.NegativeBuckets)
	}
	// PositiveSpans
	size += bucketSpansSize(fh.PositiveSpans)
	// PositiveDeltas, in packed format
	if len(fh.PositiveBuckets) > 0 {
		size += 1 + sizeVarint(uint64(8*len(fh.PositiveBuckets))) + 8*len(fh.PositiveBuckets)
	}
	// ResetHint
	if fh.CounterResetHint != 0 {
		size += 2
	}
	// Timestamp
	if timestamp != 0 {
		size += 1 + sizeVarint(uint64(timestamp))
	}
	return size
}

func bucketSpansSize(spans []histogram.Span) (size int) {
	bucketSpanSize := func(span histogram.Span) (spanSize int) {
		if span.Offset != 0 {
			spanSize += sizeVarint(uint64(span.Offset))
		}
		if span.Length != 0 {
			spanSize += sizeVarint(uint64(span.Length))
		}
		return spanSize
	}
	for _, span := range spans {
		// using wire format of (tag length data)
		spanSize := bucketSpanSize(span)
		size += 1 + sizeVarint(uint64(spanSize)) + spanSize
	}
	return size
}

func encodeZigZag64(n int64) uint64 {
	return uint64((n << 1) ^ (n >> 63))
}
