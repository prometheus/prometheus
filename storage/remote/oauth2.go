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
	"net/http"

	"github.com/prometheus/prometheus/config"
)

type oauth2RoundTripper struct {
	token string
	next  http.RoundTripper
}

// newOAuth2RoundTripper returns a new http.RoundTripper that will
// authorize requests with a provided OAuth 2.0 access token.
func newOAuth2RoundTripper(cfg *config.OAuth2Config, next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}

	return &oauth2RoundTripper{
		token: cfg.AccessToken,
		next:  next,
	}
}

func (rt *oauth2RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+rt.token)

	return rt.next.RoundTrip(req)
}
