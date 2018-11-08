// Copyright 2015 The Prometheus Authors
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

package model

import (
	"sync"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

type activeSpan struct {
	tracer *Tracer
	sync.Mutex
	Span
}

// Finish implements opentracing.Span.
func (s *activeSpan) Finish() {
	s.FinishWithOptions(opentracing.FinishOptions{})
}

// FinishWithOptions implements opentracing.Span.
func (s *activeSpan) FinishWithOptions(opts opentracing.FinishOptions) {
	s.Lock()
	defer s.Unlock()
	if opts.FinishTime.IsZero() {
		s.End = time.Now()
	} else {
		s.End = opts.FinishTime
	}
	s.Logs = append(s.Logs, fromLogRecords(opts.LogRecords)...)
	s.tracer.Collector.Collect(s.Span)
}

// Context implements opentracing.Span.
func (s *activeSpan) Context() opentracing.SpanContext {
	s.Lock()
	defer s.Unlock()
	return s.SpanContext
}

// SetOperationName implements opentracing.Span.
func (s *activeSpan) SetOperationName(operationName string) opentracing.Span {
	s.Lock()
	defer s.Unlock()
	s.OperationName = operationName
	return s
}

// SetTag implements opentracing.Span
func (s *activeSpan) SetTag(key string, value interface{}) opentracing.Span {
	s.Lock()
	defer s.Unlock()
	if kv, ok := KeyValueFrom(key, value); ok {
		s.Tags = append(s.Tags, kv)
	}
	return s
}

func (s *Span) Tag(key string) KeyValue {
	for _, tag := range s.Tags {
		if tag.Key == key {
			return tag
		}
	}
	return KeyValue{}
}

// LogFields implements opentracing.Span
func (s *activeSpan) LogFields(fields ...log.Field) {
	s.Lock()
	defer s.Unlock()
	s.Logs = append(s.Logs, fromLogFields(time.Now(), fields))
}

// LogKV implements opentracing.Span
func (s *activeSpan) LogKV(alternatingKeyValues ...interface{}) {
	fields, err := log.InterleavedKVToFields(alternatingKeyValues)
	if err != nil {
		s.LogFields(log.Error(err), log.String("function", "LogKV"))
	}
	s.LogFields(fields...)
}

// SetBaggageItem implements opentracing.Span
func (s *activeSpan) SetBaggageItem(restrictedKey, value string) opentracing.Span {
	s.Lock()
	defer s.Unlock()
	s.SpanContext = s.SpanContext.withBaggageItem(restrictedKey, value)
	return s
}

// BaggageItem implements opentracing.Span
func (s *activeSpan) BaggageItem(restrictedKey string) string {
	s.Lock()
	defer s.Unlock()
	return s.SpanContext.baggageItem(restrictedKey)
}

// Tracer implements opentracing.Span
func (s *activeSpan) Tracer() opentracing.Tracer {
	return s.tracer
}

// Deprecated: use LogFields or LogKV
func (s *activeSpan) LogEvent(event string) {}

// Deprecated: use LogFields or LogKV
func (s *activeSpan) LogEventWithPayload(event string, payload interface{}) {}

// Deprecated: use LogFields or LogKV
func (s *activeSpan) Log(data opentracing.LogData) {}

func fromLogRecords(records []opentracing.LogRecord) []LogRecord {
	result := make([]LogRecord, 0, len(records))
	for _, record := range records {
		result = append(result, fromLogFields(record.Timestamp, record.Fields))
	}
	return result
}

func fromLogFields(timestamp time.Time, fields []log.Field) LogRecord {
	var kve keyValueEncoder
	result := make([]KeyValue, 0, len(fields))
	for i, field := range fields {
		kve.kv = &result[i]
		field.Marshal(kve)
	}
	return LogRecord{
		Timestamp: time.Now(),
		Fields:    result,
	}
}

type keyValueEncoder struct {
	kv *KeyValue
}

func (e keyValueEncoder) EmitString(key, value string) {
	e.kv.Key = key
	e.kv.Type = KeyValue_String
	e.kv.String_ = value
}

func (e keyValueEncoder) EmitBool(key string, value bool) {
	e.kv.Key = key
	e.kv.Type = KeyValue_Bool
	e.kv.Bool = value
}

func (e keyValueEncoder) EmitInt(key string, value int) {
	e.kv.Key = key
	e.kv.Type = KeyValue_Int64
	e.kv.Int64 = int64(value)
}

func (e keyValueEncoder) EmitInt32(key string, value int32) {
	e.kv.Key = key
	e.kv.Type = KeyValue_Int64
	e.kv.Int64 = int64(value)
}

func (e keyValueEncoder) EmitInt64(key string, value int64) {
	e.kv.Key = key
	e.kv.Type = KeyValue_Int64
	e.kv.Int64 = value
}

func (e keyValueEncoder) EmitUint32(key string, value uint32) {
	e.kv.Key = key
	e.kv.Type = KeyValue_Uint64
	e.kv.Uint64 = uint64(value)
}

func (e keyValueEncoder) EmitUint64(key string, value uint64) {
	e.kv.Key = key
	e.kv.Type = KeyValue_Uint64
	e.kv.Uint64 = value
}

func (e keyValueEncoder) EmitFloat32(key string, value float32) {
	e.kv.Key = key
	e.kv.Type = KeyValue_Float64
	e.kv.Float64 = float64(value)
}

func (e keyValueEncoder) EmitFloat64(key string, value float64) {
	e.kv.Key = key
	e.kv.Type = KeyValue_Float64
	e.kv.Float64 = value
}

func (e keyValueEncoder) EmitObject(key string, value interface{}) {
	panic("Not supported")
}

func (e keyValueEncoder) EmitLazyLogger(value log.LazyLogger) {
	panic("Not supported")
}

func KeyValueFrom(key string, value interface{}) (KeyValue, bool) {
	switch v := value.(type) {
	case string:
		return KeyValue{
			Key:     key,
			Type:    KeyValue_String,
			String_: v,
		}, true
	case bool:
		return KeyValue{
			Key:  key,
			Type: KeyValue_Bool,
			Bool: v,
		}, true
	case int:
		return KeyValue{
			Key:   key,
			Type:  KeyValue_Int64,
			Int64: int64(v),
		}, true
	case int32:
		return KeyValue{
			Key:   key,
			Type:  KeyValue_Int64,
			Int64: int64(v),
		}, true
	case int64:
		return KeyValue{
			Key:   key,
			Type:  KeyValue_Int64,
			Int64: v,
		}, true
	case uint32:
		return KeyValue{
			Key:    key,
			Type:   KeyValue_Uint64,
			Uint64: uint64(v),
		}, true
	case uint64:
		return KeyValue{
			Key:    key,
			Type:   KeyValue_Uint64,
			Uint64: v,
		}, true
	case float32:
		return KeyValue{
			Key:     key,
			Type:    KeyValue_Float64,
			Float64: float64(v),
		}, true
	case float64:
		return KeyValue{
			Key:     key,
			Type:    KeyValue_Float64,
			Float64: v,
		}, true
	default:
		return KeyValue{}, false
	}
}

func (kv *KeyValue) Value() interface{} {
	switch kv.Type {
	case KeyValue_String:
		return kv.String_
	case KeyValue_Bool:
		return kv.Bool
	case KeyValue_Int64:
		return kv.Int64
	case KeyValue_Uint64:
		return kv.Uint64
	case KeyValue_Float64:
		return kv.Float64
	default:
		return nil
	}
}

func (t *Trace) Start() time.Time {
	var start time.Time
	for _, span := range t.Spans {
		if start.IsZero() || span.Start.Before(start) {
			start = span.Start
		}
	}
	return start
}

func (t *Trace) OperationName() string {
	for _, span := range t.Spans {
		if span.ParentSpanId == 0 {
			return span.OperationName
		}
	}
	return ""
}
