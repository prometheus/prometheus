/*
Copyright 2017 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"math"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/golang/protobuf/proto"
	proto3 "github.com/golang/protobuf/ptypes/struct"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
)

var (
	t1 = mustParseTime("2016-11-15T15:04:05.999999999Z")
	// Boundaries
	t2 = mustParseTime("0000-01-01T00:00:00.000000000Z")
	t3 = mustParseTime("9999-12-31T23:59:59.999999999Z")
	// Local timezone
	t4 = time.Now()
	d1 = mustParseDate("2016-11-15")
	d2 = mustParseDate("1678-01-01")
)

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return t
}

func mustParseDate(s string) civil.Date {
	d, err := civil.ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

// Test encoding Values.
func TestEncodeValue(t *testing.T) {
	var (
		tString = stringType()
		tInt    = intType()
		tBool   = boolType()
		tFloat  = floatType()
		tBytes  = bytesType()
		tTime   = timeType()
		tDate   = dateType()
	)
	for i, test := range []struct {
		in       interface{}
		want     *proto3.Value
		wantType *sppb.Type
	}{
		// STRING / STRING ARRAY
		{"abc", stringProto("abc"), tString},
		{NullString{"abc", true}, stringProto("abc"), tString},
		{NullString{"abc", false}, nullProto(), tString},
		{[]string(nil), nullProto(), listType(tString)},
		{[]string{"abc", "bcd"}, listProto(stringProto("abc"), stringProto("bcd")), listType(tString)},
		{[]NullString{{"abcd", true}, {"xyz", false}}, listProto(stringProto("abcd"), nullProto()), listType(tString)},
		// BYTES / BYTES ARRAY
		{[]byte("foo"), bytesProto([]byte("foo")), tBytes},
		{[]byte(nil), nullProto(), tBytes},
		{[][]byte{nil, []byte("ab")}, listProto(nullProto(), bytesProto([]byte("ab"))), listType(tBytes)},
		{[][]byte(nil), nullProto(), listType(tBytes)},
		// INT64 / INT64 ARRAY
		{7, intProto(7), tInt},
		{[]int(nil), nullProto(), listType(tInt)},
		{[]int{31, 127}, listProto(intProto(31), intProto(127)), listType(tInt)},
		{int64(81), intProto(81), tInt},
		{[]int64(nil), nullProto(), listType(tInt)},
		{[]int64{33, 129}, listProto(intProto(33), intProto(129)), listType(tInt)},
		{NullInt64{11, true}, intProto(11), tInt},
		{NullInt64{11, false}, nullProto(), tInt},
		{[]NullInt64{{35, true}, {131, false}}, listProto(intProto(35), nullProto()), listType(tInt)},
		// BOOL / BOOL ARRAY
		{true, boolProto(true), tBool},
		{NullBool{true, true}, boolProto(true), tBool},
		{NullBool{true, false}, nullProto(), tBool},
		{[]bool{true, false}, listProto(boolProto(true), boolProto(false)), listType(tBool)},
		{[]NullBool{{true, true}, {true, false}}, listProto(boolProto(true), nullProto()), listType(tBool)},
		// FLOAT64 / FLOAT64 ARRAY
		{3.14, floatProto(3.14), tFloat},
		{NullFloat64{3.1415, true}, floatProto(3.1415), tFloat},
		{NullFloat64{math.Inf(1), true}, floatProto(math.Inf(1)), tFloat},
		{NullFloat64{3.14159, false}, nullProto(), tFloat},
		{[]float64(nil), nullProto(), listType(tFloat)},
		{[]float64{3.141, 0.618, math.Inf(-1)}, listProto(floatProto(3.141), floatProto(0.618), floatProto(math.Inf(-1))), listType(tFloat)},
		{[]NullFloat64{{3.141, true}, {0.618, false}}, listProto(floatProto(3.141), nullProto()), listType(tFloat)},
		// TIMESTAMP / TIMESTAMP ARRAY
		{t1, timeProto(t1), tTime},
		{NullTime{t1, true}, timeProto(t1), tTime},
		{NullTime{t1, false}, nullProto(), tTime},
		{[]time.Time(nil), nullProto(), listType(tTime)},
		{[]time.Time{t1, t2, t3, t4}, listProto(timeProto(t1), timeProto(t2), timeProto(t3), timeProto(t4)), listType(tTime)},
		{[]NullTime{{t1, true}, {t1, false}}, listProto(timeProto(t1), nullProto()), listType(tTime)},
		// DATE / DATE ARRAY
		{d1, dateProto(d1), tDate},
		{NullDate{d1, true}, dateProto(d1), tDate},
		{NullDate{civil.Date{}, false}, nullProto(), tDate},
		{[]civil.Date(nil), nullProto(), listType(tDate)},
		{[]civil.Date{d1, d2}, listProto(dateProto(d1), dateProto(d2)), listType(tDate)},
		{[]NullDate{{d1, true}, {civil.Date{}, false}}, listProto(dateProto(d1), nullProto()), listType(tDate)},
		// GenericColumnValue
		{GenericColumnValue{tString, stringProto("abc")}, stringProto("abc"), tString},
		{GenericColumnValue{tString, nullProto()}, nullProto(), tString},
		// not actually valid (stringProto inside int list), but demonstrates pass-through.
		{
			GenericColumnValue{
				Type:  listType(tInt),
				Value: listProto(intProto(5), nullProto(), stringProto("bcd")),
			},
			listProto(intProto(5), nullProto(), stringProto("bcd")),
			listType(tInt),
		},
		// placeholder
		{CommitTimestamp, stringProto(commitTimestampPlaceholderString), tTime},
	} {
		got, gotType, err := encodeValue(test.in)
		if err != nil {
			t.Fatalf("#%d: got error during encoding: %v, want nil", i, err)
		}
		if !testEqual(got, test.want) {
			t.Errorf("#%d: got encode result: %v, want %v", i, got, test.want)
		}
		if !testEqual(gotType, test.wantType) {
			t.Errorf("#%d: got encode type: %v, want %v", i, gotType, test.wantType)
		}
	}
}

type encodeTest struct {
	desc     string
	in       interface{}
	want     *proto3.Value
	wantType *sppb.Type
}

func checkStructEncoding(desc string, got *proto3.Value, gotType *sppb.Type,
	want *proto3.Value, wantType *sppb.Type, t *testing.T) {
	if !testEqual(got, want) {
		t.Errorf("Test %s: got encode result: %v, want %v", desc, got, want)
	}
	if !testEqual(gotType, wantType) {
		t.Errorf("Test %s: got encode type: %v, want %v", desc, gotType, wantType)
	}
}

// Testcase code
func encodeStructValue(test encodeTest, t *testing.T) {
	got, gotType, err := encodeValue(test.in)
	if err != nil {
		t.Fatalf("Test %s: got error during encoding: %v, want nil", test.desc, err)
	}
	checkStructEncoding(test.desc, got, gotType, test.want, test.wantType, t)
}

func TestEncodeStructValuePointers(t *testing.T) {
	type structf struct {
		F int `spanner:"ff2"`
	}
	nestedStructProto := structType(mkField("ff2", intType()))

	type testType struct {
		Stringf    string
		Structf    *structf
		ArrStructf []*structf
	}
	testTypeProto := structType(
		mkField("Stringf", stringType()),
		mkField("Structf", nestedStructProto),
		mkField("ArrStructf", listType(nestedStructProto)))

	for _, test := range []encodeTest{
		{
			"Pointer to Go struct with pointers-to-(array)-struct fields.",
			&testType{"hello", &structf{50}, []*structf{{30}, {40}}},
			listProto(
				stringProto("hello"),
				listProto(intProto(50)),
				listProto(
					listProto(intProto(30)),
					listProto(intProto(40)))),
			testTypeProto,
		},
		{
			"Nil pointer to Go struct representing a NULL struct value.",
			(*testType)(nil),
			nullProto(),
			testTypeProto,
		},
		{
			"Slice of pointers to Go structs with NULL and non-NULL elements.",
			[]*testType{
				(*testType)(nil),
				{"hello", nil, []*structf{nil, {40}}},
				{"world", &structf{70}, nil},
			},
			listProto(
				nullProto(),
				listProto(
					stringProto("hello"),
					nullProto(),
					listProto(nullProto(), listProto(intProto(40)))),
				listProto(
					stringProto("world"),
					listProto(intProto(70)),
					nullProto())),
			listType(testTypeProto),
		},
		{
			"Nil slice of pointers to structs representing a NULL array of structs.",
			[]*testType(nil),
			nullProto(),
			listType(testTypeProto),
		},
		{
			"Empty slice of pointers to structs representing an empty array of structs.",
			[]*testType{},
			listProto(),
			listType(testTypeProto),
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueErrors(t *testing.T) {
	type Embedded struct {
		A int
	}
	type embedded struct {
		B bool
	}
	x := 0

	for _, test := range []struct {
		desc    string
		in      interface{}
		wantErr error
	}{
		{
			"Unsupported embedded fields.",
			struct{ Embedded }{Embedded{10}},
			errUnsupportedEmbeddedStructFields("Embedded"),
		},
		{
			"Unsupported pointer to embedded fields.",
			struct{ *Embedded }{&Embedded{10}},
			errUnsupportedEmbeddedStructFields("Embedded"),
		},
		{
			"Unsupported embedded + unexported fields.",
			struct {
				int
				*bool
				embedded
			}{10, nil, embedded{false}},
			errUnsupportedEmbeddedStructFields("int"),
		},
		{
			"Unsupported type.",
			(**struct{})(nil),
			errEncoderUnsupportedType((**struct{})(nil)),
		},
		{
			"Unsupported type.",
			3,
			errEncoderUnsupportedType(3),
		},
		{
			"Unsupported type.",
			&x,
			errEncoderUnsupportedType(&x),
		},
	} {
		_, _, got := encodeStruct(test.in)
		if got == nil || !testEqual(test.wantErr, got) {
			t.Errorf("Test: %s, expected error %v during decoding, got %v", test.desc, test.wantErr, got)
		}
	}
}

func TestEncodeStructValueArrayStructFields(t *testing.T) {
	type structf struct {
		Intff int
	}

	structfType := structType(mkField("Intff", intType()))
	for _, test := range []encodeTest{
		{
			"Unnamed array-of-struct-typed field.",
			struct {
				Intf       int
				ArrStructf []structf `spanner:""`
			}{10, []structf{{1}, {2}}},
			listProto(
				intProto(10),
				listProto(
					listProto(intProto(1)),
					listProto(intProto(2)))),
			structType(
				mkField("Intf", intType()),
				mkField("", listType(structfType))),
		},
		{
			"Null array-of-struct-typed field.",
			struct {
				Intf       int
				ArrStructf []structf
			}{10, []structf(nil)},
			listProto(intProto(10), nullProto()),
			structType(
				mkField("Intf", intType()),
				mkField("ArrStructf", listType(structfType))),
		},
		{
			"Array-of-struct-typed field representing empty array.",
			struct {
				Intf       int
				ArrStructf []structf
			}{10, []structf{}},
			listProto(intProto(10), listProto([]*proto3.Value{}...)),
			structType(
				mkField("Intf", intType()),
				mkField("ArrStructf", listType(structfType))),
		},
		{
			"Array-of-struct-typed field with nullable struct elements.",
			struct {
				Intf       int
				ArrStructf []*structf
			}{
				10,
				[]*structf{(*structf)(nil), {1}},
			},
			listProto(
				intProto(10),
				listProto(
					nullProto(),
					listProto(intProto(1)))),
			structType(
				mkField("Intf", intType()),
				mkField("ArrStructf", listType(structfType))),
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueStructFields(t *testing.T) {
	type structf struct {
		Intff int
	}
	structfType := structType(mkField("Intff", intType()))
	for _, test := range []encodeTest{
		{
			"Named struct-type field.",
			struct {
				Intf    int
				Structf structf
			}{10, structf{10}},
			listProto(intProto(10), listProto(intProto(10))),
			structType(
				mkField("Intf", intType()),
				mkField("Structf", structfType)),
		},
		{
			"Unnamed struct-type field.",
			struct {
				Intf    int
				Structf structf `spanner:""`
			}{10, structf{10}},
			listProto(intProto(10), listProto(intProto(10))),
			structType(
				mkField("Intf", intType()),
				mkField("", structfType)),
		},
		{
			"Duplicate struct-typed field.",
			struct {
				Structf1 structf `spanner:""`
				Structf2 structf `spanner:""`
			}{structf{10}, structf{20}},
			listProto(listProto(intProto(10)), listProto(intProto(20))),
			structType(
				mkField("", structfType),
				mkField("", structfType)),
		},
		{
			"Null struct-typed field.",
			struct {
				Intf    int
				Structf *structf
			}{10, nil},
			listProto(intProto(10), nullProto()),
			structType(
				mkField("Intf", intType()),
				mkField("Structf", structfType)),
		},
		{
			"Empty struct-typed field.",
			struct {
				Intf    int
				Structf struct{}
			}{10, struct{}{}},
			listProto(intProto(10), listProto([]*proto3.Value{}...)),
			structType(
				mkField("Intf", intType()),
				mkField("Structf", structType([]*sppb.StructType_Field{}...))),
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueFieldNames(t *testing.T) {
	type embedded struct {
		B bool
	}

	for _, test := range []encodeTest{
		{
			"Duplicate fields.",
			struct {
				Field1    int `spanner:"field"`
				DupField1 int `spanner:"field"`
			}{10, 20},
			listProto(intProto(10), intProto(20)),
			structType(
				mkField("field", intType()),
				mkField("field", intType())),
		},
		{
			"Duplicate Fields (different types).",
			struct {
				IntField    int    `spanner:"field"`
				StringField string `spanner:"field"`
			}{10, "abc"},
			listProto(intProto(10), stringProto("abc")),
			structType(
				mkField("field", intType()),
				mkField("field", stringType())),
		},
		{
			"Duplicate unnamed fields.",
			struct {
				Dup  int `spanner:""`
				Dup1 int `spanner:""`
			}{10, 20},
			listProto(intProto(10), intProto(20)),
			structType(
				mkField("", intType()),
				mkField("", intType())),
		},
		{
			"Named and unnamed fields.",
			struct {
				Field  string
				Field1 int    `spanner:""`
				Field2 string `spanner:"field"`
			}{"abc", 10, "def"},
			listProto(stringProto("abc"), intProto(10), stringProto("def")),
			structType(
				mkField("Field", stringType()),
				mkField("", intType()),
				mkField("field", stringType())),
		},
		{
			"Ignored unexported fields.",
			struct {
				Field  int
				field  bool
				Field1 string `spanner:"field"`
			}{10, false, "abc"},
			listProto(intProto(10), stringProto("abc")),
			structType(
				mkField("Field", intType()),
				mkField("field", stringType())),
		},
		{
			"Ignored unexported struct/slice fields.",
			struct {
				a      []*embedded
				b      []embedded
				c      embedded
				d      *embedded
				Field1 string `spanner:"field"`
			}{nil, nil, embedded{}, nil, "def"},
			listProto(stringProto("def")),
			structType(
				mkField("field", stringType())),
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueBasicFields(t *testing.T) {
	StructTypeProto := structType(
		mkField("Stringf", stringType()),
		mkField("Intf", intType()),
		mkField("Boolf", boolType()),
		mkField("Floatf", floatType()),
		mkField("Bytef", bytesType()),
		mkField("Timef", timeType()),
		mkField("Datef", dateType()))

	for _, test := range []encodeTest{
		{
			"Basic types.",
			struct {
				Stringf string
				Intf    int
				Boolf   bool
				Floatf  float64
				Bytef   []byte
				Timef   time.Time
				Datef   civil.Date
			}{"abc", 300, false, 3.45, []byte("foo"), t1, d1},
			listProto(
				stringProto("abc"),
				intProto(300),
				boolProto(false),
				floatProto(3.45),
				bytesProto([]byte("foo")),
				timeProto(t1),
				dateProto(d1)),
			StructTypeProto,
		},
		{
			"Basic types null values.",
			struct {
				Stringf NullString
				Intf    NullInt64
				Boolf   NullBool
				Floatf  NullFloat64
				Bytef   []byte
				Timef   NullTime
				Datef   NullDate
			}{
				NullString{"abc", false},
				NullInt64{4, false},
				NullBool{false, false},
				NullFloat64{5.6, false},
				nil,
				NullTime{t1, false},
				NullDate{d1, false},
			},
			listProto(
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto()),
			StructTypeProto,
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueArrayFields(t *testing.T) {
	StructTypeProto := structType(
		mkField("Stringf", listType(stringType())),
		mkField("Intf", listType(intType())),
		mkField("Int64f", listType(intType())),
		mkField("Boolf", listType(boolType())),
		mkField("Floatf", listType(floatType())),
		mkField("Bytef", listType(bytesType())),
		mkField("Timef", listType(timeType())),
		mkField("Datef", listType(dateType())))

	for _, test := range []encodeTest{
		{
			"Arrays of basic types with non-nullable elements",
			struct {
				Stringf []string
				Intf    []int
				Int64f  []int64
				Boolf   []bool
				Floatf  []float64
				Bytef   [][]byte
				Timef   []time.Time
				Datef   []civil.Date
			}{
				[]string{"abc", "def"},
				[]int{4, 67},
				[]int64{5, 68},
				[]bool{false, true},
				[]float64{3.45, 0.93},
				[][]byte{[]byte("foo"), nil},
				[]time.Time{t1, t2},
				[]civil.Date{d1, d2},
			},
			listProto(
				listProto(stringProto("abc"), stringProto("def")),
				listProto(intProto(4), intProto(67)),
				listProto(intProto(5), intProto(68)),
				listProto(boolProto(false), boolProto(true)),
				listProto(floatProto(3.45), floatProto(0.93)),
				listProto(bytesProto([]byte("foo")), nullProto()),
				listProto(timeProto(t1), timeProto(t2)),
				listProto(dateProto(d1), dateProto(d2))),
			StructTypeProto,
		},
		{
			"Arrays of basic types with nullable elements.",
			struct {
				Stringf []NullString
				Intf    []NullInt64
				Int64f  []NullInt64
				Boolf   []NullBool
				Floatf  []NullFloat64
				Bytef   [][]byte
				Timef   []NullTime
				Datef   []NullDate
			}{
				[]NullString{{"abc", false}, {"def", true}},
				[]NullInt64{{4, false}, {67, true}},
				[]NullInt64{{5, false}, {68, true}},
				[]NullBool{{true, false}, {false, true}},
				[]NullFloat64{{3.45, false}, {0.93, true}},
				[][]byte{[]byte("foo"), nil},
				[]NullTime{{t1, false}, {t2, true}},
				[]NullDate{{d1, false}, {d2, true}},
			},
			listProto(
				listProto(nullProto(), stringProto("def")),
				listProto(nullProto(), intProto(67)),
				listProto(nullProto(), intProto(68)),
				listProto(nullProto(), boolProto(false)),
				listProto(nullProto(), floatProto(0.93)),
				listProto(bytesProto([]byte("foo")), nullProto()),
				listProto(nullProto(), timeProto(t2)),
				listProto(nullProto(), dateProto(d2))),
			StructTypeProto,
		},
		{
			"Null arrays of basic types.",
			struct {
				Stringf []NullString
				Intf    []NullInt64
				Int64f  []NullInt64
				Boolf   []NullBool
				Floatf  []NullFloat64
				Bytef   [][]byte
				Timef   []NullTime
				Datef   []NullDate
			}{
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
			},
			listProto(
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto(),
				nullProto()),
			StructTypeProto,
		},
	} {
		encodeStructValue(test, t)
	}
}

// Test decoding Values.
func TestDecodeValue(t *testing.T) {
	for i, test := range []struct {
		in   *proto3.Value
		t    *sppb.Type
		want interface{}
		fail bool
	}{
		// STRING
		{stringProto("abc"), stringType(), "abc", false},
		{nullProto(), stringType(), "abc", true},
		{stringProto("abc"), stringType(), NullString{"abc", true}, false},
		{nullProto(), stringType(), NullString{}, false},
		// STRING ARRAY with []NullString
		{
			listProto(stringProto("abc"), nullProto(), stringProto("bcd")),
			listType(stringType()),
			[]NullString{{"abc", true}, {}, {"bcd", true}},
			false,
		},
		{nullProto(), listType(stringType()), []NullString(nil), false},
		// STRING ARRAY with []string
		{
			listProto(stringProto("abc"), stringProto("bcd")),
			listType(stringType()),
			[]string{"abc", "bcd"},
			false,
		},
		// BYTES
		{bytesProto([]byte("ab")), bytesType(), []byte("ab"), false},
		{nullProto(), bytesType(), []byte(nil), false},
		// BYTES ARRAY
		{listProto(bytesProto([]byte("ab")), nullProto()), listType(bytesType()), [][]byte{[]byte("ab"), nil}, false},
		{nullProto(), listType(bytesType()), [][]byte(nil), false},
		//INT64
		{intProto(15), intType(), int64(15), false},
		{nullProto(), intType(), int64(0), true},
		{intProto(15), intType(), NullInt64{15, true}, false},
		{nullProto(), intType(), NullInt64{}, false},
		// INT64 ARRAY with []NullInt64
		{listProto(intProto(91), nullProto(), intProto(87)), listType(intType()), []NullInt64{{91, true}, {}, {87, true}}, false},
		{nullProto(), listType(intType()), []NullInt64(nil), false},
		// INT64 ARRAY with []int64
		{listProto(intProto(91), intProto(87)), listType(intType()), []int64{91, 87}, false},
		// BOOL
		{boolProto(true), boolType(), true, false},
		{nullProto(), boolType(), true, true},
		{boolProto(true), boolType(), NullBool{true, true}, false},
		{nullProto(), boolType(), NullBool{}, false},
		// BOOL ARRAY with []NullBool
		{listProto(boolProto(true), boolProto(false), nullProto()), listType(boolType()), []NullBool{{true, true}, {false, true}, {}}, false},
		{nullProto(), listType(boolType()), []NullBool(nil), false},
		// BOOL ARRAY with []bool
		{listProto(boolProto(true), boolProto(false)), listType(boolType()), []bool{true, false}, false},
		// FLOAT64
		{floatProto(3.14), floatType(), 3.14, false},
		{nullProto(), floatType(), 0.00, true},
		{floatProto(3.14), floatType(), NullFloat64{3.14, true}, false},
		{nullProto(), floatType(), NullFloat64{}, false},
		// FLOAT64 ARRAY with []NullFloat64
		{
			listProto(floatProto(math.Inf(1)), floatProto(math.Inf(-1)), nullProto(), floatProto(3.1)),
			listType(floatType()),
			[]NullFloat64{{math.Inf(1), true}, {math.Inf(-1), true}, {}, {3.1, true}},
			false,
		},
		{nullProto(), listType(floatType()), []NullFloat64(nil), false},
		// FLOAT64 ARRAY with []float64
		{
			listProto(floatProto(math.Inf(1)), floatProto(math.Inf(-1)), floatProto(3.1)),
			listType(floatType()),
			[]float64{math.Inf(1), math.Inf(-1), 3.1},
			false,
		},
		// TIMESTAMP
		{timeProto(t1), timeType(), t1, false},
		{timeProto(t1), timeType(), NullTime{t1, true}, false},
		{nullProto(), timeType(), NullTime{}, false},
		// TIMESTAMP ARRAY with []NullTime
		{listProto(timeProto(t1), timeProto(t2), timeProto(t3), nullProto()), listType(timeType()), []NullTime{{t1, true}, {t2, true}, {t3, true}, {}}, false},
		{nullProto(), listType(timeType()), []NullTime(nil), false},
		// TIMESTAMP ARRAY with []time.Time
		{listProto(timeProto(t1), timeProto(t2), timeProto(t3)), listType(timeType()), []time.Time{t1, t2, t3}, false},
		// DATE
		{dateProto(d1), dateType(), d1, false},
		{dateProto(d1), dateType(), NullDate{d1, true}, false},
		{nullProto(), dateType(), NullDate{}, false},
		// DATE ARRAY with []NullDate
		{listProto(dateProto(d1), dateProto(d2), nullProto()), listType(dateType()), []NullDate{{d1, true}, {d2, true}, {}}, false},
		{nullProto(), listType(dateType()), []NullDate(nil), false},
		// DATE ARRAY with []civil.Date
		{listProto(dateProto(d1), dateProto(d2)), listType(dateType()), []civil.Date{d1, d2}, false},
		// STRUCT ARRAY
		// STRUCT schema is equal to the following Go struct:
		// type s struct {
		//     Col1 NullInt64
		//     Col2 []struct {
		//         SubCol1 float64
		//         SubCol2 string
		//     }
		// }
		{
			in: listProto(
				listProto(
					intProto(3),
					listProto(
						listProto(floatProto(3.14), stringProto("this")),
						listProto(floatProto(0.57), stringProto("siht")),
					),
				),
				listProto(
					nullProto(),
					nullProto(),
				),
				nullProto(),
			),
			t: listType(
				structType(
					mkField("Col1", intType()),
					mkField(
						"Col2",
						listType(
							structType(
								mkField("SubCol1", floatType()),
								mkField("SubCol2", stringType()),
							),
						),
					),
				),
			),
			want: []NullRow{
				{
					Row: Row{
						fields: []*sppb.StructType_Field{
							mkField("Col1", intType()),
							mkField(
								"Col2",
								listType(
									structType(
										mkField("SubCol1", floatType()),
										mkField("SubCol2", stringType()),
									),
								),
							),
						},
						vals: []*proto3.Value{
							intProto(3),
							listProto(
								listProto(floatProto(3.14), stringProto("this")),
								listProto(floatProto(0.57), stringProto("siht")),
							),
						},
					},
					Valid: true,
				},
				{
					Row: Row{
						fields: []*sppb.StructType_Field{
							mkField("Col1", intType()),
							mkField(
								"Col2",
								listType(
									structType(
										mkField("SubCol1", floatType()),
										mkField("SubCol2", stringType()),
									),
								),
							),
						},
						vals: []*proto3.Value{
							nullProto(),
							nullProto(),
						},
					},
					Valid: true,
				},
				{},
			},
			fail: false,
		},
		{
			in: listProto(
				listProto(
					intProto(3),
					listProto(
						listProto(floatProto(3.14), stringProto("this")),
						listProto(floatProto(0.57), stringProto("siht")),
					),
				),
				listProto(
					nullProto(),
					nullProto(),
				),
				nullProto(),
			),
			t: listType(
				structType(
					mkField("Col1", intType()),
					mkField(
						"Col2",
						listType(
							structType(
								mkField("SubCol1", floatType()),
								mkField("SubCol2", stringType()),
							),
						),
					),
				),
			),
			want: []*struct {
				Col1      NullInt64
				StructCol []*struct {
					SubCol1 NullFloat64
					SubCol2 string
				} `spanner:"Col2"`
			}{
				{
					Col1: NullInt64{3, true},
					StructCol: []*struct {
						SubCol1 NullFloat64
						SubCol2 string
					}{
						{
							SubCol1: NullFloat64{3.14, true},
							SubCol2: "this",
						},
						{
							SubCol1: NullFloat64{0.57, true},
							SubCol2: "siht",
						},
					},
				},
				{
					Col1: NullInt64{},
					StructCol: []*struct {
						SubCol1 NullFloat64
						SubCol2 string
					}(nil),
				},
				nil,
			},
			fail: false,
		},
		// GenericColumnValue
		{stringProto("abc"), stringType(), GenericColumnValue{stringType(), stringProto("abc")}, false},
		{nullProto(), stringType(), GenericColumnValue{stringType(), nullProto()}, false},
		// not actually valid (stringProto inside int list), but demonstrates pass-through.
		{
			in: listProto(intProto(5), nullProto(), stringProto("bcd")),
			t:  listType(intType()),
			want: GenericColumnValue{
				Type:  listType(intType()),
				Value: listProto(intProto(5), nullProto(), stringProto("bcd")),
			},
			fail: false,
		},
	} {
		gotp := reflect.New(reflect.TypeOf(test.want))
		if err := decodeValue(test.in, test.t, gotp.Interface()); err != nil {
			if !test.fail {
				t.Errorf("%d: cannot decode %v(%v): %v", i, test.in, test.t, err)
			}
			continue
		}
		if test.fail {
			t.Errorf("%d: decoding %v(%v) succeeds unexpectedly, want error", i, test.in, test.t)
			continue
		}
		got := reflect.Indirect(gotp).Interface()
		if !testEqual(got, test.want) {
			t.Errorf("%d: unexpected decoding result - got %v, want %v", i, got, test.want)
			continue
		}
	}
}

// Test error cases for decodeValue.
func TestDecodeValueErrors(t *testing.T) {
	var s string
	for i, test := range []struct {
		in *proto3.Value
		t  *sppb.Type
		v  interface{}
	}{
		{nullProto(), stringType(), nil},
		{nullProto(), stringType(), 1},
		{timeProto(t1), timeType(), &s},
	} {
		err := decodeValue(test.in, test.t, test.v)
		if err == nil {
			t.Errorf("#%d: want error, got nil", i)
		}
	}
}

// Test NaN encoding/decoding.
func TestNaN(t *testing.T) {
	// Decode NaN value.
	f := 0.0
	nf := NullFloat64{}
	// To float64
	if err := decodeValue(floatProto(math.NaN()), floatType(), &f); err != nil {
		t.Errorf("decodeValue returns %q for %v, want nil", err, floatProto(math.NaN()))
	}
	if !math.IsNaN(f) {
		t.Errorf("f = %v, want %v", f, math.NaN())
	}
	// To NullFloat64
	if err := decodeValue(floatProto(math.NaN()), floatType(), &nf); err != nil {
		t.Errorf("decodeValue returns %q for %v, want nil", err, floatProto(math.NaN()))
	}
	if !math.IsNaN(nf.Float64) || !nf.Valid {
		t.Errorf("f = %v, want %v", f, NullFloat64{math.NaN(), true})
	}
	// Encode NaN value
	// From float64
	v, _, err := encodeValue(math.NaN())
	if err != nil {
		t.Errorf("encodeValue returns %q for NaN, want nil", err)
	}
	x, ok := v.GetKind().(*proto3.Value_NumberValue)
	if !ok {
		t.Errorf("incorrect type for v.GetKind(): %T, want *proto3.Value_NumberValue", v.GetKind())
	}
	if !math.IsNaN(x.NumberValue) {
		t.Errorf("x.NumberValue = %v, want %v", x.NumberValue, math.NaN())
	}
	// From NullFloat64
	v, _, err = encodeValue(NullFloat64{math.NaN(), true})
	if err != nil {
		t.Errorf("encodeValue returns %q for NaN, want nil", err)
	}
	x, ok = v.GetKind().(*proto3.Value_NumberValue)
	if !ok {
		t.Errorf("incorrect type for v.GetKind(): %T, want *proto3.Value_NumberValue", v.GetKind())
	}
	if !math.IsNaN(x.NumberValue) {
		t.Errorf("x.NumberValue = %v, want %v", x.NumberValue, math.NaN())
	}
}

func TestGenericColumnValue(t *testing.T) {
	for _, test := range []struct {
		in   GenericColumnValue
		want interface{}
		fail bool
	}{
		{GenericColumnValue{stringType(), stringProto("abc")}, "abc", false},
		{GenericColumnValue{stringType(), stringProto("abc")}, 5, true},
		{GenericColumnValue{listType(intType()), listProto(intProto(91), nullProto(), intProto(87))}, []NullInt64{{91, true}, {}, {87, true}}, false},
		{GenericColumnValue{intType(), intProto(42)}, GenericColumnValue{intType(), intProto(42)}, false}, // trippy! :-)
	} {
		gotp := reflect.New(reflect.TypeOf(test.want))
		if err := test.in.Decode(gotp.Interface()); err != nil {
			if !test.fail {
				t.Errorf("cannot decode %v to %v: %v", test.in, test.want, err)
			}
			continue
		}
		if test.fail {
			t.Errorf("decoding %v to %v succeeds unexpectedly", test.in, test.want)
		}

		// Test we can go backwards as well.
		v, err := newGenericColumnValue(test.want)
		if err != nil {
			t.Errorf("NewGenericColumnValue failed: %v", err)
			continue
		}
		if !testEqual(*v, test.in) {
			t.Errorf("unexpected encode result - got %v, want %v", v, test.in)
		}
	}
}

func TestDecodeStruct(t *testing.T) {
	stype := &sppb.StructType{Fields: []*sppb.StructType_Field{
		{Name: "Id", Type: stringType()},
		{Name: "Time", Type: timeType()},
	}}
	lv := listValueProto(stringProto("id"), timeProto(t1))

	type (
		S1 struct {
			ID   string
			Time time.Time
		}
		S2 struct {
			ID   string
			Time string
		}
	)
	var (
		s1 S1
		s2 S2
	)

	for i, test := range []struct {
		ptr  interface{}
		want interface{}
		fail bool
	}{
		{
			ptr:  &s1,
			want: &S1{ID: "id", Time: t1},
		},
		{
			ptr:  &s2,
			fail: true,
		},
	} {
		err := decodeStruct(stype, lv, test.ptr)
		if (err != nil) != test.fail {
			t.Errorf("#%d: got error %v, wanted fail: %v", i, err, test.fail)
		}
		if err == nil && !testEqual(test.ptr, test.want) {
			t.Errorf("#%d: got %+v, want %+v", i, test.ptr, test.want)
		}
	}
}

func TestEncodeStructValueDynamicStructs(t *testing.T) {
	dynStructType := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf(0), Tag: `spanner:"a"`},
		{Name: "B", Type: reflect.TypeOf(""), Tag: `spanner:"b"`},
	})
	dynNullableStructType := reflect.PtrTo(dynStructType)
	dynStructArrType := reflect.SliceOf(dynStructType)
	dynNullableStructArrType := reflect.SliceOf(dynNullableStructType)

	dynStructValue := reflect.New(dynStructType)
	dynStructValue.Elem().Field(0).SetInt(10)
	dynStructValue.Elem().Field(1).SetString("abc")

	dynStructArrValue := reflect.MakeSlice(dynNullableStructArrType, 2, 2)
	dynStructArrValue.Index(0).Set(reflect.Zero(dynNullableStructType))
	dynStructArrValue.Index(1).Set(dynStructValue)

	structProtoType := structType(
		mkField("a", intType()),
		mkField("b", stringType()))

	arrProtoType := listType(structProtoType)

	for _, test := range []encodeTest{
		{
			"Dynanic non-NULL struct value.",
			dynStructValue.Elem().Interface(),
			listProto(intProto(10), stringProto("abc")),
			structProtoType,
		},
		{
			"Dynanic NULL struct value.",
			reflect.Zero(dynNullableStructType).Interface(),
			nullProto(),
			structProtoType,
		},
		{
			"Empty array of dynamic structs.",
			reflect.MakeSlice(dynStructArrType, 0, 0).Interface(),
			listProto([]*proto3.Value{}...),
			arrProtoType,
		},
		{
			"NULL array of non-NULL-able dynamic structs.",
			reflect.Zero(dynStructArrType).Interface(),
			nullProto(),
			arrProtoType,
		},
		{
			"NULL array of NULL-able(nil) dynamic structs.",
			reflect.Zero(dynNullableStructArrType).Interface(),
			nullProto(),
			arrProtoType,
		},
		{
			"Array containing NULL(nil) dynamic-typed struct elements.",
			dynStructArrValue.Interface(),
			listProto(
				nullProto(),
				listProto(intProto(10), stringProto("abc"))),
			arrProtoType,
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueEmptyStruct(t *testing.T) {
	emptyListValue := listProto([]*proto3.Value{}...)
	emptyStructType := structType([]*sppb.StructType_Field{}...)
	emptyStruct := struct{}{}
	nullEmptyStruct := (*struct{})(nil)

	dynamicEmptyStructType := reflect.StructOf(make([]reflect.StructField, 0, 0))
	dynamicStructArrType := reflect.SliceOf(reflect.PtrTo((dynamicEmptyStructType)))

	dynamicEmptyStruct := reflect.New(dynamicEmptyStructType)
	dynamicNullEmptyStruct := reflect.Zero(reflect.PtrTo(dynamicEmptyStructType))

	dynamicStructArrValue := reflect.MakeSlice(dynamicStructArrType, 2, 2)
	dynamicStructArrValue.Index(0).Set(dynamicNullEmptyStruct)
	dynamicStructArrValue.Index(1).Set(dynamicEmptyStruct)

	for _, test := range []encodeTest{
		{
			"Go empty struct.",
			emptyStruct,
			emptyListValue,
			emptyStructType,
		},
		{
			"Dynamic empty struct.",
			dynamicEmptyStruct.Interface(),
			emptyListValue,
			emptyStructType,
		},
		{
			"Go NULL empty struct.",
			nullEmptyStruct,
			nullProto(),
			emptyStructType,
		},
		{
			"Dynamic NULL empty struct.",
			dynamicNullEmptyStruct.Interface(),
			nullProto(),
			emptyStructType,
		},
		{
			"Non-empty array of dynamic NULL and non-NULL empty structs.",
			dynamicStructArrValue.Interface(),
			listProto(nullProto(), emptyListValue),
			listType(emptyStructType),
		},
		{
			"Non-empty array of nullable empty structs.",
			[]*struct{}{nullEmptyStruct, &emptyStruct},
			listProto(nullProto(), emptyListValue),
			listType(emptyStructType),
		},
		{
			"Empty array of empty struct.",
			[]struct{}{},
			emptyListValue,
			listType(emptyStructType),
		},
		{
			"Null array of empty structs.",
			[]struct{}(nil),
			nullProto(),
			listType(emptyStructType),
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestEncodeStructValueMixedStructTypes(t *testing.T) {
	type staticStruct struct {
		F int `spanner:"fStatic"`
	}
	s1 := staticStruct{10}
	s2 := (*staticStruct)(nil)

	var f float64
	dynStructType := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf(f), Tag: `spanner:"fDynamic"`},
	})
	s3 := reflect.New(dynStructType)
	s3.Elem().Field(0).SetFloat(3.14)

	for _, test := range []encodeTest{
		{
			"'struct' with static and dynamic *struct, []*struct, []struct fields",
			struct {
				A []staticStruct
				B []*staticStruct
				C interface{}
			}{
				[]staticStruct{s1, s1},
				[]*staticStruct{&s1, s2},
				s3.Interface(),
			},
			listProto(
				listProto(listProto(intProto(10)), listProto(intProto(10))),
				listProto(listProto(intProto(10)), nullProto()),
				listProto(floatProto(3.14))),
			structType(
				mkField("A", listType(structType(mkField("fStatic", intType())))),
				mkField("B", listType(structType(mkField("fStatic", intType())))),
				mkField("C", structType(mkField("fDynamic", floatType())))),
		},
	} {
		encodeStructValue(test, t)
	}
}

func TestBindParamsDynamic(t *testing.T) {
	// Verify Statement.bindParams generates correct values and types.
	st := Statement{
		SQL:    "SELECT id from t_foo WHERE col = @var",
		Params: map[string]interface{}{"var": nil},
	}
	want := &sppb.ExecuteSqlRequest{
		Params: &proto3.Struct{
			Fields: map[string]*proto3.Value{"var": nil},
		},
		ParamTypes: map[string]*sppb.Type{"var": nil},
	}
	var (
		t1, _ = time.Parse(time.RFC3339Nano, "2016-11-15T15:04:05.999999999Z")
		// Boundaries
		t2, _ = time.Parse(time.RFC3339Nano, "0001-01-01T00:00:00.000000000Z")
	)
	dynamicStructType := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf(t1), Tag: `spanner:"field"`},
		{Name: "B", Type: reflect.TypeOf(3.14), Tag: `spanner:""`},
	})
	dynamicStructArrType := reflect.SliceOf(reflect.PtrTo(dynamicStructType))
	dynamicEmptyStructType := reflect.StructOf(make([]reflect.StructField, 0, 0))

	dynamicStructTypeProto := structType(
		mkField("field", timeType()),
		mkField("", floatType()))

	s3 := reflect.New(dynamicStructType)
	s3.Elem().Field(0).Set(reflect.ValueOf(t1))
	s3.Elem().Field(1).SetFloat(1.4)

	s4 := reflect.New(dynamicStructType)
	s4.Elem().Field(0).Set(reflect.ValueOf(t2))
	s4.Elem().Field(1).SetFloat(-13.3)

	dynamicStructArrayVal := reflect.MakeSlice(dynamicStructArrType, 2, 2)
	dynamicStructArrayVal.Index(0).Set(s3)
	dynamicStructArrayVal.Index(1).Set(s4)

	for _, test := range []struct {
		val       interface{}
		wantField *proto3.Value
		wantType  *sppb.Type
	}{
		{
			s3.Interface(),
			listProto(timeProto(t1), floatProto(1.4)),
			structType(
				mkField("field", timeType()),
				mkField("", floatType())),
		},
		{
			reflect.Zero(reflect.PtrTo(dynamicEmptyStructType)).Interface(),
			nullProto(),
			structType([]*sppb.StructType_Field{}...),
		},
		{
			dynamicStructArrayVal.Interface(),
			listProto(
				listProto(timeProto(t1), floatProto(1.4)),
				listProto(timeProto(t2), floatProto(-13.3))),
			listType(dynamicStructTypeProto),
		},
		{
			[]*struct {
				F1 time.Time `spanner:"field"`
				F2 float64   `spanner:""`
			}{
				nil,
				{t1, 1.4},
			},
			listProto(
				nullProto(),
				listProto(timeProto(t1), floatProto(1.4))),
			listType(dynamicStructTypeProto),
		},
	} {
		st.Params["var"] = test.val
		want.Params.Fields["var"] = test.wantField
		want.ParamTypes["var"] = test.wantType
		gotParams, gotParamTypes, gotErr := st.convertParams()
		if gotErr != nil {
			t.Error(gotErr)
			continue
		}
		gotParamField := gotParams.Fields["var"]
		if !proto.Equal(gotParamField, test.wantField) {
			// handle NaN
			if test.wantType.Code == floatType().Code && proto.MarshalTextString(gotParamField) == proto.MarshalTextString(test.wantField) {
				continue
			}
			t.Errorf("%#v: got %v, want %v\n", test.val, gotParamField, test.wantField)
		}
		gotParamType := gotParamTypes["var"]
		if !proto.Equal(gotParamType, test.wantType) {
			t.Errorf("%#v: got %v, want %v\n", test.val, gotParamType, test.wantField)
		}
	}
}
