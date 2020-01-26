// Copyright 2019 Google LLC
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

package transport_test

import (
	"context"
	"log"

	"google.golang.org/api/option"
	"google.golang.org/api/transport"
)

func Example_applicationDefaultCredentials() {
	ctx := context.Background()

	// Providing no auth option will cause NewClient to look for Application
	// Default Creds as specified at https://godoc.org/golang.org/x/oauth2/google#FindDefaultCredentials.
	//
	// Note: Given the same set of options, transport.NewHTTPClient and
	// transport.DialGRPC use the same credentials.
	c, _, err := transport.NewHTTPClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	_ = c // Use authenticated client.
}

func Example_withCredentialsFile() {
	ctx := context.Background()

	// Download service account creds per https://cloud.google.com/docs/authentication/production.
	//
	// Note: Given the same set of options, transport.NewHTTPClient and
	// transport.DialGRPC use the same credentials.
	c, _, err := transport.NewHTTPClient(ctx, option.WithCredentialsFile("/path/to/service-account-creds.json"))
	if err != nil {
		log.Fatal(err)
	}
	_ = c // Use authenticated client.
}
