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

package storage

import (
	"context"
	"net/http"
	"testing"

	"cloud.google.com/go/internal/testutil"
)

func TestSetACL(t *testing.T) {
	ctx := context.Background()
	mt := &mockTransport{}
	client := mockClient(t, mt)
	bh := &ACLHandle{c: client, bucket: "B"}
	oh := &ACLHandle{c: client, bucket: "B", object: "O"}
	for _, test := range []struct {
		desc string
		f    func() error
		want map[string]interface{}
	}{
		{
			desc: "bucket Set",
			f:    func() error { return bh.Set(ctx, AllUsers, RoleReader) },
			want: map[string]interface{}{
				"bucket": "B",
				"entity": "allUsers",
				"role":   "READER",
			},
		},

		{
			desc: "object Set",
			f:    func() error { return oh.Set(ctx, ACLEntity("e"), RoleWriter) },
			want: map[string]interface{}{
				"bucket": "B",
				"entity": "e",
				"role":   "WRITER",
			},
		}} {

		mt.addResult(&http.Response{StatusCode: 200, Body: bodyReader("{}")}, nil)
		if err := test.f(); err != nil {
			t.Fatal(err)
		}
		got := mt.gotJSONBody()
		if diff := testutil.Diff(got, test.want); diff != "" {
			t.Errorf("%s: %s", test.desc, diff)
		}
	}
}
