package responses

import (
	"encoding/json"
	"io"
	"math"
	"strconv"
	"strings"
	"unsafe"

	jsoniter "github.com/json-iterator/go"
)

const maxUint = ^uint(0)
const maxInt = int(maxUint >> 1)
const minInt = -maxInt - 1

var jsonParser jsoniter.API

func init() {
	registerBetterFuzzyDecoder()
	jsonParser = jsoniter.Config{
		EscapeHTML:             true,
		SortMapKeys:            true,
		ValidateJsonRawMessage: true,
		CaseSensitive:          true,
	}.Froze()
}

func registerBetterFuzzyDecoder() {
	jsoniter.RegisterTypeDecoder("string", &nullableFuzzyStringDecoder{})
	jsoniter.RegisterTypeDecoder("bool", &fuzzyBoolDecoder{})
	jsoniter.RegisterTypeDecoder("float32", &nullableFuzzyFloat32Decoder{})
	jsoniter.RegisterTypeDecoder("float64", &nullableFuzzyFloat64Decoder{})
	jsoniter.RegisterTypeDecoder("int", &nullableFuzzyIntegerDecoder{func(isFloat bool, ptr unsafe.Pointer, iter *jsoniter.Iterator) {
		if isFloat {
			val := iter.ReadFloat64()
			if val > float64(maxInt) || val < float64(minInt) {
				iter.ReportError("fuzzy decode int", "exceed range")
				return
			}
			*((*int)(ptr)) = int(val)
		} else {
			*((*int)(ptr)) = iter.ReadInt()
		}
	}})
	jsoniter.RegisterTypeDecoder("uint", &nullableFuzzyIntegerDecoder{func(isFloat bool, ptr unsafe.Pointer, iter *jsoniter.Iterator) {
		if isFloat {
			val := iter.ReadFloat64()
			if val > float64(maxUint) || val < 0 {
				iter.ReportError("fuzzy decode uint", "exceed range")
				return
			}
			*((*uint)(ptr)) = uint(val)
		} else {
			*((*uint)(ptr)) = iter.ReadUint()
		}
	}})
	jsoniter.RegisterTypeDecoder("int8", &nullableFuzzyIntegerDecoder{func(isFloat bool, ptr unsafe.Pointer, iter *jsoniter.Iterator) {
		if isFloat {
			val := iter.ReadFloat64()
			if val > float64(math.MaxInt8) || val < float64(math.MinInt8) {
				iter.ReportError("fuzzy decode int8", "exceed range")
				return
			}
			*((*int8)(ptr)) = int8(val)
		} else {
			*((*int8)(ptr)) = iter.ReadInt8()
		}
	}})
	jsoniter.RegisterTypeDecoder("uint8", &nullableFuzzyIntegerDecoder{func(isFloat bool, ptr unsafe.Pointer, iter *jsoniter.Iterator) {
		if isFloat {
			val := iter.ReadFloat64()
			if val > float64(math.MaxUint8) || val < 0 {
				iter.ReportError("fuzzy decode uint8", "exceed range")
				return
			}
			*((*uint8)(ptr)) = uint8(val)
		} else {
			*((*uint8)(ptr)) = iter.ReadUint8()
		}
	}})
	jsoniter.RegisterTypeDecoder("int16", &nullableFuzzyIntegerDecoder{func(isFloat bool, ptr unsafe.Pointer, iter *jsoniter.Iterator) {
		if isFloat {
			val := iter.ReadFloat64()
			if val > float64(math.MaxInt16) || val < float64(math.MinInt16) {
				iter.ReportError("fuzzy decode int16", "exceed range")
				return
			}
			*((*int16)(ptr)) = int16(val)
		} else {
			*((*int16)(ptr)) = iter.ReadInt16()
		}
	}})
	jsoniter.RegisterTypeDecoder("uint16", &nullableFuzzyIntegerDecoder{func(isFloat bool, ptr unsafe.Pointer, iter *jsoniter.Iterator) {
		if isFloat {
			val := iter.ReadFloat64()
			if val > float64(math.MaxUint16) || val < 0 {
				iter.ReportError("fuzzy decode uint16", "exceed range")
				return
			}
			*((*uint16)(ptr)) = uint16(val)
		} else {
			*((*uint16)(ptr)) = iter.ReadUint16()
		}
	}})
	jsoniter.RegisterTypeDecoder("int32", &nullableFuzzyIntegerDecoder{func(isFloat bool, ptr unsafe.Pointer, iter *jsoniter.Iterator) {
		if isFloat {
			val := iter.ReadFloat64()
			if val > float64(math.MaxInt32) || val < float64(math.MinInt32) {
				iter.ReportError("fuzzy decode int32", "exceed range")
				return
			}
			*((*int32)(ptr)) = int32(val)
		} else {
			*((*int32)(ptr)) = iter.ReadInt32()
		}
	}})
	jsoniter.RegisterTypeDecoder("uint32", &nullableFuzzyIntegerDecoder{func(isFloat bool, ptr unsafe.Pointer, iter *jsoniter.Iterator) {
		if isFloat {
			val := iter.ReadFloat64()
			if val > float64(math.MaxUint32) || val < 0 {
				iter.ReportError("fuzzy decode uint32", "exceed range")
				return
			}
			*((*uint32)(ptr)) = uint32(val)
		} else {
			*((*uint32)(ptr)) = iter.ReadUint32()
		}
	}})
	jsoniter.RegisterTypeDecoder("int64", &nullableFuzzyIntegerDecoder{func(isFloat bool, ptr unsafe.Pointer, iter *jsoniter.Iterator) {
		if isFloat {
			val := iter.ReadFloat64()
			if val > float64(math.MaxInt64) || val < float64(math.MinInt64) {
				iter.ReportError("fuzzy decode int64", "exceed range")
				return
			}
			*((*int64)(ptr)) = int64(val)
		} else {
			*((*int64)(ptr)) = iter.ReadInt64()
		}
	}})
	jsoniter.RegisterTypeDecoder("uint64", &nullableFuzzyIntegerDecoder{func(isFloat bool, ptr unsafe.Pointer, iter *jsoniter.Iterator) {
		if isFloat {
			val := iter.ReadFloat64()
			if val > float64(math.MaxUint64) || val < 0 {
				iter.ReportError("fuzzy decode uint64", "exceed range")
				return
			}
			*((*uint64)(ptr)) = uint64(val)
		} else {
			*((*uint64)(ptr)) = iter.ReadUint64()
		}
	}})
}

type nullableFuzzyStringDecoder struct {
}

func (decoder *nullableFuzzyStringDecoder) Decode(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
	valueType := iter.WhatIsNext()
	switch valueType {
	case jsoniter.NumberValue:
		var number json.Number
		iter.ReadVal(&number)
		*((*string)(ptr)) = string(number)
	case jsoniter.StringValue:
		*((*string)(ptr)) = iter.ReadString()
	case jsoniter.BoolValue:
		*((*string)(ptr)) = strconv.FormatBool(iter.ReadBool())
	case jsoniter.NilValue:
		iter.ReadNil()
		*((*string)(ptr)) = ""
	default:
		iter.ReportError("fuzzyStringDecoder", "not number or string or bool")
	}
}

type fuzzyBoolDecoder struct {
}

func (decoder *fuzzyBoolDecoder) Decode(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
	valueType := iter.WhatIsNext()
	switch valueType {
	case jsoniter.BoolValue:
		*((*bool)(ptr)) = iter.ReadBool()
	case jsoniter.NumberValue:
		var number json.Number
		iter.ReadVal(&number)
		num, err := number.Int64()
		if err != nil {
			iter.ReportError("fuzzyBoolDecoder", "get value from json.number failed")
		}
		if num == 0 {
			*((*bool)(ptr)) = false
		} else {
			*((*bool)(ptr)) = true
		}
	case jsoniter.StringValue:
		strValue := strings.ToLower(iter.ReadString())
		if strValue == "true" {
			*((*bool)(ptr)) = true
		} else if strValue == "false" || strValue == "" {
			*((*bool)(ptr)) = false
		} else {
			iter.ReportError("fuzzyBoolDecoder", "unsupported bool value: "+strValue)
		}
	case jsoniter.NilValue:
		iter.ReadNil()
		*((*bool)(ptr)) = false
	default:
		iter.ReportError("fuzzyBoolDecoder", "not number or string or nil")
	}
}

type nullableFuzzyIntegerDecoder struct {
	fun func(isFloat bool, ptr unsafe.Pointer, iter *jsoniter.Iterator)
}

func (decoder *nullableFuzzyIntegerDecoder) Decode(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
	valueType := iter.WhatIsNext()
	var str string
	switch valueType {
	case jsoniter.NumberValue:
		var number json.Number
		iter.ReadVal(&number)
		str = string(number)
	case jsoniter.StringValue:
		str = iter.ReadString()
		// support empty string
		if str == "" {
			str = "0"
		}
	case jsoniter.BoolValue:
		if iter.ReadBool() {
			str = "1"
		} else {
			str = "0"
		}
	case jsoniter.NilValue:
		iter.ReadNil()
		str = "0"
	default:
		iter.ReportError("fuzzyIntegerDecoder", "not number or string")
	}
	newIter := iter.Pool().BorrowIterator([]byte(str))
	defer iter.Pool().ReturnIterator(newIter)
	isFloat := strings.IndexByte(str, '.') != -1
	decoder.fun(isFloat, ptr, newIter)
	if newIter.Error != nil && newIter.Error != io.EOF {
		iter.Error = newIter.Error
	}
}

type nullableFuzzyFloat32Decoder struct {
}

func (decoder *nullableFuzzyFloat32Decoder) Decode(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
	valueType := iter.WhatIsNext()
	var str string
	switch valueType {
	case jsoniter.NumberValue:
		*((*float32)(ptr)) = iter.ReadFloat32()
	case jsoniter.StringValue:
		str = iter.ReadString()
		// support empty string
		if str == "" {
			*((*float32)(ptr)) = 0
			return
		}
		newIter := iter.Pool().BorrowIterator([]byte(str))
		defer iter.Pool().ReturnIterator(newIter)
		*((*float32)(ptr)) = newIter.ReadFloat32()
		if newIter.Error != nil && newIter.Error != io.EOF {
			iter.Error = newIter.Error
		}
	case jsoniter.BoolValue:
		// support bool to float32
		if iter.ReadBool() {
			*((*float32)(ptr)) = 1
		} else {
			*((*float32)(ptr)) = 0
		}
	case jsoniter.NilValue:
		iter.ReadNil()
		*((*float32)(ptr)) = 0
	default:
		iter.ReportError("nullableFuzzyFloat32Decoder", "not number or string")
	}
}

type nullableFuzzyFloat64Decoder struct {
}

func (decoder *nullableFuzzyFloat64Decoder) Decode(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
	valueType := iter.WhatIsNext()
	var str string
	switch valueType {
	case jsoniter.NumberValue:
		*((*float64)(ptr)) = iter.ReadFloat64()
	case jsoniter.StringValue:
		str = iter.ReadString()
		// support empty string
		if str == "" {
			*((*float64)(ptr)) = 0
			return
		}
		newIter := iter.Pool().BorrowIterator([]byte(str))
		defer iter.Pool().ReturnIterator(newIter)
		*((*float64)(ptr)) = newIter.ReadFloat64()
		if newIter.Error != nil && newIter.Error != io.EOF {
			iter.Error = newIter.Error
		}
	case jsoniter.BoolValue:
		// support bool to float64
		if iter.ReadBool() {
			*((*float64)(ptr)) = 1
		} else {
			*((*float64)(ptr)) = 0
		}
	case jsoniter.NilValue:
		// support empty string
		iter.ReadNil()
		*((*float64)(ptr)) = 0
	default:
		iter.ReportError("nullableFuzzyFloat64Decoder", "not number or string")
	}
}
