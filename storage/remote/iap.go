// Copyright 2021 The Prometheus Authors
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

package remote

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/prometheus/config"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

type iapRoundTripper struct {
	next http.RoundTripper
	ts   oauth2.TokenSource
}

// newIapRoundTripper returns a new http.RoundTripper that will sign requests
// using GCP's Identity Aware Proxy (IAP) authentication method.
// The request will be handed off to the next RoundTripper provided by next. If next is nil,
// http.DefaultTransport will be used.
//
// Credentials for IAP are found using the default GCP credential chain. If credentials
// cannot be found, en error will be returned.
func newIapRoundTripper(cfg *config.IAPConfig, next http.RoundTripper) (http.RoundTripper, error) {
	if next == nil {
		next = http.DefaultClient.Transport
	}

	ctx := context.Background()
	// TODO - add reading the service account from prometheus config
	// using  idtoken.WithCredentialsJSON() as an idtoken.ClientOption
	ts, err := idtoken.NewTokenSource(ctx, cfg.Audience)
	if err != nil {
		return nil, fmt.Errorf("could not create new IAP token source: %w", err)
	}

	rt := &iapRoundTripper{
		next: next,
		ts:   ts,
	}

	return rt, nil
}

func (rt *iapRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := rt.ts.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to create IAP token: %w", err)
	}
	token.SetAuthHeader(req)

	return rt.next.RoundTrip(req)
}
