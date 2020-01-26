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
	"encoding/json"
	"reflect"
	"testing"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/internal/testutil"
)

var (
	nullsTestTime     = civil.Time{Hour: 7, Minute: 50, Second: 22, Nanosecond: 1000}
	nullsTestDateTime = civil.DateTime{Date: civil.Date{Year: 2016, Month: 11, Day: 5}, Time: nullsTestTime}
)

func TestNullsJSON(t *testing.T) {
	for _, test := range []struct {
		in   interface{}
		want string
	}{
		{&NullInt64{Valid: true, Int64: 3}, `3`},
		{&NullFloat64{Valid: true, Float64: 3.14}, `3.14`},
		{&NullBool{Valid: true, Bool: true}, `true`},
		{&NullString{Valid: true, StringVal: "foo"}, `"foo"`},
		{&NullGeography{Valid: true, GeographyVal: "ST_GEOPOINT(47.649154, -122.350220)"}, `"ST_GEOPOINT(47.649154, -122.350220)"`},
		{&NullTimestamp{Valid: true, Timestamp: testTimestamp}, `"2016-11-05T07:50:22.000000008Z"`},
		{&NullDate{Valid: true, Date: testDate}, `"2016-11-05"`},
		{&NullTime{Valid: true, Time: nullsTestTime}, `"07:50:22.000001"`},
		{&NullDateTime{Valid: true, DateTime: nullsTestDateTime}, `"2016-11-05 07:50:22.000001"`},

		{&NullInt64{}, `null`},
		{&NullFloat64{}, `null`},
		{&NullBool{}, `null`},
		{&NullString{}, `null`},
		{&NullGeography{}, `null`},
		{&NullTimestamp{}, `null`},
		{&NullDate{}, `null`},
		{&NullTime{}, `null`},
		{&NullDateTime{}, `null`},
	} {
		bytes, err := json.Marshal(test.in)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(bytes), test.want; got != want {
			t.Errorf("%#v: got %s, want %s", test.in, got, want)
		}

		typ := reflect.Indirect(reflect.ValueOf(test.in)).Type()
		value := reflect.New(typ).Interface()
		err = json.Unmarshal(bytes, value)
		if err != nil {
			t.Fatal(err)
		}

		if !testutil.Equal(value, test.in) {
			t.Errorf("%#v: got %#v, want %#v", test.in, value, test.in)
		}
	}
}
