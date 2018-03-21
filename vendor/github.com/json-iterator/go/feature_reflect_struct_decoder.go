package jsoniter

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"unsafe"
)

func createStructDecoder(cfg *frozenConfig, typ reflect.Type, fields map[string]*structFieldDecoder) ValDecoder {
	if cfg.disallowUnknownFields {
		return &generalStructDecoder{typ: typ, fields: fields, disallowUnknownFields: true}
	}
	knownHash := map[int64]struct{}{
		0: {},
	}
	switch len(fields) {
	case 0:
		return &skipObjectDecoder{typ}
	case 1:
		for fieldName, fieldDecoder := range fields {
			fieldHash := calcHash(fieldName)
			_, known := knownHash[fieldHash]
			if known {
				return &generalStructDecoder{typ, fields, false}
			}
			knownHash[fieldHash] = struct{}{}
			return &oneFieldStructDecoder{typ, fieldHash, fieldDecoder}
		}
	case 2:
		var fieldHash1 int64
		var fieldHash2 int64
		var fieldDecoder1 *structFieldDecoder
		var fieldDecoder2 *structFieldDecoder
		for fieldName, fieldDecoder := range fields {
			fieldHash := calcHash(fieldName)
			_, known := knownHash[fieldHash]
			if known {
				return &generalStructDecoder{typ, fields, false}
			}
			knownHash[fieldHash] = struct{}{}
			if fieldHash1 == 0 {
				fieldHash1 = fieldHash
				fieldDecoder1 = fieldDecoder
			} else {
				fieldHash2 = fieldHash
				fieldDecoder2 = fieldDecoder
			}
		}
		return &twoFieldsStructDecoder{typ, fieldHash1, fieldDecoder1, fieldHash2, fieldDecoder2}
	case 3:
		var fieldName1 int64
		var fieldName2 int64
		var fieldName3 int64
		var fieldDecoder1 *structFieldDecoder
		var fieldDecoder2 *structFieldDecoder
		var fieldDecoder3 *structFieldDecoder
		for fieldName, fieldDecoder := range fields {
			fieldHash := calcHash(fieldName)
			_, known := knownHash[fieldHash]
			if known {
				return &generalStructDecoder{typ, fields, false}
			}
			knownHash[fieldHash] = struct{}{}
			if fieldName1 == 0 {
				fieldName1 = fieldHash
				fieldDecoder1 = fieldDecoder
			} else if fieldName2 == 0 {
				fieldName2 = fieldHash
				fieldDecoder2 = fieldDecoder
			} else {
				fieldName3 = fieldHash
				fieldDecoder3 = fieldDecoder
			}
		}
		return &threeFieldsStructDecoder{typ,
			fieldName1, fieldDecoder1,
			fieldName2, fieldDecoder2,
			fieldName3, fieldDecoder3}
	case 4:
		var fieldName1 int64
		var fieldName2 int64
		var fieldName3 int64
		var fieldName4 int64
		var fieldDecoder1 *structFieldDecoder
		var fieldDecoder2 *structFieldDecoder
		var fieldDecoder3 *structFieldDecoder
		var fieldDecoder4 *structFieldDecoder
		for fieldName, fieldDecoder := range fields {
			fieldHash := calcHash(fieldName)
			_, known := knownHash[fieldHash]
			if known {
				return &generalStructDecoder{typ, fields, false}
			}
			knownHash[fieldHash] = struct{}{}
			if fieldName1 == 0 {
				fieldName1 = fieldHash
				fieldDecoder1 = fieldDecoder
			} else if fieldName2 == 0 {
				fieldName2 = fieldHash
				fieldDecoder2 = fieldDecoder
			} else if fieldName3 == 0 {
				fieldName3 = fieldHash
				fieldDecoder3 = fieldDecoder
			} else {
				fieldName4 = fieldHash
				fieldDecoder4 = fieldDecoder
			}
		}
		return &fourFieldsStructDecoder{typ,
			fieldName1, fieldDecoder1,
			fieldName2, fieldDecoder2,
			fieldName3, fieldDecoder3,
			fieldName4, fieldDecoder4}
	case 5:
		var fieldName1 int64
		var fieldName2 int64
		var fieldName3 int64
		var fieldName4 int64
		var fieldName5 int64
		var fieldDecoder1 *structFieldDecoder
		var fieldDecoder2 *structFieldDecoder
		var fieldDecoder3 *structFieldDecoder
		var fieldDecoder4 *structFieldDecoder
		var fieldDecoder5 *structFieldDecoder
		for fieldName, fieldDecoder := range fields {
			fieldHash := calcHash(fieldName)
			_, known := knownHash[fieldHash]
			if known {
				return &generalStructDecoder{typ, fields, false}
			}
			knownHash[fieldHash] = struct{}{}
			if fieldName1 == 0 {
				fieldName1 = fieldHash
				fieldDecoder1 = fieldDecoder
			} else if fieldName2 == 0 {
				fieldName2 = fieldHash
				fieldDecoder2 = fieldDecoder
			} else if fieldName3 == 0 {
				fieldName3 = fieldHash
				fieldDecoder3 = fieldDecoder
			} else if fieldName4 == 0 {
				fieldName4 = fieldHash
				fieldDecoder4 = fieldDecoder
			} else {
				fieldName5 = fieldHash
				fieldDecoder5 = fieldDecoder
			}
		}
		return &fiveFieldsStructDecoder{typ,
			fieldName1, fieldDecoder1,
			fieldName2, fieldDecoder2,
			fieldName3, fieldDecoder3,
			fieldName4, fieldDecoder4,
			fieldName5, fieldDecoder5}
	case 6:
		var fieldName1 int64
		var fieldName2 int64
		var fieldName3 int64
		var fieldName4 int64
		var fieldName5 int64
		var fieldName6 int64
		var fieldDecoder1 *structFieldDecoder
		var fieldDecoder2 *structFieldDecoder
		var fieldDecoder3 *structFieldDecoder
		var fieldDecoder4 *structFieldDecoder
		var fieldDecoder5 *structFieldDecoder
		var fieldDecoder6 *structFieldDecoder
		for fieldName, fieldDecoder := range fields {
			fieldHash := calcHash(fieldName)
			_, known := knownHash[fieldHash]
			if known {
				return &generalStructDecoder{typ, fields, false}
			}
			knownHash[fieldHash] = struct{}{}
			if fieldName1 == 0 {
				fieldName1 = fieldHash
				fieldDecoder1 = fieldDecoder
			} else if fieldName2 == 0 {
				fieldName2 = fieldHash
				fieldDecoder2 = fieldDecoder
			} else if fieldName3 == 0 {
				fieldName3 = fieldHash
				fieldDecoder3 = fieldDecoder
			} else if fieldName4 == 0 {
				fieldName4 = fieldHash
				fieldDecoder4 = fieldDecoder
			} else if fieldName5 == 0 {
				fieldName5 = fieldHash
				fieldDecoder5 = fieldDecoder
			} else {
				fieldName6 = fieldHash
				fieldDecoder6 = fieldDecoder
			}
		}
		return &sixFieldsStructDecoder{typ,
			fieldName1, fieldDecoder1,
			fieldName2, fieldDecoder2,
			fieldName3, fieldDecoder3,
			fieldName4, fieldDecoder4,
			fieldName5, fieldDecoder5,
			fieldName6, fieldDecoder6}
	case 7:
		var fieldName1 int64
		var fieldName2 int64
		var fieldName3 int64
		var fieldName4 int64
		var fieldName5 int64
		var fieldName6 int64
		var fieldName7 int64
		var fieldDecoder1 *structFieldDecoder
		var fieldDecoder2 *structFieldDecoder
		var fieldDecoder3 *structFieldDecoder
		var fieldDecoder4 *structFieldDecoder
		var fieldDecoder5 *structFieldDecoder
		var fieldDecoder6 *structFieldDecoder
		var fieldDecoder7 *structFieldDecoder
		for fieldName, fieldDecoder := range fields {
			fieldHash := calcHash(fieldName)
			_, known := knownHash[fieldHash]
			if known {
				return &generalStructDecoder{typ, fields, false}
			}
			knownHash[fieldHash] = struct{}{}
			if fieldName1 == 0 {
				fieldName1 = fieldHash
				fieldDecoder1 = fieldDecoder
			} else if fieldName2 == 0 {
				fieldName2 = fieldHash
				fieldDecoder2 = fieldDecoder
			} else if fieldName3 == 0 {
				fieldName3 = fieldHash
				fieldDecoder3 = fieldDecoder
			} else if fieldName4 == 0 {
				fieldName4 = fieldHash
				fieldDecoder4 = fieldDecoder
			} else if fieldName5 == 0 {
				fieldName5 = fieldHash
				fieldDecoder5 = fieldDecoder
			} else if fieldName6 == 0 {
				fieldName6 = fieldHash
				fieldDecoder6 = fieldDecoder
			} else {
				fieldName7 = fieldHash
				fieldDecoder7 = fieldDecoder
			}
		}
		return &sevenFieldsStructDecoder{typ,
			fieldName1, fieldDecoder1,
			fieldName2, fieldDecoder2,
			fieldName3, fieldDecoder3,
			fieldName4, fieldDecoder4,
			fieldName5, fieldDecoder5,
			fieldName6, fieldDecoder6,
			fieldName7, fieldDecoder7}
	case 8:
		var fieldName1 int64
		var fieldName2 int64
		var fieldName3 int64
		var fieldName4 int64
		var fieldName5 int64
		var fieldName6 int64
		var fieldName7 int64
		var fieldName8 int64
		var fieldDecoder1 *structFieldDecoder
		var fieldDecoder2 *structFieldDecoder
		var fieldDecoder3 *structFieldDecoder
		var fieldDecoder4 *structFieldDecoder
		var fieldDecoder5 *structFieldDecoder
		var fieldDecoder6 *structFieldDecoder
		var fieldDecoder7 *structFieldDecoder
		var fieldDecoder8 *structFieldDecoder
		for fieldName, fieldDecoder := range fields {
			fieldHash := calcHash(fieldName)
			_, known := knownHash[fieldHash]
			if known {
				return &generalStructDecoder{typ, fields, false}
			}
			knownHash[fieldHash] = struct{}{}
			if fieldName1 == 0 {
				fieldName1 = fieldHash
				fieldDecoder1 = fieldDecoder
			} else if fieldName2 == 0 {
				fieldName2 = fieldHash
				fieldDecoder2 = fieldDecoder
			} else if fieldName3 == 0 {
				fieldName3 = fieldHash
				fieldDecoder3 = fieldDecoder
			} else if fieldName4 == 0 {
				fieldName4 = fieldHash
				fieldDecoder4 = fieldDecoder
			} else if fieldName5 == 0 {
				fieldName5 = fieldHash
				fieldDecoder5 = fieldDecoder
			} else if fieldName6 == 0 {
				fieldName6 = fieldHash
				fieldDecoder6 = fieldDecoder
			} else if fieldName7 == 0 {
				fieldName7 = fieldHash
				fieldDecoder7 = fieldDecoder
			} else {
				fieldName8 = fieldHash
				fieldDecoder8 = fieldDecoder
			}
		}
		return &eightFieldsStructDecoder{typ,
			fieldName1, fieldDecoder1,
			fieldName2, fieldDecoder2,
			fieldName3, fieldDecoder3,
			fieldName4, fieldDecoder4,
			fieldName5, fieldDecoder5,
			fieldName6, fieldDecoder6,
			fieldName7, fieldDecoder7,
			fieldName8, fieldDecoder8}
	case 9:
		var fieldName1 int64
		var fieldName2 int64
		var fieldName3 int64
		var fieldName4 int64
		var fieldName5 int64
		var fieldName6 int64
		var fieldName7 int64
		var fieldName8 int64
		var fieldName9 int64
		var fieldDecoder1 *structFieldDecoder
		var fieldDecoder2 *structFieldDecoder
		var fieldDecoder3 *structFieldDecoder
		var fieldDecoder4 *structFieldDecoder
		var fieldDecoder5 *structFieldDecoder
		var fieldDecoder6 *structFieldDecoder
		var fieldDecoder7 *structFieldDecoder
		var fieldDecoder8 *structFieldDecoder
		var fieldDecoder9 *structFieldDecoder
		for fieldName, fieldDecoder := range fields {
			fieldHash := calcHash(fieldName)
			_, known := knownHash[fieldHash]
			if known {
				return &generalStructDecoder{typ, fields, false}
			}
			knownHash[fieldHash] = struct{}{}
			if fieldName1 == 0 {
				fieldName1 = fieldHash
				fieldDecoder1 = fieldDecoder
			} else if fieldName2 == 0 {
				fieldName2 = fieldHash
				fieldDecoder2 = fieldDecoder
			} else if fieldName3 == 0 {
				fieldName3 = fieldHash
				fieldDecoder3 = fieldDecoder
			} else if fieldName4 == 0 {
				fieldName4 = fieldHash
				fieldDecoder4 = fieldDecoder
			} else if fieldName5 == 0 {
				fieldName5 = fieldHash
				fieldDecoder5 = fieldDecoder
			} else if fieldName6 == 0 {
				fieldName6 = fieldHash
				fieldDecoder6 = fieldDecoder
			} else if fieldName7 == 0 {
				fieldName7 = fieldHash
				fieldDecoder7 = fieldDecoder
			} else if fieldName8 == 0 {
				fieldName8 = fieldHash
				fieldDecoder8 = fieldDecoder
			} else {
				fieldName9 = fieldHash
				fieldDecoder9 = fieldDecoder
			}
		}
		return &nineFieldsStructDecoder{typ,
			fieldName1, fieldDecoder1,
			fieldName2, fieldDecoder2,
			fieldName3, fieldDecoder3,
			fieldName4, fieldDecoder4,
			fieldName5, fieldDecoder5,
			fieldName6, fieldDecoder6,
			fieldName7, fieldDecoder7,
			fieldName8, fieldDecoder8,
			fieldName9, fieldDecoder9}
	case 10:
		var fieldName1 int64
		var fieldName2 int64
		var fieldName3 int64
		var fieldName4 int64
		var fieldName5 int64
		var fieldName6 int64
		var fieldName7 int64
		var fieldName8 int64
		var fieldName9 int64
		var fieldName10 int64
		var fieldDecoder1 *structFieldDecoder
		var fieldDecoder2 *structFieldDecoder
		var fieldDecoder3 *structFieldDecoder
		var fieldDecoder4 *structFieldDecoder
		var fieldDecoder5 *structFieldDecoder
		var fieldDecoder6 *structFieldDecoder
		var fieldDecoder7 *structFieldDecoder
		var fieldDecoder8 *structFieldDecoder
		var fieldDecoder9 *structFieldDecoder
		var fieldDecoder10 *structFieldDecoder
		for fieldName, fieldDecoder := range fields {
			fieldHash := calcHash(fieldName)
			_, known := knownHash[fieldHash]
			if known {
				return &generalStructDecoder{typ, fields, false}
			}
			knownHash[fieldHash] = struct{}{}
			if fieldName1 == 0 {
				fieldName1 = fieldHash
				fieldDecoder1 = fieldDecoder
			} else if fieldName2 == 0 {
				fieldName2 = fieldHash
				fieldDecoder2 = fieldDecoder
			} else if fieldName3 == 0 {
				fieldName3 = fieldHash
				fieldDecoder3 = fieldDecoder
			} else if fieldName4 == 0 {
				fieldName4 = fieldHash
				fieldDecoder4 = fieldDecoder
			} else if fieldName5 == 0 {
				fieldName5 = fieldHash
				fieldDecoder5 = fieldDecoder
			} else if fieldName6 == 0 {
				fieldName6 = fieldHash
				fieldDecoder6 = fieldDecoder
			} else if fieldName7 == 0 {
				fieldName7 = fieldHash
				fieldDecoder7 = fieldDecoder
			} else if fieldName8 == 0 {
				fieldName8 = fieldHash
				fieldDecoder8 = fieldDecoder
			} else if fieldName9 == 0 {
				fieldName9 = fieldHash
				fieldDecoder9 = fieldDecoder
			} else {
				fieldName10 = fieldHash
				fieldDecoder10 = fieldDecoder
			}
		}
		return &tenFieldsStructDecoder{typ,
			fieldName1, fieldDecoder1,
			fieldName2, fieldDecoder2,
			fieldName3, fieldDecoder3,
			fieldName4, fieldDecoder4,
			fieldName5, fieldDecoder5,
			fieldName6, fieldDecoder6,
			fieldName7, fieldDecoder7,
			fieldName8, fieldDecoder8,
			fieldName9, fieldDecoder9,
			fieldName10, fieldDecoder10}
	}
	return &generalStructDecoder{typ, fields, false}
}

type generalStructDecoder struct {
	typ                   reflect.Type
	fields                map[string]*structFieldDecoder
	disallowUnknownFields bool
}

func (decoder *generalStructDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if !iter.readObjectStart() {
		return
	}
	decoder.decodeOneField(ptr, iter)
	for iter.nextToken() == ',' {
		decoder.decodeOneField(ptr, iter)
	}
	if iter.Error != nil && iter.Error != io.EOF {
		iter.Error = fmt.Errorf("%v.%s", decoder.typ, iter.Error.Error())
	}
}

func (decoder *generalStructDecoder) decodeOneField(ptr unsafe.Pointer, iter *Iterator) {
	var field string
	var fieldDecoder *structFieldDecoder
	if iter.cfg.objectFieldMustBeSimpleString {
		fieldBytes := iter.ReadStringAsSlice()
		field = *(*string)(unsafe.Pointer(&fieldBytes))
		fieldDecoder = decoder.fields[field]
		if fieldDecoder == nil {
			fieldDecoder = decoder.fields[strings.ToLower(field)]
		}
	} else {
		field = iter.ReadString()
		fieldDecoder = decoder.fields[field]
		if fieldDecoder == nil {
			fieldDecoder = decoder.fields[strings.ToLower(field)]
		}
	}
	if fieldDecoder == nil {
		msg := "found unknown field: " + field
		if decoder.disallowUnknownFields {
			iter.ReportError("ReadObject", msg)
		}
		c := iter.nextToken()
		if c != ':' {
			iter.ReportError("ReadObject", "expect : after object field, but found "+string([]byte{c}))
		}
		iter.Skip()
		return
	}
	c := iter.nextToken()
	if c != ':' {
		iter.ReportError("ReadObject", "expect : after object field, but found "+string([]byte{c}))
	}
	fieldDecoder.Decode(ptr, iter)
}

type skipObjectDecoder struct {
	typ reflect.Type
}

func (decoder *skipObjectDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	valueType := iter.WhatIsNext()
	if valueType != ObjectValue && valueType != NilValue {
		iter.ReportError("skipObjectDecoder", "expect object or null")
		return
	}
	iter.Skip()
}

type oneFieldStructDecoder struct {
	typ          reflect.Type
	fieldHash    int64
	fieldDecoder *structFieldDecoder
}

func (decoder *oneFieldStructDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if !iter.readObjectStart() {
		return
	}
	for {
		if iter.readFieldHash() == decoder.fieldHash {
			decoder.fieldDecoder.Decode(ptr, iter)
		} else {
			iter.Skip()
		}
		if iter.isObjectEnd() {
			break
		}
	}
	if iter.Error != nil && iter.Error != io.EOF {
		iter.Error = fmt.Errorf("%v.%s", decoder.typ, iter.Error.Error())
	}
}

type twoFieldsStructDecoder struct {
	typ           reflect.Type
	fieldHash1    int64
	fieldDecoder1 *structFieldDecoder
	fieldHash2    int64
	fieldDecoder2 *structFieldDecoder
}

func (decoder *twoFieldsStructDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if !iter.readObjectStart() {
		return
	}
	for {
		switch iter.readFieldHash() {
		case decoder.fieldHash1:
			decoder.fieldDecoder1.Decode(ptr, iter)
		case decoder.fieldHash2:
			decoder.fieldDecoder2.Decode(ptr, iter)
		default:
			iter.Skip()
		}
		if iter.isObjectEnd() {
			break
		}
	}
	if iter.Error != nil && iter.Error != io.EOF {
		iter.Error = fmt.Errorf("%v.%s", decoder.typ, iter.Error.Error())
	}
}

type threeFieldsStructDecoder struct {
	typ           reflect.Type
	fieldHash1    int64
	fieldDecoder1 *structFieldDecoder
	fieldHash2    int64
	fieldDecoder2 *structFieldDecoder
	fieldHash3    int64
	fieldDecoder3 *structFieldDecoder
}

func (decoder *threeFieldsStructDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if !iter.readObjectStart() {
		return
	}
	for {
		switch iter.readFieldHash() {
		case decoder.fieldHash1:
			decoder.fieldDecoder1.Decode(ptr, iter)
		case decoder.fieldHash2:
			decoder.fieldDecoder2.Decode(ptr, iter)
		case decoder.fieldHash3:
			decoder.fieldDecoder3.Decode(ptr, iter)
		default:
			iter.Skip()
		}
		if iter.isObjectEnd() {
			break
		}
	}
	if iter.Error != nil && iter.Error != io.EOF {
		iter.Error = fmt.Errorf("%v.%s", decoder.typ, iter.Error.Error())
	}
}

type fourFieldsStructDecoder struct {
	typ           reflect.Type
	fieldHash1    int64
	fieldDecoder1 *structFieldDecoder
	fieldHash2    int64
	fieldDecoder2 *structFieldDecoder
	fieldHash3    int64
	fieldDecoder3 *structFieldDecoder
	fieldHash4    int64
	fieldDecoder4 *structFieldDecoder
}

func (decoder *fourFieldsStructDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if !iter.readObjectStart() {
		return
	}
	for {
		switch iter.readFieldHash() {
		case decoder.fieldHash1:
			decoder.fieldDecoder1.Decode(ptr, iter)
		case decoder.fieldHash2:
			decoder.fieldDecoder2.Decode(ptr, iter)
		case decoder.fieldHash3:
			decoder.fieldDecoder3.Decode(ptr, iter)
		case decoder.fieldHash4:
			decoder.fieldDecoder4.Decode(ptr, iter)
		default:
			iter.Skip()
		}
		if iter.isObjectEnd() {
			break
		}
	}
	if iter.Error != nil && iter.Error != io.EOF {
		iter.Error = fmt.Errorf("%v.%s", decoder.typ, iter.Error.Error())
	}
}

type fiveFieldsStructDecoder struct {
	typ           reflect.Type
	fieldHash1    int64
	fieldDecoder1 *structFieldDecoder
	fieldHash2    int64
	fieldDecoder2 *structFieldDecoder
	fieldHash3    int64
	fieldDecoder3 *structFieldDecoder
	fieldHash4    int64
	fieldDecoder4 *structFieldDecoder
	fieldHash5    int64
	fieldDecoder5 *structFieldDecoder
}

func (decoder *fiveFieldsStructDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if !iter.readObjectStart() {
		return
	}
	for {
		switch iter.readFieldHash() {
		case decoder.fieldHash1:
			decoder.fieldDecoder1.Decode(ptr, iter)
		case decoder.fieldHash2:
			decoder.fieldDecoder2.Decode(ptr, iter)
		case decoder.fieldHash3:
			decoder.fieldDecoder3.Decode(ptr, iter)
		case decoder.fieldHash4:
			decoder.fieldDecoder4.Decode(ptr, iter)
		case decoder.fieldHash5:
			decoder.fieldDecoder5.Decode(ptr, iter)
		default:
			iter.Skip()
		}
		if iter.isObjectEnd() {
			break
		}
	}
	if iter.Error != nil && iter.Error != io.EOF {
		iter.Error = fmt.Errorf("%v.%s", decoder.typ, iter.Error.Error())
	}
}

type sixFieldsStructDecoder struct {
	typ           reflect.Type
	fieldHash1    int64
	fieldDecoder1 *structFieldDecoder
	fieldHash2    int64
	fieldDecoder2 *structFieldDecoder
	fieldHash3    int64
	fieldDecoder3 *structFieldDecoder
	fieldHash4    int64
	fieldDecoder4 *structFieldDecoder
	fieldHash5    int64
	fieldDecoder5 *structFieldDecoder
	fieldHash6    int64
	fieldDecoder6 *structFieldDecoder
}

func (decoder *sixFieldsStructDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if !iter.readObjectStart() {
		return
	}
	for {
		switch iter.readFieldHash() {
		case decoder.fieldHash1:
			decoder.fieldDecoder1.Decode(ptr, iter)
		case decoder.fieldHash2:
			decoder.fieldDecoder2.Decode(ptr, iter)
		case decoder.fieldHash3:
			decoder.fieldDecoder3.Decode(ptr, iter)
		case decoder.fieldHash4:
			decoder.fieldDecoder4.Decode(ptr, iter)
		case decoder.fieldHash5:
			decoder.fieldDecoder5.Decode(ptr, iter)
		case decoder.fieldHash6:
			decoder.fieldDecoder6.Decode(ptr, iter)
		default:
			iter.Skip()
		}
		if iter.isObjectEnd() {
			break
		}
	}
	if iter.Error != nil && iter.Error != io.EOF {
		iter.Error = fmt.Errorf("%v.%s", decoder.typ, iter.Error.Error())
	}
}

type sevenFieldsStructDecoder struct {
	typ           reflect.Type
	fieldHash1    int64
	fieldDecoder1 *structFieldDecoder
	fieldHash2    int64
	fieldDecoder2 *structFieldDecoder
	fieldHash3    int64
	fieldDecoder3 *structFieldDecoder
	fieldHash4    int64
	fieldDecoder4 *structFieldDecoder
	fieldHash5    int64
	fieldDecoder5 *structFieldDecoder
	fieldHash6    int64
	fieldDecoder6 *structFieldDecoder
	fieldHash7    int64
	fieldDecoder7 *structFieldDecoder
}

func (decoder *sevenFieldsStructDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if !iter.readObjectStart() {
		return
	}
	for {
		switch iter.readFieldHash() {
		case decoder.fieldHash1:
			decoder.fieldDecoder1.Decode(ptr, iter)
		case decoder.fieldHash2:
			decoder.fieldDecoder2.Decode(ptr, iter)
		case decoder.fieldHash3:
			decoder.fieldDecoder3.Decode(ptr, iter)
		case decoder.fieldHash4:
			decoder.fieldDecoder4.Decode(ptr, iter)
		case decoder.fieldHash5:
			decoder.fieldDecoder5.Decode(ptr, iter)
		case decoder.fieldHash6:
			decoder.fieldDecoder6.Decode(ptr, iter)
		case decoder.fieldHash7:
			decoder.fieldDecoder7.Decode(ptr, iter)
		default:
			iter.Skip()
		}
		if iter.isObjectEnd() {
			break
		}
	}
	if iter.Error != nil && iter.Error != io.EOF {
		iter.Error = fmt.Errorf("%v.%s", decoder.typ, iter.Error.Error())
	}
}

type eightFieldsStructDecoder struct {
	typ           reflect.Type
	fieldHash1    int64
	fieldDecoder1 *structFieldDecoder
	fieldHash2    int64
	fieldDecoder2 *structFieldDecoder
	fieldHash3    int64
	fieldDecoder3 *structFieldDecoder
	fieldHash4    int64
	fieldDecoder4 *structFieldDecoder
	fieldHash5    int64
	fieldDecoder5 *structFieldDecoder
	fieldHash6    int64
	fieldDecoder6 *structFieldDecoder
	fieldHash7    int64
	fieldDecoder7 *structFieldDecoder
	fieldHash8    int64
	fieldDecoder8 *structFieldDecoder
}

func (decoder *eightFieldsStructDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if !iter.readObjectStart() {
		return
	}
	for {
		switch iter.readFieldHash() {
		case decoder.fieldHash1:
			decoder.fieldDecoder1.Decode(ptr, iter)
		case decoder.fieldHash2:
			decoder.fieldDecoder2.Decode(ptr, iter)
		case decoder.fieldHash3:
			decoder.fieldDecoder3.Decode(ptr, iter)
		case decoder.fieldHash4:
			decoder.fieldDecoder4.Decode(ptr, iter)
		case decoder.fieldHash5:
			decoder.fieldDecoder5.Decode(ptr, iter)
		case decoder.fieldHash6:
			decoder.fieldDecoder6.Decode(ptr, iter)
		case decoder.fieldHash7:
			decoder.fieldDecoder7.Decode(ptr, iter)
		case decoder.fieldHash8:
			decoder.fieldDecoder8.Decode(ptr, iter)
		default:
			iter.Skip()
		}
		if iter.isObjectEnd() {
			break
		}
	}
	if iter.Error != nil && iter.Error != io.EOF {
		iter.Error = fmt.Errorf("%v.%s", decoder.typ, iter.Error.Error())
	}
}

type nineFieldsStructDecoder struct {
	typ           reflect.Type
	fieldHash1    int64
	fieldDecoder1 *structFieldDecoder
	fieldHash2    int64
	fieldDecoder2 *structFieldDecoder
	fieldHash3    int64
	fieldDecoder3 *structFieldDecoder
	fieldHash4    int64
	fieldDecoder4 *structFieldDecoder
	fieldHash5    int64
	fieldDecoder5 *structFieldDecoder
	fieldHash6    int64
	fieldDecoder6 *structFieldDecoder
	fieldHash7    int64
	fieldDecoder7 *structFieldDecoder
	fieldHash8    int64
	fieldDecoder8 *structFieldDecoder
	fieldHash9    int64
	fieldDecoder9 *structFieldDecoder
}

func (decoder *nineFieldsStructDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if !iter.readObjectStart() {
		return
	}
	for {
		switch iter.readFieldHash() {
		case decoder.fieldHash1:
			decoder.fieldDecoder1.Decode(ptr, iter)
		case decoder.fieldHash2:
			decoder.fieldDecoder2.Decode(ptr, iter)
		case decoder.fieldHash3:
			decoder.fieldDecoder3.Decode(ptr, iter)
		case decoder.fieldHash4:
			decoder.fieldDecoder4.Decode(ptr, iter)
		case decoder.fieldHash5:
			decoder.fieldDecoder5.Decode(ptr, iter)
		case decoder.fieldHash6:
			decoder.fieldDecoder6.Decode(ptr, iter)
		case decoder.fieldHash7:
			decoder.fieldDecoder7.Decode(ptr, iter)
		case decoder.fieldHash8:
			decoder.fieldDecoder8.Decode(ptr, iter)
		case decoder.fieldHash9:
			decoder.fieldDecoder9.Decode(ptr, iter)
		default:
			iter.Skip()
		}
		if iter.isObjectEnd() {
			break
		}
	}
	if iter.Error != nil && iter.Error != io.EOF {
		iter.Error = fmt.Errorf("%v.%s", decoder.typ, iter.Error.Error())
	}
}

type tenFieldsStructDecoder struct {
	typ            reflect.Type
	fieldHash1     int64
	fieldDecoder1  *structFieldDecoder
	fieldHash2     int64
	fieldDecoder2  *structFieldDecoder
	fieldHash3     int64
	fieldDecoder3  *structFieldDecoder
	fieldHash4     int64
	fieldDecoder4  *structFieldDecoder
	fieldHash5     int64
	fieldDecoder5  *structFieldDecoder
	fieldHash6     int64
	fieldDecoder6  *structFieldDecoder
	fieldHash7     int64
	fieldDecoder7  *structFieldDecoder
	fieldHash8     int64
	fieldDecoder8  *structFieldDecoder
	fieldHash9     int64
	fieldDecoder9  *structFieldDecoder
	fieldHash10    int64
	fieldDecoder10 *structFieldDecoder
}

func (decoder *tenFieldsStructDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	if !iter.readObjectStart() {
		return
	}
	for {
		switch iter.readFieldHash() {
		case decoder.fieldHash1:
			decoder.fieldDecoder1.Decode(ptr, iter)
		case decoder.fieldHash2:
			decoder.fieldDecoder2.Decode(ptr, iter)
		case decoder.fieldHash3:
			decoder.fieldDecoder3.Decode(ptr, iter)
		case decoder.fieldHash4:
			decoder.fieldDecoder4.Decode(ptr, iter)
		case decoder.fieldHash5:
			decoder.fieldDecoder5.Decode(ptr, iter)
		case decoder.fieldHash6:
			decoder.fieldDecoder6.Decode(ptr, iter)
		case decoder.fieldHash7:
			decoder.fieldDecoder7.Decode(ptr, iter)
		case decoder.fieldHash8:
			decoder.fieldDecoder8.Decode(ptr, iter)
		case decoder.fieldHash9:
			decoder.fieldDecoder9.Decode(ptr, iter)
		case decoder.fieldHash10:
			decoder.fieldDecoder10.Decode(ptr, iter)
		default:
			iter.Skip()
		}
		if iter.isObjectEnd() {
			break
		}
	}
	if iter.Error != nil && iter.Error != io.EOF {
		iter.Error = fmt.Errorf("%v.%s", decoder.typ, iter.Error.Error())
	}
}

type structFieldDecoder struct {
	field        *reflect.StructField
	fieldDecoder ValDecoder
}

func (decoder *structFieldDecoder) Decode(ptr unsafe.Pointer, iter *Iterator) {
	fieldPtr := unsafe.Pointer(uintptr(ptr) + decoder.field.Offset)
	decoder.fieldDecoder.Decode(fieldPtr, iter)
	if iter.Error != nil && iter.Error != io.EOF {
		iter.Error = fmt.Errorf("%s: %s", decoder.field.Name, iter.Error.Error())
	}
}
