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

// Package internal supports the options and transport packages.
package internal

import (
	"net/http"
	"testing"

	"google.golang.org/grpc"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func TestSettingsValidate(t *testing.T) {
	// Valid.
	for _, ds := range []DialSettings{
		{},
		{APIKey: "x"},
		{Scopes: []string{"s"}},
		{CredentialsFile: "f"},
		{TokenSource: dummyTS{}},
		{CredentialsFile: "f", TokenSource: dummyTS{}}, // keep for backwards compatibility
		{CredentialsJSON: []byte("json")},
		{HTTPClient: &http.Client{}},
		{GRPCConn: &grpc.ClientConn{}},
		// Although NoAuth and Scopes are technically incompatible, too many
		// cloud clients add WithScopes to user-provided options to make
		// the check feasible.
		{NoAuth: true, Scopes: []string{"s"}},
	} {
		err := ds.Validate()
		if err != nil {
			t.Errorf("%+v: got %v, want nil", ds, err)
		}
	}

	// Invalid.
	for _, ds := range []DialSettings{
		{NoAuth: true, APIKey: "x"},
		{NoAuth: true, CredentialsFile: "f"},
		{NoAuth: true, TokenSource: dummyTS{}},
		{NoAuth: true, Credentials: &google.DefaultCredentials{}},
		{Credentials: &google.DefaultCredentials{}, CredentialsFile: "f"},
		{Credentials: &google.DefaultCredentials{}, TokenSource: dummyTS{}},
		{Credentials: &google.DefaultCredentials{}, CredentialsJSON: []byte("json")},
		{CredentialsFile: "f", CredentialsJSON: []byte("json")},
		{CredentialsJSON: []byte("json"), TokenSource: dummyTS{}},
		{HTTPClient: &http.Client{}, GRPCConn: &grpc.ClientConn{}},
		{HTTPClient: &http.Client{}, GRPCDialOpts: []grpc.DialOption{grpc.WithInsecure()}},
		{Audiences: []string{"foo"}, Scopes: []string{"foo"}},
		{HTTPClient: &http.Client{}, QuotaProject: "foo"},
		{HTTPClient: &http.Client{}, RequestReason: "foo"},
	} {
		err := ds.Validate()
		if err == nil {
			t.Errorf("%+v: got nil, want error", ds)
		}
	}

}

type dummyTS struct{}

func (dummyTS) Token() (*oauth2.Token, error) { return nil, nil }
