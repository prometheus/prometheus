// Copyright 2014 Google LLC
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

package datastore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	pb "google.golang.org/genproto/googleapis/datastore/v1"
	"google.golang.org/grpc"
)

type (
	myBlob   []byte
	myByte   byte
	myString string
)

func makeMyByteSlice(n int) []myByte {
	b := make([]myByte, n)
	for i := range b {
		b[i] = myByte(i)
	}
	return b
}

func makeInt8Slice(n int) []int8 {
	b := make([]int8, n)
	for i := range b {
		b[i] = int8(i)
	}
	return b
}

func makeUint8Slice(n int) []uint8 {
	b := make([]uint8, n)
	for i := range b {
		b[i] = uint8(i)
	}
	return b
}

func newKey(stringID string, parent *Key) *Key {
	return NameKey("kind", stringID, parent)
}

var (
	testKey0     = newKey("name0", nil)
	testKey1a    = newKey("name1", nil)
	testKey1b    = newKey("name1", nil)
	testKey2a    = newKey("name2", testKey0)
	testKey2b    = newKey("name2", testKey0)
	testGeoPt0   = GeoPoint{Lat: 1.2, Lng: 3.4}
	testGeoPt1   = GeoPoint{Lat: 5, Lng: 10}
	testBadGeoPt = GeoPoint{Lat: 1000, Lng: 34}

	ts = time.Unix(1e9, 0).UTC()
)

type B0 struct {
	B []byte `datastore:",noindex"`
}

type B1 struct {
	B []int8
}

type B2 struct {
	B myBlob `datastore:",noindex"`
}

type B3 struct {
	B []myByte `datastore:",noindex"`
}

type B4 struct {
	B [][]byte
}

type C0 struct {
	I int
	C chan int
}

type C1 struct {
	I int
	C *chan int
}

type C2 struct {
	I int
	C []chan int
}

type C3 struct {
	C string
}

type c4 struct {
	C string
}

type E struct{}

type G0 struct {
	G GeoPoint
}

type G1 struct {
	G []GeoPoint
}

type K0 struct {
	K *Key
}

type K1 struct {
	K []*Key
}

type S struct {
	St string
}

type NoOmit struct {
	A string
	B int  `datastore:"Bb"`
	C bool `datastore:",noindex"`
}

type OmitAll struct {
	A string    `datastore:",omitempty"`
	B int       `datastore:"Bb,omitempty"`
	C bool      `datastore:",omitempty,noindex"`
	D time.Time `datastore:",omitempty"`
	F []int     `datastore:",omitempty"`
}

type Omit struct {
	A string    `datastore:",omitempty"`
	B int       `datastore:"Bb,omitempty"`
	C bool      `datastore:",omitempty,noindex"`
	D time.Time `datastore:",omitempty"`
	F []int     `datastore:",omitempty"`
	S `datastore:",omitempty"`
}

type NoOmits struct {
	No []NoOmit `datastore:",omitempty"`
	S  `datastore:",omitempty"`
	Ss S `datastore:",omitempty"`
}

type N0 struct {
	X0
	Nonymous X0
	Ignore   string `datastore:"-"`
	Other    string
}

type N1 struct {
	X0
	Nonymous []X0
	Ignore   string `datastore:"-"`
	Other    string
}

type N2 struct {
	N1    `datastore:"red"`
	Green N1 `datastore:"green"`
	Blue  N1
	White N1 `datastore:"-"`
}

type N3 struct {
	C3 `datastore:"red"`
}

type N4 struct {
	c4
}

type N5 struct {
	c4 `datastore:"red"`
}

type O0 struct {
	I int64
}

type O1 struct {
	I int32
}

type U0 struct {
	U uint
}

type U1 struct {
	U string
}

type T struct {
	T time.Time
}

type X0 struct {
	S string
	I int
	i int
}

type X1 struct {
	S myString
	I int32
	J int64
}

type X2 struct {
	Z string
}

type X3 struct {
	S bool
	I int
}

type Y0 struct {
	B bool
	F []float64
	G []float64
}

type Y1 struct {
	B bool
	F float64
}

type Y2 struct {
	B bool
	F []int64
}

type Pointers struct {
	Pi *int
	Ps *string
	Pb *bool
	Pf *float64
	Pg *GeoPoint
	Pt *time.Time
}

type PointersOmitEmpty struct {
	Pi *int       `datastore:",omitempty"`
	Ps *string    `datastore:",omitempty"`
	Pb *bool      `datastore:",omitempty"`
	Pf *float64   `datastore:",omitempty"`
	Pg *GeoPoint  `datastore:",omitempty"`
	Pt *time.Time `datastore:",omitempty"`
}

func populatedPointers() *Pointers {
	var (
		i int
		s string
		b bool
		f float64
		g GeoPoint
		t time.Time
	)
	return &Pointers{
		Pi: &i,
		Ps: &s,
		Pb: &b,
		Pf: &f,
		Pg: &g,
		Pt: &t,
	}
}

type Tagged struct {
	A int   `datastore:"a,noindex"`
	B []int `datastore:"b"`
	C int   `datastore:",noindex"`
	D int   `datastore:""`
	E int
	I int `datastore:"-"`
	J int `datastore:",noindex" json:"j"`

	Y0 `datastore:"-"`
	Z  chan int `datastore:"-"`
}

type InvalidTagged1 struct {
	I int `datastore:"\t"`
}

type InvalidTagged2 struct {
	I int
	J int `datastore:"I"`
}

type InvalidTagged3 struct {
	X string `datastore:"-,noindex"`
}

type InvalidTagged4 struct {
	X string `datastore:",garbage"`
}

type Inner1 struct {
	W int32
	X string
}

type Inner2 struct {
	Y float64
}

type Inner3 struct {
	Z bool
}

type Inner5 struct {
	WW int
}

type Inner4 struct {
	X Inner5
}

type Outer struct {
	A int16
	I []Inner1
	J Inner2
	Inner3
}

type OuterFlatten struct {
	A      int16
	I      []Inner1 `datastore:",flatten"`
	J      Inner2   `datastore:",flatten,noindex"`
	Inner3 `datastore:",flatten"`
	K      Inner4  `datastore:",flatten"`
	L      *Inner2 `datastore:",flatten"`
}

type OuterEquivalent struct {
	A     int16
	IDotW []int32  `datastore:"I.W"`
	IDotX []string `datastore:"I.X"`
	JDotY float64  `datastore:"J.Y"`
	Z     bool
}

type Dotted struct {
	A DottedA `datastore:"A0.A1.A2"`
}

type DottedA struct {
	B DottedB `datastore:"B3"`
}

type DottedB struct {
	C int `datastore:"C4.C5"`
}

type SliceOfSlices struct {
	I int
	S []struct {
		J int
		F []float64
	} `datastore:",flatten"`
}

type Recursive struct {
	I int
	R []Recursive
}

type MutuallyRecursive0 struct {
	I int
	R []MutuallyRecursive1
}

type MutuallyRecursive1 struct {
	I int
	R []MutuallyRecursive0
}

type EntityWithKey struct {
	I int
	S string
	K *Key `datastore:"__key__"`
}

type WithNestedEntityWithKey struct {
	N EntityWithKey
}

type WithNonKeyField struct {
	I int
	K string `datastore:"__key__"`
}

type NestedWithNonKeyField struct {
	N WithNonKeyField
}

type Basic struct {
	A string
}

type PtrToStructField struct {
	B *Basic
	C *Basic `datastore:"c,noindex"`
	*Basic
	D []*Basic
}

type EmbeddedTime struct {
	time.Time
}

type SpecialTime struct {
	MyTime EmbeddedTime
}

type Doubler struct {
	S string
	I int64
	B bool
}

type Repeat struct {
	Key   string
	Value []byte
}

type Repeated struct {
	Repeats []Repeat
}

func (d *Doubler) Load(props []Property) error {
	return LoadStruct(d, props)
}

func (d *Doubler) Save() ([]Property, error) {
	// Save the default Property slice to an in-memory buffer (a PropertyList).
	props, err := SaveStruct(d)
	if err != nil {
		return nil, err
	}
	var list PropertyList
	if err := list.Load(props); err != nil {
		return nil, err
	}

	// Edit that PropertyList, and send it on.
	for i := range list {
		switch v := list[i].Value.(type) {
		case string:
			// + means string concatenation.
			list[i].Value = v + v
		case int64:
			// + means integer addition.
			list[i].Value = v + v
		}
	}
	return list.Save()
}

var _ PropertyLoadSaver = (*Doubler)(nil)

type Deriver struct {
	S, Derived, Ignored string
}

func (e *Deriver) Load(props []Property) error {
	for _, p := range props {
		if p.Name != "S" {
			continue
		}
		e.S = p.Value.(string)
		e.Derived = "derived+" + e.S
	}
	return nil
}

func (e *Deriver) Save() ([]Property, error) {
	return []Property{
		{
			Name:  "S",
			Value: e.S,
		},
	}, nil
}

var _ PropertyLoadSaver = (*Deriver)(nil)

type BadMultiPropEntity struct{}

func (e *BadMultiPropEntity) Load(props []Property) error {
	return errors.New("unimplemented")
}

func (e *BadMultiPropEntity) Save() ([]Property, error) {
	// Write multiple properties with the same name "I".
	var props []Property
	for i := 0; i < 3; i++ {
		props = append(props, Property{
			Name:  "I",
			Value: int64(i),
		})
	}
	return props, nil
}

var _ PropertyLoadSaver = (*BadMultiPropEntity)(nil)

type testCase struct {
	desc   string
	src    interface{}
	want   interface{}
	putErr string
	getErr string
}

var testCases = []testCase{
	{
		"chan save fails",
		&C0{I: -1},
		&E{},
		"unsupported struct field",
		"",
	},
	{
		"*chan save fails",
		&C1{I: -1},
		&E{},
		"unsupported struct field",
		"",
	},
	{
		"[]chan save fails",
		&C2{I: -1, C: make([]chan int, 8)},
		&E{},
		"unsupported struct field",
		"",
	},
	{
		"chan load fails",
		&C3{C: "not a chan"},
		&C0{},
		"",
		"type mismatch",
	},
	{
		"*chan load fails",
		&C3{C: "not a *chan"},
		&C1{},
		"",
		"type mismatch",
	},
	{
		"[]chan load fails",
		&C3{C: "not a []chan"},
		&C2{},
		"",
		"type mismatch",
	},
	{
		"empty struct",
		&E{},
		&E{},
		"",
		"",
	},
	{
		"geopoint",
		&G0{G: testGeoPt0},
		&G0{G: testGeoPt0},
		"",
		"",
	},
	{
		"geopoint invalid",
		&G0{G: testBadGeoPt},
		&G0{},
		"invalid GeoPoint value",
		"",
	},
	{
		"geopoint as props",
		&G0{G: testGeoPt0},
		&PropertyList{
			Property{Name: "G", Value: testGeoPt0, NoIndex: false},
		},
		"",
		"",
	},
	{
		"geopoint slice",
		&G1{G: []GeoPoint{testGeoPt0, testGeoPt1}},
		&G1{G: []GeoPoint{testGeoPt0, testGeoPt1}},
		"",
		"",
	},
	{
		"omit empty, all",
		&OmitAll{},
		new(PropertyList),
		"",
		"",
	},
	{
		"omit empty",
		&Omit{},
		&PropertyList{
			Property{Name: "St", Value: "", NoIndex: false},
		},
		"",
		"",
	},
	{
		"omit empty, fields populated",
		&Omit{
			A: "a",
			B: 10,
			C: true,
			F: []int{11},
		},
		&PropertyList{
			Property{Name: "A", Value: "a", NoIndex: false},
			Property{Name: "Bb", Value: int64(10), NoIndex: false},
			Property{Name: "C", Value: true, NoIndex: true},
			Property{Name: "F", Value: []interface{}{int64(11)}, NoIndex: false},
			Property{Name: "St", Value: "", NoIndex: false},
		},
		"",
		"",
	},
	{
		"omit empty, fields populated",
		&Omit{
			A: "a",
			B: 10,
			C: true,
			F: []int{11},
			S: S{St: "string"},
		},
		&PropertyList{
			Property{Name: "A", Value: "a", NoIndex: false},
			Property{Name: "Bb", Value: int64(10), NoIndex: false},
			Property{Name: "C", Value: true, NoIndex: true},
			Property{Name: "F", Value: []interface{}{int64(11)}, NoIndex: false},
			Property{Name: "St", Value: "string", NoIndex: false},
		},
		"",
		"",
	},
	{
		"omit empty does not propagate",
		&NoOmits{
			No: []NoOmit{
				{},
			},
			S:  S{},
			Ss: S{},
		},
		&PropertyList{
			Property{Name: "No", Value: []interface{}{
				&Entity{
					Properties: []Property{
						{Name: "A", Value: "", NoIndex: false},
						{Name: "Bb", Value: int64(0), NoIndex: false},
						{Name: "C", Value: false, NoIndex: true},
					},
				},
			}, NoIndex: false},
			Property{Name: "Ss", Value: &Entity{
				Properties: []Property{
					{Name: "St", Value: "", NoIndex: false},
				},
			}, NoIndex: false},
			Property{Name: "St", Value: "", NoIndex: false},
		},
		"",
		"",
	},
	{
		"key",
		&K0{K: testKey1a},
		&K0{K: testKey1b},
		"",
		"",
	},
	{
		"key with parent",
		&K0{K: testKey2a},
		&K0{K: testKey2b},
		"",
		"",
	},
	{
		"nil key",
		&K0{},
		&K0{},
		"",
		"",
	},
	{
		"all nil keys in slice",
		&K1{[]*Key{nil, nil}},
		&K1{[]*Key{nil, nil}},
		"",
		"",
	},
	{
		"some nil keys in slice",
		&K1{[]*Key{testKey1a, nil, testKey2a}},
		&K1{[]*Key{testKey1b, nil, testKey2b}},
		"",
		"",
	},
	{
		"overflow",
		&O0{I: 1 << 48},
		&O1{},
		"",
		"overflow",
	},
	{
		"time",
		&T{T: time.Unix(1e9, 0)},
		&T{T: time.Unix(1e9, 0)},
		"",
		"",
	},
	{
		"time as props",
		&T{T: time.Unix(1e9, 0)},
		&PropertyList{
			Property{Name: "T", Value: time.Unix(1e9, 0), NoIndex: false},
		},
		"",
		"",
	},
	{
		"uint save",
		&U0{U: 1},
		&U0{},
		"unsupported struct field",
		"",
	},
	{
		"uint load",
		&U1{U: "not a uint"},
		&U0{},
		"",
		"type mismatch",
	},
	{
		"zero",
		&X0{},
		&X0{},
		"",
		"",
	},
	{
		"basic",
		&X0{S: "one", I: 2, i: 3},
		&X0{S: "one", I: 2},
		"",
		"",
	},
	{
		"save string/int load myString/int32",
		&X0{S: "one", I: 2, i: 3},
		&X1{S: "one", I: 2},
		"",
		"",
	},
	{
		"missing fields",
		&X0{S: "one", I: 2, i: 3},
		&X2{},
		"",
		"no such struct field",
	},
	{
		"save string load bool",
		&X0{S: "one", I: 2, i: 3},
		&X3{I: 2},
		"",
		"type mismatch",
	},
	{
		"basic slice",
		&Y0{B: true, F: []float64{7, 8, 9}},
		&Y0{B: true, F: []float64{7, 8, 9}},
		"",
		"",
	},
	{
		"save []float64 load float64",
		&Y0{B: true, F: []float64{7, 8, 9}},
		&Y1{B: true},
		"",
		"requires a slice",
	},
	{
		"save []float64 load []int64",
		&Y0{B: true, F: []float64{7, 8, 9}},
		&Y2{B: true},
		"",
		"type mismatch",
	},
	{
		"single slice is too long",
		&Y0{F: make([]float64, maxIndexedProperties+1)},
		&Y0{},
		"too many indexed properties",
		"",
	},
	{
		"two slices are too long",
		&Y0{F: make([]float64, maxIndexedProperties), G: make([]float64, maxIndexedProperties)},
		&Y0{},
		"too many indexed properties",
		"",
	},
	{
		"one slice and one scalar are too long",
		&Y0{F: make([]float64, maxIndexedProperties), B: true},
		&Y0{},
		"too many indexed properties",
		"",
	},
	{
		"slice of slices of bytes",
		&Repeated{
			Repeats: []Repeat{
				{
					Key:   "key 1",
					Value: []byte("value 1"),
				},
				{
					Key:   "key 2",
					Value: []byte("value 2"),
				},
			},
		},
		&Repeated{
			Repeats: []Repeat{
				{
					Key:   "key 1",
					Value: []byte("value 1"),
				},
				{
					Key:   "key 2",
					Value: []byte("value 2"),
				},
			},
		},
		"",
		"",
	},
	{
		"long blob",
		&B0{B: makeUint8Slice(maxIndexedProperties + 1)},
		&B0{B: makeUint8Slice(maxIndexedProperties + 1)},
		"",
		"",
	},
	{
		"long []int8 is too long",
		&B1{B: makeInt8Slice(maxIndexedProperties + 1)},
		&B1{},
		"too many indexed properties",
		"",
	},
	{
		"short []int8",
		&B1{B: makeInt8Slice(3)},
		&B1{B: makeInt8Slice(3)},
		"",
		"",
	},
	{
		"long myBlob",
		&B2{B: makeUint8Slice(maxIndexedProperties + 1)},
		&B2{B: makeUint8Slice(maxIndexedProperties + 1)},
		"",
		"",
	},
	{
		"short myBlob",
		&B2{B: makeUint8Slice(3)},
		&B2{B: makeUint8Slice(3)},
		"",
		"",
	},
	{
		"long []myByte",
		&B3{B: makeMyByteSlice(maxIndexedProperties + 1)},
		&B3{B: makeMyByteSlice(maxIndexedProperties + 1)},
		"",
		"",
	},
	{
		"short []myByte",
		&B3{B: makeMyByteSlice(3)},
		&B3{B: makeMyByteSlice(3)},
		"",
		"",
	},
	{
		"slice of blobs",
		&B4{B: [][]byte{
			makeUint8Slice(3),
			makeUint8Slice(4),
			makeUint8Slice(5),
		}},
		&B4{B: [][]byte{
			makeUint8Slice(3),
			makeUint8Slice(4),
			makeUint8Slice(5),
		}},
		"",
		"",
	},
	{
		"[]byte must be noindex",
		&PropertyList{
			Property{Name: "B", Value: makeUint8Slice(1501), NoIndex: false},
		},
		nil,
		"[]byte property too long to index",
		"",
	},
	{
		"string must be noindex",
		&PropertyList{
			Property{Name: "B", Value: strings.Repeat("x", 1501), NoIndex: false},
		},
		nil,
		"string property too long to index",
		"",
	},
	{
		"slice of []byte must be noindex",
		&PropertyList{
			Property{Name: "B", Value: []interface{}{
				[]byte("short"),
				makeUint8Slice(1501),
			}, NoIndex: false},
		},
		nil,
		"[]byte property too long to index",
		"",
	},
	{
		"slice of string must be noindex",
		&PropertyList{
			Property{Name: "B", Value: []interface{}{
				"short",
				strings.Repeat("x", 1501),
			}, NoIndex: false},
		},
		nil,
		"string property too long to index",
		"",
	},
	{
		"save tagged load props",
		&Tagged{A: 1, B: []int{21, 22, 23}, C: 3, D: 4, E: 5, I: 6, J: 7},
		&PropertyList{
			// A and B are renamed to a and b; A and C are noindex, I is ignored.
			// Order is sorted as per byName.
			Property{Name: "C", Value: int64(3), NoIndex: true},
			Property{Name: "D", Value: int64(4), NoIndex: false},
			Property{Name: "E", Value: int64(5), NoIndex: false},
			Property{Name: "J", Value: int64(7), NoIndex: true},
			Property{Name: "a", Value: int64(1), NoIndex: true},
			Property{Name: "b", Value: []interface{}{int64(21), int64(22), int64(23)}, NoIndex: false},
		},
		"",
		"",
	},
	{
		"save tagged load tagged",
		&Tagged{A: 1, B: []int{21, 22, 23}, C: 3, D: 4, E: 5, I: 6, J: 7},
		&Tagged{A: 1, B: []int{21, 22, 23}, C: 3, D: 4, E: 5, J: 7},
		"",
		"",
	},
	{
		"invalid tagged1",
		&InvalidTagged1{I: 1},
		&InvalidTagged1{},
		"struct tag has invalid property name",
		"",
	},
	{
		"invalid tagged2",
		&InvalidTagged2{I: 1, J: 2},
		&InvalidTagged2{J: 2},
		"",
		"",
	},
	{
		"invalid tagged3",
		&InvalidTagged3{X: "hello"},
		&InvalidTagged3{},
		"struct tag has invalid property name: \"-\"",
		"",
	},
	{
		"invalid tagged4",
		&InvalidTagged4{X: "hello"},
		&InvalidTagged4{},
		"struct tag has invalid option: \"garbage\"",
		"",
	},
	{
		"doubler",
		&Doubler{S: "s", I: 1, B: true},
		&Doubler{S: "ss", I: 2, B: true},
		"",
		"",
	},
	{
		"save struct load props",
		&X0{S: "s", I: 1},
		&PropertyList{
			Property{Name: "I", Value: int64(1), NoIndex: false},
			Property{Name: "S", Value: "s", NoIndex: false},
		},
		"",
		"",
	},
	{
		"save props load struct",
		&PropertyList{
			Property{Name: "I", Value: int64(1), NoIndex: false},
			Property{Name: "S", Value: "s", NoIndex: false},
		},
		&X0{S: "s", I: 1},
		"",
		"",
	},
	{
		"nil-value props",
		&PropertyList{
			Property{Name: "I", Value: nil, NoIndex: false},
			Property{Name: "B", Value: nil, NoIndex: false},
			Property{Name: "S", Value: nil, NoIndex: false},
			Property{Name: "F", Value: nil, NoIndex: false},
			Property{Name: "K", Value: nil, NoIndex: false},
			Property{Name: "T", Value: nil, NoIndex: false},
			Property{Name: "J", Value: []interface{}{nil, int64(7), nil}, NoIndex: false},
		},
		&struct {
			I int64
			B bool
			S string
			F float64
			K *Key
			T time.Time
			J []int64
		}{
			J: []int64{0, 7, 0},
		},
		"",
		"",
	},
	{
		"save outer load props flatten",
		&OuterFlatten{
			A: 1,
			I: []Inner1{
				{10, "ten"},
				{20, "twenty"},
				{30, "thirty"},
			},
			J: Inner2{
				Y: 3.14,
			},
			Inner3: Inner3{
				Z: true,
			},
			K: Inner4{
				X: Inner5{
					WW: 12,
				},
			},
			L: &Inner2{
				Y: 2.71,
			},
		},
		&PropertyList{
			Property{Name: "A", Value: int64(1), NoIndex: false},
			Property{Name: "I.W", Value: []interface{}{int64(10), int64(20), int64(30)}, NoIndex: false},
			Property{Name: "I.X", Value: []interface{}{"ten", "twenty", "thirty"}, NoIndex: false},
			Property{Name: "J.Y", Value: float64(3.14), NoIndex: true},
			Property{Name: "K.X.WW", Value: int64(12), NoIndex: false},
			Property{Name: "L.Y", Value: float64(2.71), NoIndex: false},
			Property{Name: "Z", Value: true, NoIndex: false},
		},
		"",
		"",
	},
	{
		"load outer props flatten",
		&PropertyList{
			Property{Name: "A", Value: int64(1), NoIndex: false},
			Property{Name: "I.W", Value: []interface{}{int64(10), int64(20), int64(30)}, NoIndex: false},
			Property{Name: "I.X", Value: []interface{}{"ten", "twenty", "thirty"}, NoIndex: false},
			Property{Name: "J.Y", Value: float64(3.14), NoIndex: true},
			Property{Name: "L.Y", Value: float64(2.71), NoIndex: false},
			Property{Name: "Z", Value: true, NoIndex: false},
		},
		&OuterFlatten{
			A: 1,
			I: []Inner1{
				{10, "ten"},
				{20, "twenty"},
				{30, "thirty"},
			},
			J: Inner2{
				Y: 3.14,
			},
			Inner3: Inner3{
				Z: true,
			},
			L: &Inner2{
				Y: 2.71,
			},
		},
		"",
		"",
	},
	{
		"save outer load props",
		&Outer{
			A: 1,
			I: []Inner1{
				{10, "ten"},
				{20, "twenty"},
				{30, "thirty"},
			},
			J: Inner2{
				Y: 3.14,
			},
			Inner3: Inner3{
				Z: true,
			},
		},
		&PropertyList{
			Property{Name: "A", Value: int64(1), NoIndex: false},
			Property{Name: "I", Value: []interface{}{
				&Entity{
					Properties: []Property{
						{Name: "W", Value: int64(10), NoIndex: false},
						{Name: "X", Value: "ten", NoIndex: false},
					},
				},
				&Entity{
					Properties: []Property{
						{Name: "W", Value: int64(20), NoIndex: false},
						{Name: "X", Value: "twenty", NoIndex: false},
					},
				},
				&Entity{
					Properties: []Property{
						{Name: "W", Value: int64(30), NoIndex: false},
						{Name: "X", Value: "thirty", NoIndex: false},
					},
				},
			}, NoIndex: false},
			Property{Name: "J", Value: &Entity{
				Properties: []Property{
					{Name: "Y", Value: float64(3.14), NoIndex: false},
				},
			}, NoIndex: false},
			Property{Name: "Z", Value: true, NoIndex: false},
		},
		"",
		"",
	},
	{
		"save props load outer-equivalent",
		&PropertyList{
			Property{Name: "A", Value: int64(1), NoIndex: false},
			Property{Name: "I.W", Value: []interface{}{int64(10), int64(20), int64(30)}, NoIndex: false},
			Property{Name: "I.X", Value: []interface{}{"ten", "twenty", "thirty"}, NoIndex: false},
			Property{Name: "J.Y", Value: float64(3.14), NoIndex: false},
			Property{Name: "Z", Value: true, NoIndex: false},
		},
		&OuterEquivalent{
			A:     1,
			IDotW: []int32{10, 20, 30},
			IDotX: []string{"ten", "twenty", "thirty"},
			JDotY: 3.14,
			Z:     true,
		},
		"",
		"",
	},
	{
		"dotted names save",
		&Dotted{A: DottedA{B: DottedB{C: 88}}},
		&PropertyList{
			Property{Name: "A0.A1.A2", Value: &Entity{
				Properties: []Property{
					{Name: "B3", Value: &Entity{
						Properties: []Property{
							{Name: "C4.C5", Value: int64(88), NoIndex: false},
						},
					}, NoIndex: false},
				},
			}, NoIndex: false},
		},
		"",
		"",
	},
	{
		"dotted names load",
		&PropertyList{
			Property{Name: "A0.A1.A2", Value: &Entity{
				Properties: []Property{
					{Name: "B3", Value: &Entity{
						Properties: []Property{
							{Name: "C4.C5", Value: 99, NoIndex: false},
						},
					}, NoIndex: false},
				},
			}, NoIndex: false},
		},
		&Dotted{A: DottedA{B: DottedB{C: 99}}},
		"",
		"",
	},
	{
		"save struct load deriver",
		&X0{S: "s", I: 1},
		&Deriver{S: "s", Derived: "derived+s"},
		"",
		"",
	},
	{
		"save deriver load struct",
		&Deriver{S: "s", Derived: "derived+s", Ignored: "ignored"},
		&X0{S: "s"},
		"",
		"",
	},
	{
		"zero time.Time",
		&T{T: time.Time{}},
		&T{T: time.Time{}},
		"",
		"",
	},
	{
		"time.Time near Unix zero time",
		&T{T: time.Unix(0, 4e3)},
		&T{T: time.Unix(0, 4e3)},
		"",
		"",
	},
	{
		"time.Time, far in the future",
		&T{T: time.Date(99999, 1, 1, 0, 0, 0, 0, time.UTC)},
		&T{T: time.Date(99999, 1, 1, 0, 0, 0, 0, time.UTC)},
		"",
		"",
	},
	{
		"time.Time, very far in the past",
		&T{T: time.Date(-300000, 1, 1, 0, 0, 0, 0, time.UTC)},
		&T{},
		"time value out of range",
		"",
	},
	{
		"time.Time, very far in the future",
		&T{T: time.Date(294248, 1, 1, 0, 0, 0, 0, time.UTC)},
		&T{},
		"time value out of range",
		"",
	},
	{
		"structs",
		&N0{
			X0:       X0{S: "one", I: 2, i: 3},
			Nonymous: X0{S: "four", I: 5, i: 6},
			Ignore:   "ignore",
			Other:    "other",
		},
		&N0{
			X0:       X0{S: "one", I: 2},
			Nonymous: X0{S: "four", I: 5},
			Other:    "other",
		},
		"",
		"",
	},
	{
		"slice of structs",
		&N1{
			X0: X0{S: "one", I: 2, i: 3},
			Nonymous: []X0{
				{S: "four", I: 5, i: 6},
				{S: "seven", I: 8, i: 9},
				{S: "ten", I: 11, i: 12},
				{S: "thirteen", I: 14, i: 15},
			},
			Ignore: "ignore",
			Other:  "other",
		},
		&N1{
			X0: X0{S: "one", I: 2},
			Nonymous: []X0{
				{S: "four", I: 5},
				{S: "seven", I: 8},
				{S: "ten", I: 11},
				{S: "thirteen", I: 14},
			},
			Other: "other",
		},
		"",
		"",
	},
	{
		"structs with slices of structs",
		&N2{
			N1: N1{
				X0: X0{S: "rouge"},
				Nonymous: []X0{
					{S: "rosso0"},
					{S: "rosso1"},
				},
			},
			Green: N1{
				X0: X0{S: "vert"},
				Nonymous: []X0{
					{S: "verde0"},
					{S: "verde1"},
					{S: "verde2"},
				},
			},
			Blue: N1{
				X0: X0{S: "bleu"},
				Nonymous: []X0{
					{S: "blu0"},
					{S: "blu1"},
					{S: "blu2"},
					{S: "blu3"},
				},
			},
		},
		&N2{
			N1: N1{
				X0: X0{S: "rouge"},
				Nonymous: []X0{
					{S: "rosso0"},
					{S: "rosso1"},
				},
			},
			Green: N1{
				X0: X0{S: "vert"},
				Nonymous: []X0{
					{S: "verde0"},
					{S: "verde1"},
					{S: "verde2"},
				},
			},
			Blue: N1{
				X0: X0{S: "bleu"},
				Nonymous: []X0{
					{S: "blu0"},
					{S: "blu1"},
					{S: "blu2"},
					{S: "blu3"},
				},
			},
		},
		"",
		"",
	},
	{
		"save structs load props",
		&N2{
			N1: N1{
				X0: X0{S: "rouge"},
				Nonymous: []X0{
					{S: "rosso0"},
					{S: "rosso1"},
				},
			},
			Green: N1{
				X0: X0{S: "vert"},
				Nonymous: []X0{
					{S: "verde0"},
					{S: "verde1"},
					{S: "verde2"},
				},
			},
			Blue: N1{
				X0: X0{S: "bleu"},
				Nonymous: []X0{
					{S: "blu0"},
					{S: "blu1"},
					{S: "blu2"},
					{S: "blu3"},
				},
			},
		},
		&PropertyList{
			Property{Name: "Blue", Value: &Entity{
				Properties: []Property{
					{Name: "I", Value: int64(0), NoIndex: false},
					{Name: "Nonymous", Value: []interface{}{
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "blu0", NoIndex: false},
							},
						},
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "blu1", NoIndex: false},
							},
						},
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "blu2", NoIndex: false},
							},
						},
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "blu3", NoIndex: false},
							},
						},
					}, NoIndex: false},
					{Name: "Other", Value: "", NoIndex: false},
					{Name: "S", Value: "bleu", NoIndex: false},
				},
			}, NoIndex: false},
			Property{Name: "green", Value: &Entity{
				Properties: []Property{
					{Name: "I", Value: int64(0), NoIndex: false},
					{Name: "Nonymous", Value: []interface{}{
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "verde0", NoIndex: false},
							},
						},
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "verde1", NoIndex: false},
							},
						},
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "verde2", NoIndex: false},
							},
						},
					}, NoIndex: false},
					{Name: "Other", Value: "", NoIndex: false},
					{Name: "S", Value: "vert", NoIndex: false},
				},
			}, NoIndex: false},
			Property{Name: "red", Value: &Entity{
				Properties: []Property{
					{Name: "I", Value: int64(0), NoIndex: false},
					{Name: "Nonymous", Value: []interface{}{
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "rosso0", NoIndex: false},
							},
						},
						&Entity{
							Properties: []Property{
								{Name: "I", Value: int64(0), NoIndex: false},
								{Name: "S", Value: "rosso1", NoIndex: false},
							},
						},
					}, NoIndex: false},
					{Name: "Other", Value: "", NoIndex: false},
					{Name: "S", Value: "rouge", NoIndex: false},
				},
			}, NoIndex: false},
		},
		"",
		"",
	},
	{
		"nested entity with key",
		&WithNestedEntityWithKey{
			N: EntityWithKey{
				I: 12,
				S: "abcd",
				K: testKey0,
			},
		},
		&WithNestedEntityWithKey{
			N: EntityWithKey{
				I: 12,
				S: "abcd",
				K: testKey0,
			},
		},
		"",
		"",
	},
	{
		"entity with key at top level",
		&EntityWithKey{
			I: 12,
			S: "abc",
			K: testKey0,
		},
		&EntityWithKey{
			I: 12,
			S: "abc",
			K: testKey0,
		},
		"",
		"",
	},
	{
		"entity with key at top level (key is populated on load)",
		&EntityWithKey{
			I: 12,
			S: "abc",
		},
		&EntityWithKey{
			I: 12,
			S: "abc",
			K: testKey0,
		},
		"",
		"",
	},
	{
		"__key__ field not a *Key",
		&NestedWithNonKeyField{
			N: WithNonKeyField{
				I: 12,
				K: "abcd",
			},
		},
		&NestedWithNonKeyField{
			N: WithNonKeyField{
				I: 12,
				K: "abcd",
			},
		},
		"datastore: __key__ field on struct datastore.WithNonKeyField is not a *datastore.Key",
		"",
	},
	{
		"save struct with ptr to struct fields",
		&PtrToStructField{
			&Basic{
				A: "b",
			},
			&Basic{
				A: "c",
			},
			&Basic{
				A: "anon",
			},
			[]*Basic{
				{
					A: "slice0",
				},
				{
					A: "slice1",
				},
			},
		},
		&PropertyList{
			Property{Name: "A", Value: "anon", NoIndex: false},
			Property{Name: "B", Value: &Entity{
				Properties: []Property{
					{Name: "A", Value: "b", NoIndex: false},
				},
			}},
			Property{Name: "D", Value: []interface{}{
				&Entity{
					Properties: []Property{
						{Name: "A", Value: "slice0", NoIndex: false},
					},
				},
				&Entity{
					Properties: []Property{
						{Name: "A", Value: "slice1", NoIndex: false},
					},
				},
			}, NoIndex: false},
			Property{Name: "c", Value: &Entity{
				Properties: []Property{
					{Name: "A", Value: "c", NoIndex: true},
				},
			}, NoIndex: true},
		},
		"",
		"",
	},
	{
		"save and load struct with ptr to struct fields",
		&PtrToStructField{
			&Basic{
				A: "b",
			},
			&Basic{
				A: "c",
			},
			&Basic{
				A: "anon",
			},
			[]*Basic{
				{
					A: "slice0",
				},
				{
					A: "slice1",
				},
			},
		},
		&PtrToStructField{
			&Basic{
				A: "b",
			},
			&Basic{
				A: "c",
			},
			&Basic{
				A: "anon",
			},
			[]*Basic{
				{
					A: "slice0",
				},
				{
					A: "slice1",
				},
			},
		},
		"",
		"",
	},
	{
		"struct with nil ptr to struct fields",
		&PtrToStructField{
			nil,
			nil,
			nil,
			nil,
		},
		new(PropertyList),
		"",
		"",
	},
	{
		"nested load entity with key",
		&WithNestedEntityWithKey{
			N: EntityWithKey{
				I: 12,
				S: "abcd",
				K: testKey0,
			},
		},
		&PropertyList{
			Property{Name: "N", Value: &Entity{
				Key: testKey0,
				Properties: []Property{
					{Name: "I", Value: int64(12), NoIndex: false},
					{Name: "S", Value: "abcd", NoIndex: false},
				},
			},
				NoIndex: false},
		},
		"",
		"",
	},
	{
		"nested save entity with key",
		&PropertyList{
			Property{Name: "N", Value: &Entity{
				Key: testKey0,
				Properties: []Property{
					{Name: "I", Value: int64(12), NoIndex: false},
					{Name: "S", Value: "abcd", NoIndex: false},
				},
			}, NoIndex: false},
		},

		&WithNestedEntityWithKey{
			N: EntityWithKey{
				I: 12,
				S: "abcd",
				K: testKey0,
			},
		},
		"",
		"",
	},
	{
		"anonymous field with tag",
		&N3{
			C3: C3{C: "s"},
		},
		&PropertyList{
			Property{Name: "red", Value: &Entity{
				Properties: []Property{
					{Name: "C", Value: "s", NoIndex: false},
				},
			}, NoIndex: false},
		},
		"",
		"",
	},
	{
		"unexported anonymous field",
		&N4{
			c4: c4{C: "s"},
		},
		&PropertyList{
			Property{Name: "C", Value: "s", NoIndex: false},
		},
		"",
		"",
	},
	{
		"unexported anonymous field with tag",
		&N5{
			c4: c4{C: "s"},
		},
		new(PropertyList),
		"",
		"",
	},
	{
		"save props load structs with ragged fields",
		&PropertyList{
			Property{Name: "red.S", Value: "rot", NoIndex: false},
			Property{Name: "green.Nonymous.I", Value: []interface{}{int64(10), int64(11), int64(12), int64(13)}, NoIndex: false},
			Property{Name: "Blue.Nonymous.I", Value: []interface{}{int64(20), int64(21)}, NoIndex: false},
			Property{Name: "Blue.Nonymous.S", Value: []interface{}{"blau0", "blau1", "blau2"}, NoIndex: false},
		},
		&N2{
			N1: N1{
				X0: X0{S: "rot"},
			},
			Green: N1{
				Nonymous: []X0{
					{I: 10},
					{I: 11},
					{I: 12},
					{I: 13},
				},
			},
			Blue: N1{
				Nonymous: []X0{
					{S: "blau0", I: 20},
					{S: "blau1", I: 21},
					{S: "blau2"},
				},
			},
		},
		"",
		"",
	},
	{
		"save structs with noindex tags",
		&struct {
			A struct {
				X string `datastore:",noindex"`
				Y string
			} `datastore:",noindex"`
			B struct {
				X string `datastore:",noindex"`
				Y string
			}
		}{},
		&PropertyList{
			Property{Name: "A", Value: &Entity{
				Properties: []Property{
					{Name: "X", Value: "", NoIndex: true},
					{Name: "Y", Value: "", NoIndex: true},
				},
			}, NoIndex: true},
			Property{Name: "B", Value: &Entity{
				Properties: []Property{
					{Name: "X", Value: "", NoIndex: true},
					{Name: "Y", Value: "", NoIndex: false},
				},
			}, NoIndex: false},
		},
		"",
		"",
	},
	{
		"embedded struct with name override",
		&struct {
			Inner1 `datastore:"foo"`
		}{},
		&PropertyList{
			Property{Name: "foo", Value: &Entity{
				Properties: []Property{
					{Name: "W", Value: int64(0), NoIndex: false},
					{Name: "X", Value: "", NoIndex: false},
				},
			}, NoIndex: false},
		},
		"",
		"",
	},
	{
		"slice of slices",
		&SliceOfSlices{},
		nil,
		"flattening nested structs leads to a slice of slices",
		"",
	},
	{
		"recursive struct",
		&Recursive{},
		&Recursive{},
		"",
		"",
	},
	{
		"mutually recursive struct",
		&MutuallyRecursive0{},
		&MutuallyRecursive0{},
		"",
		"",
	},
	{
		"non-exported struct fields",
		&struct {
			i, J int64
		}{i: 1, J: 2},
		&PropertyList{
			Property{Name: "J", Value: int64(2), NoIndex: false},
		},
		"",
		"",
	},
	{
		"json.RawMessage",
		&struct {
			J json.RawMessage
		}{
			J: json.RawMessage("rawr"),
		},
		&PropertyList{
			Property{Name: "J", Value: []byte("rawr"), NoIndex: false},
		},
		"",
		"",
	},
	{
		"json.RawMessage to myBlob",
		&struct {
			B json.RawMessage
		}{
			B: json.RawMessage("rawr"),
		},
		&B2{B: myBlob("rawr")},
		"",
		"",
	},
	{
		"repeated property names",
		&PropertyList{
			Property{Name: "A", Value: ""},
			Property{Name: "A", Value: ""},
		},
		nil,
		"duplicate Property",
		"",
	},
	{
		"embedded time field",
		&SpecialTime{MyTime: EmbeddedTime{ts}},
		&SpecialTime{MyTime: EmbeddedTime{ts}},
		"",
		"",
	},
	{
		"embedded time load",
		&PropertyList{
			Property{Name: "MyTime.Time", Value: ts},
		},
		&SpecialTime{MyTime: EmbeddedTime{ts}},
		"",
		"",
	},
	{
		"pointer fields: nil",
		&Pointers{},
		&Pointers{},
		"",
		"",
	},
	{
		"pointer fields: populated with zeroes",
		populatedPointers(),
		populatedPointers(),
		"",
		"",
	},
}

// checkErr returns the empty string if either both want and err are zero,
// or if want is a non-empty substring of err's string representation.
func checkErr(want string, err error) string {
	if err != nil {
		got := err.Error()
		if want == "" || !strings.Contains(got, want) {
			return got
		}
	} else if want != "" {
		return fmt.Sprintf("want error %q", want)
	}
	return ""
}

func TestRoundTrip(t *testing.T) {
	for _, tc := range testCases {
		p, err := saveEntity(testKey0, tc.src)
		if s := checkErr(tc.putErr, err); s != "" {
			t.Errorf("%s: save: %s", tc.desc, s)
			continue
		}
		if p == nil {
			continue
		}
		var got interface{}
		if _, ok := tc.want.(*PropertyList); ok {
			got = new(PropertyList)
		} else {
			got = reflect.New(reflect.TypeOf(tc.want).Elem()).Interface()
		}
		err = loadEntityProto(got, p)
		if s := checkErr(tc.getErr, err); s != "" {
			t.Errorf("%s: load: %s", tc.desc, s)
			continue
		}
		if pl, ok := got.(*PropertyList); ok {
			// Sort by name to make sure we have a deterministic order.
			sortPL(*pl)
		}

		if !testutil.Equal(got, tc.want, cmp.AllowUnexported(X0{}, X2{})) {
			t.Errorf("%s: compare:\ngot:  %+#v\nwant: %+#v", tc.desc, got, tc.want)
			continue
		}
	}
}

type aPtrPLS struct {
	Count int
}

func (pls *aPtrPLS) Load([]Property) error {
	pls.Count++
	return nil
}

func (pls *aPtrPLS) Save() ([]Property, error) {
	return []Property{{Name: "Count", Value: 4}}, nil
}

type aValuePLS struct {
	Count int
}

func (pls aValuePLS) Load([]Property) error {
	pls.Count += 2
	return nil
}

func (pls aValuePLS) Save() ([]Property, error) {
	return []Property{{Name: "Count", Value: 8}}, nil
}

type aValuePtrPLS struct {
	Count int
}

func (pls *aValuePtrPLS) Load([]Property) error {
	pls.Count = 11
	return nil
}

func (pls *aValuePtrPLS) Save() ([]Property, error) {
	return []Property{{Name: "Count", Value: 12}}, nil
}

type aNotPLS struct {
	Count int
}

type plsString string

func (s *plsString) Load([]Property) error {
	*s = "LOADED"
	return nil
}

func (s *plsString) Save() ([]Property, error) {
	return []Property{{Name: "SS", Value: "SAVED"}}, nil
}

func ptrToplsString(s string) *plsString {
	plsStr := plsString(s)
	return &plsStr
}

type aSubPLS struct {
	Foo string
	Bar *aPtrPLS
	Baz aValuePtrPLS
	S   plsString
}

type aSubNotPLS struct {
	Foo string
	Bar *aNotPLS
}

type aSubPLSErr struct {
	Foo string
	Bar aValuePLS
}

type aSubPLSNoErr struct {
	Foo string
	Bar aPtrPLS
}

type GrandparentFlatten struct {
	Parent Parent `datastore:",flatten"`
}

type GrandparentOfPtrFlatten struct {
	Parent ParentOfPtr `datastore:",flatten"`
}

type GrandparentOfSlice struct {
	Parent ParentOfSlice
}

type GrandparentOfSlicePtrs struct {
	Parent ParentOfSlicePtrs
}

type GrandparentOfSliceFlatten struct {
	Parent ParentOfSlice `datastore:",flatten"`
}

type GrandparentOfSlicePtrsFlatten struct {
	Parent ParentOfSlicePtrs `datastore:",flatten"`
}

type Grandparent struct {
	Parent Parent
}

type Parent struct {
	Child  Child
	String plsString
}

type ParentOfPtr struct {
	Child  *Child
	String *plsString
}

type ParentOfSlice struct {
	Children []Child
	Strings  []plsString
}

type ParentOfSlicePtrs struct {
	Children []*Child
	Strings  []*plsString
}

type Child struct {
	I          int
	Grandchild Grandchild
}

type Grandchild struct {
	S string
}

func (c *Child) Load(props []Property) error {
	for _, p := range props {
		if p.Name == "I" {
			c.I++
		} else if p.Name == "Grandchild.S" {
			c.Grandchild.S = "grandchild loaded"
		}
	}

	return nil
}

func (c *Child) Save() ([]Property, error) {
	v := c.I + 1
	return []Property{
		{Name: "I", Value: v},
		{Name: "Grandchild.S", Value: fmt.Sprintf("grandchild saved %d", v)},
	}, nil
}

func TestLoadSavePLS(t *testing.T) {
	type testCase struct {
		desc     string
		src      interface{}
		wantSave *pb.Entity
		wantLoad interface{}
		saveErr  string
		loadErr  string
	}

	testCases := []testCase{
		{
			desc: "non-struct implements PLS (top-level)",
			src:  ptrToplsString("hello"),
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
				},
			},
			wantLoad: ptrToplsString("LOADED"),
		},
		{
			desc: "substructs do implement PLS",
			src:  &aSubPLS{Foo: "foo", Bar: &aPtrPLS{Count: 2}, Baz: aValuePtrPLS{Count: 15}, S: "something"},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Foo": {ValueType: &pb.Value_StringValue{StringValue: "foo"}},
					"Bar": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Count": {ValueType: &pb.Value_IntegerValue{IntegerValue: 4}},
							},
						},
					}},
					"Baz": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Count": {ValueType: &pb.Value_IntegerValue{IntegerValue: 12}},
							},
						},
					}},
					"S": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
							},
						},
					}},
				},
			},
			wantLoad: &aSubPLS{Foo: "foo", Bar: &aPtrPLS{Count: 1}, Baz: aValuePtrPLS{Count: 11}, S: "LOADED"},
		},
		{
			desc: "substruct (ptr) does implement PLS, nil valued substruct",
			src:  &aSubPLS{Foo: "foo", S: "something"},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Foo": {ValueType: &pb.Value_StringValue{StringValue: "foo"}},
					"Baz": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Count": {ValueType: &pb.Value_IntegerValue{IntegerValue: 12}},
							},
						},
					}},
					"S": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
							},
						},
					}},
				},
			},
			wantLoad: &aSubPLS{Foo: "foo", Baz: aValuePtrPLS{Count: 11}, S: "LOADED"},
		},
		{
			desc: "substruct (ptr) does not implement PLS",
			src:  &aSubNotPLS{Foo: "foo", Bar: &aNotPLS{Count: 2}},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Foo": {ValueType: &pb.Value_StringValue{StringValue: "foo"}},
					"Bar": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Count": {ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
							},
						},
					}},
				},
			},
			wantLoad: &aSubNotPLS{Foo: "foo", Bar: &aNotPLS{Count: 2}},
		},
		{
			desc:     "substruct (value) does implement PLS, error on save",
			src:      &aSubPLSErr{Foo: "foo", Bar: aValuePLS{Count: 2}},
			wantSave: (*pb.Entity)(nil),
			wantLoad: &aSubPLSErr{},
			saveErr:  "PropertyLoadSaver methods must be implemented on a pointer",
		},
		{
			desc: "substruct (value) does implement PLS, error on load",
			src:  &aSubPLSNoErr{Foo: "foo", Bar: aPtrPLS{Count: 2}},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Foo": {ValueType: &pb.Value_StringValue{StringValue: "foo"}},
					"Bar": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Count": {ValueType: &pb.Value_IntegerValue{IntegerValue: 4}},
							},
						},
					}},
				},
			},
			wantLoad: &aSubPLSErr{},
			loadErr:  "PropertyLoadSaver methods must be implemented on a pointer",
		},

		{
			desc: "parent does not have flatten option, child impl PLS",
			src: &Grandparent{
				Parent: Parent{
					Child: Child{
						I: 9,
						Grandchild: Grandchild{
							S: "BAD",
						},
					},
					String: plsString("something"),
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Child": {ValueType: &pb.Value_EntityValue{
									EntityValue: &pb.Entity{
										Properties: map[string]*pb.Value{
											"I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
											"Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 10"}},
										},
									},
								}},
								"String": {ValueType: &pb.Value_EntityValue{
									EntityValue: &pb.Entity{
										Properties: map[string]*pb.Value{
											"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
										},
									},
								}},
							},
						},
					}},
				},
			},
			wantLoad: &Grandparent{
				Parent: Parent{
					Child: Child{
						I: 1,
						Grandchild: Grandchild{
							S: "grandchild loaded",
						},
					},
					String: "LOADED",
				},
			},
		},
		{
			desc: "parent has flatten option enabled, child impl PLS",
			src: &GrandparentFlatten{
				Parent: Parent{
					Child: Child{
						I: 7,
						Grandchild: Grandchild{
							S: "BAD",
						},
					},
					String: plsString("something"),
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent.Child.I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 8}},
					"Parent.Child.Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 8"}},
					"Parent.String.SS":          {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
				},
			},
			wantLoad: &GrandparentFlatten{
				Parent: Parent{
					Child: Child{
						I: 1,
						Grandchild: Grandchild{
							S: "grandchild loaded",
						},
					},
					String: "LOADED",
				},
			},
		},

		{
			desc: "parent has flatten option enabled, child (ptr to) impl PLS",
			src: &GrandparentOfPtrFlatten{
				Parent: ParentOfPtr{
					Child: &Child{
						I: 7,
						Grandchild: Grandchild{
							S: "BAD",
						},
					},
					String: ptrToplsString("something"),
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent.Child.I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 8}},
					"Parent.Child.Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 8"}},
					"Parent.String.SS":          {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
				},
			},
			wantLoad: &GrandparentOfPtrFlatten{
				Parent: ParentOfPtr{
					Child: &Child{
						I: 1,
						Grandchild: Grandchild{
							S: "grandchild loaded",
						},
					},
					String: ptrToplsString("LOADED"),
				},
			},
		},
		{
			desc: "children (slice of) impl PLS",
			src: &GrandparentOfSlice{
				Parent: ParentOfSlice{
					Children: []Child{
						{
							I: 7,
							Grandchild: Grandchild{
								S: "BAD",
							},
						},
						{
							I: 9,
							Grandchild: Grandchild{
								S: "BAD2",
							},
						},
					},
					Strings: []plsString{
						"something1",
						"something2",
					},
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Children": {ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 8}},
													"Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 8"}},
												},
											},
										}},
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
													"Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 10"}},
												},
											},
										}},
									}},
								}},
								"Strings": {ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
												},
											},
										}},
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
												},
											},
										}},
									}},
								}},
							},
						},
					}},
				},
			},
			wantLoad: &GrandparentOfSlice{
				Parent: ParentOfSlice{
					Children: []Child{
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
					},
					Strings: []plsString{
						"LOADED",
						"LOADED",
					},
				},
			},
		},
		{
			desc: "children (slice of ptrs) impl PLS",
			src: &GrandparentOfSlicePtrs{
				Parent: ParentOfSlicePtrs{
					Children: []*Child{
						{
							I: 7,
							Grandchild: Grandchild{
								S: "BAD",
							},
						},
						{
							I: 9,
							Grandchild: Grandchild{
								S: "BAD2",
							},
						},
					},
					Strings: []*plsString{
						ptrToplsString("something1"),
						ptrToplsString("something2"),
					},
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent": {ValueType: &pb.Value_EntityValue{
						EntityValue: &pb.Entity{
							Properties: map[string]*pb.Value{
								"Children": {ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 8}},
													"Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 8"}},
												},
											},
										}},
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"I":            {ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
													"Grandchild.S": {ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 10"}},
												},
											},
										}},
									}},
								}},
								"Strings": {ValueType: &pb.Value_ArrayValue{
									ArrayValue: &pb.ArrayValue{Values: []*pb.Value{
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
												},
											},
										}},
										{ValueType: &pb.Value_EntityValue{
											EntityValue: &pb.Entity{
												Properties: map[string]*pb.Value{
													"SS": {ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
												},
											},
										}},
									}},
								}},
							},
						},
					}},
				},
			},
			wantLoad: &GrandparentOfSlicePtrs{
				Parent: ParentOfSlicePtrs{
					Children: []*Child{
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
					},
					Strings: []*plsString{
						ptrToplsString("LOADED"),
						ptrToplsString("LOADED"),
					},
				},
			},
		},
		{
			desc: "parent has flatten option, children (slice of) impl PLS",
			src: &GrandparentOfSliceFlatten{
				Parent: ParentOfSlice{
					Children: []Child{
						{
							I: 7,
							Grandchild: Grandchild{
								S: "BAD",
							},
						},
						{
							I: 9,
							Grandchild: Grandchild{
								S: "BAD2",
							},
						},
					},
					Strings: []plsString{
						"something1",
						"something2",
					},
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent.Children.I": {ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{
						Values: []*pb.Value{
							{ValueType: &pb.Value_IntegerValue{IntegerValue: 8}},
							{ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
						},
					},
					}},
					"Parent.Children.Grandchild.S": {ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{
						Values: []*pb.Value{
							{ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 8"}},
							{ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 10"}},
						},
					},
					}},
					"Parent.Strings.SS": {ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{
						Values: []*pb.Value{
							{ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
							{ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
						},
					},
					}},
				},
			},
			wantLoad: &GrandparentOfSliceFlatten{
				Parent: ParentOfSlice{
					Children: []Child{
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
					},
					Strings: []plsString{
						"LOADED",
						"LOADED",
					},
				},
			},
		},
		{
			desc: "parent has flatten option, children (slice of ptrs) impl PLS",
			src: &GrandparentOfSlicePtrsFlatten{
				Parent: ParentOfSlicePtrs{
					Children: []*Child{
						{
							I: 7,
							Grandchild: Grandchild{
								S: "BAD",
							},
						},
						{
							I: 9,
							Grandchild: Grandchild{
								S: "BAD2",
							},
						},
					},
					Strings: []*plsString{
						ptrToplsString("something1"),
						ptrToplsString("something1"),
					},
				},
			},
			wantSave: &pb.Entity{
				Key: keyToProto(testKey0),
				Properties: map[string]*pb.Value{
					"Parent.Children.I": {ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{
						Values: []*pb.Value{
							{ValueType: &pb.Value_IntegerValue{IntegerValue: 8}},
							{ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
						},
					},
					}},
					"Parent.Children.Grandchild.S": {ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{
						Values: []*pb.Value{
							{ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 8"}},
							{ValueType: &pb.Value_StringValue{StringValue: "grandchild saved 10"}},
						},
					},
					}},
					"Parent.Strings.SS": {ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{
						Values: []*pb.Value{
							{ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
							{ValueType: &pb.Value_StringValue{StringValue: "SAVED"}},
						},
					},
					}},
				},
			},
			wantLoad: &GrandparentOfSlicePtrsFlatten{
				Parent: ParentOfSlicePtrs{
					Children: []*Child{
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
						{
							I: 1,
							Grandchild: Grandchild{
								S: "grandchild loaded",
							},
						},
					},
					Strings: []*plsString{
						ptrToplsString("LOADED"),
						ptrToplsString("LOADED"),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		e, err := saveEntity(testKey0, tc.src)
		if tc.saveErr == "" { // Want no error.
			if err != nil {
				t.Errorf("%s: save: %v", tc.desc, err)
				continue
			}
			if !testutil.Equal(e, tc.wantSave) {
				t.Errorf("%s: save: \ngot:  %+v\nwant: %+v", tc.desc, e, tc.wantSave)
				continue
			}
		} else { // Want error.
			if err == nil {
				t.Errorf("%s: save: want err", tc.desc)
				continue
			}
			if !strings.Contains(err.Error(), tc.saveErr) {
				t.Errorf("%s: save: \ngot err  '%s'\nwant err '%s'", tc.desc, err.Error(), tc.saveErr)
			}
			continue
		}

		gota := reflect.New(reflect.TypeOf(tc.wantLoad).Elem()).Interface()
		err = loadEntityProto(gota, e)
		if tc.loadErr == "" { // Want no error.
			if err != nil {
				t.Errorf("%s: load: %v", tc.desc, err)
				continue
			}
			if !testutil.Equal(gota, tc.wantLoad) {
				t.Errorf("%s: load: \ngot:  %+v\nwant: %+v", tc.desc, gota, tc.wantLoad)
				continue
			}
		} else { // Want error.
			if err == nil {
				t.Errorf("%s: load: want err", tc.desc)
				continue
			}
			if !strings.Contains(err.Error(), tc.loadErr) {
				t.Errorf("%s: load: \ngot err  '%s'\nwant err '%s'", tc.desc, err.Error(), tc.loadErr)
			}
		}
	}
}

func TestQueryConstruction(t *testing.T) {
	tests := []struct {
		q, exp *Query
		err    string
	}{
		{
			q: NewQuery("Foo"),
			exp: &Query{
				kind:  "Foo",
				limit: -1,
			},
		},
		{
			// Regular filtered query with standard spacing.
			q: NewQuery("Foo").Filter("foo >", 7),
			exp: &Query{
				kind: "Foo",
				filter: []filter{
					{
						FieldName: "foo",
						Op:        greaterThan,
						Value:     7,
					},
				},
				limit: -1,
			},
		},
		{
			// Filtered query with no spacing.
			q: NewQuery("Foo").Filter("foo=", 6),
			exp: &Query{
				kind: "Foo",
				filter: []filter{
					{
						FieldName: "foo",
						Op:        equal,
						Value:     6,
					},
				},
				limit: -1,
			},
		},
		{
			// Filtered query with funky spacing.
			q: NewQuery("Foo").Filter(" foo< ", 8),
			exp: &Query{
				kind: "Foo",
				filter: []filter{
					{
						FieldName: "foo",
						Op:        lessThan,
						Value:     8,
					},
				},
				limit: -1,
			},
		},
		{
			// Filtered query with multicharacter op.
			q: NewQuery("Foo").Filter("foo >=", 9),
			exp: &Query{
				kind: "Foo",
				filter: []filter{
					{
						FieldName: "foo",
						Op:        greaterEq,
						Value:     9,
					},
				},
				limit: -1,
			},
		},
		{
			// Query with ordering.
			q: NewQuery("Foo").Order("bar"),
			exp: &Query{
				kind: "Foo",
				order: []order{
					{
						FieldName: "bar",
						Direction: ascending,
					},
				},
				limit: -1,
			},
		},
		{
			// Query with reverse ordering, and funky spacing.
			q: NewQuery("Foo").Order(" - bar"),
			exp: &Query{
				kind: "Foo",
				order: []order{
					{
						FieldName: "bar",
						Direction: descending,
					},
				},
				limit: -1,
			},
		},
		{
			// Query with an empty ordering.
			q:   NewQuery("Foo").Order(""),
			err: "empty order",
		},
		{
			// Query with a + ordering.
			q:   NewQuery("Foo").Order("+bar"),
			err: "invalid order",
		},
	}
	for i, test := range tests {
		if test.q.err != nil {
			got := test.q.err.Error()
			if !strings.Contains(got, test.err) {
				t.Errorf("%d: error mismatch: got %q want something containing %q", i, got, test.err)
			}
			continue
		}
		if !testutil.Equal(test.q, test.exp, cmp.AllowUnexported(Query{})) {
			t.Errorf("%d: mismatch: got %v want %v", i, test.q, test.exp)
		}
	}
}

func TestPutMultiTypes(t *testing.T) {
	ctx := context.Background()
	type S struct {
		A int
		B string
	}

	testCases := []struct {
		desc    string
		src     interface{}
		wantErr bool
	}{
		// Test cases to check each of the valid input types for src.
		// Each case has the same elements.
		{
			desc: "type []struct",
			src: []S{
				{1, "one"}, {2, "two"},
			},
		},
		{
			desc: "type []*struct",
			src: []*S{
				{1, "one"}, {2, "two"},
			},
		},
		{
			desc: "type []interface{} with PLS elems",
			src: []interface{}{
				&PropertyList{Property{Name: "A", Value: 1}, Property{Name: "B", Value: "one"}},
				&PropertyList{Property{Name: "A", Value: 2}, Property{Name: "B", Value: "two"}},
			},
		},
		{
			desc: "type []interface{} with struct ptr elems",
			src: []interface{}{
				&S{1, "one"}, &S{2, "two"},
			},
		},
		{
			desc: "type []PropertyLoadSaver{}",
			src: []PropertyLoadSaver{
				&PropertyList{Property{Name: "A", Value: 1}, Property{Name: "B", Value: "one"}},
				&PropertyList{Property{Name: "A", Value: 2}, Property{Name: "B", Value: "two"}},
			},
		},
		{
			desc: "type []P (non-pointer, *P implements PropertyLoadSaver)",
			src: []PropertyList{
				{Property{Name: "A", Value: 1}, Property{Name: "B", Value: "one"}},
				{Property{Name: "A", Value: 2}, Property{Name: "B", Value: "two"}},
			},
		},
		// Test some invalid cases.
		{
			desc: "type []interface{} with struct elems",
			src: []interface{}{
				S{1, "one"}, S{2, "two"},
			},
			wantErr: true,
		},
		{
			desc: "PropertyList",
			src: PropertyList{
				Property{Name: "A", Value: 1},
				Property{Name: "B", Value: "one"},
			},
			wantErr: true,
		},
		{
			desc:    "type []int",
			src:     []int{1, 2},
			wantErr: true,
		},
		{
			desc:    "not a slice",
			src:     S{1, "one"},
			wantErr: true,
		},
	}

	// Use the same keys and expected entities for all tests.
	keys := []*Key{
		NameKey("testKind", "first", nil),
		NameKey("testKind", "second", nil),
	}
	want := []*pb.Mutation{
		{Operation: &pb.Mutation_Upsert{
			Upsert: &pb.Entity{
				Key: keyToProto(keys[0]),
				Properties: map[string]*pb.Value{
					"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
					"B": {ValueType: &pb.Value_StringValue{StringValue: "one"}},
				},
			}}},
		{Operation: &pb.Mutation_Upsert{
			Upsert: &pb.Entity{
				Key: keyToProto(keys[1]),
				Properties: map[string]*pb.Value{
					"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
					"B": {ValueType: &pb.Value_StringValue{StringValue: "two"}},
				},
			}}},
	}

	for _, tt := range testCases {
		// Set up a fake client which captures upserts.
		var got []*pb.Mutation
		client := &Client{
			client: &fakeClient{
				commitFn: func(req *pb.CommitRequest) (*pb.CommitResponse, error) {
					got = req.Mutations
					return &pb.CommitResponse{}, nil
				},
			},
		}

		_, err := client.PutMulti(ctx, keys, tt.src)
		if err != nil {
			if !tt.wantErr {
				t.Errorf("%s: error %v", tt.desc, err)
			}
			continue
		}
		if tt.wantErr {
			t.Errorf("%s: wanted error, but none returned", tt.desc)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("%s: got %d entities, want %d", tt.desc, len(got), len(want))
			continue
		}
		for i, e := range got {
			if !proto.Equal(e, want[i]) {
				t.Logf("%s: entity %d doesn't match\ngot:  %v\nwant: %v", tt.desc, i, e, want[i])
			}
		}
	}
}

func TestNoIndexOnSliceProperties(t *testing.T) {
	// Check that ExcludeFromIndexes is set on the inner elements,
	// rather than the top-level ArrayValue value.
	pl := PropertyList{
		Property{
			Name: "repeated",
			Value: []interface{}{
				123,
				false,
				"short",
				strings.Repeat("a", 1503),
			},
			NoIndex: true,
		},
	}
	key := NameKey("dummy", "dummy", nil)

	entity, err := saveEntity(key, &pl)
	if err != nil {
		t.Fatalf("saveEntity: %v", err)
	}

	want := &pb.Value{
		ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{Values: []*pb.Value{
			{ValueType: &pb.Value_IntegerValue{IntegerValue: 123}, ExcludeFromIndexes: true},
			{ValueType: &pb.Value_BooleanValue{BooleanValue: false}, ExcludeFromIndexes: true},
			{ValueType: &pb.Value_StringValue{StringValue: "short"}, ExcludeFromIndexes: true},
			{ValueType: &pb.Value_StringValue{StringValue: strings.Repeat("a", 1503)}, ExcludeFromIndexes: true},
		}}},
	}
	if got := entity.Properties["repeated"]; !proto.Equal(got, want) {
		t.Errorf("Entity proto differs\ngot:  %v\nwant: %v", got, want)
	}
}

type byName PropertyList

func (s byName) Len() int           { return len(s) }
func (s byName) Less(i, j int) bool { return s[i].Name < s[j].Name }
func (s byName) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// sortPL sorts the property list by property name, and
// recursively sorts any nested property lists, or nested slices of
// property lists.
func sortPL(pl PropertyList) {
	sort.Stable(byName(pl))
	for _, p := range pl {
		switch p.Value.(type) {
		case *Entity:
			sortPL(p.Value.(*Entity).Properties)
		case []interface{}:
			for _, p2 := range p.Value.([]interface{}) {
				if nent, ok := p2.(*Entity); ok {
					sortPL(nent.Properties)
				}
			}
		}
	}
}

func TestValidGeoPoint(t *testing.T) {
	testCases := []struct {
		desc string
		pt   GeoPoint
		want bool
	}{
		{
			"valid",
			GeoPoint{67.21, 13.37},
			true,
		},
		{
			"high lat",
			GeoPoint{-90.01, 13.37},
			false,
		},
		{
			"low lat",
			GeoPoint{90.01, 13.37},
			false,
		},
		{
			"high lng",
			GeoPoint{67.21, 182},
			false,
		},
		{
			"low lng",
			GeoPoint{67.21, -181},
			false,
		},
	}

	for _, tc := range testCases {
		if got := tc.pt.Valid(); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.desc, got, tc.want)
		}
	}
}

func TestPutInvalidEntity(t *testing.T) {
	// Test that trying to put an invalid entity always returns the correct error
	// type.

	// Fake client that can pretend to start a transaction.
	fakeClient := &fakeDatastoreClient{
		beginTransaction: func(*pb.BeginTransactionRequest) (*pb.BeginTransactionResponse, error) {
			return &pb.BeginTransactionResponse{
				Transaction: []byte("deadbeef"),
			}, nil
		},
	}
	client := &Client{
		client: fakeClient,
	}

	ctx := context.Background()
	key := IncompleteKey("kind", nil)

	_, err := client.Put(ctx, key, "invalid entity")
	if err != ErrInvalidEntityType {
		t.Errorf("client.Put returned err %v, want %v", err, ErrInvalidEntityType)
	}

	_, err = client.PutMulti(ctx, []*Key{key}, []interface{}{"invalid entity"})
	if me, ok := err.(MultiError); !ok {
		t.Errorf("client.PutMulti returned err %v, want MultiError type", err)
	} else if len(me) != 1 || me[0] != ErrInvalidEntityType {
		t.Errorf("client.PutMulti returned err %v, want MulitError{ErrInvalidEntityType}", err)
	}

	client.RunInTransaction(ctx, func(tx *Transaction) error {
		_, err := tx.Put(key, "invalid entity")
		if err != ErrInvalidEntityType {
			t.Errorf("tx.Put returned err %v, want %v", err, ErrInvalidEntityType)
		}

		_, err = tx.PutMulti([]*Key{key}, []interface{}{"invalid entity"})
		if me, ok := err.(MultiError); !ok {
			t.Errorf("tx.PutMulti returned err %v, want MultiError type", err)
		} else if len(me) != 1 || me[0] != ErrInvalidEntityType {
			t.Errorf("tx.PutMulti returned err %v, want MulitError{ErrInvalidEntityType}", err)
		}

		return errors.New("bang") // Return error: we don't actually want to commit.
	})
}

func TestDeferred(t *testing.T) {
	type Ent struct {
		A int
		B string
	}

	keys := []*Key{
		NameKey("testKind", "first", nil),
		NameKey("testKind", "second", nil),
	}

	entity1 := &pb.Entity{
		Key: keyToProto(keys[0]),
		Properties: map[string]*pb.Value{
			"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
			"B": {ValueType: &pb.Value_StringValue{StringValue: "one"}},
		},
	}
	entity2 := &pb.Entity{
		Key: keyToProto(keys[1]),
		Properties: map[string]*pb.Value{
			"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
			"B": {ValueType: &pb.Value_StringValue{StringValue: "two"}},
		},
	}

	// count keeps track of the number of times fakeClient.lookup has been
	// called.
	var count int
	// Fake client that will return Deferred keys in resp on the first call.
	fakeClient := &fakeDatastoreClient{
		lookup: func(*pb.LookupRequest) (*pb.LookupResponse, error) {
			count++
			// On the first call, we return deferred keys.
			if count == 1 {
				return &pb.LookupResponse{
					Found: []*pb.EntityResult{
						{
							Entity:  entity1,
							Version: 1,
						},
					},
					Deferred: []*pb.Key{
						keyToProto(keys[1]),
					},
				}, nil
			}

			// On the second call, we do not return any more deferred keys.
			return &pb.LookupResponse{
				Found: []*pb.EntityResult{
					{
						Entity:  entity2,
						Version: 1,
					},
				},
			}, nil
		},
	}
	client := &Client{
		client: fakeClient,
	}

	ctx := context.Background()

	dst := make([]Ent, len(keys))
	err := client.GetMulti(ctx, keys, dst)
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}

	if count != 2 {
		t.Fatalf("expected client.lookup to be called 2 times. Got %d", count)
	}

	if len(dst) != 2 {
		t.Fatalf("expected 2 entities returned, got %d", len(dst))
	}

	for _, e := range dst {
		if e.A == 1 {
			if e.B != "one" {
				t.Fatalf("unexpected entity %+v", e)
			}
		} else if e.A == 2 {
			if e.B != "two" {
				t.Fatalf("unexpected entity %+v", e)
			}
		} else {
			t.Fatalf("unexpected entity %+v", e)
		}
	}

}

type KeyLoaderEnt struct {
	A int
	K *Key
}

func (e *KeyLoaderEnt) Load(p []Property) error {
	e.A = 2
	return nil
}

func (e *KeyLoaderEnt) LoadKey(k *Key) error {
	e.K = k
	return nil
}

func (e *KeyLoaderEnt) Save() ([]Property, error) {
	return []Property{{Name: "A", Value: int64(3)}}, nil
}

func TestKeyLoaderEndToEnd(t *testing.T) {
	keys := []*Key{
		NameKey("testKind", "first", nil),
		NameKey("testKind", "second", nil),
	}

	entity1 := &pb.Entity{
		Key: keyToProto(keys[0]),
		Properties: map[string]*pb.Value{
			"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
			"B": {ValueType: &pb.Value_StringValue{StringValue: "one"}},
		},
	}
	entity2 := &pb.Entity{
		Key: keyToProto(keys[1]),
		Properties: map[string]*pb.Value{
			"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
			"B": {ValueType: &pb.Value_StringValue{StringValue: "two"}},
		},
	}

	fakeClient := &fakeDatastoreClient{
		lookup: func(*pb.LookupRequest) (*pb.LookupResponse, error) {
			return &pb.LookupResponse{
				Found: []*pb.EntityResult{
					{
						Entity:  entity1,
						Version: 1,
					},
					{
						Entity:  entity2,
						Version: 1,
					},
				},
			}, nil
		},
	}
	client := &Client{
		client: fakeClient,
	}

	ctx := context.Background()

	dst := make([]*KeyLoaderEnt, len(keys))
	err := client.GetMulti(ctx, keys, dst)
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}

	for i := range dst {
		if !testutil.Equal(dst[i].K, keys[i]) {
			t.Fatalf("unexpected entity %d to have key %+v, got %+v", i, keys[i], dst[i].K)
		}
	}
}

func TestDeferredMissing(t *testing.T) {
	type Ent struct {
		A int
		B string
	}

	keys := []*Key{
		NameKey("testKind", "first", nil),
		NameKey("testKind", "second", nil),
	}

	entity1 := &pb.Entity{
		Key: keyToProto(keys[0]),
	}
	entity2 := &pb.Entity{
		Key: keyToProto(keys[1]),
	}

	var count int
	fakeClient := &fakeDatastoreClient{
		lookup: func(*pb.LookupRequest) (*pb.LookupResponse, error) {
			count++

			if count == 1 {
				return &pb.LookupResponse{
					Missing: []*pb.EntityResult{
						{
							Entity:  entity1,
							Version: 1,
						},
					},
					Deferred: []*pb.Key{
						keyToProto(keys[1]),
					},
				}, nil
			}

			return &pb.LookupResponse{
				Missing: []*pb.EntityResult{
					{
						Entity:  entity2,
						Version: 1,
					},
				},
			}, nil
		},
	}
	client := &Client{
		client: fakeClient,
	}

	ctx := context.Background()

	dst := make([]Ent, len(keys))
	err := client.GetMulti(ctx, keys, dst)
	errs, ok := err.(MultiError)
	if !ok {
		t.Fatalf("expected error returns to be MultiError; got %v", err)
	}
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors returns, got %d", len(errs))
	}
	if errs[0] != ErrNoSuchEntity {
		t.Fatalf("expected error to be ErrNoSuchEntity; got %v", errs[0])
	}
	if errs[1] != ErrNoSuchEntity {
		t.Fatalf("expected error to be ErrNoSuchEntity; got %v", errs[1])
	}

	if count != 2 {
		t.Fatalf("expected client.lookup to be called 2 times. Got %d", count)
	}

	if len(dst) != 2 {
		t.Fatalf("expected 2 entities returned, got %d", len(dst))
	}

	for _, e := range dst {
		if e.A != 0 || e.B != "" {
			t.Fatalf("unexpected entity %+v", e)
		}
	}
}

func TestGetWithNilKey(t *testing.T) {
	client := &Client{}
	err := client.Get(context.Background(), nil, []Property{})
	if err != ErrInvalidKey {
		t.Fatalf("want ErrInvalidKey, got %v", err)
	}
}

func TestGetMultiWithNilKey(t *testing.T) {
	client := &Client{}
	dest := make([]PropertyList, 1)
	err := client.GetMulti(context.Background(), []*Key{nil}, dest)
	if me, ok := err.(MultiError); !ok {
		t.Fatalf("want MultiError, got %v", err)
	} else if len(me) != 1 || me[0] != ErrInvalidKey {
		t.Fatalf("want MultiError{ErrInvalidKey}, got %v", me)
	}
}

func TestGetWithIncompleteKey(t *testing.T) {
	client := &Client{}
	err := client.Get(context.Background(), &Key{Kind: "testKind"}, []Property{})
	if err == nil {
		t.Fatalf("want err, got nil")
	}
}

func TestGetMultiWithIncompleteKey(t *testing.T) {
	client := &Client{}
	dest := make([]PropertyList, 1)
	err := client.GetMulti(context.Background(), []*Key{{Kind: "testKind"}}, dest)
	if me, ok := err.(MultiError); !ok {
		t.Fatalf("want MultiError, got %v", err)
	} else if len(me) != 1 || me[0] == nil {
		t.Fatalf("want MultiError{err}, got %v", me)
	}
}

func TestDeleteWithNilKey(t *testing.T) {
	client := &Client{}
	err := client.Delete(context.Background(), nil)
	if err != ErrInvalidKey {
		t.Fatalf("want ErrInvalidKey, got %v", err)
	}
}

func TestDeleteMultiWithNilKey(t *testing.T) {
	client := &Client{}
	err := client.DeleteMulti(context.Background(), []*Key{nil})
	if me, ok := err.(MultiError); !ok {
		t.Fatalf("want MultiError, got %v", err)
	} else if len(me) != 1 || me[0] != ErrInvalidKey {
		t.Fatalf("want MultiError{ErrInvalidKey}, got %v", me)
	}
}

func TestDeleteWithIncompleteKey(t *testing.T) {
	client := &Client{}
	err := client.Delete(context.Background(), &Key{Kind: "testKind"})
	if err == nil {
		t.Fatalf("want err, got nil")
	}
}

func TestDeleteMultiWithIncompleteKey(t *testing.T) {
	client := &Client{}
	err := client.DeleteMulti(context.Background(), []*Key{{Kind: "testKind"}})
	if me, ok := err.(MultiError); !ok {
		t.Fatalf("want MultiError, got %v", err)
	} else if len(me) != 1 || me[0] == nil {
		t.Fatalf("want MultiError{err}, got %v", me)
	}
}

type fakeDatastoreClient struct {
	pb.DatastoreClient

	// Optional handlers for the datastore methods.
	// Any handlers left undefined will return an error.
	lookup           func(*pb.LookupRequest) (*pb.LookupResponse, error)
	runQuery         func(*pb.RunQueryRequest) (*pb.RunQueryResponse, error)
	beginTransaction func(*pb.BeginTransactionRequest) (*pb.BeginTransactionResponse, error)
	commit           func(*pb.CommitRequest) (*pb.CommitResponse, error)
	rollback         func(*pb.RollbackRequest) (*pb.RollbackResponse, error)
	allocateIds      func(*pb.AllocateIdsRequest) (*pb.AllocateIdsResponse, error)
}

func (c *fakeDatastoreClient) Lookup(ctx context.Context, in *pb.LookupRequest, opts ...grpc.CallOption) (*pb.LookupResponse, error) {
	if c.lookup == nil {
		return nil, errors.New("no lookup handler defined")
	}
	return c.lookup(in)
}
func (c *fakeDatastoreClient) RunQuery(ctx context.Context, in *pb.RunQueryRequest, opts ...grpc.CallOption) (*pb.RunQueryResponse, error) {
	if c.runQuery == nil {
		return nil, errors.New("no runQuery handler defined")
	}
	return c.runQuery(in)
}
func (c *fakeDatastoreClient) BeginTransaction(ctx context.Context, in *pb.BeginTransactionRequest, opts ...grpc.CallOption) (*pb.BeginTransactionResponse, error) {
	if c.beginTransaction == nil {
		return nil, errors.New("no beginTransaction handler defined")
	}
	return c.beginTransaction(in)
}
func (c *fakeDatastoreClient) Commit(ctx context.Context, in *pb.CommitRequest, opts ...grpc.CallOption) (*pb.CommitResponse, error) {
	if c.commit == nil {
		return nil, errors.New("no commit handler defined")
	}
	return c.commit(in)
}
func (c *fakeDatastoreClient) Rollback(ctx context.Context, in *pb.RollbackRequest, opts ...grpc.CallOption) (*pb.RollbackResponse, error) {
	if c.rollback == nil {
		return nil, errors.New("no rollback handler defined")
	}
	return c.rollback(in)
}
func (c *fakeDatastoreClient) AllocateIds(ctx context.Context, in *pb.AllocateIdsRequest, opts ...grpc.CallOption) (*pb.AllocateIdsResponse, error) {
	if c.allocateIds == nil {
		return nil, errors.New("no allocateIds handler defined")
	}
	return c.allocateIds(in)
}
