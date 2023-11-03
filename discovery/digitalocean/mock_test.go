// Copyright 2020 The Prometheus Authors
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

package digitalocean

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// SDMock is the interface for the DigitalOcean mock.
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
}

// ShutdownServer creates the mock server.
func (m *SDMock) ShutdownServer() {
	m.Server.Close()
}

const tokenID = "3c9a75a2-24fd-4508-b4f2-11f18aa97411"

// HandleDropletsList mocks droplet list.
func (m *SDMock) HandleDropletsList() {
	m.Mux.HandleFunc("/v2/droplets", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", tokenID) {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		w.Header().Add("content-type", "application/json; charset=utf-8")
		w.Header().Add("ratelimit-limit", "1200")
		w.Header().Add("ratelimit-remaining", "965")
		w.Header().Add("ratelimit-reset", "1415984218")
		w.WriteHeader(http.StatusAccepted)

		page := 1
		if pageQuery, ok := r.URL.Query()["page"]; ok {
			var err error
			page, err = strconv.Atoi(pageQuery[0])
			if err != nil {
				panic(err)
			}
		}
		fmt.Fprint(w, []string{
			`
{
  "droplets": [
    {
      "id": 3164444,
      "name": "example.com",
      "memory": 1024,
      "vcpus": 1,
      "disk": 25,
      "locked": false,
      "status": "active",
      "kernel": {
        "id": 2233,
        "name": "Ubuntu 14.04 x64 vmlinuz-3.13.0-37-generic",
        "version": "3.13.0-37-generic"
      },
      "created_at": "2014-11-14T16:29:21Z",
      "features": [
        "backups",
        "ipv6",
        "virtio"
      ],
      "backup_ids": [
        7938002
      ],
      "snapshot_ids": [

      ],
      "image": {
        "id": 6918990,
        "name": "14.04 x64",
        "distribution": "Ubuntu",
        "slug": "ubuntu-16-04-x64",
        "public": true,
        "regions": [
          "nyc1",
          "ams1",
          "sfo1",
          "nyc2",
          "ams2",
          "sgp1",
          "lon1",
          "nyc3",
          "ams3",
          "nyc3"
        ],
        "created_at": "2014-10-17T20:24:33Z",
        "type": "snapshot",
        "min_disk_size": 20,
        "size_gigabytes": 2.34
      },
      "volume_ids": [

      ],
      "size": {
      },
      "size_slug": "s-1vcpu-1gb",
      "networks": {
        "v4": [
          {
            "ip_address": "104.236.32.182",
            "netmask": "255.255.192.0",
            "gateway": "104.236.0.1",
            "type": "public"
          }
        ],
        "v6": [
          {
            "ip_address": "2604:A880:0800:0010:0000:0000:02DD:4001",
            "netmask": 64,
            "gateway": "2604:A880:0800:0010:0000:0000:0000:0001",
            "type": "public"
          }
        ]
      },
      "region": {
        "name": "New York 3",
        "slug": "nyc3",
        "sizes": [

        ],
        "features": [
          "virtio",
          "private_networking",
          "backups",
          "ipv6",
          "metadata"
        ],
        "available": null
      },
      "tags": [

      ],
      "vpc_uuid": "f9b0769c-e118-42fb-a0c4-fed15ef69662"
    },
    {
      "id": 3164494,
      "name": "prometheus",
      "memory": 1024,
      "vcpus": 1,
      "disk": 25,
      "locked": false,
      "status": "active",
      "kernel": {
        "id": 2233,
        "name": "Ubuntu 14.04 x64 vmlinuz-3.13.0-37-generic",
        "version": "3.13.0-37-generic"
      },
      "created_at": "2014-11-14T16:36:31Z",
      "features": [
        "virtio"
      ],
      "backup_ids": [

      ],
      "snapshot_ids": [
        7938206
      ],
      "image": {
        "id": 6918990,
        "name": "14.04 x64",
        "distribution": "Ubuntu",
        "slug": "ubuntu-16-04-x64",
        "public": true,
        "regions": [
          "nyc1",
          "ams1",
          "sfo1",
          "nyc2",
          "ams2",
          "sgp1",
          "lon1",
          "nyc3",
          "ams3",
          "nyc3"
        ],
        "created_at": "2014-10-17T20:24:33Z",
        "type": "snapshot",
        "min_disk_size": 20,
        "size_gigabytes": 2.34
      },
      "volume_ids": [

      ],
      "size": {
      },
      "size_slug": "s-1vcpu-1gb",
      "networks": {
        "v4": [
          {
            "ip_address": "104.131.186.241",
            "netmask": "255.255.240.0",
            "gateway": "104.131.176.1",
            "type": "public"
          }
        ]
      },
      "region": {
        "name": "New York 3",
        "slug": "nyc3",
        "sizes": [
          "s-1vcpu-1gb",
          "s-1vcpu-2gb",
          "s-1vcpu-3gb",
          "s-2vcpu-2gb",
          "s-3vcpu-1gb",
          "s-2vcpu-4gb",
          "s-4vcpu-8gb",
          "s-6vcpu-16gb",
          "s-8vcpu-32gb",
          "s-12vcpu-48gb",
          "s-16vcpu-64gb",
          "s-20vcpu-96gb",
          "s-24vcpu-128gb",
          "s-32vcpu-192gb"
        ],
        "features": [
          "virtio",
          "private_networking",
          "backups",
          "ipv6",
          "metadata"
        ],
        "available": true
      },
      "tags": [
        "monitor"
      ],
      "vpc_uuid": "f9b0769c-e118-42fb-a0c4-fed15ef69662"
    }
  ],
  "links": {
    "pages": {
      "next": "https://api.digitalocean.com/v2/droplets?page=2&per_page=2",
      "last": "https://api.digitalocean.com/v2/droplets?page=2&per_page=2"
    }
  },
  "meta": {
    "total": 4
  }
}
	`,
			`
{
  "droplets": [
    {
      "id": 175072239,
      "name": "prometheus-demo-old",
      "memory": 1024,
      "vcpus": 1,
      "disk": 25,
      "locked": false,
      "status": "off",
      "kernel": null,
      "created_at": "2020-01-10T16:47:39Z",
      "features": [
        "ipv6",
        "private_networking"
      ],
      "backup_ids": [],
      "next_backup_window": null,
      "snapshot_ids": [],
      "image": {
        "id": 53893572,
        "name": "18.04.3 (LTS) x64",
        "distribution": "Ubuntu",
        "slug": "ubuntu-18-04-x64",
        "public": true,
        "regions": [
          "nyc3",
          "nyc1",
          "sfo1",
          "nyc2",
          "ams2",
          "sgp1",
          "lon1",
          "nyc3",
          "ams3",
          "fra1",
          "tor1",
          "sfo2",
          "blr1",
          "sfo3"
        ],
        "created_at": "2019-10-22T01:38:19Z",
        "min_disk_size": 20,
        "type": "base",
        "size_gigabytes": 2.36,
        "description": "Ubuntu 18.04 x64 20191022",
        "tags": [],
        "status": "available"
      },
      "volume_ids": [],
      "size": {
        "slug": "s-1vcpu-1gb",
        "memory": 1024,
        "vcpus": 1,
        "disk": 25,
        "transfer": 1,
        "price_monthly": 5,
        "price_hourly": 0.00744,
        "regions": [
          "ams2",
          "ams3",
          "blr1",
          "fra1",
          "lon1",
          "nyc1",
          "nyc2",
          "nyc3",
          "sfo1",
          "sfo2",
          "sfo3",
          "sgp1",
          "tor1"
        ],
        "available": true
      },
      "size_slug": "s-1vcpu-1gb",
      "networks": {
        "v4": [
          {
            "ip_address": "167.172.111.118",
            "netmask": "255.255.240.0",
            "gateway": "167.172.176.1",
            "type": "public"
          },
          {
            "ip_address": "10.135.64.211",
            "netmask": "255.255.0.0",
            "gateway": "10.135.0.1",
            "type": "private"
          }
        ],
        "v6": [
        ]
      },
      "region": {
        "name": "Frankfurt 1",
        "slug": "fra1",
        "features": [
          "private_networking",
          "backups",
          "ipv6",
          "metadata",
          "install_agent",
          "storage",
          "image_transfer"
        ],
        "available": true,
        "sizes": [
          "s-1vcpu-1gb",
          "512mb",
          "s-1vcpu-2gb",
          "1gb",
          "s-3vcpu-1gb",
          "s-2vcpu-2gb",
          "s-1vcpu-3gb",
          "s-2vcpu-4gb",
          "2gb",
          "s-4vcpu-8gb",
          "m-1vcpu-8gb",
          "c-2",
          "4gb",
          "g-2vcpu-8gb",
          "gd-2vcpu-8gb",
          "m-16gb",
          "s-6vcpu-16gb",
          "c-4",
          "8gb",
          "m-2vcpu-16gb",
          "m3-2vcpu-16gb",
          "g-4vcpu-16gb",
          "gd-4vcpu-16gb",
          "m6-2vcpu-16gb",
          "m-32gb",
          "s-8vcpu-32gb",
          "c-8",
          "16gb",
          "m-4vcpu-32gb",
          "m3-4vcpu-32gb",
          "g-8vcpu-32gb",
          "s-12vcpu-48gb",
          "gd-8vcpu-32gb",
          "m6-4vcpu-32gb",
          "m-64gb",
          "s-16vcpu-64gb",
          "c-16",
          "32gb",
          "m-8vcpu-64gb",
          "m3-8vcpu-64gb",
          "g-16vcpu-64gb",
          "s-20vcpu-96gb",
          "48gb",
          "gd-16vcpu-64gb",
          "m6-8vcpu-64gb",
          "m-128gb",
          "s-24vcpu-128gb",
          "c-32",
          "64gb",
          "m-16vcpu-128gb",
          "m3-16vcpu-128gb",
          "s-32vcpu-192gb",
          "m-24vcpu-192gb",
          "m-224gb",
          "m6-16vcpu-128gb",
          "m3-24vcpu-192gb",
          "m6-24vcpu-192gb"
        ]
      },
      "tags": [],
      "vpc_uuid": "953d698c-dc84-11e8-80bc-3cfdfea9fba1"
    },
    {
      "id": 176011507,
      "name": "prometheus-demo",
      "memory": 1024,
      "vcpus": 1,
      "disk": 25,
      "locked": false,
      "status": "active",
      "kernel": null,
      "created_at": "2020-01-17T12:06:26Z",
      "features": [
        "ipv6",
        "private_networking"
      ],
      "backup_ids": [],
      "next_backup_window": null,
      "snapshot_ids": [],
      "image": {
        "id": 53893572,
        "name": "18.04.3 (LTS) x64",
        "distribution": "Ubuntu",
        "slug": "ubuntu-18-04-x64",
        "public": true,
        "regions": [
          "nyc3",
          "nyc1",
          "sfo1",
          "nyc2",
          "ams2",
          "sgp1",
          "lon1",
          "nyc3",
          "ams3",
          "fra1",
          "tor1",
          "sfo2",
          "blr1",
          "sfo3"
        ],
        "created_at": "2019-10-22T01:38:19Z",
        "min_disk_size": 20,
        "type": "base",
        "size_gigabytes": 2.36,
        "description": "Ubuntu 18.04 x64 20191022",
        "tags": [],
        "status": "available"
      },
      "volume_ids": [],
      "size": {
        "slug": "s-1vcpu-1gb",
        "memory": 1024,
        "vcpus": 1,
        "disk": 25,
        "transfer": 1,
        "price_monthly": 5,
        "price_hourly": 0.00744,
        "regions": [
          "ams2",
          "ams3",
          "blr1",
          "fra1",
          "lon1",
          "nyc1",
          "nyc2",
          "nyc3",
          "sfo1",
          "sfo2",
          "sfo3",
          "sgp1",
          "tor1"
        ],
        "available": true
      },
      "size_slug": "s-1vcpu-1gb",
      "networks": {
        "v4": [
          {
            "ip_address": "138.65.56.69",
            "netmask": "255.255.240.0",
            "gateway": "138.65.64.1",
            "type": "public"
          },
          {
            "ip_address": "154.245.26.111",
            "netmask": "255.255.252.0",
            "gateway": "154.245.24.1",
            "type": "public"
          },
          {
            "ip_address": "10.135.64.212",
            "netmask": "255.255.0.0",
            "gateway": "10.135.0.1",
            "type": "private"
          }
        ],
        "v6": [
          {
            "ip_address": "2a03:b0c0:3:f0::cf2:4",
            "netmask": 64,
            "gateway": "2a03:b0c0:3:f0::1",
            "type": "public"
          }
        ]
      },
      "region": {
        "name": "Frankfurt 1",
        "slug": "fra1",
        "features": [
          "private_networking",
          "backups",
          "ipv6",
          "metadata",
          "install_agent",
          "storage",
          "image_transfer"
        ],
        "available": true,
        "sizes": [
          "s-1vcpu-1gb",
          "512mb",
          "s-1vcpu-2gb",
          "1gb",
          "s-3vcpu-1gb",
          "s-2vcpu-2gb",
          "s-1vcpu-3gb",
          "s-2vcpu-4gb",
          "2gb",
          "s-4vcpu-8gb",
          "m-1vcpu-8gb",
          "c-2",
          "4gb",
          "g-2vcpu-8gb",
          "gd-2vcpu-8gb",
          "m-16gb",
          "s-6vcpu-16gb",
          "c-4",
          "8gb",
          "m-2vcpu-16gb",
          "m3-2vcpu-16gb",
          "g-4vcpu-16gb",
          "gd-4vcpu-16gb",
          "m6-2vcpu-16gb",
          "m-32gb",
          "s-8vcpu-32gb",
          "c-8",
          "16gb",
          "m-4vcpu-32gb",
          "m3-4vcpu-32gb",
          "g-8vcpu-32gb",
          "s-12vcpu-48gb",
          "gd-8vcpu-32gb",
          "m6-4vcpu-32gb",
          "m-64gb",
          "s-16vcpu-64gb",
          "c-16",
          "32gb",
          "m-8vcpu-64gb",
          "m3-8vcpu-64gb",
          "g-16vcpu-64gb",
          "s-20vcpu-96gb",
          "48gb",
          "gd-16vcpu-64gb",
          "m6-8vcpu-64gb",
          "m-128gb",
          "s-24vcpu-128gb",
          "c-32",
          "64gb",
          "m-16vcpu-128gb",
          "m3-16vcpu-128gb",
          "s-32vcpu-192gb",
          "m-24vcpu-192gb",
          "m-224gb",
          "m6-16vcpu-128gb",
          "m3-24vcpu-192gb",
          "m6-24vcpu-192gb"
        ]
      },
      "tags": [],
      "vpc_uuid": "953d698c-dc84-11e8-80bc-3cfdfea9fba1"
    }
  ],
  "links": {
    "pages": {
      "first": "https://api.digitalocean.com/v2/droplets?page=1&per_page=2",
      "prev": "https://api.digitalocean.com/v2/droplets?page=1&per_page=2"
    }
  },
  "meta": {
    "total": 4
  }
}
`,
		}[page-1],
		)
	})
}
