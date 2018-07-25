// Copyright 2018 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"net/http"
	"testing"
)

var authtoken = "an-auth-token"

var testcases = []struct {
	headers map[string]string
	ok      bool
}{
	{
		headers: map[string]string{
			"Authorization": "Bearer " + authtoken,
		},
		ok: true,
	},
	{
		headers: map[string]string{
			"Authorization": "Bearer" + authtoken,
		},
		ok: false,
	},
	{
		headers: map[string]string{
			"Authorization": authtoken,
		},
		ok: false,
	},
	{
		headers: map[string]string{
			"X-Other-header": "some-value",
		},
		ok: false,
	},
}

func TestBearerToken(t *testing.T) {

	for i, test := range testcases {
		r, _ := http.NewRequest("get", "http://someurl", nil)
		for k, v := range test.headers {
			r.Header.Set(k, v)
		}
		ctx := WithBearerToken(r.Context(), r)
		token, ok := GetBearerToken(ctx)
		if test.ok != ok {
			t.Fatalf("Test %d: GetBearerToken unexpectedly returned ok %t\n", i, ok)
		}
		if test.ok && token != authtoken {
			t.Fatalf("Test %d: GetBearerToken returned unexpected token %s\n", i, token)
		}

	}
}
