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
	"fmt"
	"reflect"
	"testing"
	"time"

	ts "github.com/golang/protobuf/ptypes/timestamp"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/genproto/googleapis/type/latlng"
)

type testStruct1 struct {
	B  bool
	I  int
	U  uint32
	F  float64
	S  string
	Y  []byte
	T  time.Time
	Ts *ts.Timestamp
	G  *latlng.LatLng
	L  []int
	M  map[string]int
	P  *int
}

var (
	p = new(int)

	testVal1 = testStruct1{
		B:  true,
		I:  1,
		U:  2,
		F:  3.0,
		S:  "four",
		Y:  []byte{5},
		T:  tm,
		Ts: ptm,
		G:  ll,
		L:  []int{6},
		M:  map[string]int{"a": 7},
		P:  p,
	}

	mapVal1 = mapval(map[string]*pb.Value{
		"B":  boolval(true),
		"I":  intval(1),
		"U":  intval(2),
		"F":  floatval(3),
		"S":  {ValueType: &pb.Value_StringValue{"four"}},
		"Y":  bytesval([]byte{5}),
		"T":  tsval(tm),
		"Ts": {ValueType: &pb.Value_TimestampValue{ptm}},
		"G":  geoval(ll),
		"L":  arrayval(intval(6)),
		"M":  mapval(map[string]*pb.Value{"a": intval(7)}),
		"P":  intval(8),
	})
)

func TestToProtoValue(t *testing.T) {
	*p = 8
	for _, test := range []struct {
		in   interface{}
		want *pb.Value
	}{
		{nil, nullValue},
		{[]int(nil), nullValue},
		{map[string]int(nil), nullValue},
		{(*testStruct1)(nil), nullValue},
		{(*ts.Timestamp)(nil), nullValue},
		{(*latlng.LatLng)(nil), nullValue},
		{(*DocumentRef)(nil), nullValue},
		{true, boolval(true)},
		{3, intval(3)},
		{uint32(3), intval(3)},
		{1.5, floatval(1.5)},
		{"str", strval("str")},
		{[]byte{1, 2}, bytesval([]byte{1, 2})},
		{tm, tsval(tm)},
		{ptm, &pb.Value{ValueType: &pb.Value_TimestampValue{ptm}}},
		{ll, geoval(ll)},
		{[]int{1, 2}, arrayval(intval(1), intval(2))},
		{&[]int{1, 2}, arrayval(intval(1), intval(2))},
		{[]int{}, arrayval()},
		{map[string]int{"a": 1, "b": 2},
			mapval(map[string]*pb.Value{"a": intval(1), "b": intval(2)})},
		{map[string]int{}, mapval(map[string]*pb.Value{})},
		{p, intval(8)},
		{&p, intval(8)},
		{map[string]interface{}{"a": 1, "p": p, "s": "str"},
			mapval(map[string]*pb.Value{"a": intval(1), "p": intval(8), "s": strval("str")})},
		{map[string]fmt.Stringer{"a": tm},
			mapval(map[string]*pb.Value{"a": tsval(tm)})},
		{testVal1, mapVal1},
		{
			&DocumentRef{
				ID:   "d",
				Path: "projects/P/databases/D/documents/c/d",
				Parent: &CollectionRef{
					ID:         "c",
					parentPath: "projects/P/databases/D",
					Path:       "projects/P/databases/D/documents/c",
					Query:      Query{collectionID: "c", parentPath: "projects/P/databases/D"},
				},
			},
			refval("projects/P/databases/D/documents/c/d"),
		},
		// ServerTimestamps are removed, possibly leaving nil.
		{map[string]interface{}{"a": ServerTimestamp}, nil},
		{
			map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": ServerTimestamp,
					},
				},
			},
			nil,
		},
		{
			map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": ServerTimestamp,
						"d": ServerTimestamp,
					},
				},
			},
			nil,
		},
		{
			map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": ServerTimestamp,
						"d": ServerTimestamp,
						"e": 1,
					},
				},
			},
			mapval(map[string]*pb.Value{
				"a": mapval(map[string]*pb.Value{
					"b": mapval(map[string]*pb.Value{"e": intval(1)}),
				}),
			}),
		},

		// Transforms are allowed in maps, but won't show up in the returned proto. Instead, we rely
		// on seeing sawTransforms=true and a call to extractTransforms.
		{map[string]interface{}{"a": ServerTimestamp, "b": 5}, mapval(map[string]*pb.Value{"b": intval(5)})},
		{map[string]interface{}{"a": ArrayUnion(1, 2, 3), "b": 5}, mapval(map[string]*pb.Value{"b": intval(5)})},
		{map[string]interface{}{"a": ArrayRemove(1, 2, 3), "b": 5}, mapval(map[string]*pb.Value{"b": intval(5)})},
	} {
		got, _, err := toProtoValue(reflect.ValueOf(test.in))
		if err != nil {
			t.Errorf("%v (%T): %v", test.in, test.in, err)
			continue
		}
		if !testEqual(got, test.want) {
			t.Errorf("%+v (%T):\ngot\n%+v\nwant\n%+v", test.in, test.in, got, test.want)
		}
	}
}

type stringy struct{}

func (stringy) String() string { return "stringy" }

func TestToProtoValueErrors(t *testing.T) {
	for _, in := range []interface{}{
		uint64(0),                               // a bad fit for int64
		map[int]bool{},                          // map key type is not string
		make(chan int),                          // can't handle type
		map[string]fmt.Stringer{"a": stringy{}}, // only empty interfaces
		ServerTimestamp,                         // ServerTimestamp can only be a field value
		struct{ A interface{} }{A: ServerTimestamp},
		map[string]interface{}{"a": []interface{}{ServerTimestamp}},
		map[string]interface{}{"a": []interface{}{
			map[string]interface{}{"b": ServerTimestamp},
		}},
		Delete, // Delete should never appear
		[]interface{}{Delete},
		map[string]interface{}{"a": Delete},
		map[string]interface{}{"a": []interface{}{Delete}},

		// Transforms are not allowed to occur in an array.
		[]interface{}{ServerTimestamp},
		[]interface{}{ArrayUnion(1, 2, 3)},
		[]interface{}{ArrayRemove(1, 2, 3)},

		// Transforms are not allowed to occur in a struct.
		struct{ A interface{} }{A: ServerTimestamp},
		struct{ A interface{} }{A: ArrayUnion()},
		struct{ A interface{} }{A: ArrayRemove()},
	} {
		_, _, err := toProtoValue(reflect.ValueOf(in))
		if err == nil {
			t.Errorf("%v: got nil, want error", in)
		}
	}
}

func TestToProtoValue_SawTransform(t *testing.T) {
	for i, in := range []interface{}{
		map[string]interface{}{"a": ServerTimestamp},
		map[string]interface{}{"a": ArrayUnion()},
		map[string]interface{}{"a": ArrayRemove()},
	} {
		_, sawTransform, err := toProtoValue(reflect.ValueOf(in))
		if err != nil {
			t.Fatalf("%d %v: got err %v\nexpected nil", i, in, err)
		}
		if !sawTransform {
			t.Errorf("%d %v: got sawTransform=false, expected sawTransform=true", i, in)
		}
	}
}

type testStruct2 struct {
	Ignore        int       `firestore:"-"`
	Rename        int       `firestore:"a"`
	OmitEmpty     int       `firestore:",omitempty"`
	OmitEmptyTime time.Time `firestore:",omitempty"`
}

func TestToProtoValueTags(t *testing.T) {
	in := &testStruct2{
		Ignore:        1,
		Rename:        2,
		OmitEmpty:     3,
		OmitEmptyTime: aTime,
	}
	got, _, err := toProtoValue(reflect.ValueOf(in))
	if err != nil {
		t.Fatal(err)
	}
	want := mapval(map[string]*pb.Value{
		"a":             intval(2),
		"OmitEmpty":     intval(3),
		"OmitEmptyTime": tsval(aTime),
	})
	if !testEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	got, _, err = toProtoValue(reflect.ValueOf(testStruct2{}))
	if err != nil {
		t.Fatal(err)
	}
	want = mapval(map[string]*pb.Value{"a": intval(0)})
	if !testEqual(got, want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}
}

func TestToProtoValueEmbedded(t *testing.T) {
	// Embedded time.Time, LatLng, or Timestamp should behave like non-embedded.
	type embed struct {
		time.Time
		*latlng.LatLng
		*ts.Timestamp
	}

	got, _, err := toProtoValue(reflect.ValueOf(embed{tm, ll, ptm}))
	if err != nil {
		t.Fatal(err)
	}
	want := mapval(map[string]*pb.Value{
		"Time":      tsval(tm),
		"LatLng":    geoval(ll),
		"Timestamp": {ValueType: &pb.Value_TimestampValue{ptm}},
	})
	if !testEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestIsEmpty(t *testing.T) {
	for _, e := range []interface{}{int(0), float32(0), false, "", []int{}, []int(nil), (*int)(nil)} {
		if !isEmptyValue(reflect.ValueOf(e)) {
			t.Errorf("%v (%T): want true, got false", e, e)
		}
	}
	i := 3
	for _, n := range []interface{}{int(1), float32(1), true, "x", []int{1}, &i} {
		if isEmptyValue(reflect.ValueOf(n)) {
			t.Errorf("%v (%T): want false, got true", n, n)
		}
	}
}
