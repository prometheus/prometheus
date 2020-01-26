// Copyright 2017 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spanner

import (
	"reflect"
	"strconv"
	"testing"

	"cloud.google.com/go/civil"
	proto3 "github.com/golang/protobuf/ptypes/struct"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
)

func BenchmarkEncodeIntArray(b *testing.B) {
	for _, s := range []struct {
		name string
		f    func(a []int) (*proto3.Value, *sppb.Type, error)
	}{
		{"Orig", encodeIntArrayOrig},
		{"Func", encodeIntArrayFunc},
		{"Reflect", encodeIntArrayReflect},
	} {
		b.Run(s.name, func(b *testing.B) {
			for _, size := range []int{1, 10, 100, 1000} {
				a := make([]int, size)
				b.Run(strconv.Itoa(size), func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						s.f(a)
					}
				})
			}
		})
	}
}

func encodeIntArrayOrig(a []int) (*proto3.Value, *sppb.Type, error) {
	vs := make([]*proto3.Value, len(a))
	var err error
	for i := range a {
		vs[i], _, err = encodeValue(a[i])
		if err != nil {
			return nil, nil, err
		}
	}
	return listProto(vs...), listType(intType()), nil
}

func encodeIntArrayFunc(a []int) (*proto3.Value, *sppb.Type, error) {
	v, err := encodeArray(len(a), func(i int) interface{} { return a[i] })
	if err != nil {
		return nil, nil, err
	}
	return v, listType(intType()), nil
}

func encodeIntArrayReflect(a []int) (*proto3.Value, *sppb.Type, error) {
	v, err := encodeArrayReflect(a)
	if err != nil {
		return nil, nil, err
	}
	return v, listType(intType()), nil
}

func encodeArrayReflect(a interface{}) (*proto3.Value, error) {
	va := reflect.ValueOf(a)
	len := va.Len()
	vs := make([]*proto3.Value, len)
	var err error
	for i := 0; i < len; i++ {
		vs[i], _, err = encodeValue(va.Index(i).Interface())
		if err != nil {
			return nil, err
		}
	}
	return listProto(vs...), nil
}

func BenchmarkDecodeGeneric(b *testing.B) {
	v := stringProto("test")
	t := stringType()
	var g GenericColumnValue
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decodeValue(v, t, &g)
	}
}

func BenchmarkDecodeArray(b *testing.B) {
	for _, size := range []int{1, 10, 100, 1000} {
		vals := make([]*proto3.Value, size)
		for i := 0; i < size; i++ {
			vals[i] = dateProto(d1)
		}
		lv := &proto3.ListValue{Values: vals}
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			for _, s := range []struct {
				name   string
				decode func(*proto3.ListValue)
			}{
				{"DateDirect", decodeArrayDateDirect},
				{"DateFunc", decodeArrayDateFunc},
				{"DateReflect", decodeArrayDateReflect},
				{"StringDecodeStringArray", decodeStringArrayWrap},
				{"StringDirect", decodeArrayStringDirect},
				{"StringFunc", decodeArrayStringFunc},
				{"StringReflect", decodeArrayStringReflect},
			} {
				b.Run(s.name, func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						s.decode(lv)
					}
				})
			}
		})

	}
}

func decodeArrayDateDirect(pb *proto3.ListValue) {
	a := make([]civil.Date, len(pb.Values))
	t := dateType()
	for i, v := range pb.Values {
		if err := decodeValue(v, t, &a[i]); err != nil {
			panic(err)
		}
	}
}

func decodeArrayDateFunc(pb *proto3.ListValue) {
	a := make([]civil.Date, len(pb.Values))
	if err := decodeArrayFunc(pb, "DATE", dateType(), func(i int) interface{} { return &a[i] }); err != nil {
		panic(err)
	}
}

func decodeArrayDateReflect(pb *proto3.ListValue) {
	var a []civil.Date
	if err := decodeArrayReflect(pb, "DATE", dateType(), &a); err != nil {
		panic(err)
	}
}

func decodeStringArrayWrap(pb *proto3.ListValue) {
	if _, err := decodeStringArray(pb); err != nil {
		panic(err)
	}
}

func decodeArrayStringDirect(pb *proto3.ListValue) {
	a := make([]string, len(pb.Values))
	t := stringType()
	for i, v := range pb.Values {
		if err := decodeValue(v, t, &a[i]); err != nil {
			panic(err)
		}
	}
}

func decodeArrayStringFunc(pb *proto3.ListValue) {
	a := make([]string, len(pb.Values))
	if err := decodeArrayFunc(pb, "STRING", stringType(), func(i int) interface{} { return &a[i] }); err != nil {
		panic(err)
	}
}

func decodeArrayStringReflect(pb *proto3.ListValue) {
	var a []string
	if err := decodeArrayReflect(pb, "STRING", stringType(), &a); err != nil {
		panic(err)
	}
}

func decodeArrayFunc(pb *proto3.ListValue, name string, typ *sppb.Type, elptr func(int) interface{}) error {
	if pb == nil {
		return errNilListValue(name)
	}
	for i, v := range pb.Values {
		if err := decodeValue(v, typ, elptr(i)); err != nil {
			return errDecodeArrayElement(i, v, name, err)
		}
	}
	return nil
}

func decodeArrayReflect(pb *proto3.ListValue, name string, typ *sppb.Type, aptr interface{}) error {
	if pb == nil {
		return errNilListValue(name)
	}
	av := reflect.ValueOf(aptr).Elem()
	av.Set(reflect.MakeSlice(av.Type(), len(pb.Values), len(pb.Values)))
	for i, v := range pb.Values {
		if err := decodeValue(v, typ, av.Index(i).Addr().Interface()); err != nil {
			av.Set(reflect.Zero(av.Type())) // reset slice to nil
			return errDecodeArrayElement(i, v, name, err)
		}
	}
	return nil
}
