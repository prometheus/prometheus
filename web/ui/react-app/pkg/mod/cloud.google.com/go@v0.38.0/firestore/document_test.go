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
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	tspb "github.com/golang/protobuf/ptypes/timestamp"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

func TestToProtoDocument(t *testing.T) {
	type s struct{ I int }

	for _, test := range []struct {
		in      interface{}
		want    *pb.Document
		wantErr bool
	}{
		{nil, nil, true},
		{[]int{1}, nil, true},
		{map[string]int{"a": 1},
			&pb.Document{Fields: map[string]*pb.Value{"a": intval(1)}},
			false},
		{s{2}, &pb.Document{Fields: map[string]*pb.Value{"I": intval(2)}}, false},
		{&s{3}, &pb.Document{Fields: map[string]*pb.Value{"I": intval(3)}}, false},
	} {
		got, _, gotErr := toProtoDocument(test.in)
		if (gotErr != nil) != test.wantErr {
			t.Errorf("%v: got error %v, want %t", test.in, gotErr, test.wantErr)
		}
		if gotErr != nil {
			continue
		}
		if !testEqual(got, test.want) {
			t.Errorf("%v: got %v, want %v", test.in, got, test.want)
		}
	}
}

func TestNewDocumentSnapshot(t *testing.T) {
	c := &Client{
		projectID:  "projID",
		databaseID: "(database)",
	}
	docRef := c.Doc("C/a")
	in := &pb.Document{
		CreateTime: &tspb.Timestamp{Seconds: 10},
		UpdateTime: &tspb.Timestamp{Seconds: 20},
		Fields:     map[string]*pb.Value{"a": intval(1)},
	}
	want := &DocumentSnapshot{
		Ref:        docRef,
		CreateTime: time.Unix(10, 0).UTC(),
		UpdateTime: time.Unix(20, 0).UTC(),
		ReadTime:   aTime,
		proto:      in,
		c:          c,
	}
	got, err := newDocumentSnapshot(docRef, in, c, aTimestamp)
	if err != nil {
		t.Fatal(err)
	}
	if !testEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestData(t *testing.T) {
	doc := &DocumentSnapshot{
		proto: &pb.Document{
			Fields: map[string]*pb.Value{"a": intval(1), "b": strval("x")},
		},
	}
	got := doc.Data()
	want := map[string]interface{}{"a": int64(1), "b": "x"}
	if !testEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
	var got2 map[string]interface{}
	if err := doc.DataTo(&got2); err != nil {
		t.Fatal(err)
	}
	if !testEqual(got2, want) {
		t.Errorf("got %#v\nwant %#v", got2, want)
	}

	type s struct {
		A int
		B string
	}
	var got3 s
	if err := doc.DataTo(&got3); err != nil {
		t.Fatal(err)
	}
	want2 := s{A: 1, B: "x"}
	if !testEqual(got3, want2) {
		t.Errorf("got %#v\nwant %#v", got3, want2)
	}
}

var testDoc = &DocumentSnapshot{
	proto: &pb.Document{
		Fields: map[string]*pb.Value{
			"a": intval(1),
			"b": mapval(map[string]*pb.Value{
				"`": intval(2),
				"~": mapval(map[string]*pb.Value{
					"x": intval(3),
				}),
			}),
		},
	},
}

func TestDataAt(t *testing.T) {
	for _, test := range []struct {
		fieldPath string
		want      interface{}
	}{
		{"a", int64(1)},
		{"b.`", int64(2)},
	} {
		got, err := testDoc.DataAt(test.fieldPath)
		if err != nil {
			t.Errorf("%q: %v", test.fieldPath, err)
			continue
		}
		if !testEqual(got, test.want) {
			t.Errorf("%q: got %v, want %v", test.fieldPath, got, test.want)
		}
	}

	for _, bad := range []string{
		"c.~.x", // bad field path
		"a.b",   // "a" isn't a map
		"z.b",   // bad non-final key
		"b.z",   // bad final key
	} {
		_, err := testDoc.DataAt(bad)
		if err == nil {
			t.Errorf("%q: got nil, want error", bad)
		}
	}
}

func TestDataAtPath(t *testing.T) {
	for _, test := range []struct {
		fieldPath FieldPath
		want      interface{}
	}{
		{[]string{"a"}, int64(1)},
		{[]string{"b", "`"}, int64(2)},
		{[]string{"b", "~"}, map[string]interface{}{"x": int64(3)}},
		{[]string{"b", "~", "x"}, int64(3)},
	} {
		got, err := testDoc.DataAtPath(test.fieldPath)
		if err != nil {
			t.Errorf("%v: %v", test.fieldPath, err)
			continue
		}
		if !testEqual(got, test.want) {
			t.Errorf("%v: got %v, want %v", test.fieldPath, got, test.want)
		}
	}

	for _, bad := range []FieldPath{
		[]string{"c", "", "x"}, // bad field path
		[]string{"a", "b"},     // "a" isn't a map
		[]string{"z", "~"},     // bad non-final key
		[]string{"b", "z"},     // bad final key
	} {
		_, err := testDoc.DataAtPath(bad)
		if err == nil {
			t.Errorf("%v: got nil, want error", bad)
		}
	}
}

func TestExtractTransforms(t *testing.T) {
	type S struct {
		A time.Time  `firestore:",serverTimestamp"`
		B time.Time  `firestore:",serverTimestamp"`
		C *time.Time `firestore:",serverTimestamp"`
		D *time.Time `firestore:"d.d,serverTimestamp"`
		E *time.Time `firestore:",serverTimestamp"`
		F time.Time
		G int
	}

	m := map[string]interface{}{
		"ar": map[string]interface{}{"k2": ArrayRemove("e", "f", "g")},
		"au": map[string]interface{}{"k1": ArrayUnion("a", "b", "c")},
		"x":  1,
		"y": &S{
			// A is a zero time: included
			B: aTime, // not a zero time: excluded
			//C is nil: included
			D: &time.Time{}, // pointer to a zero time: included
			E: &aTime,       // pointer to a non-zero time: excluded
			//F is a zero time, but does not have the right tag: excluded
			G: 15, // not a time.Time
		},
		"z": map[string]interface{}{"w": ServerTimestamp},
	}
	got, err := extractTransforms(reflect.ValueOf(m), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []*pb.DocumentTransform_FieldTransform{
		{
			FieldPath: "ar.k2",
			TransformType: &pb.DocumentTransform_FieldTransform_RemoveAllFromArray{
				RemoveAllFromArray: &pb.ArrayValue{Values: []*pb.Value{
					{ValueType: &pb.Value_StringValue{"e"}},
					{ValueType: &pb.Value_StringValue{"f"}},
					{ValueType: &pb.Value_StringValue{"g"}},
				}},
			},
		},
		{
			FieldPath: "au.k1",
			TransformType: &pb.DocumentTransform_FieldTransform_AppendMissingElements{
				AppendMissingElements: &pb.ArrayValue{Values: []*pb.Value{
					{ValueType: &pb.Value_StringValue{"a"}},
					{ValueType: &pb.Value_StringValue{"b"}},
					{ValueType: &pb.Value_StringValue{"c"}},
				}},
			},
		},
		{
			FieldPath: "y.A",
			TransformType: &pb.DocumentTransform_FieldTransform_SetToServerValue{
				SetToServerValue: pb.DocumentTransform_FieldTransform_REQUEST_TIME,
			},
		},
		{
			FieldPath: "y.C",
			TransformType: &pb.DocumentTransform_FieldTransform_SetToServerValue{
				SetToServerValue: pb.DocumentTransform_FieldTransform_REQUEST_TIME,
			},
		},
		{
			FieldPath: "y.`d.d`",
			TransformType: &pb.DocumentTransform_FieldTransform_SetToServerValue{
				SetToServerValue: pb.DocumentTransform_FieldTransform_REQUEST_TIME,
			},
		},
		{
			FieldPath: "z.w",
			TransformType: &pb.DocumentTransform_FieldTransform_SetToServerValue{
				SetToServerValue: pb.DocumentTransform_FieldTransform_REQUEST_TIME,
			},
		},
	}
	if len(got) != len(want) {
		t.Fatalf("Expected output array of size %d, got %d: %v", len(want), len(got), got)
	}
	sort.Sort(byDocumentTransformFieldPath(got))
	for i, vw := range want {
		vg := got[i]
		if !testEqual(vw, vg) {
			t.Fatalf("index %d: got %#v, want %#v", i, vg, vw)
		}
	}
}

type byDocumentTransformFieldPath []*pb.DocumentTransform_FieldTransform

func (b byDocumentTransformFieldPath) Len() int      { return len(b) }
func (b byDocumentTransformFieldPath) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byDocumentTransformFieldPath) Less(i, j int) bool {
	return strings.Compare(b[i].FieldPath, b[j].FieldPath) < 1
}

func TestExtractTransformPathsErrors(t *testing.T) {
	type S struct {
		A int `firestore:",serverTimestamp"`
	}
	_, err := extractTransforms(reflect.ValueOf(S{}), nil)
	if err == nil {
		t.Error("got nil, want error")
	}
}
