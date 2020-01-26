// Copyright 2015 Google LLC
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

package bigquery

import (
	"encoding/base64"
	"fmt"
	"math"
	"math/big"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	bq "google.golang.org/api/bigquery/v2"
)

func TestConvertBasicValues(t *testing.T) {
	schema := Schema{
		{Type: StringFieldType},
		{Type: IntegerFieldType},
		{Type: FloatFieldType},
		{Type: BooleanFieldType},
		{Type: BytesFieldType},
		{Type: NumericFieldType},
		{Type: GeographyFieldType},
	}
	row := &bq.TableRow{
		F: []*bq.TableCell{
			{V: "a"},
			{V: "1"},
			{V: "1.2"},
			{V: "true"},
			{V: base64.StdEncoding.EncodeToString([]byte("foo"))},
			{V: "123.123456789"},
			{V: testGeography},
		},
	}
	got, err := convertRow(row, schema)
	if err != nil {
		t.Fatalf("error converting: %v", err)
	}

	want := []Value{"a", int64(1), 1.2, true, []byte("foo"), big.NewRat(123123456789, 1e9), testGeography}
	if !testutil.Equal(got, want) {
		t.Errorf("converting basic values: got:\n%v\nwant:\n%v", got, want)
	}
}

func TestConvertTime(t *testing.T) {
	schema := Schema{
		{Type: TimestampFieldType},
		{Type: DateFieldType},
		{Type: TimeFieldType},
		{Type: DateTimeFieldType},
	}
	ts := testTimestamp.Round(time.Millisecond)
	row := &bq.TableRow{
		F: []*bq.TableCell{
			{V: fmt.Sprintf("%.10f", float64(ts.UnixNano())/1e9)},
			{V: testDate.String()},
			{V: testTime.String()},
			{V: testDateTime.String()},
		},
	}
	got, err := convertRow(row, schema)
	if err != nil {
		t.Fatalf("error converting: %v", err)
	}
	want := []Value{ts, testDate, testTime, testDateTime}
	for i, g := range got {
		w := want[i]
		if !testutil.Equal(g, w) {
			t.Errorf("#%d: got:\n%v\nwant:\n%v", i, g, w)
		}
	}
	if got[0].(time.Time).Location() != time.UTC {
		t.Errorf("expected time zone UTC: got:\n%v", got)
	}
}

func TestConvertSmallTimes(t *testing.T) {
	for _, year := range []int{1600, 1066, 1} {
		want := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
		s := fmt.Sprintf("%.10f", float64(want.Unix()))
		got, err := convertBasicType(s, TimestampFieldType)
		if err != nil {
			t.Fatal(err)
		}
		if !got.(time.Time).Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

func TestConvertNullValues(t *testing.T) {
	schema := Schema{{Type: StringFieldType}}
	row := &bq.TableRow{
		F: []*bq.TableCell{
			{V: nil},
		},
	}
	got, err := convertRow(row, schema)
	if err != nil {
		t.Fatalf("error converting: %v", err)
	}
	want := []Value{nil}
	if !testutil.Equal(got, want) {
		t.Errorf("converting null values: got:\n%v\nwant:\n%v", got, want)
	}
}

func TestBasicRepetition(t *testing.T) {
	schema := Schema{
		{Type: IntegerFieldType, Repeated: true},
	}
	row := &bq.TableRow{
		F: []*bq.TableCell{
			{
				V: []interface{}{
					map[string]interface{}{
						"v": "1",
					},
					map[string]interface{}{
						"v": "2",
					},
					map[string]interface{}{
						"v": "3",
					},
				},
			},
		},
	}
	got, err := convertRow(row, schema)
	if err != nil {
		t.Fatalf("error converting: %v", err)
	}
	want := []Value{[]Value{int64(1), int64(2), int64(3)}}
	if !testutil.Equal(got, want) {
		t.Errorf("converting basic repeated values: got:\n%v\nwant:\n%v", got, want)
	}
}

func TestNestedRecordContainingRepetition(t *testing.T) {
	schema := Schema{
		{
			Type: RecordFieldType,
			Schema: Schema{
				{Type: IntegerFieldType, Repeated: true},
			},
		},
	}
	row := &bq.TableRow{
		F: []*bq.TableCell{
			{
				V: map[string]interface{}{
					"f": []interface{}{
						map[string]interface{}{
							"v": []interface{}{
								map[string]interface{}{"v": "1"},
								map[string]interface{}{"v": "2"},
								map[string]interface{}{"v": "3"},
							},
						},
					},
				},
			},
		},
	}

	got, err := convertRow(row, schema)
	if err != nil {
		t.Fatalf("error converting: %v", err)
	}
	want := []Value{[]Value{[]Value{int64(1), int64(2), int64(3)}}}
	if !testutil.Equal(got, want) {
		t.Errorf("converting basic repeated values: got:\n%v\nwant:\n%v", got, want)
	}
}

func TestRepeatedRecordContainingRepetition(t *testing.T) {
	schema := Schema{
		{
			Type:     RecordFieldType,
			Repeated: true,
			Schema: Schema{
				{Type: IntegerFieldType, Repeated: true},
			},
		},
	}
	row := &bq.TableRow{F: []*bq.TableCell{
		{
			V: []interface{}{ // repeated records.
				map[string]interface{}{ // first record.
					"v": map[string]interface{}{ // pointless single-key-map wrapper.
						"f": []interface{}{ // list of record fields.
							map[string]interface{}{ // only record (repeated ints)
								"v": []interface{}{ // pointless wrapper.
									map[string]interface{}{
										"v": "1",
									},
									map[string]interface{}{
										"v": "2",
									},
									map[string]interface{}{
										"v": "3",
									},
								},
							},
						},
					},
				},
				map[string]interface{}{ // second record.
					"v": map[string]interface{}{
						"f": []interface{}{
							map[string]interface{}{
								"v": []interface{}{
									map[string]interface{}{
										"v": "4",
									},
									map[string]interface{}{
										"v": "5",
									},
									map[string]interface{}{
										"v": "6",
									},
								},
							},
						},
					},
				},
			},
		},
	}}

	got, err := convertRow(row, schema)
	if err != nil {
		t.Fatalf("error converting: %v", err)
	}
	want := []Value{ // the row is a list of length 1, containing an entry for the repeated record.
		[]Value{ // the repeated record is a list of length 2, containing an entry for each repetition.
			[]Value{ // the record is a list of length 1, containing an entry for the repeated integer field.
				[]Value{int64(1), int64(2), int64(3)}, // the repeated integer field is a list of length 3.
			},
			[]Value{ // second record
				[]Value{int64(4), int64(5), int64(6)},
			},
		},
	}
	if !testutil.Equal(got, want) {
		t.Errorf("converting repeated records with repeated values: got:\n%v\nwant:\n%v", got, want)
	}
}

func TestRepeatedRecordContainingRecord(t *testing.T) {
	schema := Schema{
		{
			Type:     RecordFieldType,
			Repeated: true,
			Schema: Schema{
				{
					Type: StringFieldType,
				},
				{
					Type: RecordFieldType,
					Schema: Schema{
						{Type: IntegerFieldType},
						{Type: StringFieldType},
					},
				},
			},
		},
	}
	row := &bq.TableRow{F: []*bq.TableCell{
		{
			V: []interface{}{ // repeated records.
				map[string]interface{}{ // first record.
					"v": map[string]interface{}{ // pointless single-key-map wrapper.
						"f": []interface{}{ // list of record fields.
							map[string]interface{}{ // first record field (name)
								"v": "first repeated record",
							},
							map[string]interface{}{ // second record field (nested record).
								"v": map[string]interface{}{ // pointless single-key-map wrapper.
									"f": []interface{}{ // nested record fields
										map[string]interface{}{
											"v": "1",
										},
										map[string]interface{}{
											"v": "two",
										},
									},
								},
							},
						},
					},
				},
				map[string]interface{}{ // second record.
					"v": map[string]interface{}{
						"f": []interface{}{
							map[string]interface{}{
								"v": "second repeated record",
							},
							map[string]interface{}{
								"v": map[string]interface{}{
									"f": []interface{}{
										map[string]interface{}{
											"v": "3",
										},
										map[string]interface{}{
											"v": "four",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}}

	got, err := convertRow(row, schema)
	if err != nil {
		t.Fatalf("error converting: %v", err)
	}
	// TODO: test with flattenresults.
	want := []Value{ // the row is a list of length 1, containing an entry for the repeated record.
		[]Value{ // the repeated record is a list of length 2, containing an entry for each repetition.
			[]Value{ // record contains a string followed by a nested record.
				"first repeated record",
				[]Value{
					int64(1),
					"two",
				},
			},
			[]Value{ // second record.
				"second repeated record",
				[]Value{
					int64(3),
					"four",
				},
			},
		},
	}
	if !testutil.Equal(got, want) {
		t.Errorf("converting repeated records containing record : got:\n%v\nwant:\n%v", got, want)
	}
}

func TestConvertRowErrors(t *testing.T) {
	// mismatched lengths
	if _, err := convertRow(&bq.TableRow{F: []*bq.TableCell{{V: ""}}}, Schema{}); err == nil {
		t.Error("got nil, want error")
	}
	v3 := map[string]interface{}{"v": 3}
	for _, test := range []struct {
		value interface{}
		fs    FieldSchema
	}{
		{3, FieldSchema{Type: IntegerFieldType}}, // not a string
		{[]interface{}{v3}, // not a string, repeated
			FieldSchema{Type: IntegerFieldType, Repeated: true}},
		{map[string]interface{}{"f": []interface{}{v3}}, // not a string, nested
			FieldSchema{Type: RecordFieldType, Schema: Schema{{Type: IntegerFieldType}}}},
		{map[string]interface{}{"f": []interface{}{v3}}, // wrong length, nested
			FieldSchema{Type: RecordFieldType, Schema: Schema{}}},
	} {
		_, err := convertRow(
			&bq.TableRow{F: []*bq.TableCell{{V: test.value}}},
			Schema{&test.fs})
		if err == nil {
			t.Errorf("value %v, fs %v: got nil, want error", test.value, test.fs)
		}
	}

	// bad field type
	if _, err := convertBasicType("", FieldType("BAD")); err == nil {
		t.Error("got nil, want error")
	}
}

func TestValuesSaverConvertsToMap(t *testing.T) {
	testCases := []struct {
		vs           ValuesSaver
		wantInsertID string
		wantRow      map[string]Value
	}{
		{
			vs: ValuesSaver{
				Schema: Schema{
					{Name: "intField", Type: IntegerFieldType},
					{Name: "strField", Type: StringFieldType},
					{Name: "dtField", Type: DateTimeFieldType},
					{Name: "nField", Type: NumericFieldType},
					{Name: "geoField", Type: GeographyFieldType},
				},
				InsertID: "iid",
				Row: []Value{1, "a",
					civil.DateTime{
						Date: civil.Date{Year: 1, Month: 2, Day: 3},
						Time: civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 7000}},
					big.NewRat(123456789000, 1e9),
					testGeography,
				},
			},
			wantInsertID: "iid",
			wantRow: map[string]Value{
				"intField": 1,
				"strField": "a",
				"dtField":  "0001-02-03 04:05:06.000007",
				"nField":   "123.456789000",
				"geoField": testGeography,
			},
		},
		{
			vs: ValuesSaver{
				Schema: Schema{
					{Name: "intField", Type: IntegerFieldType},
					{
						Name: "recordField",
						Type: RecordFieldType,
						Schema: Schema{
							{Name: "nestedInt", Type: IntegerFieldType, Repeated: true},
						},
					},
				},
				InsertID: "iid",
				Row:      []Value{1, []Value{[]Value{2, 3}}},
			},
			wantInsertID: "iid",
			wantRow: map[string]Value{
				"intField": 1,
				"recordField": map[string]Value{
					"nestedInt": []Value{2, 3},
				},
			},
		},
		{ // repeated nested field
			vs: ValuesSaver{
				Schema: Schema{
					{
						Name: "records",
						Type: RecordFieldType,
						Schema: Schema{
							{Name: "x", Type: IntegerFieldType},
							{Name: "y", Type: IntegerFieldType},
						},
						Repeated: true,
					},
				},
				InsertID: "iid",
				Row: []Value{ // a row is a []Value
					[]Value{ // repeated field's value is a []Value
						[]Value{1, 2}, // first record of the repeated field
						[]Value{3, 4}, // second record
					},
				},
			},
			wantInsertID: "iid",
			wantRow: map[string]Value{
				"records": []Value{
					map[string]Value{"x": 1, "y": 2},
					map[string]Value{"x": 3, "y": 4},
				},
			},
		},
	}
	for _, tc := range testCases {
		gotRow, gotInsertID, err := tc.vs.Save()
		if err != nil {
			t.Errorf("Expected successful save; got: %v", err)
			continue
		}
		if !testutil.Equal(gotRow, tc.wantRow) {
			t.Errorf("%v row:\ngot:\n%+v\nwant:\n%+v", tc.vs, gotRow, tc.wantRow)
		}
		if !testutil.Equal(gotInsertID, tc.wantInsertID) {
			t.Errorf("%v ID:\ngot:\n%+v\nwant:\n%+v", tc.vs, gotInsertID, tc.wantInsertID)
		}
	}
}

func TestValuesToMapErrors(t *testing.T) {
	for _, test := range []struct {
		values []Value
		schema Schema
	}{
		{ // mismatched length
			[]Value{1},
			Schema{},
		},
		{ // nested record not a slice
			[]Value{1},
			Schema{{Type: RecordFieldType}},
		},
		{ // nested record mismatched length
			[]Value{[]Value{1}},
			Schema{{Type: RecordFieldType}},
		},
		{ // nested repeated record not a slice
			[]Value{[]Value{1}},
			Schema{{Type: RecordFieldType, Repeated: true}},
		},
		{ // nested repeated record mismatched length
			[]Value{[]Value{[]Value{1}}},
			Schema{{Type: RecordFieldType, Repeated: true}},
		},
	} {
		_, err := valuesToMap(test.values, test.schema)
		if err == nil {
			t.Errorf("%v, %v: got nil, want error", test.values, test.schema)
		}
	}
}

func TestStructSaver(t *testing.T) {
	schema := Schema{
		{Name: "s", Type: StringFieldType},
		{Name: "r", Type: IntegerFieldType, Repeated: true},
		{Name: "t", Type: TimeFieldType},
		{Name: "tr", Type: TimeFieldType, Repeated: true},
		{Name: "nested", Type: RecordFieldType, Schema: Schema{
			{Name: "b", Type: BooleanFieldType},
		}},
		{Name: "rnested", Type: RecordFieldType, Repeated: true, Schema: Schema{
			{Name: "b", Type: BooleanFieldType},
		}},
		{Name: "p", Type: IntegerFieldType, Required: false},
		{Name: "n", Type: NumericFieldType, Required: false},
		{Name: "nr", Type: NumericFieldType, Repeated: true},
		{Name: "g", Type: GeographyFieldType, Required: false},
		{Name: "gr", Type: GeographyFieldType, Repeated: true},
	}

	type (
		N struct{ B bool }
		T struct {
			S       string
			R       []int
			T       civil.Time
			TR      []civil.Time
			Nested  *N
			Rnested []*N
			P       NullInt64
			N       *big.Rat
			NR      []*big.Rat
			G       NullGeography
			GR      []string // Repeated Geography
		}
	)

	check := func(msg string, in interface{}, want map[string]Value) {
		ss := StructSaver{
			Schema:   schema,
			InsertID: "iid",
			Struct:   in,
		}
		got, gotIID, err := ss.Save()
		if err != nil {
			t.Fatalf("%s: %v", msg, err)
		}
		if wantIID := "iid"; gotIID != wantIID {
			t.Errorf("%s: InsertID: got %q, want %q", msg, gotIID, wantIID)
		}
		if diff := testutil.Diff(got, want); diff != "" {
			t.Errorf("%s: %s", msg, diff)
		}
	}

	ct1 := civil.Time{Hour: 1, Minute: 2, Second: 3, Nanosecond: 4000}
	ct2 := civil.Time{Hour: 5, Minute: 6, Second: 7, Nanosecond: 8000}
	in := T{
		S:       "x",
		R:       []int{1, 2},
		T:       ct1,
		TR:      []civil.Time{ct1, ct2},
		Nested:  &N{B: true},
		Rnested: []*N{{true}, {false}},
		P:       NullInt64{Valid: true, Int64: 17},
		N:       big.NewRat(123456, 1000),
		NR:      []*big.Rat{big.NewRat(3, 1), big.NewRat(56789, 1e5)},
		G:       NullGeography{Valid: true, GeographyVal: "POINT(-122.350220 47.649154)"},
		GR:      []string{"POINT(-122.350220 47.649154)", "POINT(-122.198939 47.669865)"},
	}
	want := map[string]Value{
		"s":       "x",
		"r":       []int{1, 2},
		"t":       "01:02:03.000004",
		"tr":      []string{"01:02:03.000004", "05:06:07.000008"},
		"nested":  map[string]Value{"b": true},
		"rnested": []Value{map[string]Value{"b": true}, map[string]Value{"b": false}},
		"p":       NullInt64{Valid: true, Int64: 17},
		"n":       "123.456000000",
		"nr":      []string{"3.000000000", "0.567890000"},
		"g":       NullGeography{Valid: true, GeographyVal: "POINT(-122.350220 47.649154)"},
		"gr":      []string{"POINT(-122.350220 47.649154)", "POINT(-122.198939 47.669865)"},
	}
	check("all values", in, want)
	check("all values, ptr", &in, want)
	check("empty struct", T{}, map[string]Value{"s": "", "t": "00:00:00", "p": NullInt64{}, "g": NullGeography{}})

	// Missing and extra fields ignored.
	type T2 struct {
		S string
		// missing R, Nested, RNested
		Extra int
	}
	check("missing and extra", T2{S: "x"}, map[string]Value{"s": "x"})

	check("nils in slice", T{Rnested: []*N{{true}, nil, {false}}},
		map[string]Value{
			"s":       "",
			"t":       "00:00:00",
			"p":       NullInt64{},
			"g":       NullGeography{},
			"rnested": []Value{map[string]Value{"b": true}, map[string]Value(nil), map[string]Value{"b": false}},
		})

	check("zero-length repeated", T{Rnested: []*N{}},
		map[string]Value{
			"rnested": []Value{},
			"s":       "",
			"t":       "00:00:00",
			"p":       NullInt64{},
			"g":       NullGeography{},
		})
}

func TestStructSaverErrors(t *testing.T) {
	type (
		badField struct {
			I int `bigquery:"@"`
		}
		badR  struct{ R int }
		badRN struct{ R []int }
	)

	for i, test := range []struct {
		inputStruct interface{}
		schema      Schema
	}{
		{0, nil},           // not a struct
		{&badField{}, nil}, // bad field name
		{&badR{}, Schema{{Name: "r", Repeated: true}}},        // repeated field has bad type
		{&badR{}, Schema{{Name: "r", Type: RecordFieldType}}}, // nested field has bad type
		{&badRN{[]int{0}}, // nested repeated field has bad type
			Schema{{Name: "r", Type: RecordFieldType, Repeated: true}}},
	} {
		ss := &StructSaver{Struct: test.inputStruct, Schema: test.schema}
		_, _, err := ss.Save()
		if err == nil {
			t.Errorf("#%d, %v, %v: got nil, want error", i, test.inputStruct, test.schema)
		}
	}
}

func TestNumericString(t *testing.T) {
	for _, test := range []struct {
		in   *big.Rat
		want string
	}{
		{big.NewRat(2, 3), "0.666666667"}, // round to 9 places
		{big.NewRat(1, 2), "0.500000000"},
		{big.NewRat(1, 2*1e8), "0.000000005"},
		{big.NewRat(5, 1e10), "0.000000001"},   // round up the 5 in the 10th decimal place
		{big.NewRat(-5, 1e10), "-0.000000001"}, // round half away from zero
	} {
		got := NumericString(test.in)
		if got != test.want {
			t.Errorf("%v: got %q, want %q", test.in, got, test.want)
		}
	}
}

func TestConvertRows(t *testing.T) {
	schema := Schema{
		{Type: StringFieldType},
		{Type: IntegerFieldType},
		{Type: FloatFieldType},
		{Type: BooleanFieldType},
		{Type: GeographyFieldType},
	}
	rows := []*bq.TableRow{
		{F: []*bq.TableCell{
			{V: "a"},
			{V: "1"},
			{V: "1.2"},
			{V: "true"},
			{V: "POINT(-122.350220 47.649154)"},
		}},
		{F: []*bq.TableCell{
			{V: "b"},
			{V: "2"},
			{V: "2.2"},
			{V: "false"},
			{V: "POINT(-122.198939 47.669865)"},
		}},
	}
	want := [][]Value{
		{"a", int64(1), 1.2, true, "POINT(-122.350220 47.649154)"},
		{"b", int64(2), 2.2, false, "POINT(-122.198939 47.669865)"},
	}
	got, err := convertRows(rows, schema)
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	if !testutil.Equal(got, want) {
		t.Errorf("\ngot  %v\nwant %v", got, want)
	}

	rows[0].F[0].V = 1
	_, err = convertRows(rows, schema)
	if err == nil {
		t.Error("got nil, want error")
	}
}

func TestValueList(t *testing.T) {
	schema := Schema{
		{Name: "s", Type: StringFieldType},
		{Name: "i", Type: IntegerFieldType},
		{Name: "f", Type: FloatFieldType},
		{Name: "b", Type: BooleanFieldType},
	}
	want := []Value{"x", 7, 3.14, true}
	var got []Value
	vl := (*valueList)(&got)
	if err := vl.Load(want, schema); err != nil {
		t.Fatal(err)
	}

	if !testutil.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	// Load truncates, not appends.
	// https://github.com/googleapis/google-cloud-go/issues/437
	if err := vl.Load(want, schema); err != nil {
		t.Fatal(err)
	}
	if !testutil.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestValueMap(t *testing.T) {
	ns := Schema{
		{Name: "x", Type: IntegerFieldType},
		{Name: "y", Type: IntegerFieldType},
	}
	schema := Schema{
		{Name: "s", Type: StringFieldType},
		{Name: "i", Type: IntegerFieldType},
		{Name: "f", Type: FloatFieldType},
		{Name: "b", Type: BooleanFieldType},
		{Name: "n", Type: RecordFieldType, Schema: ns},
		{Name: "rn", Type: RecordFieldType, Schema: ns, Repeated: true},
	}
	in := []Value{"x", 7, 3.14, true,
		[]Value{1, 2},
		[]Value{[]Value{3, 4}, []Value{5, 6}},
	}
	var vm valueMap
	if err := vm.Load(in, schema); err != nil {
		t.Fatal(err)
	}
	want := map[string]Value{
		"s": "x",
		"i": 7,
		"f": 3.14,
		"b": true,
		"n": map[string]Value{"x": 1, "y": 2},
		"rn": []Value{
			map[string]Value{"x": 3, "y": 4},
			map[string]Value{"x": 5, "y": 6},
		},
	}
	if !testutil.Equal(vm, valueMap(want)) {
		t.Errorf("got\n%+v\nwant\n%+v", vm, want)
	}

	in = make([]Value, len(schema))
	want = map[string]Value{
		"s":  nil,
		"i":  nil,
		"f":  nil,
		"b":  nil,
		"n":  nil,
		"rn": nil,
	}
	var vm2 valueMap
	if err := vm2.Load(in, schema); err != nil {
		t.Fatal(err)
	}
	if !testutil.Equal(vm2, valueMap(want)) {
		t.Errorf("got\n%+v\nwant\n%+v", vm2, want)
	}
}

var (
	// For testing StructLoader
	schema2 = Schema{
		{Name: "s", Type: StringFieldType},
		{Name: "s2", Type: StringFieldType},
		{Name: "by", Type: BytesFieldType},
		{Name: "I", Type: IntegerFieldType},
		{Name: "U", Type: IntegerFieldType},
		{Name: "F", Type: FloatFieldType},
		{Name: "B", Type: BooleanFieldType},
		{Name: "TS", Type: TimestampFieldType},
		{Name: "D", Type: DateFieldType},
		{Name: "T", Type: TimeFieldType},
		{Name: "DT", Type: DateTimeFieldType},
		{Name: "N", Type: NumericFieldType},
		{Name: "G", Type: GeographyFieldType},
		{Name: "nested", Type: RecordFieldType, Schema: Schema{
			{Name: "nestS", Type: StringFieldType},
			{Name: "nestI", Type: IntegerFieldType},
		}},
		{Name: "t", Type: StringFieldType},
	}

	testTimestamp = time.Date(2016, 11, 5, 7, 50, 22, 8, time.UTC)
	testDate      = civil.Date{Year: 2016, Month: 11, Day: 5}
	testTime      = civil.Time{Hour: 7, Minute: 50, Second: 22, Nanosecond: 8}
	testDateTime  = civil.DateTime{Date: testDate, Time: testTime}
	testNumeric   = big.NewRat(123, 456)
	// testGeography is a WKT string representing a single point.
	testGeography = "POINT(-122.350220 47.649154)"

	testValues = []Value{"x", "y", []byte{1, 2, 3}, int64(7), int64(8), 3.14, true,
		testTimestamp, testDate, testTime, testDateTime, testNumeric, testGeography,
		[]Value{"nested", int64(17)}, "z"}
)

type testStruct1 struct {
	B bool
	I int
	U uint16
	times
	S      string
	S2     String
	By     []byte
	F      float64
	N      *big.Rat
	G      string
	Nested nested
	Tagged string `bigquery:"t"`
}

type String string

type nested struct {
	NestS string
	NestI int
}

type times struct {
	TS time.Time
	T  civil.Time
	D  civil.Date
	DT civil.DateTime
}

func TestStructLoader(t *testing.T) {
	var ts1 testStruct1
	mustLoad(t, &ts1, schema2, testValues)
	// Note: the schema field named "s" gets matched to the exported struct
	// field "S", not the unexported "s".
	want := &testStruct1{
		B:      true,
		I:      7,
		U:      8,
		F:      3.14,
		times:  times{TS: testTimestamp, T: testTime, D: testDate, DT: testDateTime},
		S:      "x",
		S2:     "y",
		By:     []byte{1, 2, 3},
		N:      big.NewRat(123, 456),
		G:      testGeography,
		Nested: nested{NestS: "nested", NestI: 17},
		Tagged: "z",
	}
	if diff := testutil.Diff(&ts1, want, cmp.AllowUnexported(testStruct1{})); diff != "" {
		t.Error(diff)
	}

	// Test pointers to nested structs.
	type nestedPtr struct{ Nested *nested }
	var np nestedPtr
	mustLoad(t, &np, schema2, testValues)
	want2 := &nestedPtr{Nested: &nested{NestS: "nested", NestI: 17}}
	if diff := testutil.Diff(&np, want2); diff != "" {
		t.Error(diff)
	}

	// Existing values should be reused.
	nst := &nested{NestS: "x", NestI: -10}
	np = nestedPtr{Nested: nst}
	mustLoad(t, &np, schema2, testValues)
	if diff := testutil.Diff(&np, want2); diff != "" {
		t.Error(diff)
	}
	if np.Nested != nst {
		t.Error("nested struct pointers not equal")
	}
}

type repStruct struct {
	Nums      []int
	ShortNums [2]int // to test truncation
	LongNums  [5]int // to test padding with zeroes
	Nested    []*nested
}

var (
	repSchema = Schema{
		{Name: "nums", Type: IntegerFieldType, Repeated: true},
		{Name: "shortNums", Type: IntegerFieldType, Repeated: true},
		{Name: "longNums", Type: IntegerFieldType, Repeated: true},
		{Name: "nested", Type: RecordFieldType, Repeated: true, Schema: Schema{
			{Name: "nestS", Type: StringFieldType},
			{Name: "nestI", Type: IntegerFieldType},
		}},
	}
	v123      = []Value{int64(1), int64(2), int64(3)}
	repValues = []Value{v123, v123, v123,
		[]Value{
			[]Value{"x", int64(1)},
			[]Value{"y", int64(2)},
		},
	}
)

func TestStructLoaderRepeated(t *testing.T) {
	var r1 repStruct
	mustLoad(t, &r1, repSchema, repValues)
	want := repStruct{
		Nums:      []int{1, 2, 3},
		ShortNums: [...]int{1, 2}, // extra values discarded
		LongNums:  [...]int{1, 2, 3, 0, 0},
		Nested:    []*nested{{"x", 1}, {"y", 2}},
	}
	if diff := testutil.Diff(r1, want); diff != "" {
		t.Error(diff)
	}
	r2 := repStruct{
		Nums:     []int{-1, -2, -3, -4, -5},    // truncated to zero and appended to
		LongNums: [...]int{-1, -2, -3, -4, -5}, // unset elements are zeroed
	}
	mustLoad(t, &r2, repSchema, repValues)
	if diff := testutil.Diff(r2, want); diff != "" {
		t.Error(diff)
	}
	if got, want := cap(r2.Nums), 5; got != want {
		t.Errorf("cap(r2.Nums) = %d, want %d", got, want)
	}

	// Short slice case.
	r3 := repStruct{Nums: []int{-1}}
	mustLoad(t, &r3, repSchema, repValues)
	if diff := testutil.Diff(r3, want); diff != "" {
		t.Error(diff)
	}
	if got, want := cap(r3.Nums), 3; got != want {
		t.Errorf("cap(r3.Nums) = %d, want %d", got, want)
	}
}

type testStructNullable struct {
	String    NullString
	Bytes     []byte
	Integer   NullInt64
	Float     NullFloat64
	Boolean   NullBool
	Timestamp NullTimestamp
	Date      NullDate
	Time      NullTime
	DateTime  NullDateTime
	Numeric   *big.Rat
	Geography NullGeography
	Record    *subNullable
}

type subNullable struct {
	X NullInt64
}

var testStructNullableSchema = Schema{
	{Name: "String", Type: StringFieldType, Required: false},
	{Name: "Bytes", Type: BytesFieldType, Required: false},
	{Name: "Integer", Type: IntegerFieldType, Required: false},
	{Name: "Float", Type: FloatFieldType, Required: false},
	{Name: "Boolean", Type: BooleanFieldType, Required: false},
	{Name: "Timestamp", Type: TimestampFieldType, Required: false},
	{Name: "Date", Type: DateFieldType, Required: false},
	{Name: "Time", Type: TimeFieldType, Required: false},
	{Name: "DateTime", Type: DateTimeFieldType, Required: false},
	{Name: "Numeric", Type: NumericFieldType, Required: false},
	{Name: "Geography", Type: GeographyFieldType, Required: false},
	{Name: "Record", Type: RecordFieldType, Required: false, Schema: Schema{
		{Name: "X", Type: IntegerFieldType, Required: false},
	}},
}

func TestStructLoaderNullable(t *testing.T) {
	var ts testStructNullable
	nilVals := make([]Value, len(testStructNullableSchema))
	mustLoad(t, &ts, testStructNullableSchema, nilVals)
	want := testStructNullable{}
	if diff := testutil.Diff(ts, want); diff != "" {
		t.Error(diff)
	}

	nonnilVals := []Value{"x", []byte{1, 2, 3}, int64(1), 2.3, true, testTimestamp, testDate, testTime,
		testDateTime, big.NewRat(1, 2), testGeography, []Value{int64(4)}}

	// All ts fields are nil. Loading non-nil values will cause them all to
	// be allocated.
	mustLoad(t, &ts, testStructNullableSchema, nonnilVals)
	want = testStructNullable{
		String:    NullString{StringVal: "x", Valid: true},
		Bytes:     []byte{1, 2, 3},
		Integer:   NullInt64{Int64: 1, Valid: true},
		Float:     NullFloat64{Float64: 2.3, Valid: true},
		Boolean:   NullBool{Bool: true, Valid: true},
		Timestamp: NullTimestamp{Timestamp: testTimestamp, Valid: true},
		Date:      NullDate{Date: testDate, Valid: true},
		Time:      NullTime{Time: testTime, Valid: true},
		DateTime:  NullDateTime{DateTime: testDateTime, Valid: true},
		Numeric:   big.NewRat(1, 2),
		Geography: NullGeography{GeographyVal: testGeography, Valid: true},
		Record:    &subNullable{X: NullInt64{Int64: 4, Valid: true}},
	}
	if diff := testutil.Diff(ts, want); diff != "" {
		t.Error(diff)
	}

	// Struct pointers are reused, byte slices are not.
	want = ts
	want.Bytes = []byte{17}
	vals2 := []Value{nil, []byte{17}, nil, nil, nil, nil, nil, nil, nil, nil, nil, []Value{int64(7)}}
	mustLoad(t, &ts, testStructNullableSchema, vals2)
	if ts.Record != want.Record {
		t.Error("record pointers not identical")
	}
}

func TestStructLoaderOverflow(t *testing.T) {
	type S struct {
		I int16
		U uint16
		F float32
	}
	schema := Schema{
		{Name: "I", Type: IntegerFieldType},
		{Name: "U", Type: IntegerFieldType},
		{Name: "F", Type: FloatFieldType},
	}
	var s S
	z64 := int64(0)
	for _, vals := range [][]Value{
		{int64(math.MaxInt16 + 1), z64, 0},
		{z64, int64(math.MaxInt32), 0},
		{z64, int64(-1), 0},
		{z64, z64, math.MaxFloat32 * 2},
	} {
		if err := load(&s, schema, vals); err == nil {
			t.Errorf("%+v: got nil, want error", vals)
		}
	}
}

func TestStructLoaderFieldOverlap(t *testing.T) {
	// It's OK if the struct has fields that the schema does not, and vice versa.
	type S1 struct {
		I int
		X [][]int // not in the schema; does not even correspond to a valid BigQuery type
		// many schema fields missing
	}
	var s1 S1
	if err := load(&s1, schema2, testValues); err != nil {
		t.Fatal(err)
	}
	want1 := S1{I: 7}
	if diff := testutil.Diff(s1, want1); diff != "" {
		t.Error(diff)
	}

	// It's even valid to have no overlapping fields at all.
	type S2 struct{ Z int }

	var s2 S2
	mustLoad(t, &s2, schema2, testValues)
	want2 := S2{}
	if diff := testutil.Diff(s2, want2); diff != "" {
		t.Error(diff)
	}
}

func TestStructLoaderErrors(t *testing.T) {
	check := func(sp interface{}) {
		var sl structLoader
		err := sl.set(sp, schema2)
		if err == nil {
			t.Errorf("%T: got nil, want error", sp)
		}
	}

	type bad1 struct{ F int32 } // wrong type for FLOAT column
	check(&bad1{})

	type bad2 struct{ I uint } // unsupported integer type
	check(&bad2{})

	type bad3 struct {
		I int `bigquery:"@"`
	} // bad field name
	check(&bad3{})

	type bad4 struct{ Nested int } // non-struct for nested field
	check(&bad4{})

	type bad5 struct{ Nested struct{ NestS int } } // bad nested struct
	check(&bad5{})

	bad6 := &struct{ Nums int }{} // non-slice for repeated field
	sl := structLoader{}
	err := sl.set(bad6, repSchema)
	if err == nil {
		t.Errorf("%T: got nil, want error", bad6)
	}

	// sl.set's error is sticky, even with good input.
	err2 := sl.set(&repStruct{}, repSchema)
	if err2 != err {
		t.Errorf("%v != %v, expected equal", err2, err)
	}
	// sl.Load is similarly sticky
	err2 = sl.Load(nil, nil)
	if err2 != err {
		t.Errorf("%v != %v, expected equal", err2, err)
	}

	// Null values.
	schema := Schema{
		{Name: "i", Type: IntegerFieldType},
		{Name: "f", Type: FloatFieldType},
		{Name: "b", Type: BooleanFieldType},
		{Name: "s", Type: StringFieldType},
		{Name: "d", Type: DateFieldType},
		{Name: "r", Type: RecordFieldType, Schema: Schema{{Name: "X", Type: IntegerFieldType}}},
	}
	type s struct {
		I int
		F float64
		B bool
		S string
		D civil.Date
	}
	vals := []Value{int64(0), 0.0, false, "", testDate}
	mustLoad(t, &s{}, schema, vals)
	for i, e := range vals {
		vals[i] = nil
		got := load(&s{}, schema, vals)
		if got != errNoNulls {
			t.Errorf("#%d: got %v, want %v", i, got, errNoNulls)
		}
		vals[i] = e
	}

	// Using more than one struct type with the same structLoader.
	type different struct {
		B bool
		I int
		times
		S    string
		Nums []int
	}

	sl = structLoader{}
	if err := sl.set(&testStruct1{}, schema2); err != nil {
		t.Fatal(err)
	}
	err = sl.set(&different{}, schema2)
	if err == nil {
		t.Error("different struct types: got nil, want error")
	}
}

func mustLoad(t *testing.T, pval interface{}, schema Schema, vals []Value) {
	if err := load(pval, schema, vals); err != nil {
		t.Fatalf("loading: %v", err)
	}
}

func load(pval interface{}, schema Schema, vals []Value) error {
	var sl structLoader
	if err := sl.set(pval, schema); err != nil {
		return err
	}
	return sl.Load(vals, nil)
}

func BenchmarkStructLoader_NoCompile(b *testing.B) {
	benchmarkStructLoader(b, false)
}

func BenchmarkStructLoader_Compile(b *testing.B) {
	benchmarkStructLoader(b, true)
}

func benchmarkStructLoader(b *testing.B, compile bool) {
	var ts1 testStruct1
	for i := 0; i < b.N; i++ {
		var sl structLoader
		for j := 0; j < 10; j++ {
			if err := load(&ts1, schema2, testValues); err != nil {
				b.Fatal(err)
			}
			if !compile {
				sl.typ = nil
			}
		}
	}
}
