// Copyright 2013 The Prometheus Authors
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

package httputil

import (
	"net/http"
	"regexp"
)

var corsHeaders = map[string]string{
	"Access-Control-Allow-Headers":  "Accept, Authorization, Content-Type, Origin",
	"Access-Control-Allow-Methods":  "GET, POST, OPTIONS",
	"Access-Control-Expose-Headers": "Date",
	"Vary":                          "Origin",
}

// SetCORS enables cross-site script calls.
func SetCORS(w http.ResponseWriter, o *regexp.Regexp, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}

	for k, v := range corsHeaders {
		w.Header().Set(k, v)
	}

	if o.String() == "^(?:.*)$" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		return
	}

	if o.MatchString(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
}
