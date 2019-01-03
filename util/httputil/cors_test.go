// Copyright 2016 The Prometheus Authors
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
	"testing"
)

func getCORSHandlerFunc() http.Handler {
	hf := func(w http.ResponseWriter, r *http.Request) {
		SetCORS(w, []string{"https://foo.com"}, r)
		w.WriteHeader(http.StatusOK)
	}
	return http.HandlerFunc(hf)
}

func TestCORSHandler(t *testing.T) {
	tearDown := setup()
	defer tearDown()
	client := &http.Client{}

	ch := getCORSHandlerFunc()
	mux.Handle("/any_path", ch)

	dummyOrigin := "https://foo.com"

	// OPTIONS WITH LEGIT ORIGIN
	req, err := http.NewRequest("OPTIONS", server.URL+"/any_path", nil)
	req.Header.Set("Origin", dummyOrigin)
	resp, err := client.Do(req)

	if err != nil {
		t.Error("client get failed with unexpected error")
	}

	AccessControlAllowOrigin := resp.Header.Get("Access-Control-Allow-Origin")

	if AccessControlAllowOrigin != dummyOrigin {
		t.Fatalf("%q does not match %q", dummyOrigin, AccessControlAllowOrigin)
	}

	// OPTIONS WITH BAD ORIGIN
	req, err = http.NewRequest("OPTIONS", server.URL+"/any_path", nil)
	req.Header.Set("Origin", "https://not-foo.com")
	resp, err = client.Do(req)

	if err != nil {
		t.Error("client get failed with unexpected error")
	}

	AccessControlAllowOrigin = resp.Header.Get("Access-Control-Allow-Origin")

	if AccessControlAllowOrigin != "" {
		t.Fatalf("Access-Control-Allow-Origin should not exist but it was set to: %q", AccessControlAllowOrigin)
	}
}
