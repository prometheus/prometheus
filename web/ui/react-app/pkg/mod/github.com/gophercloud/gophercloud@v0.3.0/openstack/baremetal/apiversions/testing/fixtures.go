package testing

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetal/apiversions"
	th "github.com/gophercloud/gophercloud/testhelper"
	"github.com/gophercloud/gophercloud/testhelper/client"
)

const IronicAPIAllVersionResponse = `
{
  "default_version": {
    "status": "CURRENT",
    "min_version": "1.1",
    "version": "1.56",
    "id": "v1",
    "links": [
      {
        "href": "http://localhost:6385/v1/",
        "rel": "self"
      }
    ]
  },
  "versions": [
    {
      "status": "CURRENT",
      "min_version": "1.1",
      "version": "1.56",
      "id": "v1",
      "links": [
        {
          "href": "http://localhost:6385/v1/",
          "rel": "self"
        }
      ]
    }
  ],
  "name": "OpenStack Ironic API",
  "description": "Ironic is an OpenStack project which aims to provision baremetal machines."
}
`

const IronicAPIVersionResponse = `
{
  "media_types": [
    {
      "base": "application/json",
      "type": "application/vnd.openstack.ironic.v1+json"
    }
  ],
  "version": {
    "status": "CURRENT",
    "min_version": "1.1",
    "version": "1.56",
    "id": "v1",
    "links": [
      {
        "href": "http://localhost:6385/v1/",
        "rel": "self"
      }
    ]
  },
  "id": "v1"
}
`

var IronicAPIVersion1Result = apiversions.APIVersion{
	ID:         "v1",
	Status:     "CURRENT",
	MinVersion: "1.1",
	Version:    "1.56",
}

var IronicAllAPIVersionResults = apiversions.APIVersions{
	DefaultVersion: IronicAPIVersion1Result,
	Versions: []apiversions.APIVersion{
		IronicAPIVersion1Result,
	},
}

func MockListResponse(t *testing.T) {
	th.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", client.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, IronicAPIAllVersionResponse)
	})
}

func MockGetResponse(t *testing.T) {
	th.Mux.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", client.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, IronicAPIVersionResponse)
	})
}
