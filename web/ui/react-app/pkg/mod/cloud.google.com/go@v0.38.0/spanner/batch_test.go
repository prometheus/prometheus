/*
Copyright 2018 Google LLC

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

	sppb "google.golang.org/genproto/googleapis/spanner/v1"
)

func TestPartitionRoundTrip(t *testing.T) {
	t.Parallel()
	for i, want := range []Partition{
		{rreq: &sppb.ReadRequest{Table: "t"}},
		{qreq: &sppb.ExecuteSqlRequest{Sql: "sql"}},
	} {
		got := serdesPartition(t, i, &want)
		if !testEqual(got, want) {
			t.Errorf("got: %#v\nwant:%#v", got, want)
		}
	}
}

func TestBROTIDRoundTrip(t *testing.T) {
	t.Parallel()
	tm := time.Now()
	want := BatchReadOnlyTransactionID{
		tid: []byte("tid"),
		sid: "sid",
		rts: tm,
	}
	data, err := want.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	var got BatchReadOnlyTransactionID
	if err := got.UnmarshalBinary(data); err != nil {
		t.Fatal(err)
	}
	if !testEqual(got, want) {
		t.Errorf("got: %#v\nwant:%#v", got, want)
	}
}

// serdesPartition is a helper that serialize a Partition then deserialize it
func serdesPartition(t *testing.T, i int, p1 *Partition) (p2 Partition) {
	var (
		data []byte
		err  error
	)
	if data, err = p1.MarshalBinary(); err != nil {
		t.Fatalf("#%d: encoding failed %v", i, err)
	}
	if err = p2.UnmarshalBinary(data); err != nil {
		t.Fatalf("#%d: decoding failed %v", i, err)
	}
	return p2
}
