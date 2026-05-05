// Copyright The Prometheus Authors
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

// Package writev2 stateset.go contains hand-written Go types for stateset
// data in the remote write v2 protocol. These types map to protobuf fields 7
// and 8 of TimeSeries (stateset_metadata and stateset_samples) as defined in
// types.proto. They are encoded using a minimal protobuf binary writer so that
// the bytes are fully compatible with any proto3 decoder that knows those field
// numbers, without requiring protoc code generation.
//
// Proto field allocations in TimeSeries (field 6 is reserved):
//
//	stateset_metadata StatesetMetadata = 7
//	repeated stateset_samples StatesetSample = 8
package writev2

// StatesetMetadata describes the state names for a stateset series.
// Protobuf field numbers match those in types.proto (field 7 of TimeSeries).
type StatesetMetadata struct {
	// LabelName is the label that carries the state value in the OM1 wire format.
	LabelName string
	// StateNames lists all known state names, sorted lexicographically.
	StateNames []string
}

// StatesetSample is a single stateset data point.
// Protobuf field numbers match those in types.proto (field 8 of TimeSeries).
type StatesetSample struct {
	// TimestampMs is the sample timestamp in milliseconds.
	TimestampMs int64
	// Values is a bitset: bit i is set when StateNames[i] is active.
	Values uint64
}

const (
	// timeSeriesFieldStatesetMetadata is the protobuf field number for
	// stateset_metadata in TimeSeries.
	timeSeriesFieldStatesetMetadata = 7
	// timeSeriesFieldStatesetSample is the protobuf field number for the
	// repeated stateset_samples in TimeSeries.
	timeSeriesFieldStatesetSample = 8

	// Proto wire types.
	wireVarint = 0
	wireBytes  = 2
)

// appendVarint appends a protobuf-encoded varint to b.
func appendVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

// appendTag appends a protobuf field tag (field number + wire type) to b.
func appendTag(b []byte, field uint32, wireType uint64) []byte {
	return appendVarint(b, uint64(field)<<3|wireType)
}

// appendBytes appends a protobuf length-delimited field to b.
func appendBytes(b []byte, field uint32, data []byte) []byte {
	b = appendTag(b, field, wireBytes)
	b = appendVarint(b, uint64(len(data)))
	return append(b, data...)
}

// marshalStatesetMetadata encodes meta into protobuf binary format.
// Proto layout:
//
//	string label_name = 1;
//	repeated string state_names = 2;
func marshalStatesetMetadata(meta StatesetMetadata) []byte {
	var b []byte
	if meta.LabelName != "" {
		b = appendBytes(b, 1, []byte(meta.LabelName))
	}
	for _, name := range meta.StateNames {
		b = appendBytes(b, 2, []byte(name))
	}
	return b
}

// marshalStatesetSample encodes s into protobuf binary format.
// Proto layout:
//
//	int64 timestamp_ms = 1;
//	uint64 values = 2;
func marshalStatesetSample(s StatesetSample) []byte {
	var b []byte
	if s.TimestampMs != 0 {
		b = appendTag(b, 1, wireVarint)
		b = appendVarint(b, uint64(s.TimestampMs))
	}
	if s.Values != 0 {
		b = appendTag(b, 2, wireVarint)
		b = appendVarint(b, s.Values)
	}
	return b
}

// AppendStatesetToTimeSeries encodes meta and samples into ts.XXX_unrecognized
// using protobuf field numbers 7 (stateset_metadata) and 8 (repeated
// stateset_samples). The bytes are fully proto3-compatible: any decoder that
// registers field 7/8 can re-parse them without loss.
func AppendStatesetToTimeSeries(ts *TimeSeries, meta StatesetMetadata, samples []StatesetSample) {
	ts.XXX_unrecognized = appendBytes(ts.XXX_unrecognized, timeSeriesFieldStatesetMetadata, marshalStatesetMetadata(meta))
	for _, s := range samples {
		ts.XXX_unrecognized = appendBytes(ts.XXX_unrecognized, timeSeriesFieldStatesetSample, marshalStatesetSample(s))
	}
}

// ExtractStatesetFromTimeSeries decodes stateset metadata and samples from
// ts.XXX_unrecognized. Returns ok=false when no stateset data is present.
func ExtractStatesetFromTimeSeries(ts *TimeSeries) (meta StatesetMetadata, samples []StatesetSample, ok bool) {
	data := ts.XXX_unrecognized
	for len(data) > 0 {
		tag, n := decodeVarint(data)
		if n <= 0 {
			break
		}
		data = data[n:]
		field := uint32(tag >> 3)
		wt := tag & 0x7
		if wt != wireBytes {
			// Skip non-bytes fields (shouldn't happen for our messages, but be safe).
			data = skipField(data, wt)
			continue
		}
		length, n2 := decodeVarint(data)
		if n2 <= 0 {
			break
		}
		data = data[n2:]
		if uint64(len(data)) < length {
			break
		}
		value := data[:length]
		data = data[length:]

		switch field {
		case timeSeriesFieldStatesetMetadata:
			meta = unmarshalStatesetMetadata(value)
			ok = true
		case timeSeriesFieldStatesetSample:
			samples = append(samples, unmarshalStatesetSample(value))
			ok = true
		}
	}
	return meta, samples, ok
}

// unmarshalStatesetMetadata decodes a StatesetMetadata from its proto bytes.
func unmarshalStatesetMetadata(b []byte) StatesetMetadata {
	var meta StatesetMetadata
	for len(b) > 0 {
		tag, n := decodeVarint(b)
		if n <= 0 {
			break
		}
		b = b[n:]
		field := uint32(tag >> 3)
		wt := tag & 0x7
		if wt != wireBytes {
			b = skipField(b, wt)
			continue
		}
		length, n2 := decodeVarint(b)
		if n2 <= 0 {
			break
		}
		b = b[n2:]
		if uint64(len(b)) < length {
			break
		}
		value := b[:length]
		b = b[length:]
		switch field {
		case 1:
			meta.LabelName = string(value)
		case 2:
			meta.StateNames = append(meta.StateNames, string(value))
		}
	}
	return meta
}

// unmarshalStatesetSample decodes a StatesetSample from its proto bytes.
func unmarshalStatesetSample(b []byte) StatesetSample {
	var s StatesetSample
	for len(b) > 0 {
		tag, n := decodeVarint(b)
		if n <= 0 {
			break
		}
		b = b[n:]
		field := uint32(tag >> 3)
		wt := tag & 0x7
		if wt != wireVarint {
			b = skipField(b, wt)
			continue
		}
		v, n2 := decodeVarint(b)
		if n2 <= 0 {
			break
		}
		b = b[n2:]
		switch field {
		case 1:
			s.TimestampMs = int64(v)
		case 2:
			s.Values = v
		}
	}
	return s
}

// decodeVarint reads a protobuf varint from b. Returns (value, bytesConsumed).
// bytesConsumed is ≤ 0 on error.
func decodeVarint(b []byte) (uint64, int) {
	var x uint64
	for i, byt := range b {
		if i >= 10 {
			return 0, -1
		}
		x |= uint64(byt&0x7f) << (uint(i) * 7)
		if byt < 0x80 {
			return x, i + 1
		}
	}
	return 0, 0
}

// skipField skips a protobuf field of the given wire type, returning the
// remaining bytes. Returns nil on error.
func skipField(b []byte, wt uint64) []byte {
	switch wt {
	case wireVarint: // varint
		for i, byt := range b {
			if i >= 10 {
				return nil
			}
			if byt < 0x80 {
				return b[i+1:]
			}
		}
		return nil
	case 1: // 64-bit
		if len(b) < 8 {
			return nil
		}
		return b[8:]
	case wireBytes: // length-delimited
		length, n := decodeVarint(b)
		if n <= 0 {
			return nil
		}
		b = b[n:]
		if uint64(len(b)) < length {
			return nil
		}
		return b[length:]
	case 5: // 32-bit
		if len(b) < 4 {
			return nil
		}
		return b[4:]
	}
	return nil
}
