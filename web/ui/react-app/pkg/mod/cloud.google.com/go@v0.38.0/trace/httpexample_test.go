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

package trace_test

import (
	"log"
	"net/http"

	"cloud.google.com/go/trace"
)

var traceClient *trace.Client

func ExampleHTTPClient_Do() {
	client := http.Client{
		Transport: &trace.Transport{},
	}
	span := traceClient.NewSpan("/foo") // traceClient is a *trace.Client

	req, _ := http.NewRequest("GET", "https://metadata/users", nil)
	req = req.WithContext(trace.NewContext(req.Context(), span))

	if _, err := client.Do(req); err != nil {
		log.Fatal(err)
	}
}

func ExampleClient_HTTPHandler() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := http.Client{
			Transport: &trace.Transport{},
		}

		req, _ := http.NewRequest("GET", "https://metadata/users", nil)
		req = req.WithContext(r.Context())

		// The outgoing request will be traced with r's trace ID.
		if _, err := client.Do(req); err != nil {
			log.Fatal(err)
		}
	})
	http.Handle("/foo", traceClient.HTTPHandler(handler)) // traceClient is a *trace.Client
}
