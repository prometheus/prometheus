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
	"testing"
	"time"

	pbd "github.com/golang/protobuf/ptypes/duration"
	pbt "github.com/golang/protobuf/ptypes/timestamp"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
)

// Test generating TimestampBound for strong reads.
func TestStrong(t *testing.T) {
	got := StrongRead()
	want := TimestampBound{mode: strong}
	if !testEqual(got, want) {
		t.Errorf("Strong() = %v; want %v", got, want)
	}
}

// Test generating TimestampBound for reads with exact staleness.
func TestExactStaleness(t *testing.T) {
	got := ExactStaleness(10 * time.Second)
	want := TimestampBound{mode: exactStaleness, d: 10 * time.Second}
	if !testEqual(got, want) {
		t.Errorf("ExactStaleness(10*time.Second) = %v; want %v", got, want)
	}
}

// Test generating TimestampBound for reads with max staleness.
func TestMaxStaleness(t *testing.T) {
	got := MaxStaleness(10 * time.Second)
	want := TimestampBound{mode: maxStaleness, d: 10 * time.Second}
	if !testEqual(got, want) {
		t.Errorf("MaxStaleness(10*time.Second) = %v; want %v", got, want)
	}
}

// Test generating TimestampBound for reads with minimum freshness requirement.
func TestMinReadTimestamp(t *testing.T) {
	ts := time.Now()
	got := MinReadTimestamp(ts)
	want := TimestampBound{mode: minReadTimestamp, t: ts}
	if !testEqual(got, want) {
		t.Errorf("MinReadTimestamp(%v) = %v; want %v", ts, got, want)
	}
}

// Test generating TimestampBound for reads requesting data at a exact timestamp.
func TestReadTimestamp(t *testing.T) {
	ts := time.Now()
	got := ReadTimestamp(ts)
	want := TimestampBound{mode: readTimestamp, t: ts}
	if !testEqual(got, want) {
		t.Errorf("ReadTimestamp(%v) = %v; want %v", ts, got, want)
	}
}

// Test TimestampBound.String.
func TestTimestampBoundString(t *testing.T) {
	ts := time.Unix(1136239445, 0).UTC()
	var tests = []struct {
		tb   TimestampBound
		want string
	}{
		{
			tb:   TimestampBound{mode: strong},
			want: "(strong)",
		},
		{
			tb:   TimestampBound{mode: exactStaleness, d: 10 * time.Second},
			want: "(exactStaleness: 10s)",
		},
		{
			tb:   TimestampBound{mode: maxStaleness, d: 10 * time.Second},
			want: "(maxStaleness: 10s)",
		},
		{
			tb:   TimestampBound{mode: minReadTimestamp, t: ts},
			want: "(minReadTimestamp: 2006-01-02 22:04:05 +0000 UTC)",
		},
		{
			tb:   TimestampBound{mode: readTimestamp, t: ts},
			want: "(readTimestamp: 2006-01-02 22:04:05 +0000 UTC)",
		},
	}
	for _, test := range tests {
		got := test.tb.String()
		if got != test.want {
			t.Errorf("%#v.String():\ngot  %q\nwant %q", test.tb, got, test.want)
		}
	}
}

// Test time.Duration to pdb.Duration conversion.
func TestDurationProto(t *testing.T) {
	var tests = []struct {
		d    time.Duration
		want pbd.Duration
	}{
		{time.Duration(0), pbd.Duration{Seconds: 0, Nanos: 0}},
		{time.Second, pbd.Duration{Seconds: 1, Nanos: 0}},
		{time.Millisecond, pbd.Duration{Seconds: 0, Nanos: 1e6}},
		{15 * time.Nanosecond, pbd.Duration{Seconds: 0, Nanos: 15}},
		{42 * time.Hour, pbd.Duration{Seconds: 151200}},
		{-(1*time.Hour + 4*time.Millisecond), pbd.Duration{Seconds: -3600, Nanos: -4e6}},
	}
	for _, test := range tests {
		got := durationProto(test.d)
		if !testEqual(got, &test.want) {
			t.Errorf("durationProto(%v) = %v; want %v", test.d, got, test.want)
		}
	}
}

// Test time.Time to pbt.Timestamp conversion.
func TestTimeProto(t *testing.T) {
	var tests = []struct {
		t    time.Time
		want pbt.Timestamp
	}{
		{time.Unix(0, 0), pbt.Timestamp{}},
		{time.Unix(1136239445, 12345), pbt.Timestamp{Seconds: 1136239445, Nanos: 12345}},
		{time.Unix(-1000, 12345), pbt.Timestamp{Seconds: -1000, Nanos: 12345}},
	}
	for _, test := range tests {
		got := timestampProto(test.t)
		if !testEqual(got, &test.want) {
			t.Errorf("timestampProto(%v) = %v; want %v", test.t, got, test.want)
		}
	}
}

// Test readonly transaction option builder.
func TestBuildTransactionOptionsReadOnly(t *testing.T) {
	ts := time.Unix(1136239445, 12345)
	var tests = []struct {
		tb   TimestampBound
		ts   bool
		want sppb.TransactionOptions_ReadOnly
	}{
		{
			StrongRead(), false,
			sppb.TransactionOptions_ReadOnly{
				TimestampBound: &sppb.TransactionOptions_ReadOnly_Strong{
					Strong: true},
				ReturnReadTimestamp: false,
			},
		},
		{
			ExactStaleness(10 * time.Second), true,
			sppb.TransactionOptions_ReadOnly{
				TimestampBound: &sppb.TransactionOptions_ReadOnly_ExactStaleness{
					ExactStaleness: &pbd.Duration{Seconds: 10}},
				ReturnReadTimestamp: true,
			},
		},
		{
			MaxStaleness(10 * time.Second), true,
			sppb.TransactionOptions_ReadOnly{
				TimestampBound: &sppb.TransactionOptions_ReadOnly_MaxStaleness{
					MaxStaleness: &pbd.Duration{Seconds: 10}},
				ReturnReadTimestamp: true,
			},
		},

		{
			MinReadTimestamp(ts), true,
			sppb.TransactionOptions_ReadOnly{
				TimestampBound: &sppb.TransactionOptions_ReadOnly_MinReadTimestamp{
					MinReadTimestamp: &pbt.Timestamp{Seconds: 1136239445, Nanos: 12345}},
				ReturnReadTimestamp: true,
			},
		},
		{
			ReadTimestamp(ts), true,
			sppb.TransactionOptions_ReadOnly{
				TimestampBound: &sppb.TransactionOptions_ReadOnly_ReadTimestamp{
					ReadTimestamp: &pbt.Timestamp{Seconds: 1136239445, Nanos: 12345}},
				ReturnReadTimestamp: true,
			},
		},
	}
	for _, test := range tests {
		got := buildTransactionOptionsReadOnly(test.tb, test.ts)
		if !testEqual(got, &test.want) {
			t.Errorf("buildTransactionOptionsReadOnly(%v,%v) = %v; want %v", test.tb, test.ts, got, test.want)
		}
	}
}
