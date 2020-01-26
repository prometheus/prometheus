// Copyright 2016 Google LLC
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
	"context"
	"errors"
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	bq "google.golang.org/api/bigquery/v2"
)

var scalarTests = []struct {
	val      interface{}            // The Go value
	wantVal  string                 // paramValue's desired output
	wantType *bq.QueryParameterType // paramType's desired output
}{
	{int64(0), "0", int64ParamType},
	{3.14, "3.14", float64ParamType},
	{3.14159e-87, "3.14159e-87", float64ParamType},
	{true, "true", boolParamType},
	{"string", "string", stringParamType},
	{"\u65e5\u672c\u8a9e\n", "\u65e5\u672c\u8a9e\n", stringParamType},
	{math.NaN(), "NaN", float64ParamType},
	{[]byte("foo"), "Zm9v", bytesParamType}, // base64 encoding of "foo"
	{time.Date(2016, 3, 20, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720)),
		"2016-03-20 04:22:09.000005-01:02",
		timestampParamType},
	{civil.Date{Year: 2016, Month: 3, Day: 20}, "2016-03-20", dateParamType},
	{civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000}, "04:05:06.789000", timeParamType},
	{civil.DateTime{Date: civil.Date{Year: 2016, Month: 3, Day: 20}, Time: civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000}},
		"2016-03-20 04:05:06.789000",
		dateTimeParamType},
	{big.NewRat(12345, 1000), "12.345000000", numericParamType},
}

type (
	S1 struct {
		A int
		B *S2
		C bool
	}
	S2 struct {
		D string
	}
)

var (
	s1 = S1{
		A: 1,
		B: &S2{D: "s"},
		C: true,
	}

	s1ParamType = &bq.QueryParameterType{
		Type: "STRUCT",
		StructTypes: []*bq.QueryParameterTypeStructTypes{
			{Name: "A", Type: int64ParamType},
			{Name: "B", Type: &bq.QueryParameterType{
				Type: "STRUCT",
				StructTypes: []*bq.QueryParameterTypeStructTypes{
					{Name: "D", Type: stringParamType},
				},
			}},
			{Name: "C", Type: boolParamType},
		},
	}

	s1ParamValue = bq.QueryParameterValue{
		StructValues: map[string]bq.QueryParameterValue{
			"A": sval("1"),
			"B": {
				StructValues: map[string]bq.QueryParameterValue{
					"D": sval("s"),
				},
			},
			"C": sval("true"),
		},
	}

	s1ParamReturnValue = map[string]interface{}{
		"A": int64(1),
		"B": map[string]interface{}{"D": "s"},
		"C": true,
	}
)

func sval(s string) bq.QueryParameterValue {
	return bq.QueryParameterValue{Value: s}
}

func TestParamValueScalar(t *testing.T) {
	for _, test := range scalarTests {
		got, err := paramValue(reflect.ValueOf(test.val))
		if err != nil {
			t.Errorf("%v: got %v, want nil", test.val, err)
			continue
		}
		want := sval(test.wantVal)
		if !testutil.Equal(got, want) {
			t.Errorf("%v:\ngot  %+v\nwant %+v", test.val, got, want)
		}
	}
}

func TestParamValueArray(t *testing.T) {
	qpv := bq.QueryParameterValue{ArrayValues: []*bq.QueryParameterValue{
		{Value: "1"},
		{Value: "2"},
	},
	}
	for _, test := range []struct {
		val  interface{}
		want bq.QueryParameterValue
	}{
		{[]int(nil), bq.QueryParameterValue{}},
		{[]int{}, bq.QueryParameterValue{}},
		{[]int{1, 2}, qpv},
		{[2]int{1, 2}, qpv},
	} {
		got, err := paramValue(reflect.ValueOf(test.val))
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(got, test.want) {
			t.Errorf("%#v:\ngot  %+v\nwant %+v", test.val, got, test.want)
		}
	}
}

func TestParamValueStruct(t *testing.T) {
	got, err := paramValue(reflect.ValueOf(s1))
	if err != nil {
		t.Fatal(err)
	}
	if !testutil.Equal(got, s1ParamValue) {
		t.Errorf("got  %+v\nwant %+v", got, s1ParamValue)
	}
}

func TestParamValueErrors(t *testing.T) {
	// paramValue lets a few invalid types through, but paramType catches them.
	// Since we never call one without the other that's fine.
	for _, val := range []interface{}{nil, new([]int)} {
		_, err := paramValue(reflect.ValueOf(val))
		if err == nil {
			t.Errorf("%v (%T): got nil, want error", val, val)
		}
	}
}

func TestParamType(t *testing.T) {
	for _, test := range scalarTests {
		got, err := paramType(reflect.TypeOf(test.val))
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(got, test.wantType) {
			t.Errorf("%v (%T): got %v, want %v", test.val, test.val, got, test.wantType)
		}
	}
	for _, test := range []struct {
		val  interface{}
		want *bq.QueryParameterType
	}{
		{uint32(32767), int64ParamType},
		{[]byte("foo"), bytesParamType},
		{[]int{}, &bq.QueryParameterType{Type: "ARRAY", ArrayType: int64ParamType}},
		{[3]bool{}, &bq.QueryParameterType{Type: "ARRAY", ArrayType: boolParamType}},
		{S1{}, s1ParamType},
	} {
		got, err := paramType(reflect.TypeOf(test.val))
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(got, test.want) {
			t.Errorf("%v (%T): got %v, want %v", test.val, test.val, got, test.want)
		}
	}
}

func TestParamTypeErrors(t *testing.T) {
	for _, val := range []interface{}{
		nil, uint(0), new([]int), make(chan int),
	} {
		_, err := paramType(reflect.TypeOf(val))
		if err == nil {
			t.Errorf("%v (%T): got nil, want error", val, val)
		}
	}
}

func TestConvertParamValue(t *testing.T) {
	// Scalars.
	for _, test := range scalarTests {
		pval, err := paramValue(reflect.ValueOf(test.val))
		if err != nil {
			t.Fatal(err)
		}
		ptype, err := paramType(reflect.TypeOf(test.val))
		if err != nil {
			t.Fatal(err)
		}
		got, err := convertParamValue(&pval, ptype)
		if err != nil {
			t.Fatalf("convertParamValue(%+v, %+v): %v", pval, ptype, err)
		}
		if !testutil.Equal(got, test.val) {
			t.Errorf("%#v: got %#v", test.val, got)
		}
	}
	// Arrays.
	for _, test := range []struct {
		pval *bq.QueryParameterValue
		want []interface{}
	}{
		{
			&bq.QueryParameterValue{},
			nil,
		},
		{
			&bq.QueryParameterValue{
				ArrayValues: []*bq.QueryParameterValue{{Value: "1"}, {Value: "2"}},
			},
			[]interface{}{int64(1), int64(2)},
		},
	} {
		ptype := &bq.QueryParameterType{Type: "ARRAY", ArrayType: int64ParamType}
		got, err := convertParamValue(test.pval, ptype)
		if err != nil {
			t.Fatalf("%+v: %v", test.pval, err)
		}
		if !testutil.Equal(got, test.want) {
			t.Errorf("%+v: got %+v, want %+v", test.pval, got, test.want)
		}
	}
	// Structs.
	got, err := convertParamValue(&s1ParamValue, s1ParamType)
	if err != nil {
		t.Fatal(err)
	}
	if !testutil.Equal(got, s1ParamReturnValue) {
		t.Errorf("got %+v, want %+v", got, s1ParamReturnValue)
	}
}

func TestIntegration_ScalarParam(t *testing.T) {
	roundToMicros := cmp.Transformer("RoundToMicros",
		func(t time.Time) time.Time { return t.Round(time.Microsecond) })
	c := getClient(t)
	for _, test := range scalarTests {
		gotData, gotParam, err := paramRoundTrip(c, test.val)
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(gotData, test.val, roundToMicros) {
			t.Errorf("\ngot  %#v (%T)\nwant %#v (%T)", gotData, gotData, test.val, test.val)
		}
		if !testutil.Equal(gotParam, test.val, roundToMicros) {
			t.Errorf("\ngot  %#v (%T)\nwant %#v (%T)", gotParam, gotParam, test.val, test.val)
		}
	}
}

func TestIntegration_OtherParam(t *testing.T) {
	c := getClient(t)
	for _, test := range []struct {
		val       interface{}
		wantData  interface{}
		wantParam interface{}
	}{
		{[]int(nil), []Value(nil), []interface{}(nil)},
		{[]int{}, []Value(nil), []interface{}(nil)},
		{
			[]int{1, 2},
			[]Value{int64(1), int64(2)},
			[]interface{}{int64(1), int64(2)},
		},
		{
			[3]int{1, 2, 3},
			[]Value{int64(1), int64(2), int64(3)},
			[]interface{}{int64(1), int64(2), int64(3)},
		},
		{
			S1{},
			[]Value{int64(0), nil, false},
			map[string]interface{}{
				"A": int64(0),
				"B": nil,
				"C": false,
			},
		},
		{
			s1,
			[]Value{int64(1), []Value{"s"}, true},
			s1ParamReturnValue,
		},
	} {
		gotData, gotParam, err := paramRoundTrip(c, test.val)
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(gotData, test.wantData) {
			t.Errorf("%#v:\ngot  %#v (%T)\nwant %#v (%T)",
				test.val, gotData, gotData, test.wantData, test.wantData)
		}
		if !testutil.Equal(gotParam, test.wantParam) {
			t.Errorf("%#v:\ngot  %#v (%T)\nwant %#v (%T)",
				test.val, gotParam, gotParam, test.wantParam, test.wantParam)
		}
	}
}

// paramRoundTrip passes x as a query parameter to BigQuery. It returns
// the resulting data value from running the query and the parameter value from
// the returned job configuration.
func paramRoundTrip(c *Client, x interface{}) (data Value, param interface{}, err error) {
	ctx := context.Background()
	q := c.Query("select ?")
	q.Parameters = []QueryParameter{{Value: x}}
	job, err := q.Run(ctx)
	if err != nil {
		return nil, nil, err
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, nil, err
	}
	var val []Value
	err = it.Next(&val)
	if err != nil {
		return nil, nil, err
	}
	if len(val) != 1 {
		return nil, nil, errors.New("wrong number of values")
	}
	conf, err := job.Config()
	if err != nil {
		return nil, nil, err
	}
	return val[0], conf.(*QueryConfig).Parameters[0].Value, nil
}

func TestQueryParameter_toBQ(t *testing.T) {
	tests := []struct {
		in   QueryParameter
		want []string
	}{
		{
			in:   QueryParameter{Name: "name", Value: ""},
			want: []string{"Value"},
		},
	}

	for _, test := range tests {
		q, err := test.in.toBQ()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		got := q.ParameterValue.ForceSendFields
		if !cmp.Equal(test.want, got) {
			t.Fatalf("want %v, got %v", test.want, got)
		}
	}
}
