// Copyright 2017 The Prometheus Authors
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

package openstack

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// SDMock is the interface for the OpenStack mock.
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

const tokenID = "cbc36478b0bd8e67e89469c7749d4127"

func testMethod(t *testing.T, r *http.Request, expected string) {
	require.Equal(t, expected, r.Method, "Unexpected request method.")
}

func testHeader(t *testing.T, r *http.Request, header, expected string) {
	t.Helper()
	actual := r.Header.Get(header)
	require.Equal(t, expected, actual, "Unexpected value for request header %s.", header)
}

// HandleVersionsSuccessfully mocks version call.
func (m *SDMock) HandleVersionsSuccessfully() {
	m.Mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `
                        {
                                "versions": {
                                        "values": [
                                                {
                                                        "status": "stable",
                                                        "id": "v3.0",
                                                        "links": [
                                                                { "href": "%s", "rel": "self" }
                                                        ]
                                                },
                                                {
                                                        "status": "stable",
                                                        "id": "v2.0",
                                                        "links": [
                                                                { "href": "%s", "rel": "self" }
                                                        ]
                                                }
                                        ]
                                }
                        }
                `, m.Endpoint()+"v3/", m.Endpoint()+"v2.0/")
	})
}

// HandleAuthSuccessfully mocks auth call.
func (m *SDMock) HandleAuthSuccessfully() {
	m.Mux.HandleFunc("/v3/auth/tokens", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("X-Subject-Token", tokenID)

		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `
	{
    "token": {
        "audit_ids": ["VcxU2JYqT8OzfUVvrjEITQ", "qNUTIJntTzO1-XUk5STybw"],
        "catalog": [
            {
                "endpoints": [
                    {
                        "id": "39dc322ce86c4111b4f06c2eeae0841b",
                        "interface": "public",
                        "region": "RegionOne",
                        "url": "http://localhost:5000"
                    },
                    {
                        "id": "ec642f27474842e78bf059f6c48f4e99",
                        "interface": "internal",
                        "region": "RegionOne",
                        "url": "http://localhost:5000"
                    },
                    {
                        "id": "c609fc430175452290b62a4242e8a7e8",
                        "interface": "admin",
                        "region": "RegionOne",
                        "url": "http://localhost:35357"
                    }
                ],
                "id": "4363ae44bdf34a3981fde3b823cb9aa2",
                "type": "identity",
                "name": "keystone"
            },
            {
                "endpoints": [
                    {
                        "id": "e2ffee808abc4a60916715b1d4b489dd",
                        "interface": "public",
                        "region": "RegionOne",
                        "region_id": "RegionOne",
                        "url": "%s"
                    }
                ],
                "id": "b7f2a5b1a019459cb956e43a8cb41e31",
                "type": "compute"
            },
            {
                "endpoints": [
                    {
                        "id": "5448e46679564d7d95466c2bef54c296",
                        "interface": "public",
                        "region": "RegionOne",
                        "region_id": "RegionOne",
                        "url": "%s"
                    }
                ],
                "id": "589f3d99a3d94f5f871e9f5cf206d2e8",
                "type": "network"
            },
            {
                "endpoints": [
                    {
                        "id": "39dc322ce86c1234b4f06c2eeae0841b",
                        "interface": "public",
                        "region": "RegionOne",
                        "region_id": "RegionOne",
                        "url": "%s"
                    }
                ],
                "id": "26968f704a68417bbddd29508455ff90",
                "type": "load-balancer"
            }
        ],
        "expires_at": "2013-02-27T18:30:59.999999Z",
        "is_domain": false,
        "issued_at": "2013-02-27T16:30:59.999999Z",
        "methods": [
            "password"
        ],
        "project": {
            "domain": {
                "id": "1789d1",
                "name": "example.com"
            },
            "id": "263fd9",
            "name": "project-x"
        },
        "roles": [
            {
                "id": "76e72a",
                "name": "admin"
            },
            {
                "id": "f4f392",
                "name": "member"
            }
        ],
        "user": {
            "domain": {
                "id": "1789d1",
                "name": "example.com"
            },
            "id": "0ca8f6",
            "name": "Joe",
            "password_expires_at": "2016-11-06T15:32:17.000000"
        }
    }
}
	`, m.Endpoint(), m.Endpoint(), m.Endpoint())
	})
}

const hypervisorListBody = `
{
    "hypervisors": [
        {
            "status": "enabled",
            "service": {
                "host": "nc14.cloud.com",
                "disabled_reason": null,
                "id": 16
            },
            "vcpus_used": 18,
            "hypervisor_type": "QEMU",
            "local_gb_used": 84,
            "vcpus": 24,
            "hypervisor_hostname": "nc14.cloud.com",
            "memory_mb_used": 24064,
            "memory_mb": 96484,
            "current_workload": 1,
            "state": "up",
            "host_ip": "172.16.70.14",
            "cpu_info": "{\"vendor\": \"Intel\", \"model\": \"IvyBridge\", \"arch\": \"x86_64\", \"features\": [\"pge\", \"avx\", \"clflush\", \"sep\", \"syscall\", \"vme\", \"dtes64\", \"msr\", \"fsgsbase\", \"xsave\", \"vmx\", \"erms\", \"xtpr\", \"cmov\", \"smep\", \"ssse3\", \"est\", \"pat\", \"monitor\", \"smx\", \"pbe\", \"lm\", \"tsc\", \"nx\", \"fxsr\", \"tm\", \"sse4.1\", \"pae\", \"sse4.2\", \"pclmuldq\", \"acpi\", \"tsc-deadline\", \"mmx\", \"osxsave\", \"cx8\", \"mce\", \"de\", \"tm2\", \"ht\", \"dca\", \"lahf_lm\", \"popcnt\", \"mca\", \"pdpe1gb\", \"apic\", \"sse\", \"f16c\", \"pse\", \"ds\", \"invtsc\", \"pni\", \"rdtscp\", \"aes\", \"sse2\", \"ss\", \"ds_cpl\", \"pcid\", \"fpu\", \"cx16\", \"pse36\", \"mtrr\", \"pdcm\", \"rdrand\", \"x2apic\"], \"topology\": {\"cores\": 6, \"cells\": 2, \"threads\": 2, \"sockets\": 1}}",
            "running_vms": 10,
            "free_disk_gb": 315,
            "hypervisor_version": 2003000,
            "disk_available_least": 304,
            "local_gb": 399,
            "free_ram_mb": 72420,
            "id": 1
        },
        {
            "status": "enabled",
            "service": {
                "host": "cc13.cloud.com",
                "disabled_reason": null,
                "id": 17
            },
            "vcpus_used": 1,
            "hypervisor_type": "QEMU",
            "local_gb_used": 20,
            "vcpus": 24,
            "hypervisor_hostname": "cc13.cloud.com",
            "memory_mb_used": 2560,
            "memory_mb": 96484,
            "current_workload": 0,
            "state": "up",
            "host_ip": "172.16.70.13",
            "cpu_info": "{\"vendor\": \"Intel\", \"model\": \"IvyBridge\", \"arch\": \"x86_64\", \"features\": [\"pge\", \"avx\", \"clflush\", \"sep\", \"syscall\", \"vme\", \"dtes64\", \"msr\", \"fsgsbase\", \"xsave\", \"vmx\", \"erms\", \"xtpr\", \"cmov\", \"smep\", \"ssse3\", \"est\", \"pat\", \"monitor\", \"smx\", \"pbe\", \"lm\", \"tsc\", \"nx\", \"fxsr\", \"tm\", \"sse4.1\", \"pae\", \"sse4.2\", \"pclmuldq\", \"acpi\", \"tsc-deadline\", \"mmx\", \"osxsave\", \"cx8\", \"mce\", \"de\", \"tm2\", \"ht\", \"dca\", \"lahf_lm\", \"popcnt\", \"mca\", \"pdpe1gb\", \"apic\", \"sse\", \"f16c\", \"pse\", \"ds\", \"invtsc\", \"pni\", \"rdtscp\", \"aes\", \"sse2\", \"ss\", \"ds_cpl\", \"pcid\", \"fpu\", \"cx16\", \"pse36\", \"mtrr\", \"pdcm\", \"rdrand\", \"x2apic\"], \"topology\": {\"cores\": 6, \"cells\": 2, \"threads\": 2, \"sockets\": 1}}",
            "running_vms": 0,
            "free_disk_gb": 379,
            "hypervisor_version": 2003000,
            "disk_available_least": 384,
            "local_gb": 399,
            "free_ram_mb": 93924,
            "id": 721
        }
    ]
}`

// HandleHypervisorListSuccessfully mocks os-hypervisors detail call.
func (m *SDMock) HandleHypervisorListSuccessfully() {
	m.Mux.HandleFunc("/os-hypervisors/detail", func(w http.ResponseWriter, r *http.Request) {
		testMethod(m.t, r, http.MethodGet)
		testHeader(m.t, r, "X-Auth-Token", tokenID)

		w.Header().Add("Content-Type", "application/json")
		fmt.Fprint(w, hypervisorListBody)
	})
}

const serverListBody = `
{
	"servers": [
		{
			"status": "ERROR",
			"updated": "2014-09-25T13:10:10Z",
			"hostId": "29d3c8c896a45aa4c34e52247875d7fefc3d94bbcc9f622b5d204362",
			"OS-EXT-SRV-ATTR:host": "devstack",
			"addresses": {},
			"links": [
				{
					"href": "http://104.130.131.164:8774/v2/fcad67a6189847c4aecfa3c81a05783b/servers/af9bcad9-3c87-477d-9347-b291eabf480e",
					"rel": "self"
				},
				{
					"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/servers/af9bcad9-3c87-477d-9347-b291eabf480e",
					"rel": "bookmark"
				}
			],
			"key_name": null,
			"image": {
				"id": "f90f6034-2570-4974-8351-6b49732ef2eb",
				"links": [
					{
						"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/images/f90f6034-2570-4974-8351-6b49732ef2eb",
						"rel": "bookmark"
					}
				]
			},
			"OS-EXT-STS:task_state": null,
			"OS-EXT-STS:vm_state": "error",
			"OS-EXT-SRV-ATTR:instance_name": "instance-00000010",
			"OS-SRV-USG:launched_at": "2014-09-25T13:10:10.000000",
			"OS-EXT-SRV-ATTR:hypervisor_hostname": "devstack",
			"flavor": {
				"id": "1",
				"links": [
					{
						"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/flavors/1",
						"rel": "bookmark"
					}
				]
			},
			"id": "af9bcad9-3c87-477d-9347-b291eabf480e",
			"security_groups": [
				{
					"name": "default"
				}
			],
			"OS-SRV-USG:terminated_at": null,
			"OS-EXT-AZ:availability_zone": "nova",
			"user_id": "9349aff8be7545ac9d2f1d00999a23cd",
			"name": "herp2",
			"created": "2014-09-25T13:10:02Z",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"OS-DCF:diskConfig": "MANUAL",
			"os-extended-volumes:volumes_attached": [],
			"accessIPv4": "",
			"accessIPv6": "",
			"progress": 0,
			"OS-EXT-STS:power_state": 1,
			"config_drive": "",
			"metadata": {}
		},
		{
			"status": "ACTIVE",
			"updated": "2014-09-25T13:10:10Z",
			"hostId": "29d3c8c896a45aa4c34e52247875d7fefc3d94bbcc9f622b5d204362",
			"OS-EXT-SRV-ATTR:host": "devstack",
			"addresses": {
				"private": [
					{
						"OS-EXT-IPS-MAC:mac_addr": "fa:16:3e:7c:1b:2b",
						"version": 4,
						"addr": "10.0.0.32",
						"OS-EXT-IPS:type": "fixed"
					},
					{
						"version": 4,
						"addr": "10.10.10.2",
						"OS-EXT-IPS:type": "floating"
					}
				]
			},
			"links": [
				{
					"href": "http://104.130.131.164:8774/v2/fcad67a6189847c4aecfa3c81a05783b/servers/ef079b0c-e610-4dfb-b1aa-b49f07ac48e5",
					"rel": "self"
				},
				{
					"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/servers/ef079b0c-e610-4dfb-b1aa-b49f07ac48e5",
					"rel": "bookmark"
				}
			],
			"key_name": null,
			"image": {
				"id": "f90f6034-2570-4974-8351-6b49732ef2eb",
				"links": [
					{
						"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/images/f90f6034-2570-4974-8351-6b49732ef2eb",
						"rel": "bookmark"
					}
				]
			},
			"OS-EXT-STS:task_state": null,
			"OS-EXT-STS:vm_state": "active",
			"OS-EXT-SRV-ATTR:instance_name": "instance-0000001e",
			"OS-SRV-USG:launched_at": "2014-09-25T13:10:10.000000",
			"OS-EXT-SRV-ATTR:hypervisor_hostname": "devstack",
			"flavor": {
				"id": "1",
				"links": [
					{
						"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/flavors/1",
						"rel": "bookmark"
					}
				]
			},
			"id": "ef079b0c-e610-4dfb-b1aa-b49f07ac48e5",
			"security_groups": [
				{
					"name": "default"
				}
			],
			"OS-SRV-USG:terminated_at": null,
			"OS-EXT-AZ:availability_zone": "nova",
			"user_id": "9349aff8be7545ac9d2f1d00999a23cd",
			"name": "herp",
			"created": "2014-09-25T13:10:02Z",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"OS-DCF:diskConfig": "MANUAL",
			"os-extended-volumes:volumes_attached": [],
			"accessIPv4": "",
			"accessIPv6": "",
			"progress": 0,
			"OS-EXT-STS:power_state": 1,
			"config_drive": "",
			"metadata": {}
		},
		{
			"status": "ACTIVE",
			"updated": "2014-09-25T13:04:49Z",
			"hostId": "29d3c8c896a45aa4c34e52247875d7fefc3d94bbcc9f622b5d204362",
			"OS-EXT-SRV-ATTR:host": "devstack",
			"addresses": {
				"private": [
					{
						"OS-EXT-IPS-MAC:mac_addr": "fa:16:3e:9e:89:be",
						"version": 4,
						"addr": "10.0.0.31",
						"OS-EXT-IPS:type": "fixed"
					}
				]
			},
			"links": [
				{
					"href": "http://104.130.131.164:8774/v2/fcad67a6189847c4aecfa3c81a05783b/servers/9e5476bd-a4ec-4653-93d6-72c93aa682ba",
					"rel": "self"
				},
				{
					"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/servers/9e5476bd-a4ec-4653-93d6-72c93aa682ba",
					"rel": "bookmark"
				}
			],
			"key_name": null,
			"image": {
				"id": "f90f6034-2570-4974-8351-6b49732ef2eb",
				"links": [
					{
						"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/images/f90f6034-2570-4974-8351-6b49732ef2eb",
						"rel": "bookmark"
					}
				]
			},
			"OS-EXT-STS:task_state": null,
			"OS-EXT-STS:vm_state": "active",
			"OS-EXT-SRV-ATTR:instance_name": "instance-0000001d",
			"OS-SRV-USG:launched_at": "2014-09-25T13:04:49.000000",
			"OS-EXT-SRV-ATTR:hypervisor_hostname": "devstack",
			"flavor": {
				"vcpus": 2,
				"ram": 4096,
				"disk": 0,
				"ephemeral": 0,
				"swap": 0,
				"original_name": "m1.medium",
				"extra_specs": {
					"aggregate_instance_extra_specs:general": "true",
					"hw:mem_page_size": "large",
					"hw:vif_multiqueue_enabled": "true"
				}
			},
			"id": "9e5476bd-a4ec-4653-93d6-72c93aa682ba",
			"security_groups": [
				{
					"name": "default"
				}
			],
			"OS-SRV-USG:terminated_at": null,
			"OS-EXT-AZ:availability_zone": "nova",
			"user_id": "9349aff8be7545ac9d2f1d00999a23cd",
			"name": "derp",
			"created": "2014-09-25T13:04:41Z",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"OS-DCF:diskConfig": "MANUAL",
			"os-extended-volumes:volumes_attached": [],
			"accessIPv4": "",
			"accessIPv6": "",
			"progress": 0,
			"OS-EXT-STS:power_state": 1,
			"config_drive": "",
			"metadata": {}
		},
		{
			"status": "ACTIVE",
			"updated": "2014-09-25T13:04:49Z",
			"hostId": "29d3c8c896a45aa4c34e52247875d7fefc3d94bbcc9f622b5d204362",
			"OS-EXT-SRV-ATTR:host": "devstack",
			"addresses": {
				"private": [
					{
						"version": 4,
						"addr": "10.0.0.33",
						"OS-EXT-IPS:type": "fixed"
					},
					{
						"version": 4,
						"addr": "10.0.0.34",
						"OS-EXT-IPS:type": "fixed"
					},
					{
						"version": 4,
						"addr": "10.10.10.4",
						"OS-EXT-IPS:type": "floating"
					}
				]
			},
			"links": [
				{
					"href": "http://104.130.131.164:8774/v2/fcad67a6189847c4aecfa3c81a05783b/servers/9e5476bd-a4ec-4653-93d6-72c93aa682ba",
					"rel": "self"
				},
				{
					"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/servers/9e5476bd-a4ec-4653-93d6-72c93aa682ba",
					"rel": "bookmark"
				}
			],
			"key_name": null,
			"image": "",
			"OS-EXT-STS:task_state": null,
			"OS-EXT-STS:vm_state": "active",
			"OS-EXT-SRV-ATTR:instance_name": "instance-0000001d",
			"OS-SRV-USG:launched_at": "2014-09-25T13:04:49.000000",
			"OS-EXT-SRV-ATTR:hypervisor_hostname": "devstack",
			"flavor": {
				"vcpus": 2,
				"ram": 4096,
				"disk": 0,
				"ephemeral": 0,
				"swap": 0,
				"original_name": "m1.small",
				"extra_specs": {
				"aggregate_instance_extra_specs:general": "true",
				"hw:mem_page_size": "large",
				"hw:vif_multiqueue_enabled": "true"
				}
			},
			"id": "9e5476bd-a4ec-4653-93d6-72c93aa682bb",
			"security_groups": [
				{
					"name": "default"
				}
			],
			"OS-SRV-USG:terminated_at": null,
			"OS-EXT-AZ:availability_zone": "nova",
			"user_id": "9349aff8be7545ac9d2f1d00999a23cd",
			"name": "merp",
			"created": "2014-09-25T13:04:41Z",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"OS-DCF:diskConfig": "MANUAL",
			"os-extended-volumes:volumes_attached": [],
			"accessIPv4": "",
			"accessIPv6": "",
			"progress": 0,
			"OS-EXT-STS:power_state": 1,
			"config_drive": "",
			"metadata": {
				"env": "prod"
			}
		},
		{
			"status": "ACTIVE",
			"updated": "2014-09-25T13:04:49Z",
			"hostId": "29d3c8c896a45aa4c34e52247875d7fefc3d94bbcc9f622b5d204362",
			"OS-EXT-SRV-ATTR:host": "devstack",
			"addresses": {
				"private": [
					{
						"version": 4,
						"addr": "10.0.0.33",
						"OS-EXT-IPS:type": "fixed"
					},
					{
						"version": 4,
						"addr": "10.0.0.34",
						"OS-EXT-IPS:type": "fixed"
					},
					{
						"version": 4,
						"addr": "10.10.10.24",
						"OS-EXT-IPS:type": "floating"
					}
				]
			},
			"links": [
				{
					"href": "http://104.130.131.164:8774/v2/b78fef2305934dbbbeb9a10b4c326f7a/servers/9e5476bd-a4ec-4653-93d6-72c93aa682ba",
					"rel": "self"
				},
				{
					"href": "http://104.130.131.164:8774/b78fef2305934dbbbeb9a10b4c326f7a/servers/9e5476bd-a4ec-4653-93d6-72c93aa682ba",
					"rel": "bookmark"
				}
			],
			"key_name": null,
			"image": "",
			"OS-EXT-STS:task_state": null,
			"OS-EXT-STS:vm_state": "active",
			"OS-EXT-SRV-ATTR:instance_name": "instance-0000002d",
			"OS-SRV-USG:launched_at": "2014-09-25T13:04:49.000000",
			"OS-EXT-SRV-ATTR:hypervisor_hostname": "devstack",
			"flavor": {
				"vcpus": 2,
				"ram": 4096,
				"disk": 0,
				"ephemeral": 0,
				"swap": 0,
				"original_name": "m1.small",
				"extra_specs": {
				"aggregate_instance_extra_specs:general": "true",
				"hw:mem_page_size": "large",
				"hw:vif_multiqueue_enabled": "true"
				}
			},
			"id": "87caf8ed-d92a-41f6-9dcd-d1399e39899f",
			"security_groups": [
				{
					"name": "default"
				}
			],
			"OS-SRV-USG:terminated_at": null,
			"OS-EXT-AZ:availability_zone": "nova",
			"user_id": "9349aff8be7545ac9d2f1d00999a23cd",
			"name": "merp-project2",
			"created": "2014-09-25T13:04:41Z",
			"tenant_id": "b78fef2305934dbbbeb9a10b4c326f7a",
			"OS-DCF:diskConfig": "MANUAL",
			"os-extended-volumes:volumes_attached": [],
			"accessIPv4": "",
			"accessIPv6": "",
			"progress": 0,
			"OS-EXT-STS:power_state": 1,
			"config_drive": "",
			"metadata": {
				"env": "prod"
			}
		}
	]
}
`

// HandleServerListSuccessfully mocks server detail call.
func (m *SDMock) HandleServerListSuccessfully() {
	m.Mux.HandleFunc("/servers/detail", func(w http.ResponseWriter, r *http.Request) {
		testMethod(m.t, r, http.MethodGet)
		testHeader(m.t, r, "X-Auth-Token", tokenID)

		w.Header().Add("Content-Type", "application/json")
		fmt.Fprint(w, serverListBody)
	})
}

const listOutput = `
{
	"floatingips": [
		{
			"id": "03a77860-ae03-46c4-b502-caea11467a79",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"floating_ip_address": "10.10.10.1",
			"floating_network_id": "d02c4f18-d606-4864-b12a-1c9b39a46be2",
			"router_id": "f03af93b-4e8f-4f55-adcf-a0317782ede2",
			"port_id": "d5597901-48c8-4a69-a041-cfc5be158a04",
			"fixed_ip_address": null,
			"status": "ACTIVE",
			"description": "",
			"dns_domain": "",
			"dns_name": "",
			"port_forwardings": [],
			"tags": [],
			"created_at": "2023-08-30T16:30:27Z",
			"updated_at": "2023-08-30T16:30:28Z"
		},
		{
			"id": "03e28c79-5a4c-491e-a4fe-3ff6bba830c6",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"floating_ip_address": "10.10.10.2",
			"floating_network_id": "d02c4f18-d606-4864-b12a-1c9b39a46be2",
			"router_id": "f03af93b-4e8f-4f55-adcf-a0317782ede2",
			"port_id": "4a45b012-0478-484d-8cf3-c8abdb194d08",
			"fixed_ip_address": "10.0.0.32",
			"status": "ACTIVE",
			"description": "",
			"dns_domain": "",
			"dns_name": "",
			"port_forwardings": [],
			"tags": [],
			"created_at": "2023-09-06T15:45:36Z",
			"updated_at": "2023-09-06T15:45:36Z"
		},
		{
			"id": "087fcdd2-1d13-4f72-9c0e-c759e796d558",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"floating_ip_address": "10.10.10.4",
			"floating_network_id": "d02c4f18-d606-4864-b12a-1c9b39a46be2",
			"router_id": "f03af93b-4e8f-4f55-adcf-a0317782ede2",
			"port_id": "a0e244e8-7910-4427-b8d1-20470cad4f8a",
			"fixed_ip_address": "10.0.0.34",
			"status": "ACTIVE",
			"description": "",
			"dns_domain": "",
			"dns_name": "",
			"port_forwardings": [],
			"tags": [],
			"created_at": "2024-01-24T13:30:50Z",
			"updated_at": "2024-01-24T13:30:51Z"
		},
		{
			"id": "b23df91a-a74a-4f75-b252-750aff4a5a0c",
			"tenant_id": "b78fef2305934dbbbeb9a10b4c326f7a",
			"floating_ip_address": "10.10.10.24",
			"floating_network_id": "b19ff5bc-a49a-46cc-8d14-ca5f1e94791f",
			"router_id": "65a5e5af-17f0-4124-9a81-c08b44f5b8a7",
			"port_id": "b926ab68-ec54-46d8-8c50-1c07aafd5ae9",
			"fixed_ip_address": "10.0.0.34",
			"status": "ACTIVE",
			"description": "",
			"dns_domain": "",
			"dns_name": "",
			"port_forwardings": [],
			"tags": [],
			"created_at": "2024-01-24T13:30:50Z",
			"updated_at": "2024-01-24T13:30:51Z"
		},
		{
			"id": "fea7332d-9027-4cf9-bf62-c3c4c6ebaf84",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"floating_ip_address": "192.168.1.2",
			"floating_network_id": "d02c4f18-d606-4864-b12a-1c9b39a46be2",
			"router_id": "f03af93b-4e8f-4f55-adcf-a0317782ede2",
			"port_id": "b47c39f5-238d-4b17-ae87-9b5d19af8a2e",
			"fixed_ip_address": "10.0.0.32",
			"status": "ACTIVE",
			"description": "",
			"dns_domain": "",
			"dns_name": "",
			"port_forwardings": [],
			"tags": [],
			"created_at": "2023-08-30T15:11:37Z",
			"updated_at": "2023-08-30T15:11:38Z",
			"revision_number": 1,
			"project_id": "fcad67a6189847c4aecfa3c81a05783b"
		},
		{
			"id": "febb9554-cf83-4f9b-94d9-1b3c34be357f",
			"tenant_id": "ac57f03dba1a4fdebff3e67201bc7a85",
			"floating_ip_address": "192.168.3.4",
			"floating_network_id": "d02c4f18-d606-4864-b12a-1c9b39a46be2",
			"router_id": "f03af93b-4e8f-4f55-adcf-a0317782ede2",
			"port_id": "c83b6e12-4e5d-4673-a4b3-5bc72a7f3ef9",
			"fixed_ip_address": "10.0.2.78",
			"status": "ACTIVE",
			"description": "",
			"dns_domain": "",
			"dns_name": "",
			"port_forwardings": [],
			"tags": [],
			"created_at": "2023-08-30T15:11:37Z",
			"updated_at": "2023-08-30T15:11:38Z",
			"revision_number": 1,
			"project_id": "ac57f03dba1a4fdebff3e67201bc7a85"
		},
		{
			"id": "febb9554-cf83-4f9b-94d9-1b3c34be357f",
			"tenant_id": "fa8c372dfe4d4c92b0c4e3a2d9b3c9fa",
			"floating_ip_address": "192.168.4.5",
			"floating_network_id": "d02c4f18-d606-4864-b12a-1c9b39a46be2",
			"router_id": "f03af93b-4e8f-4f55-adcf-a0317782ede2",
			"port_id": "f9e8b6e12-7e4d-4963-a5b3-6cd82a7f3ff6",
			"fixed_ip_address": "10.0.3.99",
			"status": "ACTIVE",
			"description": "",
			"dns_domain": "",
			"dns_name": "",
			"port_forwardings": [],
			"tags": [],
			"created_at": "2023-08-30T15:11:37Z",
			"updated_at": "2023-08-30T15:11:38Z",
			"revision_number": 1,
			"project_id": "fa8c372dfe4d4c92b0c4e3a2d9b3c9fa"
		}
	]
}
`

// HandleFloatingIPListSuccessfully mocks floating ips call.
func (m *SDMock) HandleFloatingIPListSuccessfully() {
	m.Mux.HandleFunc("/v2.0/floatingips", func(w http.ResponseWriter, r *http.Request) {
		testMethod(m.t, r, http.MethodGet)
		testHeader(m.t, r, "X-Auth-Token", tokenID)

		w.Header().Add("Content-Type", "application/json")
		fmt.Fprint(w, listOutput)
	})
}

const portsListBody = `
{
	"ports": [
		{
			"id": "d5597901-48c8-4a69-a041-cfc5be158a04",
			"name": "",
			"network_id": "d02c4f18-d606-4864-b12a-1c9b39a46be2",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"mac_address": "",
			"admin_state_up": true,
			"status": "DOWN",
			"device_id": "",
			"device_owner": "",
			"fixed_ips": [],
			"allowed_address_pairs": [],
			"extra_dhcp_opts": [],
			"security_groups": [],
			"description": "",
			"binding:vnic_type": "normal",
			"port_security_enabled": true,
			"dns_name": "",
			"dns_assignment": [],
			"dns_domain": "",
			"tags": [],
			"created_at": "2023-08-30T16:30:27Z",
			"updated_at": "2023-08-30T16:30:28Z",
			"revision_number": 0,
			"project_id": "fcad67a6189847c4aecfa3c81a05783b"
		},
		{
			"id": "4a45b012-0478-484d-8cf3-c8abdb194d08",
			"name": "ovn-lb-vip-0980c8de-58c3-481d-89e3-ed81f44286c0",
			"network_id": "03200a39-b399-44f3-a778-6dbb93343a31",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"mac_address": "fa:16:3e:23:12:a3",
			"admin_state_up": true,
			"status": "ACTIVE",
			"device_id": "ef079b0c-e610-4dfb-b1aa-b49f07ac48e5",
			"device_owner": "",
			"fixed_ips": [
				{
					"subnet_id": "",
					"ip_address": "10.10.10.2"
				}
			],
			"allowed_address_pairs": [],
			"extra_dhcp_opts": [],
			"security_groups": [],
			"description": "",
			"binding:vnic_type": "normal",
			"port_security_enabled": true,
			"dns_name": "",
			"dns_assignment": [],
			"dns_domain": "",
			"tags": [],
			"created_at": "2023-09-06T15:45:36Z",
			"updated_at": "2023-09-06T15:45:36Z",
			"revision_number": 0,
			"project_id": "fcad67a6189847c4aecfa3c81a05783b"
		},
		{
			"id": "a0e244e8-7910-4427-b8d1-20470cad4f8a",
			"name": "ovn-lb-vip-26c0ccb1-3036-4345-99e8-d8f34a8ba6b2",
			"network_id": "03200a39-b399-44f3-a778-6dbb93343a31",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"mac_address": "fa:16:3e:5f:43:10",
			"admin_state_up": true,
			"status": "ACTIVE",
			"device_id": "9e5476bd-a4ec-4653-93d6-72c93aa682bb",
			"device_owner": "",
			"fixed_ips": [
				{
					"subnet_id": "",
					"ip_address": "10.10.10.4"
				}
			],
			"allowed_address_pairs": [],
			"extra_dhcp_opts": [],
			"security_groups": [],
			"description": "",
			"binding:vnic_type": "normal",
			"port_security_enabled": true,
			"dns_name": "",
			"dns_assignment": [],
			"dns_domain": "",
			"tags": [],
			"created_at": "2024-01-24T13:30:50Z",
			"updated_at": "2024-01-24T13:30:51Z",
			"revision_number": 0,
			"project_id": "fcad67a6189847c4aecfa3c81a05783b"
		},
		{
			"id": "b926ab68-ec54-46d8-8c50-1c07aafd5ae9",
			"name": "dummy-port",
			"network_id": "03200a39-b399-44f3-a778-6dbb93343a31",
			"tenant_id": "b78fef2305934dbbbeb9a10b4c326f7a",
			"mac_address": "fa:16:3e:5f:12:10",
			"admin_state_up": true,
			"status": "ACTIVE",
			"device_id": "87caf8ed-d92a-41f6-9dcd-d1399e39899f",
			"device_owner": "",
			"fixed_ips": [
				{
					"subnet_id": "",
					"ip_address": "10.10.10.24"
				}
			],
			"allowed_address_pairs": [],
			"extra_dhcp_opts": [],
			"security_groups": [],
			"description": "",
			"binding:vnic_type": "normal",
			"port_security_enabled": true,
			"dns_name": "",
			"dns_assignment": [],
			"dns_domain": "",
			"tags": [],
			"created_at": "2024-01-24T13:30:50Z",
			"updated_at": "2024-01-24T13:30:51Z",
			"revision_number": 0,
			"project_id": "b78fef2305934dbbbeb9a10b4c326f7a"
		}
	]
}
`

// HandlePortsListSuccessfully mocks the ports list API.
func (m *SDMock) HandlePortsListSuccessfully() {
	m.Mux.HandleFunc("/v2.0/ports", func(w http.ResponseWriter, r *http.Request) {
		testMethod(m.t, r, http.MethodGet)
		testHeader(m.t, r, "X-Auth-Token", tokenID)

		w.Header().Add("Content-Type", "application/json")
		fmt.Fprint(w, portsListBody)
	})
}

const lbListBody = `
{
	"loadbalancers": [
		{
			"id": "ef079b0c-e610-4dfb-b1aa-b49f07ac48e5",
			"name": "lb1",
			"description": "",
			"provisioning_status": "ACTIVE",
			"operating_status": "ONLINE",
			"admin_state_up": true,
			"project_id": "fcad67a6189847c4aecfa3c81a05783b",
			"created_at": "2024-12-01T10:00:00",
			"updated_at": "2024-12-01T10:30:00",
			"vip_address": "10.0.0.32",
			"vip_port_id": "b47c39f5-238d-4b17-ae87-9b5d19af8a2e",
			"vip_subnet_id": "14a4c6a5-fe71-4a94-9071-4cd12fb8337f",
			"vip_network_id": "d02c4f18-d606-4864-b12a-1c9b39a46be2",
			"tags": ["tag1", "tag2"],
			"availability_zone": "az1",
			"vip_vnic_type": "normal",
			"provider": "amphora",
			"listeners": [
				{
					"id": "c4146b54-febc-4caf-a53f-ed1cab6faba5"
				},
				{
					"id": "a058d20e-82de-4eff-bb65-5c76a8554435"
				}
			],
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b"
		},
		{
			"id": "d92c471e-8d3e-4b9f-b2b5-9c72a9e3ef54",
			"name": "lb3",
			"description": "",
			"provisioning_status": "ACTIVE",
			"operating_status": "ONLINE",
			"admin_state_up": true,
			"project_id": "ac57f03dba1a4fdebff3e67201bc7a85",
			"created_at": "2024-12-01T12:00:00",
			"updated_at": "2024-12-01T12:45:00",
			"vip_address": "10.0.2.78",
			"vip_port_id": "c83b6e12-4e5d-4673-a4b3-5bc72a7f3ef9",
			"vip_subnet_id": "36c5e9f6-e7a2-4975-a8c6-3b8e4f93cf45",
			"vip_network_id": "g03c6f27-e617-4975-c8f7-4c9f3f94cf68",
			"tags": ["tag5", "tag6"],
			"availability_zone": "az3",
			"vip_vnic_type": "normal",
			"provider": "amphora",
			"listeners": [
				{
					"id": "5b9529a4-6cbf-48f8-a006-d99cbc717da0"
				},
				{
					"id": "5d26333b-74d1-4b2a-90ab-2b2c0f5a8048"
				}
			],
			"tenant_id": "ac57f03dba1a4fdebff3e67201bc7a85"
		},
		{
			"id": "f5c7e918-df38-4a5a-a7d4-d9c27ab2cf67",
			"name": "lb4",
			"description": "",
			"provisioning_status": "ACTIVE",
			"operating_status": "ONLINE",
			"admin_state_up": true,
			"project_id": "fa8c372dfe4d4c92b0c4e3a2d9b3c9fa",
			"created_at": "2024-12-01T13:00:00",
			"updated_at": "2024-12-01T13:20:00",
			"vip_address": "10.0.3.99",
			"vip_port_id": "f9e8b6e12-7e4d-4963-a5b3-6cd82a7f3ff6",
			"vip_subnet_id": "47d6f8f9-f7b2-4876-a9d8-4e8f4g95df79",
			"vip_network_id": "h04d7f38-f718-4876-d9g8-5d8g5h95df89",
			"tags": [],
			"availability_zone": "az1",
			"vip_vnic_type": "normal",
			"provider": "amphora",
			"listeners": [
				{
					"id": "84c87596-1ff0-4f6d-b151-0a78e1f407a3"
				},
				{
					"id": "fe460a7c-16a9-4984-9fe6-f6e5153ebab1"
				}
			],
			"tenant_id": "fa8c372dfe4d4c92b0c4e3a2d9b3c9fa"
		},
		{
			"id": "e83a6d92-7a3e-4567-94b3-20c83b32a75e",
			"name": "lb5",
			"description": "",
			"provisioning_status": "ACTIVE",
			"operating_status": "ONLINE",
			"admin_state_up": true,
			"project_id": "a5d3b2e1e6f34cd9a5f7c2f01a6b8e29",
			"created_at": "2024-12-01T11:00:00",
			"updated_at": "2024-12-01T11:15:00",
			"vip_address": "10.0.4.88",
			"vip_port_id": "d83a6d92-7a3e-4567-94b3-20c83b32a75e",
			"vip_subnet_id": "25b4d8e5-fe81-4a87-9071-4cc12fb8337f",
			"vip_network_id": "f02c5e19-c507-4864-b16e-2b7a39e56be3",
			"tags": [],
			"availability_zone": "az4",
			"vip_vnic_type": "normal",
			"provider": "amphora",
			"listeners": [
				{
					"id": "50902e62-34b8-46b2-9ed4-9053e7ad46dc"
				},
				{
					"id": "98a867ad-ff07-4880-b05f-32088866a68a"
				}
			],
			"tenant_id": "a5d3b2e1e6f34cd9a5f7c2f01a6b8e29"
		}
	]
}
`

// HandleLoadBalancerListSuccessfully mocks the load balancer list API.
func (m *SDMock) HandleLoadBalancerListSuccessfully() {
	m.Mux.HandleFunc("/v2.0/lbaas/loadbalancers", func(w http.ResponseWriter, r *http.Request) {
		testMethod(m.t, r, http.MethodGet)
		testHeader(m.t, r, "X-Auth-Token", tokenID)

		w.Header().Add("Content-Type", "application/json")
		fmt.Fprint(w, lbListBody)
	})
}

const listenerListBody = `
{
	"listeners": [
		{
			"id": "c4146b54-febc-4caf-a53f-ed1cab6faba5",
			"name": "stats-listener",
			"description": "",
			"provisioning_status": "ACTIVE",
			"operating_status": "ONLINE",
			"admin_state_up": true,
			"protocol": "PROMETHEUS",
			"protocol_port": 9273,
			"connection_limit": -1,
			"default_tls_container_ref": null,
			"sni_container_refs": [],
			"project_id": "fcad67a6189847c4aecfa3c81a05783b",
			"default_pool_id": null,
			"l7policies": [],
			"insert_headers": {},
			"created_at": "2024-08-29T18:05:24",
			"updated_at": "2024-12-04T21:21:10",
			"loadbalancers": [
				{
				"id": "ef079b0c-e610-4dfb-b1aa-b49f07ac48e5"
				}
			],
			"timeout_client_data": 50000,
			"timeout_member_connect": 5000,
			"timeout_member_data": 50000,
			"timeout_tcp_inspect": 0,
			"tags": [],
			"client_ca_tls_container_ref": null,
			"client_authentication": "NONE",
			"client_crl_container_ref": null,
			"allowed_cidrs": null,
			"tls_ciphers": null,
			"tls_versions": null,
			"alpn_protocols": null,
			"hsts_max_age": null,
			"hsts_include_subdomains": null,
			"hsts_preload": null,
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b"
		},
		{
			"id": "5b9529a4-6cbf-48f8-a006-d99cbc717da0",
			"name": "stats-listener2",
			"description": "",
			"provisioning_status": "ACTIVE",
			"operating_status": "ONLINE",
			"admin_state_up": true,
			"protocol": "PROMETHEUS",
			"protocol_port": 8080,
			"connection_limit": -1,
			"default_tls_container_ref": null,
			"sni_container_refs": [],
			"project_id": "ac57f03dba1a4fdebff3e67201bc7a85",
			"default_pool_id": null,
			"l7policies": [],
			"insert_headers": {},
			"created_at": "2024-08-29T18:05:24",
			"updated_at": "2024-12-04T21:21:10",
			"loadbalancers": [
				{
				"id": "d92c471e-8d3e-4b9f-b2b5-9c72a9e3ef54"
				}
			],
			"timeout_client_data": 50000,
			"timeout_member_connect": 5000,
			"timeout_member_data": 50000,
			"timeout_tcp_inspect": 0,
			"tags": [],
			"client_ca_tls_container_ref": null,
			"client_authentication": "NONE",
			"client_crl_container_ref": null,
			"allowed_cidrs": null,
			"tls_ciphers": null,
			"tls_versions": null,
			"alpn_protocols": null,
			"hsts_max_age": null,
			"hsts_include_subdomains": null,
			"hsts_preload": null,
			"tenant_id": "ac57f03dba1a4fdebff3e67201bc7a85"
		},
		{
			"id": "84c87596-1ff0-4f6d-b151-0a78e1f407a3",
			"name": "stats-listener3",
			"description": "",
			"provisioning_status": "ACTIVE",
			"operating_status": "ONLINE",
			"admin_state_up": true,
			"protocol": "PROMETHEUS",
			"protocol_port": 9090,
			"connection_limit": -1,
			"default_tls_container_ref": null,
			"sni_container_refs": [],
			"project_id": "fa8c372dfe4d4c92b0c4e3a2d9b3c9fa",
			"default_pool_id": null,
			"l7policies": [],
			"insert_headers": {},
			"created_at": "2024-08-29T18:05:24",
			"updated_at": "2024-12-04T21:21:10",
			"loadbalancers": [
				{
				"id": "f5c7e918-df38-4a5a-a7d4-d9c27ab2cf67"
				}
			],
			"timeout_client_data": 50000,
			"timeout_member_connect": 5000,
			"timeout_member_data": 50000,
			"timeout_tcp_inspect": 0,
			"tags": [],
			"client_ca_tls_container_ref": null,
			"client_authentication": "NONE",
			"client_crl_container_ref": null,
			"allowed_cidrs": null,
			"tls_ciphers": null,
			"tls_versions": null,
			"alpn_protocols": null,
			"hsts_max_age": null,
			"hsts_include_subdomains": null,
			"hsts_preload": null,
			"tenant_id": "fa8c372dfe4d4c92b0c4e3a2d9b3c9fa"
		},
		{
			"id": "50902e62-34b8-46b2-9ed4-9053e7ad46dc",
			"name": "stats-listener4",
			"description": "",
			"provisioning_status": "ACTIVE",
			"operating_status": "ONLINE",
			"admin_state_up": true,
			"protocol": "PROMETHEUS",
			"protocol_port": 9876,
			"connection_limit": -1,
			"default_tls_container_ref": null,
			"sni_container_refs": [],
			"project_id": "a5d3b2e1e6f34cd9a5f7c2f01a6b8e29",
			"default_pool_id": null,
			"l7policies": [],
			"insert_headers": {},
			"created_at": "2024-08-29T18:05:24",
			"updated_at": "2024-12-04T21:21:10",
			"loadbalancers": [
				{
				"id": "e83a6d92-7a3e-4567-94b3-20c83b32a75e"
				}
			],
			"timeout_client_data": 50000,
			"timeout_member_connect": 5000,
			"timeout_member_data": 50000,
			"timeout_tcp_inspect": 0,
			"tags": [],
			"client_ca_tls_container_ref": null,
			"client_authentication": "NONE",
			"client_crl_container_ref": null,
			"allowed_cidrs": null,
			"tls_ciphers": null,
			"tls_versions": null,
			"alpn_protocols": null,
			"hsts_max_age": null,
			"hsts_include_subdomains": null,
			"hsts_preload": null,
			"tenant_id": "a5d3b2e1e6f34cd9a5f7c2f01a6b8e29"
		},
		{
			"id": "a058d20e-82de-4eff-bb65-5c76a8554435",
			"name": "port6443",
			"description": "",
			"provisioning_status": "ACTIVE",
			"operating_status": "ONLINE",
			"admin_state_up": true,
			"protocol": "TCP",
			"protocol_port": 6443,
			"connection_limit": -1,
			"default_tls_container_ref": null,
			"sni_container_refs": [],
			"project_id": "a5d3b2e1e6f34cd9a5f7c2f01a6b8e29",
			"default_pool_id": "5643208b-b691-4b1f-a6b8-356f14903e56",
			"l7policies": [],
			"insert_headers": {},
			"created_at": "2024-10-02T19:32:48",
			"updated_at": "2024-12-04T21:44:34",
			"loadbalancers": [
				{
				"id": "ef079b0c-e610-4dfb-b1aa-b49f07ac48e5"
				}
			],
			"timeout_client_data": 50000,
			"timeout_member_connect": 5000,
			"timeout_member_data": 50000,
			"timeout_tcp_inspect": 0,
			"tags": [],
			"client_ca_tls_container_ref": null,
			"client_authentication": "NONE",
			"client_crl_container_ref": null,
			"allowed_cidrs": null,
			"tls_ciphers": null,
			"tls_versions": null,
			"alpn_protocols": null,
			"hsts_max_age": null,
			"hsts_include_subdomains": null,
			"hsts_preload": null,
			"tenant_id": "a5d3b2e1e6f34cd9a5f7c2f01a6b8e29"
		},
		{
			"id": "5d26333b-74d1-4b2a-90ab-2b2c0f5a8048",
			"name": "port6444",
			"description": "",
			"provisioning_status": "ACTIVE",
			"operating_status": "ONLINE",
			"admin_state_up": true,
			"protocol": "TCP",
			"protocol_port": 6444,
			"connection_limit": -1,
			"default_tls_container_ref": null,
			"sni_container_refs": [],
			"project_id": "ac57f03dba1a4fdebff3e67201bc7a85",
			"default_pool_id": "5643208b-b691-4b1f-a6b8-356f14903e56",
			"l7policies": [],
			"insert_headers": {},
			"created_at": "2024-10-02T19:32:48",
			"updated_at": "2024-12-04T21:44:34",
			"loadbalancers": [
				{
				"id": "d92c471e-8d3e-4b9f-b2b5-9c72a9e3ef54"
				}
			],
			"timeout_client_data": 50000,
			"timeout_member_connect": 5000,
			"timeout_member_data": 50000,
			"timeout_tcp_inspect": 0,
			"tags": [],
			"client_ca_tls_container_ref": null,
			"client_authentication": "NONE",
			"client_crl_container_ref": null,
			"allowed_cidrs": null,
			"tls_ciphers": null,
			"tls_versions": null,
			"alpn_protocols": null,
			"hsts_max_age": null,
			"hsts_include_subdomains": null,
			"hsts_preload": null,
			"tenant_id": "ac57f03dba1a4fdebff3e67201bc7a85"
		},
		{
			"id": "fe460a7c-16a9-4984-9fe6-f6e5153ebab1",
			"name": "port6445",
			"description": "",
			"provisioning_status": "ACTIVE",
			"operating_status": "ONLINE",
			"admin_state_up": true,
			"protocol": "TCP",
			"protocol_port": 6445,
			"connection_limit": -1,
			"default_tls_container_ref": null,
			"sni_container_refs": [],
			"project_id": "fa8c372dfe4d4c92b0c4e3a2d9b3c9fa",
			"default_pool_id": "5643208b-b691-4b1f-a6b8-356f14903e56",
			"l7policies": [],
			"insert_headers": {},
			"created_at": "2024-10-02T19:32:48",
			"updated_at": "2024-12-04T21:44:34",
			"loadbalancers": [
				{
				"id": "f5c7e918-df38-4a5a-a7d4-d9c27ab2cf67"
				}
			],
			"timeout_client_data": 50000,
			"timeout_member_connect": 5000,
			"timeout_member_data": 50000,
			"timeout_tcp_inspect": 0,
			"tags": [],
			"client_ca_tls_container_ref": null,
			"client_authentication": "NONE",
			"client_crl_container_ref": null,
			"allowed_cidrs": null,
			"tls_ciphers": null,
			"tls_versions": null,
			"alpn_protocols": null,
			"hsts_max_age": null,
			"hsts_include_subdomains": null,
			"hsts_preload": null,
			"tenant_id": "fa8c372dfe4d4c92b0c4e3a2d9b3c9fa"
		},
		{
			"id": "98a867ad-ff07-4880-b05f-32088866a68a",
			"name": "port6446",
			"description": "",
			"provisioning_status": "ACTIVE",
			"operating_status": "ONLINE",
			"admin_state_up": true,
			"protocol": "TCP",
			"protocol_port": 6446,
			"connection_limit": -1,
			"default_tls_container_ref": null,
			"sni_container_refs": [],
			"project_id": "a5d3b2e1e6f34cd9a5f7c2f01a6b8e29",
			"default_pool_id": "5643208b-b691-4b1f-a6b8-356f14903e56",
			"l7policies": [],
			"insert_headers": {},
			"created_at": "2024-10-02T19:32:48",
			"updated_at": "2024-12-04T21:44:34",
			"loadbalancers": [
				{
				"id": "e83a6d92-7a3e-4567-94b3-20c83b32a75e"
				}
			],
			"timeout_client_data": 50000,
			"timeout_member_connect": 5000,
			"timeout_member_data": 50000,
			"timeout_tcp_inspect": 0,
			"tags": [],
			"client_ca_tls_container_ref": null,
			"client_authentication": "NONE",
			"client_crl_container_ref": null,
			"allowed_cidrs": null,
			"tls_ciphers": null,
			"tls_versions": null,
			"alpn_protocols": null,
			"hsts_max_age": null,
			"hsts_include_subdomains": null,
			"hsts_preload": null,
			"tenant_id": "a5d3b2e1e6f34cd9a5f7c2f01a6b8e29"
		}
	]
}
`

// HandleListenersListSuccessfully mocks the listeners endpoint.
func (m *SDMock) HandleListenersListSuccessfully() {
	m.Mux.HandleFunc("/v2.0/lbaas/listeners", func(w http.ResponseWriter, r *http.Request) {
		testMethod(m.t, r, http.MethodGet)
		testHeader(m.t, r, "X-Auth-Token", tokenID)

		w.Header().Add("Content-Type", "application/json")
		fmt.Fprint(w, listenerListBody)
	})
}
