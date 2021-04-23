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

package huawei

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/huaweicloud/golangsdk/openstack/identity/v3/tokens"
)

type SDMock struct {
	t      *testing.T
	Server *httptest.Server
	Mux    *http.ServeMux
}

func NewSDMock(t *testing.T) *SDMock {
	return &SDMock{
		t: t,
	}
}

// Endpoint returns the URI to the mock server
func (m *SDMock) Endpoint() string {
	return m.Server.URL + "/v3"
}

func (m *SDMock) Setup() {
	m.Mux = http.NewServeMux()
	m.Server = httptest.NewServer(m.Mux)
}

func (m *SDMock) ShutDownServer() {
	m.Server.Close()
}

type TokenResult struct {
	Token *Token `json:"token"`
}

type Token struct {
	Entries []tokens.CatalogEntry `json:"catalog"`
	Project *tokens.Project
}

func (m *SDMock) HandleGetToken() {
	m.Mux.HandleFunc("/v3/auth/tokens", func(w http.ResponseWriter, request *http.Request) {
		w.WriteHeader(http.StatusCreated)
		tokenResult := &TokenResult{
			Token: &Token{
				Entries: []tokens.CatalogEntry{
					{
						ID:   "ee4e76bc704d447fa188397bd851d891",
						Name: "ecs",
						Type: "ecs",
						Endpoints: []tokens.Endpoint{
							{
								ID:        "edee92705bb64a8fb57e3ccf400985b1",
								Region:    "ap-southeast-2",
								Interface: "public",
								URL:       m.Server.URL + "/v1/0bdfeaac5280f5eb2ff3c00f6512b576",
							},
						},
					},
				},
				Project: &tokens.Project{
					ID:   "0bdfeaac5280f5eb2ff3c00f6512b576",
					Name: "ap-southeast-2",
					Domain: tokens.Domain{
						ID:   "0bde1aa04b800fc00f69c00f1b059d80",
						Name: "xxxx",
					},
				},
			},
		}
		jsonBytes, err := json.Marshal(tokenResult)
		if err != nil {
			fmt.Fprint(w, err)
		}
		fmt.Fprint(w, string(jsonBytes))
	})
}

func (m *SDMock) HandleListProjects() {
	m.Mux.HandleFunc("/v3/projects", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, `
{
"projects":[{
  "is_domain": false,
  "description": "",
  "domain_id": "0bde1aa04b800fc00f69c00f1b059d80",
  "enabled": true,
  "id": "0bdfeaac5280f5eb2ff3c00f6512b576",
  "name": "ap-southeast-2",
  "parent_id": "0bde1aa04b800fc00f69c00f1b059d80"
}]
}`,
		)
	})
}

func (m *SDMock) HandleListCatalog() {
	m.Mux.HandleFunc("/v3/auth/catalog", func(w http.ResponseWriter, request *http.Request) {
		w.Header().Add("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		serviceCatalog := &tokens.ServiceCatalog{
			Entries: []tokens.CatalogEntry{
				{
					ID:   "ee4e76bc704d447fa188397bd851d891",
					Name: "ecs",
					Type: "ecs",
					Endpoints: []tokens.Endpoint{
						{
							ID:        "edee92705bb64a8fb57e3ccf400985b1",
							Region:    "ap-southeast-2",
							Interface: "public",
							URL:       m.Server.URL + "/v1/0bdfeaac5280f5eb2ff3c00f6512b576",
						},
					},
				},
			},
		}
		jsonBytes, err := json.Marshal(serviceCatalog)
		if err != nil {
			fmt.Fprint(w, err)
		}

		fmt.Fprint(w, string(jsonBytes))
	})
}

func (m *SDMock) HandleListServer() {
	m.Mux.HandleFunc("/v1/0bdfeaac5280f5eb2ff3c00f6512b576/cloudservers/detail", func(w http.ResponseWriter,
		request *http.Request) {
		w.Header().Add("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, `
{
"count": 2,
"servers":[{
  "status": "BUILD",
  "updated": "2021-04-22T01:59:13Z",
  "hostId": "3f88ca6718fc7484d835a346c310aaaaaasssdsd",
  "addresses": {
    "bc8d0989-8b85-4ca0-a0ac-9f6dsb615208": [
      {
        "version": "4",
        "addr": "10.1.1.224",
        "OS-EXT-IPS-MAC:mac_addr": "fa:12:3e:d9:20:28",
        "OS-EXT-IPS:port_id": "1518ff58-767a-4sc8-ab1b-811ddd1de9f6",
        "OS-EXT-IPS:type": "fixed"
      }
    ]
  },
  "id": "ccbe39f1-26df-4443-b28a-002b547542ce",
  "name": "test4FVIK",
  "accessIPv4": "",
  "accessIPv6": "",
  "created": "2021-04-22T01:59:07Z",
  "tags": [
    
  ],
  "description": "",
  "locked": false,
  "config_drive": "",
  "tenant_id": "0bdfeaac2310f5eb2ff3c00f6512b576",
  "user_id": "0bea661fd400asd61fc1c00faa56b700",
  "host_status": "UP",
  "enterprise_project_id": "0",
  "sys_tags": [
    {
      "key": "_sys_enterprise_project_id",
      "value": "0"
    }
  ],
  "flavor": {
    "disk": "0",
    "vcpus": "8",
    "ram": "16384",
    "id": "c3.2xlarge.2",
    "name": "c3.2xlarge.2"
  },
  "metadata": {
    "charging_mode": "0",
    "metering.order_id": "",
    "metering.product_id": "",
    "vpc_id": "bc8d0sd9-8b85-4ca0-a0ac-9f680b615208",
    "EcmResStatus": "",
    "metering.image_id": "6b33sd3d-c3ef-4d00-bf9d-9535cb9dd42f",
    "metering.imagetype": "gold",
    "metering.resourcespeccode": "c3.2xlarge.2.linux",
    "image_name": "CentOS 7.6 64bit",
    "os_bit": "64",
    "lock_check_endpoint": "",
    "lock_source": "",
    "lock_source_id": "",
    "lock_scene": "",
    "virtual_env_type": "",
    "agency_name": ""
  },
  "security_groups": [
    {
      "name": "test4FVIK"
    }
  ],
  "key_name": "testkey",
  "image": {
    "id": "6bsa903d-c3ef-4d00-bf9d-9535cb9dd42f"
  },
  "progress": 0,
  "OS-EXT-STS:power_state": 0,
  "OS-EXT-STS:vm_state": "building",
  "OS-EXT-STS:task_state": "spawning",
  "OS-DCF:diskConfig": "MANUAL",
  "OS-EXT-AZ:availability_zone": "ap-southeast-2a",
  "OS-SRV-USG:launched_at": "",
  "OS-SRV-USG:terminated_at": "",
  "OS-EXT-SRV-ATTR:root_device_name": "/dev/vda",
  "OS-EXT-SRV-ATTR:ramdisk_id": "",
  "OS-EXT-SRV-ATTR:kernel_id": "",
  "OS-EXT-SRV-ATTR:launch_index": 0,
  "OS-EXT-SRV-ATTR:reservation_id": "r-o5a8u6u0",
  "OS-EXT-SRV-ATTR:hostname": "test4fvik",
  "OS-EXT-SRV-ATTR:user_data": "IyEvdXNyL2Jpbi9lbnYgYmFzaAplY2hvICJ0ZXN0Ig==",
  "OS-EXT-SRV-ATTR:host": "3f88ca6718fcasdd835a346c3108c1cbfec8864baece143d43e4a63",
  "OS-EXT-SRV-ATTR:instance_name": "instance-001ebf09",
  "OS-EXT-SRV-ATTR:hypervisor_hostname": "e9c2eqwac40f0526adf2f2c57ad8e2218ab305fb0f146fec0bfc4ad",
  "os-extended-volumes:volumes_attached": [
    {
      "id": "8a927dfa-93dc-4c86-a1a3-057as984699f",
      "delete_on_termination": "true",
      "bootIndex": "0",
      "device": "/dev/vda"
    }
  ],
  "os:scheduler_hints": {
    "group": null
  }
},
{
  "status": "ACTIVE",
  "updated": "2021-04-22T01:47:55Z",
  "hostId": "3f88ca6718fc7484d835a346cqwe8c1cbfec8864baece143d43e4a63",
  "addresses": {
    "bc8d0989-8b85-4ca0-a0ac-9f612b615208": [
      {
        "version": "4",
        "addr": "10.1.1.3",
        "OS-EXT-IPS-MAC:mac_addr": "fa:16:3e:65:71:0a",
        "OS-EXT-IPS:port_id": "951ba323-3eae-4c41-bd0f-cf61f1f435a4",
        "OS-EXT-IPS:type": "fixed"
      }
    ]
  },
  "id": "77140bce-4f2e-454a-9221-940bebf5cf17",
  "name": "onevm",
  "accessIPv4": "",
  "accessIPv6": "",
  "created": "2021-04-20T10:15:36Z",
  "tags": [
    "1=2",
    "Name=onevm",
    "identifier=test"
  ],
  "description": "",
  "locked": false,
  "config_drive": "",
  "tenant_id": "0bdfeaacas80f5eb2ff3c00f6512b576",
  "user_id": "0bea661fd40w0fc61fc1c00faa56b700",
  "host_status": "UP",
  "enterprise_project_id": "0",
  "sys_tags": [
    {
      "key": "_sys_enterprise_project_id",
      "value": "0"
    }
  ],
  "flavor": {
    "disk": "0",
    "vcpus": "2",
    "ram": "4096",
    "id": "c3.large.2",
    "name": "c3.large.2"
  },
  "metadata": {
    "charging_mode": "0",
    "metering.order_id": "",
    "metering.product_id": "",
    "vpc_id": "bc8dqw89-8b85-4ca0-a0ac-9f680b615208",
    "EcmResStatus": "",
    "metering.image_id": "89525832-eab6-432e-aed1-574dd8196810",
    "metering.imagetype": "gold",
    "metering.resourcespeccode": "c3.large.2.linux",
    "image_name": "CentOS 7.9 64bit",
    "os_bit": "64",
    "lock_check_endpoint": "",
    "lock_source": "",
    "lock_source_id": "",
    "lock_scene": "",
    "virtual_env_type": "",
    "agency_name": ""
  },
  "security_groups": [
    {
      "name": "one-vm-sg"
    }
  ],
  "key_name": "test",
  "image": {
    "id": "89as58fd-eab6-432e-aed1-574dd8196810"
  },
  "progress": 0,
  "OS-EXT-STS:power_state": 1,
  "OS-EXT-STS:vm_state": "active",
  "OS-EXT-STS:task_state": "",
  "OS-DCF:diskConfig": "MANUAL",
  "OS-EXT-AZ:availability_zone": "ap-southeast-2a",
  "OS-SRV-USG:launched_at": "2021-04-20T10:15:49.000000",
  "OS-SRV-USG:terminated_at": "",
  "OS-EXT-SRV-ATTR:root_device_name": "/dev/vda",
  "OS-EXT-SRV-ATTR:ramdisk_id": "",
  "OS-EXT-SRV-ATTR:kernel_id": "",
  "OS-EXT-SRV-ATTR:launch_index": 0,
  "OS-EXT-SRV-ATTR:reservation_id": "r-0jq4kv5m",
  "OS-EXT-SRV-ATTR:hostname": "onevm",
  "OS-EXT-SRV-ATTR:user_data": "IyEvdXNyL2Jpbi9lbnYgYmFzaAplY2hvICJ0ZXN0Ig==",
  "OS-EXT-SRV-ATTR:host": "3f88ca6718fc7484sda5a346c3108c1cbfec8864baece143d43e4a63",
  "OS-EXT-SRV-ATTR:instance_name": "instance-001ea76f",
  "OS-EXT-SRV-ATTR:hypervisor_hostname": "8499bewc8838c550f9bf75978c491beb7fef7ec1d625511cd0fa9a1",
  "os-extended-volumes:volumes_attached": [
    {
      "id": "86b1c53d-3f76-4cf1-aa85-qw602e6d6493",
      "delete_on_termination": "true",
      "bootIndex": "0",
      "device": "/dev/vda"
    },
    {
      "id": "be7bdd3a-2e17-47we3-beb9-319f0c8c7507",
      "delete_on_termination": "false",
      "bootIndex": "",
      "device": "/dev/vdb"
    }
  ],
  "os:scheduler_hints": {
    "group": null
  }
}]
}`,
		)
	})
}
