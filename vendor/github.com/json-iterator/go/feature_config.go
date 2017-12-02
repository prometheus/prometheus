package jsoniter

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"unsafe"
)

// Config customize how the API should behave.
// The API is created from Config by Froze.
type Config struct {
	IndentionStep                 int
	MarshalFloatWith6Digits       bool
	EscapeHTML                    bool
	SortMapKeys                   bool
	UseNumber                     bool
	DisallowUnknownFields         bool
	TagKey                        string
	OnlyTaggedField               bool
	ValidateJsonRawMessage        bool
	ObjectFieldMustBeSimpleString bool
}

// API the public interface of this package.
// Primary Marshal and Unmarshal.
type API interface {
	IteratorPool
	StreamPool
	MarshalToString(v interface{}) (string, error)
	Marshal(v interface{}) ([]byte, error)
	MarshalIndent(v interface{}, prefix, indent string) ([]byte, error)
	UnmarshalFromString(str string, v interface{}) error
	Unmarshal(data []byte, v interface{}) error
	Get(data []byte, path ...interface{}) Any
	NewEncoder(writer io.Writer) *Encoder
	NewDecoder(reader io.Reader) *Decoder
	Valid(data []byte) bool
	RegisterExtension(extension Extension)
}

// ConfigDefault the default API
var ConfigDefault = Config{
	EscapeHTML: true,
}.Froze()

// ConfigCompatibleWithStandardLibrary tries to be 100% compatible with standard library behavior
var ConfigCompatibleWithStandardLibrary = Config{
	EscapeHTML:             true,
	SortMapKeys:            true,
	ValidateJsonRawMessage: true,
}.Froze()

// ConfigFastest marshals float with only 6 digits precision
var ConfigFastest = Config{
	EscapeHTML:                    false,
	MarshalFloatWith6Digits:       true, // will lose precession
	ObjectFieldMustBeSimpleString: true, // do not unescape object field
}.Froze()

// Froze forge API from config
func (cfg Config) Froze() API {
	api := &frozenConfig{
		sortMapKeys:                   cfg.SortMapKeys,
		indentionStep:                 cfg.IndentionStep,
		objectFieldMustBeSimpleString: cfg.ObjectFieldMustBeSimpleString,
		onlyTaggedField:               cfg.OnlyTaggedField,
		disallowUnknownFields:         cfg.DisallowUnknownFields,
		streamPool:                    make(chan *Stream, 16),
		iteratorPool:                  make(chan *Iterator, 16),
	}
	api.initCache()
	if cfg.MarshalFloatWith6Digits {
		api.marshalFloatWith6Digits()
	}
	if cfg.EscapeHTML {
		api.escapeHTML()
	}
	if cfg.UseNumber {
		api.useNumber()
	}
	if cfg.ValidateJsonRawMessage {
		api.validateJsonRawMessage()
	}
	api.configBeforeFrozen = cfg
	return api
}

func (cfg Config) frozeWithCacheReuse() *frozenConfig {
	api := getFrozenConfigFromCache(cfg)
	if api != nil {
		return api
	}
	api = cfg.Froze().(*frozenConfig)
	addFrozenConfigToCache(cfg, api)
	return api
}

func (cfg *frozenConfig) validateJsonRawMessage() {
	encoder := &funcEncoder{func(ptr unsafe.Pointer, stream *Stream) {
		rawMessage := *(*json.RawMessage)(ptr)
		iter := cfg.BorrowIterator([]byte(rawMessage))
		iter.Read()
		if iter.Error != nil {
			stream.WriteRaw("null")
		} else {
			cfg.ReturnIterator(iter)
			stream.WriteRaw(string(rawMessage))
		}
	}, func(ptr unsafe.Pointer) bool {
		return false
	}}
	cfg.addEncoderToCache(reflect.TypeOf((*json.RawMessage)(nil)).Elem(), encoder)
	cfg.addEncoderToCache(reflect.TypeOf((*RawMessage)(nil)).Elem(), encoder)
}

func (cfg *frozenConfig) useNumber() {
	cfg.addDecoderToCache(reflect.TypeOf((*interface{})(nil)).Elem(), &funcDecoder{func(ptr unsafe.Pointer, iter *Iterator) {
		if iter.WhatIsNext() == NumberValue {
			*((*interface{})(ptr)) = json.Number(iter.readNumberAsString())
		} else {
			*((*interface{})(ptr)) = iter.Read()
		}
	}})
}
func (cfg *frozenConfig) getTagKey() string {
	tagKey := cfg.configBeforeFrozen.TagKey
	if tagKey == "" {
		return "json"
	}
	return tagKey
}

func (cfg *frozenConfig) RegisterExtension(extension Extension) {
	cfg.extensions = append(cfg.extensions, extension)
}

type lossyFloat32Encoder struct {
}

func (encoder *lossyFloat32Encoder) Encode(ptr unsafe.Pointer, stream *Stream) {
	stream.WriteFloat32Lossy(*((*float32)(ptr)))
}

func (encoder *lossyFloat32Encoder) EncodeInterface(val interface{}, stream *Stream) {
	WriteToStream(val, stream, encoder)
}

func (encoder *lossyFloat32Encoder) IsEmpty(ptr unsafe.Pointer) bool {
	return *((*float32)(ptr)) == 0
}

type lossyFloat64Encoder struct {
}

func (encoder *lossyFloat64Encoder) Encode(ptr unsafe.Pointer, stream *Stream) {
	stream.WriteFloat64Lossy(*((*float64)(ptr)))
}

func (encoder *lossyFloat64Encoder) EncodeInterface(val interface{}, stream *Stream) {
	WriteToStream(val, stream, encoder)
}

func (encoder *lossyFloat64Encoder) IsEmpty(ptr unsafe.Pointer) bool {
	return *((*float64)(ptr)) == 0
}

// EnableLossyFloatMarshalling keeps 10**(-6) precision
// for float variables for better performance.
func (cfg *frozenConfig) marshalFloatWith6Digits() {
	// for better performance
	cfg.addEncoderToCache(reflect.TypeOf((*float32)(nil)).Elem(), &lossyFloat32Encoder{})
	cfg.addEncoderToCache(reflect.TypeOf((*float64)(nil)).Elem(), &lossyFloat64Encoder{})
}

type htmlEscapedStringEncoder struct {
}

func (encoder *htmlEscapedStringEncoder) Encode(ptr unsafe.Pointer, stream *Stream) {
	str := *((*string)(ptr))
	stream.WriteStringWithHTMLEscaped(str)
}

func (encoder *htmlEscapedStringEncoder) EncodeInterface(val interface{}, stream *Stream) {
	WriteToStream(val, stream, encoder)
}

func (encoder *htmlEscapedStringEncoder) IsEmpty(ptr unsafe.Pointer) bool {
	return *((*string)(ptr)) == ""
}

func (cfg *frozenConfig) escapeHTML() {
	cfg.addEncoderToCache(reflect.TypeOf((*string)(nil)).Elem(), &htmlEscapedStringEncoder{})
}

func (cfg *frozenConfig) cleanDecoders() {
	typeDecoders = map[string]ValDecoder{}
	fieldDecoders = map[string]ValDecoder{}
	*cfg = *(cfg.configBeforeFrozen.Froze().(*frozenConfig))
}

func (cfg *frozenConfig) cleanEncoders() {
	typeEncoders = map[string]ValEncoder{}
	fieldEncoders = map[string]ValEncoder{}
	*cfg = *(cfg.configBeforeFrozen.Froze().(*frozenConfig))
}

func (cfg *frozenConfig) MarshalToString(v interface{}) (string, error) {
	stream := cfg.BorrowStream(nil)
	defer cfg.ReturnStream(stream)
	stream.WriteVal(v)
	if stream.Error != nil {
		return "", stream.Error
	}
	return string(stream.Buffer()), nil
}

func (cfg *frozenConfig) Marshal(v interface{}) ([]byte, error) {
	stream := cfg.BorrowStream(nil)
	defer cfg.ReturnStream(stream)
	stream.WriteVal(v)
	if stream.Error != nil {
		return nil, stream.Error
	}
	result := stream.Buffer()
	copied := make([]byte, len(result))
	copy(copied, result)
	return copied, nil
}

func (cfg *frozenConfig) MarshalIndent(v interface{}, prefix, indent string) ([]byte, error) {
	if prefix != "" {
		panic("prefix is not supported")
	}
	for _, r := range indent {
		if r != ' ' {
			panic("indent can only be space")
		}
	}
	newCfg := cfg.configBeforeFrozen
	newCfg.IndentionStep = len(indent)
	return newCfg.frozeWithCacheReuse().Marshal(v)
}

func (cfg *frozenConfig) UnmarshalFromString(str string, v interface{}) error {
	data := []byte(str)
	data = data[:lastNotSpacePos(data)]
	iter := cfg.BorrowIterator(data)
	defer cfg.ReturnIterator(iter)
	iter.ReadVal(v)
	if iter.head == iter.tail {
		iter.loadMore()
	}
	if iter.Error == io.EOF {
		return nil
	}
	if iter.Error == nil {
		iter.ReportError("UnmarshalFromString", "there are bytes left after unmarshal")
	}
	return iter.Error
}

func (cfg *frozenConfig) Get(data []byte, path ...interface{}) Any {
	iter := cfg.BorrowIterator(data)
	defer cfg.ReturnIterator(iter)
	return locatePath(iter, path)
}

func (cfg *frozenConfig) Unmarshal(data []byte, v interface{}) error {
	data = data[:lastNotSpacePos(data)]
	iter := cfg.BorrowIterator(data)
	defer cfg.ReturnIterator(iter)
	typ := reflect.TypeOf(v)
	if typ.Kind() != reflect.Ptr {
		// return non-pointer error
		return errors.New("the second param must be ptr type")
	}
	iter.ReadVal(v)
	if iter.head == iter.tail {
		iter.loadMore()
	}
	if iter.Error == io.EOF {
		return nil
	}
	if iter.Error == nil {
		iter.ReportError("Unmarshal", "there are bytes left after unmarshal")
	}
	return iter.Error
}

func (cfg *frozenConfig) NewEncoder(writer io.Writer) *Encoder {
	stream := NewStream(cfg, writer, 512)
	return &Encoder{stream}
}

func (cfg *frozenConfig) NewDecoder(reader io.Reader) *Decoder {
	iter := Parse(cfg, reader, 512)
	return &Decoder{iter}
}

func (cfg *frozenConfig) Valid(data []byte) bool {
	iter := cfg.BorrowIterator(data)
	defer cfg.ReturnIterator(iter)
	iter.Skip()
	return iter.Error == nil
}
