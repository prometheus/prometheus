package jsoniter

import (
	"reflect"
	"unsafe"
)

func decoderOfOptional(cfg *frozenConfig, prefix string, typ reflect.Type) ValDecoder {
	elemType := typ.Elem()
	decoder := decoderOfType(cfg, prefix, elemType)
	return &OptionalDecoder{elemType, decoder}
}

func encoderOfOptional(cfg *frozenConfig, prefix string, typ reflect.Type) ValEncoder {
	elemType := typ.Elem()
	elemEncoder := encoderOfType(cfg, prefix, elemType)
	encoder := &OptionalEncoder{elemEncoder}
	if elemType.Kind() == reflect.Map {
		encoder = &OptionalEncoder{encoder}
	}
	return encoder
}

type OptionalDecoder struct {
	ValueType    reflect.Type
	ValueDecoder ValDecoder
}

func (decoder *OptionalDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if iter.ReadNil() {
		*((*unsafe.Pointer)(ptr)) = nil
	} else {
		if *((*unsafe.Pointer)(ptr)) == nil {
			//pointer to null, we have to allocate memory to hold the value
			value := reflect.New(decoder.ValueType)
			newPtr := extractInterface(value.Interface()).word
			decoder.ValueDecoder.Decode(newPtr, iter)
			*((*uintptr)(ptr)) = uintptr(newPtr)
		} else {
			//reuse existing instance
			decoder.ValueDecoder.Decode(*((*unsafe.Pointer)(ptr)), iter)
		}
	}
}

type dereferenceDecoder struct {
	// only to deference a pointer
	valueType    reflect.Type
	valueDecoder ValDecoder
}

func (decoder *dereferenceDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if *((*unsafe.Pointer)(ptr)) == nil {
		//pointer to null, we have to allocate memory to hold the value
		value := reflect.New(decoder.valueType)
		newPtr := extractInterface(value.Interface()).word
		decoder.valueDecoder.Decode(newPtr, iter)
		*((*uintptr)(ptr)) = uintptr(newPtr)
	} else {
		//reuse existing instance
		decoder.valueDecoder.Decode(*((*unsafe.Pointer)(ptr)), iter)
	}
}

type OptionalEncoder struct {
	ValueEncoder ValEncoder
}

func (encoder *OptionalEncoder) Encode(ptr unsafe.Pointer, stream *Stream) {
	if *((*unsafe.Pointer)(ptr)) == nil {
		stream.WriteNil()
	} else {
		encoder.ValueEncoder.Encode(*((*unsafe.Pointer)(ptr)), stream)
	}
}

func (encoder *OptionalEncoder) EncodeInterface(val interface{}, stream *Stream) {
	WriteToStream(val, stream, encoder)
}

func (encoder *OptionalEncoder) IsEmpty(ptr unsafe.Pointer) bool {
	return *((*unsafe.Pointer)(ptr)) == nil
}

type dereferenceEncoder struct {
	ValueEncoder ValEncoder
}

func (encoder *dereferenceEncoder) Encode(ptr unsafe.Pointer, stream *Stream) {
	if *((*unsafe.Pointer)(ptr)) == nil {
		stream.WriteNil()
	} else {
		encoder.ValueEncoder.Encode(*((*unsafe.Pointer)(ptr)), stream)
	}
}

func (encoder *dereferenceEncoder) EncodeInterface(val interface{}, stream *Stream) {
	WriteToStream(val, stream, encoder)
}

func (encoder *dereferenceEncoder) IsEmpty(ptr unsafe.Pointer) bool {
	return encoder.ValueEncoder.IsEmpty(*((*unsafe.Pointer)(ptr)))
}

type optionalMapEncoder struct {
	valueEncoder ValEncoder
}

func (encoder *optionalMapEncoder) Encode(ptr unsafe.Pointer, stream *Stream) {
	if *((*unsafe.Pointer)(ptr)) == nil {
		stream.WriteNil()
	} else {
		encoder.valueEncoder.Encode(*((*unsafe.Pointer)(ptr)), stream)
	}
}

func (encoder *optionalMapEncoder) EncodeInterface(val interface{}, stream *Stream) {
	WriteToStream(val, stream, encoder)
}

func (encoder *optionalMapEncoder) IsEmpty(ptr unsafe.Pointer) bool {
	p := *((*unsafe.Pointer)(ptr))
	return p == nil || encoder.valueEncoder.IsEmpty(p)
}
