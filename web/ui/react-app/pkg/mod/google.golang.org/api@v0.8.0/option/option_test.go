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

package option

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/internal"
	"google.golang.org/grpc"
)

// Check that the slice passed into WithScopes is copied.
func TestCopyScopes(t *testing.T) {
	o := &internal.DialSettings{}

	scopes := []string{"a", "b"}
	WithScopes(scopes...).Apply(o)

	// Modify after using.
	scopes[1] = "c"

	if o.Scopes[0] != "a" || o.Scopes[1] != "b" {
		t.Errorf("want ['a', 'b'], got %+v", o.Scopes)
	}
}

func TestApply(t *testing.T) {
	conn := &grpc.ClientConn{}
	opts := []ClientOption{
		WithEndpoint("https://example.com:443"),
		WithScopes("a"), // the next WithScopes should overwrite this one
		WithScopes("https://example.com/auth/helloworld", "https://example.com/auth/otherthing"),
		WithGRPCConn(conn),
		WithUserAgent("ua"),
		WithCredentialsFile("service-account.json"),
		WithCredentialsJSON([]byte(`{some: "json"}`)),
		WithCredentials(&google.DefaultCredentials{ProjectID: "p"}),
		WithAPIKey("api-key"),
		WithAudiences("https://example.com/"),
		WithQuotaProject("user-project"),
		WithRequestReason("Request Reason"),
	}
	var got internal.DialSettings
	for _, opt := range opts {
		opt.Apply(&got)
	}
	want := internal.DialSettings{
		Scopes:          []string{"https://example.com/auth/helloworld", "https://example.com/auth/otherthing"},
		UserAgent:       "ua",
		Endpoint:        "https://example.com:443",
		GRPCConn:        conn,
		Credentials:     &google.DefaultCredentials{ProjectID: "p"},
		CredentialsFile: "service-account.json",
		CredentialsJSON: []byte(`{some: "json"}`),
		APIKey:          "api-key",
		Audiences:       []string{"https://example.com/"},
		QuotaProject:    "user-project",
		RequestReason:   "Request Reason",
	}
	if !cmp.Equal(got, want, cmpopts.IgnoreUnexported(grpc.ClientConn{})) {
		t.Errorf("\ngot  %#v\nwant %#v", got, want)
	}
}
