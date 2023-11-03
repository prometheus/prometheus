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

package hetzner

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// SDMock is the interface for the Hetzner Cloud mock.
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

const hcloudTestToken = "LRK9DAWQ1ZAEFSrCNEEzLCUwhYX1U3g7wMg4dTlkkDC96fyDuyJ39nVbVjCKSDfj"

// HandleHcloudServers mocks the cloud servers list endpoint.
func (m *SDMock) HandleHcloudServers() {
	m.Mux.HandleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", hcloudTestToken) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Add("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, `
{
  "servers": [
    {
      "id": 42,
      "name": "my-server",
      "status": "running",
      "created": "2016-01-30T23:50:00+00:00",
      "public_net": {
        "ipv4": {
          "ip": "1.2.3.4",
          "blocked": false,
          "dns_ptr": "server01.example.com"
        },
        "ipv6": {
          "ip": "2001:db8::/64",
          "blocked": false,
          "dns_ptr": [
            {
              "ip": "2001:db8::1",
              "dns_ptr": "server.example.com"
            }
          ]
        },
        "floating_ips": [
          478
        ]
      },
      "private_net": [
        {
          "network": 4711,
          "ip": "10.0.0.2",
          "alias_ips": [],
          "mac_address": "86:00:ff:2a:7d:e1"
        }
      ],
      "server_type": {
        "id": 1,
        "name": "cx11",
        "description": "CX11",
        "cores": 1,
        "memory": 1,
        "disk": 25,
        "deprecated": false,
        "prices": [
          {
            "location": "fsn1",
            "price_hourly": {
              "net": "1.0000000000",
              "gross": "1.1900000000000000"
            },
            "price_monthly": {
              "net": "1.0000000000",
              "gross": "1.1900000000000000"
            }
          }
        ],
        "storage_type": "local",
        "cpu_type": "shared"
      },
      "datacenter": {
        "id": 1,
        "name": "fsn1-dc8",
        "description": "Falkenstein 1 DC 8",
        "location": {
          "id": 1,
          "name": "fsn1",
          "description": "Falkenstein DC Park 1",
          "country": "DE",
          "city": "Falkenstein",
          "latitude": 50.47612,
          "longitude": 12.370071,
          "network_zone": "eu-central"
        },
        "server_types": {
          "supported": [
            1,
            2,
            3
          ],
          "available": [
            1,
            2,
            3
          ],
          "available_for_migration": [
            1,
            2,
            3
          ]
        }
      },
      "image": {
        "id": 4711,
        "type": "system",
        "status": "available",
        "name": "ubuntu-20.04",
        "description": "Ubuntu 20.04 Standard 64 bit",
        "image_size": 2.3,
        "disk_size": 10,
        "created": "2016-01-30T23:50:00+00:00",
        "created_from": {
          "id": 1,
          "name": "Server"
        },
        "bound_to": null,
        "os_flavor": "ubuntu",
        "os_version": "20.04",
        "rapid_deploy": false,
        "protection": {
          "delete": false
        },
        "deprecated": "2018-02-28T00:00:00+00:00",
        "labels": {}
      },
      "iso": null,
      "rescue_enabled": false,
      "locked": false,
      "backup_window": "22-02",
      "outgoing_traffic": 123456,
      "ingoing_traffic": 123456,
      "included_traffic": 654321,
      "protection": {
        "delete": false,
        "rebuild": false
      },
      "labels": {
        "my-key": "my-value"
      },
      "volumes": [],
      "load_balancers": []
    },
    {
      "id": 44,
      "name": "another-server",
      "status": "stopped",
      "created": "2016-01-30T23:50:00+00:00",
      "public_net": {
        "ipv4": {
          "ip": "1.2.3.5",
          "blocked": false,
          "dns_ptr": "server01.example.org"
        },
        "ipv6": {
          "ip": "2001:db9::/64",
          "blocked": false,
          "dns_ptr": [
            {
              "ip": "2001:db9::1",
              "dns_ptr": "server01.example.org"
            }
          ]
        },
        "floating_ips": []
      },
      "private_net": [],
      "server_type": {
        "id": 2,
        "name": "cpx11",
        "description": "CPX11",
        "cores": 2,
        "memory": 1,
        "disk": 50,
        "deprecated": false,
        "prices": [
          {
            "location": "fsn1",
            "price_hourly": {
              "net": "1.0000000000",
              "gross": "1.1900000000000000"
            },
            "price_monthly": {
              "net": "1.0000000000",
              "gross": "1.1900000000000000"
            }
          }
        ],
        "storage_type": "local",
        "cpu_type": "shared"
      },
      "datacenter": {
        "id": 2,
        "name": "fsn1-dc14",
        "description": "Falkenstein 1 DC 14",
        "location": {
          "id": 1,
          "name": "fsn1",
          "description": "Falkenstein DC Park 1",
          "country": "DE",
          "city": "Falkenstein",
          "latitude": 50.47612,
          "longitude": 12.370071,
          "network_zone": "eu-central"
        },
        "server_types": {
          "supported": [
            1,
            2,
            3
          ],
          "available": [
            1,
            2,
            3
          ],
          "available_for_migration": [
            1,
            2,
            3
          ]
        }
      },
      "image": {
        "id": 4711,
        "type": "system",
        "status": "available",
        "name": "ubuntu-20.04",
        "description": "Ubuntu 20.04 Standard 64 bit",
        "image_size": 2.3,
        "disk_size": 10,
        "created": "2016-01-30T23:50:00+00:00",
        "created_from": {
          "id": 1,
          "name": "Server"
        },
        "bound_to": null,
        "os_flavor": "ubuntu",
        "os_version": "20.04",
        "rapid_deploy": false,
        "protection": {
          "delete": false
        },
        "deprecated": "2018-02-28T00:00:00+00:00",
        "labels": {}
      },
      "iso": null,
      "rescue_enabled": false,
      "locked": false,
      "backup_window": "22-02",
      "outgoing_traffic": 123456,
      "ingoing_traffic": 123456,
      "included_traffic": 654321,
      "protection": {
        "delete": false,
        "rebuild": false
      },
      "labels": {
        "key": "",
        "other-key": "value"
      },
      "volumes": [],
      "load_balancers": []
    },
    {
      "id": 36,
      "name": "deleted-image-server",
      "status": "stopped",
      "created": "2016-01-30T23:50:00+00:00",
      "public_net": {
        "ipv4": {
          "ip": "1.2.3.6",
          "blocked": false,
          "dns_ptr": "server01.example.org"
        },
        "ipv6": {
          "ip": "2001:db7::/64",
          "blocked": false,
          "dns_ptr": [
            {
              "ip": "2001:db7::1",
              "dns_ptr": "server01.example.org"
            }
          ]
        },
        "floating_ips": []
      },
      "private_net": [],
      "server_type": {
        "id": 2,
        "name": "cpx11",
        "description": "CPX11",
        "cores": 2,
        "memory": 1,
        "disk": 50,
        "deprecated": false,
        "prices": [
          {
            "location": "fsn1",
            "price_hourly": {
              "net": "1.0000000000",
              "gross": "1.1900000000000000"
            },
            "price_monthly": {
              "net": "1.0000000000",
              "gross": "1.1900000000000000"
            }
          }
        ],
        "storage_type": "local",
        "cpu_type": "shared"
      },
      "datacenter": {
        "id": 2,
        "name": "fsn1-dc14",
        "description": "Falkenstein 1 DC 14",
        "location": {
          "id": 1,
          "name": "fsn1",
          "description": "Falkenstein DC Park 1",
          "country": "DE",
          "city": "Falkenstein",
          "latitude": 50.47612,
          "longitude": 12.370071,
          "network_zone": "eu-central"
        },
        "server_types": {
          "supported": [
            1,
            2,
            3
          ],
          "available": [
            1,
            2,
            3
          ],
          "available_for_migration": [
            1,
            2,
            3
          ]
        }
      },
      "image": null,
      "iso": null,
      "rescue_enabled": false,
      "locked": false,
      "backup_window": "22-02",
      "outgoing_traffic": 123456,
      "ingoing_traffic": 123456,
      "included_traffic": 654321,
      "protection": {
        "delete": false,
        "rebuild": false
      },
      "labels": {},
      "volumes": [],
      "load_balancers": []
    }
  ],
  "meta": {
    "pagination": {
      "page": 1,
      "per_page": 25,
      "previous_page": null,
      "next_page": null,
      "last_page": 1,
      "total_entries": 2
    }
  }
}`,
		)
	})
}

// HandleHcloudNetworks mocks the cloud networks list endpoint.
func (m *SDMock) HandleHcloudNetworks() {
	m.Mux.HandleFunc("/networks", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", hcloudTestToken) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Add("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, `
{
  "networks": [
    {
      "id": 4711,
      "name": "mynet",
      "ip_range": "10.0.0.0/16",
      "subnets": [
        {
          "type": "cloud",
          "ip_range": "10.0.1.0/24",
          "network_zone": "eu-central",
          "gateway": "10.0.0.1"
        }
      ],
      "routes": [
        {
          "destination": "10.100.1.0/24",
          "gateway": "10.0.1.1"
        }
      ],
      "servers": [
        42
      ],
      "load_balancers": [
        42
      ],
      "protection": {
        "delete": false
      },
      "labels": {},
      "created": "2016-01-30T23:50:00+00:00"
    }
  ],
  "meta": {
    "pagination": {
      "page": 1,
      "per_page": 25,
      "previous_page": null,
      "next_page": null,
      "last_page": 1,
      "total_entries": 1
    }
  }
}`,
		)
	})
}

const (
	robotTestUsername = "my-hetzner"
	robotTestPassword = "my-password"
)

// HandleRobotServers mocks the robot servers list endpoint.
func (m *SDMock) HandleRobotServers() {
	m.Mux.HandleFunc("/server", func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if username != robotTestUsername && password != robotTestPassword && !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Add("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, `
[
  {
    "server":{
      "server_ip":"123.123.123.123",
      "server_number":321,
      "server_name":"server1",
      "product":"DS 3000",
      "dc":"NBG1-DC1",
      "traffic":"5 TB",
      "flatrate":true,
      "status":"ready",
      "throttled":true,
      "cancelled":false,
      "paid_until":"2010-09-02",
      "ip":[
        "123.123.123.123"
      ],
      "subnet":[
        {
          "ip":"2a01:4f8:111:4221::",
          "mask":"64"
        }
      ]
    }
  },
  {
    "server":{
      "server_ip":"123.123.123.124",
      "server_number":421,
      "server_name":"server2",
      "product":"X5",
      "dc":"FSN1-DC10",
      "traffic":"2 TB",
      "flatrate":true,
      "status":"in process",
      "throttled":false,
      "cancelled":true,
      "paid_until":"2010-06-11",
      "ip":[
        "123.123.123.124"
      ],
      "subnet":null
    }
  }
]`,
		)
	})
}
