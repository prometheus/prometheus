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
	"io"
	"io/ioutil"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
)

const (
	prefixTracerState  = "x-trace-"
	prefixBaggage      = "ot-baggage-"
	headerTraceID      = prefixTracerState + "traceid"
	headerParentSpanID = prefixTracerState + "parentspanid"
)

var (
	seededIDGen = rand.New(rand.NewSource(time.Now().UnixNano()))
	// The golang rand generators are *not* intrinsically thread-safe.
	seededIDLock sync.Mutex
)

func randomID() uint64 {
	seededIDLock.Lock()
	defer seededIDLock.Unlock()

	x := seededIDGen.Uint64()
	for x == 0 {
		x = seededIDGen.Uint64()
	}

	return x
}

type Collector interface {
	Collect(span Span) error
}

type Tracer struct {
	Collector Collector
}

func (t *Tracer) StartSpan(operationName string, ssos ...opentracing.StartSpanOption) opentracing.Span {
	var opts opentracing.StartSpanOptions
	for _, sso := range ssos {
		sso.Apply(&opts)
	}

	if opts.StartTime.IsZero() {
		opts.StartTime = time.Now()
	}

	var tags []KeyValue
	if opts.Tags != nil {
		for k, v := range opts.Tags {
			if tag, ok := KeyValueFrom(k, v); ok {
				tags = append(tags, tag)
			}
		}
	}

	var traceID, parentSpanID uint64
	if len(opts.References) == 0 {
		traceID = randomID()
	} else {
		parentContext := opts.References[0].ReferencedContext.(SpanContext)
		traceID = parentContext.TraceId
		parentSpanID = parentContext.SpanId
	}

	spanID := randomID()

	return &activeSpan{
		tracer: t,
		Span: Span{
			SpanContext: SpanContext{
				TraceId: traceID,
				SpanId:  spanID,
			},
			ParentSpanId:  parentSpanID,
			Start:         opts.StartTime,
			OperationName: operationName,
			Tags:          tags,
		},
	}
}

func (t *Tracer) Inject(sc opentracing.SpanContext, format interface{}, carrier interface{}) error {
	context := sc.(SpanContext)
	switch format {
	case opentracing.TextMap, opentracing.HTTPHeaders:
		textMapWriter, ok := carrier.(opentracing.TextMapWriter)
		if !ok {
			return opentracing.ErrInvalidCarrier
		}
		return t.injectText(context, textMapWriter)
	case opentracing.Binary:
		ioWriter, ok := carrier.(io.Writer)
		if !ok {
			return opentracing.ErrInvalidCarrier
		}
		return t.injectBinary(context, ioWriter)
	}
	return opentracing.ErrUnsupportedFormat
}

func (t *Tracer) injectText(context SpanContext, carrier opentracing.TextMapWriter) error {
	carrier.Set(headerTraceID, strconv.FormatUint(context.TraceId, 16))
	carrier.Set(headerParentSpanID, strconv.FormatUint(context.SpanId, 16))
	context.ForeachBaggageItem(func(k, v string) bool {
		carrier.Set(prefixBaggage+k, v)
		return true
	})
	return nil
}

func (t *Tracer) injectBinary(context SpanContext, carrier io.Writer) error {
	buf, err := context.Marshal()
	if err == nil {
		_, err = carrier.Write(buf)
	}
	return err
}

func (t *Tracer) Extract(format interface{}, carrier interface{}) (opentracing.SpanContext, error) {
	switch format {
	case opentracing.TextMap, opentracing.HTTPHeaders:
		textMapReader, ok := carrier.(opentracing.TextMapReader)
		if !ok {
			return SpanContext{}, opentracing.ErrInvalidCarrier
		}
		return t.extractText(textMapReader)
	case opentracing.Binary:
		ioReader, ok := carrier.(io.Reader)
		if !ok {
			return SpanContext{}, opentracing.ErrInvalidCarrier
		}
		return t.extractBinary(ioReader)
	}
	return SpanContext{}, opentracing.ErrUnsupportedFormat
}

func (t *Tracer) extractText(carrier opentracing.TextMapReader) (opentracing.SpanContext, error) {
	var (
		spanID, traceID           uint64
		foundSpanID, foundTraceID bool
		baggage                   []Baggage
	)

	if err := carrier.ForeachKey(func(key string, val string) error {
		var err error
		switch {
		case key == headerTraceID:
			foundTraceID = true
			traceID, err = strconv.ParseUint(val, 16, 64)
			if err != nil {
				return err
			}
		case key == headerParentSpanID:
			foundSpanID = false
			spanID, err = strconv.ParseUint(val, 16, 64)
			if err != nil {
				return err
			}
		case strings.HasPrefix(key, prefixBaggage):
			baggage = append(baggage, Baggage{
				Key:   strings.TrimPrefix(key, prefixBaggage),
				Value: val,
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if !foundSpanID || !foundTraceID {
		return nil, opentracing.ErrSpanContextNotFound
	}

	return SpanContext{
		SpanId:  spanID,
		TraceId: traceID,
		Baggage: baggage,
	}, nil
}

func (t *Tracer) extractBinary(carrier io.Reader) (opentracing.SpanContext, error) {
	var context SpanContext
	buf, err := ioutil.ReadAll(carrier)
	if err != nil {
		err = context.Unmarshal(buf)
	}
	return context, err
}
