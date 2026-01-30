// Copyright The Prometheus Authors
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

package stackit

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// SDMock is the interface for the STACKIT IAAS API mock.
type SDMock struct {
	t      *testing.T
	Server *httptest.Server
	Mux    *http.ServeMux
}

// NewSDMock returns a new SDMock.
func NewSDMock(t *testing.T) *SDMock {
	return &SDMock{
		t: t,
	}
}

// Endpoint returns the URI to the mock server.
func (m *SDMock) Endpoint() string {
	return m.Server.URL + "/"
}

// Setup creates the mock server.
func (m *SDMock) Setup() {
	m.Mux = http.NewServeMux()
	m.Server = httptest.NewServer(m.Mux)
	m.t.Cleanup(m.Server.Close)
}

// ShutdownServer creates the mock server.
func (m *SDMock) ShutdownServer() {
	m.Server.Close()
}

const (
	testToken     = "LRK9DAWQ1ZAEFSrCNEEzLCUwhYX1U3g7wMg4dTlkkDC96fyDuyJ39nVbVjCKSDfj"
	testProjectID = "00000000-0000-0000-0000-000000000000"
)

// HandleServers mocks the STACKIT IAAS API.
func (m *SDMock) HandleServers() {
	// /token endpoint mocks the token endpoint for service account authentication.
	// It checks if the request body starts with "assertion=ey" to simulate a valid assertion
	// as defined in RFC 7523.
	m.Mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		reqBody, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, err)
			return
		}

		// Expecting HTTP form encoded body with the field assertion.
		// JWT always start with "ey" (base64url encoded).
		if !bytes.HasPrefix(reqBody, []byte("assertion=ey")) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Add("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		_, _ = fmt.Fprintf(w, `{"access_token": "%s"}`, testToken)
	})

	m.Mux.HandleFunc(fmt.Sprintf("/v1/projects/%s/servers", testProjectID), func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", testToken) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Add("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		_, _ = fmt.Fprint(w, `
{
  "items": [
    {
      "availabilityZone": "eu01-3",
      "bootVolume": {
        "deleteOnTermination": false,
        "id": "1c15e4cc-8474-46be-b875-b473ea9fe80c"
      },
      "createdAt": "2025-03-12T14:48:17Z",
      "id": "b4176700-596a-4f80-9fc8-5f9c58a606e1",
      "labels": {
        "provisionSTACKITServerAgent": "true",
        "stackit_project_id": "00000000-0000-0000-0000-000000000000"
      },
      "launchedAt": "2025-03-12T14:48:52Z",
      "machineType": "g1.1",
      "name": "runcommandtest",
      "nics": [
        {
          "ipv4": "10.0.0.153",
          "mac": "fa:16:4f:42:1c:d3",
          "networkId": "3173494f-2f6c-490d-8c12-4b3c86b4338b",
          "networkName": "test",
          "publicIp": "192.0.2.1",
          "nicId": "b36097c5-e1c5-4e12-ae97-c03e144db127",
          "nicSecurity": true,
          "securityGroups": [
            "6e60809f-bed3-46c6-a39c-adddd6455674"
          ]
        }
      ],
      "powerStatus": "STOPPED",
      "serviceAccountMails": [],
      "status": "INACTIVE",
      "updatedAt": "2025-03-13T07:08:29Z",
      "userData": null,
      "volumes": [
        "1c15e4cc-8474-46be-b875-b473ea9fe80c"
      ]
    },
	{
	  "availabilityZone": "eu01-m",
	  "bootVolume": {
	    "deleteOnTermination": false,
	    "id": "1e3ffe2b-878f-46e5-b39e-372e13a09551"
	  },
	  "createdAt": "2025-04-10T16:45:25Z",
	  "id": "ee337436-1f15-4647-a03e-154009966179",
	  "labels": {},
	  "launchedAt": "2025-04-10T16:46:00Z",
	  "machineType": "t1.1",
	  "name": "server1",
	  "nics": [],
	  "powerStatus": "RUNNING",
	  "serviceAccountMails": [],
	  "status": "ACTIVE",
	  "updatedAt": "2025-04-10T16:46:00Z",
	  "volumes": [
	    "1e3ffe2b-878f-46e5-b39e-372e13a09551"
	  ]
  	}
  ]
}`,
		)
	})
}
