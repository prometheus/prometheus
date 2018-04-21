package jsoniter

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"time"
	"unsafe"
)

// ValDecoder is an internal type registered to cache as needed.
// Don't confuse jsoniter.ValDecoder with json.Decoder.
// For json.Decoder's adapter, refer to jsoniter.AdapterDecoder(todo link).
//
// Reflection on type to create decoders, which is then cached
// Reflection on value is avoided as we can, as the reflect.Value itself will allocate, with following exceptions
// 1. create instance of new value, for example *int will need a int to be allocated
// 2. append to slice, if the existing cap is not enough, allocate will be done using Reflect.New
// 3. assignment to map, both key and value will be reflect.Value
// For a simple struct binding, it will be reflect.Value free and allocation free
type ValDecoder interface {
	Decode(ptr unsafe.Pointer, iter *Iterator)
}

// ValEncoder is an internal type registered to cache as needed.
// Don't confuse jsoniter.ValEncoder with json.Encoder.
// For json.Encoder's adapter, refer to jsoniter.AdapterEncoder(todo godoc link).
type ValEncoder interface {
	IsEmpty(ptr unsafe.Pointer) bool
	Encode(ptr unsafe.Pointer, stream *Stream)
	EncodeInterface(val interface{}, stream *Stream)
}

type checkIsEmpty interface {
	IsEmpty(ptr unsafe.Pointer) bool
}

// WriteToStream the default implementation for TypeEncoder method EncodeInterface
func WriteToStream(val interface{}, stream *Stream, encoder ValEncoder) {
	e := (*emptyInterface)(unsafe.Pointer(&val))
	if e.word == nil {
		stream.WriteNil()
		return
	}
	if reflect.TypeOf(val).Kind() == reflect.Ptr {
		encoder.Encode(unsafe.Pointer(&e.word), stream)
	} else {
		encoder.Encode(e.word, stream)
	}
}

var jsonNumberType reflect.Type
var jsoniterNumberType reflect.Type
var jsonRawMessageType reflect.Type
var jsoniterRawMessageType reflect.Type
var anyType reflect.Type
var marshalerType reflect.Type
var unmarshalerType reflect.Type
var textMarshalerType reflect.Type
var textUnmarshalerType reflect.Type

func init() {
	jsonNumberType = reflect.TypeOf((*json.Number)(nil)).Elem()
	jsoniterNumberType = reflect.TypeOf((*Number)(nil)).Elem()
	jsonRawMessageType = reflect.TypeOf((*json.RawMessage)(nil)).Elem()
	jsoniterRawMessageType = reflect.TypeOf((*RawMessage)(nil)).Elem()
	anyType = reflect.TypeOf((*Any)(nil)).Elem()
	marshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	unmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
}

// ReadVal copy the underlying JSON into go interface, same as json.Unmarshal
func (iter *Iterator) ReadVal(obj interface{}) {
	typ := reflect.TypeOf(obj)
	cacheKey := typ.Elem()
	decoder := decoderOfType(iter.cfg, "", cacheKey)
	e := (*emptyInterface)(unsafe.Pointer(&obj))
	if e.word == nil {
		iter.ReportError("ReadVal", "can not read into nil pointer")
		return
	}
	decoder.Decode(e.word, iter)
}

// WriteVal copy the go interface into underlying JSON, same as json.Marshal
func (stream *Stream) WriteVal(val interface{}) {
	if nil == val {
		stream.WriteNil()
		return
	}
	typ := reflect.TypeOf(val)
	cacheKey := typ
	encoder := encoderOfType(stream.cfg, "", cacheKey)
	encoder.EncodeInterface(val, stream)
}

func decoderOfType(cfg *frozenConfig, prefix string, typ reflect.Type) ValDecoder {
	cacheKey := typ
	decoder := cfg.getDecoderFromCache(cacheKey)
	if decoder != nil {
		return decoder
	}
	decoder = getTypeDecoderFromExtension(cfg, typ)
	if decoder != nil {
		cfg.addDecoderToCache(cacheKey, decoder)
		return decoder
	}
	decoder = &placeholderDecoder{cfg: cfg, cacheKey: cacheKey}
	cfg.addDecoderToCache(cacheKey, decoder)
	decoder = createDecoderOfType(cfg, prefix, typ)
	for _, extension := range extensions {
		decoder = extension.DecorateDecoder(typ, decoder)
	}
	for _, extension := range cfg.extensions {
		decoder = extension.DecorateDecoder(typ, decoder)
	}
	cfg.addDecoderToCache(cacheKey, decoder)
	return decoder
}

func createDecoderOfType(cfg *frozenConfig, prefix string, typ reflect.Type) ValDecoder {
	typeName := typ.String()
	if typ == jsonRawMessageType {
		return &jsonRawMessageCodec{}
	}
	if typ == jsoniterRawMessageType {
		return &jsoniterRawMessageCodec{}
	}
	if typ.AssignableTo(jsonNumberType) {
		return &jsonNumberCodec{}
	}
	if typ.AssignableTo(jsoniterNumberType) {
		return &jsoniterNumberCodec{}
	}
	if typ.Implements(unmarshalerType) {
		templateInterface := reflect.New(typ).Elem().Interface()
		var decoder ValDecoder = &unmarshalerDecoder{extractInterface(templateInterface)}
		if typ.Kind() == reflect.Ptr {
			decoder = &OptionalDecoder{typ.Elem(), decoder}
		}
		return decoder
	}
	if reflect.PtrTo(typ).Implements(unmarshalerType) {
		templateInterface := reflect.New(typ).Interface()
		var decoder ValDecoder = &unmarshalerDecoder{extractInterface(templateInterface)}
		return decoder
	}
	if typ.Implements(textUnmarshalerType) {
		templateInterface := reflect.New(typ).Elem().Interface()
		var decoder ValDecoder = &textUnmarshalerDecoder{extractInterface(templateInterface)}
		if typ.Kind() == reflect.Ptr {
			decoder = &OptionalDecoder{typ.Elem(), decoder}
		}
		return decoder
	}
	if reflect.PtrTo(typ).Implements(textUnmarshalerType) {
		templateInterface := reflect.New(typ).Interface()
		var decoder ValDecoder = &textUnmarshalerDecoder{extractInterface(templateInterface)}
		return decoder
	}
	if typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Uint8 {
		sliceDecoder := decoderOfSlice(cfg, prefix, typ)
		return &base64Codec{sliceDecoder: sliceDecoder}
	}
	if typ.Implements(anyType) {
		return &anyCodec{}
	}
	switch typ.Kind() {
	case reflect.String:
		if typeName != "string" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*string)(nil)).Elem())
		}
		return &stringCodec{}
	case reflect.Int:
		if typeName != "int" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*int)(nil)).Elem())
		}
		return &intCodec{}
	case reflect.Int8:
		if typeName != "int8" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*int8)(nil)).Elem())
		}
		return &int8Codec{}
	case reflect.Int16:
		if typeName != "int16" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*int16)(nil)).Elem())
		}
		return &int16Codec{}
	case reflect.Int32:
		if typeName != "int32" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*int32)(nil)).Elem())
		}
		return &int32Codec{}
	case reflect.Int64:
		if typeName != "int64" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*int64)(nil)).Elem())
		}
		return &int64Codec{}
	case reflect.Uint:
		if typeName != "uint" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*uint)(nil)).Elem())
		}
		return &uintCodec{}
	case reflect.Uint8:
		if typeName != "uint8" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*uint8)(nil)).Elem())
		}
		return &uint8Codec{}
	case reflect.Uint16:
		if typeName != "uint16" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*uint16)(nil)).Elem())
		}
		return &uint16Codec{}
	case reflect.Uint32:
		if typeName != "uint32" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*uint32)(nil)).Elem())
		}
		return &uint32Codec{}
	case reflect.Uintptr:
		if typeName != "uintptr" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*uintptr)(nil)).Elem())
		}
		return &uintptrCodec{}
	case reflect.Uint64:
		if typeName != "uint64" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*uint64)(nil)).Elem())
		}
		return &uint64Codec{}
	case reflect.Float32:
		if typeName != "float32" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*float32)(nil)).Elem())
		}
		return &float32Codec{}
	case reflect.Float64:
		if typeName != "float64" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*float64)(nil)).Elem())
		}
		return &float64Codec{}
	case reflect.Bool:
		if typeName != "bool" {
			return decoderOfType(cfg, prefix, reflect.TypeOf((*bool)(nil)).Elem())
		}
		return &boolCodec{}
	case reflect.Interface:
		if typ.NumMethod() == 0 {
			return &emptyInterfaceCodec{}
		}
		return &nonEmptyInterfaceCodec{}
	case reflect.Struct:
		return decoderOfStruct(cfg, prefix, typ)
	case reflect.Array:
		return decoderOfArray(cfg, prefix, typ)
	case reflect.Slice:
		return decoderOfSlice(cfg, prefix, typ)
	case reflect.Map:
		return decoderOfMap(cfg, prefix, typ)
	case reflect.Ptr:
		return decoderOfOptional(cfg, prefix, typ)
	default:
		return &lazyErrorDecoder{err: fmt.Errorf("%s%s is unsupported type", prefix, typ.String())}
	}
}

func encoderOfType(cfg *frozenConfig, prefix string, typ reflect.Type) ValEncoder {
	cacheKey := typ
	encoder := cfg.getEncoderFromCache(cacheKey)
	if encoder != nil {
		return encoder
	}
	encoder = getTypeEncoderFromExtension(cfg, typ)
	if encoder != nil {
		cfg.addEncoderToCache(cacheKey, encoder)
		return encoder
	}
	encoder = &placeholderEncoder{cfg: cfg, cacheKey: cacheKey}
	cfg.addEncoderToCache(cacheKey, encoder)
	encoder = createEncoderOfType(cfg, prefix, typ)
	for _, extension := range extensions {
		encoder = extension.DecorateEncoder(typ, encoder)
	}
	for _, extension := range cfg.extensions {
		encoder = extension.DecorateEncoder(typ, encoder)
	}
	cfg.addEncoderToCache(cacheKey, encoder)
	return encoder
}

func createEncoderOfType(cfg *frozenConfig, prefix string, typ reflect.Type) ValEncoder {
	if typ == jsonRawMessageType {
		return &jsonRawMessageCodec{}
	}
	if typ == jsoniterRawMessageType {
		return &jsoniterRawMessageCodec{}
	}
	if typ.AssignableTo(jsonNumberType) {
		return &jsonNumberCodec{}
	}
	if typ.AssignableTo(jsoniterNumberType) {
		return &jsoniterNumberCodec{}
	}
	if typ.Implements(marshalerType) {
		checkIsEmpty := createCheckIsEmpty(cfg, typ)
		templateInterface := reflect.New(typ).Elem().Interface()
		var encoder ValEncoder = &marshalerEncoder{
			templateInterface: extractInterface(templateInterface),
			checkIsEmpty:      checkIsEmpty,
		}
		if typ.Kind() == reflect.Ptr {
			encoder = &OptionalEncoder{encoder}
		}
		return encoder
	}
	if reflect.PtrTo(typ).Implements(marshalerType) {
		checkIsEmpty := createCheckIsEmpty(cfg, reflect.PtrTo(typ))
		templateInterface := reflect.New(typ).Interface()
		var encoder ValEncoder = &marshalerEncoder{
			templateInterface: extractInterface(templateInterface),
			checkIsEmpty:      checkIsEmpty,
		}
		return encoder
	}
	if typ.Implements(textMarshalerType) {
		checkIsEmpty := createCheckIsEmpty(cfg, typ)
		templateInterface := reflect.New(typ).Elem().Interface()
		var encoder ValEncoder = &textMarshalerEncoder{
			templateInterface: extractInterface(templateInterface),
			checkIsEmpty:      checkIsEmpty,
		}
		if typ.Kind() == reflect.Ptr {
			encoder = &OptionalEncoder{encoder}
		}
		return encoder
	}
	if typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Uint8 {
		return &base64Codec{}
	}
	if typ.Implements(anyType) {
		return &anyCodec{}
	}
	return createEncoderOfSimpleType(cfg, prefix, typ)
}

func createCheckIsEmpty(cfg *frozenConfig, typ reflect.Type) checkIsEmpty {
	kind := typ.Kind()
	switch kind {
	case reflect.String:
		return &stringCodec{}
	case reflect.Int:
		return &intCodec{}
	case reflect.Int8:
		return &int8Codec{}
	case reflect.Int16:
		return &int16Codec{}
	case reflect.Int32:
		return &int32Codec{}
	case reflect.Int64:
		return &int64Codec{}
	case reflect.Uint:
		return &uintCodec{}
	case reflect.Uint8:
		return &uint8Codec{}
	case reflect.Uint16:
		return &uint16Codec{}
	case reflect.Uint32:
		return &uint32Codec{}
	case reflect.Uintptr:
		return &uintptrCodec{}
	case reflect.Uint64:
		return &uint64Codec{}
	case reflect.Float32:
		return &float32Codec{}
	case reflect.Float64:
		return &float64Codec{}
	case reflect.Bool:
		return &boolCodec{}
	case reflect.Interface:
		if typ.NumMethod() == 0 {
			return &emptyInterfaceCodec{}
		}
		return &nonEmptyInterfaceCodec{}
	case reflect.Struct:
		return &structEncoder{typ: typ}
	case reflect.Array:
		return &arrayEncoder{}
	case reflect.Slice:
		return &sliceEncoder{}
	case reflect.Map:
		return encoderOfMap(cfg, "", typ)
	case reflect.Ptr:
		return &OptionalEncoder{}
	default:
		return &lazyErrorEncoder{err: fmt.Errorf("unsupported type: %v", typ)}
	}
}

func createEncoderOfSimpleType(cfg *frozenConfig, prefix string, typ reflect.Type) ValEncoder {
	typeName := typ.String()
	kind := typ.Kind()
	switch kind {
	case reflect.String:
		if typeName != "string" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*string)(nil)).Elem())
		}
		return &stringCodec{}
	case reflect.Int:
		if typeName != "int" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*int)(nil)).Elem())
		}
		return &intCodec{}
	case reflect.Int8:
		if typeName != "int8" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*int8)(nil)).Elem())
		}
		return &int8Codec{}
	case reflect.Int16:
		if typeName != "int16" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*int16)(nil)).Elem())
		}
		return &int16Codec{}
	case reflect.Int32:
		if typeName != "int32" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*int32)(nil)).Elem())
		}
		return &int32Codec{}
	case reflect.Int64:
		if typeName != "int64" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*int64)(nil)).Elem())
		}
		return &int64Codec{}
	case reflect.Uint:
		if typeName != "uint" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*uint)(nil)).Elem())
		}
		return &uintCodec{}
	case reflect.Uint8:
		if typeName != "uint8" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*uint8)(nil)).Elem())
		}
		return &uint8Codec{}
	case reflect.Uint16:
		if typeName != "uint16" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*uint16)(nil)).Elem())
		}
		return &uint16Codec{}
	case reflect.Uint32:
		if typeName != "uint32" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*uint32)(nil)).Elem())
		}
		return &uint32Codec{}
	case reflect.Uintptr:
		if typeName != "uintptr" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*uintptr)(nil)).Elem())
		}
		return &uintptrCodec{}
	case reflect.Uint64:
		if typeName != "uint64" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*uint64)(nil)).Elem())
		}
		return &uint64Codec{}
	case reflect.Float32:
		if typeName != "float32" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*float32)(nil)).Elem())
		}
		return &float32Codec{}
	case reflect.Float64:
		if typeName != "float64" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*float64)(nil)).Elem())
		}
		return &float64Codec{}
	case reflect.Bool:
		if typeName != "bool" {
			return encoderOfType(cfg, prefix, reflect.TypeOf((*bool)(nil)).Elem())
		}
		return &boolCodec{}
	case reflect.Interface:
		if typ.NumMethod() == 0 {
			return &emptyInterfaceCodec{}
		}
		return &nonEmptyInterfaceCodec{}
	case reflect.Struct:
		return encoderOfStruct(cfg, prefix, typ)
	case reflect.Array:
		return encoderOfArray(cfg, prefix, typ)
	case reflect.Slice:
		return encoderOfSlice(cfg, prefix, typ)
	case reflect.Map:
		return encoderOfMap(cfg, prefix, typ)
	case reflect.Ptr:
		return encoderOfOptional(cfg, prefix, typ)
	default:
		return &lazyErrorEncoder{err: fmt.Errorf("%s%s is unsupported type", prefix, typ.String())}
	}
}

type placeholderEncoder struct {
	cfg      *frozenConfig
	cacheKey reflect.Type
}

func (encoder *placeholderEncoder) Encode(ptr unsafe.Pointer, stream *Stream) {
	encoder.getRealEncoder().Encode(ptr, stream)
}

func (encoder *placeholderEncoder) EncodeInterface(val interface{}, stream *Stream) {
	encoder.getRealEncoder().EncodeInterface(val, stream)
}

func (encoder *placeholderEncoder) IsEmpty(ptr unsafe.Pointer) bool {
	return encoder.getRealEncoder().IsEmpty(ptr)
}

func (encoder *placeholderEncoder) getRealEncoder() ValEncoder {
	for i := 0; i < 500; i++ {
		realDecoder := encoder.cfg.getEncoderFromCache(encoder.cacheKey)
		_, isPlaceholder := realDecoder.(*placeholderEncoder)
		if isPlaceholder {
			time.Sleep(10 * time.Millisecond)
		} else {
			return realDecoder
		}
	}
	panic(fmt.Sprintf("real encoder not found for cache key: %v", encoder.cacheKey))
}

type placeholderDecoder struct {
	cfg      *frozenConfig
	cacheKey reflect.Type
}

func (decoder *placeholderDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	for i := 0; i < 500; i++ {
		realDecoder := decoder.cfg.getDecoderFromCache(decoder.cacheKey)
		_, isPlaceholder := realDecoder.(*placeholderDecoder)
		if isPlaceholder {
			time.Sleep(10 * time.Millisecond)
		} else {
			realDecoder.Decode(ptr, iter)
			return
		}
	}
	panic(fmt.Sprintf("real decoder not found for cache key: %v", decoder.cacheKey))
}

type lazyErrorDecoder struct {
	err error
}

func (decoder *lazyErrorDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if iter.WhatIsNext() != NilValue {
		if iter.Error == nil {
			iter.Error = decoder.err
		}
	} else {
		iter.Skip()
	}
}

type lazyErrorEncoder struct {
	err error
}

func (encoder *lazyErrorEncoder) Encode(ptr unsafe.Pointer, stream *Stream) {
	if ptr == nil {
		stream.WriteNil()
	} else if stream.Error == nil {
		stream.Error = encoder.err
	}
}

func (encoder *lazyErrorEncoder) EncodeInterface(val interface{}, stream *Stream) {
	if val == nil {
		stream.WriteNil()
	} else if stream.Error == nil {
		stream.Error = encoder.err
	}
}

func (encoder *lazyErrorEncoder) IsEmpty(ptr unsafe.Pointer) bool {
	return false
}

func extractInterface(val interface{}) emptyInterface {
	return *((*emptyInterface)(unsafe.Pointer(&val)))
}

// emptyInterface is the header for an interface{} value.
type emptyInterface struct {
	typ  unsafe.Pointer
	word unsafe.Pointer
}

// emptyInterface is the header for an interface with method (not interface{})
type nonEmptyInterface struct {
	// see ../runtime/iface.go:/Itab
	itab *struct {
		ityp   unsafe.Pointer // static interface type
		typ    unsafe.Pointer // dynamic concrete type
		link   unsafe.Pointer
		bad    int32
		unused int32
		fun    [100000]unsafe.Pointer // method table
	}
	word unsafe.Pointer
}
