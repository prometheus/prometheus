// Copyright 2017 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package firestore

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	ts "github.com/golang/protobuf/ptypes/timestamp"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/genproto/googleapis/type/latlng"
)

var (
	tm  = time.Date(2016, 12, 25, 0, 0, 0, 123456789, time.UTC)
	ll  = &latlng.LatLng{Latitude: 20, Longitude: 30}
	ptm = &ts.Timestamp{Seconds: 12345, Nanos: 67890}
)

func TestCreateFromProtoValue(t *testing.T) {
	for _, test := range []struct {
		in   *pb.Value
		want interface{}
	}{
		{in: nullValue, want: nil},
		{in: boolval(true), want: true},
		{in: intval(3), want: int64(3)},
		{in: floatval(1.5), want: 1.5},
		{in: strval("str"), want: "str"},
		{in: tsval(tm), want: tm},
		{
			in:   bytesval([]byte{1, 2}),
			want: []byte{1, 2},
		},
		{
			in:   &pb.Value{ValueType: &pb.Value_GeoPointValue{ll}},
			want: ll,
		},
		{
			in:   arrayval(intval(1), intval(2)),
			want: []interface{}{int64(1), int64(2)},
		},
		{
			in:   arrayval(),
			want: []interface{}{},
		},
		{
			in:   mapval(map[string]*pb.Value{"a": intval(1), "b": intval(2)}),
			want: map[string]interface{}{"a": int64(1), "b": int64(2)},
		},
		{
			in:   mapval(map[string]*pb.Value{}),
			want: map[string]interface{}{},
		},
		{
			in: refval("projects/P/databases/D/documents/c/d"),
			want: &DocumentRef{
				ID:        "d",
				Path:      "projects/P/databases/D/documents/c/d",
				shortPath: "c/d",
				Parent: &CollectionRef{
					ID:         "c",
					parentPath: "projects/P/databases/D/documents",
					selfPath:   "c",
					Path:       "projects/P/databases/D/documents/c",
					Query: Query{
						collectionID: "c",
						parentPath:   "projects/P/databases/D/documents",
						path:         "projects/P/databases/D/documents/c",
					},
				},
			},
		},
	} {
		got, err := createFromProtoValue(test.in, nil)
		if err != nil {
			t.Errorf("%+v: %+v", test.in, err)
			continue
		}
		if !testEqual(got, test.want) {
			t.Errorf("%+v:\ngot\n%#v\nwant\n%#v", test.in, got, test.want)
		}
	}
}

func TestSetFromProtoValue(t *testing.T) {
	testSetFromProtoValue(t, "json", jsonTester{})
	testSetFromProtoValue(t, "firestore", protoTester{})
}

func testSetFromProtoValue(t *testing.T, prefix string, r tester) {
	pi := newfloat(7)
	s := []float64{7, 8}
	ar1 := [1]float64{7}
	ar2 := [2]float64{7, 8}
	ar3 := [3]float64{7, 8, 9}
	mf := map[string]float64{"a": 7}

	type T struct {
		I **float64
		J float64
	}

	one := newfloat(1)
	six := newfloat(6)
	st := []*T{{I: &six}, nil, {I: &six, J: 7}}
	vs := interface{}(T{J: 1})
	vm := interface{}(map[string]float64{"i": 1})
	var (
		i   int
		i8  int8
		i16 int16
		i32 int32
		i64 int64
		u8  uint8
		u16 uint16
		u32 uint32
		b   bool
		ll  *latlng.LatLng
		mi  map[string]interface{}
		ms  map[string]T
	)

	for i, test := range []struct {
		in   interface{}
		val  interface{}
		want interface{}
	}{
		{&pi, r.Null(), (*float64)(nil)},
		{pi, r.Float(1), 1.0},
		{&s, r.Null(), ([]float64)(nil)},
		{&s, r.Array(r.Float(1), r.Float(2)), []float64{1, 2}},
		{&ar1, r.Array(r.Float(1), r.Float(2)), [1]float64{1}},
		{&ar2, r.Array(r.Float(1), r.Float(2)), [2]float64{1, 2}},
		{&ar3, r.Array(r.Float(1), r.Float(2)), [3]float64{1, 2, 0}},
		{&mf, r.Null(), (map[string]float64)(nil)},
		{&mf, r.Map("a", r.Float(1), "b", r.Float(2)), map[string]float64{"a": 1, "b": 2}},
		{&st, r.Array(
			r.Null(),               // overwrites st[0] with nil
			r.Map("i", r.Float(1)), // sets st[1] to a new struct
			r.Map("i", r.Float(2)), // modifies st[2]
		),
			[]*T{nil, {I: &one}, {I: &six, J: 7}}},
		{&mi, r.Map("a", r.Float(1), "b", r.Float(2)), map[string]interface{}{"a": 1.0, "b": 2.0}},
		{&ms, r.Map("a", r.Map("j", r.Float(1))), map[string]T{"a": {J: 1}}},
		{&vs, r.Map("i", r.Float(2)), map[string]interface{}{"i": 2.0}},
		{&vm, r.Map("i", r.Float(2)), map[string]interface{}{"i": 2.0}},
		{&ll, r.Null(), (*latlng.LatLng)(nil)},
		{&i, r.Int(1), int(1)},
		{&i8, r.Int(1), int8(1)},
		{&i16, r.Int(1), int16(1)},
		{&i32, r.Int(1), int32(1)},
		{&i64, r.Int(1), int64(1)},
		{&u8, r.Int(1), uint8(1)},
		{&u16, r.Int(1), uint16(1)},
		{&u32, r.Int(1), uint32(1)},
		{&b, r.Bool(true), true},
		{&i, r.Float(1), int(1)},   // can put a float with no fractional part into an int
		{pi, r.Int(1), float64(1)}, // can put an int into a float
	} {
		if err := r.Set(test.in, test.val); err != nil {
			t.Errorf("%s: #%d: got error %v", prefix, i, err)
			continue
		}
		got := reflect.ValueOf(test.in).Elem().Interface()
		if !testEqual(got, test.want) {
			t.Errorf("%s: #%d, %v:\ngot\n%+v (%T)\nwant\n%+v (%T)",
				prefix, i, test.val, got, got, test.want, test.want)
		}
	}
}

func TestSetFromProtoValueNoJSON(t *testing.T) {
	// Test code paths that we cannot compare to JSON.
	var (
		bs  []byte
		tmi time.Time
		lli *latlng.LatLng
		tmp *ts.Timestamp
	)
	bytes := []byte{1, 2, 3}

	for i, test := range []struct {
		in   interface{}
		val  *pb.Value
		want interface{}
	}{
		{&bs, bytesval(bytes), bytes},
		{&tmi, tsval(tm), tm},
		{&tmp, &pb.Value{ValueType: &pb.Value_TimestampValue{ptm}}, ptm},
		{&lli, geoval(ll), ll},
	} {
		if err := setFromProtoValue(test.in, test.val, &Client{}); err != nil {
			t.Errorf("#%d: got error %v", i, err)
			continue
		}
		got := reflect.ValueOf(test.in).Elem().Interface()
		if !testEqual(got, test.want) {
			t.Errorf("#%d, %v:\ngot\n%+v (%T)\nwant\n%+v (%T)",
				i, test.val, got, got, test.want, test.want)
		}
	}
}

func TestSetFromProtoValueErrors(t *testing.T) {
	c := &Client{}
	ival := intval(3)
	for i, test := range []struct {
		in  interface{}
		val *pb.Value
	}{
		{3, ival},                 // not a pointer
		{new(int8), intval(128)},  // int overflow
		{new(uint8), intval(256)}, // uint overflow
		{new(float32), floatval(2 * math.MaxFloat32)}, // float overflow
		{new(uint), ival},      // cannot set type
		{new(uint64), ival},    // cannot set type
		{new(io.Reader), ival}, // cannot set type
		{new(map[int]int),
			mapval(map[string]*pb.Value{"x": ival})}, // map key type is not string
		// the rest are all type mismatches
		{new(bool), ival},
		{new(*latlng.LatLng), ival},
		{new(time.Time), ival},
		{new(string), ival},
		{new([]byte), ival},
		{new([]int), ival},
		{new([1]int), ival},
		{new(map[string]int), ival},
		{new(*bool), ival},
		{new(struct{}), ival},
		{new(int), floatval(2.5)},                // float must be integral
		{new(uint16), intval(-1)},                // uint cannot be negative
		{new(int16), floatval(math.MaxFloat32)},  // doesn't fit
		{new(uint16), floatval(math.MaxFloat32)}, // doesn't fit
		{new(float32),
			&pb.Value{ValueType: &pb.Value_IntegerValue{math.MaxInt64}}}, // overflow
	} {
		err := setFromProtoValue(test.in, test.val, c)
		if err == nil {
			t.Errorf("#%d: %v, %v: got nil, want error", i, test.in, test.val)
		}
	}
}

func TestSetFromProtoValuePointers(t *testing.T) {
	// Verify that pointers are set, instead of being replaced.
	// Confirm that the behavior matches encoding/json.
	testSetPointer(t, "json", jsonTester{})
	testSetPointer(t, "firestore", protoTester{&Client{}})
}

func testSetPointer(t *testing.T, prefix string, r tester) {
	// If an interface{} holds a pointer, the pointer is set.

	set := func(x, val interface{}) {
		if err := r.Set(x, val); err != nil {
			t.Fatalf("%s: set(%v, %v): %v", prefix, x, val, err)
		}
	}
	p := new(float64)
	var st struct {
		I interface{}
	}

	// A pointer in a slice of interface{} is set.
	s := []interface{}{p}
	set(&s, r.Array(r.Float(1)))
	if s[0] != p {
		t.Errorf("%s: pointers not identical", prefix)
	}
	if *p != 1 {
		t.Errorf("%s: got %f, want 1", prefix, *p)
	}
	// Setting a null will set the pointer to nil.
	set(&s, r.Array(r.Null()))
	if got := s[0]; got != nil {
		t.Errorf("%s: got %v, want null", prefix, got)
	}

	// It doesn't matter how deep the pointers nest.
	p = new(float64)
	p2 := &p
	p3 := &p2
	s = []interface{}{p3}
	set(&s, r.Array(r.Float(1)))
	if s[0] != p3 {
		t.Errorf("%s: pointers not identical", prefix)
	}
	if *p != 1 {
		t.Errorf("%s: got %f, want 1", prefix, *p)
	}

	// A pointer in an interface{} field is set.
	p = new(float64)
	st.I = p
	set(&st, r.Map("i", r.Float(1)))
	if st.I != p {
		t.Errorf("%s: pointers not identical", prefix)
	}
	if *p != 1 {
		t.Errorf("%s: got %f, want 1", prefix, *p)
	}
	// Setting a null will set the pointer to nil.
	set(&st, r.Map("i", r.Null()))
	if got := st.I; got != nil {
		t.Errorf("%s: got %v, want null", prefix, got)
	}

	// A pointer to a slice (instead of to float64) is set.
	psi := &[]float64{7, 8, 9}
	st.I = psi
	set(&st, r.Map("i", r.Array(r.Float(1))))
	if st.I != psi {
		t.Errorf("%s: pointers not identical", prefix)
	}
	// The slice itself should be truncated and filled, not replaced.
	if got, want := cap(*psi), 3; got != want {
		t.Errorf("cap: got %d, want %d", got, want)
	}
	if want := &[]float64{1}; !testEqual(st.I, want) {
		t.Errorf("got %+v, want %+v", st.I, want)
	}

	// A pointer to a map is set.
	pmf := &map[string]float64{"a": 7, "b": 8}
	st.I = pmf
	set(&st, r.Map("i", r.Map("a", r.Float(1))))
	if st.I != pmf {
		t.Errorf("%s: pointers not identical", prefix)
	}
	if want := map[string]float64{"a": 1, "b": 8}; !testEqual(*pmf, want) {
		t.Errorf("%s: got %+v, want %+v", prefix, *pmf, want)
	}

	// Maps are different: since the map values aren't addressable, they
	// are always discarded, even if the map element type is not interface{}.

	// A map's values are discarded if the value type is a pointer type.
	p = new(float64)
	m := map[string]*float64{"i": p}
	set(&m, r.Map("i", r.Float(1)))
	if m["i"] == p {
		t.Errorf("%s: pointers are identical", prefix)
	}
	if got, want := *m["i"], 1.0; got != want {
		t.Errorf("%s: got %v, want %v", prefix, got, want)
	}
	// A map's values are discarded if the value type is interface{}.
	p = new(float64)
	m2 := map[string]interface{}{"i": p}
	set(&m2, r.Map("i", r.Float(1)))
	if m2["i"] == p {
		t.Errorf("%s: pointers are identical", prefix)
	}
	if got, want := m2["i"].(float64), 1.0; got != want {
		t.Errorf("%s: got %f, want %f", prefix, got, want)
	}
}

// An interface for setting and building values, to facilitate comparing firestore deserialization
// with encoding/json.
type tester interface {
	Set(x, val interface{}) error
	Null() interface{}
	Int(int) interface{}
	Float(float64) interface{}
	Bool(bool) interface{}
	Array(...interface{}) interface{}
	Map(keysvals ...interface{}) interface{}
}

type protoTester struct {
	c *Client
}

func (p protoTester) Set(x, val interface{}) error { return setFromProtoValue(x, val.(*pb.Value), p.c) }
func (protoTester) Null() interface{}              { return nullValue }
func (protoTester) Int(i int) interface{}          { return intval(i) }
func (protoTester) Float(f float64) interface{}    { return floatval(f) }
func (protoTester) Bool(b bool) interface{}        { return boolval(b) }

func (protoTester) Array(els ...interface{}) interface{} {
	var s []*pb.Value
	for _, el := range els {
		s = append(s, el.(*pb.Value))
	}
	return arrayval(s...)
}

func (protoTester) Map(keysvals ...interface{}) interface{} {
	m := map[string]*pb.Value{}
	for i := 0; i < len(keysvals); i += 2 {
		m[keysvals[i].(string)] = keysvals[i+1].(*pb.Value)
	}
	return mapval(m)
}

type jsonTester struct{}

func (jsonTester) Set(x, val interface{}) error { return json.Unmarshal([]byte(val.(string)), x) }
func (jsonTester) Null() interface{}            { return "null" }
func (jsonTester) Int(i int) interface{}        { return fmt.Sprint(i) }
func (jsonTester) Float(f float64) interface{}  { return fmt.Sprint(f) }

func (jsonTester) Bool(b bool) interface{} {
	if b {
		return "true"
	}
	return "false"
}

func (jsonTester) Array(els ...interface{}) interface{} {
	var s []string
	for _, el := range els {
		s = append(s, el.(string))
	}
	return "[" + strings.Join(s, ", ") + "]"
}

func (jsonTester) Map(keysvals ...interface{}) interface{} {
	var s []string
	for i := 0; i < len(keysvals); i += 2 {
		s = append(s, fmt.Sprintf("%q: %v", keysvals[i], keysvals[i+1]))
	}
	return "{" + strings.Join(s, ", ") + "}"
}

func newfloat(f float64) *float64 {
	p := new(float64)
	*p = f
	return p
}

func TestParseDocumentPath(t *testing.T) {
	for _, test := range []struct {
		in        string
		pid, dbid string
		dpath     []string
	}{
		{"projects/foo-bar/databases/db2/documents/c1/d1",
			"foo-bar", "db2", []string{"c1", "d1"}},
		{"projects/P/databases/D/documents/c1/d1/c2/d2",
			"P", "D", []string{"c1", "d1", "c2", "d2"}},
	} {
		gotPid, gotDbid, gotDpath, err := parseDocumentPath(test.in)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := gotPid, test.pid; got != want {
			t.Errorf("project ID: got %q, want %q", got, want)
		}
		if got, want := gotDbid, test.dbid; got != want {
			t.Errorf("db ID: got %q, want %q", got, want)
		}
		if got, want := gotDpath, test.dpath; !testEqual(got, want) {
			t.Errorf("doc path: got %q, want %q", got, want)
		}
	}
}

func TestParseDocumentPathErrors(t *testing.T) {
	for _, badPath := range []string{
		"projects/P/databases/D/documents/c",    // collection path
		"/projects/P/databases/D/documents/c/d", // initial slash
		"projects/P/databases/D/c/d",            // missing "documents"
		"project/P/database/D/document/c/d",
	} {
		// Every prefix of a bad path is also bad.
		for i := 0; i <= len(badPath); i++ {
			in := badPath[:i]
			_, _, _, err := parseDocumentPath(in)
			if err == nil {
				t.Errorf("%q: got nil, want error", in)
			}
		}
	}
}

func TestPathToDoc(t *testing.T) {
	c := &Client{}
	path := "projects/P/databases/D/documents/c1/d1/c2/d2"
	got, err := pathToDoc(path, c)
	if err != nil {
		t.Fatal(err)
	}
	want := &DocumentRef{
		ID:        "d2",
		Path:      "projects/P/databases/D/documents/c1/d1/c2/d2",
		shortPath: "c1/d1/c2/d2",
		Parent: &CollectionRef{
			ID:         "c2",
			parentPath: "projects/P/databases/D/documents/c1/d1",
			Path:       "projects/P/databases/D/documents/c1/d1/c2",
			selfPath:   "c1/d1/c2",
			c:          c,
			Query: Query{
				c:            c,
				collectionID: "c2",
				parentPath:   "projects/P/databases/D/documents/c1/d1",
				path:         "projects/P/databases/D/documents/c1/d1/c2",
			},
			Parent: &DocumentRef{
				ID:        "d1",
				Path:      "projects/P/databases/D/documents/c1/d1",
				shortPath: "c1/d1",
				Parent: &CollectionRef{
					ID:         "c1",
					c:          c,
					parentPath: "projects/P/databases/D/documents",
					Path:       "projects/P/databases/D/documents/c1",
					selfPath:   "c1",
					Parent:     nil,
					Query: Query{
						c:            c,
						collectionID: "c1",
						parentPath:   "projects/P/databases/D/documents",
						path:         "projects/P/databases/D/documents/c1",
					},
				},
			},
		},
	}
	if !testEqual(got, want) {
		t.Errorf("\ngot  %+v\nwant %+v", got, want)
		t.Logf("\ngot.Parent  %+v\nwant.Parent %+v", got.Parent, want.Parent)
		t.Logf("\ngot.Parent.Query  %+v\nwant.Parent.Query %+v", got.Parent.Query, want.Parent.Query)
		t.Logf("\ngot.Parent.Parent  %+v\nwant.Parent.Parent %+v", got.Parent.Parent, want.Parent.Parent)
		t.Logf("\ngot.Parent.Parent.Parent  %+v\nwant.Parent.Parent.Parent %+v", got.Parent.Parent.Parent, want.Parent.Parent.Parent)
		t.Logf("\ngot.Parent.Parent.Parent.Query  %+v\nwant.Parent.Parent.Parent.Query %+v", got.Parent.Parent.Parent.Query, want.Parent.Parent.Parent.Query)
	}
}

func TestTypeString(t *testing.T) {
	for _, test := range []struct {
		in   *pb.Value
		want string
	}{
		{nullValue, "null"},
		{intval(1), "int"},
		{floatval(1), "float"},
		{boolval(true), "bool"},
		{strval(""), "string"},
		{tsval(tm), "timestamp"},
		{geoval(ll), "GeoPoint"},
		{bytesval(nil), "bytes"},
		{refval(""), "reference"},
		{arrayval(nil), "array"},
		{mapval(nil), "map"},
	} {
		got := typeString(test.in)
		if got != test.want {
			t.Errorf("%+v: got %q, want %q", test.in, got, test.want)
		}
	}
}
