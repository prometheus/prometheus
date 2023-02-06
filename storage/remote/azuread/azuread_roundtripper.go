// Copyright 2023 The Prometheus Authors
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

package azuread

import (
	"context"
	"net/http"
)

// Round tripper adding Azure AD authorization to calls
type azureAdRoundTripper struct {
	next          http.RoundTripper
	tokenProvider TokenProvider
}

// Creates round tripper adding Azure AD authorization to calls
func NewAzureAdRoundTripper(cfg *AzureAdConfig, next http.RoundTripper) (http.RoundTripper, error) {
	if next == nil {
		next = http.DefaultTransport
	}

	cred, err := NewTokenCredential(cfg)
	if err != nil {
		return nil, err
	}

	tokenProvider, err := NewTokenProvider(context.Background(), cfg, cred)
	if err != nil {
		return nil, err
	}

	rt := &azureAdRoundTripper{
		next:          next,
		tokenProvider: tokenProvider,
	}
	return rt, nil
}

// Sets Auhtorization header for requests
func (rt *azureAdRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	bearerAccessToken := "Bearer " + rt.tokenProvider.GetAccessToken()
	req.Header.Set("Authorization", bearerAccessToken)

	return rt.next.RoundTrip(req)
}
